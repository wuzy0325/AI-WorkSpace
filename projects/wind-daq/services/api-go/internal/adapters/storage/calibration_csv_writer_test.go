package storage

import (
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

	file, err := os.Open(savePath)
	if err != nil {
		t.Fatalf("open csv: %v", err)
	}
	defer file.Close()

	records, err := csv.NewReader(file).ReadAll()
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
