package hardware

import (
	"context"
	"math"
	"math/rand"
	"time"

	"daq-mvp/internal/core"
	"daq-mvp/internal/ports"
)

const (
	defaultSampleRate = 1000.0
	defaultChannels   = 4
	defaultBatchSize  = 50 // one batch every 50ms at 1kHz
)

var channelFreqs = [4]float64{10, 17, 23, 31}

// MockDevice simulates a DAQ card without real hardware.
type MockDevice struct {
	seq uint64
	rng *rand.Rand
}

func NewMockDevice() *MockDevice {
	return &MockDevice{rng: rand.New(rand.NewSource(time.Now().UnixNano()))}
}

func (m *MockDevice) Info() ports.DeviceInfo {
	return ports.DeviceInfo{
		ID:       "mock-001",
		Name:     "Mock DAQ Device",
		Channels: defaultChannels,
	}
}

func (m *MockDevice) Connect(_ context.Context) error { return nil }
func (m *MockDevice) Disconnect() error               { return nil }

func (m *MockDevice) ReadBatch(_ context.Context) (core.SampleBatch, error) {
	n := defaultBatchSize * defaultChannels
	values := make([]float32, n)
	now := time.Now().UnixMilli()

	t0 := float64(m.seq) / defaultSampleRate
	for s := range defaultBatchSize {
		t := t0 + float64(s)/defaultSampleRate
		for ch := range defaultChannels {
			v := math.Sin(2*math.Pi*channelFreqs[ch]*t)
			v += m.rng.Float64()*0.02 - 0.01
			values[s*defaultChannels+ch] = float32(v)
		}
	}

	batch := core.SampleBatch{
		DeviceID:        "mock-001",
		SequenceStart:   m.seq,
		SampleRateHz:    defaultSampleRate,
		ChannelCount:    defaultChannels,
		SampleCount:     defaultBatchSize,
		Values:          values,
		HostTimestampMs: now,
	}
	m.seq += uint64(defaultBatchSize)
	return batch, nil
}
