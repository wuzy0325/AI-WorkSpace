// Package storage 提供 CSV 文件持久化的字节 I/O 实现。
//
// 本文件实现校准 CSV 写入器，负责文件打开、追加写入、关闭等字节 I/O 操作。
// 列布局与数据行格式由 core/calibration.CsvSchema 提供（纯领域知识）。
//
// 这遵循 CLAUDE.md "Constraint Clarifications" 规则 1：
// core 定义格式描述，adapters 负责字节 I/O。
//
// 首行写入 UTF-8 BOM，避免 Excel / 中文 Windows 端打开中文表头（点位编号、α(°) 等）乱码。
// 追加模式仅在文件不存在（新建）时写 BOM，覆盖模式始终写 BOM。
package storage

import (
	"encoding/csv"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"wind-daq/services/api-go/internal/core/calibration"
)

// CalibrationCsvWriter 校准专用 CSV 写入器（字节 I/O 层）
// 每采集一个点就追加写入 CSV，校准结束或异常时 flush
type CalibrationCsvWriter struct {
	mu       sync.Mutex
	file     *os.File
	writer   *csv.Writer
	path     string
	schema   calibration.CsvSchema
	header   []string
	truncate bool // true 时以覆盖模式打开（用于按需全量导出）
}

// NewCalibrationCsvWriter 创建校准 CSV 写入器（追加模式，用于逐点采集）
// config 用于构建 CsvSchema（列布局由校准类型决定）
func NewCalibrationCsvWriter(config calibration.Config) *CalibrationCsvWriter {
	return &CalibrationCsvWriter{
		schema: calibration.NewCsvSchema(config),
	}
}

// NewCalibrationCsvWriterOverwrite 创建按需全量导出用的 CSV 写入器（覆盖模式）
// 每次 Initialize 都会截断已存在的文件，适合 SaveCsv 这类"一次性导出全部结果"场景，
// 避免多次保存到同一路径时 append 模式产生重复数据。
func NewCalibrationCsvWriterOverwrite(config calibration.Config) *CalibrationCsvWriter {
	return &CalibrationCsvWriter{
		schema:   calibration.NewCsvSchema(config),
		truncate: true,
	}
}

// Initialize 初始化 CSV 文件，写入表头
//
// 路径约定：config.SavePath 必须是完整的文件路径（含扩展名，如 .csv）。
// 不再做"无扩展名则视为目录"的二义性兜底——前端统一拼好完整路径传入，
// 调用方（backend / api server）负责在调用前做路径归一（ResolvePath / Abs）。
func (w *CalibrationCsvWriter) Initialize(config calibration.Config) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if config.SavePath == "" {
		return fmt.Errorf("保存路径为空")
	}
	if filepath.Ext(config.SavePath) == "" {
		return fmt.Errorf("保存路径必须是完整的文件路径（含 .csv 扩展名），当前传入: %q", config.SavePath)
	}
	w.schema = calibration.NewCsvSchema(config)

	// 确保目录存在
	dir := filepath.Dir(config.SavePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("创建目录失败: %w", err)
	}

	// 文件路径直接使用 SavePath（已是完整文件路径）
	w.path = config.SavePath

	// 打开文件：覆盖模式用于按需全量导出，追加模式用于逐点采集
	// 追加模式下，文件已存在且非空时不应再写 BOM（会出现在数据行中间），故先判断是否新建。
	// 同时处理 0 字节残留文件：前次 Initialize 在写 BOM 前异常退出会留下空文件，
	// 此时仍需写 BOM，否则 Excel 打开中文表头乱码。
	isNewFile := false
	if w.truncate {
		isNewFile = true
	} else {
		// 追加模式：文件不存在或为空（异常残留）视为新建，需要写 BOM
		if info, err := os.Stat(w.path); err != nil {
			if os.IsNotExist(err) {
				isNewFile = true
			}
		} else if info.Size() == 0 {
			isNewFile = true
		}
	}

	flags := os.O_CREATE | os.O_WRONLY
	if w.truncate {
		flags |= os.O_TRUNC
	} else {
		flags |= os.O_APPEND
	}
	file, err := os.OpenFile(w.path, flags, 0644)
	if err != nil {
		return fmt.Errorf("打开CSV文件失败: %w", err)
	}

	w.file = file
	w.writer = csv.NewWriter(file)

	// 首行写 UTF-8 BOM，避免 Excel / 中文 Windows 端打开中文表头乱码
	// （仅在新文件时写入；追加模式遇到已有文件不再重复写）
	if isNewFile {
		if _, err := file.Write(utf8BOM); err != nil {
			return fmt.Errorf("写入 BOM 失败: %w", err)
		}
	}

	// 写入表头（列布局来自 core 的 CsvSchema）
	w.header = w.schema.BuildHeader()
	if err := w.writer.Write(w.header); err != nil {
		return fmt.Errorf("写入CSV表头失败: %w", err)
	}
	w.writer.Flush()

	return nil
}

// AppendPoint 追加一个数据点到 CSV
func (w *CalibrationCsvWriter) AppendPoint(dataPoint calibration.DataPoint) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.writer == nil {
		return fmt.Errorf("CSV写入器未初始化")
	}

	// 数据行格式来自 core 的 CsvSchema
	record := w.schema.BuildRecord(dataPoint)
	if err := w.writer.Write(record); err != nil {
		return fmt.Errorf("写入CSV数据失败: %w", err)
	}
	w.writer.Flush()

	return nil
}

// Flush 刷新并关闭 CSV 文件
func (w *CalibrationCsvWriter) Flush() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.writer != nil {
		w.writer.Flush()
	}
	if w.file != nil {
		err := w.file.Close()
		w.file = nil
		w.writer = nil
		return err
	}
	return nil
}

// Path 获取 CSV 文件路径
func (w *CalibrationCsvWriter) Path() string {
	return w.path
}
