package usecase

import (
	"fmt"
	"strings"
	"sync"

	"wind-daq/services/api-go/internal/core/report"
	"wind-daq/services/api-go/internal/ports"
)

type ReportManager struct {
	mu   sync.Mutex
	gen  ports.ReportGenerator
	busy bool
}

func NewReportManager(gen ports.ReportGenerator) *ReportManager {
	return &ReportManager{gen: gen}
}

func (m *ReportManager) Generate(cfg report.ReportConfig) (report.ReportResult, error) {
	if strings.TrimSpace(cfg.OutputDir) == "" {
		return report.ReportResult{}, fmt.Errorf("outputDir is required")
	}
	if strings.TrimSpace(cfg.FilePrefix) == "" {
		return report.ReportResult{}, fmt.Errorf("filePrefix is required")
	}
	if m.gen == nil {
		return report.ReportResult{}, fmt.Errorf("report generator is required")
	}

	m.mu.Lock()
	if m.busy {
		m.mu.Unlock()
		return report.ReportResult{}, fmt.Errorf("report generation already in progress")
	}
	m.busy = true
	m.mu.Unlock()

	defer func() {
		m.mu.Lock()
		m.busy = false
		m.mu.Unlock()
	}()

	const headers = "device_id,timestamp,channel,value"
	data := [][]string{
		{cfg.DeviceID, "sample", "0", "0.0"},
	}
	return m.gen.Generate(cfg, data, strings.Split(headers, ","))
}

func (m *ReportManager) Status() report.ReportStatus {
	m.mu.Lock()
	defer m.mu.Unlock()
	return report.ReportStatus{Generating: m.busy}
}
