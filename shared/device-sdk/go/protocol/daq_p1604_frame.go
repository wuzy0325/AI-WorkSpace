package protocol

import (
	"encoding/binary"
	"fmt"
	"math"
	"net"
	"sync"
)

const (
	// p1604LengthPrefixSize 是长度前缀的字节数。
	p1604LengthPrefixSize = 2
	// maxFramePayloadLen 是单帧 payload 的最大安全长度。
	// DAQ-P-1604 实际帧远小于此值，设置上限可防止异常长度前缀导致内存暴涨。
	maxFramePayloadLen = 4096
	// StreamFrameHeaderSize 是二进制流帧头部大小。
	StreamFrameHeaderSize = 5
)

// FrameReader 实现带缓冲的 2 字节大端长度前缀帧读取。
//
// 与旧版无缓冲实现相比，新增内部缓冲区可平滑处理 TCP 分段到达；
// 同时提供 Reset/Resync 接口，用于停止采集后清理残留数据、
// 以及连续帧错误时丢弃首字节尝试重新对齐。
type FrameReader struct {
	conn net.Conn
	mu   sync.Mutex
	buf  []byte
}

// NewFrameReader 创建一个新的帧读取器。
func NewFrameReader(conn net.Conn) *FrameReader {
	return &FrameReader{
		conn: conn,
		buf:  make([]byte, 0, maxFramePayloadLen),
	}
}

// ReadFrame 读取一帧完整数据（长度前缀 + payload），返回 payload。
// 调用方通常应在调用前设置 conn 的 read deadline，以控制等待超时。
func (r *FrameReader) ReadFrame() ([]byte, error) {
	r.mu.Lock()
	if len(r.buf) < p1604LengthPrefixSize {
		r.mu.Unlock()
		if err := r.fillAtLeast(p1604LengthPrefixSize); err != nil {
			return nil, err
		}
		r.mu.Lock()
	}

	frameLen := int(binary.BigEndian.Uint16(r.buf[:p1604LengthPrefixSize]))
	payloadLen := frameLen - p1604LengthPrefixSize
	if payloadLen <= 0 || payloadLen > maxFramePayloadLen {
		r.mu.Unlock()
		return nil, fmt.Errorf("invalid frame length: %d", frameLen)
	}

	total := p1604LengthPrefixSize + payloadLen
	if len(r.buf) < total {
		r.mu.Unlock()
		if err := r.fillAtLeast(total); err != nil {
			return nil, err
		}
		r.mu.Lock()
	}

	payload := make([]byte, payloadLen)
	copy(payload, r.buf[p1604LengthPrefixSize:total])
	r.buf = r.buf[total:]
	if len(r.buf) == 0 {
		r.buf = make([]byte, 0, maxFramePayloadLen)
	}
	r.mu.Unlock()
	return payload, nil
}

// fillAtLeast 不断从 conn 读取数据，直到缓冲区至少包含 need 字节或发生错误。
// 注意：某些 TCP 栈在边界情况下可能返回 (0, nil)，这里作为瞬态处理继续读取，
// 避免单次零字节读取终止整个采集循环。
func (r *FrameReader) fillAtLeast(need int) error {
	tmp := make([]byte, maxFramePayloadLen)
	for {
		r.mu.Lock()
		have := len(r.buf)
		r.mu.Unlock()
		if have >= need {
			return nil
		}
		n, err := r.conn.Read(tmp)
		if n > 0 {
			r.mu.Lock()
			r.buf = append(r.buf, tmp[:n]...)
			r.mu.Unlock()
		}
		if err != nil {
			return err
		}
		if n == 0 {
			continue
		}
	}
}

// Reset 清空内部缓冲区，用于停止采集后重置读取器状态，
// 避免残留数据干扰下一次采集的帧解析。
func (r *FrameReader) Reset() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.buf = make([]byte, 0, maxFramePayloadLen)
}

// Resync 丢弃缓冲区首字节以尝试重新对齐帧。
// 仅操作内部缓冲区；缓冲区为空时不执行 I/O，避免阻塞或引入额外时延。
func (r *FrameReader) Resync() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.buf) > 0 {
		r.buf = r.buf[1:]
		if len(r.buf) == 0 {
			r.buf = make([]byte, 0, maxFramePayloadLen)
		}
	}
}

// IsASCIIFrame checks if data is ASCII (first 64 bytes in 0x20-0x7E + \r\n)
func IsASCIIFrame(data []byte) bool {
	checkLen := len(data)
	if checkLen > 64 {
		checkLen = 64
	}
	for _, b := range data[:checkLen] {
		if b != 0x0d && b != 0x0a && (b < 0x20 || b > 0x7e) {
			return false
		}
	}
	return true
}

