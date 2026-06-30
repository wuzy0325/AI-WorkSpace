// daq_p1604_handler.go 实现 DAQ-P-1604 设备协议的模拟端。
// 作为第一个具体设备 handler，验证 Simulator 框架对真实协议可用。
// 帧格式对齐 shared/device-sdk/go/protocol/daq_p1604_frame.go 的解析逻辑，
// 命令序列对齐 projects/wind-daq/.../adapters/hardware/daq_p1604.go 真实 adapter。

package sim

import (
	"encoding/binary"
	"math"
	"strings"
	"sync"
	"time"
)

// DAQ-P-1604 协议常量，与 protocol/daq_p1604_frame.go 保持一致。
const (
	p1604HeaderSize     = 5                    // 5 字节帧头
	p1604NumPressure    = 16                   // 16 路压力通道
	p1604PressureBytes  = p1604NumPressure * 4 // 64 字节
	p1604TimestampBytes = 8                    // 设备时间戳 8 字节（uint32 秒 + uint32 纳秒小数）
	p1604Atmospheric    = 2                    // CH17 大气压 + CH18 大气温
	// 默认帧（无时间戳）：5 + 64 + 8 = 77 字节
	p1604DefaultPayload = p1604HeaderSize + p1604PressureBytes + p1604Atmospheric*4
	// 帧率 20Hz，对齐真实设备默认采样率（50ms/帧）
	p1604EmitInterval = 50 * time.Millisecond

	// 内容掩码位（对齐 adapter 的 contentMask 计算）
	p1604MaskDeviceTimestamp uint16 = 0x0400 // 设备时间戳
	p1604MaskAtmospheric     uint16 = 0x0800 // 大气数据
	p1604MaskStream          uint16 = 0x0010 // 流数据
)

// DAQP1604Handler 实现 DAQ-P-1604 设备协议的模拟端。
//
// 协议要点（对齐真实 adapter projects/wind-daq/.../hardware/daq_p1604.go）：
//   - 命令为 SCPI 文本，以 "\r\n" 结尾，由 Simulator 的 LineCommandReader 剥离行结束符后传入：
//     w1601                启用 2 字节大端长度前缀（本模拟器始终用长度前缀发帧，静默接受）
//     c 00 1 FFFF 1 100 7 0  配置流参数（通道掩码/采样率等，静默接受）
//     c 05 1 XXXX          配置内容掩码；解析 0x0400 时间戳位决定数据帧是否含 8 字节时间戳
//     c 01 1               开始采集 → 启动 emit goroutine 周期推送数据帧
//     c 02 1               停止采集 → 停止 emit goroutine
//   - 真实 adapter 的 sendCommand 只写不读（setup 命令不读响应），故 setup 命令返回 nil
//     不回写，避免响应帧污染后续 readLoop 的数据帧流。
//   - 数据帧线上格式：2 字节大端长度前缀(值=payloadLen+2) + payload。
//     payload = 5 字节头(0x01 seq(2B BE) 0x00 0x00) + 16×float32 压力(BE)
//   - 可选 8 字节时间戳 + 2×float32 大气(BE)。
//     头部 0x01 为非可打印字符，使 IsASCIIFrame=false，走二进制解析路径。
type DAQP1604Handler struct {
	mu          sync.Mutex
	emit        func(frame []byte) // 由 Simulator 在 StartAcquisition 注入的帧推送回调
	emitting    bool               // 是否正在 emit
	stopCh      chan struct{}      // 通知 emit goroutine 退出
	emitDone    chan struct{}      // emit goroutine 退出时关闭，供 stopEmitting 等待
	useDeviceTs bool               // 是否在数据帧中包含设备时间戳（c 05 掩码 0x0400）
	seq         uint32             // 帧序号，嵌入帧头便于排序验证
}

// NewDAQP1604Handler 构造 DAQ-P-1604 协议处理器。
func NewDAQP1604Handler() *DAQP1604Handler {
	return &DAQP1604Handler{}
}

// HandleCommand 解析 P1604 命令。
// setup 命令静默接受（返回 nil 不回写，因 adapter 不读响应）；
// c 01 1 / c 02 1 控制采集启停。
func (h *DAQP1604Handler) HandleCommand(cmd []byte) []byte {
	s := strings.TrimSpace(string(cmd))
	if s == "" {
		return nil
	}
	switch {
	case s == "w1601":
		// 启用长度前缀模式：本模拟器始终用长度前缀发帧，无需特殊处理。
		return nil
	case strings.HasPrefix(s, "c 00 1"):
		// 流参数配置（通道掩码、采样率等）：静默接受。
		return nil
	case strings.HasPrefix(s, "c 05 1"):
		// 内容掩码：解析 0x0400 时间戳位，决定数据帧是否含 8 字节时间戳。
		h.parseContentMask(s)
		return nil
	case s == "c 01 1":
		h.startEmitting()
		return nil
	case s == "c 02 1":
		h.stopEmitting()
		return nil
	default:
		return nil
	}
}

// parseContentMask 解析 "c 05 1 XXXX" 末尾的 4 位十六进制掩码，
// 提取 0x0400（设备时间戳）位。0x0010 流数据、0x0800 大气数据始终默认包含。
func (h *DAQP1604Handler) parseContentMask(cmd string) {
	fields := strings.Fields(cmd)
	if len(fields) < 3 {
		return
	}
	hexStr := fields[len(fields)-1]
	mask := parseHexUint16(hexStr)
	h.mu.Lock()
	h.useDeviceTs = mask&p1604MaskDeviceTimestamp != 0
	h.mu.Unlock()
}

