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

// deadlineIgnoringConn 模拟 Windows 故障环境下 SetReadDeadline/SetWriteDeadline 失效的连接。
//
// 设计依据 ADR-009：必须包含忽略 deadline、仅在 Close 后才返回的连接替身，
// 用于验证 close(stop) 无法解除内核 Read 阻塞时，Stop/Disconnect 能通过 Close 兜底返回。
// 同时统计并发 Read 数，用于检测"双 reader 同时 Read conn"的数据错位 bug。
//
// 覆盖 SetWriteDeadline 为 no-op 的原因：sendStartAcquisitionLocked 同时设置
// 5s Write deadline 和 5s watchdog，若 SetWriteDeadline 生效会与 watchdog 产生竞态，
// 导致 watchdog 触发测试不稳定。屏蔽后 Write 永久阻塞，只有 watchdog Close conn 能解除，
// 测试可稳定验证 watchdog 触发路径。
type deadlineIgnoringConn struct {
	net.Conn
	closed    chan struct{}
	once      sync.Once
	active    int32 // 当前并发 Read 数
	maxActive int32 // 历史最大并发 Read 数
}

func newDeadlineIgnoringConn(inner net.Conn) *deadlineIgnoringConn {
	return &deadlineIgnoringConn{
		Conn:   inner,
		closed: make(chan struct{}),
	}
}

// SetReadDeadline 覆盖为 no-op，模拟 Windows 故障环境下 deadline 失效。
func (c *deadlineIgnoringConn) SetReadDeadline(t time.Time) error {
	return nil
}

// SetWriteDeadline 覆盖为 no-op，模拟 Windows 故障环境下 Write deadline 失效。
// 确保被测代码的 Write 只能由 watchdog Close conn 解除阻塞，避免 deadline 与 watchdog 竞态。
func (c *deadlineIgnoringConn) SetWriteDeadline(t time.Time) error {
	return nil
}

// Read 计数并发调用，用于检测双 reader 同时 Read 的 bug。
func (c *deadlineIgnoringConn) Read(p []byte) (int, error) {
	cur := atomic.AddInt32(&c.active, 1)
	for {
		max := atomic.LoadInt32(&c.maxActive)
		if cur <= max {
			break
		}
		if atomic.CompareAndSwapInt32(&c.maxActive, max, cur) {
			break
		}
	}
	defer atomic.AddInt32(&c.active, -1)
	return c.Conn.Read(p)
}

// Close 关闭连接并通知 closed channel。
func (c *deadlineIgnoringConn) Close() error {
	c.once.Do(func() { close(c.closed) })
	return c.Conn.Close()
}

// MaxActiveReads 返回历史最大并发 Read 数。
func (c *deadlineIgnoringConn) MaxActiveReads() int32 {
	return atomic.LoadInt32(&c.maxActive)
}

// TestDAQP1064PreStopAcquisition_ReturnsWithinBudgetOnDeadlineIgnoringConn 验证：
// 当 readLoop 阻塞在 deadline 失效的 Read 上时，StopAcquisition 通过 Close 兜底
// 在 ReadLoopJoinTimeout + 1s 内返回，并标记连接为 Error。
//
// 修复前：stopAcquisitionLocked 仅 close(d.stop)，无 join + 无 Close 兜底，
// readLoop 永久阻塞 → StopAcquisition 永久不返回（生产环境卡死）。
// 修复后：close(stop) 后 join readLoopDone，1s 超时触发 invalidate Close conn，
// readLoop 解除阻塞退出，StopAcquisition 返回 "reconnect required" 错误。
func TestDAQP1064PreStopAcquisition_ReturnsWithinBudgetOnDeadlineIgnoringConn(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()

	d := NewDAQP1064Pre(device.Profile{ID: "test-stop-stuck-reader", Type: device.DeviceDAQP1604Pre})
	d.mu.Lock()
	d.conn = newDeadlineIgnoringConn(client)
	d.acquiring = true
	d.stop = make(chan struct{})
	d.readLoopDone = make(chan struct{})
	d.status.Connection = device.ConnectionAcquiring
	d.status.Acquiring = true
	d.mu.Unlock()

	// 启动 readLoop，等待其进入 Read 阻塞
	go d.readLoop(d.stop)
	time.Sleep(100 * time.Millisecond)

	// 调用 StopAcquisition，应在 ReadLoopJoinTimeout + 1s 内返回
	start := time.Now()
	err := d.StopAcquisition()
	elapsed := time.Since(start)

	if err == nil || !strings.Contains(err.Error(), "reconnect required") {
		t.Fatalf("StopAcquisition error = %v, want 'reconnect required'", err)
	}
	budget := sharedproto.ReadLoopJoinTimeout + time.Second
	if elapsed > budget {
		t.Fatalf("StopAcquisition took too long: %v (budget %v)", elapsed, budget)
	}

	// 验证连接已作废（invalidate 已执行）
	d.mu.RLock()
	conn := d.conn
	status := d.status.Connection
	done := d.readLoopDone
	d.mu.RUnlock()
	if conn != nil {
		t.Fatal("d.conn should be nil after StopAcquisition timeout")
	}
	if status != device.ConnectionError {
		t.Fatalf("status = %v, want Error", status)
	}

	// 等待 readLoop 完全退出，避免 race detector 误报
	if done != nil {
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Fatal("readLoop did not exit after StopAcquisition")
		}
	}
}

