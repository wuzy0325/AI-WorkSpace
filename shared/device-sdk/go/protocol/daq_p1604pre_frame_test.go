package protocol

import (
	"encoding/binary"
	"math"
	"testing"
)

// TestBuildP1604PreFrame_FrameStructure 验证帧结构：A5 5A + cmd + len(BE) + data + checksum
//
// 测试前置：
//   - 准备一个固定 cmd 和 data 的输入
//
// 测试步骤：
//   - 调用 BuildP1604PreFrame 构建帧
//   - 检查帧头字节、cmd、len、data、checksum 各字段
//
// 期待结果：
//   - 帧头为 A5 5A
//   - cmd 字段等于输入 cmd
//   - len 字段为大端序，等于 data 长度
//   - data 部分等于输入 data
//   - checksum 字段等于从头到 data 末尾的累加和低 8 位
func TestBuildP1604PreFrame_FrameStructure(t *testing.T) {
	cmd := P1604PreCmdAcquisitionCtrl
	data := []byte{0xFF, 0x13, 0x64, 0x00, 0xFF, 0xFF, 0x01} // 启动采集 7B data
	frame := BuildP1604PreFrame(cmd, data)

	// 帧头：A5 5A
	if frame[0] != P1604PreHeader0 || frame[1] != P1604PreHeader1 {
		t.Fatalf("frame header = %02X %02X, want A5 5A", frame[0], frame[1])
	}
	// cmd
	if P1604PreCmd(frame[2]) != cmd {
		t.Fatalf("cmd = 0x%02X, want 0x%02X", frame[2], byte(cmd))
	}
	// len（大端）
	gotLen := int(binary.BigEndian.Uint16(frame[3:5]))
	if gotLen != len(data) {
		t.Fatalf("len = %d, want %d", gotLen, len(data))
	}
	// data
	dataStart := P1604PreFrameHeaderSize
	dataEnd := dataStart + len(data)
	for i, b := range data {
		if frame[dataStart+i] != b {
			t.Fatalf("data[%d] = 0x%02X, want 0x%02X", i, frame[dataStart+i], b)
		}
	}
	// checksum
	expectedChecksum := P1604PreCalculateChecksum(frame[:dataEnd])
	if frame[dataEnd] != expectedChecksum {
		t.Fatalf("checksum = 0x%02X, want 0x%02X", frame[dataEnd], expectedChecksum)
	}
}

// TestParseP1604PreFrame_RoundTrip 验证 BuildFrame → ParseFrame 往返一致
//
// 测试前置：
//   - 构建一个已知 cmd 和 data 的帧
//
// 测试步骤：
//   - 调用 BuildP1604PreFrame 构建帧
//   - 调用 ParseP1604PreFrame 解析帧
//
// 期待结果：
//   - 解析出的 cmd 等于构建时的 cmd
//   - 解析出的 data 等于构建时的 data
//   - consumed 等于帧总长度
func TestParseP1604PreFrame_RoundTrip(t *testing.T) {
	cmd := P1604PreCmdReadCalibration
	data := []byte{0x01, 0x02, 0x03, 0x04}
	frame := BuildP1604PreFrame(cmd, data)

	parsed, consumed, err := ParseP1604PreFrame(frame)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if parsed.Cmd != cmd {
		t.Fatalf("cmd = 0x%02X, want 0x%02X", byte(parsed.Cmd), byte(cmd))
	}
	if len(parsed.Data) != len(data) {
		t.Fatalf("data len = %d, want %d", len(parsed.Data), len(data))
	}
	for i, b := range data {
		if parsed.Data[i] != b {
			t.Fatalf("data[%d] = 0x%02X, want 0x%02X", i, parsed.Data[i], b)
		}
	}
	if consumed != len(frame) {
		t.Fatalf("consumed = %d, want %d", consumed, len(frame))
	}
}

