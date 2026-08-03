package hardware

import (
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	sharedproto "shared.local/device-sdk/go/protocol"
	"wind-daq/services/api-go/internal/core/device"
)

func TestRunDAQP1604HandshakeTimesOutAndClosesConn(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()

	started := time.Now()
	err := runDAQP1604Handshake(client, 20*time.Millisecond, func() error {
		buf := make([]byte, 1)
		_, readErr := client.Read(buf)
		return readErr
	})

	if err == nil || !strings.Contains(err.Error(), "handshake timed out") {
		t.Fatalf("expected handshake timeout, got %v", err)
	}
	if elapsed := time.Since(started); elapsed > 200*time.Millisecond {
		t.Fatalf("handshake timeout took too long: %v", elapsed)
	}
}

func TestDAQP1604StopClosesConnWhenReadLoopDoesNotExit(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()

	d := NewDAQP1604(device.Profile{ID: "test-stop-stuck-reader", Type: device.DeviceDAQP1604})
	d.mu.Lock()
	d.conn = client
	d.frameReader = sharedproto.NewFrameReader(client)
	d.acquiring = true
	d.stop = make(chan struct{})
	d.readLoopDone = make(chan struct{})
	d.status.Connection = device.ConnectionAcquiring
	d.status.Acquiring = true
	d.mu.Unlock()

	err := d.StopAcquisition()
	if err == nil || !strings.Contains(err.Error(), "reconnect required") {
		t.Fatalf("StopAcquisition error = %v, want reconnect required", err)
	}

	_ = server.SetReadDeadline(time.Now().Add(200 * time.Millisecond))
	buf := make([]byte, 16)
	n, readErr := server.Read(buf)
	if n != 0 {
		t.Fatalf("received command after read-loop timeout: %q", string(buf[:n]))
	}
	if readErr == nil {
		t.Fatal("server read should fail after client connection is closed")
	}

	d.mu.RLock()
	conn := d.conn
	status := d.status.Connection
	d.mu.RUnlock()
	if conn != nil {
		t.Fatal("connection should be cleared after read-loop timeout")
	}
	if status != device.ConnectionError {
		t.Fatalf("connection status = %v, want Error", status)
	}
}

// TestDAQP1604_SyncUnitFromHardware_EOFReturnsError 验证：u01101 读到 io.EOF 时
// syncUnitFromHardware 必须返回 error（连接已死），不能当作软错误吞掉。
//
// 场景：设备拔网线后重连，TCP 拨号成功、w1601 发送成功，但 u01101 交换时
// 设备主动 FIN 关闭连接（对端 EOF）。若吞掉此错误，后续 StartAcquisition
// 的 c 00 命令会爆 WSAECONNABORTED。
//
// 模拟方式：net.Pipe 服务端读掉 u01101 命令后 sleep 让客户端进入 Read 阻塞，
// 再 Close 触发客户端 Read 返回 io.EOF。
//
// 时序要求：必须让客户端先进入 Read 阻塞再 Close。若 Close 先于 Read 调用，
// 客户端会返回 "use of closed network connection"——该错误由 IsClosedConnError
// 匹配，但 IsConnResetByPeer 并不匹配（IsConnResetByPeer 只覆盖对端 FIN/RST
// 硬证据，不覆盖本地主动 Close）。sleep 50ms 确保走 io.EOF 路径，断言更精确。
func TestDAQP1604_SyncUnitFromHardware_EOFReturnsError(t *testing.T) {
	server, client := net.Pipe()

	d := NewDAQP1604(device.Profile{
		ID:   "test-sync-eof",
		Type: device.DeviceDAQP1604,
		Channels: []device.ChannelConfig{
			{Index: 0, Name: "ch0", Enabled: true, Unit: "Pa"},
		},
	})
	d.mu.Lock()
	d.conn = client
	d.frameReader = sharedproto.NewFrameReader(client)
	d.mu.Unlock()

	// 服务端：读掉 u01101 → sleep → Close 触发客户端 Read 返回 io.EOF
	go func() {
		buf := make([]byte, 64)
		_, _ = server.Read(buf)
		time.Sleep(50 * time.Millisecond)
		_ = server.Close()
	}()

	err := d.syncUnitFromHardware(DAQ_P_1604_TIMEOUT)
	if err == nil {
		t.Fatal("syncUnitFromHardware should return error on EOF, got nil")
	}
	if !sharedproto.IsConnResetByPeer(err) {
		t.Errorf("returned error should be detected as conn reset, got: %v", err)
	}

	_ = client.Close()
}

// TestDAQP1604_SetUnit_V01101EOFTriggersOnError 验证：SetUnit 写 v01101 时
// 设备 FIN 关闭连接，应清理 driver 内部状态（conn/frameReader=nil, status=Error）
// 并触发 onError 回调通知 DeviceManager 从 map 中删除。
//
// 预期：
//   - SetUnit 返回 error
//   - d.conn == nil, d.frameReader == nil（driver 已清理）
//   - d.status.Connection == Error
//   - onError 回调被调用
func TestDAQP1604_SetUnit_V01101EOFTriggersOnError(t *testing.T) {
	server, client := net.Pipe()

	d := NewDAQP1604(device.Profile{
		ID:   "test-setunit-eof",
		Type: device.DeviceDAQP1604,
		Channels: []device.ChannelConfig{
			{Index: 0, Name: "ch0", Enabled: true, Unit: "Pa"},
		},
	})
	d.mu.Lock()
	d.conn = client
	d.frameReader = sharedproto.NewFrameReader(client)
	d.status.Connection = device.ConnectionConnected
	d.mu.Unlock()

	// 注册 onError 回调，记录调用次数
	var onErrorCalled int32
	d.SetOnError(func(err error) {
		atomic.StoreInt32(&onErrorCalled, 1)
	})

	// 服务端：读掉 v01101 → sleep → Close 触发客户端 Read 返回 io.EOF
	go func() {
		buf := make([]byte, 64)
		_, _ = server.Read(buf)
		time.Sleep(50 * time.Millisecond)
		_ = server.Close()
	}()

	// 单位从 Pa → psi，触发 v01101 写入路径
	err := d.SetUnit("psi")

	// 1. SetUnit 必须返回 error
	if err == nil {
		t.Fatal("SetUnit should return error on v01101 EOF, got nil")
	}
	// 2. driver 内部状态应被清理
	d.mu.RLock()
	conn := d.conn
	fr := d.frameReader
	status := d.status.Connection
	d.mu.RUnlock()
	if conn != nil {
		t.Error("d.conn should be nil after v01101 EOF cleanup")
	}
	if fr != nil {
		t.Error("d.frameReader should be nil after v01101 EOF cleanup")
	}
	if status != device.ConnectionError {
		t.Errorf("status should be Error, got %v", status)
	}
	// 3. onError 回调应被调用
	if atomic.LoadInt32(&onErrorCalled) != 1 {
		t.Error("onError callback should be invoked after v01101 EOF")
	}
}

