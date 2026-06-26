package backend

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"daq-p1604/core"
	"daq-p1604/usecase"
	"github.com/wailsapp/wails/v3/pkg/application"
)

const (
	uiPayloadRefreshInterval    = 100 * time.Millisecond
	recordingStatusEmitInterval = time.Second
)

// App Wails 后端应用
type App struct {
	ctx      context.Context
	cancel   context.CancelFunc
	deviceUC *usecase.DeviceUsecase
	recordUC *usecase.RecordingUsecase
	logUC    *usecase.LogUsecase
	logDir   string
	app      *application.App
	mu       sync.Mutex
	relays   map[string]*relayControl
}

// relayControl 采集数据中继控制
type relayControl struct {
	cancel context.CancelFunc
	done   chan struct{}
}

// LogEvent 日志事件
type LogEvent struct {
	Level     string `json:"level"`
	Category  string `json:"category"`
	DeviceID  string `json:"deviceId,omitempty"`
	Source    string `json:"source"`
	Message   string `json:"message"`
	Detail    string `json:"detail,omitempty"`
	Timestamp int64  `json:"timestamp"`
}

// LogFileState 日志文件写入状态
type LogFileState struct {
	Active    bool   `json:"active"`
	OutputDir string `json:"outputDir,omitempty"`
}

// NewApp 创建后端应用
func NewApp(deviceUC *usecase.DeviceUsecase, recordUC *usecase.RecordingUsecase, logUC *usecase.LogUsecase, logDir string) *App {
	return &App{
		deviceUC: deviceUC,
		recordUC: recordUC,
		logUC:    logUC,
		logDir:   logDir,
		relays:   make(map[string]*relayControl),
	}
}

// ServiceStartup 应用启动回调
func (a *App) ServiceStartup(ctx context.Context, options application.ServiceOptions) error {
	a.ctx, a.cancel = context.WithCancel(ctx)
	a.app = application.Get()

	// 自动启动日志文件写入
	if a.logDir != "" {
		if err := a.logUC.Start(a.logDir, "daq-log"); err != nil {
			slog.Error("自动启动日志文件写入失败", "error", err)
		} else {
			slog.Info("日志文件自动保存已开启", "dir", a.logDir)
		}
	}

	slog.Info("DAQ-P-1604 application started")
	a.EmitLog(LogEvent{
		Level:    "info",
		Category: "system",
		Source:   "app",
		Message:  "DAQ-P-1604 application started",
	})
	return nil
}

// ServiceShutdown 应用关闭回调
func (a *App) ServiceShutdown() error {
	_ = a.recordUC.Stop()
	_ = a.logUC.Stop()
	a.stopAllRelays()
	if a.cancel != nil {
		a.cancel()
	}
	a.EmitLog(LogEvent{
		Level:    "info",
		Category: "system",
		Source:   "app",
		Message:  "DAQ-P-1604 application shut down",
	})
	slog.Info("DAQ-P-1604 application shut down")
	return nil
}

// EmitLog 发送日志事件
func (a *App) EmitLog(entry LogEvent) {
	if entry.Timestamp == 0 {
		entry.Timestamp = core.TimestampMs()
	}
	if entry.Source == "" {
		entry.Source = "backend"
	}

	// 同步写入日志文件
	if a.logUC != nil && a.logUC.IsActive() {
		if err := a.logUC.Write(entry.Timestamp, entry.Level, entry.Category, entry.DeviceID, entry.Source, entry.Message, entry.Detail); err != nil {
			slog.Error("写入日志文件失败", "error", err)
		}
	}

	if a.app == nil {
		return
	}
	a.app.Event.Emit("daq:log", entry)
}

func (a *App) emitPayload(snapshot core.PressureSnapshot) {
	if a.app == nil {
		return
	}
	a.app.Event.Emit("daq:payload", snapshot)
}

func (a *App) emitRecordingStatus(session core.RecordingSession) {
	if a.app == nil {
		return
	}
	a.app.Event.Emit("daq:recording-status", session)
}

// EmitDeviceState 发送设备状态变更事件到前端
func (a *App) EmitDeviceState(id string, state core.DeviceState) {
	if a.app == nil {
		return
	}
	a.app.Event.Emit("daq:device-state", id, state)
}

// ScanDevices 扫描设备
func (a *App) ScanDevices() ([]core.ScanResult, error) {
	return a.deviceUC.ScanDevices()
}

// GetProfiles 获取所有设备配置
func (a *App) GetProfiles() []core.PressureProfile {
	return a.deviceUC.GetProfiles()
}

