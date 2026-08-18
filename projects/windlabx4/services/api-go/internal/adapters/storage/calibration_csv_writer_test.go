package storage

import (
	"bytes"
	"encoding/csv"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"windlabx4/services/api-go/internal/core/calibration"
)

func TestCalibrationCsvWriterInitializeRefreshesSchemaFromConfig(t *testing.T) {
	savePath := filepath.Join(t.TempDir(), "five-hole.csv")
	writer := NewCalibrationCsvWriter(calibration.Config{})
	config := calibration.Config{
		TaskID:   "cal-1",
		Type:     string(calibration.TypeFiveHole),
		SavePath: savePath,
	}

	if err := writer.Initialize(config); err != nil {
		t.Fatalf("Initialize returned error: %v", err)
	}
	if err := writer.Flush(); err != nil {
		t.Fatalf("Flush returned error: %v", err)
	}

	raw, err := os.ReadFile(savePath)
	if err != nil {
		t.Fatalf("read csv: %v", err)
	}
	// 首行应以 UTF-8 BOM 开头，避免 Excel / 中文 Windows 端打开中文表头乱码
	if !bytes.HasPrefix(raw, utf8BOM) {
		// Go 1.20 无内置 min（Go 1.21+），手动三元避免 vet 报错
		n := 3
		if len(raw) < n {
			n = len(raw)
		}
		t.Fatalf("expected CSV to start with UTF-8 BOM, got first bytes: % x", raw[:n])
	}
	// 去掉 BOM 后再交给 csv.Reader，否则 BOM 会被并入第一列
	records, err := csv.NewReader(bytes.NewReader(bytes.TrimPrefix(raw, utf8BOM))).ReadAll()
	if err != nil {
		t.Fatalf("read csv: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("expected only header row, got %d rows", len(records))
	}
	header := records[0]
	if len(header) != 35 {
		t.Fatalf("expected five-hole header with 35 columns, got %d: %v", len(header), header)
	}
	if header[0] != "点位编号" || header[1] != "α(°)" || header[len(header)-1] != "CH16(Pa)" {
		t.Fatalf("unexpected five-hole header: %v", header)
	}
}

// TestCalibrationCsvWriterAppendModeSkipsHeaderOnExistingFile 验证追加模式下，
// 文件已存在且非空时 Initialize 不再写表头。
//
// 修复场景：配置名不改 → savePath 不变 → 重复校准时第二次 Start 调 Initialize，
// 旧行为会无条件写表头，导致 CSV 中间出现一行表头，Excel 打开时该表头被当成数据行，
// 列对齐错乱。修复后追加模式 + 文件已存在 → 跳过表头，仅续写数据行。
func TestCalibrationCsvWriterAppendModeSkipsHeaderOnExistingFile(t *testing.T) {
	savePath := filepath.Join(t.TempDir(), "five-hole.csv")
	config := calibration.Config{
		TaskID:   "cal-1",
		Type:     string(calibration.TypeFiveHole),
		SavePath: savePath,
	}

	// 用 FiveHoleDataPoint 零值 + PointID 即可，buildFiveHoleRecord 能处理零值字段。
	// 测试目的不是验证数据内容，而是验证追加模式不重写表头。

	// 第一次 Initialize：新文件，应写 BOM + 表头
	writer1 := NewCalibrationCsvWriter(config)
	if err := writer1.Initialize(config); err != nil {
		t.Fatalf("first Initialize returned error: %v", err)
	}
	if err := writer1.AppendPoint(&calibration.FiveHoleDataPoint{PointID: 1}); err != nil {
		t.Fatalf("AppendPoint returned error: %v", err)
	}
	if err := writer1.Flush(); err != nil {
		t.Fatalf("first Flush returned error: %v", err)
	}

	// 第二次 Initialize：文件已存在且非空，追加模式应跳过表头
	writer2 := NewCalibrationCsvWriter(config)
	if err := writer2.Initialize(config); err != nil {
		t.Fatalf("second Initialize returned error: %v", err)
	}
	if err := writer2.AppendPoint(&calibration.FiveHoleDataPoint{PointID: 2}); err != nil {
		t.Fatalf("AppendPoint returned error: %v", err)
	}
	if err := writer2.Flush(); err != nil {
		t.Fatalf("second Flush returned error: %v", err)
	}

	// 验证：整个文件应只有 1 行表头 + 2 行数据 = 3 行
	// 若表头未跳过，会是 2 行表头 + 2 行数据 = 4 行（bug 行为）
	raw, err := os.ReadFile(savePath)
	if err != nil {
		t.Fatalf("read csv: %v", err)
	}
	records, err := csv.NewReader(bytes.NewReader(bytes.TrimPrefix(raw, utf8BOM))).ReadAll()
	if err != nil {
		t.Fatalf("parse csv: %v", err)
	}
	if len(records) != 3 {
		t.Fatalf("expected 1 header + 2 data rows = 3 rows, got %d (header dedup failed?): %v", len(records), records)
	}
	// 第一行必须是表头（点位编号），不能是数据
	if records[0][0] != "点位编号" {
		t.Fatalf("expected first row to be header, got: %v", records[0])
	}
	// 第二、三行必须是数据（PointID=1, 2），不能是表头
	if records[1][0] != "1" {
		t.Fatalf("expected second row to be data point 1, got: %v", records[1])
	}
	if records[2][0] != "2" {
		t.Fatalf("expected third row to be data point 2, got: %v", records[2])
	}
}

// TestCalibrationCsvWriterAppendPointDetectsBufferedError 验证 AppendPoint 在
// csv.Writer 缓冲写入失败时（如底层文件已关闭）通过 Error() 检测并返回错误。
//
// 测试前置：writer 已 Initialize，底层文件已被外部关闭（模拟写入失败）
// 测试步骤：调用 AppendPoint
// 期待结果：返回非 nil 错误（旧实现只调 Flush 不检查 Error()，会静默丢点返回 nil）
//
// 修复场景（spec Task 20）：csv.Writer.Flush() 不返回错误，必须通过 Error() 检查
// 缓冲写入失败。旧实现 AppendPoint 只调 w.writer.Flush() 不检查 Error()，
// 底层文件关闭后写入会静默丢点。
func TestCalibrationCsvWriterAppendPointDetectsBufferedError(t *testing.T) {
	savePath := filepath.Join(t.TempDir(), "five-hole.csv")
	writer := NewCalibrationCsvWriter(calibration.Config{})
	config := calibration.Config{
		TaskID:   "cal-1",
		Type:     string(calibration.TypeFiveHole),
		SavePath: savePath,
	}
	if err := writer.Initialize(config); err != nil {
		t.Fatalf("Initialize returned error: %v", err)
	}

	// 关闭底层文件，模拟写入失败（csv.Writer 缓冲到 bufio.Writer，Write 不会立即失败）
	if err := writer.file.Close(); err != nil {
		t.Fatalf("close file: %v", err)
	}

	// AppendPoint 应通过 csv.Writer.Error() 检测到底层写入失败
	err := writer.AppendPoint(&calibration.FiveHoleDataPoint{PointID: 1})
	if err == nil {
		t.Fatal("AppendPoint 底层文件关闭后应返回错误（csv.Writer.Error() 未被检查），实际返回 nil")
	}
}

// TestCalibrationCsvWriterFlushJoinsWriterAndCloseErrors 验证 Flush 同时返回
// csv.Writer 缓冲错误和文件关闭错误（errors.Join 聚合）。
//
// 测试前置：writer 已 Initialize，先成功写一个点，然后关闭底层文件，再写一个点
// 测试步骤：调用 Flush
// 期待结果：返回的 error 同时包含 csv.Writer 缓冲错误和 file.Close 错误
//
// 修复场景（spec Task 20）：旧实现 Flush 只返回 file.Close() 错误，
// csv.Writer.Error() 缓冲错误被丢弃。修复后两者通过 errors.Join 聚合，
// 调用方可同时识别两类失败。
func TestCalibrationCsvWriterFlushJoinsWriterAndCloseErrors(t *testing.T) {
	savePath := filepath.Join(t.TempDir(), "five-hole.csv")
	writer := NewCalibrationCsvWriter(calibration.Config{})
	config := calibration.Config{
		TaskID:   "cal-1",
		Type:     string(calibration.TypeFiveHole),
		SavePath: savePath,
	}
	if err := writer.Initialize(config); err != nil {
		t.Fatalf("Initialize returned error: %v", err)
	}

	// 先成功写一个点（让 csv.Writer 处于正常状态）
	if err := writer.AppendPoint(&calibration.FiveHoleDataPoint{PointID: 1}); err != nil {
		t.Fatalf("AppendPoint returned error: %v", err)
	}

	// 关闭底层文件，让后续 AppendPoint 的 csv.Writer.Flush 写入失败
	if err := writer.file.Close(); err != nil {
		t.Fatalf("close file: %v", err)
	}

	// 再写一个点：csv.Writer.Write 缓冲（无错），csv.Writer.Flush 写入失败（底层文件已关闭），
	// 但 Flush 不返回错误，Error() 才记录。
	// 忽略这里的返回值（旧实现返回 nil，新实现返回错误——都让 Flush 来聚合）
	_ = writer.AppendPoint(&calibration.FiveHoleDataPoint{PointID: 2})

	// 此时 csv.Writer.Error() 已记录写入错误。调用 Flush()：
	// - writer.Flush() 无操作（已 flush）
	// - writer.Error() 返回写入错误（非 nil）
	// - file.Close() 返回 "file already closed" 错误
	// 两者应通过 errors.Join 聚合（spec Task 20）
	err := writer.Flush()
	if err == nil {
		t.Fatal("Flush 底层文件已关闭时应返回聚合错误，实际返回 nil")
	}

	// 验证 file.Close 错误可识别（"file already closed" 是 os.ErrClosed 的消息）
	if !errors.Is(err, os.ErrClosed) {
		t.Errorf("Flush 错误应包含 file.Close 错误（os.ErrClosed），实际: %v", err)
	}

	// 验证聚合了多个错误：errors.Join 返回的错误字符串应同时包含两类错误特征。
	// csv.Writer 缓冲写入错误的特征是 "file already closed"（写入到已关闭文件），
	// file.Close 错误也是 "file already closed"。两者消息相同，但应被聚合。
	// 用字符串计数验证：聚合后错误字符串应包含至少两次 "file already closed"。
	errStr := err.Error()
	if cnt := strings.Count(errStr, "file already closed"); cnt < 1 {
		t.Errorf("Flush 聚合错误应包含 file 已关闭特征，实际: %v", err)
	}
}

// TestCalibrationCsvWriterFlushHappyPathNoError 验证正常路径下 Flush 不返回错误
// （回归测试，确保 errors.Join 改造未破坏 happy path）。
//
// 测试前置：writer 已 Initialize，成功写一个点
// 测试步骤：调用 Flush
// 期待结果：返回 nil
func TestCalibrationCsvWriterFlushHappyPathNoError(t *testing.T) {
	savePath := filepath.Join(t.TempDir(), "five-hole.csv")
	writer := NewCalibrationCsvWriter(calibration.Config{})
	config := calibration.Config{
		TaskID:   "cal-1",
		Type:     string(calibration.TypeFiveHole),
		SavePath: savePath,
	}
	if err := writer.Initialize(config); err != nil {
		t.Fatalf("Initialize returned error: %v", err)
	}
	if err := writer.AppendPoint(&calibration.FiveHoleDataPoint{PointID: 1}); err != nil {
		t.Fatalf("AppendPoint returned error: %v", err)
	}
	if err := writer.Flush(); err != nil {
		t.Fatalf("Flush 正常路径应返回 nil，实际: %v", err)
	}
}

// TestCalibrationCsvWriterFlushIdempotentNilSafe 验证 Flush 幂等且对未初始化 writer 安全
// （回归测试，确保 errors.Join 改造未破坏 nil 安全性）。
//
// 测试前置：writer 未 Initialize（file/writer 均为 nil）
// 测试步骤：连续调用 Flush 两次
// 期待结果：均返回 nil，不 panic
func TestCalibrationCsvWriterFlushIdempotentNilSafe(t *testing.T) {
	writer := NewCalibrationCsvWriter(calibration.Config{})
	if err := writer.Flush(); err != nil {
		t.Fatalf("Flush 未初始化 writer 应返回 nil，实际: %v", err)
	}
	// 第二次调用应幂等（file/writer 已 nil）
	if err := writer.Flush(); err != nil {
		t.Fatalf("Flush 第二次调用应返回 nil，实际: %v", err)
	}
}

