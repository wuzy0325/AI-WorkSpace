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
	DAQ_P_1064PRE_DEFAULT_HOST = "192.168.1.100"
	DAQ_P_1064PRE_DEFAULT_PORT = 9001
	DAQ_P_1064PRE_TIMEOUT      = 5 * time.Second
)

const (
	CMD_READ_STATUS       = 0x00
	CMD_READ_RANGE        = 0x01
	CMD_READ_CALIBRATION  = 0x03
	CMD_WRITE_RANGE       = 0x81
	CMD_WRITE_CALIBRATION = 0x83
	CMD_FACTORY_RESET     = 0x84
	CMD_READ_EXT_TRIGGER  = 0x13
	CMD_WRITE_EXT_TRIGGER = 0x93
	CMD_ACQUISITION_CTRL  = 0x10
)

const (
	ACQ_ACTION_STOP       = 0
	ACQ_ACTION_SINGLE     = 1
	ACQ_ACTION_CONTINUOUS = 2
	ACQ_DATA_MODE_RAW     = 0
	ACQ_DATA_MODE_CALIB   = 1
)

type CalibrationParams struct {
	Channel int
	B       float32
	K1      float32
}

type DeviceStatus1064Pre struct {
	EEPROMStatus uint16
	ADStatus     uint16
}

type DAQP1064Pre struct {
	mu               sync.RWMutex
	profile          device.Profile
	status           device.Status
	sink             device.DataSink
	stop             chan struct{}
	acquiring        bool
	conn             net.Conn
	streamFrameSeq   uint32
	pendingResponses []pendingResponse
	recvBuffer       []byte
}

type pendingResponse struct {
	cmd   byte
	data  chan []byte
	timer *time.Timer
}

func NewDAQP1064Pre(profile device.Profile) *DAQP1064Pre {
	return &DAQP1064Pre{
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

func (d *DAQP1064Pre) ID() string { return d.profile.ID }

func (d *DAQP1064Pre) Connect() error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.conn != nil {
		return nil
	}

	host := d.profile.Address
	if host == "" {
		host = DAQ_P_1064PRE_DEFAULT_HOST
	}
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("%s:%d", host, DAQ_P_1064PRE_DEFAULT_PORT), DAQ_P_1064PRE_TIMEOUT)
	if err != nil {
		return fmt.Errorf("connect to %s:%d: %w", host, DAQ_P_1064PRE_DEFAULT_PORT, err)
	}

	d.conn = conn
	d.status.Connection = device.ConnectionConnected
	return nil
}

func (d *DAQP1064Pre) Disconnect() error {
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

func (d *DAQP1064Pre) StartAcquisition() error {
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

func (d *DAQP1064Pre) StopAcquisition() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.stopAcquisitionLocked()
}

func (d *DAQP1064Pre) stopAcquisitionLocked() error {
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

func (d *DAQP1064Pre) SetDataSink(sink device.DataSink) {
	d.mu.Lock()
	d.sink = sink
	d.mu.Unlock()
}

func (d *DAQP1064Pre) Status() device.Status {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.status
}

func (d *DAQP1064Pre) SetUnit(unit string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	for i := range d.profile.Channels {
		d.profile.Channels[i].Unit = unit
	}
	return nil
}

func (d *DAQP1064Pre) SetTare(channelIndex int, offset float64) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if channelIndex < 0 || channelIndex >= len(d.profile.Channels) {
		return fmt.Errorf("invalid channel index: %d", channelIndex)
	}
	d.profile.Channels[channelIndex].TareOffset = offset
	return nil
}

func (d *DAQP1064Pre) GetTare(channelIndex int) (float64, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	if channelIndex < 0 || channelIndex >= len(d.profile.Channels) {
		return 0, fmt.Errorf("invalid channel index: %d", channelIndex)
	}
	return d.profile.Channels[channelIndex].TareOffset, nil
}

func (d *DAQP1064Pre) ClearTare(channelIndex int) error {
	return d.SetTare(channelIndex, 0)
}

func (d *DAQP1064Pre) readLoop(stop <-chan struct{}) {
	if d.conn == nil {
		return
	}
	buf := make([]byte, 4096)
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
				slog.Debug("DAQ-P-1064Pre read error", "device", d.profile.ID, "error", err)
				return
			}
			if n > 0 {
				d.processData(buf[:n])
			}
		}
	}
}

func (d *DAQP1064Pre) processData(data []byte) {
	d.mu.Lock()
	d.recvBuffer = append(d.recvBuffer, data...)

	for len(d.recvBuffer) >= 6 {
		if d.recvBuffer[0] != 0xA5 || d.recvBuffer[1] != 0x5A {
			d.recvBuffer = d.recvBuffer[1:]
			continue
		}

		cmd := d.recvBuffer[2]
		dataLen := int(d.recvBuffer[3])<<8 | int(d.recvBuffer[4])
		frameLen := 2 + 1 + 2 + dataLen + 1

		if len(d.recvBuffer) < frameLen {
			break
		}

		payload := make([]byte, dataLen)
		copy(payload, d.recvBuffer[5:5+dataLen])

		expectedChecksum := d.recvBuffer[5+dataLen]
		checksum := d.calculateChecksum(d.recvBuffer[:5+dataLen])
		if checksum != expectedChecksum {
			slog.Debug("DAQ-P-1064Pre checksum mismatch", "device", d.profile.ID)
		}

		if cmd == CMD_ACQUISITION_CTRL && d.acquiring && dataLen == 72 {
			d.handleAcquisitionData(payload)
		}

		d.recvBuffer = d.recvBuffer[frameLen:]
	}
	d.mu.Unlock()
}