// UpsertProfile 保存设备配置
func (a *App) UpsertProfile(profile core.PressureProfile) error {
	a.EmitLog(LogEvent{Level: "info", Category: "system", DeviceID: profile.ID, Source: "config", Message: "保存设备配置"})
	if err := a.deviceUC.UpsertProfile(profile); err != nil {
		a.EmitLog(LogEvent{Level: "error", Category: "system", DeviceID: profile.ID, Source: "config", Message: "保存设备配置失败", Detail: err.Error()})
		return err
	}
	a.EmitLog(LogEvent{Level: "info", Category: "system", DeviceID: profile.ID, Source: "config", Message: "设备配置已保存"})
	return nil
}

// DeleteProfile 删除设备配置
func (a *App) DeleteProfile(id string) error {
	a.EmitLog(LogEvent{Level: "info", Category: "system", DeviceID: id, Source: "config", Message: "删除设备配置"})
	if err := a.deviceUC.DeleteProfile(id); err != nil {
		a.EmitLog(LogEvent{Level: "error", Category: "system", DeviceID: id, Source: "config", Message: "删除设备配置失败", Detail: err.Error()})
		return err
	}
	a.EmitLog(LogEvent{Level: "info", Category: "system", DeviceID: id, Source: "config", Message: "设备配置已删除"})
	return nil
}

// Connect 连接设备
func (a *App) Connect(id string) error {
	a.EmitLog(LogEvent{Level: "info", Category: "system", DeviceID: id, Source: "device", Message: "Connect requested"})
	if err := a.deviceUC.Connect(id); err != nil {
		a.EmitLog(LogEvent{Level: "error", Category: "system", DeviceID: id, Source: "device", Message: "Connect failed", Detail: err.Error()})
		return err
	}
	a.EmitLog(LogEvent{Level: "info", Category: "system", DeviceID: id, Source: "device", Message: "Device connected"})
	return nil
}

// Disconnect 断开设备
func (a *App) Disconnect(id string) error {
	a.EmitLog(LogEvent{Level: "info", Category: "system", DeviceID: id, Source: "device", Message: "Disconnect requested"})
	if err := a.deviceUC.Disconnect(id); err != nil {
		a.EmitLog(LogEvent{Level: "error", Category: "system", DeviceID: id, Source: "device", Message: "Disconnect failed", Detail: err.Error()})
		return err
	}
	a.waitRelay(id)
	a.EmitLog(LogEvent{Level: "info", Category: "system", DeviceID: id, Source: "device", Message: "Device disconnected"})
	return nil
}

// StartAcquisition 启动采集
func (a *App) StartAcquisition(id string) error {
	a.EmitLog(LogEvent{Level: "info", Category: "acquisition", DeviceID: id, Source: "device", Message: "Start acquisition requested"})
	ch, err := a.deviceUC.StartAcquisition(id)
	if err != nil {
		a.EmitLog(LogEvent{Level: "error", Category: "acquisition", DeviceID: id, Source: "device", Message: "Start acquisition failed", Detail: err.Error()})
		return err
	}
	a.startRelay(id, ch)
	a.EmitLog(LogEvent{Level: "info", Category: "acquisition", DeviceID: id, Source: "device", Message: "Acquisition started"})
	return nil
}

// StopAcquisition 停止采集
func (a *App) StopAcquisition(id string) error {
	a.EmitLog(LogEvent{Level: "info", Category: "acquisition", DeviceID: id, Source: "device", Message: "Stop acquisition requested"})
	if err := a.deviceUC.StopAcquisition(id); err != nil {
		a.EmitLog(LogEvent{Level: "error", Category: "acquisition", DeviceID: id, Source: "device", Message: "Stop acquisition failed", Detail: err.Error()})
		return err
	}
	a.waitRelay(id)
	a.EmitLog(LogEvent{Level: "info", Category: "acquisition", DeviceID: id, Source: "device", Message: "Acquisition stopped"})
	return nil
}

// GetStatus 获取设备状态
func (a *App) GetStatus(id string) (core.DeviceState, bool) {
	return a.deviceUC.GetStatus(id)
}

// ApplyConfig 应用设备配置
func (a *App) ApplyConfig(id string, cfg core.P1604Config) error {
	a.EmitLog(LogEvent{Level: "info", Category: "system", DeviceID: id, Source: "device", Message: "Apply config requested"})
	if err := a.deviceUC.ApplyConfig(id, cfg); err != nil {
		a.EmitLog(LogEvent{Level: "error", Category: "system", DeviceID: id, Source: "device", Message: "Apply config failed", Detail: err.Error()})
		return err
	}
	a.EmitLog(LogEvent{Level: "info", Category: "system", DeviceID: id, Source: "device", Message: "Config applied"})
	return nil
}

