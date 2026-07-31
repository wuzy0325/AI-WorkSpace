package hardware

import (
	"bufio"
	"errors"
	"net"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	sharedproto "shared.local/device-sdk/go/protocol"
	"wind-daq/services/api-go/internal/core/device"
)

func TestDSA3217StartStopDoesNotDeadlock(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	d := NewDSA3217(device.Profile{ID: "dsa-1", Name: "DSA", Type: device.DeviceDSA3217})
	d.conn = client
	d.reader = bufio.NewReader(client)
	d.status.Connection = device.ConnectionConnected

	commands := make(chan string, 2)
	go func() {
		reader := bufio.NewReader(server)
		for i := 0; i < 2; i++ {
			line, err := reader.ReadString('\n')
			if err != nil {
				return
			}
			commands <- line
			_, _ = server.Write([]byte("OK\n"))
		}
	}()

	mustFinish(t, "StartAcquisition", func() error { return d.StartAcquisition() })
	mustReceiveCommand(t, commands, "SCAN\r\n")
	mustFinish(t, "StopAcquisition", func() error { return d.StopAcquisition() })
	mustReceiveCommand(t, commands, "STOP\r\n")
}

func mustFinish(t *testing.T, name string, fn func() error) {
	t.Helper()
	done := make(chan error, 1)
	go func() { done <- fn() }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("%s returned error: %v", name, err)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatalf("%s deadlocked", name)
	}
}

func mustReceiveCommand(t *testing.T, commands <-chan string, want string) {
	t.Helper()
	select {
	case got := <-commands:
		if got != want {
			t.Fatalf("expected command %q, got %q", want, got)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatalf("timed out waiting for command %q", want)
	}
}

// deadlineIgnoringConn 复用 daq_p1064pre_test.go 中的同款替身（包内共享）。
// 设计依据 ADR-009：忽略 SetReadDeadline，仅在 Close 后返回，模拟 Windows 故障环境。

// TestDSA3217SendCommand_WatchdogTriggersOnDeadlineIgnoringConn 验证 sendCommand
// 在 SetReadDeadline 失效（Read 无限阻塞）场景下能通过 watchdog 强制 Close conn 解除阻塞，
// 在预算内返回且错误含 "watchdog triggered"。
//
// 设计依据 ADR-009：socket deadline 不能作为有界硬件 I/O 的唯一取消机制，
// 必须有独立 owner 能直接调用 conn.Close()。
func TestDSA3217SendCommand_WatchdogTriggersOnDeadlineIgnoringConn(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()

	ignored := newDeadlineIgnoringConn(client)

	d := NewDSA3217(device.Profile{ID: "dsa-watchdog", Name: "DSA", Type: device.DeviceDSA3217})
	d.conn = ignored
	d.reader = bufio.NewReader(ignored)
	d.status.Connection = device.ConnectionConnected
	// 测试用 200ms watchdog 加速，避免等待生产默认 10s。
	d.cmdTimeout = 200 * time.Millisecond

	started := time.Now()
	_, err := d.sendCommand("TEST")

	elapsed := time.Since(started)
	if err == nil {
		t.Fatal("expected watchdog-triggered error, got nil")
	}
	if !strings.Contains(err.Error(), "watchdog triggered") {
		t.Errorf("error should mention 'watchdog triggered', got: %v", err)
	}
	// 预算 2s 足够覆盖 200ms watchdog + ReadString 在 conn Close 后返回的延迟。
	if elapsed > 2*time.Second {
		t.Fatalf("sendCommand took too long: %v (watchdog should have triggered at ~200ms)", elapsed)
	}

	// 验证 conn 已被 watchdog Close，后续 Write 应失败
	if _, writeErr := server.Write([]byte("test")); writeErr == nil {
		t.Error("expected server.Write to fail after client was closed by watchdog")
	}
}

// TestDSA3217Disconnect_DoesNotDeadlockWhenReadLoopBlocked 验证 readLoop 阻塞时
// Disconnect 能在 2s 内返回，不会因 readLoop 持 ioMu 卡死而永久阻塞。
//
// 场景：readLoop 持 ioMu 阻塞在 ReadString 上（deadline 失效），
// Disconnect close(stop) 后等 readLoopDone 超时，触发 invalidateConnection Close conn，
// readLoop 的 ReadString 在 conn Close 后返回错误释放 ioMu 并退出。
func TestDSA3217Disconnect_DoesNotDeadlockWhenReadLoopBlocked(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()

	ignored := newDeadlineIgnoringConn(client)

	d := NewDSA3217(device.Profile{ID: "dsa-disc", Name: "DSA", Type: device.DeviceDSA3217})
	d.conn = ignored
	d.reader = bufio.NewReader(ignored)
	d.status.Connection = device.ConnectionAcquiring
	d.acquiring = true
	d.scanning = true
	d.stop = make(chan struct{})
	d.readLoopDone = make(chan struct{})
	// readLoop watchdog 设长，确保测试场景中 readLoop 一直阻塞（验证 Disconnect 兜底）
	d.readLoopWatchdog = 30 * time.Second

	stop := d.stop
	go d.readLoop(stop)

	// 等待 readLoop 进入 ReadString 阻塞（持 ioMu）
	time.Sleep(100 * time.Millisecond)

	done := make(chan error, 1)
	go func() { done <- d.Disconnect() }()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Disconnect returned error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Disconnect deadlocked: readLoop holds ioMu and Disconnect cannot proceed")
	}

	// 验证连接状态：Disconnect 后应 Disconnected 或 Error（invalidate 路径）
	d.mu.RLock()
	conn := d.conn
	status := d.status.Connection
	lastName := d.status.LastError
	d.mu.RUnlock()
	if conn != nil {
		t.Error("d.conn should be nil after Disconnect")
	}
	// ADR-009 R2-2：readLoop 卡死走 invalidate 路径，必须保留 Error 状态，
	// 不能被 Phase 3 的 Disconnected 覆盖。原实现无条件置 Disconnected 会丢失
	// "连接被强制关闭"诊断状态，前端误判为"正常断开"。
	if status != device.ConnectionError {
		t.Errorf("status should be Error (preserved from invalidate), got %v", status)
	}
	if lastName == "" {
		t.Error("LastError should be set by invalidateConnection")
	}
}