func (d *DAQP1064Pre) calculateChecksum(data []byte) byte {
	var sum byte
	for _, b := range data {
		sum += b
	}
	return sum
}

func (d *DAQP1064Pre) handleAcquisitionData(payload []byte) {
	if len(payload) < 72 {
		return
	}

	d.mu.RLock()
	sink := d.sink
	channels := d.profile.Channels
	d.mu.RUnlock()

	if sink == nil {
		return
	}

	values := make([]float64, 0, len(channels))
	indices := make([]int, 0, len(channels))

	for i := 0; i < 16 && i < len(channels); i++ {
		if !channels[i].Enabled {
			continue
		}
		val := float64(readFloat32LE(payload, 8+i*4))
		indices = append(indices, i)
		values = append(values, val)
	}

	d.streamFrameSeq++
	sink(device.DataPayload{
		DeviceID:       d.profile.ID,
		Timestamp:      device.NowMs(),
		Channels:       values,
		ChannelIndices: indices,
	})
}

func readFloat32LE(data []byte, offset int) float32 {
	if offset+4 > len(data) {
		return 0
	}
	return math.Float32frombits(binary.LittleEndian.Uint32(data[offset : offset+4]))
}

func (d *DAQP1064Pre) ReadCalibration(channel int) (*CalibrationParams, error) {
	if channel < 0 || channel > 15 {
		return nil, fmt.Errorf("invalid channel: %d", channel)
	}

	resp, err := d.sendCommand(CMD_READ_CALIBRATION, []byte{byte(channel)}, 2000)
	if err != nil {
		return nil, err
	}
	if len(resp) < 10 || resp[0] == 0xFF {
		return nil, fmt.Errorf("read calibration failed")
	}

	return &CalibrationParams{
		Channel: int(resp[1]),
		B:       readFloat32LE(resp, 2),
		K1:      readFloat32LE(resp, 6),
	}, nil
}

func (d *DAQP1064Pre) WriteCalibration(channel int, b, k1 float32) error {
	if channel < 0 || channel > 15 {
		return fmt.Errorf("invalid channel: %d", channel)
	}

	data := make([]byte, 29)
	data[0] = byte(channel)
	writeFloat32LE(data[1:5], b)
	writeFloat32LE(data[5:9], k1)

	resp, err := d.sendCommand(CMD_WRITE_CALIBRATION, data, 2000)
	if err != nil {
		return err
	}
	if len(resp) < 2 || resp[0] != 0x00 {
		return fmt.Errorf("write calibration failed")
	}
	return nil
}

func (d *DAQP1064Pre) TareChannel(channel int, currentValue float64) error {
	return d.WriteCalibration(channel, float32(currentValue), 1.0)
}

func (d *DAQP1064Pre) sendCommand(cmd byte, data []byte, timeoutMs int) ([]byte, error) {
	d.mu.Lock()
	conn := d.conn
	d.mu.Unlock()

	if conn == nil {
		return nil, fmt.Errorf("not connected")
	}

	frame := d.buildFrame(cmd, data)
	if _, err := conn.Write(frame); err != nil {
		return nil, err
	}

	conn.SetReadDeadline(time.Now().Add(time.Duration(timeoutMs) * time.Millisecond))

	header := make([]byte, 6)
	_, err := conn.Read(header)
	if err != nil {
		return nil, err
	}

	if header[0] != 0xA5 || header[1] != 0x5A {
		return nil, fmt.Errorf("invalid response header")
	}

	dataLen := int(header[3])<<8 | int(header[4])
	if dataLen > 4096-6 {
		return nil, fmt.Errorf("response too long")
	}

	respData := make([]byte, dataLen)
	read := 0
	for read < dataLen {
		n, err := conn.Read(respData[read:])
		if err != nil {
			return nil, err
		}
		read += n
	}

	return respData, nil
}

func (d *DAQP1064Pre) buildFrame(cmd byte, data []byte) []byte {
	frame := make([]byte, 0, 6+len(data))
	frame = append(frame, 0xA5, 0x5A, cmd)
	frame = append(frame, byte(len(data)>>8), byte(len(data)&0xFF))
	frame = append(frame, data...)
	frame = append(frame, d.calculateChecksum(frame))
	return frame
}

func writeFloat32LE(data []byte, v float32) {
	binary.LittleEndian.PutUint32(data, math.Float32bits(v))
}

func ptrFloat32(v float32) *float32 {
	return &v
}