// ParseStreamFrame parses binary stream frame (legacy, no timestamp extraction).
// Deprecated: use ParseStreamFrameEx for frames that may include device timestamp.
// Frame: 5-byte header + 18 x float32 (big-endian) = 77 bytes
// Device order: CH16..CH1 (pressure) + CH17 (atm pressure) + CH18 (atm temp)
// Reverses first 16 pressure channels to CH1..CH16
func ParseStreamFrame(data []byte) ([]float64, error) {
	channels, _, err := ParseStreamFrameEx(data, false, true)
	return channels, err
}

// ParseStreamFrameEx 解析 DAQ-P-1604 二进制流帧，支持可选的设备时间戳字段（0x0400 掩码）。
//
// 帧格式（按内容掩码 c 05 决定）：
//
//	[5-byte header] [16 x float32 压力通道 CH16..CH1]
//
//	[可选 8-byte 设备时间戳：uint32 秒 + uint32 纳秒小数]
//	[可选 2 x float32 大气数据：CH17 大气压 + CH18 大气温]
//
// 参数：
//   - hasDeviceTimestamp: 是否启用了 0x0400 时间戳掩码
//   - hasAtmosphericData: 是否启用了 0x0800 大气数据掩码
//
// 返回：
//   - channels: CH1..CH16 压力 + 可选 CH17/CH18 大气数据，全部为 float64
//   - deviceTimestampMs: 设备时间戳（毫秒），仅当 hasDeviceTimestamp=true 且帧足够长时有效
func ParseStreamFrameEx(data []byte, hasDeviceTimestamp bool, hasAtmosphericData bool) (channels []float64, deviceTimestampMs int64, err error) {
	const headerSize = 5
	const numPressure = 16
	const pressureBytes = numPressure * 4
	const timestampBytes = 8
	const atmosphericChannels = 2

	// 计算期望的精确帧长度，用于检测长度前缀错位或帧截断。
	expectedLen := headerSize + pressureBytes
	if hasDeviceTimestamp {
		expectedLen += timestampBytes
	}
	if hasAtmosphericData {
		expectedLen += atmosphericChannels * 4
	}
	if len(data) != expectedLen {
		return nil, 0, fmt.Errorf("frame length mismatch: got %d, expected %d", len(data), expectedLen)
	}

	// 协议规定帧头第 0 字节固定为 0x01，作为二进制帧的同步标记。
	if data[0] != 0x01 {
		return nil, 0, fmt.Errorf("invalid stream frame header: expected 0x01, got 0x%02X", data[0])
	}

	// 解析 16 路压力通道（设备顺序 CH16..CH1）
	pressureValues := make([]float64, numPressure)
	for i := 0; i < numPressure; i++ {
		bits := binary.BigEndian.Uint32(data[headerSize+i*4:])
		v := math.Float32frombits(bits)
		if isInvalidFloat32(v) {
			return nil, 0, fmt.Errorf("pressure channel %d is invalid: %v", i, v)
		}
		pressureValues[i] = float64(v)
	}

	// 反转压力通道为 CH1..CH16
	for i := 0; i < numPressure/2; i++ {
		pressureValues[i], pressureValues[numPressure-1-i] = pressureValues[numPressure-1-i], pressureValues[i]
	}

	// 解析设备时间戳（8 字节：uint32 秒 + uint32 纳秒小数）
	// 位于压力通道之后、大气数据之前
	tsOffset := headerSize + pressureBytes
	if hasDeviceTimestamp {
		seconds := binary.BigEndian.Uint32(data[tsOffset:])
		fractional := binary.BigEndian.Uint32(data[tsOffset+4:])
		deviceTimestampMs = int64(seconds)*1000 + int64(math.Round(float64(fractional)/float64(0x100000000)*1000))
		tsOffset += timestampBytes
	}

	// 解析大气数据（CH17 大气压 + CH18 大气温）
	// 位于时间戳之后（如果有时间戳）或压力通道之后
	channels = pressureValues
	if hasAtmosphericData {
		atmoValues := make([]float64, atmosphericChannels)
		for i := 0; i < atmosphericChannels; i++ {
			bits := binary.BigEndian.Uint32(data[tsOffset+i*4:])
			v := math.Float32frombits(bits)
			if isInvalidFloat32(v) {
				return nil, 0, fmt.Errorf("atmospheric channel %d is invalid: %v", i, v)
			}
			atmoValues[i] = float64(v)
		}
		channels = append(channels, atmoValues...)
	}

	return channels, deviceTimestampMs, nil
}

// isInvalidFloat32 判断 float32 是否为 NaN 或 Inf。
func isInvalidFloat32(v float32) bool {
	return math.IsNaN(float64(v)) || math.IsInf(float64(v), 0)
}
