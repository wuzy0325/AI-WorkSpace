package hardware

import (
	"math"
	"sync"
	"time"

	"wind-daq/services/api-go/internal/core/device"
)

type SimulatedDevice struct {
	mu        sync.RWMutex
	profile   device.Profile
	status    device.Status
	sink      device.DataSink
	stop      chan struct{}
	acquiring bool
}

func NewSimulatedDevice(profile device.Profile) *SimulatedDevice {
	return &SimulatedDevice{
		profile: profile,
		status: device.Status{
			ID:         profile.ID,
			Name:       profile.Name,
			Type:       profile.Type,
			Connection: device.ConnectionDisconnected,
		},
	}
}

func (d *SimulatedDevice) ID() string { return d.profile.ID }

func (d *SimulatedDevice) Connect() error {
	d.mu.Lock()
	d.status.Connection = device.ConnectionConnected
	d.mu.Unlock()
	return nil
}

func (d *SimulatedDevice) Disconnect() error {
	_ = d.StopAcquisition()
	d.mu.Lock()
	d.status.Connection = device.ConnectionDisconnected
	d.mu.Unlock()
	return nil
}

func (d *SimulatedDevice) StartAcquisition() error {
	d.mu.Lock()
	if d.acquiring {
		d.mu.Unlock()
		return nil
	}
	d.acquiring = true
	d.status.Acquiring = true
	d.status.Connection = device.ConnectionAcquiring
	d.stop = make(chan struct{})
	stop := d.stop
	d.mu.Unlock()

	go d.loop(stop)
	return nil
}

func (d *SimulatedDevice) StopAcquisition() error {
	d.mu.Lock()
	if d.acquiring && d.stop != nil {
		close(d.stop)
	}
	d.acquiring = false
	d.stop = nil
	d.status.Acquiring = false
	if d.status.Connection == device.ConnectionAcquiring {
		d.status.Connection = device.ConnectionConnected
	}
	d.mu.Unlock()
	return nil
}

func (d *SimulatedDevice) SetDataSink(sink device.DataSink) {
	d.mu.Lock()
	d.sink = sink
	d.mu.Unlock()
}

func (d *SimulatedDevice) SetUnit(unit string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	for i := range d.profile.Channels {
		d.profile.Channels[i].Unit = unit
	}
	return nil
}

func (d *SimulatedDevice) Status() device.Status {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.status
}

func (d *SimulatedDevice) loop(stop <-chan struct{}) {
	interval := time.Second / time.Duration(d.profile.SamplingRate)
	if interval <= 0 {
		interval = 50 * time.Millisecond
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	start := time.Now()
	for {
		select {
		case <-stop:
			return
		case now := <-ticker.C:
			d.emit(now.Sub(start).Seconds())
		}
	}
}

func (d *SimulatedDevice) emit(seconds float64) {
	d.mu.RLock()
	sink := d.sink
	channels := d.profile.Channels
	d.mu.RUnlock()
	if sink == nil {
		return
	}
	values := make([]float64, 0, len(channels))
	indices := make([]int, 0, len(channels))
	for _, channel := range channels {
		if !channel.Enabled {
			continue
		}
		indices = append(indices, channel.Index)
		values = append(values, math.Sin(seconds+float64(channel.Index)))
	}
	sink(device.DataPayload{
		DeviceID:       d.profile.ID,
		Timestamp:      device.NowMs(),
		Channels:       values,
		ChannelIndices: indices,
	})
}
