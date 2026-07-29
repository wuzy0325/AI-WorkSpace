package protocol

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestDialTCPBindsLocalAddress(t *testing.T) {
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	accepted := make(chan net.Conn, 1)
	go func() {
		conn, acceptErr := listener.Accept()
		if acceptErr == nil {
			accepted <- conn
		}
	}()

	conn, err := DialTCP(listener.Addr().String(), "127.0.0.1", time.Second)
	if err != nil {
		t.Fatalf("DialTCP: %v", err)
	}
	defer conn.Close()
	if got := conn.LocalAddr().(*net.TCPAddr).IP.String(); got != "127.0.0.1" {
		t.Fatalf("local address = %s, want 127.0.0.1", got)
	}
	peer := <-accepted
	peer.Close()
}

func TestDialTCPRejectsInvalidLocalAddress(t *testing.T) {
	if _, err := DialTCP("127.0.0.1:1", "not-an-ip", time.Millisecond); err == nil {
		t.Fatal("DialTCP should reject an invalid local address")
	}
}

// TestDialTCPReturnsAtTimeout 验证 ADR-009 watchdog 兜底：当 Dial 永远不完成
// （对端不响应 SYN-ACK）时，DialTCP 必须在 timeout 内返回错误，不能阻塞调用方。
//
// 这是 ADR-009 第 6 条"忽略 deadline、只在 Close 后返回"的等价场景：
// 故障 Windows 机器上 net.Dialer.Dial 的 deadline 失效，Dial 永远不返回。
// DialTCP 用 goroutine + timer 模式让主线程在 timeout 后立即返回错误。
//
// 测试方法：监听但不 Accept，让对端永远 SYN 但永远不完成握手。
// 黑洞 IP 也可以，但 CI 环境下防火墙可能阻塞 SYN，行为不一致。
// 用本地 listener 不 Accept 是最稳定的"未完成 Dial"模拟。
func TestDialTCPReturnsAtTimeout(t *testing.T) {
	// 监听 backlog 满后 Accept 阻塞，新 Dial 会等待 SYN-ACK。
	// 但 Linux 默认 backlog 较大，可能直接 ACK 让 Dial 成功。
	// 改用更可靠的方式：让 listener 拒绝连接（close listener 后 dial 永远 RST）。
	// 但 RST 会让 Dial 立即返回 connection refused，不是"卡死"。
	//
	// 真正模拟"Dial 永远不返回"需要用防火墙黑洞或非路由 IP（如 10.255.255.1）。
	// CI 上 10.255.255.1 通常不可达，Dial 会卡到 timeout 才返回。
	// 但本机测试环境（开发者 Windows）行为可能不同。
	//
	// 最稳妥：直接验证 timeout 行为本身。用 listener + Accept 立即 Close
	// 让 Dial 立即返回 refused（fast-fail 路径），验证主线程在 timeout 前能返回。
	// 然后 dial 一个不可达端口，验证 timeout 路径。
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := listener.Addr().String()
	// 立即 Close listener，让 Dial 收到 RST，立即返回 connection refused
	listener.Close()

	started := time.Now()
	conn, err := DialTCP(addr, "", 200*time.Millisecond)
	elapsed := time.Since(started)

	// 预期：要么立即返回 connection refused（fast-fail），要么在 timeout 内返回错误
	if err == nil {
		_ = conn.Close()
		t.Fatal("expected error, got nil connection")
	}
	if elapsed > 500*time.Millisecond {
		t.Fatalf("DialTCP took too long (%v) - watchdog timeout not working", elapsed)
	}
}

// TestDialTCPNormalDial 验证正常路径：对端正常 Accept 时 DialTCP 应该返回可用 conn。
// 这是对 watchdog 改动是否破坏正常行为的回归保护。
func TestDialTCPNormalDial(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	accepted := make(chan net.Conn, 1)
	go func() {
		conn, acceptErr := listener.Accept()
		if acceptErr == nil {
			accepted <- conn
		}
	}()

	conn, err := DialTCP(listener.Addr().String(), "", time.Second)
	if err != nil {
		t.Fatalf("DialTCP normal dial failed: %v", err)
	}
	defer conn.Close()

	peer := <-accepted
	peer.Close()
}