// TestDAQP1604_SetUnit_V01101ErrorKeepsDriver 验证设备拒绝 v01101 时返回错误。
// 不触发 driver 清理，driver 保留在 map 中，前端可继续 Disconnect/重试。
//
// 模拟方式：服务端回复 N01 帧（设备拒绝，软错误）。
func TestDAQP1604_SetUnit_V01101ErrorKeepsDriver(t *testing.T) {
	server, client := net.Pipe()

	d := NewDAQP1604(device.Profile{
		ID:   "test-setunit-soft",
		Type: device.DeviceDAQP1604,
		Channels: []device.ChannelConfig{
			{Index: 0, Name: "ch0", Enabled: true, Unit: "Pa"},
		},
	})
	d.mu.Lock()
	d.conn = client
	d.frameReader = sharedproto.NewFrameReader(client)
	d.status.Connection = device.ConnectionConnected
	d.mu.Unlock()

	var onErrorCalled int32
	d.SetOnError(func(err error) {
		atomic.StoreInt32(&onErrorCalled, 1)
	})

	// 服务端：读掉 v01101 → 回复 N01 帧。
	go func() {
		buf := make([]byte, 64)
		_, _ = server.Read(buf)
		// 写一个长度前缀帧：2 字节大端长度 + payload "N01"
		_, _ = server.Write([]byte{0x00, 0x03, 'N', '0', '1'})
	}()

	err := d.SetUnit("psi")

	// 1. SetUnit 必须返回 error（设备拒绝）
	if err == nil {
		t.Fatal("SetUnit should return error on v01101 N01, got nil")
	}
	// 设备拒绝命令不代表 TCP 已断开，driver 必须保留。
	d.mu.RLock()
	conn := d.conn
	fr := d.frameReader
	status := d.status.Connection
	d.mu.RUnlock()
	if conn == nil {
		t.Error("d.conn should REMAIN after soft error (N01)")
	}
	if fr == nil {
		t.Error("d.frameReader should REMAIN after soft error (N01)")
	}
	if status != device.ConnectionConnected {
		t.Errorf("status should remain Connected after soft error, got %v", status)
	}
	// 3. onError 不应被调用
	if atomic.LoadInt32(&onErrorCalled) != 0 {
		t.Error("onError should NOT be invoked on soft error")
	}

	_ = server.Close()
	_ = client.Close()
}

