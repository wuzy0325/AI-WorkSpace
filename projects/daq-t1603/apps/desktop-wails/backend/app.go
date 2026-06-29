package backend

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"daq-t1603/core"
	"daq-t1603/usecase"
	"github.com/wailsapp/wails/v3/pkg/application"
)

const (
	uiPayloadRefreshInterval    = 100 * time.Millisecond
	recordingStatusEmitInterval = time.Second
)

type App struct {
	ctx         context.Context
	cancel      context.CancelFunc
	deviceUC    *usecase.DeviceUsecase
	recordUC    *usecase.RecordingUsecase
	logUC       *usecase.LogUsecase
	logDir      string // 默认日志文件保存目录
	mu          sync.Mutex
	relays      map[string]*relayControl
}

type relayControl struct {
	cancel context.CancelFunc
	done   chan struct{}
}

type LogEvent struct {
	Level     string `json:"level"`
	Category  string `json:"category"`
	DeviceID  string `json:"deviceId,omitempty"`
	Source    string `json:"source"`
	Message   string `json:"message"`
	Detail    string `json:"detail,omitempty"`
	Timestamp int64  `json:"timestamp"`
}

// LogFileState 日志文件写入状态，用于前端展示
type LogFileState struct {
	Active    bool   `json:"active"`
	OutputDir string `json:"outputDir,omitempty"`
}

func NewApp(deviceUC *usecase.DeviceUsecase, recordUC *usecase.RecordingUsecase, logUC *usecase.LogUsecase, logDir string) *App {
	return &App{
		deviceUC: deviceUC,
		recordUC: recordUC,
		logUC:    logUC,
		logDir:   logDir,
		relays:   make(map[string]*relayControl),
	}
}

func (a *App) Startup(ctx context.Context) {
	a.ctx, a.cancel = context.WithCancel(ctx)

	// 自动启动日志文件写入
	if a.logDir != "" {
		if err := a.logUC.Start(a.logDir, "daq-log"); err != nil {
			slog.Error("自动启动日志文件写入失败", "error", err)
		} else {
			slog.Info("日志文件自动保存已开启", "dir", a.logDir)
		}
	}

	slog.Info("DAQ-T-1603 application started")
	a.EmitLog(LogEvent{
		Level:    "info",
		Category: "system",
		Source:   "app",
		Message:  "DAQ-T-1603 application started",
	})
}

func (a *App) Shutdown(ctx context.Context) {
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
		Message:  "DAQ-T-1603 application shut down",
	})
	slog.Info("DAQ-T-1603 application shut down")
}

func (a *App) EmitLog(entry LogEvent) {
	if entry.Timestamp == 0 {
		entry.Timestamp = core.TimestampMs()
	}
	if entry.Source == "" {
		entry.Source = "backend"
	}

	// 同步写入日志文件（如果已开启）
	if a.logUC != nil && a.logUC.IsActive() {
		if err := a.logUC.Write(entry.Timestamp, entry.Level, entry.Category, entry.DeviceID, entry.Source, entry.Message, entry.Detail); err != nil {
			slog.Error("写入日志文件失败", "error", err)
		}
	}

	if a.ctx == nil {
		return
	}
	application.Get().Event.Emit("daq:log", entry)
}

func (a *App) emitPayload(snapshot core.TemperatureSnapshot) {
	if a.ctx == nil {
		return
	}
	application.Get().Event.Emit("daq:payload", snapshot)
}

func (a *App) emitRecordingStatus(session core.RecordingSession) {
	if a.ctx == nil {
		return
	}
	application.Get().Event.Emit("daq:recording-status", session)
}

func (a *App) ScanDevices() ([]core.ScanResult, error) {
	return a.deviceUC.ScanDevices()
}

func (a *App) GetProfiles() []core.TemperatureProfile {
	return a.deviceUC.GetProfiles()
}