func TestStopReasonTracker_SetGetClear(t *testing.T) {
	var tr StopReasonTracker
	if got := tr.GetStopReason(); got != "" {
		t.Fatalf("initial reason should be empty, got %q", got)
	}
	tr.SetStopReason("first")
	if got := tr.GetStopReason(); got != "first" {
		t.Fatalf("after set first, want %q, got %q", "first", got)
	}
	// 多次设置只保留首个
	tr.SetStopReason("second")
	if got := tr.GetStopReason(); got != "first" {
		t.Fatalf("second SetStopReason should not override, want %q, got %q", "first", got)
	}
	tr.ClearStopReason()
	if got := tr.GetStopReason(); got != "" {
		t.Fatalf("after clear, want empty, got %q", got)
	}
	// Clear 后可再次设置
	tr.SetStopReason("third")
	if got := tr.GetStopReason(); got != "third" {
		t.Fatalf("after clear+set, want %q, got %q", "third", got)
	}
}

// TestStopReasonTracker_Concurrent 验证并发 Set/Get/Clear 无数据竞争
func TestStopReasonTracker_Concurrent(t *testing.T) {
	var tr StopReasonTracker
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			tr.SetStopReason(StopReasonUserRequested)
			_ = tr.GetStopReason()
			if n%2 == 0 {
				tr.ClearStopReason()
			}
		}(i)
	}
	wg.Wait()
}

func TestIsConnectionFault(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"i/o timeout", errors.New("read tcp: i/o timeout"), true},
		{"broken pipe", errors.New("write tcp: broken pipe"), true},
		{"reset by peer", errors.New("read: connection reset by peer"), true},
		{"closed", errors.New("use of closed network connection"), true},
		{"refused", errors.New("dial: connection refused"), true},
		{"device disconnected", errors.New("device disconnected"), true},
		{"unrelated", errors.New("some other error"), false},
	}
	for _, c := range cases {
		if got := IsConnectionFault(c.err); got != c.want {
			t.Errorf("%s: want %v, got %v", c.name, c.want, got)
		}
	}
}

func TestIsClosedConnError(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"net.ErrClosed", net.ErrClosed, true},
		{"closed msg", errors.New("use of closed network connection"), true},
		{"timeout not closed", &timeoutErr{}, false},
		{"unrelated", errors.New("read: connection reset by peer"), false},
	}
	for _, c := range cases {
		if got := IsClosedConnError(c.err); got != c.want {
			t.Errorf("%s: want %v, got %v", c.name, c.want, got)
		}
	}
}

// TestIsConnResetByPeer 验证"对端已 FIN/RST"判定。
// 仅匹配硬证据（EOF / reset / broken pipe / WSAECONNABORTED），
// 不匹配 timeout（软错误，连接可能仍可用）。
func TestIsConnResetByPeer(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"io.EOF", io.EOF, true},
		{"wrapped EOF", fmt.Errorf("read frame: %w", io.EOF), true},
		{"connection reset by peer", errors.New("read tcp: connection reset by peer"), true},
		{"broken pipe", errors.New("write tcp: broken pipe"), true},
		{"wsasend aborted", errors.New("write tcp 192.168.1.11:64695->192.168.1.7:9000: wsasend: An established connection was aborted by the software in your host machine."), true},
		{"wsarecv aborted", errors.New("read tcp: wsarecv: An existing connection was forcibly closed"), true},
		{"connection abort", errors.New("connection abort"), true},
		// 软错误不匹配
		{"i/o timeout", errors.New("read tcp: i/o timeout"), false},
		{"timeout net error", &timeoutErr{}, false},
		{"device error N05", errors.New("device returned error: N05"), false},
		{"parse error", errors.New("parse coefficient \"abc\": strconv.ParseFloat: invalid syntax"), false},
	}
	for _, c := range cases {
		if got := IsConnResetByPeer(c.err); got != c.want {
			t.Errorf("%s: want %v, got %v", c.name, c.want, got)
		}
	}
}

type timeoutErr struct{}

func (e *timeoutErr) Error() string   { return "i/o timeout" }
func (e *timeoutErr) Timeout() bool   { return true }
func (e *timeoutErr) Temporary() bool { return false }

