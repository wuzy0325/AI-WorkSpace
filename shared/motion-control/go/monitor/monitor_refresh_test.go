package monitor

import (
	"sync"
	"testing"
	"time"

	"shared.local/device-sdk/go/motion/core"
)

// TestMonitorRequestRefreshTriggersImmediatePoll 验证 RequestRefresh 在 pollLoop 空闲时立即触发一轮采集。
//
// spec RequestRefresh: "若无采集在途则尽快启动"。
// 测试：pollLoop 等待 tick 期间调用 RequestRefresh，应立即触发一次 Status()，无需 ticker.Fire。
func TestMonitorRequestRefreshTriggersImmediatePoll(t *testing.T) {
	clock := NewFakeClock(time.Now())
	m := NewMotionStatusMonitor(DefaultConfig(), clock, DefaultFreshnessPolicy{
		StaleThreshold: 1 * time.Second,
	})
	fc := newFakeController("c1")
	statusCh := make(chan struct{}, 16)
	fc.statusCalled = statusCh

	m.RegisterController("c1", fc)
	defer m.UnregisterController("c1")

	// 首帧通过 tick 建立
	ticker := clock.LatestTicker()
	ticker.Fire(clock.Now())
	waitForStatusCall(t, statusCh, "initial poll")
	waitForSnapshot(t, m, "c1", 1)

	// 不触发 ticker，仅调用 RequestRefresh——应触发额外一次 Status()
	m.RequestRefresh("c1")
	waitForStatusCall(t, statusCh, "refresh poll")
	waitForSnapshot(t, m, "c1", 2)
}

// TestMonitorRequestRefreshMergesWhileInFlight 验证在途时多次 RequestRefresh 合并为一次补轮。
//
// spec RequestRefresh: "若已有采集在途只置位一次，完成后清位并最多补一轮"。
// 测试：阻塞首个 Status()（gate），连续 RequestRefresh 多次，释放 gate 后应只补一轮
//（总 Status 调用数 = 1 首轮 + 1 补轮，而非 N 次补轮）。
func TestMonitorRequestRefreshMergesWhileInFlight(t *testing.T) {
	clock := NewFakeClock(time.Now())
	m := NewMotionStatusMonitor(DefaultConfig(), clock, DefaultFreshnessPolicy{
		StaleThreshold: 1 * time.Second,
	})
	fc := newFakeController("c1")
	fc.gate = make(chan struct{})
	statusCh := make(chan struct{}, 32)
	fc.statusCalled = statusCh

	var gateOnce sync.Once
	gateClose := func() { gateOnce.Do(func() { close(fc.gate) }) }

	m.RegisterController("c1", fc)
	defer func() {
		gateClose()
		m.UnregisterController("c1")
	}()

	ticker := clock.LatestTicker()

	// 触发首帧 tick，Status() 阻塞在 gate 上
	ticker.Fire(clock.Now())
	waitForStatusCall(t, statusCh, "first poll (blocked)")

	// 在途期间连续 5 次 RequestRefresh——应合并为 1 次补轮
	for i := 0; i < 5; i++ {
		m.RequestRefresh("c1")
	}

	// 释放 gate，Status() 返回；pollLoop 应处理合并的补轮信号
	gateClose()
	// 补轮会触发第 2 次 Status()——先等其 statusCalled 信号再断言 sequence
	waitForStatusCall(t, statusCh, "merged refresh poll")
	waitForSnapshot(t, m, "c1", 2)

	// 再等一个宽限期，确认没有更多补轮（合并后只补一次）
	select {
	case <-statusCh:
		// 还有额外 Status 调用——可能未正确合并
		t.Fatalf("expected only 1 extra poll after merge, got additional Status() call")
	case <-time.After(50 * time.Millisecond):
		// 期望路径：无额外调用
	}

	count := fc.statusCallCount()
	if count != 2 {
		t.Fatalf("expected 2 Status calls (1 initial + 1 merged refresh), got %d", count)
	}
}

// TestMonitorRequestRefreshNonBlocking 验证 RequestRefresh 不阻塞调用方。
//
// spec RequestRefresh: "非阻塞"。
// 测试：在 pollLoop 在途（gate 阻塞）时调用 RequestRefresh，应在毫秒级返回。
func TestMonitorRequestRefreshNonBlocking(t *testing.T) {
	clock := NewFakeClock(time.Now())
	m := NewMotionStatusMonitor(DefaultConfig(), clock, DefaultFreshnessPolicy{
		StaleThreshold: 1 * time.Second,
	})
	fc := newFakeController("c1")
	fc.gate = make(chan struct{})
	statusCh := make(chan struct{}, 16)
	fc.statusCalled = statusCh

	var gateOnce sync.Once
	gateClose := func() { gateOnce.Do(func() { close(fc.gate) }) }

	m.RegisterController("c1", fc)
	defer func() {
		gateClose()
		m.UnregisterController("c1")
	}()

	ticker := clock.LatestTicker()
	ticker.Fire(clock.Now())
	waitForStatusCall(t, statusCh, "first poll (blocked)")

	// 在途时调用 RequestRefresh，应立即返回
	done := make(chan struct{})
	go func() {
		m.RequestRefresh("c1")
		close(done)
	}()
	select {
	case <-done:
		// 期望路径：立即返回
	case <-time.After(100 * time.Millisecond):
		t.Fatal("RequestRefresh blocked for >100ms while pollOnce in flight")
	}

	gateClose()
}

