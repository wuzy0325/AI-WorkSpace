package sim

import (
	"encoding/binary"
	"fmt"
	"math"
	"strings"
)

// producers.go 为每种设备实现 FrameProducer。
//
// 每个 producer 生成一帧完整的线上字节（含长度前缀、校验和等设备协议封装），
// 供 TCPSimulator.sendLoop 直接写入 TCP 连接，由真实 adapter 的帧解析器读取。
// 字段顺序与字节序严格对齐 shared/device-sdk/go/protocol 的解析器定义，
// 确保生成的帧能被真实 adapter 正确解析。
//
// 各设备默认通道数（与协议/adapter 约定）：
//   - DAQ-P-1604: 18（16 压力 + 大气压 + 大气温度，见 ParseStreamFrame）
//   - DAQ-T-1603: 16（16 路热电偶，见 TCPFrameSize=64）
//   - DSA3217:   16（扫描数据行 16 个值）
//   - DAQ-P-1604Pre: 18（16 压力 + 大气压 + 大气温度，见 P1604PreFrameProducer）
//   - WTN_PXI:   8（WTN_PXI_REQUIRED_CHANNELS）

const (
	p1604DefaultChannels    = 18
	t1603DefaultChannels    = 16
	dsa3217DefaultChannels  = 16
	p1604preDefaultChannels = 18
	wtnpxiDefaultChannels   = 8

	// p1604StreamHeader 是 P1604 二进制帧的 5 字节头。
	// 包含 0xAA（>0x7E）确保 IsASCIIFrame 返回 false，走二进制解析路径。
	p1604StreamHeaderSize = 5

	// p1604preCmdAcquisition 是 DAQ-P-1604Pre 采集数据命令码，
	// 参考 Cursor DAQ 实测值 0x14（旧值 0x10 设备不识别）
	p1604preCmdAcquisition = 0x14
	// p1604preAcqDataLen 是采集帧 data 段长度：8 字节头 + 16×float32 = 72 字节。
	p1604preAcqDataLen = 72
)

// DefaultChannelsForType 返回设备类型的默认通道数。
func DefaultChannelsForType(devType string) int {
	switch devType {
	case "DAQ-P-1604":
		return p1604DefaultChannels
	case "DAQ-T-1603":
		return t1603DefaultChannels
	case "DSA3217":
		return dsa3217DefaultChannels
	case "DAQ-P-1604Pre":
		return p1604preDefaultChannels
	case "WTN_PXI":
		return wtnpxiDefaultChannels
	default:
		return 16
	}
}

// P1604BinaryFrameProducer 生成 DAQ-P-1604 二进制流帧。
// 线上格式：2 字节大端长度前缀 + 77 字节 payload（5 字节头 + 18×float32 BE）。
// adapter 的 FrameReader.ReadFrame 按长度前缀读 payload，ParseStreamFrame 解析。
func P1604BinaryFrameProducer(seq int, channels int) ([]byte, error) {
	return p1604BinaryFrame(seq, channels, false)
}

// P1604BinaryFrameProducerWithDeviceTimestamp 生成带 8 字节设备时间戳的 P1604 二进制帧。
func P1604BinaryFrameProducerWithDeviceTimestamp(seq int, channels int) ([]byte, error) {
	return p1604BinaryFrame(seq, channels, true)
}

func p1604BinaryFrame(seq int, channels int, withDeviceTimestamp bool) ([]byte, error) {
	if channels <= 0 {
		channels = p1604DefaultChannels
	}
	atmosphericChannels := 0
	if channels > 16 {
		atmosphericChannels = channels - 16
	}
	payloadLen := p1604StreamHeaderSize + 16*4 + atmosphericChannels*4
	if withDeviceTimestamp {
		payloadLen += 8
	}
	payload := make([]byte, payloadLen)
	// 5 字节头：协议规定 byte0 固定为 0x01，byte1~2 为序号，byte3~4 保留。
	// 0x01 是非可打印字符，IsASCIIFrame 返回 false，走二进制解析路径。
	payload[0] = 0x01
	binary.BigEndian.PutUint16(payload[1:3], uint16(seq&0xFFFF))
	payload[3] = 0x00
	payload[4] = 0x00
	for i := 0; i < 16; i++ {
		// 生成合理压力值（约 101325 Pa 标准大气压）+ 序号扰动，便于断言非零
		v := float32(101325.0 + float64(i)*100.0 + math.Sin(float64(seq)*0.1+float64(i))*50.0)
		binary.BigEndian.PutUint32(payload[p1604StreamHeaderSize+i*4:], math.Float32bits(v))
	}
	offset := p1604StreamHeaderSize + 16*4
	if withDeviceTimestamp {
		seconds := uint32(1700000000 + seq)
		fractional := uint32(0x80000000)
		binary.BigEndian.PutUint32(payload[offset:], seconds)
		binary.BigEndian.PutUint32(payload[offset+4:], fractional)
		offset += 8
	}
	for i := 0; i < atmosphericChannels; i++ {
		v := float32(101325.0 + float64(i)*100.0 + math.Sin(float64(seq)*0.1+float64(16+i))*50.0)
		binary.BigEndian.PutUint32(payload[offset+i*4:], math.Float32bits(v))
	}
	return withBigEndianLengthPrefix(payload), nil
}

