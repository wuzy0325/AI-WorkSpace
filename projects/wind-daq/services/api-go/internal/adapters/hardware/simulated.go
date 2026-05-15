package hardware

import (
	"context"
	"math/rand"
	"sync"
	"time"

	"wind-daq/services/api-go/internal/core/device"
)

type SimulatedDevice struct {
	*BaseDevice
	mu        sync.Mutex
	acquiring bool
	cancel    context.CancelFunc
}

func NewSimulatedDevice(config device.DeviceConfig) *SimulatedDevice {
	return &SimulatedDevice{BaseDevice: NewBaseDevice(config)}
}

func (s *SimulatedDevice) Connect() error {
	s.setState(device.StateConnected)
	return nil
}

func (s *SimulatedDevice) Disconnect() error {
	s.mu.Lock()
	if s.cancel != nil {
		s.cancel()
		s.cancel = nil
	}
	s.acquiring = false
	s.mu.Unlock()
	s.setState(device.StateDisconnected)
	return nil
}

func (s *SimulatedDevice) StartAcquisition() error {
	s.mu.Lock()
	if s.acquiring {
		s.mu.Unlock()
		return nil
	}
	s.acquiring = true
	ctx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel
	s.mu.Unlock()
	s.setState(device.StateAcquiring)
	s.setAcquiring(true)
	go s.generateData(ctx)
	return nil
}

func (s *SimulatedDevice) StopAcquisition() error {
	s.mu.Lock()
	if s.cancel != nil {
		s.cancel()
		s.cancel = nil
	}
	s.acquiring = false
	s.mu.Unlock()
	s.setState(device.StateConnected)
	s.setAcquiring(false)
	return nil
}

func (s *SimulatedDevice) generateData(ctx context.Context) {
	config := s.Config()
	interval := time.Duration(1000/config.SamplingRate) * time.Millisecond
	if interval < 10*time.Millisecond {
		interval = 10 * time.Millisecond
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	numChannels := 16
	indices := make([]int, numChannels)
	for i := range indices {
		indices[i] = i
	}

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			channels := make([]float64, numChannels)
			for i := range channels {
				channels[i] = rand.Float64()*200 - 100
			}
			s.EmitData(device.DataPayload{
				DeviceID: s.ID(), Timestamp: device.NowMs(),
				Channels: channels, ChannelIndices: indices,
			})
		}
	}
}