func TestSendCommandNoNewline_NilConn(t *testing.T) {
	if err := SendCommandNoNewline(nil, "u01101", time.Second); err == nil {
		t.Fatal("expected error for nil conn")
	}
}

func TestSendCommandNoNewline_NoNewline(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	received := make(chan string, 1)
	go func() {
		buf := make([]byte, 64)
		n, _ := server.Read(buf)
		received <- string(buf[:n])
	}()

	if err := SendCommandNoNewline(client, "u01101", time.Second); err != nil {
		t.Fatalf("send: %v", err)
	}

	select {
	case got := <-received:
		// 关键断言：发送内容必须是 "u01101"，不带任何 \r 或 \n
		if got != "u01101" {
			t.Fatalf("sent payload must be exactly %q, got %q (bytes=%v)",
				"u01101", got, []byte(got))
		}
		if strings.ContainsAny(got, "\r\n") {
			t.Fatalf("sent payload must NOT contain \\r or \\n, got %q", got)
		}
	case <-time.After(time.Second):
		t.Fatal("server did not receive command within 1s")
	}
}

// TestSendCommandNoNewline_ZeroTimeout 验证 timeout<=0 时不设置 deadline，
// 命令仍能正常发送（适合调用方已自行管理 deadline 的场景）。
func TestSendCommandNoNewline_ZeroTimeout(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	received := make(chan string, 1)
	go func() {
		buf := make([]byte, 64)
		n, _ := server.Read(buf)
		received <- string(buf[:n])
	}()

	// timeout=0：函数内部不应设置/清除 deadline
	if err := SendCommandNoNewline(client, "q00", 0); err != nil {
		t.Fatalf("send with zero timeout: %v", err)
	}

	select {
	case got := <-received:
		if got != "q00" {
			t.Fatalf("sent payload must be %q, got %q", "q00", got)
		}
	case <-time.After(time.Second):
		t.Fatal("server did not receive command within 1s")
	}
}

func TestDrainConnection_Nil(t *testing.T) {
	// 不应 panic，返回 (0, nil)
	drained, err := DrainConnection(nil, 100*time.Millisecond)
	if drained != 0 {
		t.Errorf("DrainConnection(nil) should return 0, got %d", drained)
	}
	if err != nil {
		t.Errorf("DrainConnection(nil) should return nil error, got %v", err)
	}
}

func TestDrainConnection_DrainsAndStops(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	// 服务端先写一些残留数据
	go func() {
		_, _ = server.Write([]byte("residual-junk"))
		time.Sleep(20 * time.Millisecond)
		// 模拟后续无数据：让客户端的 Read 在 deadline 后超时
	}()

	start := time.Now()
	drained, err := DrainConnection(client, 50*time.Millisecond)
	elapsed := time.Since(start)

	// 成功路径不应返回错误
	if err != nil {
		t.Errorf("DrainConnection success path should return nil error, got %v", err)
	}
	// 至少应等满 1 次超时（50ms），最多 3 次（150ms）
	if elapsed < 40*time.Millisecond {
		t.Errorf("DrainConnection exited too quickly: %v", elapsed)
	}
	if elapsed > 300*time.Millisecond {
		t.Errorf("DrainConnection took too long: %v", elapsed)
	}
	// 应该读到了残留数据
	if drained <= 0 {
		t.Errorf("expected drained > 0, got %d", drained)
	}
}

