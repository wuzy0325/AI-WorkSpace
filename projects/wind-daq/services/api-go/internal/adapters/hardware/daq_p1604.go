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

type DAQP1604 struct {
	mu          sync.RWMutex
	profile     device.Profile
	status      device.Status
	sink        device.DataSink
	stop        chan struct{}
	acquiring   bool
	port        *serialport.Port
	readErrors  int
	frameErrors int
}

func NewDAQP1604(profile device.Profile) *DAQP1604 {
	return &DAQP1604{
		profile: profile,
		status: device.Status{
			ID:         profile.ID,
			Name:       profile.Name,
			Type:       profile.Type,
			Connection: device.ConnectionDisconnected,
		},
	}
}

func (d *DAQP1604) ID() string { return d.profile.ID }

func (d *DAQP1604) Connect() error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.port != nil {
		return nil
	}

	portName := d.profile.Address
	if portName == "" {
		return fmt.Errorf("serial port address is required — set Address in device profile")
	}

	cfg := serialport.DefaultConfig(portName)
	cfg.ReadTimeout = 3 * time.Second
	port, err := serialport.Open(cfg)
	if err != nil {
		return fmt.Errorf("open serial %s: %w", portName, err)
	}

	d.port = port
	d.status.Connection = device.ConnectionConnected
	return nil
}

func (d *DAQP1604) Disconnect() error {
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

func (d *DAQP1604) StartAcquisition() error {
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

func (d *DAQP1604) StopAcquisition() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.stopAcquisitionLocked()
}

func (d *DAQP1604) stopAcquisitionLocked() error {
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

func (d *DAQP1604) SetDataSink(sink device.DataSink) {
	d.mu.Lock()
	d.sink = sink
	d.mu.Unlock()
}

func (d *DAQP1604) Status() device.Status {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.status
}

func (d *DAQP1604) SetUnit(unit string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	for i := range d.profile.Channels {
		d.profile.Channels[i].Unit = unit
	}
	return nil
}

func (d *DAQP1604) SetTare(channelIndex int, offset float64) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if channelIndex < 0 || channelIndex >= len(d.profile.Channels) {
		return fmt.Errorf("invalid channel index: %d", channelIndex)
	}
	d.profile.Channels[channelIndex].TareOffset = offset
	return nil
}

func (d *DAQP1604) GetTare(channelIndex int) (float64, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	if channelIndex < 0 || channelIndex >= len(d.profile.Channels) {
		return 0, fmt.Errorf("invalid channel index: %d", channelIndex)
	}
	return d.profile.Channels[channelIndex].TareOffset, nil
}

func (d *DAQP1604) ClearTare(channelIndex int) error {
	return d.SetTare(channelIndex, 0)
}

func (d *DAQP1604) readLoop(stop <-chan struct{}) {
	if err := d.port.SetReadTimeout(10 * time.Millisecond); err != nil {
		slog.Warn("DAQ-P-1604 set read timeout", "error", err)
	}
	for {
		select {
		case <-stop:
			return
		default:
			d.readFrame()
		}
	}
}

func (d *DAQP1604) readFrame() {
	d.mu.RLock()
	port := d.port
	sink := d.sink
	d.mu.RUnlock()

	if port == nil || sink == nil {
		return
	}

	buf := make([]byte, 1024)
	n, err := port.Read(buf)
	if err != nil {
		d.mu.Lock()
		d.readErrors++
		d.mu.Unlock()
		slog.Debug("DAQ-P-1604 read error", "device", d.profile.ID, "error", err)
		return
	}
	if n < 5 {
		return
	}

	var channels []float64
	if sharedproto.IsASCIIFrame(buf[:n]) {
		channels = d.parseASCIIFrame(buf[:n])
	} else {
		channels, err = sharedproto.ParseStreamFrame(buf[:n])
		if err != nil {
			d.mu.Lock()
			d.frameErrors++
			d.mu.Unlock()
			slog.Debug("DAQ-P-1604 frame parse error", "device", d.profile.ID, "n", n, "error", err)
			return
		}
	}

	if len(channels) == 0 {
		return
	}

	indices := make([]int, len(channels))
	values := make([]float64, len(channels))
	for i := range channels {
		indices[i] = i
		values[i] = channels[i]
	}

	sink(device.DataPayload{
		DeviceID:       d.profile.ID,
		Timestamp:      device.NowMs(),
		Channels:       values,
		ChannelIndices: indices,
	})
}

func (d *DAQP1604) parseASCIIFrame(data []byte) []float64 {
	var values []float64
	start := 0
	for i, b := range data {
		if b == ',' || b == '\r' || b == '\n' || b == ' ' {
			if i > start {
				var v float64
				if _, err := fmt.Sscanf(string(data[start:i]), "%f", &v); err == nil {
					values = append(values, v)
				}
			}
			start = i + 1
		}
	}
	return values
}
