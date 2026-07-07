// Package protocol 提供设备底层协议原语（帧解析、命令收发、单位系数等）。
// 本文件实现 DAQ-P-1604Pre 的二进制帧协议：
//   - 帧头 A5 5A + cmd(1B) + len(2B,BE) + data(NB) + checksum(1B)
//   - 校验和：从 A5 到数据最后一个字节的累加和取低 8 位
//   - 数据载荷中的多字节字段为小端序（Intel/x86 字节序）
//
// 与 daq_p1604_frame.go 中的 1604 ASCII 协议完全不同，1604Pre 是独立型号，
// 协议参考 Cursor DAQ 的 1064Pre 实现（同一厂商同一协议族）。
package protocol

import (
	"encoding/binary"
	"fmt"
	"math"
)

// P1604Pre 帧头固定字节
const (
	P1604PreHeader0 = 0xA5
	P1604PreHeader1 = 0x5A
	// P1604PreFrameHeaderSize 帧头固定长度：A5 5A + cmd(1) + len(2) = 5B
	P1604PreFrameHeaderSize = 5
	// P1604PreChecksumSize 帧尾校验和长度
	P1604PreChecksumSize = 1
	// P1604PreMaxPayloadLen 单帧 payload 最大长度，防止异常长度字段导致内存暴涨
	// 正常帧 < 100B（采集数据帧 72B），256B 足够覆盖所有合法命令/响应，
	// 异常设备发送超大 len 字段时直接报错而非分配大缓冲。
	P1604PreMaxPayloadLen = 256
)

// P1604PreCmd 1604Pre 命令码
//
// 命令码对应帧结构中的 cmd 字段（第 3 字节）。
// 设备返回的响应帧与下发命令使用相同 cmd 码，便于命令-响应配对。
type P1604PreCmd byte

const (
	// P1604PreCmdReadStatus 读取设备状态（16 模块 EEPROM + AD 状态）
	P1604PreCmdReadStatus P1604PreCmd = 0x00
	// P1604PreCmdReadRange 读取单通道量程
	P1604PreCmdReadRange P1604PreCmd = 0x01
	// P1604PreCmdReadCalibration 读取单通道校准参数（b 偏移 + K1 系数）
	P1604PreCmdReadCalibration P1604PreCmd = 0x03
	// P1604PreCmdReadExtTrigger 读取外部触发参数
	P1604PreCmdReadExtTrigger P1604PreCmd = 0x13
	// P1604PreCmdAcquisitionCtrl 采集控制（启动/停止/单次）
	// 启动命令不返回确认帧，设备直接开始推送数据帧；
	// 停止命令返回 1 字节确认帧（0x00=成功）。
	P1604PreCmdAcquisitionCtrl P1604PreCmd = 0x14
	// P1604PreCmdWriteRange 写入单通道量程
	P1604PreCmdWriteRange P1604PreCmd = 0x81
	// P1604PreCmdWriteCalibration 写入单通道校准参数
	P1604PreCmdWriteCalibration P1604PreCmd = 0x83
	// P1604PreCmdFactoryReset 恢复出厂设置
	P1604PreCmdFactoryReset P1604PreCmd = 0x84
	// P1604PreCmdWriteExtTrigger 写入外部触发参数
	P1604PreCmdWriteExtTrigger P1604PreCmd = 0x93
)

// P1604Pre 采集动作（ACQUISITION_CTRL 命令 data[0]）
const (
	// P1604PreActionStop 停止采集
	P1604PreActionStop byte = 0x00
	// P1604PreActionSingle 单次采集
	P1604PreActionSingle byte = 0x01
	// P1604PreActionContinuous 连续采集（最常用）
	P1604PreActionContinuous byte = 0xFF
)

// P1604Pre 数据模式（ACQUISITION_CTRL 命令 data[1]）
const (
	// P1604PreDataModeRaw 原始 AD 数据
	P1604PreDataModeRaw byte = 0x11
	// P1604PreDataModeCalibrated 校准后数据（工程单位，最常用）
	P1604PreDataModeCalibrated byte = 0x13
)

// P1604Pre 采集数据帧载荷布局（含气象数据时共 72 字节 = 18 × 4）：
//
//	[0..3]   大气压力 (float32 LE) → CH17
//	[4..7]   大气温度 (float32 LE) → CH18
//	[8..71]  CH0~CH15 压力值 (各 float32 LE) → CH1~CH16
const (
	// P1604PreAcquisitionDataLen 采集数据帧载荷长度（含气象数据）
	P1604PreAcquisitionDataLen = 72
	// P1604PrePressureChannelCount 压力通道数
	P1604PrePressureChannelCount = 16
	// P1604PreWeatherChannelCount 气象通道数（大气压 + 温度）
	P1604PreWeatherChannelCount = 2
	// P1604PreWeatherOffset 气象数据在载荷中的偏移
	P1604PreWeatherOffset = 0
	// P1604PreWeatherBytes 气象数据字节数（2 × float32）
	P1604PreWeatherBytes = 8
	// P1604PrePressureOffset 压力数据在载荷中的偏移
	P1604PrePressureOffset = 8
)