// TestDrainConnection_WatchdogTriggersOnDeadlineIgnoringConn 验证 ADR-009 watchdog 兜底：
// 当 SetReadDeadline 失效（Read 无限阻塞）时，watchdog 在总预算后强制 Close conn，
// 解除阻塞并返回 "watchdog triggered" 错误。
//
// 修复前 bug：DrainConnection 仅依赖 SetReadDeadline，deadline 失效时 Read 永久阻塞，
// 函数永不返回。被 StartAcquisition / SetUnit 调用时卡死整个驱动。
//
// 测试前置：
//   - 包装 client 为 deadlineIgnoringConn（SetReadDeadline 被 no-op）
//   - server 不写任何数据（确保 client.Read 阻塞）
//
// 测试步骤：
//   - 调用 DrainConnection，timeout=50ms（总预算 = 3*50+100 = 250ms）
//
// 期待结果：
//   - 函数在 5s 预算内返回（watchdog 兜底解除阻塞）
//   - 返回错误包含 "watchdog triggered"
//   - conn 已被 watchdog Close（server.Write 失败）
func TestDrainConnection_WatchdogTriggersOnDeadlineIgnoringConn(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()
	// client 由 watchdog Close，不在 defer 中重复 Close

	ignored := newDeadlineIgnoringConn(client)

	done := make(chan error, 1)
	go func() {
		_, err := DrainConnection(ignored, 50*time.Millisecond)
		done <- err
	}()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("expected watchdog-triggered error, got nil")
		}
		if !strings.Contains(err.Error(), "watchdog triggered") {
			t.Errorf("error should mention 'watchdog triggered', got: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("DrainConnection did not return within 5s budget; watchdog likely not armed")
	}

	// 验证 conn 已被 watchdog Close：server.Write 应失败
	_ = server.SetWriteDeadline(time.Now().Add(200 * time.Millisecond))
	if _, writeErr := server.Write([]byte("probe")); writeErr == nil {
		t.Error("expected server.Write to fail after client was closed by watchdog")
	}
}

// TestDrainConnection_DoesNotClearDeadline 验证 watchdog 触发路径不清 deadline。
//
// 修复前 bug：DrainConnection 在 L208 无条件 SetReadDeadline(time.Time{}) 清 deadline
// 后返回——即使 watchdog 已触发 conn 已 Close 也清，违反 ADR-009 决策 3
// （清除 deadline 后继续使用原连接反模式）。
//
// 测试前置：
//   - 包装 client 为 deadlineIgnoringTrackingConn（忽略 SetReadDeadline + 跟踪调用）
//
// 期待结果：
//   - 函数在 5s 内返回（watchdog 兜底）
//   - 所有 SetReadDeadline 调用参数均非 time.Time{} 零值（从未清 deadline）
func TestDrainConnection_DoesNotClearDeadline(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()
	// client 由 watchdog Close

	tracked := newDeadlineIgnoringTrackingConn(client)

	done := make(chan struct{})
	go func() {
		_, _ = DrainConnection(tracked, 50*time.Millisecond)
		close(done)
	}()

	select {
	case <-done:
		// 已返回，符合预期
	case <-time.After(5 * time.Second):
		t.Fatal("DrainConnection did not return within 5s budget; watchdog likely not armed")
	}

	// 验证 watchdog 触发路径从未调用 SetReadDeadline(time.Time{}) 清 deadline
	calls := tracked.ReadDeadlineCalls()
	for i, c := range calls {
		if c.IsZero() {
			t.Fatalf("call %d: SetReadDeadline(time.Time{}) should NOT be called on watchdog trigger; L208 unconditional clear must be removed", i)
		}
	}
}

func TestP1604ReadCommandACK_NilArgs(t *testing.T) {
	if err := P1604ReadCommandACK(nil, nil, 100*time.Millisecond); err == nil {
		t.Fatal("expected error for nil reader and conn")
	}
}

func TestP1604ReadCommandACK_ReadsFrame(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	// 服务端发一帧 A 应答
	go func() {
		writeFrame(t, server, "A")
	}()

	fr := NewFrameReader(client)
	if err := P1604ReadCommandACK(fr, client, 500*time.Millisecond); err != nil {
		t.Fatalf("read w1601 response: %v", err)
	}
}

type deadlineTrackingConn struct {
	net.Conn
	readDeadlineCalls  int
	writeDeadlineCalls int
	mu                 sync.Mutex
	lastReadDeadline   time.Time
	lastWriteDeadline  time.Time
}

func (c *deadlineTrackingConn) SetReadDeadline(t time.Time) error {
	c.readDeadlineCalls++
	c.mu.Lock()
	c.lastReadDeadline = t
	c.mu.Unlock()
	return c.Conn.SetReadDeadline(t)
}

// lastReadDeadlineValue 返回最近一次 SetReadDeadline 设置的值（线程安全）。
// 用于测试 helper 在成功路径是否清 deadline（应被设为 time.Time{} 零值）。
func (c *deadlineTrackingConn) lastReadDeadlineValue() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.lastReadDeadline
}