// TestDAQP1064PreStartAcquisition_DoesNotStartSecondReadLoop 验证：
// StartAcquisition → StopAcquisition → StartAcquisition 序列不会启动两个 readLoop。
//
// 修复前：Stop 不等 readLoop 退出也不 Close conn，readLoop #1 仍在 Read 阻塞；
// 再次 Start 时旧 readLoop 未退出，新 readLoop 启动后双 reader 同时 Read conn，
// TCP 字节随机分配导致数据错位/丢失。
// 修复后：Stop 的 invalidate 关闭 conn 解除 readLoop 阻塞，d.conn 置 nil；
// 再次 Start 看到 d.conn==nil 返回 "device not connected" 错误，不启动第二个 readLoop。
func TestDAQP1064PreStartAcquisition_DoesNotStartSecondReadLoop(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()

	// 服务端持续读取并丢弃，避免 sendStartAcquisitionLocked 的 Write 阻塞
	go func() {
		buf := make([]byte, 256)
		for {
			if _, err := server.Read(buf); err != nil {
				return
			}
		}
	}()

	ignored := newDeadlineIgnoringConn(client)
	d := NewDAQP1064Pre(device.Profile{ID: "test-no-double-reader", Type: device.DeviceDAQP1604Pre})
	d.mu.Lock()
	d.conn = ignored
	d.status.Connection = device.ConnectionConnected
	d.mu.Unlock()

	// 第一次 StartAcquisition：成功启动 readLoop #1，阻塞在 Read
	if err := d.StartAcquisition(); err != nil {
		t.Fatalf("StartAcquisition #1 failed: %v", err)
	}
	time.Sleep(100 * time.Millisecond) // 等待 readLoop #1 进入 Read

	// StopAcquisition：readLoop #1 仍在 Read 阻塞，1s 内未退出 → invalidate + 返回错误
	_ = d.StopAcquisition()

	// 第二次 StartAcquisition：应返回错误（d.conn==nil），不启动第二个 readLoop
	err := d.StartAcquisition()
	if err == nil {
		t.Fatal("StartAcquisition #2 should fail after StopAcquisition with stuck readLoop")
	}

	// 验证无并发 Read（maxActive <= 1）
	// 修复前（bug）：readLoop #1 仍在 Read，readLoop #2 启动后 maxActive=2
	// 修复后：readLoop #1 已退出，readLoop #2 未启动，maxActive=1
	if max := ignored.MaxActiveReads(); max > 1 {
		t.Fatalf("concurrent reads detected: maxActive=%d, want <=1 (two readLoops ran simultaneously)", max)
	}

	// 等待 readLoop 退出，避免 race detector 误报
	d.mu.RLock()
	done := d.readLoopDone
	d.mu.RUnlock()
	if done != nil {
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			// readLoop 已退出即可，超时不一定代表 bug（可能是时序问题）
		}
	}
}