// TestDAQP1604_SetUnit_OldConnFailurePreservesNewConn 验证 ADR-009 复核修订 finding 2：
// 旧 SetUnit 在 v01101 EOF 失败时调用 invalidateConnection(oldConn, ...)，必须比较
// expectedConn，仅当 d.conn 仍是 oldConn 时才清状态。若 d.conn 已被 Disconnect -> Connect
// 替换为 newConn，旧操作的 invalidation 不应误杀 newConn。
//
// 测试前置：
//   - 创建 oldClient/oldServer 模拟旧连接（v01101 会 EOF）
//   - 设备初始化 d.conn=oldClient, d.frameReader=NewFrameReader(oldClient), status=Connected
//   - 模拟"旧 SetUnit 进行中，同时 Disconnect -> Connect 已替换为新连接"：
//     在 SetUnit 调用前，手动将 d.conn 替换为 newClient，d.frameReader 替换为 NewFrameReader(newClient)
//   - 此时旧 SetUnit 仍持有 oldConn 引用（在 SetUnit 入口 d.mu.Lock 内捕获）
//
// 测试步骤：
//   - 调用 d.SetUnit("psi")：SetUnit 入口捕获 conn=oldClient（旧值），但 d.conn 已是 newClient
//   - 服务端 oldServer 读掉 v01101 命令后 Close 触发 oldClient Read 返回 io.EOF
//   - SetUnit 的 v01101 路径收到 EOF，调用 invalidateConnection(oldClient, ...)
//
// 期待结果：
//   - SetUnit 返回 error（v01101 EOF）
//   - d.conn 仍是 newClient（未被旧操作的 invalidation 误杀）
//   - d.frameReader 仍绑定 newClient（未被清空）
//   - status.Connection 仍是 Connected（未被置为 Error）
//   - onError 不被调用（invalidateConnection 检测到 d.conn != expectedConn，走 no-op 分支）
//
// 修复前：SetUnit 内联 d.mu.Lock + d.conn=nil + d.frameReader=nil + status=Error，
//
//	无条件清掉当前 d.conn（newClient），新连接被误杀。
//
// 修复后：SetUnit 调用 invalidateConnection(oldClient, ...)，比较 d.conn(newClient) != oldClient，
//
//	走 no-op 分支，仅 Close oldClient 不动 newClient。
func TestDAQP1604_SetUnit_OldConnFailurePreservesNewConn(t *testing.T) {
	oldServer, oldClient := net.Pipe()
	newServer, newClient := net.Pipe()
	defer oldServer.Close()
	defer newServer.Close()

	d := NewDAQP1604(device.Profile{
		ID:   "test-setunit-old-new",
		Type: device.DeviceDAQP1604,
		Channels: []device.ChannelConfig{
			{Index: 0, Name: "ch0", Enabled: true, Unit: "Pa"},
		},
	})

	// 步骤 1：初始化设备持有旧连接
	d.mu.Lock()
	d.conn = oldClient
	d.frameReader = sharedproto.NewFrameReader(oldClient)
	d.status.Connection = device.ConnectionConnected
	d.mu.Unlock()

	// 步骤 2：模拟"旧 SetUnit 进行中，Disconnect -> Connect 已替换为新连接"
	// 在 SetUnit 调用前手动替换 d.conn/d.frameReader 为 newClient
	// 注意：SetUnit 入口会先 RLock 捕获 conn 引用，所以这里替换后 SetUnit 拿到的是 newClient
	// 为了模拟"旧 SetUnit 已捕获 oldConn"，我们需要让 SetUnit 入口拿到 oldClient，
	// 但 d.conn 此时已是 newClient。
	// 由于 SetUnit 在 RLock 内捕获 conn 后释放锁，再调用 P1604WriteUnitCoefficient，
	// 我们需要在 SetUnit 释放 RLock 后、调用 P1604WriteUnitCoefficient 前替换 d.conn。
	// 这通过同步机制较难实现。简化方案：直接验证 invalidateConnection 的行为。
	//
	// 替代方案：直接调用 invalidateConnection 验证 expectedConn 比较逻辑。
	// 这与 SetUnit 内部调用路径一致，能完整覆盖 finding 2 的修复。
	d.mu.Lock()
	d.conn = newClient // 替换为新连接
	d.frameReader = sharedproto.NewFrameReader(newClient)
	d.status.Connection = device.ConnectionConnected
	d.mu.Unlock()

	var onErrorCalled int32
	d.SetOnError(func(err error) {
		atomic.StoreInt32(&onErrorCalled, 1)
	})

	// 直接调用 invalidateConnection(oldClient, ...)，模拟旧 SetUnit 的 invalidation 路径
	// d.conn 此时是 newClient，oldClient 是旧连接
	d.invalidateConnection(oldClient, "old setUnit v01101 EOF: simulated")

	// 期待结果：d.conn 仍是 newClient（未被误杀）
	d.mu.RLock()
	conn := d.conn
	fr := d.frameReader
	status := d.status.Connection
	d.mu.RUnlock()
	if conn != newClient {
		t.Fatal("d.conn should remain newClient after old SetUnit invalidation (expected-conn 比较必须保护新连接)")
	}
	if fr == nil {
		t.Fatal("d.frameReader should remain non-nil after old SetUnit invalidation")
	}
	if status != device.ConnectionConnected {
		t.Fatalf("status should remain Connected, got %v (旧操作的 invalidation 不应改状态)", status)
	}
	if atomic.LoadInt32(&onErrorCalled) != 0 {
		t.Fatal("onError should NOT be invoked when d.conn != expectedConn (no-op 分支)")
	}

	// oldClient 应被 Close（锁外关闭旧 conn）
	// 验证 oldClient 已关闭：Write 应返回 errClosed
	_ = oldClient.SetWriteDeadline(time.Now().Add(100 * time.Millisecond))
	_, err := oldClient.Write([]byte("test"))
	if err == nil {
		t.Error("oldClient should be closed by invalidateConnection (锁外关闭旧 conn)")
	}
}

