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
//
// 七孔双 CSV writer 路由（spec Task 9 + §7.1）：
//   - 七孔按 region+sector 分文件落盘（1 内区 + 6 外区，共 7 个 CSV 文件）
//   - 每个文件有独立的列布局（外区表头 Kθ[n] 中 n 由 sector 替换为具体扇区编号）
//   - 通过 NewCalibrationCsvWriterWithSchema 构造器注入 schema，跳过 Initialize 中的
//     NewCsvSchema 重建——同一 writer 实例不再被多 schema 复用
//   - CalibrationCsvWriter 同时实现 ports.CalibrationCsvWriter 与 ports.CalibrationWriterFactory
//     接口，工厂方法 NewWriter 创建独立 writer 实例并完成 Initialize
package storage

import (
	"encoding/csv"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"wind-daq/services/api-go/internal/core/calibration"
	"wind-daq/services/api-go/internal/ports"
)

// CalibrationCsvWriter 校准专用 CSV 写入器（字节 I/O 层）
// 每采集一个点就追加写入 CSV，校准结束或异常时 flush
//
// 同时实现两个端口：
//   - ports.CalibrationCsvWriter：五孔/三孔/总压/总温单 writer 场景（按 config.Type 重建 schema）
//   - ports.CalibrationWriterFactory：七孔多 writer 场景（按 region+sector 注入 schema）
//
// schemaOverride=true 时 Initialize 跳过 NewCsvSchema 重建，使用外部注入的 schema。
// 这避免七孔 7 个 writer 实例共享同一 config 时 schema 被覆盖为相同值。
type CalibrationCsvWriter struct {
	mu       sync.Mutex
	file     *os.File
	writer   *csv.Writer
	path     string
	schema   calibration.CsvSchema
	header   []string
	truncate bool // true 时以覆盖模式打开（用于按需全量导出）
	// schemaOverride true 时 Initialize 跳过 NewCsvSchema 重建，使用外部注入的 schema
	// （七孔场景下 schema 由 NewSevenHoleCsvSchema 构建，含 region+sector 路由信息）
	schemaOverride bool
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

// NewCalibrationCsvWriterWithSchema 创建带外部注入 schema 的 CSV 写入器
//
// 用于七孔多 writer 路由场景：schema 由调用方通过 calibration.NewSevenHoleCsvSchema
// 构建后注入，Initialize 不再重建 schema（避免覆盖 region+sector 路由信息）。
//
// 注意：此构造器创建的 writer 仍需调用 Initialize 完成文件打开 + BOM + 表头写入。
// 调用 Initialize 时 config.SavePath 决定文件路径，schema 字段保持构造器注入的值。
//
// 参数：
//   - schema：列布局描述（七孔场景按 region+sector 区分内/外区表头）
//   - truncate：true 时覆盖模式（适合 SaveCsv 全量导出），false 时追加模式（适合逐点采集）
func NewCalibrationCsvWriterWithSchema(schema calibration.CsvSchema, truncate bool) *CalibrationCsvWriter {
	return &CalibrationCsvWriter{
		schema:         schema,
		truncate:       truncate,
		schemaOverride: true,
	}
}

// NewWriter 实现 ports.CalibrationWriterFactory 接口
//
// 按 path + schema 创建独立的 CSV writer 实例并完成 Initialize（写 BOM + 表头）。
// 用于七孔场景：CalibrationManager 通过工厂为每个 region+sector 创建独立 writer。
//
// 与 Initialize(config) 的差异：
//   - Initialize 用 config.SavePath 作为文件路径，schema 由 config.Type 重建
//   - NewWriter 用显式 path 参数，schema 由外部注入（七孔 region+sector 路由）
//
// 返回的 writer 已完成 Initialize，调用方直接 AppendPoint 即可。
// 文件打开模式为追加（非 truncate），与 NewCalibrationCsvWriter 默认行为一致——
// 七孔场景下每个 region+sector 文件首次创建，追加模式安全。
func (w *CalibrationCsvWriter) NewWriter(path string, schema calibration.CsvSchema) (ports.CalibrationCsvWriter, error) {
	if path == "" {
		return nil, fmt.Errorf("保存路径为空")
	}
	if filepath.Ext(path) == "" {
		return nil, fmt.Errorf("保存路径必须是完整的文件路径（含 .csv 扩展名），当前传入: %q", path)
	}

	// 创建新实例，避免共享 mu/file 等可变状态
	// schemaOverride=true 确保 Initialize 不覆盖注入的 schema
	writer := &CalibrationCsvWriter{
		schema:         schema,
		truncate:       false, // 七孔每个 region+sector 文件首次创建，追加模式安全
		schemaOverride: true,
	}
	if err := writer.initializeWithPath(path); err != nil {
		return nil, err
	}
	return writer, nil
}

// Initialize 初始化 CSV 文件，写入表头
//
// 路径约定：config.SavePath 必须是完整的文件路径（含扩展名，如 .csv）。
// 不再做"无扩展名则视为目录"的二义性兜底——前端统一拼好完整路径传入，
// 调用方（backend / api server）负责在调用前做路径归一（ResolvePath / Abs）。
//
// schemaOverride=true 时跳过 NewCsvSchema 重建，使用构造器注入的 schema
// （七孔场景下 schema 含 region+sector 路由信息，不能被 config.Type 重建覆盖）。
func (w *CalibrationCsvWriter) Initialize(config calibration.Config) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if config.SavePath == "" {
		return fmt.Errorf("保存路径为空")
	}
	if filepath.Ext(config.SavePath) == "" {
		return fmt.Errorf("保存路径必须是完整的文件路径（含 .csv 扩展名），当前传入: %q", config.SavePath)
	}
	if !w.schemaOverride {
		w.schema = calibration.NewCsvSchema(config)
	}
	return w.initializeFileLocked(config.SavePath)
}

// initializeWithPath 按 path 初始化文件（工厂模式专用，不加锁——调用方保证单线程创建）
//
// 与 Initialize 的差异：
//   - 不依赖 config.SavePath，path 由参数传入
//   - 不重建 schema（schemaOverride 已在构造器中设为 true）
//   - 不加 w.mu（NewWriter 创建新实例时无并发）
func (w *CalibrationCsvWriter) initializeWithPath(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("创建目录失败: %w", err)
	}
	w.path = path
	return w.initializeFileLocked(path)
}

// initializeFileLocked 共享的文件初始化逻辑（必须持有 w.mu 或在单线程创建路径下调用）
//
// 步骤：
//  1. 确保目录存在
//  2. 设置 w.path
//  3. 判断是否新建文件（决定是否写 BOM）
//  4. 打开文件（追加或覆盖模式）
//  5. 写 UTF-8 BOM（仅新建文件）
//  6. 写表头（追加模式下文件已存在且非空时跳过，避免重复表头）
func (w *CalibrationCsvWriter) initializeFileLocked(path string) error {
	// 确保目录存在
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("创建目录失败: %w", err)
	}

	// 文件路径直接使用 path（已是完整文件路径）
	w.path = path

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
	// 追加模式下文件已存在且非空时跳过表头：否则重复校准同名文件时，
	// 第二次 Start 会再写一行表头，Excel 打开时该表头被当成数据行，
	// 列对齐错乱（参见 calibration_csv_writer_test.go 的追加去重用例）。
	// 覆盖模式（truncate）或新文件仍需写表头。
	w.header = w.schema.BuildHeader()
	if !w.truncate && !isNewFile {
		// 追加模式 + 文件已存在且非空：跳过表头，仅记录 schema 供 AppendPoint 使用
		return nil
	}
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
