package main

import (
	"context"
	"log/slog"

	wails "github.com/wailsapp/wails/v2/pkg/runtime"

	"daq-mvp/internal/adapters/hardware"
	"daq-mvp/internal/core"
	"daq-mvp/internal/ports"
	"daq-mvp/internal/usecase"
)

// App bridges the Wails frontend to the hexagonal backend.
type App struct {
	ctx   context.Context
	actor *usecase.DeviceActor
	dev   ports.DeviceDriver
}

func NewApp() *App {
	dev := hardware.NewMockDevice()
	app := &App{dev: dev}
	app.actor = usecase.NewDeviceActor(dev, app.onFrame)
	return app
}

// Startup is called by Wails when the app starts.
func (a *App) Startup(ctx context.Context) {
	a.ctx = ctx
	slog.Info("daq-mvp app started")
}

// Shutdown is called by Wails when the app closes.
func (a *App) Shutdown(_ context.Context) {
	a.actor.Stop()
	slog.Info("daq-mvp app stopped")
}

// --- Bound methods (callable from frontend via window.go.main.App.Xxx) ---

func (a *App) StartAcquisition() {
	if a.actor.Status().State == usecase.StateRunning {
		return
	}
	a.actor.Start()
	a.emitStatus()
}

func (a *App) StopAcquisition() {
	if a.actor.Status().State == usecase.StateIdle {
		return
	}
	a.actor.Stop()
	a.emitStatus()
}

// GetStatus returns the current acquisition status.
func (a *App) GetStatus() usecase.Status {
	return a.actor.Status()
}

// GetDevices returns the list of available devices.
func (a *App) GetDevices() []ports.DeviceInfo {
	return []ports.DeviceInfo{a.dev.Info()}
}

// GetRuntimeStats returns operational counters.
func (a *App) GetRuntimeStats() usecase.RuntimeStats {
	return a.actor.Stats()
}

// --- Internal ---

func (a *App) onFrame(frame core.UiSampleFrame) {
	if a.ctx == nil {
		return
	}
	wails.EventsEmit(a.ctx, "waveform", frame)
}

func (a *App) emitStatus() {
	wails.EventsEmit(a.ctx, "status", a.actor.Status())
}
