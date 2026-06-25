// Package storage 提供 CSV 文件持久化的字节 I/O 实现。
//
// 本文件实现校准 CSV 写入器，负责文件打开、追加写入、关闭等字节 I/O 操作。
// 列布局与数据行格式由 core/calibration.CsvSchema 提供（纯领域知识）。
//
// 这遵循 CLAUDE.md "Constraint Clarifications" 规则 1：
// core 定义格式描述，adapters 负责字节 I/O。
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
func (w *CalibrationCsvWriter) Initialize(config calibration.Config) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if config.SavePath == "" {
		return fmt.Errorf("保存路径为空")
	}

	// 确保目录存在
	dir := filepath.Dir(config.SavePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("创建目录失败: %w", err)
	}

	// 构建文件路径
	w.path = config.SavePath
	if filepath.Ext(w.path) == "" {
		w.path = filepath.Join(w.path, fmt.Sprintf("calibration_%s.csv", config.TaskID))
	}

	// 打开文件：覆盖模式用于按需全量导出，追加模式用于逐点采集
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