func (a *App) startRelay(deviceID string, ch <-chan core.PressureSnapshot) {
	baseCtx := a.ctx
	if baseCtx == nil {
		baseCtx = context.Background()
	}
	ctx, cancel := context.WithCancel(baseCtx)
	control := &relayControl{cancel: cancel, done: make(chan struct{})}

	a.mu.Lock()
	if oldControl, ok := a.relays[deviceID]; ok {
		oldControl.cancel()
	}
	a.relays[deviceID] = control
	a.mu.Unlock()

	go func() {
		defer close(control.done)
		defer a.clearRelay(deviceID, control)
		a.relayStream(ctx, deviceID, ch)
	}()
}

func (a *App) stopRelay(deviceID string) {
	a.mu.Lock()
	control := a.relays[deviceID]
	delete(a.relays, deviceID)
	a.mu.Unlock()
	if control != nil {
		control.cancel()
	}
}

func (a *App) waitRelay(deviceID string) {
	a.mu.Lock()
	control := a.relays[deviceID]
	a.mu.Unlock()
	if control != nil {
		<-control.done
	}
}

func (a *App) stopAllRelays() {
	a.mu.Lock()
	relays := a.relays
	a.relays = make(map[string]*relayControl)
	a.mu.Unlock()
	for _, control := range relays {
		control.cancel()
	}
}

func (a *App) clearRelay(deviceID string, control *relayControl) {
	a.mu.Lock()
	if a.relays[deviceID] == control {
		delete(a.relays, deviceID)
	}
	a.mu.Unlock()
}

// relayStream 中继数据流到前端
func (a *App) relayStream(ctx context.Context, deviceID string, ch <-chan core.PressureSnapshot) {
	uiTicker := time.NewTicker(uiPayloadRefreshInterval)
	statusTicker := time.NewTicker(recordingStatusEmitInterval)
	defer uiTicker.Stop()
	defer statusTicker.Stop()

	var latest core.PressureSnapshot
	hasLatest := false

	defer func() {
		if hasLatest {
			a.emitPayload(latest)
		}
		a.emitRecordingStatus(a.recordUC.Status())
	}()

	for {
		select {
		case <-ctx.Done():
			return
		case snapshot, ok := <-ch:
			if !ok {
				return
			}
			latest = snapshot
			hasLatest = true
			if a.recordUC.Status().Status == core.RecordingActive {
				if err := a.recordUC.Write(snapshot); err != nil {
					a.EmitLog(LogEvent{Level: "error", Category: "acquisition", DeviceID: deviceID, Source: "recording", Message: "Record snapshot failed", Detail: err.Error()})
				}
			}
		case <-uiTicker.C:
			if hasLatest {
				a.emitPayload(latest)
			}
		case <-statusTicker.C:
			if a.recordUC.Status().Status == core.RecordingActive {
				a.emitRecordingStatus(a.recordUC.Status())
			}
		}
	}
}

// StartRecording 开始录制
func (a *App) StartRecording(outputDir string, filePrefix string) error {
	// 取第一个设备的通道配置作为录制精度参考（CSV 为单设备 18 通道格式）
	var channels []core.ChannelConfig
	if profiles := a.deviceUC.GetProfiles(); len(profiles) > 0 {
		channels = profiles[0].Channels
	}
	if err := a.recordUC.Start(outputDir, filePrefix, channels); err != nil {
		return err
	}
	a.emitRecordingStatus(a.recordUC.Status())
	return nil
}

// StopRecording 停止录制
func (a *App) StopRecording() error {
	if err := a.recordUC.Stop(); err != nil {
		return err
	}
	a.emitRecordingStatus(a.recordUC.Status())
	return nil
}

// GetRecordingStatus 获取录制状态
func (a *App) GetRecordingStatus() core.RecordingSession {
	return a.recordUC.Status()
}

// StartLogFile 开始将日志写入文件
func (a *App) StartLogFile(outputDir string, prefix string) error {
	if err := a.logUC.Start(outputDir, prefix); err != nil {
		return err
	}
	a.EmitLog(LogEvent{Level: "info", Category: "system", Source: "logging", Message: "日志文件保存已开启"})
	return nil
}

// StopLogFile 停止日志文件写入
func (a *App) StopLogFile() error {
	if err := a.logUC.Stop(); err != nil {
		return err
	}
	a.EmitLog(LogEvent{Level: "info", Category: "system", Source: "logging", Message: "日志文件保存已关闭"})
	return nil
}

// GetLogFileState 获取日志文件写入状态
func (a *App) GetLogFileState() LogFileState {
	return LogFileState{
		Active:    a.logUC.IsActive(),
		OutputDir: a.logUC.GetOutputDir(),
	}
}

// PickDirectory 选择目录对话框
func (a *App) PickDirectory() (string, error) {
	app := a.app
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
