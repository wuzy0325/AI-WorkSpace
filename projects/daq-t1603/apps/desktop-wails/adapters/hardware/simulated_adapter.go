package hardware

import (
	"fmt"
	"math"
	"sync"
	"time"

	"daq-t1603/core"
	"daq-t1603/ports"
)

type SimulatedAdapter struct {
	mu       sync.RWMutex
	status   map[string]*core.DeviceState
	sinks    map[string]func(core.TemperatureSnapshot)
	channels map[string]chan core.TemperatureSnapshot
	stopChs  map[string]chan struct{}
}

func NewSimulatedAdapter() *SimulatedAdapter {
	return &SimulatedAdapter{
		status:   make(map[string]*core.DeviceState),
		sinks:    make(map[string]func(core.TemperatureSnapshot)),
		channels: make(map[string]chan core.TemperatureSnapshot),
		stopChs:  make(map[string]chan struct{}),
	}
}

var _ ports.DevicePort = (*SimulatedAdapter)(nil)

func (a *SimulatedAdapter) Connect(profile core.TemperatureProfile) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if _, exists := a.status[profile.ID]; exists {
		return fmt.Errorf("device %s already connected", profile.ID)
	}

	a.status[profile.ID] = &core.DeviceState{
		Profile:     profile,
		Status:      core.StatusConnected,
		ConnectedAt: core.TimestampMs(),
	}
	return nil
}

func (a *SimulatedAdapter) Disconnect(id string) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.stopAcquisitionLocked(id)
	if st, exists := a.status[id]; exists {
		st.Status = core.StatusDisconnected
	}
	return nil
}

func (a *SimulatedAdapter) StartAcquisition(id string) (<-chan core.TemperatureSnapshot, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if _, exists := a.status[id]; !exists {
		return nil, fmt.Errorf("device %s not connected", id)
	}
	if _, exists := a.channels[id]; exists {
		return nil, fmt.Errorf("device %s already acquiring", id)
	}

	ch := make(chan core.TemperatureSnapshot, 64)
	done := make(chan struct{})
	a.channels[id] = ch
	a.stopChs[id] = done

	if st, exists := a.status[id]; exists {
		st.Status = core.StatusAcquiring
		st.AcquiringAt = core.TimestampMs()
	}

	t0 := time.Now()
	go a.simulateLoop(id, ch, done, t0)
	return ch, nil
}

func (a *SimulatedAdapter) simulateLoop(id string, ch chan<- core.TemperatureSnapshot, done <-chan struct{}, t0 time.Time) {
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-done:
			return
		case t := <-ticker.C:
			elapsed := t.Sub(t0).Seconds()
			values := make([]float64, 16)
			for i := range values {
				base := 25.0 + float64(i)*0.5
				values[i] = base + 5*math.Sin(elapsed+float64(i)*0.3) + 0.5*math.Sin(elapsed*3+float64(i)*0.7)
			}
			ch <- core.TemperatureSnapshot{
				DeviceID:  id,
				Timestamp: t.UnixMilli(),
				Values:    values,
				Unit:      "°C",
			}
		}
	}
}

func (a *SimulatedAdapter) stopAcquisitionLocked(id string) {
	if done, ok := a.stopChs[id]; ok {
		close(done)
		delete(a.stopChs, id)
	}
	delete(a.sinks, id)
	if ch, ok := a.channels[id]; ok {
		close(ch)
		delete(a.channels, id)
	}
	if st, exists := a.status[id]; exists {
		st.Status = core.StatusConnected
	}
}

func (a *SimulatedAdapter) StopAcquisition(id string) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.stopAcquisitionLocked(id)
	return nil
}

func (a *SimulatedAdapter) Status(id string) (core.DeviceState, bool) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	st, ok := a.status[id]
	if !ok {
		return core.DeviceState{}, false
	}
	return *st, true
}

func (a *SimulatedAdapter) ApplyConfig(id string, cfg core.T1603Config) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if st, exists := a.status[id]; exists {
		st.Profile.T1603Cfg = cfg
		return nil
	}
	return fmt.Errorf("device %s not connected", id)
}

func (a *SimulatedAdapter) SetDataSink(id string, sink func(core.TemperatureSnapshot)) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.sinks[id] = sink
}