// P1604PreAcquisitionTxData 启动/停止采集命令的 7 字节 data 载荷
//
// 启动采集（CONTINUOUS）：
//
//	data[0] = 0xFF (CONTINUOUS)
//	data[1] = 0x13 (CALIBRATED)
//	data[2..3] = sampleRateValue (LE, 1000/目标频率)
//	data[4..5] = channelEnable (LE, 0xFFFF = 全部 16 通道)
//	data[6] = 0x01 (含气象数据)
//
// 停止采集：data[0] = 0x00 (STOP)，其余字段同上但 data[6] = 0x00
type P1604PreAcquisitionTxData [7]byte

// BuildP1604PreAcquisitionStart 构建启动采集命令的 data 载荷
//
// 参数：
//   - samplingRateHz: 目标采样率（Hz），实际下发的 sampleRateValue = 1000/samplingRateHz
//   - channelEnable: 通道使能位掩码（bit i = 1 表示启用通道 i）
//   - withWeatherData: 是否包含气象数据（大气压 + 温度）
func BuildP1604PreAcquisitionStart(samplingRateHz int, channelEnable uint16, withWeatherData bool) P1604PreAcquisitionTxData {
	if samplingRateHz <= 0 {
		samplingRateHz = 10
	}
	sampleRateValue := uint16(1000 / samplingRateHz)
	if sampleRateValue < 1 {
		sampleRateValue = 1
	}
	var d P1604PreAcquisitionTxData
	d[0] = P1604PreActionContinuous
	d[1] = P1604PreDataModeCalibrated
	binary.LittleEndian.PutUint16(d[2:4], sampleRateValue)
	binary.LittleEndian.PutUint16(d[4:6], channelEnable)
	if withWeatherData {
		d[6] = 0x01
	} else {
		d[6] = 0x00
	}
	return d
}

// BuildP1604PreAcquisitionStop 构建停止采集命令的 data 载荷
func BuildP1604PreAcquisitionStop() P1604PreAcquisitionTxData {
	var d P1604PreAcquisitionTxData
	d[0] = P1604PreActionStop
	d[1] = P1604PreDataModeCalibrated
	binary.LittleEndian.PutUint16(d[2:4], 2) // 停止时 sampleRateValue 无意义，按 Cursor DAQ 实测填 2
	binary.LittleEndian.PutUint16(d[4:6], 0xFFFF)
	d[6] = 0x00
	return d
}

// P1604PreCalculateChecksum 计算校验和：从 A5 到数据最后一个字节的累加和取低 8 位
//
// 入参 frameWithoutChecksum 为不含校验和的完整帧（含 A5 5A 头、cmd、len、data）。
func P1604PreCalculateChecksum(frameWithoutChecksum []byte) byte {
	var sum uint32
	for _, b := range frameWithoutChecksum {
		sum += uint32(b)
	}
	return byte(sum & 0xFF)
}

// BuildP1604PreFrame 构建 1604Pre 协议帧
//
// 帧格式：A5 5A + cmd(1B) + len(2B,BE) + data(NB) + checksum(1B)
// 多字节字段（len）为大端序；data 内字段由调用方按小端序填充。
//
// 返回的 slice 是独立分配的副本，调用方可安全修改 data 不影响帧。
func BuildP1604PreFrame(cmd P1604PreCmd, data []byte) []byte {
	header := []byte{P1604PreHeader0, P1604PreHeader1, byte(cmd), 0x00, 0x00}
	binary.BigEndian.PutUint16(header[3:5], uint16(len(data)))
	withoutChecksum := append(header, data...)
	checksum := P1604PreCalculateChecksum(withoutChecksum)
	return append(withoutChecksum, checksum)
}

// P1604PreFrame 表示解析出的 1604Pre 帧
type P1604PreFrame struct {
	Cmd  P1604PreCmd
	Data []byte // 不含校验和的 payload
}