// TestDAQP1064PreDisconnect_JoinsReadLoop 验证：
// Disconnect 在 readLoop 阻塞时能在 ReadLoopJoinTimeout + 1s 内返回。
//
// 场景：deadlineIgnoringConn 屏蔽 SetReadDeadline，模拟 Windows 上 deadline 失效。
// readLoop 永久阻塞在 Read，close(stop) 无法解除（Read 不响应 stop channel）。
//
// I-4 修复前：Disconnect join 超时只打 warn 不调 invalidate，readLoop 卡死静默泄漏
// goroutine，上层无法感知需要重连。
// I-4 修复后：Disconnect join 超时调用 invalidateConnectionAfterReadLoopTimeout，
// 置 status=Error、调 onError 让上层感知、Close conn 让 readLoop 退出。
// 这是 ADR-009 的预期行为：readLoop 卡死属于异常状态，必须通知上层重连。
func TestDAQP1064PreDisconnect_JoinsReadLoop(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()

	d := NewDAQP1064Pre(device.Profile{ID: "test-disconnect-join", Type: device.DeviceDAQP1604Pre})
	var onErrorCalled int32
	d.SetOnError(func(err error) {
		atomic.StoreInt32(&onErrorCalled, 1)
	})

	d.mu.Lock()
	d.conn = newDeadlineIgnoringConn(client)
	d.acquiring = true
	d.stop = make(chan struct{})
	d.readLoopDone = make(chan struct{})
	d.status.Connection = device.ConnectionAcquiring
	d.status.Acquiring = true
	d.mu.Unlock()

	// 启动 readLoop，等待其进入 Read 阻塞
	go d.readLoop(d.stop)
	time.Sleep(100 * time.Millisecond)

	// 调用 Disconnect，应在 ReadLoopJoinTimeout + 1s 内返回
	start := time.Now()
	err := d.Disconnect()
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("Disconnect returned error: %v", err)
	}
	budget := sharedproto.ReadLoopJoinTimeout + time.Second
	if elapsed > budget {
		t.Fatalf("Disconnect took too long: %v (budget %v)", elapsed, budget)
	}

	// 验证连接已清理（invalidate 已置 d.conn=nil）
	d.mu.RLock()
	conn := d.conn
	status := d.status.Connection
	done := d.readLoopDone
	d.mu.RUnlock()
	if conn != nil {
		t.Fatal("d.conn should be nil after Disconnect")
	}
	// I-4 修复：readLoop 卡死场景下 invalidate 把 status 修正为 Error，
	// 让上层感知需要重连（不再是 Disconnected，否则上层会以为正常断开）
	if status != device.ConnectionError {
		t.Fatalf("status = %v, want Error (readLoop stuck, invalidate triggered)", status)
	}

	// 等待 readLoop 完全退出（invalidate Close conn 后 Read 返回错误，readLoop 退出）
	if done != nil {
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Fatal("readLoop did not exit after Disconnect")
		}
	}

	// I-4 修复：readLoop 卡死属于异常状态，onError 应被调用以上报故障。
	// 这与"正常 Disconnect 不调 onError"不同——本测试场景刻意构造了 readLoop
	// 永久阻塞的异常情况，必须让 DeviceManager 感知并触发重连流程。
	if atomic.LoadInt32(&onErrorCalled) == 0 {
		t.Fatal("onError should be called when readLoop is stuck (invalidate triggered)")
	}
}

// TestDAQP1064PreSendCommand_InvalidatesConnectionOnWriteWatchdogTrigger 验证：
// sendCommand 在 Write 阶段 watchdog 触发后调用 invalidateConnection 清理连接状态。
//
// 修复前（P1-3.a）：Write 失败直接返回错误，未置 d.conn=nil、未调 onError。
// watchdog 触发后 d.conn 仍非 nil，下次操作通过 nil 检查但实际失败。
// 修复后：watchdog 触发时（wdStop 返回 false）调用 invalidateConnection，
// 置 d.conn=nil、status=Error、调 onError，调用方走重连路径。
//
// 场景：deadlineIgnoringConn 屏蔽 SetWriteDeadline（如有）+ 服务端不读 →
// Write 永久阻塞 → watchdog 超时 Close conn → Write 返回 closed 错误 →
// sendCommand 检测 wdStop()==false 调 invalidateConnection。
func TestDAQP1064PreSendCommand_InvalidatesConnectionOnWriteWatchdogTrigger(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()

	ignored := newDeadlineIgnoringConn(client)

	d := NewDAQP1064Pre(device.Profile{ID: "test-sendcmd-write-invalidate", Type: device.DeviceDAQP1604Pre})
	var onErrorCalled int32
	d.SetOnError(func(err error) {
		atomic.StoreInt32(&onErrorCalled, 1)
	})
	d.mu.Lock()
	d.conn = ignored
	d.status.Connection = device.ConnectionConnected
	d.mu.Unlock()

	// 服务端不读 → 客户端 Write 永久阻塞；timeoutMs=50 → watchdog 100ms 触发 Close conn
	started := time.Now()
	_, err := d.sendCommand(CMD_READ_CALIBRATION, []byte{0}, 50)
	elapsed := time.Since(started)

	if err == nil {
		t.Fatal("expected watchdog-triggered error, got nil")
	}
	if !strings.Contains(err.Error(), "watchdog triggered") {
		t.Errorf("error should mention 'watchdog triggered', got: %v", err)
	}
	// 预算 2s 足够覆盖 100ms watchdog + Write 在 conn Close 后返回的延迟
	if elapsed > 2*time.Second {
		t.Fatalf("sendCommand took too long: %v (watchdog should have triggered at ~100ms)", elapsed)
	}

	// 验证 d.conn 已被 invalidateConnection 置 nil
	d.mu.RLock()
	conn := d.conn
	status := d.status.Connection
	lastErr := d.status.LastError
	d.mu.RUnlock()
	if conn != nil {
		t.Fatal("d.conn should be nil after watchdog triggered invalidate")
	}
	if status != device.ConnectionError {
		t.Fatalf("status = %v, want Error", status)
	}
	if lastErr == "" {
		t.Error("status.LastError should be set after invalidate")
	}
	if atomic.LoadInt32(&onErrorCalled) != 1 {
		t.Fatal("onError callback should be invoked after watchdog triggered invalidate")
	}
}

