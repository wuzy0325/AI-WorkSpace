// simulator_test.go 验证模拟器框架本身的行为（与具体设备协议无关）。
// 使用 echoHandler 原样回显命令，聚焦测试 TCP 收发、故障注入与多客户端。

package sim

import (
	"bytes"
	"io"
	"net"
	"sync/atomic"
	"testing"
	"time"
)

// echoHandler 是测试用的回显协议处理器：原样返回收到的命令，不做采集发帧。
// 用于验证 Simulator 的命令收发、故障注入、多客户端等框架行为，与具体设备协议解耦。
type echoHandler struct {
	emitInjected atomic.Bool // 仅记录 emit 回调是否被注入，不实际发帧
}

// HandleCommand 原样回显命令（去掉 Simulator 已剥离的行结束符后回写）。
func (h *echoHandler) HandleCommand(cmd []byte) []byte { return cmd }

// StartAcquisition 记录 emit 已注入。echo 场景不发帧，避免干扰命令回显测试。
func (h *echoHandler) StartAcquisition(emit func(frame []byte)) {
	h.emitInjected.Store(true)
}

// StopAcquisition 清理，echo 场景无 goroutine 需停止。
func (h *echoHandler) StopAcquisition() {}

// startEchoSim 启动一个 echo 模拟器，返回模拟器；测试结束自动 Close。
func startEchoSim(t *testing.T) *Simulator {
	t.Helper()
	s := NewSimulator("127.0.0.1:0", &echoHandler{})
	if err := s.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

// dialSim 拨号连接模拟器并返回连接；测试结束自动 Close。
func dialSim(t *testing.T, addr string) net.Conn {
	t.Helper()
	conn, err := net.DialTimeout("tcp", addr, 2*time.Second)
	if err != nil {
		t.Fatalf("Dial %s: %v", addr, err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}

// waitClientCount 轮询直到 ClientCount 达到 want，或超时失败。
// 用于同步"Dial 成功但模拟器尚未 Accept 注册 client"的竞态窗口。
func waitClientCount(t *testing.T, s *Simulator, want int, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if s.ClientCount() >= want {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatalf("ClientCount 达到 %d 超时（当前 %d）", want, s.ClientCount())
}

// sendRecv 发送一条命令并读取回显响应。
func sendRecv(t *testing.T, conn net.Conn, cmd string) []byte {
	t.Helper()
	if _, err := conn.Write([]byte(cmd + "\r\n")); err != nil {
		t.Fatalf("Write %q: %v", cmd, err)
	}
	buf := make([]byte, len(cmd))
	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	if _, err := io.ReadFull(conn, buf); err != nil {
		t.Fatalf("Read 回显: %v", err)
	}
	return buf
}

// TestSimulator_EchoCommand 验证连接后发命令能收到原样回显。
func TestSimulator_EchoCommand(t *testing.T) {
	s := startEchoSim(t)
	conn := dialSim(t, s.Addr())
	waitClientCount(t, s, 1, time.Second)

	resp := sendRecv(t, conn, "HELLO")
	if string(resp) != "HELLO" {
		t.Fatalf("回显不符: got %q want %q", resp, "HELLO")
	}
}

// TestSimulator_AddrAfterStart 验证端口 0 分配后 Addr() 返回真实地址。
func TestSimulator_AddrAfterStart(t *testing.T) {
	s := NewSimulator("127.0.0.1:0", &echoHandler{})
	if err := s.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })

	addr := s.Addr()
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		t.Fatalf("SplitHostPort %q: %v", addr, err)
	}
	if host != "127.0.0.1" {
		t.Fatalf("host = %q, want 127.0.0.1", host)
	}
	if port == "0" {
		t.Fatal("端口仍为 0，未由系统分配")
	}
}

// TestSimulator_SetLatency 验证设置延迟后响应延迟符合预期。
func TestSimulator_SetLatency(t *testing.T) {
	s := startEchoSim(t)
	conn := dialSim(t, s.Addr())
	waitClientCount(t, s, 1, time.Second)

	// 基线：无延迟
	start := time.Now()
	resp := sendRecv(t, conn, "A")
	baseElapsed := time.Since(start)
	if string(resp) != "A" {
		t.Fatalf("基线回显不符: got %q", resp)
	}

	// 注入 120ms 延迟
	s.SetLatency(120 * time.Millisecond)
	defer s.SetLatency(0)

	start = time.Now()
	resp = sendRecv(t, conn, "B")
	slowElapsed := time.Since(start)
	if string(resp) != "B" {
		t.Fatalf("延迟回显不符: got %q", resp)
	}
	if slowElapsed < 120*time.Millisecond {
		t.Fatalf("延迟未生效: slow=%v base=%v, want slow>=120ms", slowElapsed, baseElapsed)
	}
}

// TestSimulator_DropNext 验证 DropNext(2) 后发 2 个命令无响应，第 3 个有响应。
func TestSimulator_DropNext(t *testing.T) {
	s := startEchoSim(t)
	conn := dialSim(t, s.Addr())
	waitClientCount(t, s, 1, time.Second)

	// 丢弃接下来 2 个命令的响应
	s.DropNext(2)

	// 发送 2 个命令：不应有响应（读取应超时）
	for i := 0; i < 2; i++ {
		if _, err := conn.Write([]byte("X\r\n")); err != nil {
			t.Fatalf("Write X: %v", err)
		}
	}
	_ = conn.SetReadDeadline(time.Now().Add(150 * time.Millisecond))
	tmp := make([]byte, 1)
	if _, err := conn.Read(tmp); err == nil {
		t.Fatal("DropNext 期间不应有响应，但读到了数据")
	}

	// 第 3 个命令：应有响应
	resp := sendRecv(t, conn, "Y")
	if string(resp) != "Y" {
		t.Fatalf("第 3 个命令回显不符: got %q want Y", resp)
	}
}

// TestSimulator_InjectFrame 验证 InjectFrame 后客户端读到注入的帧。
func TestSimulator_InjectFrame(t *testing.T) {
	s := startEchoSim(t)
	conn := dialSim(t, s.Addr())
	waitClientCount(t, s, 1, time.Second)

	injected := []byte("INJECTED-FRAME\r\n")
	s.InjectFrame(injected)

	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	got := make([]byte, len(injected))
	if _, err := io.ReadFull(conn, got); err != nil {
		t.Fatalf("Read 注入帧: %v", err)
	}
	if !bytes.Equal(got, injected) {
		t.Fatalf("注入帧不符: got %q want %q", got, injected)
	}
}

// TestSimulator_DisconnectAll 验证断开后客户端 Read 返回错误，但模拟器仍可接受新连接。
func TestSimulator_DisconnectAll(t *testing.T) {
	s := startEchoSim(t)
	conn := dialSim(t, s.Addr())
	waitClientCount(t, s, 1, time.Second)

	s.DisconnectAll()

	// 客户端 Read 应返回错误（EOF 或连接重置）
	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	tmp := make([]byte, 16)
	if _, err := conn.Read(tmp); err == nil {
		t.Fatal("DisconnectAll 后客户端仍可读，期望 EOF/错误")
	}

	// 模拟器仍可接受新连接（监听未关闭，模拟设备掉线后可重连）
	conn2 := dialSim(t, s.Addr())
	waitClientCount(t, s, 1, time.Second)
	resp := sendRecv(t, conn2, "AFTER-RECONNECT")
	if string(resp) != "AFTER-RECONNECT" {
		t.Fatalf("重连后回显不符: got %q", resp)
	}
}

// TestSimulator_ConcurrentClients 验证 3 个客户端并发连接，ClientCount==3 且互不影响。
func TestSimulator_ConcurrentClients(t *testing.T) {
	s := startEchoSim(t)
	const n = 3
	conns := make([]net.Conn, n)
	for i := 0; i < n; i++ {
		conns[i] = dialSim(t, s.Addr())
	}
	waitClientCount(t, s, n, time.Second)
	if got := s.ClientCount(); got != n {
		t.Fatalf("ClientCount = %d, want %d", got, n)
	}

	// 每个客户端独立发命令验证互不影响
	for i, c := range conns {
		cmd := string(rune('A' + i)) // "A","B","C"
		_ = c.SetReadDeadline(time.Now().Add(2 * time.Second))
		if _, err := c.Write([]byte(cmd + "\r\n")); err != nil {
			t.Fatalf("conn%d Write: %v", i, err)
		}
		buf := make([]byte, 1)
		if _, err := io.ReadFull(c, buf); err != nil {
			t.Fatalf("conn%d Read: %v", i, err)
		}
		if string(buf) != cmd {
			t.Fatalf("conn%d 回显不符: got %q want %q", i, buf, cmd)
		}
	}
}

// TestClient_SendCountsDroppedFrames 验证 writeCh 缓冲满时 send 走 default 分支
// 丢帧并递增 dropped 计数（TDD：先写测试，再实现计数与访问器）。
//
// 用容量 1 的 writeCh 构造独立 client，无需经 Simulator，确定性无竞态：
//   - 第一帧入队；
//   - 第二、三帧走 default 丢弃，dropped 依次为 1、2。
func TestClient_SendCountsDroppedFrames(t *testing.T) {
	c := &client{
		writeCh: make(chan []byte, 1), // 容量 1，易于填满验证丢帧计数
		done:    make(chan struct{}),
	}
	// 未关闭：第一帧应入队
	c.send([]byte("frame-1"))
	if got := len(c.writeCh); got != 1 {
		t.Fatalf("writeCh len = %d, want 1", got)
	}
	// 第二帧应走 default 丢弃，dropped 计数 +1
	c.send([]byte("frame-2"))
	if got := c.dropped.Load(); got != 1 {
		t.Fatalf("第一次丢帧后 dropped = %d, want 1", got)
	}
	// 第三帧再次丢弃，计数递增
	c.send([]byte("frame-3"))
	if got := c.dropped.Load(); got != 2 {
		t.Fatalf("第二次丢帧后 dropped = %d, want 2", got)
	}
}

// TestSimulator_DroppedFrames 验证 Simulator.DroppedFrames() 聚合所有客户端的
// 丢帧计数：客户端不读 conn 时，InjectFrame 灌入大量大帧填满 conn 发送缓冲与
// writeCh(64) 后触发背压丢帧。
//
// 原理：echoHandler 不自发帧，用 InjectFrame 主动灌帧。客户端不读 conn，
// writeLoop 写满 conn 内核发送缓冲后阻塞，writeCh 随之填满，后续 send 走
// default 分支丢弃并计数。8KB/帧 + 500 帧远超 conn 缓冲 + writeCh(64)，
// 必定丢帧。Close 经 t.Cleanup 触发，关闭 conn 使阻塞的 writeLoop 退出，无需手动排空。
func TestSimulator_DroppedFrames(t *testing.T) {
	s := startEchoSim(t)
	_ = dialSim(t, s.Addr()) // 建立客户端连接，不读 conn 以触发背压
	waitClientCount(t, s, 1, time.Second)

	// 初始无丢帧
	if got := s.DroppedFrames(); got != 0 {
		t.Fatalf("初始 DroppedFrames = %d, want 0", got)
	}

	// 灌入大量大帧填满 conn 发送缓冲 + writeCh(64)，触发背压丢帧。
	bigFrame := make([]byte, 8192) // 8KB/帧，64KB conn 缓冲约容纳 8 帧
	for i := range bigFrame {
		bigFrame[i] = 'X'
	}
	for i := 0; i < 500; i++ {
		s.InjectFrame(bigFrame)
	}
	if got := s.DroppedFrames(); got == 0 {
		t.Fatal("期望缓冲满后丢帧，但 DroppedFrames()=0")
	}
	t.Logf("DroppedFrames = %d（已验证背压丢帧计数生效）", s.DroppedFrames())
}