// TestDSA3217Disconnect_PreservesErrorStatusFromInvalidate 验证 ADR-009 R2-2：
// readLoop join 超时走 invalidateConnectionAfterReadLoopTimeout 后，Disconnect 的
// Phase 3 必须保留 Error 状态，不能无条件覆盖为 Disconnected。
//
// 场景：readLoop 卡死在 ReadString（持 ioMu），Disconnect close(stop) 后等
// readLoopDone 超时（ReadLoopJoinTimeout=1s），触发 invalidate Close conn + Error。
// Phase 3 必须跳过 Disconnected 覆盖，保留 Error + LastError 让前端感知连接异常。
func TestDSA3217Disconnect_PreservesErrorStatusFromInvalidate(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()

	ignored := newDeadlineIgnoringConn(client)

	d := NewDSA3217(device.Profile{ID: "dsa-r22", Name: "DSA", Type: device.DeviceDSA3217})
	d.conn = ignored
	d.reader = bufio.NewReader(ignored)
	d.status.Connection = device.ConnectionAcquiring
	d.acquiring = true
	d.scanning = true
	d.stop = make(chan struct{})
	d.readLoopDone = make(chan struct{})
	// readLoop watchdog 设长，确保 readLoop 一直阻塞触发 invalidate 路径
	d.readLoopWatchdog = 30 * time.Second

	stop := d.stop
	go d.readLoop(stop)

	// 等待 readLoop 进入 ReadString 阻塞
	time.Sleep(100 * time.Millisecond)

	if err := d.Disconnect(); err != nil {
		t.Fatalf("Disconnect returned error: %v", err)
	}

	d.mu.RLock()
	status := d.status.Connection
	lastName := d.status.LastError
	d.mu.RUnlock()

	// R2-2 整改核心断言：readLoop 卡死场景必须保留 Error，不能覆盖为 Disconnected
	if status != device.ConnectionError {
		t.Errorf("status should be Error (preserved from invalidate), got %v", status)
	}
	if !strings.Contains(lastName, "reconnect required") {
		t.Errorf("LastError should contain 'reconnect required', got %q", lastName)
	}
}