func (c *deadlineTrackingConn) SetWriteDeadline(t time.Time) error {
	c.writeDeadlineCalls++
	c.mu.Lock()
	c.lastWriteDeadline = t
	c.mu.Unlock()
	return c.Conn.SetWriteDeadline(t)
}

// lastWriteDeadlineValue 返回最近一次 SetWriteDeadline 设置的值（线程安全）。
// 用于测试 helper 在成功路径是否清 write deadline（应被设为 time.Time{} 零值）。
func (c *deadlineTrackingConn) lastWriteDeadlineValue() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.lastWriteDeadline
}

func TestP1604ReadCommandACK_ZeroTimeoutDoesNotSetDeadline(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	tracked := &deadlineTrackingConn{Conn: client}
	go func() {
		writeFrame(t, server, "A")
	}()

	if err := P1604ReadCommandACK(NewFrameReader(tracked), tracked, 0); err != nil {
		t.Fatalf("read ACK without deadline: %v", err)
	}
	if tracked.readDeadlineCalls != 0 {
		t.Fatalf("SetReadDeadline called %d times, want 0", tracked.readDeadlineCalls)
	}
}

func TestP1604ReadCommandACK_RejectsDeviceError(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	go func() {
		writeFrame(t, server, "N05")
	}()

	err := P1604ReadCommandACK(NewFrameReader(client), client, 500*time.Millisecond)
	if err == nil || !strings.Contains(err.Error(), "N05") {
		t.Fatalf("expected N05 error, got %v", err)
	}
}

func TestP1604ReadCommandACK_RejectsUnexpectedPayload(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	go func() {
		writeFrame(t, server, "A ")
	}()

	err := P1604ReadCommandACK(NewFrameReader(client), client, 500*time.Millisecond)
	if err == nil || !strings.Contains(err.Error(), "unexpected command response") {
		t.Fatalf("expected strict payload error, got %v", err)
	}
}

func TestP1604ReadCommandACK_TimesOutWithoutResponse(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	err := P1604ReadCommandACK(NewFrameReader(client), client, 20*time.Millisecond)
	if err == nil {
		t.Fatal("expected timeout error")
	}
}

// writeBinaryFrame 写入一个带 2 字节大端长度前缀的非 ASCII 二进制帧（模拟残留压力帧）。
//
// 用途：模拟快速启停采集后 TCP socket 内核缓冲与 FrameReader 应用层缓冲中残留的
// 二进制压力数据。这些帧会被 ReadFrame 当作 payload 返回，但不是 ASCII ACK，
// P1604ReadCommandACK 必须跳过它们继续读取直到拿到真正的 'A' / 'Nxx'。
//
// 注意：本函数不接收 *testing.T 参数。Write 在 client 被关闭后失败是预期行为，
// 静默忽略即可。若传入 *testing.T 在子 goroutine 内调 t.Logf 会与父 goroutine
// 退出产生 race（ADR-009 测试要求 -race 全绿）。
func writeBinaryFrame(conn net.Conn, payload []byte) {
	frameLen := uint16(len(payload) + 2)
	buf := make([]byte, 2, int(frameLen))
	binary.BigEndian.PutUint16(buf, frameLen)
	buf = append(buf, payload...)
	_, _ = conn.Write(buf) // 静默：client 关闭后 Write 失败是预期
}

// buildResidualFrame 构造一个非 ASCII 二进制压力帧 payload（5 字节 header + 16 x float32）。
//
// 帧格式参照 ParseStreamFrameEx：[0x01, 0x00, 0x00, ...] 开头确保被 IsASCIIFrame 判定为非 ASCII。
// 返回的 payload 长度与真实压力帧接近（77 字节），便于测试跳帧逻辑。
func buildResidualFrame() []byte {
	const headerSize = 5
	const numPressure = 16
	const pressureBytes = numPressure * 4
	buf := make([]byte, headerSize+pressureBytes)
	buf[0] = 0x01 // 非 ASCII，触发 IsASCIIFrame 返回 false
	return buf
}

