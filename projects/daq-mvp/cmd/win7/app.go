package main

import (
	"log/slog"

	"daq-mvp/internal/adapters/hardware"
	"daq-mvp/internal/core"
	"daq-mvp/internal/ports"
	"daq-mvp/internal/usecase"

	"github.com/lxn/walk"
)

// WalkApp bridges the hexagonal backend to Walk UI.
type WalkApp struct {
	actor   *usecase.DeviceActor
	dev     ports.DeviceDriver
	mw      *walk.MainWindow
	wf      *WaveformWidget
	metrics *MetricsDisplay
	status  *walk.Label
}

func NewWalkApp() *WalkApp {
	dev := hardware.NewMockDevice()
	app := &WalkApp{dev: dev}
	app.actor = usecase.NewDeviceActor(dev, app.onFrame)
	return app
}

func (a *WalkApp) SetMainWindow(mw *walk.MainWindow) {
	a.mw = mw
}

func (a *WalkApp) SetUI(wf *WaveformWidget, metrics *MetricsDisplay, status *walk.Label) {
	a.wf = wf
	a.metrics = metrics
	a.status = status
}

func (a *WalkApp) Start() {
	if a.actor.Status().State == usecase.StateRunning {
		return
	}
	a.actor.Start()
	a.updateStatus()
}

func (a *WalkApp) Stop() {
	if a.actor.Status().State == usecase.StateIdle {
		return
	}
	a.actor.Stop()
	a.updateStatus()
}

func (a *WalkApp) Cleanup() {
	a.actor.Stop()
}

func (a *WalkApp) onFrame(frame core.UiSampleFrame) {
	if a.wf != nil {
		a.wf.PushFrame(frame)
	}
	if a.metrics != nil {
		a.metrics.Update(a.actor.Status())
	}
}

func (a *WalkApp) updateStatus() {
	st := a.actor.Status()
	slog.Info("status", "state", st.State, "batches", st.BatchCount, "samples", st.SampleCount)
	if a.mw != nil {
		a.mw.Synchronize(func() {
			if a.status != nil {
				if st.State == usecase.StateRunning {
					a.status.SetText("ACQUIRING")
				} else {
					a.status.SetText("IDLE")
				}
			}
			if a.metrics != nil {
				a.metrics.Update(st)
			}
		})
	}
}