// ParseP1604PreFrame 尝试从缓冲区解析一个完整 1604Pre 帧
//
// 返回值：
//   - frame: 解析出的帧（Data 为独立副本，调用方可修改 buf 不影响 frame.Data）
//   - consumed: 本帧在 buf 中占用的字节数（含校验和），调用方应从 buf 前移 consumed 字节
//   - err: 数据不完整返回 ErrP1604PreIncomplete；帧头错误返回 ErrP1604PreBadHeader；
//     校验和错误返回 ErrP1604PreBadChecksum
//
// 帧头不匹配时，本函数不会盲目消费字节，而是返回 BadHeader 让调用方决定如何 resync。
// 调用方通常的 resync 策略：丢弃 buf 首字节后重试，直到找到 A5 5A 头或缓冲区空。
func ParseP1604PreFrame(buf []byte) (frame P1604PreFrame, consumed int, err error) {
	if len(buf) < P1604PreFrameHeaderSize+P1604PreChecksumSize {
		return P1604PreFrame{}, 0, ErrP1604PreIncomplete
	}
	if buf[0] != P1604PreHeader0 || buf[1] != P1604PreHeader1 {
		return P1604PreFrame{}, 0, ErrP1604PreBadHeader
	}
	cmd := P1604PreCmd(buf[2])
	dataLen := int(binary.BigEndian.Uint16(buf[3:5]))
	if dataLen > P1604PreMaxPayloadLen {
		return P1604PreFrame{}, 0, ErrP1604PreBadLength
	}
	totalLen := P1604PreFrameHeaderSize + dataLen + P1604PreChecksumSize
	if len(buf) < totalLen {
		return P1604PreFrame{}, 0, ErrP1604PreIncomplete
	}
	withoutChecksum := buf[:P1604PreFrameHeaderSize+dataLen]
	expectedChecksum := buf[P1604PreFrameHeaderSize+dataLen]
	actualChecksum := P1604PreCalculateChecksum(withoutChecksum)
	if expectedChecksum != actualChecksum {
		return P1604PreFrame{}, 0, ErrP1604PreBadChecksum
	}
	data := make([]byte, dataLen)
	copy(data, buf[P1604PreFrameHeaderSize:P1604PreFrameHeaderSize+dataLen])
	return P1604PreFrame{Cmd: cmd, Data: data}, totalLen, nil
}

// 1604Pre 帧解析错误
var (
	ErrP1604PreIncomplete   = fmt.Errorf("p1604pre frame incomplete")
	ErrP1604PreBadHeader    = fmt.Errorf("p1604pre frame bad header")
	ErrP1604PreBadChecksum  = fmt.Errorf("p1604pre frame bad checksum")
	ErrP1604PreBadLength    = fmt.Errorf("p1604pre frame bad length")
	ErrP1604PrePayloadShort = fmt.Errorf("p1604pre payload too short")
)

// ParseP1604PreAcquisitionData 解析 1604Pre 采集数据帧载荷
//
// 载荷布局（含气象数据时 72 字节，小端序）：
//
//	[0..3]   大气压力 (float32 LE) → CH17
//	[4..7]   大气温度 (float32 LE) → CH18
//	[8..71]  CH0~CH15 压力值 (各 float32 LE) → CH1~CH16
//
// 不含气象数据时载荷为 64 字节，CH0~CH15 直接从 [0..63] 开始（无气象前缀）。
// 设备按启动命令的 data[6] 决定是否返回气象数据：0x01 含气象，0x00 不含。
// 当前 adapter 始终发送 data[6]=0x01，实际帧均为 72B 含气象格式。
//
// 返回值：
//   - channels: 18 通道值，顺序为 CH1..CH16 + CH17(大气压) + CH18(大气温)
//     缺失的气象通道填 0
//   - hasWeather: 是否包含气象数据
func ParseP1604PreAcquisitionData(data []byte) (channels []float64, hasWeather bool, err error) {
	pressureBytes := P1604PrePressureChannelCount * 4
	if len(data) < pressureBytes {
		return nil, false, fmt.Errorf("%w: got %d, want >= %d", ErrP1604PrePayloadShort, len(data), pressureBytes)
	}
	hasWeather = len(data) >= pressureBytes+P1604PreWeatherBytes

	channels = make([]float64, P1604PrePressureChannelCount+P1604PreWeatherChannelCount)
	// 气象数据（位于载荷最前）
	if hasWeather {
		atmPressure := math.Float32frombits(binary.LittleEndian.Uint32(data[0:4]))
		atmTemp := math.Float32frombits(binary.LittleEndian.Uint32(data[4:8]))
		if isInvalidFloat32(atmPressure) || isInvalidFloat32(atmTemp) {
			return nil, false, fmt.Errorf("p1604pre weather channel invalid: atm=%v temp=%v", atmPressure, atmTemp)
		}
		channels[16] = float64(atmPressure)
		channels[17] = float64(atmTemp)
	}
	// 16 路压力：含气象时从 offset 8 开始，不含气象时从 offset 0 开始
	pressureOffset := 0
	if hasWeather {
		pressureOffset = P1604PrePressureOffset
	}
	for i := 0; i < P1604PrePressureChannelCount; i++ {
		offset := pressureOffset + i*4
		v := math.Float32frombits(binary.LittleEndian.Uint32(data[offset : offset+4]))
		if isInvalidFloat32(v) {
			return nil, false, fmt.Errorf("p1604pre pressure channel %d invalid: %v", i, v)
		}
		channels[i] = float64(v)
	}
	return channels, hasWeather, nil
}
