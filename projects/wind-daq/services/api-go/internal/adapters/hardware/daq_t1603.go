package hardware

import (
	"fmt"
	"log/slog"
	"sync"
	"time"

	sharedproto "shared/device-sdk/go/protocol"
	"shared/device-sdk/go/serialport"
	"wind-daq/services/api-go/internal/core/device"
)

type DAQT1603 struct {
	mu          sync.RWMutex
	profile     device.Profile
	status      device.Status
	sink        device.DataSink
	stop        chan struct{}
	acquiring   bool
	port        *serialport.Port
	tcpAddress  string
	readErrors  int
	frameErrors int
}

func NewDAQT1603(profile device.Profile) *DAQT1603 {
	return &DAQT1603{
		profile: profile,
		status: device.Status{
			ID:         profile.ID,
			Name:       profile.Name,
			Type:       profile.Type,
			Connection: device.ConnectionDisconnected,
		},
		tcpAddress: profile.Address,
	}
}

func (d *DAQT1603) ID() string { return d.profile.ID }

func (d *DAQT1603) Connect() error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.port != nil {
		return nil
	}

	addr := d.tcpAddress
	if addr == "" {
		return fmt.Errorf("serial port address is required — set Address in device profile")
	}

	cfg := serialport.DefaultConfig(addr)
	cfg.BaudRate = 115200
	cfg.ReadTimeout = 3 * time.Second
	port, err := serialport.Open(cfg)
	if err != nil {
		return fmt.Errorf("open serial %s: %w", addr, err)
	}

	d.port = port
	d.status.Connection = device.ConnectionConnected
	return nil
}

func (d *DAQT1603) Disconnect() error {
	d.mu.Lock()
	defer d.mu.Unlock()

	_ = d.stopAcquisitionLocked()

	if d.port != nil {
		_ = d.port.Close()
		d.port = nil
	}

	d.status.Connection = device.ConnectionDisconnected
	return nil
}

func (d *DAQT1603) StartAcquisition() error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.acquiring {
		return nil
	}
	if d.port == nil {
		return fmt.Errorf("device not connected")
	}

	d.acquiring = true
	d.status.Acquiring = true
	d.status.Connection = device.ConnectionAcquiring
	d.stop = make(chan struct{})
	stop := d.stop

	go d.readLoop(stop)
	return nil
}

func (d *DAQT1603) StopAcquisition() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.stopAcquisitionLocked()
}

func (d *DAQT1603) stopAcquisitionLocked() error {
	if d.acquiring && d.stop != nil {
		close(d.stop)
	}
	d.acquiring = false
	d.stop = nil
	d.status.Acquiring = false
	if d.status.Connection == device.ConnectionAcquiring {
		d.status.Connection = device.ConnectionConnected
	}
	return nil
}

func (d *DAQT1603) SetDataSink(sink device.DataSink) {
	d.mu.Lock()
	d.sink = sink
	d.mu.Unlock()
}

func (d *DAQT1603) Status() device.Status {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.status
}

func (d *DAQT1603) SetUnit(unit string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	for i := range d.profile.Channels {
		d.profile.Channels[i].Unit = unit
	}
	return nil
}

func (d *DAQT1603) SetTare(channelIndex int, offset float64) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if channelIndex < 0 || channelIndex >= len(d.profile.Channels) {
		return fmt.Errorf("invalid channel index: %d", channelIndex)
	}
	d.profile.Channels[channelIndex].TareOffset = offset
	return nil
}

func (d *DAQT1603) GetTare(channelIndex int) (float64, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	if channelIndex < 0 || channelIndex >= len(d.profile.Channels) {
		return 0, fmt.Errorf("invalid channel index: %d", channelIndex)
	}
	return d.profile.Channels[channelIndex].TareOffset, nil
}

func (d *DAQT1603) ClearTare(channelIndex int) error {
	return d.SetTare(channelIndex, 0)
}

func (d *DAQT1603) GetDaqT1603Config() (device.DaqT1603HardwareConfig, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.profile.DaqT1603Config, nil
}

func (d *DAQT1603) ApplyDaqT1603Config(cfg device.DaqT1603HardwareConfig) error {
	d.mu.Lock()
	d.profile.DaqT1603Config = cfg
	d.mu.Unlock()
	return nil
}

func (d *DAQT1603) readLoop(stop <-chan struct{}) {
	const frameInterval = 200 * time.Millisecond
	ticker := time.NewTicker(frameInterval)
	defer ticker.Stop()

	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
			d.readFrame()
		}
	}
}

func (d *DAQT1603) readFrame() {
	d.mu.RLock()
	port := d.port
	sink := d.sink
	d.mu.RUnlock()

	if port == nil || sink == nil {
		return
	}

	buf := make([]byte, sharedproto.SerialFrameSize)
	n, err := port.Read(buf)
	if err != nil {
		d.mu.Lock()
		d.readErrors++
		d.mu.Unlock()
		slog.Debug("DAQ-T-1603 read error", "device", d.profile.ID, "error", err)
		return
	}
	if n < 8 {
		return
	}

	temps, err := sharedproto.ParseSerialFrame(buf[:n])
	if err != nil {
		d.mu.Lock()
		d.frameErrors++
		d.mu.Unlock()
		slog.Debug("DAQ-T-1603 frame parse error", "device", d.profile.ID, "n", n, "error", err)
		return
	}

	indices := make([]int, len(temps))
	for i := range temps {
		indices[i] = i
	}

	sink(device.DataPayload{
		DeviceID:       d.profile.ID,
		Timestamp:      device.NowMs(),
		Channels:       temps,
		ChannelIndices: indices,
	})
}