// TestMonitorRequestRefreshUnknownControllerIsNoop 验证未注册 ID 静默返回。
//
// spec RequestRefresh: "按 controller ID"——未注册 ID 不应 panic 或阻塞。
func TestMonitorRequestRefreshUnknownControllerIsNoop(t *testing.T) {
	clock := NewFakeClock(time.Now())
	m := NewMotionStatusMonitor(DefaultConfig(), clock, DefaultFreshnessPolicy{
		StaleThreshold: 1 * time.Second,
	})

	// 未注册 ID 应静默返回，不 panic
	m.RequestRefresh("nonexistent")

	// 已注册控制器调用 RequestRefresh 后立即 Unregister，再 RequestRefresh 应也静默
	fc := newFakeController("c1")
	m.RegisterController("c1", fc)
	m.UnregisterController("c1")
	m.RequestRefresh("c1") // 已注销，应 no-op
}

// TestMonitorRequestRefreshDoesNotStarveTicker 验证 RequestRefresh 不无限推迟周期轮询。
//
// spec RequestRefresh: "连续请求不得无限推迟既定周期轮询"。
// 测试：交替触发 RequestRefresh 与 ticker.Fire，两者都能推进 sequence。
func TestMonitorRequestRefreshDoesNotStarveTicker(t *testing.T) {
	clock := NewFakeClock(time.Now())
	m := NewMotionStatusMonitor(DefaultConfig(), clock, DefaultFreshnessPolicy{
		StaleThreshold: 1 * time.Second,
	})
	fc := newFakeController("c1")
	statusCh := make(chan struct{}, 32)
	fc.statusCalled = statusCh

	m.RegisterController("c1", fc)
	defer m.UnregisterController("c1")

	ticker := clock.LatestTicker()

	// 首帧
	ticker.Fire(clock.Now())
	waitForStatusCall(t, statusCh, "initial poll")
	waitForSnapshot(t, m, "c1", 1)

	// 交替触发 ticker 与 RequestRefresh
	for i := 0; i < 3; i++ {
		ticker.Fire(clock.Now())
		waitForStatusCall(t, statusCh, "ticker poll")
		waitForSnapshot(t, m, "c1", uint64(2+i*2))

		m.RequestRefresh("c1")
		waitForStatusCall(t, statusCh, "refresh poll")
		waitForSnapshot(t, m, "c1", uint64(3+i*2))
	}
}

// TestMonitorRequestRefreshDoesNotAffectOtherControllers 验证 RequestRefresh 只刷新目标控制器。
//
// spec RequestRefresh: "也不得刷新无关控制器"。
// 测试：注册两台控制器，对 c1 调 RequestRefresh，c2 的 Status 调用次数不应增加。
func TestMonitorRequestRefreshDoesNotAffectOtherControllers(t *testing.T) {
	clock := NewFakeClock(time.Now())
	m := NewMotionStatusMonitor(DefaultConfig(), clock, DefaultFreshnessPolicy{
		StaleThreshold: 1 * time.Second,
	})
	fc1 := newFakeController("c1")
	fc1.setStatus(core.ControllerStatus{ID: "c1", Connected: true, Axes: []core.AxisStatus{{Name: core.AxisX}}})
	statusCh1 := make(chan struct{}, 16)
	fc1.statusCalled = statusCh1

	fc2 := newFakeController("c2")
	fc2.setStatus(core.ControllerStatus{ID: "c2", Connected: true, Axes: []core.AxisStatus{{Name: core.AxisX}}})
	statusCh2 := make(chan struct{}, 16)
	fc2.statusCalled = statusCh2

	m.RegisterController("c1", fc1)
	defer m.UnregisterController("c1")
	m.RegisterController("c2", fc2)
	defer m.UnregisterController("c2")

	// 首帧
	clock.TickAll()
	waitForStatusCall(t, statusCh1, "c1 initial")
	waitForStatusCall(t, statusCh2, "c2 initial")
	waitForSnapshot(t, m, "c1", 1)
	waitForSnapshot(t, m, "c2", 1)

	c2Before := fc2.statusCallCount()

	// 对 c1 调用 RequestRefresh，c2 不应被刷新
	m.RequestRefresh("c1")
	waitForStatusCall(t, statusCh1, "c1 refresh")
	waitForSnapshot(t, m, "c1", 2)

	// 短暂等待确认 c2 无新调用
	select {
	case <-statusCh2:
		t.Fatal("c2 was refreshed by RequestRefresh(c1)")
	case <-time.After(50 * time.Millisecond):
		// 期望路径：c2 无新调用
	}

	c2After := fc2.statusCallCount()
	if c2After != c2Before {
		t.Fatalf("c2 Status calls changed: %d -> %d", c2Before, c2After)
	}
}