// P1604ASCIIFrameProducer 生成 DAQ-P-1604 ASCII 流帧。
// 线上格式：2 字节大端长度前缀 + ASCII payload（逗号分隔 + \r\n）。
// adapter 的 IsASCIIFrame 返回 true 时走 parseASCIIFrame 路径。
func P1604ASCIIFrameProducer(seq int, channels int) ([]byte, error) {
	if channels <= 0 {
		channels = p1604DefaultChannels
	}
	var sb strings.Builder
	for i := 0; i < channels; i++ {
		if i > 0 {
			sb.WriteByte(',')
		}
		v := 101325.0 + float64(i)*100.0 + math.Sin(float64(seq)*0.1+float64(i))*50.0
		sb.WriteString(fmt.Sprintf("%.3f", v))
	}
	sb.WriteString("\r\n")
	payload := []byte(sb.String())
	return withBigEndianLengthPrefix(payload), nil
}

// T1603BinaryFrameProducer 生成 DAQ-T-1603 二进制帧。
// 线上格式：64 字节 payload（16×float32 LE），无长度前缀。
// 设备发送顺序为 CH15→CH0，adapter 的 ParseTCPFrame 会反转为 CH0→CH15。
// 生成值须在热电偶合理范围（-200~1350°C）以通过 looksLikeReasonableTemperatureFrame 校验。
func T1603BinaryFrameProducer(seq int, channels int) ([]byte, error) {
	if channels <= 0 {
		channels = t1603DefaultChannels
	}
	frame := make([]byte, channels*4)
	for i := 0; i < channels; i++ {
		// 25~33°C 合理室温范围
		v := float32(25.0 + float64(i)*0.5 + math.Sin(float64(seq)*0.05+float64(i))*0.1)
		binary.LittleEndian.PutUint32(frame[i*4:], math.Float32bits(v))
	}
	return frame, nil
}

// DSA3217FrameProducer 生成 DSA3217 扫描数据行。
// 线上格式：ASCII 文本（空格分隔 16 个 float + '\n'）。
// adapter 的 readLoop 用 ReadString('\n') 读行，parseDataLine 按空格分割解析。
func DSA3217FrameProducer(seq int, channels int) ([]byte, error) {
	if channels <= 0 {
		channels = dsa3217DefaultChannels
	}
	var sb strings.Builder
	for i := 0; i < channels; i++ {
		if i > 0 {
			sb.WriteByte(' ')
		}
		v := 100.0 + float64(i)*10.0 + math.Sin(float64(seq)*0.1+float64(i))*5.0
		sb.WriteString(fmt.Sprintf("%.3f", v))
	}
	sb.WriteByte('\n')
	return []byte(sb.String()), nil
}

// P1604PreFrameProducer 生成 DAQ-P-1604Pre 采集帧。
// 线上格式：0xA5 0x5A 头 + cmd(0x14) + len(2字节大端) + 72字节data + 1字节累加和校验。
//
// data 段布局（72 字节，与实测设备一致）：
//
//	[0..3]  大气压（float32 LE，单位 Pa，模拟 ~101325）
//	[4..7]  大气温度（float32 LE，单位 °C，模拟 ~25）
//	[8..71] 16 路压力（16×float32 LE，单位 Pa）
//
// adapter handleAcquisitionDataLocked 按此布局解析并分发到对应通道。
func P1604PreFrameProducer(seq int, channels int) ([]byte, error) {
	if channels <= 0 {
		channels = p1604preDefaultChannels
	}
	data := make([]byte, p1604preAcqDataLen)
	// 前 8 字节为气象数据（大气压 + 大气温度）
	binary.LittleEndian.PutUint32(data[0:4], math.Float32bits(float32(101325.0+math.Sin(float64(seq)*0.05)*50.0)))
	binary.LittleEndian.PutUint32(data[4:8], math.Float32bits(float32(25.0+math.Sin(float64(seq)*0.03)*2.0)))
	// data[8..71] 为 16 路压力数据
	for i := 0; i < channels && i < 16; i++ {
		v := float32(100.0 + float64(i)*10.0 + math.Sin(float64(seq)*0.1+float64(i))*5.0)
		binary.LittleEndian.PutUint32(data[8+i*4:], math.Float32bits(v))
	}

	// 组帧：头(2) + cmd(1) + len(2) + data + checksum(1)
	frame := make([]byte, 0, 6+len(data))
	frame = append(frame, 0xA5, 0x5A, p1604preCmdAcquisition)
	frame = append(frame, byte(len(data)>>8), byte(len(data)&0xFF))
	frame = append(frame, data...)
	// 校验和 = 头到 data 末尾的累加和（低 8 位）
	var sum byte
	for _, b := range frame {
		sum += b
	}
	frame = append(frame, sum)
	return frame, nil
}

// WTNPXIFrameProducer 生成 WTN_PXI 流帧。
// 线上格式：4 字节大端长度前缀 + N×float32 LE payload。
// adapter 的 processData 按长度前缀读 payload，handlePayload 读 float32 LE。
func WTNPXIFrameProducer(seq int, channels int) ([]byte, error) {
	if channels <= 0 {
		channels = wtnpxiDefaultChannels
	}
	payload := make([]byte, channels*4)
	for i := 0; i < channels; i++ {
		v := float32(100.0 + float64(i)*10.0 + math.Sin(float64(seq)*0.1+float64(i))*5.0)
		binary.LittleEndian.PutUint32(payload[i*4:], math.Float32bits(v))
	}
	prefix := make([]byte, 4)
	binary.BigEndian.PutUint32(prefix, uint32(len(payload)))
	return append(prefix, payload...), nil
}

// withBigEndianLengthPrefix 给 payload 加 2 字节大端长度前缀。
// 长度值 = len(payload) + 2，对齐 FrameReader.ReadFrame 的解析（长度值 - 2 = payload 长度）。
func withBigEndianLengthPrefix(payload []byte) []byte {
	out := make([]byte, 2+len(payload))
	binary.BigEndian.PutUint16(out, uint16(len(payload)+2))
	copy(out[2:], payload)
	return out
}
