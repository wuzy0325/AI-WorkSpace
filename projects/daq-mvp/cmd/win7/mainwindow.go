package main

import (
	"time"

	"daq-mvp/internal/usecase"

	"github.com/lxn/walk"
	. "github.com/lxn/walk/declarative"
)

// CreateMainWindow builds the main application window using Walk declarative API.
func CreateMainWindow(app *WalkApp) (*walk.MainWindow, error) {
	var mw *walk.MainWindow
	var statusLabel *walk.Label

	var waveform *WaveformWidget
	var metrics *MetricsDisplay

	// Create the waveform first so we have it for the CustomWidget Paint callback.
	var wfErr error
	waveform, wfErr = NewWaveformWidget(nil) // parent set via declarative
	if wfErr != nil {
		return nil, wfErr
	}

	if err := (MainWindow{
		AssignTo: &mw,
		Title:    "DAQ MVP",
		Size:     Size{Width: 960, Height: 600},
		MinSize:  Size{Width: 640, Height: 400},
		Layout:   VBox{MarginsZero: true, SpacingZero: true},
		Children: []Widget{
			// Control bar
			Composite{
				Layout:     HBox{Margins: Margins{Left: 12, Top: 6, Right: 12, Bottom: 6}},
				Background: SolidColorBrush{Color: walk.RGB(0x1e, 0x29, 0x3b)},
				Children: []Widget{
					PushButton{
						Text:      "Start",
						MinSize:   Size{Width: 80, Height: 28},
						OnClicked: app.Start,
					},
					PushButton{
						Text:      "Stop",
						MinSize:   Size{Width: 80, Height: 28},
						OnClicked: app.Stop,
					},
					Label{
						AssignTo:  &statusLabel,
						Text:      "IDLE",
						Font:      Font{PointSize: 12, Bold: true},
						TextColor: walk.RGB(0x94, 0xa3, 0xb8),
					},
				},
			},
			// Waveform
			CustomWidget{
				Paint: func(canvas *walk.Canvas, bounds walk.Rectangle) error {
					if waveform != nil {
						return waveform.onPaint(canvas, bounds)
					}
					return nil
				},
			},
		},
	}.Create()); err != nil {
		return nil, err
	}

	// Create metrics display as a child of the main window
	var metricsErr error
	metrics, metricsErr = NewMetricsDisplay(mw)
	if metricsErr != nil {
		return nil, metricsErr
	}

	app.SetUI(waveform, metrics, statusLabel)

	// Periodic UI refresh timer running on the UI thread
	timer := time.NewTicker(50 * time.Millisecond)
	go func() {
		for range timer.C {
			if mw != nil {
				mw.Synchronize(func() {
					st := app.actor.Status()
					if metrics != nil {
						metrics.Update(st)
					}
					if statusLabel != nil {
						if st.State == usecase.StateRunning {
							statusLabel.SetText("ACQUIRING")
							statusLabel.SetTextColor(walk.RGB(0x22, 0xc5, 0x5e))
						} else {
							statusLabel.SetText("IDLE")
							statusLabel.SetTextColor(walk.RGB(0x94, 0xa3, 0xb8))
						}
					}
					if waveform != nil {
						waveform.CustomWidget.Invalidate()
					}
				})
			}
		}
	}()

	return mw, nil
}