// TestDSA3217Disconnect_NormalStopSetsDisconnected 验证正常停止路径下
// Disconnect 置 Disconnected（readLoop 在 join timeout 内退出，不走 invalidate）。
// 这是 R2-2 整改的反向断言：只有 invalidate 路径才保留 Error，正常路径仍 Disconnected。
func TestDSA3217Disconnect_NormalStopSetsDisconnected(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()

	d := NewDSA3217(device.Profile{ID: "dsa-normal", Name: "DSA", Type: device.DeviceDSA3217})
	d.conn = client
	d.reader = bufio.NewReader(client)
	d.status.Connection = device.ConnectionAcquiring
	d.acquiring = true
	d.scanning = true
	d.stop = make(chan struct{})
	d.readLoopDone = make(chan struct{})
	d.readLoopWatchdog = 30 * time.Second

	stop := d.stop
	// readLoop 正常退出：服务端立即 Close 让 ReadString 返回 EOF
	go func() {
		time.Sleep(50 * time.Millisecond)
		server.Close()
	}()
	go d.readLoop(stop)

	if err := d.Disconnect(); err != nil {
		t.Fatalf("Disconnect returned error: %v", err)
	}

	d.mu.RLock()
	status := d.status.Connection
	d.mu.RUnlock()

	// 正常停止路径不走 invalidate，状态应为 Disconnected（不是 Error）
	if status != device.ConnectionDisconnected {
		t.Errorf("status should be Disconnected on normal stop, got %v", status)
	}
}

// TestDSA3217StopAcquisition_JoinsReadLoopOrClosesConn 验证 StopAcquisition 在 readLoop
// 阻塞时通过 Close 兜底在 ReadLoopJoinTimeout+1s 内返回。
//
// 场景：readLoop 持 ioMu 阻塞 ReadString，StopAcquisition close(stop) 后等 readLoopDone
// 超时（ReadLoopJoinTimeout=1s），触发 invalidateConnection Close conn，
// readLoop 退出，StopAcquisition 返回。
func TestDSA3217StopAcquisition_JoinsReadLoopOrClosesConn(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()

	ignored := newDeadlineIgnoringConn(client)

	d := NewDSA3217(device.Profile{ID: "dsa-stop", Name: "DSA", Type: device.DeviceDSA3217})
	d.conn = ignored
	d.reader = bufio.NewReader(ignored)
	d.status.Connection = device.ConnectionAcquiring
	d.acquiring = true
	d.scanning = true
	d.stop = make(chan struct{})
	d.readLoopDone = make(chan struct{})
	// readLoop watchdog 设长，确保 readLoop 一直阻塞（验证 StopAcquisition 兜底）
	d.readLoopWatchdog = 30 * time.Second

	stop := d.stop
	go d.readLoop(stop)

	// 等待 readLoop 进入 ReadString 阻塞
	time.Sleep(100 * time.Millisecond)

	started := time.Now()
	done := make(chan error, 1)
	go func() { done <- d.StopAcquisition() }()

	select {
	case err := <-done:
		// StopAcquisition 在 readLoop 卡死时返回 "reconnect required" 错误
		if err == nil {
			t.Fatal("expected reconnect required error, got nil")
		}
		if !strings.Contains(err.Error(), "reconnect required") {
			t.Errorf("error should mention 'reconnect required', got: %v", err)
		}
	case <-time.After(sharedproto.ReadLoopJoinTimeout + 2*time.Second):
		t.Fatal("StopAcquisition deadlocked: readLoop holds ioMu and StopAcquisition cannot proceed")
	}

	elapsed := time.Since(started)
	// 预算 ReadLoopJoinTimeout(1s) + 2s 缓冲
	if elapsed > sharedproto.ReadLoopJoinTimeout+2*time.Second {
		t.Fatalf("StopAcquisition took too long: %v", elapsed)
	}

	// 验证连接已 invalidate
	d.mu.RLock()
	conn := d.conn
	status := d.status.Connection
	d.mu.RUnlock()
	if conn != nil {
		t.Error("d.conn should be nil after StopAcquisition timeout")
	}
	if status != device.ConnectionError {
		t.Errorf("status should be Error after invalidate, got %v", status)
	}
}