// TestDAQP1604_SetUnit_RealInterleavingPreservesNewConn 验证 SetUnit 真实
// 交错场景：SetUnit 已捕获旧 conn 并阻塞在 P1604WriteUnitCoefficient 内的
// ReadFrame 时，Disconnect -> Connect 替换 d.conn 为新连接。SetUnit 返回
// 错误路径调用 invalidateConnection(oldConn, ...)，expectedConn 比较必须
// 保护新连接不被误杀。
//
// 与 TestDAQP1604_SetUnit_OldConnFailurePreservesNewConn 的差异：
//   - 前者直接调用 invalidateConnection 验证 helper 行为；
//   - 本测试驱动完整 SetUnit 路径，覆盖 RLock 捕获 → writeMu Lock →
//     P1604WriteUnitCoefficient → invalidateConnection 全链路。
//
// 测试前置：
//   - 两对 net.Pipe：oldServer/oldClient（设备原连接）、newServer/newClient（替换连接）
//   - 设备持有 oldClient 作为 d.conn
//   - onError 回调用 atomic 计数
//
// 测试步骤：
//   - goroutine A 调用 d.SetUnit("Pa")，进入 P1604WriteUnitCoefficient 后阻塞在 ReadFrame
//   - 主线程 sleep 100ms 确保 goroutine A 已进入阻塞
//   - 主线程替换 d.conn = newClient, d.frameReader = newFrameReader（模拟 Disconnect -> Connect）
//   - 主线程 close oldServer → oldClient.Read 返回 EOF → P1604WriteUnitCoefficient 返回错误
//   - goroutine A 的 SetUnit 检测 IsConnResetByPeer(EOF)=true，调用 invalidateConnection(oldClient, ...)
//   - invalidateConnection 比较发现 d.conn(newClient) != expectedConn(oldClient)，仅 Close oldClient
//
// 期待结果：
//   - d.conn 仍是 newClient（未被误杀）
//   - d.frameReader 仍指向 newFrameReader
//   - status.Connection 仍是 Connected（未被覆盖为 Error）
//   - onError 未被调用（expected-conn 不匹配，走 no-op 分支）
//   - SetUnit 返回错误（包含 "write hardware unit coefficient"）
func TestDAQP1604_SetUnit_RealInterleavingPreservesNewConn(t *testing.T) {
	oldServer, oldClient := net.Pipe()
	newServer, newClient := net.Pipe()
	defer oldServer.Close()
	defer newServer.Close()
	// oldClient 由 invalidateConnection 锁外 Close
	// newClient 由 defer newServer.Close() 间接关闭（net.Pipe 双向关闭）

	d := NewDAQP1604(device.Profile{
		ID:   "test-setunit-interleave",
		Type: device.DeviceDAQP1604,
		Channels: []device.ChannelConfig{
			{Index: 0, Name: "ch0", Enabled: true, Unit: "Pa"},
		},
	})

	// 步骤 1：设备持有 oldClient
	d.mu.Lock()
	d.conn = oldClient
	d.frameReader = sharedproto.NewFrameReader(oldClient)
	d.status.Connection = device.ConnectionConnected
	d.mu.Unlock()

	var onErrorCalled int32
	d.SetOnError(func(err error) {
		atomic.StoreInt32(&onErrorCalled, 1)
	})

	// 步骤 2：goroutine A 调用 SetUnit，预期会阻塞在 P1604WriteUnitCoefficient 的 ReadFrame
	setUnitDone := make(chan error, 1)
	go func() {
		setUnitDone <- d.SetUnit("Pa")
	}()

	// 等待 goroutine A 进入 P1604WriteUnitCoefficient 阻塞：
	// server 端读到 v01101 命令后 client 等 ReadFrame 永远阻塞（因 server 不写响应）。
	// 100ms 足够 net.Pipe 同步 Write 完成。
	time.Sleep(100 * time.Millisecond)

	// 步骤 3：模拟 Disconnect -> Connect 替换 d.conn 为 newClient
	d.mu.Lock()
	d.conn = newClient
	d.frameReader = sharedproto.NewFrameReader(newClient)
	// status 保持 Connected（Connect 后状态）
	d.mu.Unlock()

	// 步骤 4：关闭 oldServer 触发 oldClient.Read 返回 EOF
	// P1604WriteUnitCoefficient 的 ReadFrame 收到 EOF，wrapP1604IOError 包装错误返回。
	// SetUnit 检测 IsConnResetByPeer(EOF)=true，调用 invalidateConnection(oldClient, ...)。
	oldServer.Close()

	// 步骤 5：等待 SetUnit 返回
	select {
	case err := <-setUnitDone:
		if err == nil {
			t.Fatal("SetUnit should return error after old conn EOF")
		}
		if !strings.Contains(err.Error(), "write hardware unit coefficient") {
			t.Errorf("SetUnit error = %v, want contains 'write hardware unit coefficient'", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("SetUnit did not return within 5s budget; EOF did not propagate")
	}

	// 期待结果：新连接未被误杀
	d.mu.RLock()
	conn := d.conn
	fr := d.frameReader
	status := d.status.Connection
	d.mu.RUnlock()

	if conn != newClient {
		t.Fatalf("d.conn should remain newClient after SetUnit interleaving, got %v", conn)
	}
	if fr == nil {
		t.Fatal("d.frameReader should remain non-nil (new frameReader)")
	}
	if status != device.ConnectionConnected {
		t.Errorf("status should remain Connected (expected-conn no-op), got %v", status)
	}
	if atomic.LoadInt32(&onErrorCalled) != 0 {
		t.Error("onError should NOT be invoked when d.conn != expectedConn (no-op branch)")
	}
}

// writeBlockingConn 模拟 Windows 故障环境下 SetWriteDeadline 失效且 Write 永久阻塞的连接。
//
// 设计依据 ADR-009：socket deadline 不能作为有界硬件 I/O 的唯一取消机制。
// 本替身覆盖 SetWriteDeadline 为 no-op，并将 Write 阻塞至 Close 被调用，
// 用于验证 sendCommandACK Write 阶段的外部 watchdog 能在 deadline 失效时
// 强制 Close conn 解除阻塞。
//
// 与 daq_p1064pre_test.go:deadlineIgnoringConn 的差异：
//   - deadlineIgnoringConn 仅忽略 SetReadDeadline，用于 Read 阶段 watchdog 测试
//   - writeBlockingConn 忽略 SetWriteDeadline 且 Write 永久阻塞，用于 Write 阶段 watchdog 测试
type writeBlockingConn struct {
	net.Conn
	closed chan struct{}
	once   sync.Once
}

func newWriteBlockingConn(inner net.Conn) *writeBlockingConn {
	return &writeBlockingConn{
		Conn:   inner,
		closed: make(chan struct{}),
	}
}

// SetWriteDeadline 覆盖为 no-op，模拟 Windows 故障环境下 write deadline 失效。
func (c *writeBlockingConn) SetWriteDeadline(t time.Time) error { return nil }

// Write 阻塞直到 Close 被调用，模拟内核层 Write 永久阻塞。
// Close 后返回 net.ErrClosed，与正常 conn Close 后的 Write 错误一致。
func (c *writeBlockingConn) Write(p []byte) (int, error) {
	<-c.closed
	return 0, net.ErrClosed
}

// Close 关闭连接并解除 Write 阻塞。幂等：多次调用安全。
func (c *writeBlockingConn) Close() error {
	c.once.Do(func() { close(c.closed) })
	return c.Conn.Close()
}

// TestDAQP1604SendCommandACK_WatchdogTriggersOnWriteDeadlineIgnoringConn 验证（P1-2.a）：
// 当 Write 阶段 conn 忽略 SetWriteDeadline 且 Write 永久阻塞时，sendCommandACK
// 的外部 watchdog 在预算内触发 Close conn 解除阻塞，返回的错误含 "watchdog triggered"。
//
// 修复前（P1-2.a）：sendCommandACK 委托 sendCommand 仅设 SetWriteDeadline，无 watchdog。
// Write 永久阻塞时 sendCommandACK 永远不返回（生产环境卡死）。
// 修复后：sendCommandACK 在 Write 前启 WatchdogClose，超时强制 Close conn，
// Write 返回错误，sendCommandACK 检测到 watchdog 触发并在预算内返回。
func TestDAQP1604SendCommandACK_WatchdogTriggersOnWriteDeadlineIgnoringConn(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()

	blocked := newWriteBlockingConn(client)

	d := NewDAQP1604(device.Profile{ID: "p1604-write-watchdog", Type: device.DeviceDAQP1604})
	d.mu.Lock()
	d.conn = blocked
	d.frameReader = sharedproto.NewFrameReader(blocked)
	d.status.Connection = device.ConnectionConnected
	// 测试用 200ms watchdog 加速，避免等待生产默认 5s。
	d.cmdTimeout = 200 * time.Millisecond
	d.mu.Unlock()

	started := time.Now()
	err := d.sendCommandACK("c 01 1")
	elapsed := time.Since(started)

	if err == nil {
		t.Fatal("expected watchdog-triggered error, got nil")
	}
	if !strings.Contains(err.Error(), "watchdog triggered") {
		t.Errorf("error should mention 'watchdog triggered', got: %v", err)
	}
	// 预算 2s 足够覆盖 200ms watchdog + Write 在 conn Close 后返回的延迟。
	if elapsed > 2*time.Second {
		t.Fatalf("sendCommandACK took too long: %v (watchdog should have triggered at ~200ms)", elapsed)
	}
}

// TestDAQP1604SendCommandACK_InvalidatesConnectionOnWatchdogTrigger 验证（P1-3.d）：
// sendCommandACK 的 watchdog 触发后，调用 invalidateConnection 清理连接状态：
// d.conn==nil, d.frameReader==nil, status==Error, onError 被调用。
//
// 修复前（P1-3.d）：sendCommandACK 失败路径仅 fmt.Errorf 包装返回，未清理 d.conn，
// 下次 sendCommand/sendCommandACK 通过 nil 检查但操作失败。
// 修复后：watchdog 触发时调 invalidateConnection，置 nil + 调 onError，
// 与 invalidateConnectionAfterReadLoopTimeout 标杆行为一致。
func TestDAQP1604SendCommandACK_InvalidatesConnectionOnWatchdogTrigger(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()

	blocked := newWriteBlockingConn(client)

	d := NewDAQP1604(device.Profile{ID: "p1604-invalidate", Type: device.DeviceDAQP1604})
	d.mu.Lock()
	d.conn = blocked
	d.frameReader = sharedproto.NewFrameReader(blocked)
	d.status.Connection = device.ConnectionConnected
	d.cmdTimeout = 200 * time.Millisecond
	d.mu.Unlock()

	var onErrorCalled int32
	d.SetOnError(func(err error) {
		atomic.StoreInt32(&onErrorCalled, 1)
	})

	err := d.sendCommandACK("c 01 1")
	if err == nil {
		t.Fatal("expected watchdog-triggered error, got nil")
	}
	if !strings.Contains(err.Error(), "watchdog triggered") {
		t.Errorf("error should mention 'watchdog triggered', got: %v", err)
	}

	// 验证 driver 内部状态已被 invalidateConnection 清理
	d.mu.RLock()
	conn := d.conn
	fr := d.frameReader
	status := d.status.Connection
	d.mu.RUnlock()
	if conn != nil {
		t.Error("d.conn should be nil after watchdog triggered invalidate")
	}
	if fr != nil {
		t.Error("d.frameReader should be nil after watchdog triggered invalidate")
	}
	if status != device.ConnectionError {
		t.Errorf("status should be Error, got %v", status)
	}
	if atomic.LoadInt32(&onErrorCalled) != 1 {
		t.Error("onError callback should be invoked after watchdog triggered invalidate")
	}
}

// TestDAQP1604SendCommandACK_PreservesReadWatchdog 验证（P1-2.a 回归保护）：
// sendCommandACK 在 Write 完成后停止外部 watchdog，由 P1604ReadCommandACK 内嵌
// watchdog 接管 Read 阶段。本测试用 deadlineIgnoringConn（忽略 SetReadDeadline）
// 让 Read 永久阻塞，验证 P1604ReadCommandACK 内嵌 watchdog 仍能触发，未被外部
// watchdog 改造破坏。
//
// 修复前（P1-2.a 修复时易引入的回归）：若外部 watchdog 覆盖 Read 阶段且未正确停止，
// 可能与 P1604ReadCommandACK 内嵌 watchdog 双重计时或相互干扰。
// 修复后：Write 完成后立即 stop 外部 watchdog，P1604ReadCommandACK 独立保护 Read。
func TestDAQP1604SendCommandACK_PreservesReadWatchdog(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()

	// deadlineIgnoringConn 复用 daq_p1064pre_test.go 中的同款替身（包内共享）：
	// 忽略 SetReadDeadline，模拟 Windows 故障环境下 Read 永久阻塞。
	ignored := newDeadlineIgnoringConn(client)

	d := NewDAQP1604(device.Profile{ID: "p1604-read-watchdog", Type: device.DeviceDAQP1604})
	d.mu.Lock()
	d.conn = ignored
	d.frameReader = sharedproto.NewFrameReader(ignored)
	d.status.Connection = device.ConnectionConnected
	d.cmdTimeout = 200 * time.Millisecond
	d.mu.Unlock()

	// 服务端：读掉命令但不发 ACK，让客户端 Read 阶段阻塞。
	// 即便服务端不读，net.Pipe 缓冲区足以吸收 6 字节命令，客户端 Write 仍能立即返回。
	// 此处读掉仅为避免 server 端 buffer 残留影响断言。
	go func() {
		buf := make([]byte, 64)
		_, _ = server.Read(buf)
		// 不写 ACK，客户端 P1604ReadCommandACK 阻塞等 ACK → 触发内嵌 watchdog
	}()

	started := time.Now()
	err := d.sendCommandACK("c 01 1")
	elapsed := time.Since(started)

	if err == nil {
		t.Fatal("expected watchdog-triggered error from P1604ReadCommandACK, got nil")
	}
	if !strings.Contains(err.Error(), "watchdog triggered") {
		t.Errorf("error should mention 'watchdog triggered' from P1604ReadCommandACK internal watchdog, got: %v", err)
	}
	// 预算 2s 足够覆盖 200ms watchdog + Read 在 conn Close 后返回的延迟。
	if elapsed > 2*time.Second {
		t.Fatalf("sendCommandACK took too long: %v (Read watchdog should have triggered at ~200ms)", elapsed)
	}
}

// writeP1604ACKFrame 写入一个带 2 字节大端长度前缀的 P1604 ACK 帧。
//
// 帧格式：[uint16 big-endian total length][payload]
// 例如 "A" → [0x00, 0x03, 'A']（total=3，含 2 字节前缀）。
//
// 用于 R0-5 验收测试：模拟设备对 c 00/c 05/c 01/v01101 命令的 ACK 响应。
// 静默忽略 Write 错误：client 关闭后 Write 失败是预期行为，避免在子 goroutine
// 内调 t.Logf 触发 race（ADR-009 测试要求 -race 全绿）。
func writeP1604ACKFrame(conn net.Conn, payload string) {
	totalLen := uint16(len(payload) + 2)
	buf := make([]byte, 2, int(totalLen))
	buf[0] = byte(totalLen >> 8)
	buf[1] = byte(totalLen)
	buf = append(buf, []byte(payload)...)
	_, _ = conn.Write(buf)
}

// TestDAQP1604StartAcquisition_DoesNotCloseHealthyConnWhenNoData 验证 ADR-009 R0-5：
// StartAcquisition 在 TCP 缓冲区无残留数据时不得通过 drain 关闭健康连接。
//
// 历史背景：原实现 StartAcquisition 在 sendCommandACK 之前调用
// sharedproto.DrainConnection(conn, 100ms) 排空缓冲区。在故障 Windows 环境下
// SetReadDeadline 失效，DrainConnection 的阻塞 Read 永不返回，watchdog 在 400ms 后
// 强制 Close conn——但空缓冲是正常状态，watchdog 到期只能证明探测无法完成，
// 不能证明物理连接故障，违反 ADR-009 决策 8。
//
// 测试前置：
//   - net.Pipe 建立双向连接
//   - client 包装为 deadlineIgnoringConn（SetReadDeadline/SetWriteDeadline 失效）
//   - server 同步响应每条命令的 ACK
//
// 测试步骤：
//   - 调用 device.StartAcquisition()
//
// 期待结果（整改后）：
//   - StartAcquisition 返回 nil（命令链成功完成）
//   - d.conn 仍非 nil（未被 drain watchdog 关闭）
//   - status.Connection == ConnectionAcquiring
//
// 期待结果（整改前——本测试应失败）：
//   - DrainConnection 在 sendCommandACK 之前触发 watchdog Close conn
//   - StartAcquisition 返回 "drain residual data: ... watchdog triggered" 错误
//   - d.conn 为 nil
func TestDAQP1604StartAcquisition_DoesNotCloseHealthyConnWhenNoData(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()

	d := NewDAQP1604(device.Profile{
		ID:   "test-start-healthy",
		Type: device.DeviceDAQP1604,
		Channels: []device.ChannelConfig{
			{Index: 0, Name: "ch0", Enabled: true, Unit: "Pa"},
		},
		SamplingRate: 20,
	})

	// deadlineIgnoringConn 模拟 Windows 故障环境：SetReadDeadline/SetWriteDeadline 失效。
	// 整改前的 DrainConnection 会因此 watchdog 触发关闭 conn。
	ignored := newDeadlineIgnoringConn(client)
	d.mu.Lock()
	d.conn = ignored
	d.frameReader = sharedproto.NewFrameReader(ignored)
	d.status.Connection = device.ConnectionConnected
	d.mu.Unlock()

	// 服务端：同步读取每条命令并回复 "A" ACK 帧。
	// 命令序列：c 00 ... (initStream) → c 05 ... (initStream) → c 01 1 (start stream)
	// net.Pipe 无缓冲，每次 Write-Read 同步配对，server.Read 收到一条命令后立即写 ACK。
	serverDone := make(chan struct{})
	go func() {
		defer close(serverDone)
		buf := make([]byte, 256)
		for i := 0; i < 3; i++ { // 期望 3 条命令
			_ = server.SetReadDeadline(time.Now().Add(2 * time.Second))
			n, err := server.Read(buf)
			if err != nil {
				return
			}
			_ = n // 调试用：可观察收到的命令字节数
			// 回复 "A" ACK 帧
			_ = server.SetWriteDeadline(time.Now().Add(500 * time.Millisecond))
			writeP1604ACKFrame(server, "A")
		}
	}()

	err := d.StartAcquisition()
	if err != nil {
		t.Fatalf("StartAcquisition should succeed on healthy conn with no buffer data, got: %v", err)
	}

	// 验证 conn 仍可用且状态正确
	d.mu.RLock()
	conn := d.conn
	status := d.status.Connection
	d.mu.RUnlock()
	if conn == nil {
		t.Fatal("d.conn should not be nil after successful StartAcquisition (drain should not have closed it)")
	}
	if status != device.ConnectionAcquiring {
		t.Fatalf("status should be Acquiring after successful StartAcquisition, got %v", status)
	}

	// 清理：停止采集。StopAcquisition 会再发 c 02 1 命令，需要 server 响应 ACK。
	// server goroutine 已退出（3 条命令已响应完），重启一个 goroutine 响应 c 02 1。
	go func() {
		buf := make([]byte, 64)
		_ = server.SetReadDeadline(time.Now().Add(2 * time.Second))
		_, err := server.Read(buf)
		if err == nil {
			_ = server.SetWriteDeadline(time.Now().Add(500 * time.Millisecond))
			writeP1604ACKFrame(server, "A")
		}
	}()

	_ = d.StopAcquisition()

	// 等 server goroutine 退出，避免 defer close 后 goroutine 访问 server 触发 race。
	_ = client.Close()
	select {
	case <-serverDone:
	case <-time.After(500 * time.Millisecond):
	}
}

// TestDAQP1604SetUnit_DoesNotCloseHealthyConnWhenNoData 验证 ADR-009 R0-5：
// SetUnit 在 TCP 缓冲区无残留数据时不得通过 drain 关闭健康连接。
//
// 历史背景：原实现 SetUnit 在 P1604WriteUnitCoefficient 之前调用
// sharedproto.DrainConnection(conn, 100ms) 排空缓冲区。在故障 Windows 环境下
// SetReadDeadline 失效，DrainConnection 的 watchdog 会关闭健康连接。
//
// 测试前置：
//   - net.Pipe 建立双向连接
//   - client 包装为 deadlineIgnoringConn（deadline 失效）
//   - server 同步响应 v01101 命令的 ACK
//
// 测试步骤：
//   - 调用 device.SetUnit("psi")
//
// 期待结果（整改后）：
//   - SetUnit 返回 nil（v01101 写入成功）
//   - d.conn 仍非 nil（未被 drain watchdog 关闭）
//   - status.Connection == ConnectionConnected
//
// 期待结果（整改前——本测试应失败）：
//   - DrainConnection 在 P1604WriteUnitCoefficient 之前触发 watchdog Close conn
//   - SetUnit 返回 "drain residual data before SetUnit: ... watchdog triggered" 错误
func TestDAQP1604SetUnit_DoesNotCloseHealthyConnWhenNoData(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()

	d := NewDAQP1604(device.Profile{
		ID:   "test-setunit-healthy",
		Type: device.DeviceDAQP1604,
		Channels: []device.ChannelConfig{
			{Index: 0, Name: "ch0", Enabled: true, Unit: "Pa"},
		},
	})

	// deadlineIgnoringConn 模拟 Windows 故障环境：deadline 失效。
	// 整改前的 DrainConnection 会因此 watchdog 触发关闭 conn。
	ignored := newDeadlineIgnoringConn(client)
	d.mu.Lock()
	d.conn = ignored
	d.frameReader = sharedproto.NewFrameReader(ignored)
	d.status.Connection = device.ConnectionConnected
	d.mu.Unlock()

	// 服务端：读掉 v01101 命令后回复 "A" ACK 帧。
	serverDone := make(chan struct{})
	go func() {
		defer close(serverDone)
		buf := make([]byte, 64)
		_ = server.SetReadDeadline(time.Now().Add(2 * time.Second))
		_, err := server.Read(buf)
		if err != nil {
			return
		}
		_ = server.SetWriteDeadline(time.Now().Add(500 * time.Millisecond))
		writeP1604ACKFrame(server, "A")
	}()

	err := d.SetUnit("psi")
	if err != nil {
		t.Fatalf("SetUnit should succeed on healthy conn with no buffer data, got: %v", err)
	}

	// 验证 conn 仍可用且状态正确
	d.mu.RLock()
	conn := d.conn
	status := d.status.Connection
	d.mu.RUnlock()
	if conn == nil {
		t.Fatal("d.conn should not be nil after successful SetUnit (drain should not have closed it)")
	}
	if status != device.ConnectionConnected {
		t.Fatalf("status should remain Connected after successful SetUnit, got %v", status)
	}

	// 验证连接仍可读写：server 写 probe 数据，client 应能读到
	readCh := make(chan struct {
		data string
		err  error
	}, 1)
	go func() {
		_ = client.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
		buf := make([]byte, 16)
		n, err := client.Read(buf)
		readCh <- struct {
			data string
			err  error
		}{string(buf[:n]), err}
	}()

	// 短暂等待确保 client.Read goroutine 已启动
	time.Sleep(20 * time.Millisecond)

	_ = server.SetWriteDeadline(time.Now().Add(500 * time.Millisecond))
	if _, err := server.Write([]byte("alive")); err != nil {
		t.Fatalf("server.Write failed after SetUnit, conn was killed: %v", err)
	}

	select {
	case r := <-readCh:
		if r.err != nil {
			t.Fatalf("client.Read failed after SetUnit, conn was killed: %v", r.err)
		}
		if r.data != "alive" {
			t.Fatalf("client.Read got %q, want %q", r.data, "alive")
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("client.Read did not complete within 500ms")
	}

	_ = client.Close()
	select {
	case <-serverDone:
	case <-time.After(500 * time.Millisecond):
	}
}

// TestDAQP1604ReadLoop_InvalidatesConnOnTerminalReadError 验证 ADR-009 R0-11：
// readLoop 在收到 terminal read error（EOF/RST/协议错误）后必须统一毒化连接——
// 清空 d.conn/d.frameReader、置 Error 状态、保存 LastError、close conn、
// 通知 onError 回调。
//
// 历史背景：原实现 readLoop defer 在 unexpectedErr != nil 时虽设置 status=Error，
// 但未清空 d.conn/d.frameReader，也未 close conn。EOF 后连接已死，下次
// StartAcquisition 会用旧 conn 发命令爆 WSAECONNABORTED。
//
// 测试前置：
//   - net.Pipe 建立双向连接
//   - d.conn / frameReader 已设置，acquiring=true
//   - 启动 readLoop goroutine
//
// 测试步骤：
//   - 关闭 server 端模拟对端 EOF（client.Read 返回 io.EOF）
//   - 等待 readLoopDone 关闭
//
// 期待结果：
//   - d.conn 被置为 nil
//   - d.frameReader 被置为 nil
//   - d.status.Connection = ConnectionError
//   - d.status.LastError 非空
//   - conn 已被 Close（server.Write 失败）
//   - onError 回调被调用并收到非 nil error
func TestDAQP1604ReadLoop_InvalidatesConnOnTerminalReadError(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()
	// 注意：client 由 readLoop defer -> invalidateConnection 关闭，
	// 这里不 defer client.Close() 以避免重复 Close 报错。

	d := NewDAQP1604(device.Profile{
		ID:   "p1604-eof",
		Type: device.DeviceDAQP1604,
		Channels: []device.ChannelConfig{
			{Index: 0, Name: "ch0", Enabled: true, Unit: "Pa"},
		},
	})
	d.mu.Lock()
	d.conn = client
	d.frameReader = sharedproto.NewFrameReader(client)
	d.acquiring = true
	d.stop = make(chan struct{})
	d.readLoopDone = make(chan struct{})
	d.status.Connection = device.ConnectionAcquiring
	d.status.Acquiring = true
	d.mu.Unlock()

	var onErrorCalled int32
	var capturedErr error
	var errMu sync.Mutex
	d.SetOnError(func(err error) {
		atomic.StoreInt32(&onErrorCalled, 1)
		errMu.Lock()
		capturedErr = err
		errMu.Unlock()
	})

	// 启动 readLoop（直接调用未导出方法，仅在包内测试可用）
	stop := d.stop
	go d.readLoop(stop)

	// 等待 readLoop 进入 ReadFrame 阻塞
	time.Sleep(50 * time.Millisecond)

	// 关闭 server 模拟对端 EOF：client.Read 返回 io.EOF
	server.Close()

	// 等待 readLoop 退出
	d.mu.RLock()
	done := d.readLoopDone
	d.mu.RUnlock()
	if done != nil {
		select {
		case <-done:
		case <-time.After(3 * time.Second):
			t.Fatal("readLoop did not exit within 3s after server EOF")
		}
	}

	// 短暂等待 defer 完成状态清理
	time.Sleep(100 * time.Millisecond)

	// 验证状态已被统一毒化
	d.mu.RLock()
	connAfter := d.conn
	frameReaderAfter := d.frameReader
	statusAfter := d.status.Connection
	lastError := d.status.LastError
	d.mu.RUnlock()
	if connAfter != nil {
		t.Error("d.conn should be nil after terminal read error")
	}
	if frameReaderAfter != nil {
		t.Error("d.frameReader should be nil after terminal read error")
	}
	if statusAfter != device.ConnectionError {
		t.Errorf("status.Connection = %v, want Error", statusAfter)
	}
	if lastError == "" {
		t.Error("status.LastError should be non-empty after terminal read error")
	}
	if atomic.LoadInt32(&onErrorCalled) != 1 {
		t.Error("onError callback should be invoked")
	}
	errMu.Lock()
	if capturedErr == nil {
		t.Error("onError callback should receive non-nil error")
	}
	errMu.Unlock()
}

// TestDAQP1604ReadLoop_InvalidatesConnOnNoDataTimeout 验证 ADR-009 R0-10：
// readLoop 入口启动的独立 no-data timer 在 deadline 失效（Read 永久阻塞）且
// 无任何数据到达时，必须在 noDataTimeout 到期后独立触发连接毒化——
// 清空 d.conn/d.frameReader、置 Error 状态、close conn。
//
// 关键差异（与 terminal read error 测试的对比）：
//   - terminal read error：对端 EOF → Read 返回错误 → defer invalidate
//   - no-data timer：无数据 → timer 到期 → Close conn → Read 返回 closed 错误
//     → readLoop 走 unexpectedErr 路径调用 invalidate（或检查 conn==nil 提前 return）
//
// timer 必须独立于 readLoop 循环体执行。本测试用 deadlineIgnoringConn 让 Read
// 永久阻塞（模拟 Windows 故障环境下 deadline 失效），循环体不可达，
// 仅靠 timer 到期能触发毒化。若 timer 未启动或依赖循环体，测试会超时。
//
// 测试前置：
//   - net.Pipe 建立双向连接，client 端包 deadlineIgnoringConn（Read 永久阻塞）
//   - d.conn / frameReader / readLoopDone 已设置，acquiring=true
//   - noDataTimeout 临时覆盖为 200ms（生产默认 10s）
//   - server 端不写任何数据，让 client Read 永久阻塞
//
// 测试步骤：
//   - 启动 readLoop goroutine
//   - 等待 noDataTimeout(200ms) + 余量(800ms) = 1s 预算
//
// 期待结果：
//   - d.conn 被置为 nil（timer 回调 + 可能的 invalidate 双重设置）
//   - d.frameReader 被置为 nil
//   - d.status.Connection = ConnectionError
//   - d.status.LastError 非空（timer 设 "no data" 或 invalidate 覆盖为 closed err）
//   - readLoopDone 已关闭
//
// 注意：LastError 具体内容取决于时序——
//   - 若 readLoop 在 timer Close conn 后通过 conn==nil 检查提前 return，
//     LastError 保持 timer 设置的 "no data received for ..."
//   - 若 readLoop 的 ReadFrame 先返回 closed error，defer invalidate 覆盖
//     LastError 为 closed error 消息
//
// 两种路径都符合 R0-10 要求，测试只验证非空。
func TestDAQP1604ReadLoop_InvalidatesConnOnNoDataTimeout(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()

	d := NewDAQP1604(device.Profile{
		ID:   "p1604-nodata",
		Type: device.DeviceDAQP1604,
		Channels: []device.ChannelConfig{
			{Index: 0, Name: "ch0", Enabled: true, Unit: "Pa"},
		},
	})

	// 临时覆盖 noDataTimeout 为 200ms 加速测试。
	// 同一包内测试默认串行执行，覆盖安全；t.Cleanup 确保恢复。
	// ADR-009 finding 4：使用 atomic 包装的 helper 函数，避免 readLoop 跨测试边界读取
	// 全局 noDataTimeout 与测试修改并发触发 data race。
	origTimeout := getNoDataTimeout()
	setNoDataTimeout(200 * time.Millisecond)
	t.Cleanup(func() { setNoDataTimeout(origTimeout) })

	d.mu.Lock()
	d.conn = newDeadlineIgnoringConn(client)
	d.frameReader = sharedproto.NewFrameReader(d.conn)
	d.acquiring = true
	d.stop = make(chan struct{})
	d.readLoopDone = make(chan struct{})
	d.status.Connection = device.ConnectionAcquiring
	d.status.Acquiring = true
	d.mu.Unlock()

	stop := d.stop
	go d.readLoop(stop)

	// 预算 1s：覆盖 200ms noDataTimer + Read 在 conn Close 后返回的延迟 + defer 清理。
	// 若 timer 未触发，readLoop 永久阻塞，select 会超时 fatal。
	done := d.readLoopDone
	select {
	case <-done:
	case <-time.After(1 * time.Second):
		t.Fatal("readLoop did not exit within 1s; no-data timer may not have fired")
	}

	// 短暂等待 defer 完成状态清理
	time.Sleep(100 * time.Millisecond)

	d.mu.RLock()
	connAfter := d.conn
	frameReaderAfter := d.frameReader
	statusAfter := d.status.Connection
	lastError := d.status.LastError
	d.mu.RUnlock()
	if connAfter != nil {
		t.Error("d.conn should be nil after no-data timer fired")
	}
	if frameReaderAfter != nil {
		t.Error("d.frameReader should be nil after no-data timer fired")
	}
	if statusAfter != device.ConnectionError {
		t.Errorf("status.Connection = %v, want Error", statusAfter)
	}
	if lastError == "" {
		t.Error("status.LastError should be non-empty after no-data timer fired")
	}
}
