// daq_p1604_handler_test.go 验证 P1604 协议处理器：
//   - 帧字节布局与真实 protocol.ParseStreamFrameEx 解析兼容
//   - 采集启停（c 01 1 / c 02 1）正确驱动 emit goroutine
//   - 端到端：经 Simulator 连接后，用真实 FrameReader 读取并解析数据帧
//   - 内容掩码 c 05 解析（是否含设备时间戳）

package sim

import (
	"encoding/binary"
	"io"
	"math"
	"net"
	"sync/atomic"
	"testing"
	"time"

	sharedproto "shared.local/device-sdk/go/protocol"
)

// TestBuildP1604Frame_NoTimestamp 验证默认帧（无时间戳）：2 长度前缀 + 77 payload，
// 并用真实 ParseStreamFrameEx 解析得到 18 通道。
func TestBuildP1604Frame_NoTimestamp(t *testing.T) {
	frame := buildP1604Frame(1, false)
	if len(frame) != 2+p1604DefaultPayload {
		t.Fatalf("帧长度 = %d, want %d", len(frame), 2+p1604DefaultPayload)
	}
	// 长度前缀值 = 77+2 = 79
	if got := binary.BigEndian.Uint16(frame[:2]); got != uint16(p1604DefaultPayload+2) {
		t.Fatalf("长度前缀 = %d, want %d", got, p1604DefaultPayload+2)
	}
	// 头部 0x01 seq(2B) 0x00 0x00（0x01 使 IsASCIIFrame=false）
	if frame[2] != 0x01 {
		t.Fatalf("帧头首字节 = %02x, want 0x01", frame[2])
	}
	if sharedproto.IsASCIIFrame(frame[2:]) {
		t.Fatal("IsASCIIFrame 应为 false（二进制帧）")
	}

	// 用真实解析器验证通道
	channels, tsMs, err := sharedproto.ParseStreamFrameEx(frame[2:], false, true)
	if err != nil {
		t.Fatalf("ParseStreamFrameEx: %v", err)
	}
	if len(channels) != 18 {
		t.Fatalf("通道数 = %d, want 18", len(channels))
	}
	if tsMs != 0 {
		t.Fatalf("无时间戳时 tsMs 应为 0, got %d", tsMs)
	}
	for i, v := range channels {
		if math.IsNaN(v) || v == 0 {
			t.Fatalf("通道 %d 值异常: %v", i, v)
		}
	}
}

// TestBuildP1604Frame_WithTimestamp 验证含时间戳帧：85 payload，tsMs>0。
func TestBuildP1604Frame_WithTimestamp(t *testing.T) {
	frame := buildP1604Frame(1, true)
	wantPayload := p1604HeaderSize + p1604PressureBytes + p1604TimestampBytes + p1604Atmospheric*4 // 85
	if len(frame) != 2+wantPayload {
		t.Fatalf("帧长度 = %d, want %d", len(frame), 2+wantPayload)
	}

	channels, tsMs, err := sharedproto.ParseStreamFrameEx(frame[2:], true, true)
	if err != nil {
		t.Fatalf("ParseStreamFrameEx: %v", err)
	}
	if len(channels) != 18 {
		t.Fatalf("通道数 = %d, want 18", len(channels))
	}
	if tsMs <= 0 {
		t.Fatalf("设备时间戳未解析: tsMs=%d", tsMs)
	}
}

// TestDAQP1604Handler_StartStopAcquisition 验证 c 01 1 启动 emit、c 02 1 停止 emit。
// 用计数 emit 直接驱动 handler，不经过 Simulator，聚焦启停语义。
func TestDAQP1604Handler_StartStopAcquisition(t *testing.T) {
	h := NewDAQP1604Handler()
	var count atomic.Int32
	h.StartAcquisition(func(frame []byte) { count.Add(1) })

	// 未收到 c 01 1 前，不应 emit
	time.Sleep(80 * time.Millisecond)
	if got := count.Load(); got != 0 {
		t.Fatalf("start 命令前已 emit %d 帧", got)
	}

	// c 01 1 启动采集
	h.HandleCommand([]byte("c 01 1"))
	time.Sleep(180 * time.Millisecond) // ~3 帧 @ 50ms
	n := count.Load()
	if n < 2 {
		t.Fatalf("启动后 emit 帧数 %d, want >=2", n)
	}

	// c 02 1 停止采集；stopEmitting 同步等待 goroutine 退出
	h.HandleCommand([]byte("c 02 1"))
	after := count.Load()
	time.Sleep(150 * time.Millisecond)
	final := count.Load()
	if final != after {
		t.Fatalf("停止后仍 emit: after=%d final=%d", after, final)
	}
}

