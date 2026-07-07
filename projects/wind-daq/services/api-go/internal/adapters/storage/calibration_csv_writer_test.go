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