// TestDAQP1064PreSendCommand_InvalidatesConnectionOnReadWatchdogTrigger 验证：
// sendCommand 在 Read 阶段 watchdog 触发后调用 invalidateConnection 清理连接状态。
//
// 修复前（P1-3.a）：Read 失败直接返回错误，未置 d.conn=nil、未调 onError。
// 修复后：readResponseFrame 内部用 WrapWatchdogError 包装错误，sendCommand 检测
// wdStop()==false 调 invalidateConnection。
//
// 场景：服务端读取命令帧（Write 成功）但不发响应 → io.ReadFull 永久阻塞
// （deadlineIgnoringConn 屏蔽 SetReadDeadline）→ watchdog 超时 Close conn →
// Read 返回 closed 错误 → readResponseFrame 包装 → sendCommand 检测 wdStop()==false 调 invalidate。
func TestDAQP1064PreSendCommand_InvalidatesConnectionOnReadWatchdogTrigger(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()

	ignored := newDeadlineIgnoringConn(client)

	d := NewDAQP1064Pre(device.Profile{ID: "test-sendcmd-read-invalidate", Type: device.DeviceDAQP1604Pre})
	var onErrorCalled int32
	d.SetOnError(func(err error) {
		atomic.StoreInt32(&onErrorCalled, 1)
	})
	d.mu.Lock()
	d.conn = ignored
	d.status.Connection = device.ConnectionConnected
	d.mu.Unlock()

	// 服务端持续读取并丢弃命令帧，但不发响应 → 客户端 Read 永久阻塞
	go func() {
		buf := make([]byte, 256)
		for {
			if _, err := server.Read(buf); err != nil {
				return
			}
		}
	}()

	// timeoutMs=50 → watchdog 100ms 触发 Close conn，解除 io.ReadFull 阻塞
	started := time.Now()
	_, err := d.sendCommand(CMD_READ_CALIBRATION, []byte{0}, 50)
	elapsed := time.Since(started)

	if err == nil {
		t.Fatal("expected watchdog-triggered error, got nil")
	}
	if !strings.Contains(err.Error(), "watchdog triggered") {
		t.Errorf("error should mention 'watchdog triggered', got: %v", err)
	}
	if elapsed > 2*time.Second {
		t.Fatalf("sendCommand took too long: %v (watchdog should have triggered at ~100ms)", elapsed)
	}

	// 验证 d.conn 已被 invalidateConnection 置 nil
	d.mu.RLock()
	conn := d.conn
	status := d.status.Connection
	lastErr := d.status.LastError
	d.mu.RUnlock()
	if conn != nil {
		t.Fatal("d.conn should be nil after watchdog triggered invalidate")
	}
	if status != device.ConnectionError {
		t.Fatalf("status = %v, want Error", status)
	}
	if lastErr == "" {
		t.Error("status.LastError should be set after invalidate")
	}
	if atomic.LoadInt32(&onErrorCalled) != 1 {
		t.Fatal("onError callback should be invoked after watchdog triggered invalidate")
	}
}