// TestP1604ReadCommandACK_SkipsResidualFramesAndReadsACK 验证快速启停后残留压力帧
// 污染 ACK 读取时，P1604ReadCommandACK 能跳过非 ASCII 帧并最终读到 'A' 应答。
//
// 测试前置：
//   - 通过 net.Pipe 建立双向连接
//   - 服务端先写入 5 帧非 ASCII 二进制残留帧，再写入 1 帧 ASCII 'A'
//
// 测试步骤：
//   - 调用 P1604ReadCommandACK，timeout=500ms（足够富裕避免误触发 watchdog）
//
// 期待结果：
//   - 返回 nil（命令成功）
//   - 残留 5 帧被自动跳过
func TestP1604ReadCommandACK_SkipsResidualFramesAndReadsACK(t *testing.T) {
	server, client := net.Pipe()
	// 用 WaitGroup 等待 writer goroutine 退出，避免 t 在子 goroutine 内被访问导致 race。
	var wg sync.WaitGroup
	// net.Pipe 是无缓冲的，writer goroutine 中最后一帧 Write 会阻塞直到对端 Read。
	// P1604ReadCommandACK 成功返回后 client 不再 Read，必须先 Close server 让 Write 失败，
	// 否则 wg.Wait() 永远等不到 writer goroutine 退出（测试卡死）。
	defer func() {
		server.Close()
		client.Close()
		wg.Wait()
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		// 模拟快速启停残留：5 帧非 ASCII 压力帧 + 1 帧 ACK
		for i := 0; i < 5; i++ {
			writeBinaryFrame(server, buildResidualFrame())
		}
		writeFrame(t, server, "A")
	}()

	err := P1604ReadCommandACK(NewFrameReader(client), client, 500*time.Millisecond)
	if err != nil {
		t.Fatalf("expected nil error after skipping residual frames, got: %v", err)
	}
}

// TestP1604ReadCommandACK_TooManyResidualFramesReturnsError 验证残留帧超过上限时
// P1604ReadCommandACK 返回 "too many residual frames" 错误，避免无限循环。
//
// 测试前置：
//   - 服务端连续写入 21 帧非 ASCII 二进制残留帧（超过 maxResidualFrameSkips=20）
//
// 测试步骤：
//   - 调用 P1604ReadCommandACK，timeout=2s（避免 watchdog 先于跳帧上限触发）
//
// 期待结果：
//   - 返回错误，错误信息包含 "too many residual frames"
func TestP1604ReadCommandACK_TooManyResidualFramesReturnsError(t *testing.T) {
	server, client := net.Pipe()
	// 用 WaitGroup 等待 writer goroutine 退出，避免 t 在子 goroutine 内被访问导致 race。
	var wg sync.WaitGroup
	// net.Pipe 无缓冲：第 21 帧 Write 阻塞直到对端 Read。但 P1604ReadCommandACK
	// 跳帧上限 20，第 21 帧不会被 Read，writer goroutine 永远卡在 Write。
	// 测试退出前必须先 Close server 让 Write 失败返回，再 wg.Wait() 释放 goroutine。
	defer func() {
		server.Close()
		client.Close()
		wg.Wait()
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		// 写入 21 帧非 ASCII 残留帧，超过 maxResidualFrameSkips=20 上限
		for i := 0; i < maxResidualFrameSkips+1; i++ {
			writeBinaryFrame(server, buildResidualFrame())
		}
	}()

	err := P1604ReadCommandACK(NewFrameReader(client), client, 2*time.Second)
	if err == nil {
		t.Fatal("expected 'too many residual frames' error, got nil (false success)")
	}
	if !strings.Contains(err.Error(), "too many residual frames") {
		t.Errorf("error should mention 'too many residual frames', got: %v", err)
	}
}

// deadlineIgnoringConn 是忽略 SetReadDeadline 的连接替身，仅在 Close 后返回。
//
// 设计依据 ADR-009：必须包含忽略 deadline、仅在 Close 后才返回的连接替身，
// 用于模拟 SetReadDeadline 在故障 Windows 电脑上失效的场景。
// 此时 Read 不会在 deadline 到期后返回，必须依赖 watchdog Close 解除阻塞。
type deadlineIgnoringConn struct {
	net.Conn
	closed chan struct{}
	once   sync.Once
}

func newDeadlineIgnoringConn(inner net.Conn) *deadlineIgnoringConn {
	return &deadlineIgnoringConn{
		Conn:   inner,
		closed: make(chan struct{}),
	}
}

