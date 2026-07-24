package hardware

import (
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"log/slog"
	"math"
	"net"
	"sync"
	"time"

	"wind-daq/services/api-go/internal/core/device"
	"wind-daq/services/api-go/internal/ports"
)

const (
	// DAQ-P-1604Pre 默认连接参数（与 Cursor DAQ 实测一致）
	// 旧值 192.168.0.7:9001 是参考设备文档写的，实际设备是 192.168.3.232:23
	DAQ_P_1064PRE_DEFAULT_HOST = "192.168.3.232"
	DAQ_P_1064PRE_DEFAULT_PORT = 23
	DAQ_P_1064PRE_TIMEOUT      = 5 * time.Second
)

const (
	CMD_READ_STATUS       = 0x00
	CMD_READ_RANGE        = 0x01
	CMD_READ_CALIBRATION  = 0x03
	CMD_READ_EXT_TRIGGER  = 0x13
	CMD_ACQUISITION_CTRL  = 0x14 // 采集控制命令码（参考 Cursor DAQ 实测值，旧值 0x10 设备不识别）
	CMD_WRITE_RANGE       = 0x81
	CMD_WRITE_CALIBRATION = 0x83
	CMD_FACTORY_RESET     = 0x84
	CMD_WRITE_EXT_TRIGGER = 0x93
)

const (
	ACQ_ACTION_STOP       = 0x00  // 停止采集
	ACQ_ACTION_SINGLE     = 0x01  // 单次采集
	ACQ_ACTION_CONTINUOUS = 0xFF  // 连续采集（参考 Cursor DAQ 实测值，旧值 2 设备不识别）
	ACQ_DATA_MODE_RAW     = 0x11  // 原始 AD 数据
	ACQ_DATA_MODE_CALIB   = 0x13  // 校准后工程单位数据（参考 Cursor DAQ 实测值，旧值 1 设备不识别）
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
	// firstDataLogged 首次收到数据时打印 hex dump，用于诊断协议格式
	firstDataLogged  bool
	// onError 由 DeviceManager 在 Connect 阶段注入，readLoop 异常退出时回调，
	// 用于将设备异常（断网、读错误等）向上传播，由 DeviceManager 统一更新状态。
	onError          func(err error)
}

// 编译期断言：DAQP1064Pre 实现 ports.ErrorNotifiable
var _ ports.ErrorNotifiable = (*DAQP1064Pre)(nil)

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

// SetOnError 实现 ports.ErrorNotifiable：DeviceManager 在 Connect 阶段注入回调。
func (d *DAQP1064Pre) SetOnError(fn func(err error)) {
	d.mu.Lock()
	d.onError = fn
	d.mu.Unlock()
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
	port := d.profile.Port
	if port <= 0 {
		port = DAQ_P_1064PRE_DEFAULT_PORT
	}
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("%s:%d", host, port), DAQ_P_1064PRE_TIMEOUT)
	if err != nil {
		return fmt.Errorf("connect to %s:%d: %w", host, port, err)
	}
	slog.Info("DAQ-P-1064Pre TCP connected", "category", "hardware-recv", "component", "hardware", "device", d.profile.ID, "address", host, "port", port)

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
	slog.Info("DAQ-P-1064Pre TCP disconnected", "category", "hardware-recv", "component", "hardware", "device", d.profile.ID)

	d.status.Connection = device.ConnectionDisconnected
	return nil
}

