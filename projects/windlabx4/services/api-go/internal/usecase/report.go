package usecase

import (
	"fmt"
	"shared.local/device-sdk/go/pkg/slog"
	"strings"
	"sync"

	"windlabx4/services/api-go/internal/core/report"
	"windlabx4/services/api-go/internal/ports"
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
	slog.Info("ReportManager Generate 开始", "component", "ReportManager", "deviceID", cfg.DeviceID, "outputDir", cfg.OutputDir, "filePrefix", cfg.FilePrefix)

	if strings.TrimSpace(cfg.OutputDir) == "" {
		err := fmt.Errorf("outputDir is required")
		slog.Error("ReportManager Generate 失败", "component", "ReportManager", "error", err)
		return report.ReportResult{}, err
	}
	if strings.TrimSpace(cfg.FilePrefix) == "" {
		err := fmt.Errorf("filePrefix is required")
		slog.Error("ReportManager Generate 失败", "component", "ReportManager", "error", err)
		return report.ReportResult{}, err
	}
	if m.gen == nil {
		err := fmt.Errorf("report generator is required")
		slog.Error("ReportManager Generate 失败", "component", "ReportManager", "error", err)
		return report.ReportResult{}, err
	}

	m.mu.Lock()
	if m.busy {
		m.mu.Unlock()
		err := fmt.Errorf("report generation already in progress")
		slog.Warn("ReportManager Generate 冲突：正在生成中", "component", "ReportManager", "deviceID", cfg.DeviceID)
		return report.ReportResult{}, err
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
	result, err := m.gen.Generate(cfg, data, strings.Split(headers, ","))
	if err != nil {
		slog.Error("ReportManager Generate 失败", "component", "ReportManager", "deviceID", cfg.DeviceID, "error", err)
		return report.ReportResult{}, err
	}
	slog.Info("ReportManager Generate 成功", "component", "ReportManager", "deviceID", cfg.DeviceID, "outputDir", cfg.OutputDir)
	return result, nil
}

func (m *ReportManager) Status() report.ReportStatus {
	m.mu.Lock()
	defer m.mu.Unlock()
	status := report.ReportStatus{Generating: m.busy}
	slog.Info("ReportManager Status 查询", "component", "ReportManager", "generating", status.Generating)
	return status
}