// TestDAQP1064PreSendStartAcquisition_InvalidatesConnectionOnWatchdogTrigger 验证：
// sendStartAcquisitionLocked 在 Write 阶段 watchdog 触发后通过 StartAcquisition 调用
// invalidateConnection 清理连接状态。
//
// 修复前（P1-3.b）：Write 失败用 WrapWatchdogError 包装返回，但未置 d.conn=nil、未调 onError。
// 修复后：sendStartAcquisitionLocked 返回 invalidateNeeded=true，StartAcquisition 在释放锁后
// 调用 invalidateConnection（不能在 sendStartAcquisitionLocked 内部调，因为 *Locked 方法
// 持有 d.mu 锁，invalidateConnection 内部 d.mu.Lock() 会自死锁）。
//
// 场景：deadlineIgnoringConn 屏蔽 SetWriteDeadline（避免 5s deadline 与 5s watchdog 竞态）+
// 服务端不读 → Write 永久阻塞 → watchdog 5s 超时 Close conn → Write 返回 closed 错误 →
// sendStartAcquisitionLocked 返回 (true, wrappedErr) → StartAcquisition 释放锁后调 invalidateConnection。
//
// 通过 StartAcquisition（而非直接调 sendStartAcquisitionLocked）验证生产代码完整路径。
// watchdog 超时为生产常量 DAQ_P_1064PRE_TIMEOUT=5s，测试预算 7s。
func TestDAQP1064PreSendStartAcquisition_InvalidatesConnectionOnWatchdogTrigger(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()

	ignored := newDeadlineIgnoringConn(client)

	d := NewDAQP1064Pre(device.Profile{ID: "test-start-invalidate", Type: device.DeviceDAQP1604Pre})
	var onErrorCalled int32
	d.SetOnError(func(err error) {
		atomic.StoreInt32(&onErrorCalled, 1)
	})
	d.mu.Lock()
	d.conn = ignored
	d.status.Connection = device.ConnectionConnected
	d.mu.Unlock()

	// 服务端不读 → 客户端 Write 永久阻塞；watchdog 5s 后触发 Close conn
	started := time.Now()
	err := d.StartAcquisition()
	elapsed := time.Since(started)

	if err == nil {
		t.Fatal("expected watchdog-triggered error, got nil")
	}
	if !strings.Contains(err.Error(), "watchdog triggered") {
		t.Errorf("error should mention 'watchdog triggered', got: %v", err)
	}
	// 预算 7s 覆盖 5s watchdog + Write 在 conn Close 后返回的延迟
	if elapsed > DAQ_P_1064PRE_TIMEOUT+2*time.Second {
		t.Fatalf("StartAcquisition took too long: %v (watchdog should have triggered at ~5s)", elapsed)
	}

	// 验证 d.conn 已被 invalidateConnection 置 nil
	d.mu.RLock()
	conn := d.conn
	status := d.status.Connection
	lastErr := d.status.LastError
	d.mu.RUnlock()
	if conn != nil {
		t.Fatal("d.conn should be nil after watchdog triggered invalidate")
	}
	if status != device.ConnectionError {
		t.Fatalf("status = %v, want Error", status)
	}
	if lastErr == "" {
		t.Error("status.LastError should be set after invalidate")
	}
	if atomic.LoadInt32(&onErrorCalled) != 1 {
		t.Fatal("onError callback should be invoked after watchdog triggered invalidate")
	}
}

// TestDAQP1064PreSendCommand_InvalidatesConnectionOnResponseCmdMismatch 验证 ADR-009 R0-12：
// 命令-响应路径在响应 cmd 与请求 cmd 不匹配时必须毒化连接（Close + 清 conn + Error + onError）。
//
// 历史背景：原 sendCommand 只在 watchdog 触发时毒化连接。若 soft deadline 正常先于
// watchdog 返回（OS 兑现 deadline），或响应帧 cmd 与请求不匹配（迟到响应/采集帧串入/帧错位），
// helper 返回普通错误，但迟到响应仍可能进入 TCP 流被下一命令消费——协议边界已不可信。
//
// 测试前置：
//   - net.Pipe 建立双向连接
//   - client 包装为 deadlineIgnoringConn（确保 SetReadDeadline 不先返回，让 io.ReadFull
//     必然读到完整 6 字节 header，避免与 watchdog 路径混淆）
//   - server 收到命令后回复一个 cmd 不匹配的响应帧（expected=0x03, actual=0xFF）
//
// 测试步骤：
//   - 调用 d.sendCommand(CMD_READ_CALIBRATION=0x03, []byte{0}, 2000)
//
// 期待结果：
//   - sendCommand 返回 "response cmd mismatch" 错误
//   - d.conn == nil（毒化连接，避免下一命令消费迟到响应）
//   - status.Connection == Error
//   - status.LastError 非空，包含 "protocol error" 上下文
//   - onError 被调用
func TestDAQP1064PreSendCommand_InvalidatesConnectionOnResponseCmdMismatch(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()

	ignored := newDeadlineIgnoringConn(client)

	d := NewDAQP1064Pre(device.Profile{ID: "test-cmd-mismatch", Type: device.DeviceDAQP1604Pre})
	var onErrorCalled int32
	d.SetOnError(func(err error) {
		atomic.StoreInt32(&onErrorCalled, 1)
	})
	d.mu.Lock()
	d.conn = ignored
	d.status.Connection = device.ConnectionConnected
	d.mu.Unlock()

	// 服务端：读取命令帧后回复一个 cmd 不匹配的响应帧。
	// buildFrame 格式：[0xA5, 0x5A, cmd, lenHi, lenLo, data..., checksum]
	// 回复 cmd=0xFF（与请求 0x03 不匹配），dataLen=0，checksum 简单求和（不影响测试）。
	serverDone := make(chan struct{})
	go func() {
		defer close(serverDone)
		buf := make([]byte, 256)
		// 读掉请求帧（6 字节 header + 1 字节 data + 1 字节 checksum）
		if _, err := server.Read(buf); err != nil {
			return
		}
		// 构造响应帧：cmd=0xFF，dataLen=0。checksum 简单求和（设备算法），溢出 byte 取低 8 位
		// 分步计算避免编译期 int 常量 510 溢出 byte 报错
		checksum := byte(0xA5)
		checksum += 0x5A
		checksum += 0xFF
		resp := []byte{0xA5, 0x5A, 0xFF, 0x00, 0x00, checksum}
		_ = server.SetWriteDeadline(time.Now().Add(500 * time.Millisecond))
		_, _ = server.Write(resp)
	}()

	_, err := d.sendCommand(CMD_READ_CALIBRATION, []byte{0}, 2000)
	if err == nil {
		t.Fatal("expected cmd mismatch error, got nil")
	}
	if !strings.Contains(err.Error(), "cmd mismatch") {
		t.Errorf("error should mention 'cmd mismatch', got: %v", err)
	}

	// 验证连接已毒化
	d.mu.RLock()
	conn := d.conn
	status := d.status.Connection
	lastErr := d.status.LastError
	d.mu.RUnlock()
	if conn != nil {
		t.Fatal("d.conn should be nil after cmd mismatch invalidate (late response could poison next command)")
	}
	if status != device.ConnectionError {
		t.Fatalf("status = %v, want Error (protocol boundary untrusted)", status)
	}
	if lastErr == "" {
		t.Error("status.LastError should be set after cmd mismatch invalidate")
	}
	if atomic.LoadInt32(&onErrorCalled) != 1 {
		t.Fatal("onError callback should be invoked after cmd mismatch invalidate")
	}

	// 等 server goroutine 退出避免 race
	_ = client.Close()
	select {
	case <-serverDone:
	case <-time.After(500 * time.Millisecond):
	}
}

