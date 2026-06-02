package hardware

import (
	"fmt"
	"log/slog"
	"net"
	"sync"
	"time"

	sharedproto "shared.local/device-sdk/go/protocol"
	"wind-daq/services/api-go/internal/core/device"
)

const (
	DAQ_T_1603_DEFAULT_HOST = "192.168.3.101"
	DAQ_T_1603_DEFAULT_PORT = 9000
	DAQ_T_1603_TIMEOUT      = 5 * time.Second
)

type DAQT1603 struct {
	mu          sync.RWMutex
	profile     device.Profile
	status      device.Status
	sink        device.DataSink
	stop        chan struct{}
	acquiring   bool
	conn        net.Conn
	frameReader *sharedproto.FrameReader
	recvBuffer  []byte
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
		recvBuffer: make([]byte, 0, 4096),
	}
}

func (d *DAQT1603) ID() string { return d.profile.ID }

func (d *DAQT1603) Connect() error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.conn != nil {
		return nil
	}

	host := d.profile.Address
	if host == "" {
		host = DAQ_T_1603_DEFAULT_HOST
	}
	port := d.profile.Port
	if port <= 0 {
		port = DAQ_T_1603_DEFAULT_PORT
	}

	conn, err := net.DialTimeout("tcp", fmt.Sprintf("%s:%d", host, port), DAQ_T_1603_TIMEOUT)
	if err != nil {
		return fmt.Errorf("connect to %s:%d: %w", host, port, err)
	}

	d.conn = conn
	d.frameReader = sharedproto.NewFrameReader(conn)
	d.status.Connection = device.ConnectionConnected
	return nil
}

func (d *DAQT1603) Disconnect() error {
	d.mu.Lock()
	defer d.mu.Unlock()

	_ = d.stopAcquisitionLocked()

	if d.conn != nil {
		_ = d.conn.Close()
		d.conn = nil
		d.frameReader = nil
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
	if d.conn == nil {
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
	for {
		select {
		case <-stop:
			return
		default:
			d.conn.SetReadDeadline(time.Now().Add(200 * time.Millisecond))
			payload, err := d.frameReader.ReadFrame()
			if err != nil {
				if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
					continue
				}
				d.mu.Lock()
				d.readErrors++
				d.mu.Unlock()
				slog.Debug("DAQ-T-1603 read error", "device", d.profile.ID, "error", err)
				return
			}
			if len(payload) > 0 {
				d.processPayload(payload)
			}
		}
	}
}

func (d *DAQT1603) processPayload(data []byte) {
	d.mu.RLock()
	sink := d.sink
	d.mu.RUnlock()

	if sink == nil || len(data) < 8 {
		return
	}

	temps, err := sharedproto.ParseTCPFrame(data)
	if err != nil {
		d.mu.Lock()
		d.frameErrors++
		d.mu.Unlock()
		slog.Debug("DAQ-T-1603 frame parse error", "device", d.profile.ID, "n", len(data), "error", err)
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
