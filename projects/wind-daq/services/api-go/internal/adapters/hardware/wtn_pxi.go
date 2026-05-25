package hardware

import (
	"encoding/binary"
	"fmt"
	"log/slog"
	"math"
	"net"
	"sync"
	"time"

	"wind-daq/services/api-go/internal/core/device"
)

const (
	WTN_PXI_DEFAULT_HOST = "127.0.0.1"
	WTN_PXI_DEFAULT_PORT = 9000
	WTN_PXI_TIMEOUT      = 5 * time.Second
)

const (
	WTN_PXI_LENGTH_PREFIX_BYTES = 4
	WTN_PXI_MAX_PAYLOAD_BYTES   = 64 * 1024
	WTN_PXI_REQUIRED_CHANNELS   = 8
)

type WTNPXI struct {
	mu         sync.RWMutex
	profile    device.Profile
	status     device.Status
	sink       device.DataSink
	stop       chan struct{}
	acquiring  bool
	conn       net.Conn
	recvBuffer []byte
}

func NewWTNPXI(profile device.Profile) *WTNPXI {
	return &WTNPXI{
		profile: profile,
		status: device.Status{
			ID:         profile.ID,
			Name:       profile.Name,
			Type:       profile.Type,
			Connection: device.ConnectionDisconnected,
		},
		recvBuffer: make([]byte, 0, 8192),
	}
}

func (d *WTNPXI) ID() string { return d.profile.ID }

func (d *WTNPXI) Connect() error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.conn != nil {
		return nil
	}

	host := d.profile.Address
	if host == "" {
		host = WTN_PXI_DEFAULT_HOST
	}
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("%s:%d", host, WTN_PXI_DEFAULT_PORT), WTN_PXI_TIMEOUT)
	if err != nil {
		return fmt.Errorf("connect to %s:%d: %w", host, WTN_PXI_DEFAULT_PORT, err)
	}

	d.conn = conn
	d.status.Connection = device.ConnectionConnected
	return nil
}

func (d *WTNPXI) Disconnect() error {
	d.mu.Lock()
	defer d.mu.Unlock()

	_ = d.stopAcquisitionLocked()

	if d.conn != nil {
		_ = d.conn.Close()
		d.conn = nil
	}

	d.status.Connection = device.ConnectionDisconnected
	return nil
}

func (d *WTNPXI) StartAcquisition() error {
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

func (d *WTNPXI) StopAcquisition() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.stopAcquisitionLocked()
}

func (d *WTNPXI) stopAcquisitionLocked() error {
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

func (d *WTNPXI) SetDataSink(sink device.DataSink) {
	d.mu.Lock()
	d.sink = sink
	d.mu.Unlock()
}

func (d *WTNPXI) Status() device.Status {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.status
}

func (d *WTNPXI) SetUnit(unit string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	for i := range d.profile.Channels {
		d.profile.Channels[i].Unit = unit
	}
	return nil
}

func (d *WTNPXI) SetTare(channelIndex int, offset float64) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if channelIndex < 0 || channelIndex >= len(d.profile.Channels) {
		return fmt.Errorf("invalid channel index: %d", channelIndex)
	}
	d.profile.Channels[channelIndex].TareOffset = offset
	return nil
}

func (d *WTNPXI) GetTare(channelIndex int) (float64, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	if channelIndex < 0 || channelIndex >= len(d.profile.Channels) {
		return 0, fmt.Errorf("invalid channel index: %d", channelIndex)
	}
	return d.profile.Channels[channelIndex].TareOffset, nil
}

func (d *WTNPXI) ClearTare(channelIndex int) error {
	return d.SetTare(channelIndex, 0)
}

func (d *WTNPXI) readLoop(stop <-chan struct{}) {
	if d.conn == nil {
		return
	}

	buf := make([]byte, 8192)
	for {
		select {
		case <-stop:
			return
		default:
			d.conn.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
			n, err := d.conn.Read(buf)
			if err != nil {
				if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
					continue
				}
				slog.Debug("WTN_PXI read error", "device", d.profile.ID, "error", err)
				return
			}
			if n > 0 {
				d.processData(buf[:n])
			}
		}
	}
}

func (d *WTNPXI) processData(data []byte) {
	d.mu.Lock()
	d.recvBuffer = append(d.recvBuffer, data...)

	for len(d.recvBuffer) >= WTN_PXI_LENGTH_PREFIX_BYTES {
		payloadLen := int(d.recvBuffer[0])<<24 | int(d.recvBuffer[1])<<16 |
			int(d.recvBuffer[2])<<8 | int(d.recvBuffer[3])

		if payloadLen == 0 || payloadLen > WTN_PXI_MAX_PAYLOAD_BYTES {
			slog.Debug("WTN_PXI invalid payload length", "device", d.profile.ID, "length", payloadLen)
			d.recvBuffer = d.recvBuffer[1:]
			continue
		}

		frameLen := WTN_PXI_LENGTH_PREFIX_BYTES + payloadLen
		if len(d.recvBuffer) < frameLen {
			break
		}

		payload := make([]byte, payloadLen)
		copy(payload, d.recvBuffer[WTN_PXI_LENGTH_PREFIX_BYTES:frameLen])

		d.recvBuffer = d.recvBuffer[frameLen:]

		go d.handlePayload(payload)
	}
	d.mu.Unlock()
}

func (d *WTNPXI) handlePayload(payload []byte) {
	d.mu.RLock()
	acquiring := d.acquiring
	sink := d.sink
	channels := d.profile.Channels
	d.mu.RUnlock()

	if !acquiring || sink == nil {
		return
	}

	valueCount := len(payload) / 4
	if valueCount < WTN_PXI_REQUIRED_CHANNELS {
		slog.Debug("WTN_PXI insufficient values", "device", d.profile.ID, "count", valueCount)
		return
	}

	values := make([]float64, 0, len(channels))
	indices := make([]int, 0, len(channels))

	for _, ch := range channels {
		if !ch.Enabled {
			continue
		}
		if ch.Index >= 0 && ch.Index < valueCount {
			idx := ch.Index * 4
			val := math.Float32frombits(binary.LittleEndian.Uint32(payload[idx : idx+4]))
			indices = append(indices, ch.Index)
			values = append(values, float64(val))
		}
	}

	sink(device.DataPayload{
		DeviceID:       d.profile.ID,
		Timestamp:      device.NowMs(),
		Channels:       values,
		ChannelIndices: indices,
	})
}