// TestDAQP1064PreSendCommand_NonEmptyPayloadSuccess 验证 ADR-009 复核修订 finding 1：
// readResponseFrame 必须按 5 字节 header + dataLen 字节 payload + 1 字节 checksum 读取，
// 不能误读 6 字节 header（会把 payload 首字节吃进 header，末尾混入 checksum）。
//
// 测试前置：
//   - net.Pipe 建立双向连接，client 端包 deadlineIgnoringConn
//   - d.conn 已设置，status=Connected
//   - 服务端回复一个 dataLen=4 的有效响应帧（payload = [0x00, 0x01, 0x02, 0x03]）
//
// 测试步骤：
//   - 调用 d.sendCommand(CMD_READ_CALIBRATION=0x03, []byte{0}, 2000)
//
// 期待结果：
//   - sendCommand 返回 nil 错误
//   - 返回的 respData == []byte{0x00, 0x01, 0x02, 0x03}（payload 首字节不丢失，末尾不混入 checksum）
//   - d.conn 仍非 nil（连接未毒化，可继续使用）
//   - status.Connection == Connected
//
// 修复前：readResponseFrame 读 6 字节 header，dataLen=4 时：
//   - header = [0xA5, 0x5A, 0x03, 0x00, 0x04, 0x00]（第 6 字节是 payload 首字节 0x00）
//   - dataLen = 0x00<<8 | 0x04 = 4
//   - respData = io.ReadFull(4) = [0x01, 0x02, 0x03, checksum]
//   - 返回 [0x01, 0x02, 0x03, checksum]，payload 首字节 0x00 丢失，末尾混入 checksum
//   - 测试断言 respData == [0x00, 0x01, 0x02, 0x03] 失败
//
// 修复后：readResponseFrame 读 5 字节 header + 4 字节 payload + 1 字节 checksum：
//   - header = [0xA5, 0x5A, 0x03, 0x00, 0x04]
//   - respData = io.ReadFull(4) = [0x00, 0x01, 0x02, 0x03]
//   - checksum = io.ReadFull(1) = 计算值
//   - 校验 checksum 通过，返回 [0x00, 0x01, 0x02, 0x03]
func TestDAQP1064PreSendCommand_NonEmptyPayloadSuccess(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()

	ignored := newDeadlineIgnoringConn(client)

	d := NewDAQP1064Pre(device.Profile{ID: "test-non-empty-payload", Type: device.DeviceDAQP1604Pre})
	d.mu.Lock()
	d.conn = ignored
	d.status.Connection = device.ConnectionConnected
	d.mu.Unlock()

	// 服务端：读取命令帧后回复一个 dataLen=4 的有效响应帧。
	// 帧布局：[0xA5, 0x5A, cmd=0x03, lenHi=0x00, lenLo=0x04, payload[4], checksum]
	// payload = [0x00, 0x01, 0x02, 0x03]，checksum = sum(header + payload) & 0xFF
	expectedPayload := []byte{0x00, 0x01, 0x02, 0x03}
	serverDone := make(chan struct{})
	go func() {
		defer close(serverDone)
		buf := make([]byte, 256)
		// 读掉请求帧（5 字节 header + 1 字节 data + 1 字节 checksum = 7 字节）
		if _, err := server.Read(buf); err != nil {
			return
		}
		// 构造响应帧
		resp := []byte{0xA5, 0x5A, 0x03, 0x00, 0x04}
		resp = append(resp, expectedPayload...)
		// checksum = sum(resp) & 0xFF
		var sum byte
		for _, b := range resp {
			sum += b
		}
		resp = append(resp, sum)
		_ = server.SetWriteDeadline(time.Now().Add(500 * time.Millisecond))
		_, _ = server.Write(resp)
	}()

	resp, err := d.sendCommand(CMD_READ_CALIBRATION, []byte{0}, 2000)
	if err != nil {
		t.Fatalf("sendCommand should succeed on valid non-empty payload, got err: %v", err)
	}
	if len(resp) != len(expectedPayload) {
		t.Fatalf("respData length = %d, want %d (payload首字节不能丢失，末尾不能混入checksum)", len(resp), len(expectedPayload))
	}
	for i, b := range resp {
		if b != expectedPayload[i] {
			t.Fatalf("respData[%d] = 0x%02X, want 0x%02X (修复前会丢失首字节0x00、末尾混入checksum)", i, b, expectedPayload[i])
		}
	}

	// 连接未毒化，可继续使用
	d.mu.RLock()
	conn := d.conn
	status := d.status.Connection
	d.mu.RUnlock()
	if conn == nil {
		t.Fatal("d.conn should remain non-nil after successful non-empty payload response")
	}
	if status != device.ConnectionConnected {
		t.Fatalf("status = %v, want Connected", status)
	}

	// 等 server goroutine 退出避免 race
	_ = client.Close()
	select {
	case <-serverDone:
	case <-time.After(500 * time.Millisecond):
	}
}