// TestParseP1604PreFrame_Incomplete 验证数据不完整时返回 ErrP1604PreIncomplete
//
// 测试前置：
//   - 构建一个完整帧后截断尾部
//
// 测试步骤：
//   - 截断帧，使其长度小于完整帧
//   - 调用 ParseP1604PreFrame
//
// 期待结果：
//   - 返回 ErrP1604PreIncomplete
func TestParseP1604PreFrame_Incomplete(t *testing.T) {
	frame := BuildP1604PreFrame(P1604PreCmdReadStatus, []byte{0x01, 0x02})
	// 截断最后一字节
	truncated := frame[:len(frame)-1]
	_, _, err := ParseP1604PreFrame(truncated)
	if err != ErrP1604PreIncomplete {
		t.Fatalf("err = %v, want ErrP1604PreIncomplete", err)
	}
}

// TestParseP1604PreFrame_BadHeader 验证帧头错误时返回 ErrP1604PreBadHeader
func TestParseP1604PreFrame_BadHeader(t *testing.T) {
	buf := []byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}
	_, _, err := ParseP1604PreFrame(buf)
	if err != ErrP1604PreBadHeader {
		t.Fatalf("err = %v, want ErrP1604PreBadHeader", err)
	}
}

// TestParseP1604PreFrame_BadChecksum 验证校验和错误时返回 ErrP1604PreBadChecksum
//
// 测试前置：
//   - 构建一个完整帧后修改 checksum 字节
//
// 测试步骤：
//   - 修改帧最后一字节（checksum）
//   - 调用 ParseP1604PreFrame
//
// 期待结果：
//   - 返回 ErrP1604PreBadChecksum
func TestParseP1604PreFrame_BadChecksum(t *testing.T) {
	frame := BuildP1604PreFrame(P1604PreCmdReadStatus, []byte{0x01, 0x02})
	frame[len(frame)-1] ^= 0xFF // 翻转 checksum 位
	_, _, err := ParseP1604PreFrame(frame)
	if err != ErrP1604PreBadChecksum {
		t.Fatalf("err = %v, want ErrP1604PreBadChecksum", err)
	}
}

// TestParseP1604PreFrame_ResyncOnBadHeader 验证帧头错位时调用方可以 resync
//
// 测试场景：缓冲区前面有 1 字节垃圾，后面跟着完整帧。
// 调用方按"丢弃首字节重试"策略 resync，最终能解析出完整帧。
//
// 测试前置：
//   - 构建一个完整帧
//   - 在帧前面插入 1 字节垃圾
//
// 测试步骤：
//   - 第一次 ParseP1604PreFrame 返回 ErrP1604PreBadHeader
//   - 丢弃首字节后再次 ParseP1604PreFrame
//
// 期待结果：
//   - 第二次解析成功，consumed 等于帧长度
func TestParseP1604PreFrame_ResyncOnBadHeader(t *testing.T) {
	frame := BuildP1604PreFrame(P1604PreCmdReadStatus, []byte{0x01, 0x02})
	buf := append([]byte{0xCC}, frame...) // 前置 1 字节垃圾

	// 第一次解析应失败
	_, _, err := ParseP1604PreFrame(buf)
	if err != ErrP1604PreBadHeader {
		t.Fatalf("first parse err = %v, want ErrP1604PreBadHeader", err)
	}

	// 丢弃首字节后重试
	buf = buf[1:]
	parsed, consumed, err := ParseP1604PreFrame(buf)
	if err != nil {
		t.Fatalf("second parse failed: %v", err)
	}
	if parsed.Cmd != P1604PreCmdReadStatus {
		t.Fatalf("cmd = 0x%02X, want 0x%02X", byte(parsed.Cmd), byte(P1604PreCmdReadStatus))
	}
	if consumed != len(frame) {
		t.Fatalf("consumed = %d, want %d", consumed, len(frame))
	}
}

