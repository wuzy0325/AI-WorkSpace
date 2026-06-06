package backend

import (
	"context"
	"log/slog"

	"daq-t1603/core"
	"daq-t1603/usecase"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

type App struct {
	ctx      context.Context
	cancel   context.CancelFunc
	deviceUC *usecase.DeviceUsecase
	recordUC *usecase.RecordingUsecase
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

func NewApp(deviceUC *usecase.DeviceUsecase, recordUC *usecase.RecordingUsecase) *App {
	return &App{
		deviceUC: deviceUC,
		recordUC: recordUC,
	}
}

func (a *App) Startup(ctx context.Context) {
	a.ctx, a.cancel = context.WithCancel(ctx)
	slog.Info("DAQ-T-1603 application started")
	a.EmitLog(LogEvent{
		Level:    "info",
		Category: "system",
		Source:   "app",
		Message:  "DAQ-T-1603 application started",
	})
}

func (a *App) Shutdown(ctx context.Context) {
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
	if a.ctx == nil {
		return
	}
	runtime.EventsEmit(a.ctx, "daq:log", entry)
}

func (a *App) ScanDevices() ([]core.ScanResult, error) {
	return a.deviceUC.ScanDevices()
}

func (a *App) GetProfiles() []core.TemperatureProfile {
	return a.deviceUC.GetProfiles()
}

func (a *App) UpsertProfile(profile core.TemperatureProfile) error {
	return a.deviceUC.UpsertProfile(profile)
}

func (a *App) DeleteProfile(id string) error {
	return a.deviceUC.DeleteProfile(id)
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
	go a.relayStream(id, ch)
	a.EmitLog(LogEvent{Level: "info", Category: "acquisition", DeviceID: id, Source: "device", Message: "Acquisition started"})
	return nil
}

func (a *App) StopAcquisition(id string) error {
	a.EmitLog(LogEvent{Level: "info", Category: "acquisition", DeviceID: id, Source: "device", Message: "Stop acquisition requested"})
	if err := a.deviceUC.StopAcquisition(id); err != nil {
		a.EmitLog(LogEvent{Level: "error", Category: "acquisition", DeviceID: id, Source: "device", Message: "Stop acquisition failed", Detail: err.Error()})
		return err
	}
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

func (a *App) relayStream(deviceID string, ch <-chan core.TemperatureSnapshot) {
	for snapshot := range ch {
		runtime.EventsEmit(a.ctx, "daq:payload", snapshot)
		if a.recordUC.Status().Status == core.RecordingActive {
			_ = a.recordUC.Write(snapshot)
			runtime.EventsEmit(a.ctx, "daq:recording-status", a.recordUC.Status())
		}
	}
}

func (a *App) StartRecording(outputDir string, filePrefix string) error {
	return a.recordUC.Start(outputDir, filePrefix)
}

func (a *App) StopRecording() error {
	return a.recordUC.Stop()
}

func (a *App) GetRecordingStatus() core.RecordingSession {
	return a.recordUC.Status()
}

func (a *App) PickDirectory() (string, error) {
	return runtime.OpenDirectoryDialog(a.ctx, runtime.OpenDialogOptions{
		Title:                "选择保存目录",
		CanCreateDirectories: true,
	})
}
