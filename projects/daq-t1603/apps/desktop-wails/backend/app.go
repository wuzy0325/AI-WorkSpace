package backend

import (
	"context"
	"log/slog"

	"github.com/wailsapp/wails/v2/pkg/runtime"
	"daq-t1603/core"
	"daq-t1603/usecase"
)

type App struct {
	ctx      context.Context
	cancel   context.CancelFunc
	deviceUC *usecase.DeviceUsecase
	recordUC *usecase.RecordingUsecase
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
}

func (a *App) Shutdown(ctx context.Context) {
	if a.cancel != nil {
		a.cancel()
	}
	slog.Info("DAQ-T-1603 application shut down")
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
	return a.deviceUC.Connect(id)
}

func (a *App) Disconnect(id string) error {
	return a.deviceUC.Disconnect(id)
}

func (a *App) StartAcquisition(id string) error {
	ch, err := a.deviceUC.StartAcquisition(id)
	if err != nil {
		return err
	}
	go a.relayStream(id, ch)
	return nil
}

func (a *App) StopAcquisition(id string) error {
	return a.deviceUC.StopAcquisition(id)
}

func (a *App) GetStatus(id string) (core.DeviceState, bool) {
	return a.deviceUC.GetStatus(id)
}

func (a *App) ApplyConfig(id string, cfg core.T1603Config) error {
	return a.deviceUC.ApplyConfig(id, cfg)
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