func (d *DAQP1064Pre) StartAcquisition() error {
	slog.Info("1064Pre: StartAcquisition called", "device", d.profile.Name, "id", d.profile.ID)
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.acquiring {
		slog.Info("1064Pre: already acquiring, skip", "device", d.profile.Name)
		return nil
	}
	if d.conn == nil {
		slog.Error("1064Pre: StartAcquisition failed - not connected", "device", d.profile.Name)
		return fmt.Errorf("device not connected")
	}

	// 发送启动采集命令（CMD_ACQUISITION_CTRL = 0x10）
	// 设备收到此命令后开始推送 72B 采集数据帧，无需等待响应帧
	if err := d.sendStartAcquisitionLocked(); err != nil {
		return fmt.Errorf("start acquisition command failed: %w", err)
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

// sendStartAcquisitionLocked 发送启动采集命令（调用方已持有 d.mu 锁）
//
// 命令帧格式：A5 5A 0x10 len(2B BE) data(7B) checksum
// data 载荷按 1064Pre 协议规范构造：
//
//	data[0] = ACQ_ACTION_CONTINUOUS (2)  // 连续采集模式
//	data[1] = ACQ_DATA_MODE_CALIB (1)    // 工程单位（校准后数据）
//	data[2:4] = 采样周期（ms，LE uint16） // 1000 / SamplingRate(Hz)
//	data[4:6] = 通道使能（LE uint16）    // 0xFFFF = 全部通道
//	data[6] = 气象数据开关（1 = 包含）    // 含大气压/温度
func (d *DAQP1064Pre) sendStartAcquisitionLocked() error {
	samplingRate := d.profile.SamplingRate
	if samplingRate <= 0 {
		samplingRate = 10 // 默认 10Hz
	}
	periodMs := uint16(1000 / samplingRate)

	data := make([]byte, 7)
	data[0] = ACQ_ACTION_CONTINUOUS
	data[1] = ACQ_DATA_MODE_CALIB
	binary.LittleEndian.PutUint16(data[2:4], periodMs)
	binary.LittleEndian.PutUint16(data[4:6], 0xFFFF)
	data[6] = 1

	frame := d.buildFrame(CMD_ACQUISITION_CTRL, data)

	slog.Info("1064Pre: sending acquisition start frame",
		"device", d.profile.Name,
		"hex", hex.EncodeToString(frame),
		"samplingRate", samplingRate, "periodMs", periodMs)

	_ = d.conn.SetWriteDeadline(time.Now().Add(DAQ_P_1064PRE_TIMEOUT))
	defer d.conn.SetWriteDeadline(time.Time{})
	_, err := d.conn.Write(frame)
	if err != nil {
		slog.Error("1064Pre: send start acquisition command failed",
			"device", d.profile.Name, "error", err)
		return err
	}
	slog.Info("1064Pre: acquisition start command sent", "device", d.profile.Name)
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
			// 用户主动停止（StopAcquisition/Disconnect），不触发 onError
			return
		default:
			d.conn.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
			n, err := d.conn.Read(buf)
			if err != nil {
				if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
					continue
				}
				// 用户可能在 Read 阻塞期间发起 Stop：close(stop) 后 Read 才返回错误。
				// 这种情况下错误不是真正的设备异常，不应触发 onError。
				select {
				case <-stop:
					return
				default:
				}
				slog.Warn("DAQ-P-1064Pre read loop 异常退出", "device", d.profile.ID, "error", err)
				// 异常退出：清理本地采集状态并向上传播错误
				d.mu.Lock()
				// 仅在仍认为采集中的情况下做状态回退；避免与 StopAcquisition 竞争时重复操作。
				if d.acquiring {
					d.acquiring = false
					d.stop = nil
				}
				if d.conn != nil {
					_ = d.conn.Close()
					d.conn = nil
				}
				d.status.Acquiring = false
				// 读取失败通常意味着链路已断
				d.status.Connection = device.ConnectionDisconnected
				fn := d.onError
				d.mu.Unlock()
				if fn != nil {
					fn(err)
				}
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

	if !d.firstDataLogged && len(d.recvBuffer) > 0 {
		d.firstDataLogged = true
		dumpLen := len(d.recvBuffer)
		if dumpLen > 128 {
			dumpLen = 128
		}
		slog.Info("1064Pre: first data received after start",
			"device", d.profile.Name,
			"totalBytes", len(d.recvBuffer),
			"hex", hex.EncodeToString(d.recvBuffer[:dumpLen]))
	}

	for len(d.recvBuffer) >= 6 {
		if d.recvBuffer[0] != 0xA5 || d.recvBuffer[1] != 0x5A {
			if len(d.recvBuffer) >= 2 {
				slog.Debug("1064Pre: bad frame header, resyncing",
					"device", d.profile.Name, "buf0", d.recvBuffer[0], "buf1", d.recvBuffer[1],
					"buf[0:2] hex", hex.EncodeToString(d.recvBuffer[:2]))
			}
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
			slog.Debug("1064Pre: received valid data frame",
				"device", d.profile.Name, "dataLen", dataLen)
			d.handleAcquisitionDataLocked(payload)
		} else {
			if d.acquiring {
				slog.Debug("1064Pre: non-data frame ignored",
					"device", d.profile.Name, "cmd", cmd, "dataLen", dataLen, "expectedCmd", CMD_ACQUISITION_CTRL, "expectedLen", 72)
			}
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

// handleAcquisitionDataLocked 在调用方已持有 d.mu.Lock 的前提下运行。
// 不再内部 RLock，避免与父 processData 的 Lock 形成自死锁。
//
// 1604Pre 数据帧 payload 布局（72 字节）：
//
//	[0..3]  大气压（float32 LE，单位 Pa）
//	[4..7]  大气温度（float32 LE，单位 °C）
//	[8..71] 16 路压力（16×float32 LE，单位 Pa）
//
// 通道映射（与 defaultDAQP1604PreChannels 一致）：
//
//	Index 0..15 → payload[8+i*4]  压力通道
//	Index 16    → payload[0..3]   大气压
//	Index 17    → payload[4..7]   大气温度
//
// 兼容性：若 profile.Channels 仅 16 通道（历史 profile），不读气象数据，不影响功能。
func (d *DAQP1064Pre) handleAcquisitionDataLocked(payload []byte) {
	if len(payload) < 72 {
		return
	}

	// 调用方持锁，直接读取 sink/channels
	sink := d.sink
	channels := d.profile.Channels
	deviceID := d.profile.ID

	if sink == nil {
		return
	}

	values := make([]float64, 0, len(channels))
	indices := make([]int, 0, len(channels))

	for i := 0; i < len(channels); i++ {
		if !channels[i].Enabled {
			continue
		}
		var val float64
		switch channels[i].Index {
		case device.P1604PreAtmChannelIndex: // 大气压
			val = float64(readFloat32LE(payload, 0))
		case device.P1604PreAtmTempChannelIndex: // 大气温度
			val = float64(readFloat32LE(payload, 4))
		default: // 16 路压力通道（Index 0..15）
			if channels[i].Index >= 0 && channels[i].Index < device.P1604PrePressureChannelCount {
				val = float64(readFloat32LE(payload, 8+channels[i].Index*4))
			}
		}
		indices = append(indices, channels[i].Index)
		values = append(values, val)
	}

	d.streamFrameSeq++
	// sink 是 device.DataSink 函数，通常是非阻塞发送到 channel；
	// 但仍建议避免在持锁状态下长时间执行——目前实现仅做指针拷贝，可接受。
	sink(device.DataPayload{
		DeviceID:       deviceID,
		DeviceType:     d.profile.Type,
		DeviceName:     d.profile.Name,
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
	// 命令收发用 Info 级别：ring buffer 透传，stderr / file 由 CategorySkipHandler 跳过。
	slog.Info("DAQ-P-1064Pre command send", "category", "hardware-send", "component", "hardware", "device", d.profile.ID, "command", fmt.Sprintf("0x%02X", cmd), "bytes", len(frame))
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

	slog.Info("DAQ-P-1064Pre command response", "category", "hardware-recv", "component", "hardware", "device", d.profile.ID, "command", fmt.Sprintf("0x%02X", cmd), "bytes", dataLen)
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
