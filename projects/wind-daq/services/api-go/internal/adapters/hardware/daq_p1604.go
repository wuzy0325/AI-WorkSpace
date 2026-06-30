package hardware

import (
	"fmt"
	"log/slog"
	"net"
	"sync"
	"time"

	sharedproto "shared.local/device-sdk/go/protocol"
	"wind-daq/services/api-go/internal/core/device"
	"wind-daq/services/api-go/internal/ports"
)

const (
	DAQ_P_1604_DEFAULT_HOST = "192.168.3.101"
	DAQ_P_1604_DEFAULT_PORT = 9000
	DAQ_P_1604_TIMEOUT      = 5 * time.Second
)

type DAQP1604 struct {
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
	onError     func(err error) // 设备异常退出通知回调
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
		recvBuffer: make([]byte, 0, 4096),
	}
}

// 编译时接口检查
var _ ports.Device = (*DAQP1604)(nil)
var _ ports.TareConfigurable = (*DAQP1604)(nil)
var _ ports.ErrorNotifiable = (*DAQP1604)(nil)

// SetOnError 设置设备异常退出回调，实现 ports.ErrorNotifiable 接口
func (d *DAQP1604) SetOnError(fn func(err error)) {
	d.mu.Lock()
	d.onError = fn
	d.mu.Unlock()
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
	d.status.Connection = device.ConnectionConnected
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

	d.status.Connection = device.ConnectionDisconnected
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

func (d *DAQP1604) initStream() error {
	if err := d.sendCommand("w1601"); err != nil {
		return fmt.Errorf("enable length prefix: %w", err)
	}
	time.Sleep(50 * time.Millisecond)

	if err := d.sendCommand("c 00 1 FFFF 1 100 7 0"); err != nil {
		return fmt.Errorf("set stream params: %w", err)
	}
	time.Sleep(50 * time.Millisecond)

	// 配置流返回内容：0010=压力，0400=设备时间戳，0800=大气数据
	// 掩码计算：压力(0010) + 可选时间戳(0400) + 大气数据(0800)
	contentMask := 0x0010
	if d.profile.DaqP1604UseDeviceTimestamp {
		contentMask |= 0x0400
	}
	contentMask |= 0x0800 // 始终包含大气数据
	contentMaskHex := fmt.Sprintf("%04X", contentMask)
	if err := d.sendCommand(fmt.Sprintf("c 05 1 %s", contentMaskHex)); err != nil {
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
	d.conn.SetWriteDeadline(time.Time{}) // 清除写截止时间，避免影响后续操作
	return err
}

func (d *DAQP1604) readLoop(stop <-chan struct{}) {
	var unexpectedErr error

	defer func() {
		// 异常退出时更新状态并通知上层
		if unexpectedErr != nil {
			d.mu.Lock()
			d.acquiring = false
			d.stop = nil
			d.status.Acquiring = false
			d.status.LastError = unexpectedErr.Error()
			if d.status.Connection == device.ConnectionAcquiring {
				d.status.Connection = device.ConnectionConnected
			}
			fn := d.onError
			d.mu.Unlock()

			slog.Warn("DAQ-P-1604 read loop exited unexpectedly", "device", d.profile.ID, "error", unexpectedErr)
			if fn != nil {
				fn(unexpectedErr)
			}
		}
	}()

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
				unexpectedErr = err
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
	useDeviceTs := d.profile.DaqP1604UseDeviceTimestamp
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
		payload := device.DataPayload{
			DeviceID:       d.profile.ID,
			Timestamp:      device.NowMs(),
			Channels:       values,
			ChannelIndices: indices,
		}
		sink(payload)
		return
	}

	// 使用扩展解析函数，支持可选设备时间戳字段
	// DAQ-P-1604 始终请求大气数据（0800），所以 hasAtmosphericData = true
	channels, deviceTimestampMs, err := sharedproto.ParseStreamFrameEx(data, useDeviceTs, true)
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

	payload := device.DataPayload{
		DeviceID:       d.profile.ID,
		Timestamp:      device.NowMs(),
		Channels:       values,
		ChannelIndices: indices,
	}
	// 如果开启了设备时间戳且解析到有效时间戳，填入
	if useDeviceTs && deviceTimestampMs > 0 {
		payload.DeviceTimestamp = deviceTimestampMs
	}

	sink(payload)
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