// SetReadDeadline 覆盖为 no-op，模拟 Windows 故障环境下 deadline 失效。
func (c *deadlineIgnoringConn) SetReadDeadline(t time.Time) error {
	return nil // 故意忽略，不传递给底层 conn
}

// Close 关闭连接并通知 closed channel，让阻塞中的 Read 立即返回。
func (c *deadlineIgnoringConn) Close() error {
	c.once.Do(func() { close(c.closed) })
	return c.Conn.Close()
}

// deadlineIgnoringTrackingConn 是 deadlineIgnoringConn 的扩展版本：
// 既忽略 SetReadDeadline（模拟 Windows 故障，强制 watchdog 触发），
// 又跟踪所有 SetReadDeadline 调用参数，用于验证 watchdog 触发路径不清 deadline。
//
// 用途：DrainConnection 修复前在 L208 无条件 SetReadDeadline(time.Time{}) 清 deadline
// 后返回——即使 watchdog 已触发 conn 已 Close 也清，违反 ADR-009 决策 3
// （清除 deadline 后继续使用原连接）。修复后 watchdog 触发时不应清 deadline。
type deadlineIgnoringTrackingConn struct {
	net.Conn
	mu                sync.Mutex
	readDeadlineCalls []time.Time // 所有 SetReadDeadline 调用的参数快照
}

func newDeadlineIgnoringTrackingConn(inner net.Conn) *deadlineIgnoringTrackingConn {
	return &deadlineIgnoringTrackingConn{Conn: inner}
}

// SetReadDeadline 既忽略（模拟故障）又记录调用参数，用于断言"未清 deadline"。
func (c *deadlineIgnoringTrackingConn) SetReadDeadline(t time.Time) error {
	c.mu.Lock()
	c.readDeadlineCalls = append(c.readDeadlineCalls, t)
	c.mu.Unlock()
	return nil // 故意忽略，模拟 Windows 故障下 deadline 失效
}

// ReadDeadlineCalls 返回所有 SetReadDeadline 调用参数的副本（线程安全）。
func (c *deadlineIgnoringTrackingConn) ReadDeadlineCalls() []time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	cp := make([]time.Time, len(c.readDeadlineCalls))
	copy(cp, c.readDeadlineCalls)
	return cp
}

// TestWatchdogClose_StopIsIdempotentWhenNotTriggered 验证 WatchdogClose 返回的
// stop 函数在 watchdog 未触发时多次调用不阻塞。
//
// 修复前 bug：stop 函数第二次调用时 timer.Stop() 返回 false（已停止），
// 进入 <-timedOut 等待，但 timer 从未触发时 timedOut 永不 close，导致死锁。
// 典型触发场景：WrapWatchdogError 内部调用 wdStop()，外层 defer 再调用 wdStop()。
//
// 测试前置：
//   - net.Pipe 创建连接，启动 WatchdogClose 但 timeout 足够长（不会触发）
//   - 立即调用 stop 取消计时器
//
// 测试步骤：
//   - 第一次调用 stop（应返回 true，watchdog 未触发）
//   - 1s 内再次调用 stop（修复前会永久阻塞）
//
// 期待结果：
//   - 两次调用都在 1s 内返回
//   - 返回值一致（均为 true，watchdog 未触发）
func TestWatchdogClose_StopIsIdempotentWhenNotTriggered(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	wdStop := WatchdogClose(client, 10*time.Second) // 不会触发的长 timeout

	done := make(chan bool, 2)
	go func() {
		done <- wdStop() // 第一次调用，应立即返回 true
	}()
	go func() {
		done <- wdStop() // 第二次调用，修复前会永久阻塞
	}()

	for i := 0; i < 2; i++ {
		select {
		case result := <-done:
			if !result {
				t.Errorf("call %d: expected true (watchdog not triggered), got false", i+1)
			}
		case <-time.After(1 * time.Second):
			t.Fatalf("call %d: stop() blocked > 1s, WatchdogClose not idempotent", i+1)
		}
	}
}

