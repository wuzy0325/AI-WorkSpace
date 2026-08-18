package backend

import (
	"context"
	"sync"

	"shared.local/device-sdk/go/pkg/slog"

	"wista/core"
	"wista/usecase"
)

// LogFileState 日志文件写入状态，用于前端展示。
// 由 LogService.GetLogFileState 返回，前端通过 HTTP API 读取。
type LogFileState struct {
	Active    bool   `json:"active"`
	OutputDir string `json:"outputDir,omitempty"`
}

// LogService 暴露日志相关能力给前端，同时实现 core.LogEmitter 接口，
// 由 Hub 统一调度日志事件分发。
//
// Win7 分支：移除 *application.App 依赖，事件推送改为 hub.EmitEvent，
// 由 httpserver.WSHub 实现具体传输。
type LogService struct {
	hub    *core.Hub
	logUC  *usecase.LogUsecase
	logDir string

	// mu 保护下方 ctx 字段，ServiceStartup/Shutdown 期间被读写。
	mu  sync.Mutex
	ctx context.Context
}

// NewLogService 创建日志 Service。
//   - hub: 共享状态中心；
//   - logUC: 日志持久化业务逻辑；
//   - logDir: 应用启动时默认开启日志文件保存的目录。
func NewLogService(hub *core.Hub, logUC *usecase.LogUsecase, logDir string) *LogService {
	return &LogService{
		hub:    hub,
		logUC:  logUC,
		logDir: logDir,
	}
}

// ServiceStartup 在应用启动阶段被 main.go 调用一次。
// 该方法负责：
//   1. 抢占 Hub 的 ctx 并设置自身为日志发布器；
//   2. 自动开启日志文件保存（如果配置了 logDir）；
//   3. 通过 Hub 发布一条 "应用已启动" 日志。
func (s *LogService) ServiceStartup(ctx context.Context) error {
	// Hub 在 LogService 启动时初始化 ctx，并将自身注册为日志发布器
	s.hub.SetContext(ctx)
	s.hub.SetEmitter(s)

	if s.logDir != "" {
		if err := s.logUC.Start(s.logDir, "daq-log"); err != nil {
			slog.Error("自动启动日志文件写入失败", "error", err)
		} else {
			slog.Info("日志文件自动保存已开启", "dir", s.logDir)
		}
	}

	slog.Info("WISTA application started")
	s.EmitLog(core.LogEvent{
		Level:    "info",
		Category: "system",
		Source:   "app",
		Message:  "WISTA application started",
	})
	return nil
}

// ServiceShutdown 在应用关闭阶段被 main.go 调用一次，
// 负责停止日志文件写入、广播一条关闭日志，并释放 Hub 中的 ctx。
func (s *LogService) ServiceShutdown() error {
	_ = s.logUC.Stop()
	s.EmitLog(core.LogEvent{
		Level:    "info",
		Category: "system",
		Source:   "app",
		Message:  "WISTA application shut down",
	})
	s.hub.CancelContext()
	slog.Info("WISTA application shut down")
	return nil
}

// EmitLog 是 core.LogEmitter 的实现：
//   1. 补全时间戳与来源；
//   2. 同步写入日志文件（若已开启）；
//   3. 通过 hub.EmitEvent 推送给前端（事件名 core.EventLog）。
func (s *LogService) EmitLog(entry core.LogEvent) {
	if entry.Timestamp == 0 {
		entry.Timestamp = core.TimestampMs()
	}
	if entry.Source == "" {
		entry.Source = "backend"
	}

	// 同步写入日志文件（如果已开启）
	if s.logUC != nil && s.logUC.IsActive() {
		if err := s.logUC.Write(entry.Timestamp, entry.Level, entry.Category, entry.DeviceID, entry.Source, entry.Message, entry.Detail); err != nil {
			slog.Error("写入日志文件失败", "error", err)
		}
	}

	s.hub.EmitEvent(core.EventLog, entry)
}

// StartLogFile 开始将日志写入文件。
func (s *LogService) StartLogFile(outputDir string, prefix string) error {
	if err := s.logUC.Start(outputDir, prefix); err != nil {
		return err
	}
	s.EmitLog(core.LogEvent{Level: "info", Category: "system", Source: "logging", Message: "日志文件保存已开启"})
	return nil
}

// StopLogFile 停止日志文件写入。
func (s *LogService) StopLogFile() error {
	if err := s.logUC.Stop(); err != nil {
		return err
	}
	s.EmitLog(core.LogEvent{Level: "info", Category: "system", Source: "logging", Message: "日志文件保存已关闭"})
	return nil
}

// GetLogFileState 返回日志文件当前状态。
func (s *LogService) GetLogFileState() LogFileState {
	return LogFileState{
		Active:    s.logUC.IsActive(),
		OutputDir: s.logUC.GetOutputDir(),
	}
}

// PickDirectory 在 Win7 分支中返回 ErrDialogNotSupported。
// 前端通过 Electron IPC 调用原生对话框，后端不再处理 UI 交互。
func (s *LogService) PickDirectory() (string, error) {
	return "", ErrDialogNotSupported
}