// TestDSA3217SendCommand_InvalidatesConnectionOnWatchdogTrigger 验证 watchdog 触发后
// sendCommand 调用 invalidateConnection 清理连接状态：d.conn==nil 且 onError 被调用。
func TestDSA3217SendCommand_InvalidatesConnectionOnWatchdogTrigger(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()

	ignored := newDeadlineIgnoringConn(client)

	d := NewDSA3217(device.Profile{ID: "dsa-invalidate", Name: "DSA", Type: device.DeviceDSA3217})
	d.conn = ignored
	d.reader = bufio.NewReader(ignored)
	d.status.Connection = device.ConnectionConnected
	d.cmdTimeout = 200 * time.Millisecond

	var onErrorCalled int32
	d.SetOnError(func(err error) {
		atomic.StoreInt32(&onErrorCalled, 1)
	})

	_, err := d.sendCommand("TEST")
	if err == nil {
		t.Fatal("expected watchdog-triggered error, got nil")
	}
	if !strings.Contains(err.Error(), "watchdog triggered") {
		t.Errorf("error should mention 'watchdog triggered', got: %v", err)
	}

	// 验证 d.conn 已被 invalidateConnection 置 nil
	d.mu.RLock()
	conn := d.conn
	reader := d.reader
	status := d.status.Connection
	d.mu.RUnlock()
	if conn != nil {
		t.Error("d.conn should be nil after watchdog triggered invalidate")
	}
	if reader != nil {
		t.Error("d.reader should be nil after watchdog triggered invalidate")
	}
	if status != device.ConnectionError {
		t.Errorf("status should be Error, got %v", status)
	}
	if atomic.LoadInt32(&onErrorCalled) != 1 {
		t.Error("onError callback should be invoked after watchdog triggered invalidate")
	}
}

// TestDSA3217ReadLoop_InvalidatesConnOnTerminalReadError 验证 ADR-009 R1-3：
// terminal read error（EOF/RST/协议错误等）后 readLoop 必须走 invalidateConnection
// 统一毒化连接：清 d.conn/d.reader/d.stop + Error 状态 + Close + 调 onError。
//
// 修复前 bug：readLoop defer 在 unexpectedErr != nil 时仅把 Connection 从 Acquiring
// 恢复为 Connected，未清 conn/reader 也未 Close conn。EOF 后连接已死，下次 sendCommand
// 会通过 d.conn != nil 检查但实际 conn 已失效，导致 WSAECONNABORTED 等级联错误。
//
// 测试前置：
//   - net.Pipe 建立双向连接
//   - device.conn / reader 已设置，acquiring=true，启动 readLoop goroutine
//
// 测试步骤：
//   - 关闭 server 端模拟对端 EOF（client.Read 返回 io.EOF）
//
// 期待结果：
//   - d.conn == nil
//   - d.reader == nil
//   - status.Connection == Error
//   - status.LastError 非空
//   - onError 回调被调用并收到非 nil error
func TestDSA3217ReadLoop_InvalidatesConnOnTerminalReadError(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()

	d := NewDSA3217(device.Profile{ID: "dsa-eof", Name: "DSA", Type: device.DeviceDSA3217})
	d.conn = client
	d.reader = bufio.NewReader(client)
	d.status.Connection = device.ConnectionAcquiring
	d.acquiring = true
	d.scanning = true
	d.stop = make(chan struct{})
	d.readLoopDone = make(chan struct{})

	var onErrorCalled int32
	var onErrorErr error
	d.SetOnError(func(err error) {
		onErrorErr = err
		atomic.StoreInt32(&onErrorCalled, 1)
	})

	stop := d.stop
	go d.readLoop(stop)

	// 等待 readLoop 进入 ReadString 阻塞
	time.Sleep(100 * time.Millisecond)

	// 关闭 server 端模拟对端 EOF：client.Read 返回 io.EOF
	server.Close()

	// 等待 readLoop 退出并完成 invalidate
	select {
	case <-d.readLoopDone:
	case <-time.After(2 * time.Second):
		t.Fatal("readLoop did not exit within 2s after server EOF")
	}

	// 等待 onError 回调（invalidateConnection 锁外调用）
	select {
	case <-time.After(500 * time.Millisecond):
	}

	d.mu.RLock()
	conn := d.conn
	reader := d.reader
	status := d.status.Connection
	lastError := d.status.LastError
	acquiring := d.acquiring
	scanning := d.scanning
	d.mu.RUnlock()

	if conn != nil {
		t.Error("d.conn should be nil after terminal read error (invalidateConnection)")
	}
	if reader != nil {
		t.Error("d.reader should be nil after terminal read error (invalidateConnection)")
	}
	if status != device.ConnectionError {
		t.Errorf("status.Connection should be Error, got %v", status)
	}
	if lastError == "" {
		t.Error("status.LastError should be populated with terminal error cause")
	}
	if acquiring {
		t.Error("d.acquiring should be false after invalidate")
	}
	if scanning {
		t.Error("d.scanning should be false after invalidate")
	}
	if atomic.LoadInt32(&onErrorCalled) != 1 {
		t.Error("onError callback should be invoked exactly once after terminal read error")
	}
	if onErrorErr == nil {
		t.Error("onError should receive non-nil error")
	}
}