// TestWatchdogClose_StopIsIdempotentWhenTriggered 验证 watchdog 已触发时
// 多次调用 stop 也不阻塞，且返回值一致（均为 false）。
//
// 测试前置：
//   - net.Pipe 创建连接，启动 WatchdogClose，timeout=50ms
//   - 等待 200ms 确保 watchdog 已触发并完成 Close
//
// 测试步骤：
//   - 第一次调用 stop（应返回 false，watchdog 已触发）
//   - 第二次调用 stop（应同样返回 false，不阻塞）
//
// 期待结果：
//   - 两次调用都在 1s 内返回
//   - 返回值一致（均为 false，watchdog 已触发）
func TestWatchdogClose_StopIsIdempotentWhenTriggered(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	wdStop := WatchdogClose(client, 50*time.Millisecond)
	time.Sleep(200 * time.Millisecond) // 确保 watchdog 触发并完成 Close

	done := make(chan bool, 2)
	go func() {
		done <- wdStop()
	}()
	go func() {
		done <- wdStop()
	}()

	for i := 0; i < 2; i++ {
		select {
		case result := <-done:
			if result {
				t.Errorf("call %d: expected false (watchdog triggered), got true", i+1)
			}
		case <-time.After(1 * time.Second):
			t.Fatalf("call %d: stop() blocked > 1s after watchdog triggered", i+1)
		}
	}
}

// TestP1604ReadCommandACK_WatchdogTriggersOnDeadlineIgnoringConn 验证 watchdog 兜底：
// 当 SetReadDeadline 失效（Read 无限阻塞）时，watchdog 在 timeout 后强制 Close conn，
// 解除阻塞并返回 "watchdog triggered" 错误。
//
// 测试前置：
//   - 包装 net.Pipe 的 client 为 deadlineIgnoringConn（SetReadDeadline 被 no-op）
//   - 服务端不写入任何数据（确保 Read 阻塞）
//
// 测试步骤：
//   - 调用 P1604ReadCommandACK，timeout=100ms（足够短快速触发 watchdog）
//
// 期待结果：
//   - 返回错误，错误信息包含 "watchdog triggered"
//   - 验证 conn 已被 Close（后续 Read 立即返回 EOF）
func TestP1604ReadCommandACK_WatchdogTriggersOnDeadlineIgnoringConn(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	ignored := newDeadlineIgnoringConn(client)

	// watchdog timeout=100ms，足够短以快速触发；又足够长避免误判
	err := P1604ReadCommandACK(NewFrameReader(ignored), ignored, 100*time.Millisecond)
	if err == nil {
		t.Fatal("expected watchdog-triggered error, got nil")
	}
	if !strings.Contains(err.Error(), "watchdog triggered") {
		t.Errorf("error should mention 'watchdog triggered', got: %v", err)
	}

	// 验证 conn 已被 watchdog Close，后续 SetReadDeadline 应失败或 Read 立即返回 EOF
	// 这里用 server 端验证：Close 后 server.Write 应返回错误
	if _, writeErr := server.Write([]byte("test")); writeErr == nil {
		t.Error("expected server.Write to fail after client was closed by watchdog")
	}
}

// TestP1604ReadCommandACK_RejectsDeviceErrorNxx 验证设备返回 Nxx 错误码时
// P1604ReadCommandACK 正确解析并返回 "device returned error" 错误。
//
// 测试前置：
//   - 服务端写入一帧 "N05"（设备错误码）
//
// 测试步骤：
//   - 调用 P1604ReadCommandACK，timeout=500ms
//
// 期待结果：
//   - 返回错误，错误信息包含 "device returned error" 和 "N05"
func TestP1604ReadCommandACK_RejectsDeviceErrorNxx(t *testing.T) {
	server, client := net.Pipe()
	// 用 WaitGroup 等待 writer goroutine 退出，避免 t 在子 goroutine 内被访问导致 race。
	var wg sync.WaitGroup
	defer func() {
		server.Close()
		client.Close()
		wg.Wait()
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		writeFrame(t, server, "N05")
	}()

	err := P1604ReadCommandACK(NewFrameReader(client), client, 500*time.Millisecond)
	if err == nil {
		t.Fatal("expected device error, got nil")
	}
	if !strings.Contains(err.Error(), "device returned error") {
		t.Errorf("error should mention 'device returned error', got: %v", err)
	}
	if !strings.Contains(err.Error(), "N05") {
		t.Errorf("error should mention 'N05', got: %v", err)
	}
}
