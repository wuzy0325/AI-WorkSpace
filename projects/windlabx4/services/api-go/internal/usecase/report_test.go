package usecase

import (
	"testing"

	"windlabx4/services/api-go/internal/core/report"
)

type fakeReportGenerator struct {
	err error
}

func (g *fakeReportGenerator) Generate(cfg report.ReportConfig, data [][]string, headers []string) (report.ReportResult, error) {
	if g.err != nil {
		return report.ReportResult{}, g.err
	}
	return report.ReportResult{Path: cfg.OutputDir + "/" + cfg.FilePrefix + ".csv", Size: 42, Records: len(data)}, nil
}

func TestReportManagerGeneratesReportWithValidConfig(t *testing.T) {
	gen := &fakeReportGenerator{}
	mgr := NewReportManager(gen)
	result, err := mgr.Generate(report.ReportConfig{OutputDir: t.TempDir(), FilePrefix: "test-report", DeviceID: "sim-1"})
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if result.Path == "" {
		t.Fatal("expected non-empty result path")
	}
	if result.Records < 1 {
		t.Fatal("expected at least one record")
	}
}

func TestReportManagerRejectsEmptyConfig(t *testing.T) {
	mgr := NewReportManager(&fakeReportGenerator{})
	if _, err := mgr.Generate(report.ReportConfig{}); err == nil {
		t.Fatal("expected error for empty config")
	}
	if _, err := mgr.Generate(report.ReportConfig{OutputDir: "/tmp"}); err == nil {
		t.Fatal("expected error when filePrefix is empty")
	}
}

func TestReportManagerStatus(t *testing.T) {
	mgr := NewReportManager(&fakeReportGenerator{})
	status := mgr.Status()
	if status.Generating {
		t.Fatal("expected not generating initially")
	}
}

func TestReportManagerRejectsConcurrentGeneration(t *testing.T) {
	mgr := NewReportManager(&fakeReportGenerator{})
	mgr.mu.Lock()
	mgr.busy = true
	mgr.mu.Unlock()

	_, err := mgr.Generate(report.ReportConfig{OutputDir: t.TempDir(), FilePrefix: "b", DeviceID: "sim-1"})
	if err == nil {
		t.Fatal("expected concurrent generation to fail")
	}

	mgr.mu.Lock()
	mgr.busy = false
	mgr.mu.Unlock()

	result, err := mgr.Generate(report.ReportConfig{OutputDir: t.TempDir(), FilePrefix: "a", DeviceID: "sim-1"})
	if err != nil {
		t.Fatalf("expected success after clearing busy, got: %v", err)
	}
	if result.Records < 1 {
		t.Fatal("expected at least one record")
	}
}
