package usecase

import (
	"context"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"daq-mvp/internal/core"
	"daq-mvp/internal/ports"
)

// State represents the device acquisition state.
type State int32

const (
	StateIdle    State = 0
	StateRunning State = 1
)

func (s State) String() string {
	switch s {
	case StateIdle:
		return "idle"
	case StateRunning:
		return "running"
	default:
		return "unknown"
	}
}

// Status is the public snapshot returned to the frontend.
type Status struct {
	State        State     `json:"state"`
	SampleRateHz float64   `json:"sampleRateHz"`
	BatchCount   int64     `json:"batchCount"`
	SampleCount  int64     `json:"sampleCount"`
	LatestValues []float32 `json:"latestValues"`
}

// RuntimeStats carries operational counters.
type RuntimeStats struct {
	BatchesEmitted int64 `json:"batchesEmitted"`
	DroppedFrames  int64 `json:"droppedFrames"`
	UptimeMs       int64 `json:"uptimeMs"`
}

// FrameHandler is a callback invoked for each UiSampleFrame produced by the actor.
type FrameHandler func(core.UiSampleFrame)

// DeviceActor manages the lifecycle of a single device's acquisition loop.
type DeviceActor struct {
	device     ports.DeviceDriver
	onFrame    FrameHandler
	state      atomic.Int32
	batchCount atomic.Int64
	totalCount atomic.Int64
	dropped    atomic.Int64
	latest     [4]float32
	latestMu   sync.RWMutex
	cancel     context.CancelFunc
	startTime  time.Time
}

func NewDeviceActor(device ports.DeviceDriver, onFrame FrameHandler) *DeviceActor {
	a := &DeviceActor{device: device, onFrame: onFrame}
	a.state.Store(int32(StateIdle))
	return a
}

// Start begins the acquisition loop. Idempotent if already running.
func (a *DeviceActor) Start() {
	if !a.state.CompareAndSwap(int32(StateIdle), int32(StateRunning)) {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	a.cancel = cancel
	a.batchCount.Store(0)
	a.totalCount.Store(0)
	a.dropped.Store(0)
	a.startTime = time.Now()

	go a.loop(ctx)
	slog.Info("acquisition started")
}

// Stop ends the acquisition loop. Idempotent.
func (a *DeviceActor) Stop() {
	if !a.state.CompareAndSwap(int32(StateRunning), int32(StateIdle)) {
		return
	}
	if a.cancel != nil {
		a.cancel()
	}
	slog.Info("acquisition stopped")
}

// Status returns the current public snapshot.
func (a *DeviceActor) Status() Status {
	a.latestMu.RLock()
	lv := a.latest
	a.latestMu.RUnlock()
	return Status{
		State:        State(a.state.Load()),
		SampleRateHz: defaultRate,
		BatchCount:   a.batchCount.Load(),
		SampleCount:  a.totalCount.Load(),
		LatestValues: []float32{lv[0], lv[1], lv[2], lv[3]},
	}
}

// DeviceInfo returns the underlying device metadata.
func (a *DeviceActor) DeviceInfo() ports.DeviceInfo {
	return a.device.Info()
}

// Stats returns runtime counters.
func (a *DeviceActor) Stats() RuntimeStats {
	return RuntimeStats{
		BatchesEmitted: a.batchCount.Load(),
		DroppedFrames:  a.dropped.Load(),
		UptimeMs:       time.Since(a.startTime).Milliseconds(),
	}
}

const (
	defaultRate   = 1000.0
	batchInterval = 50 * time.Millisecond
)

func (a *DeviceActor) loop(ctx context.Context) {
	ticker := time.NewTicker(batchInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			slog.Info("acquisition loop exited")
			return
		case <-ticker.C:
			batch, err := a.device.ReadBatch(ctx)
			if err != nil {
				slog.Error("read batch failed", "err", err)
				a.dropped.Add(1)
				continue
			}

			a.batchCount.Add(1)
			a.totalCount.Add(int64(batch.SampleCount))

			nc := batch.ChannelCount
			ns := batch.SampleCount
			lv := make([]float32, nc)
			if ns > 0 {
				lastSampleOffset := (ns - 1) * nc
				for ch := range nc {
					lv[ch] = batch.Values[lastSampleOffset+ch]
				}
			}
			a.latestMu.Lock()
			copy(a.latest[:], lv)
			a.latestMu.Unlock()

			chIDs := make([]int, nc)
			for i := range nc {
				chIDs[i] = i
			}

			frame := core.UiSampleFrame{
				DeviceID:          batch.DeviceID,
				SequenceStart:     batch.SequenceStart,
				SampleCount:       batch.SampleCount,
				ChannelIDs:        chIDs,
				LatestValues:      lv,
				SamplesPerChannel: batch.SampleCount,
				HostTimestampMs:   batch.HostTimestampMs,
			}

			if a.onFrame != nil {
				a.onFrame(frame)
			}
		}
	}
}
