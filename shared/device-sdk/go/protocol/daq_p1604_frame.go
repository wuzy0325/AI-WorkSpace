package protocol

import (
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"net"
)

// -- DAQ-P-1604 frame parsing --

// FrameReader implements 2-byte big-endian length-prefix framing
type FrameReader struct {
	conn net.Conn
}

func NewFrameReader(conn net.Conn) *FrameReader {
	return &FrameReader{conn: conn}
}

// ReadFrame reads one frame (2-byte big-endian length prefix + payload)
func (r *FrameReader) ReadFrame() ([]byte, error) {
	var lenBuf [2]byte
	if _, err := io.ReadFull(r.conn, lenBuf[:]); err != nil {
		return nil, err
	}
	frameLen := int(binary.BigEndian.Uint16(lenBuf[:])) - 2
	if frameLen <= 0 {
		return nil, fmt.Errorf("invalid frame length: %d", frameLen+2)
	}

	payload := make([]byte, frameLen)
	if _, err := io.ReadFull(r.conn, payload); err != nil {
		return nil, err
	}
	return payload, nil
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

// ParseStreamFrameEx 解析二进制流帧，支持可选的设备时间戳字段（0x0400 掩码）。
//
// 帧格式（按内容掩码 c 05 决定）：
//
//	[5-byte header] [16 x float32 压力通道 CH16..CH1]
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

	// 计算期望的最小帧长度
	minLen := headerSize + pressureBytes
	if hasDeviceTimestamp {
		minLen += timestampBytes
	}
	if hasAtmosphericData {
		minLen += atmosphericChannels * 4
	}

	if len(data) < minLen {
		return nil, 0, fmt.Errorf("frame too short: %d, expected at least %d", len(data), minLen)
	}

	// 解析 16 路压力通道（设备顺序 CH16..CH1）
	pressureValues := make([]float64, numPressure)
	for i := 0; i < numPressure; i++ {
		bits := binary.BigEndian.Uint32(data[headerSize+i*4:])
		pressureValues[i] = float64(math.Float32frombits(bits))
	}

	// 反转压力通道为 CH1..CH16
	for i := 0; i < numPressure/2; i++ {
		pressureValues[i], pressureValues[numPressure-1-i] = pressureValues[numPressure-1-i], pressureValues[i]
	}

	// 解析设备时间戳（8 字节：uint32 秒 + uint32 纳秒小数）
	// 位于压力通道之后、大气数据之前
	tsOffset := headerSize + pressureBytes
	if hasDeviceTimestamp && len(data) >= tsOffset+timestampBytes {
		seconds := binary.BigEndian.Uint32(data[tsOffset:])
		fractional := binary.BigEndian.Uint32(data[tsOffset+4:])
		deviceTimestampMs = int64(seconds)*1000 + int64(math.Round(float64(fractional)/float64(0x100000000)*1000))
		tsOffset += timestampBytes
	}

	// 解析大气数据（CH17 大气压 + CH18 大气温）
	// 位于时间戳之后（如果有时间戳）或压力通道之后
	channels = pressureValues
	if hasAtmosphericData && len(data) >= tsOffset+atmosphericChannels*4 {
		atmoValues := make([]float64, atmosphericChannels)
		for i := 0; i < atmosphericChannels; i++ {
			bits := binary.BigEndian.Uint32(data[tsOffset+i*4:])
			atmoValues[i] = float64(math.Float32frombits(bits))
		}
		channels = append(channels, atmoValues...)
	}

	return channels, deviceTimestampMs, nil
}