func (a *App) UpsertProfile(profile core.TemperatureProfile) error {
	a.EmitLog(LogEvent{Level: "info", Category: "system", DeviceID: profile.ID, Source: "config", Message: "保存设备配置"})
	if err := a.deviceUC.UpsertProfile(profile); err != nil {
		a.EmitLog(LogEvent{Level: "error", Category: "system", DeviceID: profile.ID, Source: "config", Message: "保存设备配置失败", Detail: err.Error()})
		return err
	}
	a.EmitLog(LogEvent{Level: "info", Category: "system", DeviceID: profile.ID, Source: "config", Message: "设备配置已保存"})
	return nil
}

func (a *App) DeleteProfile(id string) error {
	a.EmitLog(LogEvent{Level: "info", Category: "system", DeviceID: id, Source: "config", Message: "删除设备配置"})
	if err := a.deviceUC.DeleteProfile(id); err != nil {
		a.EmitLog(LogEvent{Level: "error", Category: "system", DeviceID: id, Source: "config", Message: "删除设备配置失败", Detail: err.Error()})
		return err
	}
	a.EmitLog(LogEvent{Level: "info", Category: "system", DeviceID: id, Source: "config", Message: "设备配置已删除"})
	return nil
}

func (a *App) Connect(id string) error {
	a.EmitLog(LogEvent{Level: "info", Category: "system", DeviceID: id, Source: "device", Message: "Connect requested"})
	if err := a.deviceUC.Connect(id); err != nil {
		a.EmitLog(LogEvent{Level: "error", Category: "system", DeviceID: id, Source: "device", Message: "Connect failed", Detail: err.Error()})
		return err
	}
	a.EmitLog(LogEvent{Level: "info", Category: "system", DeviceID: id, Source: "device", Message: "Device connected"})
	return nil
}

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

func (a *App) GetStatus(id string) (core.DeviceState, bool) {
	return a.deviceUC.GetStatus(id)
}

func (a *App) ApplyConfig(id string, cfg core.T1603Config) error {
	a.EmitLog(LogEvent{Level: "info", Category: "system", DeviceID: id, Source: "device", Message: "Apply config requested"})
	if err := a.deviceUC.ApplyConfig(id, cfg); err != nil {
		a.EmitLog(LogEvent{Level: "error", Category: "system", DeviceID: id, Source: "device", Message: "Apply config failed", Detail: err.Error()})
		return err
	}
	a.EmitLog(LogEvent{Level: "info", Category: "system", DeviceID: id, Source: "device", Message: "Config applied"})
	return nil
}

func (a *App) startRelay(deviceID string, ch <-chan core.TemperatureSnapshot) {
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

func (a *App) relayStream(ctx context.Context, deviceID string, ch <-chan core.TemperatureSnapshot) {
	uiTicker := time.NewTicker(uiPayloadRefreshInterval)
	statusTicker := time.NewTicker(recordingStatusEmitInterval)
	defer uiTicker.Stop()
	defer statusTicker.Stop()

	var latest core.TemperatureSnapshot
	hasLatest := false

	// 录制状态判断通过 RecordingUsecase.IsActive()（atomic.Bool），无锁、O(1)，
	// 每条 snapshot 直接查询，既避免锁竞争又不丢数据。
	// （此前缓存策略每秒只刷新一次，会丢失录制开启后 1s 内的全部样本。）

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
			if a.recordUC.IsActive() {
				if err := a.recordUC.Write(snapshot); err != nil {
					a.EmitLog(LogEvent{Level: "error", Category: "acquisition", DeviceID: deviceID, Source: "recording", Message: "Record snapshot failed", Detail: err.Error()})
				}
			}
		case <-uiTicker.C:
			if hasLatest {
				a.emitPayload(latest)
			}
		case <-statusTicker.C:
			if a.recordUC.IsActive() {
				a.emitRecordingStatus(a.recordUC.Status())
			}
		}
	}
}

func (a *App) StartRecording(outputDir string, filePrefix string) error {
	if err := a.recordUC.Start(outputDir, filePrefix); err != nil {
		return err
	}
	a.emitRecordingStatus(a.recordUC.Status())
	return nil
}

func (a *App) StopRecording() error {
	if err := a.recordUC.Stop(); err != nil {
		return err
	}
	a.emitRecordingStatus(a.recordUC.Status())
	return nil
}

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

func (a *App) PickDirectory() (string, error) {
	return pickDirectory(nil)
}
