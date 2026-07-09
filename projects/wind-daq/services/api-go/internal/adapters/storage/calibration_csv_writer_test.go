package storage

import (
	"bytes"
	"encoding/csv"
	"os"
	"path/filepath"
	"testing"

	"wind-daq/services/api-go/internal/core/calibration"
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
		t.Fatalf("expected CSV to start with UTF-8 BOM, got first bytes: % x", raw[:min(len(raw), 3)])
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