// StartAcquisition 由 Simulator 在 Start() 时调用，注入 emit 回调。
// 不立即发帧；等收到 "c 01 1" 才启动 emit goroutine（P1604 协议语义）。
func (h *DAQP1604Handler) StartAcquisition(emit func(frame []byte)) {
	h.mu.Lock()
	h.emit = emit
	h.mu.Unlock()
}

// StopAcquisition 由 Simulator 在 Close() 时调用，停止 emit goroutine 并等待退出。
func (h *DAQP1604Handler) StopAcquisition() {
	h.stopEmitting()
}

// startEmitting 启动 emit goroutine 周期推送数据帧。
// 已在 emit 则幂等返回；emit 回调未注入则不启动（防御）。
func (h *DAQP1604Handler) startEmitting() {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.emitting || h.emit == nil {
		return
	}
	h.emitting = true
	h.stopCh = make(chan struct{})
	h.emitDone = make(chan struct{})
	stop := h.stopCh
	emit := h.emit
	go func() {
		defer close(h.emitDone)
		h.emitLoop(stop, emit)
	}()
}

// stopEmitting 停止 emit goroutine 并等待其退出，保证资源释放确定性。
func (h *DAQP1604Handler) stopEmitting() {
	h.mu.Lock()
	if !h.emitting {
		h.mu.Unlock()
		return
	}
	h.emitting = false
	close(h.stopCh)
	done := h.emitDone
	h.stopCh = nil
	h.mu.Unlock()
	if done != nil {
		<-done // 等待 emit goroutine 退出
	}
}

// emitLoop 按 p1604EmitInterval 间隔生成数据帧并经 emit 推送。
// 从 stop 通道退出，保证 Simulator.Close 时能清理 goroutine。
func (h *DAQP1604Handler) emitLoop(stop <-chan struct{}, emit func(frame []byte)) {
	ticker := time.NewTicker(p1604EmitInterval)
	defer ticker.Stop()
	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
			h.mu.Lock()
			seq := h.seq
			h.seq++
			useTs := h.useDeviceTs
			h.mu.Unlock()
			emit(buildP1604Frame(seq, useTs))
		}
	}
}

// buildP1604Frame 构造一帧完整的 P1604 线上字节：
//
//	[2 字节大端长度前缀] [payload]
//
// payload =
//
//	[5 字节头: 0x01 seq(2B BE) 0x00 0x00]
//	[16×float32 压力通道(BE)，设备顺序 CH16..CH1]
//	[可选 8 字节时间戳: uint32 秒 + uint32 纳秒小数]
//	[2×float32 大气(BE): CH17 大气压 + CH18 大气温]
//
// 长度前缀值 = len(payload)+2，对齐 FrameReader.ReadFrame（值-2=payload 长度）。
// 头部 0x01 为非可打印字符，使 IsASCIIFrame=false（走二进制解析路径）。
func buildP1604Frame(seq uint32, useDeviceTs bool) []byte {
	tsBytes := 0
	if useDeviceTs {
		tsBytes = p1604TimestampBytes
	}
	payloadSize := p1604HeaderSize + p1604PressureBytes + tsBytes + p1604Atmospheric*4
	payload := make([]byte, payloadSize)

	// 5 字节头：协议规定 byte0 固定为 0x01，byte1~2 为序号，byte3~4 保留。
	payload[0] = 0x01
	binary.BigEndian.PutUint16(payload[1:3], uint16(seq&0xFFFF))
	payload[3] = 0x00
	payload[4] = 0x00

	// 16 路压力通道（设备顺序 CH16..CH1），值随序号变化便于断言区分
	for i := 0; i < p1604NumPressure; i++ {
		// 基线大气压 + 通道偏移 + 正弦扰动，确保非零、可解析、帧间可区分
		v := float32(101325.0 + float64(i)*100.0 + math.Sin(float64(seq)*0.1+float64(i))*50.0)
		binary.BigEndian.PutUint32(payload[p1604HeaderSize+i*4:], math.Float32bits(v))
	}

	// 可选设备时间戳（uint32 秒 + uint32 纳秒小数），位于压力后、大气前
	off := p1604HeaderSize + p1604PressureBytes
	if useDeviceTs {
		now := time.Now()
		binary.BigEndian.PutUint32(payload[off:], uint32(now.Unix())) // 秒
		// 纳秒小数：纳秒值映射到 [0, 2^32) 区间。用 uint64 避免溢出。
		frac := uint64(now.Nanosecond()) * (uint64(1) << 32) / uint64(1000000000)
		binary.BigEndian.PutUint32(payload[off+4:], uint32(frac))
		off += p1604TimestampBytes
	}

	// 大气数据：CH17 大气压 + CH18 大气温
	binary.BigEndian.PutUint32(payload[off:], math.Float32bits(float32(101325.0)))
	binary.BigEndian.PutUint32(payload[off+4:], math.Float32bits(float32(25.0)))

	// 2 字节大端长度前缀（值 = payloadLen + 2）
	frame := make([]byte, 2+payloadSize)
	binary.BigEndian.PutUint16(frame, uint16(payloadSize+2))
	copy(frame[2:], payload)
	return frame
}

// parseHexUint16 解析 4 位十六进制字符串为 uint16。非法字符返回 0。
func parseHexUint16(s string) uint16 {
	var v uint16
	for _, r := range s {
		var d uint16
		switch {
		case r >= '0' && r <= '9':
			d = uint16(r - '0')
		case r >= 'a' && r <= 'f':
			d = uint16(r-'a') + 10
		case r >= 'A' && r <= 'F':
			d = uint16(r-'A') + 10
		default:
			return 0
		}
		v = v*16 + d
	}
	return v
}

// 编译期断言：DAQP1604Handler 实现 ProtocolHandler 接口。
// 若接口签名变更导致未实现，编译在此报错，及早暴露契约不一致。
var _ ProtocolHandler = (*DAQP1604Handler)(nil)
