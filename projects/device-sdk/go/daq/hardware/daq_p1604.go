package hardware

import (
	"fmt"
	"log/slog"
	"net"
	"sync"
	"time"

	sharedproto "shared.local/device-sdk/go/protocol"
	"shared.local/device-sdk/go/daq/core"
)

const (
	DAQ_P_1604_DEFAULT_HOST = "192.168.3.101"
	DAQ_P_1604_DEFAULT_PORT = 9000
	DAQ_P_1604_TIMEOUT      = 5 * time.Second
)

type DAQP1604 struct {
	mu          sync.RWMutex
	profile     core.Profile
	status      core.Status
	sink        core.DataSink
	stop        chan struct{}
	acquiring   bool
	conn        net.Conn
	frameReader *sharedproto.FrameReader
	recvBuffer  []byte
	readErrors  int
	frameErrors int
}

func NewDAQP1604(profile core.Profile) *DAQP1604 {
	return &DAQP1604{
		profile: profile,
		status: core.Status{
			ID:         profile.ID,
			Name:       profile.Name,
			Type:       profile.Type,
			Connection: core.ConnectionDisconnected,
		},
		recvBuffer: make([]byte, 0, 4096),
	}
}

func (d *DAQP1604) ID() string { return d.profile.ID }

func (d *DAQP1604) Connect() error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.conn != nil {
		return nil
	}

	host := d.profile.Address
	if host == "" {
		host = DAQ_P_1604_DEFAULT_HOST
	}
	port := d.profile.Port
	if port <= 0 {
		port = DAQ_P_1604_DEFAULT_PORT
	}

	conn, err := net.DialTimeout("tcp", fmt.Sprintf("%s:%d", host, port), DAQ_P_1604_TIMEOUT)
	if err != nil {
		return fmt.Errorf("connect to %s:%d: %w", host, port, err)
	}

	d.conn = conn
	d.frameReader = sharedproto.NewFrameReader(conn)
	d.status.Connection = core.ConnectionConnected
	return nil
}

func (d *DAQP1604) Disconnect() error {
	d.mu.Lock()
	defer d.mu.Unlock()

	_ = d.stopAcquisitionLocked()

	if d.conn != nil {
		_ = d.conn.Close()
		d.conn = nil
		d.frameReader = nil
	}

	d.status.Connection = core.ConnectionDisconnected
	return nil
}

func (d *DAQP1604) StartAcquisition() error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.acquiring {
		return nil
	}
	if d.conn == nil {
		return fmt.Errorf("device not connected")
	}

	if err := d.initStream(); err != nil {
		return fmt.Errorf("init stream: %w", err)
	}

	if err := d.sendCommand("c 01 1"); err != nil {
		return fmt.Errorf("start stream: %w", err)
	}

	d.acquiring = true
	d.status.Acquiring = true
	d.status.Connection = core.ConnectionAcquiring
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
	if d.acquiring {
		if d.conn != nil {
			if err := d.sendCommand("c 02 1"); err != nil {
				slog.Warn("DAQ-P-1604 stop stream command failed", "device", d.profile.ID, "error", err)
			}
		}
		if d.stop != nil {
			close(d.stop)
		}
	}
	d.acquiring = false
	d.stop = nil
	d.status.Acquiring = false
	if d.status.Connection == core.ConnectionAcquiring {
		d.status.Connection = core.ConnectionConnected
	}
	return nil
}

func (d *DAQP1604) SetDataSink(sink core.DataSink) {
	d.mu.Lock()
	d.sink = sink
	d.mu.Unlock()
}

func (d *DAQP1604) Status() core.Status {
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

func (d *DAQP1604) initStream() error {
	if err := d.sendCommand("w1601"); err != nil {
		return fmt.Errorf("enable length prefix: %w", err)
	}
	time.Sleep(50 * time.Millisecond)

	if err := d.sendCommand("c 00 1 FFFF 1 100 7 0"); err != nil {
		return fmt.Errorf("set stream params: %w", err)
	}
	time.Sleep(50 * time.Millisecond)

	if err := d.sendCommand("c 05 1 0810"); err != nil {
		return fmt.Errorf("set stream content: %w", err)
	}
	time.Sleep(50 * time.Millisecond)

	return nil
}

func (d *DAQP1604) sendCommand(cmd string) error {
	if d.conn == nil {
		return fmt.Errorf("not connected")
	}
	d.conn.SetWriteDeadline(time.Now().Add(DAQ_P_1604_TIMEOUT))
	_, err := d.conn.Write([]byte(cmd + "\r\n"))
	return err
}

func (d *DAQP1604) readLoop(stop <-chan struct{}) {
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
				slog.Debug("DAQ-P-1604 read error", "device", d.profile.ID, "error", err)
				return
			}
			if len(payload) > 0 {
				d.processPayload(payload)
			}
		}
	}
}

func (d *DAQP1604) processPayload(data []byte) {
	d.mu.RLock()
	sink := d.sink
	d.mu.RUnlock()

	if sink == nil {
		return
	}

	if sharedproto.IsASCIIFrame(data) {
		channels := d.parseASCIIFrame(data)
		if len(channels) == 0 {
			return
		}
		indices := make([]int, len(channels))
		values := make([]float64, len(channels))
		for i := range channels {
			indices[i] = i
			values[i] = channels[i]
		}
		sink(core.DataPayload{
			DeviceID:       d.profile.ID,
			Timestamp:      core.NowMs(),
			Channels:       values,
			ChannelIndices: indices,
		})
		return
	}

	channels, err := sharedproto.ParseStreamFrame(data)
	if err != nil {
		d.mu.Lock()
		d.frameErrors++
		d.mu.Unlock()
		slog.Debug("DAQ-P-1604 frame parse error", "device", d.profile.ID, "n", len(data), "error", err)
		return
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

	sink(core.DataPayload{
		DeviceID:       d.profile.ID,
		Timestamp:      core.NowMs(),
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