// TestParseP1604PreAcquisitionData_WithWeather 验证含气象数据的采集帧解析
//
// 测试前置：
//   - 构造 72 字节载荷：前 8 字节气象数据（大气压 + 大气温），后 64 字节 16 路压力
//   - 大气压 = 101325.0 Pa，大气温 = 25.5 °C
//   - 16 路压力 = 0.0, 1.0, 2.0, ..., 15.0
//
// 测试步骤：
//   - 调用 ParseP1604PreAcquisitionData 解析
//
// 期待结果：
//   - channels 长度为 18
//   - hasWeather 为 true
//   - channels[16] = 101325.0, channels[17] = 25.5
//   - channels[0..15] = 0.0, 1.0, ..., 15.0
func TestParseP1604PreAcquisitionData_WithWeather(t *testing.T) {
	data := make([]byte, 72)
	// 气象数据（小端 float32）
	binary.LittleEndian.PutUint32(data[0:4], math.Float32bits(101325.0))
	binary.LittleEndian.PutUint32(data[4:8], math.Float32bits(25.5))
	// 16 路压力
	for i := 0; i < 16; i++ {
		offset := 8 + i*4
		binary.LittleEndian.PutUint32(data[offset:offset+4], math.Float32bits(float32(i)))
	}

	channels, hasWeather, err := ParseP1604PreAcquisitionData(data)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if !hasWeather {
		t.Fatalf("hasWeather = false, want true")
	}
	if len(channels) != 18 {
		t.Fatalf("channels len = %d, want 18", len(channels))
	}
	if channels[16] != 101325.0 {
		t.Fatalf("channels[16] = %f, want 101325.0", channels[16])
	}
	if channels[17] != 25.5 {
		t.Fatalf("channels[17] = %f, want 25.5", channels[17])
	}
	for i := 0; i < 16; i++ {
		if channels[i] != float64(i) {
			t.Fatalf("channels[%d] = %f, want %f", i, channels[i], float64(i))
		}
	}
}

// TestParseP1604PreAcquisitionData_PressureOnly 验证不含气象数据的采集帧解析
//
// 测试前置：
//   - 构造 64 字节载荷（仅 16 路压力，无气象数据）
//
// 测试步骤：
//   - 调用 ParseP1604PreAcquisitionData 解析
//
// 期待结果：
//   - hasWeather 为 false
//   - channels[16] 和 channels[17] 为 0
//   - channels[0..15] 正确解析
func TestParseP1604PreAcquisitionData_PressureOnly(t *testing.T) {
	data := make([]byte, 64)
	for i := 0; i < 16; i++ {
		offset := i * 4
		binary.LittleEndian.PutUint32(data[offset:offset+4], math.Float32bits(float32(i)*10))
	}

	channels, hasWeather, err := ParseP1604PreAcquisitionData(data)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if hasWeather {
		t.Fatalf("hasWeather = true, want false")
	}
	if len(channels) != 18 {
		t.Fatalf("channels len = %d, want 18", len(channels))
	}
	if channels[16] != 0 || channels[17] != 0 {
		t.Fatalf("weather channels should be 0, got %f, %f", channels[16], channels[17])
	}
	for i := 0; i < 16; i++ {
		if channels[i] != float64(i)*10 {
			t.Fatalf("channels[%d] = %f, want %f", i, channels[i], float64(i)*10)
		}
	}
}

// TestParseP1604PreAcquisitionData_PayloadTooShort 验证载荷过短时返回错误
//
// 测试前置：
//   - 构造 60 字节载荷（小于 64 字节最小压力数据长度）
//
// 测试步骤：
//   - 调用 ParseP1604PreAcquisitionData
//
// 期待结果：
//   - 返回错误（包含 ErrP1604PrePayloadShort）
func TestParseP1604PreAcquisitionData_PayloadTooShort(t *testing.T) {
	data := make([]byte, 60)
	_, _, err := ParseP1604PreAcquisitionData(data)
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
}

