package hardware

import (
	"fmt"
	"sync"

	sharedcore "shared.local/device-sdk/go/daq/core"
	sharedhw "shared.local/device-sdk/go/daq/hardware"

	"windlabx4/services/api-go/internal/core/device"
	"windlabx4/services/api-go/internal/ports"
)

type PACE1000Adapter struct {
	mu      sync.RWMutex
	profile device.Profile
	driver  *sharedhw.PACE1000
	sink    device.DataSink
	onError func(error)
}

var _ ports.Device = (*PACE1000Adapter)(nil)
var _ ports.ErrorNotifiable = (*PACE1000Adapter)(nil)

func NewPACE1000Adapter(profile device.Profile) *PACE1000Adapter {
	return &PACE1000Adapter{profile: profile}
}

func (a *PACE1000Adapter) ID() string { return a.profile.ID }

func (a *PACE1000Adapter) SetOnError(fn func(error)) {
	a.mu.Lock()
	a.onError = fn
	a.mu.Unlock()
}

func (a *PACE1000Adapter) Connect() error {
	a.mu.Lock()
	if a.driver != nil {
		a.mu.Unlock()
		return nil
	}
	driver := sharedhw.NewPACE1000(mapToSharedPACE1000Profile(a.profile))
	driver.SetOnError(a.handleError)
	if a.sink != nil {
		a.setDriverSink(driver)
	}
	a.mu.Unlock()
	if err := driver.Connect(); err != nil {
		return fmt.Errorf("connect PACE1000: %w", err)
	}
	a.mu.Lock()
	a.driver = driver
	a.mu.Unlock()
	return nil
}

func (a *PACE1000Adapter) Disconnect() error {
	a.mu.Lock()
	driver := a.driver
	a.driver = nil
	a.mu.Unlock()
	if driver != nil {
		return driver.Disconnect()
	}
	return nil
}

func (a *PACE1000Adapter) StartAcquisition() error {
	a.mu.RLock()
	driver := a.driver
	a.mu.RUnlock()
	if driver == nil {
		return fmt.Errorf("device not connected")
	}
	return driver.StartAcquisition()
}

func (a *PACE1000Adapter) StopAcquisition() error {
	a.mu.RLock()
	driver := a.driver
	a.mu.RUnlock()
	if driver == nil {
		return nil
	}
	return driver.StopAcquisition()
}

func (a *PACE1000Adapter) SetDataSink(sink device.DataSink) {
	a.mu.Lock()
	a.sink = sink
	driver := a.driver
	a.mu.Unlock()
	if driver != nil {
		a.setDriverSink(driver)
	}
}

func (a *PACE1000Adapter) Status() device.Status {
	a.mu.RLock()
	driver := a.driver
	a.mu.RUnlock()
	if driver == nil {
		return device.Status{ID: a.profile.ID, Name: a.profile.Name, Type: a.profile.Type, Connection: device.ConnectionDisconnected}
	}
	return mapToDeviceStatus(driver.Status())
}

func (a *PACE1000Adapter) setDriverSink(driver *sharedhw.PACE1000) {
	driver.SetDataSink(func(payload sharedcore.DataPayload) {
		a.mu.RLock()
		sink := a.sink
		a.mu.RUnlock()
		if sink != nil {
			sink(mapToDevicePayload(payload, a.profile.Type, a.profile.Name))
		}
	})
}

func (a *PACE1000Adapter) handleError(err error) {
	a.mu.RLock()
	fn := a.onError
	a.mu.RUnlock()
	if fn != nil {
		fn(err)
	}
}

func mapToSharedPACE1000Profile(profile device.Profile) sharedcore.Profile {
	channels := make([]sharedcore.ChannelConfig, len(profile.Channels))
	for i, channel := range profile.Channels {
		channels[i] = sharedcore.ChannelConfig{
			Index: channel.Index, Name: channel.Name, Enabled: channel.Enabled,
			Unit: channel.Unit, Precision: channel.Precision,
			RangeMin: channel.RangeMin, RangeMax: channel.RangeMax,
		}
	}
	return sharedcore.Profile{
		ID: profile.ID, Name: profile.Name, Type: sharedcore.DevicePACE1000,
		Transport: "serial", SerialPort: profile.SerialPort, BaudRate: profile.BaudRate,
		SamplingRate: profile.SamplingRate, Channels: channels,
	}
}
