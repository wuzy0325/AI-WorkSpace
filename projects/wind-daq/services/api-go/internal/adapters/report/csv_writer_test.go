package report

import (
	"os"
	"path/filepath"
	"testing"

	"wind-daq/services/api-go/internal/core/report"
)

func TestCSVReportWriterCreatesValidCSV(t *testing.T) {
	dir := t.TempDir()
	writer := NewCSVReportWriter()
	result, err := writer.Generate(report.ReportConfig{OutputDir: dir, FilePrefix: "test"}, [][]string{{"sim-1", "100", "0", "1.5"}}, []string{"device_id", "timestamp", "channel", "value"})
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if result.Path == "" {
		t.Fatal("expected non-empty path")
	}
	if result.Records != 1 {
		t.Fatalf("expected 1 record, got %d", result.Records)
	}
	if result.Size <= 0 {
		t.Fatal("expected positive file size")
	}
	data, err := os.ReadFile(result.Path)
	if err != nil {
		t.Fatalf("read file: %v", err)
	}
	expected := "device_id,timestamp,channel,value\nsim-1,100,0,1.5\n"
	if string(data) != expected {
		t.Fatalf("expected %q, got %q", expected, string(data))
	}
}

func TestCSVReportWriterCreatesDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "subdir", "nested")
	writer := NewCSVReportWriter()
	_, err := writer.Generate(report.ReportConfig{OutputDir: dir, FilePrefix: "nested-test"}, [][]string{{"a"}}, []string{"col"})
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		t.Fatal("expected directory to be created")
	}
}

func TestCSVReportWriterHandlesMultipleRecords(t *testing.T) {
	writer := NewCSVReportWriter()
	result, err := writer.Generate(report.ReportConfig{OutputDir: t.TempDir(), FilePrefix: "multi"}, [][]string{{"1"}, {"2"}, {"3"}}, []string{"val"})
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if result.Records != 3 {
		t.Fatalf("expected 3 records, got %d", result.Records)
	}
}

func TestCSVReportWriterEmptyRecords(t *testing.T) {
	writer := NewCSVReportWriter()
	_, err := writer.Generate(report.ReportConfig{OutputDir: t.TempDir(), FilePrefix: "empty"}, [][]string{}, []string{"col"})
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
}