// TestDSA3217SendCommand_SoftTimeoutInvalidatesConn 验证 ADR-009 R0-12：
// 当 SetReadDeadline 正常兑现（soft deadline 先于 watchdog 触发）时，sendCommand
// 必须调 invalidateConnection 毒化连接，并返回 ErrWatchdogTriggered sentinel
// 让调用方通过 errors.Is 判定。
//
// 修复前 bug：soft deadline 兑现时 sendCommand 仅返回普通 timeout 错误，不清 conn、
// 不 Close conn。迟到响应随后进入 TCP 流被下一命令消费，导致协议错位（如 LIST S
// 的迟到响应被 SCAN 命令读取）。
//
// 测试前置：
//   - net.Pipe 建立双向连接（SetReadDeadline 在普通 Windows 环境下正常兑现）
//   - cmdSoftTimeout=50ms（soft deadline），cmdTimeout=200ms（watchdog）
//   - 服务端读掉命令但不发响应，确保 client.Read 在 soft deadline 到期后返回 timeout
//
// 测试步骤：
//   - 调用 sendCommand("TEST")
//
// 期待结果：
//   - 函数在 1s 预算内返回
//   - 错误通过 errors.Is(err, ErrWatchdogTriggered) 精确匹配 sentinel
//   - d.conn == nil
//   - d.reader == nil
//   - status.Connection == Error
//   - onError 被调用
//   - server 端写入迟到响应应失败（conn 已被 Close）
func TestDSA3217SendCommand_SoftTimeoutInvalidatesConn(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()

	d := NewDSA3217(device.Profile{ID: "dsa-soft-timeout", Name: "DSA", Type: device.DeviceDSA3217})
	d.conn = client
	d.reader = bufio.NewReader(client)
	d.status.Connection = device.ConnectionConnected
	// soft deadline 50ms 先于 watchdog 200ms 触发，验证 soft timeout 路径
	d.cmdSoftTimeout = 50 * time.Millisecond
	d.cmdTimeout = 200 * time.Millisecond

	var onErrorCalled int32
	d.SetOnError(func(err error) {
		atomic.StoreInt32(&onErrorCalled, 1)
	})

	// 服务端读掉命令但不发响应，让 client.Read 在 soft deadline 到期后返回 timeout
	serverDone := make(chan struct{})
	go func() {
		defer close(serverDone)
		buf := make([]byte, 64)
		_, _ = server.Read(buf)
		// 故意不写响应，触发 soft timeout
		time.Sleep(500 * time.Millisecond)
	}()

	started := time.Now()
	done := make(chan error, 1)
	go func() { done <- func() error { _, err := d.sendCommand("TEST"); return err }() }()

	select {
	case err := <-done:
		elapsed := time.Since(started)
		if err == nil {
			t.Fatal("expected soft timeout error, got nil")
		}
		if !errors.Is(err, sharedproto.ErrWatchdogTriggered) {
			t.Errorf("error must wrap ErrWatchdogTriggered for soft timeout, got: %v", err)
		}
		// soft deadline 50ms 触发，watchdog 200ms 未触发，总耗时应在 100ms 内
		if elapsed > 1*time.Second {
			t.Fatalf("sendCommand took too long: %v (soft deadline should trigger at ~50ms)", elapsed)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("sendCommand did not return within 2s budget")
	}

	// 验证 d.conn 已被 invalidateConnection 置 nil
	d.mu.RLock()
	conn := d.conn
	reader := d.reader
	status := d.status.Connection
	lastError := d.status.LastError
	d.mu.RUnlock()

	if conn != nil {
		t.Error("d.conn should be nil after soft timeout invalidate")
	}
	if reader != nil {
		t.Error("d.reader should be nil after soft timeout invalidate")
	}
	if status != device.ConnectionError {
		t.Errorf("status.Connection should be Error, got %v", status)
	}
	if lastError == "" {
		t.Error("status.LastError should be populated with soft timeout cause")
	}
	if atomic.LoadInt32(&onErrorCalled) != 1 {
		t.Error("onError callback should be invoked after soft timeout invalidate")
	}

	// 验证 conn 已被 Close：服务端写入迟到响应应失败
	_ = server.SetWriteDeadline(time.Now().Add(200 * time.Millisecond))
	if _, writeErr := server.Write([]byte("OK\r\n")); writeErr == nil {
		t.Error("expected server.Write to fail after client was closed by soft timeout invalidate")
	}

	<-serverDone
}
