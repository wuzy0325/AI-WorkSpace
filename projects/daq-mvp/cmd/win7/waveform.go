package main

import (
	"sync"

	"daq-mvp/internal/core"

	"github.com/lxn/walk"
)

const maxPoints = 2000

var channelColors = [4]walk.Color{
	walk.RGB(0x3B, 0x82, 0xF6),
	walk.RGB(0x22, 0xC5, 0x5E),
	walk.RGB(0xF5, 0x9E, 0x0B),
	walk.RGB(0xA8, 0x55, 0xF7),
}

var (
	bgColor   = walk.RGB(0x0f, 0x17, 0x2a)
	gridColor = walk.RGB(0x1e, 0x29, 0x3b)
)

// WaveformWidget handles real-time waveform rendering via GDI.
type WaveformWidget struct {
	*walk.CustomWidget

	mu       sync.Mutex
	buffers  [4][]float32
	writeIdx int
	count    int
}

func NewWaveformWidget(parent walk.Container) (*WaveformWidget, error) {
	w := &WaveformWidget{}
	for i := range w.buffers {
		w.buffers[i] = make([]float32, maxPoints)
	}

	cw, err := walk.NewCustomWidget(parent, 0, w.onPaint)
	if err != nil {
		return nil, err
	}
	w.CustomWidget = cw
	return w, nil
}

func (w *WaveformWidget) PushFrame(frame core.UiSampleFrame) {
	w.mu.Lock()
	defer w.mu.Unlock()

	if frame.SampleCount == 0 {
		return
	}

	nc := len(frame.LatestValues)
	idx := w.writeIdx % maxPoints
	for ch := 0; ch < nc && ch < 4; ch++ {
		w.buffers[ch][idx] = frame.LatestValues[ch]
	}
	w.writeIdx++
	if w.count < maxPoints {
		w.count++
	}
}

func (w *WaveformWidget) onPaint(canvas *walk.Canvas, bounds walk.Rectangle) error {
	w.mu.Lock()
	writeIdx := w.writeIdx
	count := w.count
	bufCopy := w.buffers
	w.mu.Unlock()

	cw := bounds.Width
	ch := bounds.Height

	// Background
	brush, err := walk.NewSolidColorBrush(bgColor)
	if err != nil {
		return err
	}
	defer brush.Dispose()
	if err := canvas.FillRectangle(brush, bounds); err != nil {
		return err
	}

	// Grid — 6 horizontal lines
	gridPen, err := walk.NewCosmeticPen(walk.PenSolid, gridColor)
	if err != nil {
		return err
	}
	defer gridPen.Dispose()

	for gy := 0; gy <= 6; gy++ {
		y := bounds.Y + (ch * gy / 6)
		canvas.DrawLine(gridPen,
			walk.Point{X: bounds.X, Y: y},
			walk.Point{X: bounds.X + cw, Y: y})
	}

	if count < 2 {
		return nil
	}

	start := 0
	if writeIdx > maxPoints {
		start = writeIdx % maxPoints
	}

	// 4 channels, each in a vertical band
	const nc = 4
	bandH := ch / nc

	for chIdx := 0; chIdx < nc; chIdx++ {
		buf := bufCopy[chIdx]
		yOff := bandH*chIdx + bandH/2
		amp := float64(bandH) * 0.4

		pen, err := walk.NewCosmeticPen(walk.PenSolid, channelColors[chIdx])
		if err != nil {
			return err
		}

		for i := 0; i < count-1; i++ {
			idx1 := (start + i) % maxPoints
			idx2 := (start + i + 1) % maxPoints

			x1 := bounds.X + (i * cw / (count - 1))
			x2 := bounds.X + ((i + 1) * cw / (count - 1))
			y1 := yOff - int(float64(buf[idx1])*amp)
			y2 := yOff - int(float64(buf[idx2])*amp)

			canvas.DrawLine(pen,
				walk.Point{X: x1, Y: y1},
				walk.Point{X: x2, Y: y2})
		}
		pen.Dispose()
	}

	return nil
}
