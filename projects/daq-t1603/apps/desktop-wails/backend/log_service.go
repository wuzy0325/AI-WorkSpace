package backend

import (
	"context"
	"log/slog"
	"sync"

	"daq-t1603/core"
	"daq-t1603/usecase"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// LogFileState 日志文件写入状态，用于前端展示
type LogFileState struct {
	Active    bool   `json:"active"`
	OutputDir string `json:"outputDir,omitempty"`
}

// LogService 暴露日志相关能力给前端，同时实现 core.LogEmitter 接口，
// 由 Hub 统一调度日志事件分发。
//
// 注意：在 Wails v3 下，所有 frontend 可调用的方法都必须挂在以指针接收器导出的方法上，
// 且通过 application.NewService(&LogService{...}) 注册。
type LogService struct {
	hub     *core.Hub
	logUC   *usecase.LogUsecase
	logDir  string

	mu  sync.Mutex
	app *application.App
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

// ServiceName 返回 Wails 绑定时使用的服务名。
// 显式声明以避免不同包导致的反射结果不一致（v3 默认会用类型路径）。
func (s *LogService) ServiceName() string {
	return "LogService"
}

// ServiceStartup 在应用启动阶段被 Wails 调用一次。
// 该方法负责：
//   1. 抢占 Hub 的 ctx 并设置自身为日志发布器；
//   2. 自动开启日志文件保存（如果配置了 logDir）；
//   3. 通过 Hub 发布一条 "应用已启动" 日志。
func (s *LogService) ServiceStartup(ctx context.Context, options application.ServiceOptions) error {
	s.mu.Lock()
	s.app = application.Get()
	s.mu.Unlock()

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

	slog.Info("DAQ-T-1603 application started")
	s.EmitLog(core.LogEvent{
		Level:    "info",
		Category: "system",
		Source:   "app",
		Message:  "DAQ-T-1603 application started",
	})
	return nil
}

// ServiceShutdown 在应用关闭阶段被 Wails 调用一次，
// 负责停止日志文件写入、广播一条关闭日志，并释放 Hub 中的 ctx。
func (s *LogService) ServiceShutdown() error {
	_ = s.logUC.Stop()
	s.EmitLog(core.LogEvent{
		Level:    "info",
		Category: "system",
		Source:   "app",
		Message:  "DAQ-T-1603 application shut down",
	})
	s.hub.CancelContext()
	slog.Info("DAQ-T-1603 application shut down")
	return nil
}

// EmitLog 是 core.LogEmitter 的实现：
//   1. 补全时间戳与来源；
//   2. 同步写入日志文件（若已开启）；
//   3. 通过 Wails v3 Event 总线推送给前端。
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

	s.mu.Lock()
	app := s.app
	s.mu.Unlock()
	if app == nil {
		return
	}
	app.Event.Emit("daq:log", entry)
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

// PickDirectory 让用户在系统对话框中选择目录，返回选定的绝对路径。
//
// 在 Wails v3 中，目录选择通过 app.Dialog.OpenFile().CanChooseDirectories(true) 完成；
// 该方法被 LogService 和 RecordingService 共用（前端 bridge 各自调用同名 PickDirectory）。
func (s *LogService) PickDirectory() (string, error) {
	s.mu.Lock()
	app := s.app
	s.mu.Unlock()
	if app == nil {
		app = application.Get()
	}
	return app.Dialog.OpenFile().
		CanChooseDirectories(true).
		CanChooseFiles(false).
		CanCreateDirectories(true).
		SetTitle("选择保存目录").
		PromptForSingleSelection()
}