// TestDAQP1064PreSendCommand_ChecksumMismatchInvalidatesConn 验证 ADR-009 复核修订 finding 1：
// readResponseFrame 必须校验 checksum，不匹配时毒化连接（协议边界不可信）。
//
// 测试前置：
//   - net.Pipe 建立双向连接，client 端包 deadlineIgnoringConn
//   - d.conn 已设置，status=Connected
//   - 服务端回复一个 checksum 故意写错的响应帧（dataLen=2，checksum=0xFF）
//
// 测试步骤：
//   - 调用 d.sendCommand(CMD_READ_CALIBRATION=0x03, []byte{0}, 2000)
//
// 期待结果：
//   - sendCommand 返回 "checksum mismatch" 错误
//   - d.conn == nil（毒化连接）
//   - status.Connection == Error
//   - onError 被调用
func TestDAQP1064PreSendCommand_ChecksumMismatchInvalidatesConn(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()

	ignored := newDeadlineIgnoringConn(client)

	d := NewDAQP1064Pre(device.Profile{ID: "test-checksum-mismatch", Type: device.DeviceDAQP1604Pre})
	var onErrorCalled int32
	d.SetOnError(func(err error) {
		atomic.StoreInt32(&onErrorCalled, 1)
	})
	d.mu.Lock()
	d.conn = ignored
	d.status.Connection = device.ConnectionConnected
	d.mu.Unlock()

	// 服务端：读取命令帧后回复一个 checksum 故意写错的响应帧。
	serverDone := make(chan struct{})
	go func() {
		defer close(serverDone)
		buf := make([]byte, 256)
		if _, err := server.Read(buf); err != nil {
			return
		}
		// 构造响应帧：cmd=0x03, dataLen=2, payload=[0xAA, 0xBB], checksum=0xFF（故意写错）
		resp := []byte{0xA5, 0x5A, 0x03, 0x00, 0x02, 0xAA, 0xBB, 0xFF}
		_ = server.SetWriteDeadline(time.Now().Add(500 * time.Millisecond))
		_, _ = server.Write(resp)
	}()

	_, err := d.sendCommand(CMD_READ_CALIBRATION, []byte{0}, 2000)
	if err == nil {
		t.Fatal("sendCommand should fail on checksum mismatch, got nil")
	}
	if !strings.Contains(err.Error(), "checksum mismatch") {
		t.Errorf("error should mention 'checksum mismatch', got: %v", err)
	}

	d.mu.RLock()
	conn := d.conn
	status := d.status.Connection
	d.mu.RUnlock()
	if conn != nil {
		t.Fatal("d.conn should be nil after checksum mismatch invalidate")
	}
	if status != device.ConnectionError {
		t.Fatalf("status = %v, want Error", status)
	}
	if atomic.LoadInt32(&onErrorCalled) != 1 {
		t.Fatal("onError callback should be invoked after checksum mismatch invalidate")
	}

	_ = client.Close()
	select {
	case <-serverDone:
	case <-time.After(500 * time.Millisecond):
	}
}