// TestDAQP1604Handler_ContentMask 验证 c 05 命令解析时间戳掩码位。
func TestDAQP1604Handler_ContentMask(t *testing.T) {
	h := NewDAQP1604Handler()

	// 0810：无时间戳
	h.HandleCommand([]byte("c 05 1 0810"))
	h.mu.Lock()
	useTs := h.useDeviceTs
	h.mu.Unlock()
	if useTs {
		t.Fatal("掩码 0810 不应启用时间戳")
	}

	// 0C10：含时间戳（0x0400）
	h.HandleCommand([]byte("c 05 1 0C10"))
	h.mu.Lock()
	useTs = h.useDeviceTs
	h.mu.Unlock()
	if !useTs {
		t.Fatal("掩码 0C10 应启用时间戳")
	}
}

// TestDAQP1604Handler_EndToEnd 端到端：经 Simulator 连接，发送真实 adapter 的
// initStream 命令序列，用真实 FrameReader 读取数据帧并解析验证。
func TestDAQP1604Handler_EndToEnd(t *testing.T) {
	s := NewSimulator("127.0.0.1:0", NewDAQP1604Handler())
	if err := s.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })

	conn, err := net.DialTimeout("tcp", s.Addr(), 2*time.Second)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	waitClientCount(t, s, 1, time.Second)

	// 发送 P1604 初始化命令序列（对齐真实 adapter initStream）
	for _, cmd := range []string{
		"w1601",
		"c 00 1 FFFF 1 100 7 0",
		"c 05 1 0810",
		"c 01 1",
	} {
		if _, err := conn.Write([]byte(cmd + "\r\n")); err != nil {
			t.Fatalf("Write %q: %v", cmd, err)
		}
	}

	// 用真实 FrameReader 读取数据帧
	fr := sharedproto.NewFrameReader(conn)
	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	payload, err := fr.ReadFrame()
	if err != nil {
		t.Fatalf("ReadFrame: %v", err)
	}
	// 默认帧 77 字节（5 头 + 64 压力 + 8 大气）
	if len(payload) != p1604DefaultPayload {
		t.Fatalf("payload 长度 = %d, want %d", len(payload), p1604DefaultPayload)
	}
	channels, _, err := sharedproto.ParseStreamFrameEx(payload, false, true)
	if err != nil {
		t.Fatalf("ParseStreamFrameEx: %v", err)
	}
	if len(channels) != 18 {
		t.Fatalf("通道数 = %d, want 18", len(channels))
	}

	// 停止采集：c 02 1 应被接受且不崩溃
	if _, err := conn.Write([]byte("c 02 1\r\n")); err != nil {
		t.Fatalf("Write stop: %v", err)
	}

	// 停止后主动 drain：读 conn 直到 200ms 无新帧，排空所有在途帧。
	// 容差放宽到 2：stopEmitting 同步等待 emit goroutine 退出，但 stop 前
	// emit 已推入 writeCh(缓冲 64) 的帧仍会被 writeLoop 写入 conn，故停止后
	// 可能读到少量在途帧；高负载 CI 上可能读到 2 帧，放宽容差防 flaky。
	const drainDeadline = 200 * time.Millisecond
	_ = conn.SetReadDeadline(time.Now().Add(drainDeadline))
	framesAfterStop := 0
	for {
		if _, err := fr.ReadFrame(); err != nil {
			break // 超时/EOF/错误：在途帧已排空，无更多帧
		}
		framesAfterStop++
	}
	_ = conn.SetReadDeadline(time.Time{}) // 清除 deadline，避免影响后续操作
	if framesAfterStop > 2 {
		t.Fatalf("停止后收到 %d 帧，期望至多 2 个在途帧", framesAfterStop)
	}
}

// TestDAQP1604Handler_InjectDirtyFrame 验证 InjectFrame 注入的错误帧能让客户端读到。
// 这是后续测试 adapter 错误帧容错的基础。
func TestDAQP1604Handler_InjectDirtyFrame(t *testing.T) {
	s := NewSimulator("127.0.0.1:0", NewDAQP1604Handler())
	if err := s.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })

	conn, err := net.DialTimeout("tcp", s.Addr(), 2*time.Second)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	waitClientCount(t, s, 1, time.Second)

	// 注入一个畸形长度前缀帧（声明 200 字节但只给 2 字节），
	// 真实 adapter 的 FrameReader 会因 ReadFull 失败而报错，从而触发 onError。
	dirty := []byte{0x00, 0xC8, 0xAA, 0x55} // 长度前缀 200，payload 不足
	s.InjectFrame(dirty)

	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	got := make([]byte, len(dirty))
	if _, err := io.ReadFull(conn, got); err != nil {
		t.Fatalf("Read 注入帧: %v", err)
	}
	if string(got) != string(dirty) {
		t.Fatalf("注入帧不符: got %v want %v", got, dirty)
	}
}
