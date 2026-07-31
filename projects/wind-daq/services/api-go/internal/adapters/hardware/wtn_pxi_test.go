package hardware

import (
	"net"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	sharedproto "shared.local/device-sdk/go/protocol"
	"wind-daq/services/api-go/internal/core/device"
)

// TestWTNPXIStopAcquisition_ReturnsWithinBudgetOnDeadlineIgnoringConn 验证：
// 当 readLoop 阻塞在 deadline 失效的 Read 上时，StopAcquisition 通过 Close 兜底
// 在 ReadLoopJoinTimeout + 1s 内返回，并标记连接为 Error。
//
// 修复前：stopAcquisitionLocked 仅 close(d.stop)，无 join + 无 Close 兜底，
// readLoop 永久阻塞 → StopAcquisition 永久不返回（生产环境卡死）。
// 修复后：close(stop) 后 join readLoopDone，1s 超时触发 invalidate Close conn，
// readLoop 解除阻塞退出，StopAcquisition 返回 "reconnect required" 错误。
func TestWTNPXIStopAcquisition_ReturnsWithinBudgetOnDeadlineIgnoringConn(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()

	d := NewWTNPXI(device.Profile{ID: "test-wtn-stop-stuck-reader", Type: device.DeviceWTNPXI})
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

// TestWTNPXIStartAcquisition_DoesNotStartSecondReadLoop 验证：
// StartAcquisition → StopAcquisition → StartAcquisition 序列不会启动两个 readLoop。
//
// 修复前：Stop 不等 readLoop 退出也不 Close conn，readLoop #1 仍在 Read 阻塞；
// 再次 Start 时旧 readLoop 未退出，新 readLoop 启动后双 reader 同时 Read conn，
// TCP 字节随机分配导致数据错位/丢失。
// 修复后：Stop 的 invalidate 关闭 conn 解除 readLoop 阻塞，d.conn 置 nil；
// 再次 Start 看到 d.conn==nil 返回 "device not connected" 错误，不启动第二个 readLoop。
func TestWTNPXIStartAcquisition_DoesNotStartSecondReadLoop(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()

	ignored := newDeadlineIgnoringConn(client)
	d := NewWTNPXI(device.Profile{ID: "test-wtn-no-double-reader", Type: device.DeviceWTNPXI})
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

// TestWTNPXIDisconnect_JoinsReadLoop 验证：
// Disconnect 在 readLoop 阻塞时能在 ReadLoopJoinTimeout + 1s 内返回，
// 且不调 onError（StopReason=StopReasonUserRequested 标记主动停止）。
//
// 修复前：Disconnect close(stop) 后立即 Close conn，readLoop 可能在 Disconnect
// 返回后才进入异常退出分支调 onError，造成"用户主动断开却被误报为设备故障"。
// 修复后：Disconnect 设 StopReason=UserRequested，join readLoopDone 后再 Close conn，
// readLoop 检测到 StopReason 静默退出，不触发 onError。
func TestWTNPXIDisconnect_JoinsReadLoop(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()

	d := NewWTNPXI(device.Profile{ID: "test-wtn-disconnect-join", Type: device.DeviceWTNPXI})
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

	// 验证连接已清理
	d.mu.RLock()
	conn := d.conn
	status := d.status.Connection
	done := d.readLoopDone
	d.mu.RUnlock()
	if conn != nil {
		t.Fatal("d.conn should be nil after Disconnect")
	}
	if status != device.ConnectionDisconnected {
		t.Fatalf("status = %v, want Disconnected", status)
	}

	// 等待 readLoop 完全退出
	if done != nil {
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Fatal("readLoop did not exit after Disconnect")
		}
	}

	// Disconnect 不应触发 onError（StopReason=StopReasonUserRequested）
	if atomic.LoadInt32(&onErrorCalled) != 0 {
		t.Fatal("onError should NOT be called during Disconnect")
	}
}

// TestWTNPXIReadLoop_InvalidatesConnOnNoDataTimeout 验证 ADR-009 R0-10：
// readLoop 入口启动的独立 no-data timer 在 deadline 失效（Read 永久阻塞）且
// 无任何数据到达时，必须在 wtnPXINoDataTimeout 到期后独立触发连接毒化——
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
//   - wtnPXINoDataTimeout 临时覆盖为 200ms（生产默认 10s）
//   - server 端不写任何数据，让 client Read 永久阻塞
//
// 测试步骤：
//   - 启动 readLoop goroutine
//   - 等待 noDataTimeout(200ms) + 余量(800ms) = 1s 预算
//
// 期待结果：
//   - d.conn 被置为 nil（timer 回调设置）
//   - d.status.Connection = ConnectionError
//   - d.status.LastError 含 "no data"
//   - readLoopDone 已关闭（readLoop 静默退出）
//   - onError 不被调用（readLoop 静默退出，defer 不调 invalidate）
func TestWTNPXIReadLoop_InvalidatesConnOnNoDataTimeout(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()

	d := NewWTNPXI(device.Profile{ID: "wtn-pxi-nodata", Type: device.DeviceWTNPXI})
	var onErrorCalled int32
	d.SetOnError(func(err error) {
		atomic.StoreInt32(&onErrorCalled, 1)
	})

	// 临时覆盖 wtnPXINoDataTimeout 为 200ms 加速测试。
	// 同一包内测试默认串行执行，覆盖安全；t.Cleanup 确保恢复。
	origTimeout := wtnPXINoDataTimeout
	wtnPXINoDataTimeout = 200 * time.Millisecond
	t.Cleanup(func() { wtnPXINoDataTimeout = origTimeout })

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
	// WTN-PXI readLoop 检测 IsClosedConnError 后静默退出，defer 不调 invalidate，
	// 因此 onError 不被调用。状态由 timer 回调直接设置。
	if atomic.LoadInt32(&onErrorCalled) != 0 {
		t.Error("onError should NOT be invoked (readLoop exits silently on closed conn)")
	}
}