// TestDAQP1064PreReadLoop_InvalidatesConnOnNoDataTimeout 验证 ADR-009 R0-10：
// readLoop 入口启动的独立 no-data timer 在 deadline 失效（Read 永久阻塞）且
// 无任何数据到达时，必须在 noDataTimeout 到期后独立触发连接毒化——
// 清空 d.conn、置 Error 状态、保存 LastError、close conn。
//
// 关键差异（与 terminal read error 测试的对比）：
//   - terminal read error：对端 EOF → Read 返回错误 → defer invalidate
//   - no-data timer：无数据 → timer 到期 → Close conn → Read 返回 closed 错误
//     → readLoop 检测 IsClosedConnError 静默退出（不调 invalidate，状态由 timer 设置）
//
// timer 必须独立于 readLoop 循环体执行。本测试用 deadlineIgnoringConn 让 Read
// 永久阻塞（模拟 Windows 故障环境下 deadline 失效），循环体不可达，
// 仅靠 timer 到期能触发毒化。若 timer 未启动或依赖循环体，测试会超时。
//
// 测试前置：
//   - net.Pipe 建立双向连接，client 端包 deadlineIgnoringConn（Read 永久阻塞）
//   - d.conn / readLoopDone 已设置，acquiring=true
//   - noDataTimeout=200ms（加速 noDataTimer，生产默认 10s）
//   - readLoopWatchdog 保持默认 10s（与 noDataTimeout 解耦，避免同时触发覆盖 LastError）
//   - server 端不写任何数据，让 client Read 永久阻塞
//
// 测试步骤：
//   - 启动 readLoop goroutine
//   - 等待 noDataTimeout(200ms) + 余量(800ms) = 1s 预算
//
// 期待结果：
//   - d.conn 被置为 nil（timer 回调 Close）
//   - d.status.Connection = ConnectionError
//   - d.status.LastError 非空（含 "no data"）
//   - onError 回调不被调用（readLoop 静默退出，defer 不调 invalidate）
//   - readLoopDone 已关闭（readLoop 已退出）
func TestDAQP1064PreReadLoop_InvalidatesConnOnNoDataTimeout(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()

	// 临时覆盖 noDataTimeout 为 200ms 加速测试。
	// 同一包内测试默认串行执行，覆盖安全；t.Cleanup 确保恢复。
	// ADR-009 finding 4：使用 atomic 包装的 helper 函数，避免 readLoop 跨测试边界读取
	// 全局 noDataTimeout 与测试修改并发触发 data race。
	origTimeout := getNoDataTimeout()
	setNoDataTimeout(200 * time.Millisecond)
	t.Cleanup(func() { setNoDataTimeout(origTimeout) })

	d := NewDAQP1064Pre(device.Profile{ID: "p1064pre-nodata", Type: device.DeviceDAQP1604Pre})
	var onErrorCalled int32
	d.SetOnError(func(err error) {
		atomic.StoreInt32(&onErrorCalled, 1)
	})

	d.mu.Lock()
	d.conn = newDeadlineIgnoringConn(client)
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
	statusAfter := d.status.Connection
	lastError := d.status.LastError
	d.mu.RUnlock()
	if connAfter != nil {
		t.Error("d.conn should be nil after no-data timer fired")
	}
	if statusAfter != device.ConnectionError {
		t.Errorf("status.Connection = %v, want Error", statusAfter)
	}
	if !strings.Contains(lastError, "no data") {
		t.Errorf("status.LastError should mention 'no data', got: %q", lastError)
	}
	// DAQP1064Pre readLoop 检测 IsClosedConnError 后静默退出，defer 不调 invalidate，
	// 因此 onError 不被调用。状态由 timer 回调直接设置。
	if atomic.LoadInt32(&onErrorCalled) != 0 {
		t.Error("onError should NOT be invoked (readLoop exits silently on closed conn)")
	}
}