// TestBuildP1604PreAcquisitionStart_DataLayout 验证启动采集命令的 7B data 布局
//
// 测试前置：
//   - 输入：samplingRateHz=10, channelEnable=0xFFFF, withWeatherData=true
//
// 测试步骤：
//   - 调用 BuildP1604PreAcquisitionStart
//
// 期待结果：
//   - data[0] = 0xFF (CONTINUOUS)
//   - data[1] = 0x13 (CALIBRATED)
//   - data[2..3] = sampleRateValue (LE) = 1000/10 = 100 = 0x0064
//   - data[4..5] = 0xFFFF (LE)
//   - data[6] = 0x01 (含气象数据)
func TestBuildP1604PreAcquisitionStart_DataLayout(t *testing.T) {
	txData := BuildP1604PreAcquisitionStart(10, 0xFFFF, true)
	if txData[0] != P1604PreActionContinuous {
		t.Fatalf("data[0] = 0x%02X, want 0xFF (CONTINUOUS)", txData[0])
	}
	if txData[1] != P1604PreDataModeCalibrated {
		t.Fatalf("data[1] = 0x%02X, want 0x13 (CALIBRATED)", txData[1])
	}
	sampleRateValue := binary.LittleEndian.Uint16(txData[2:4])
	if sampleRateValue != 100 {
		t.Fatalf("sampleRateValue = %d, want 100", sampleRateValue)
	}
	channelEnable := binary.LittleEndian.Uint16(txData[4:6])
	if channelEnable != 0xFFFF {
		t.Fatalf("channelEnable = 0x%04X, want 0xFFFF", channelEnable)
	}
	if txData[6] != 0x01 {
		t.Fatalf("data[6] = 0x%02X, want 0x01 (with weather)", txData[6])
	}
}

// TestBuildP1604PreAcquisitionStop_DataLayout 验证停止采集命令的 7B data 布局
//
// 测试前置：
//   - 无输入参数
//
// 测试步骤：
//   - 调用 BuildP1604PreAcquisitionStop
//
// 期待结果：
//   - data[0] = 0x00 (STOP)
//   - data[6] = 0x00 (无气象数据)
func TestBuildP1604PreAcquisitionStop_DataLayout(t *testing.T) {
	txData := BuildP1604PreAcquisitionStop()
	if txData[0] != P1604PreActionStop {
		t.Fatalf("data[0] = 0x%02X, want 0x00 (STOP)", txData[0])
	}
	if txData[6] != 0x00 {
		t.Fatalf("data[6] = 0x%02X, want 0x00 (no weather)", txData[6])
	}
}

// TestP1604PreCalculateChecksum 验证校验和计算：累加和取低 8 位
//
// 测试前置：
//   - 输入固定字节序列
//
// 测试步骤：
//   - 调用 P1604PreCalculateChecksum
//
// 期待结果：
//   - 返回累加和的低 8 位
func TestP1604PreCalculateChecksum(t *testing.T) {
	// A5 + 5A + 14 + 00 + 07 + FF + 13 + 64 + 00 + FF + FF + 01 = ?
	// 0xA5 + 0x5A = 0xFF
	// 0xFF + 0x14 = 0x113
	// 0x113 + 0x00 = 0x113
	// 0x113 + 0x07 = 0x11A
	// 0x11A + 0xFF = 0x219
	// 0x219 + 0x13 = 0x22C
	// 0x22C + 0x64 = 0x290
	// 0x290 + 0x00 = 0x290
	// 0x290 + 0xFF = 0x38F
	// 0x38F + 0xFF = 0x48E
	// 0x48E + 0x01 = 0x48F
	// 低 8 位 = 0x8F
	frame := []byte{0xA5, 0x5A, 0x14, 0x00, 0x07, 0xFF, 0x13, 0x64, 0x00, 0xFF, 0xFF, 0x01}
	got := P1604PreCalculateChecksum(frame)
	want := byte(0x8F)
	if got != want {
		t.Fatalf("checksum = 0x%02X, want 0x%02X", got, want)
	}
}
