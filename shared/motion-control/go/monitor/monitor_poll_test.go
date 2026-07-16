package monitor

import (
	"sync"
	"testing"
	"time"

	"shared.local/device-sdk/go/motion/core"
)

// waitForStatusCall 等待 fake controller 的 statusCalled 信号或超时。
//
// 设计理由：避免 time.Sleep 等待 polling goroutine 调度——通过 channel 同步，
// 测试既快又确定。1s 超时是兜底，正常情况下信号在毫秒级到达。
func waitForStatusCall(t *testing.T, ch <-chan struct{}, desc string) {
	t.Helper()
	select {
	case <-ch:
		return
	case <-time.After(1 * time.Second):
		t.Fatalf("%s: Status() was not called within 1s", desc)
	}
}

// TestMonitorPollsAfterRegister 验证 RegisterController 启动轮询 goroutine，
// tick 触发后 Status() 被调用。
//
// spec Interface Contract: 每控制器唯一轮询 goroutine。
func TestMonitorPollsAfterRegister(t *testing.T) {
	clock := NewFakeClock(time.Now())
	m := NewMotionStatusMonitor(DefaultConfig(), clock, DefaultFreshnessPolicy{
		StaleThreshold: 1 * time.Second,
	})
	fc := newFakeController("c1")
	statusCh := make(chan struct{}, 16)
	fc.statusCalled = statusCh

	m.RegisterController("c1", fc)
	defer m.UnregisterController("c1")

	ticker := clock.LatestTicker()
	if ticker == nil {
		t.Fatal("expected ticker to be created after RegisterController")
	}

	// 触发一次 tick，等待 Status() 被调用
	ticker.Fire(clock.Now())
	waitForStatusCall(t, statusCh, "initial poll")

	if count := fc.statusCallCount(); count < 1 {
		t.Fatalf("expected at least 1 Status call, got %d", count)
	}
}

// TestMonitorSingleFlight 验证单个控制器的轮询 goroutine 不会并发调用 Status()。
//
// spec Decision 2: 每台已连接控制器同一时刻最多一轮 Status() 在途。
// 实现层面：单 goroutine + select 自然保证 single-flight——goroutine 在 Status() 中阻塞时
// 无法接收新 tick。FakeTicker 缓冲为 1（latest-only），新 tick 被丢弃，不积压。
func TestMonitorSingleFlight(t *testing.T) {
	clock := NewFakeClock(time.Now())
	m := NewMotionStatusMonitor(DefaultConfig(), clock, DefaultFreshnessPolicy{
		StaleThreshold: 1 * time.Second,
	})
	fc := newFakeController("c1")
	fc.gate = make(chan struct{}) // 阻塞 Status() 直到关闭
	statusCh := make(chan struct{}, 16)
	fc.statusCalled = statusCh

	// gateOnce 保证 close(gate) 只调用一次：测试中途会显式 close 释放 gate，
	// defer 兜底也要 close 防止 Unregister 阻塞，sync.Once 避免重复 close panic
	var gateOnce sync.Once
	gateClose := func() { gateOnce.Do(func() { close(fc.gate) }) }

	m.RegisterController("c1", fc)
	defer func() {
		gateClose() // 兜底：测试中途失败时也释放 gate，防止 Unregister 阻塞
		m.UnregisterController("c1")
	}()

	ticker := clock.LatestTicker()

	// 触发一次 tick，等待 Status() 被调用（阻塞在 gate 上）
	ticker.Fire(clock.Now())
	waitForStatusCall(t, statusCh, "first poll")

	// Status() 现在阻塞在 gate 上。再触发多次 tick，应被 FakeTicker 丢弃（latest-only）
	for i := 0; i < 5; i++ {
		ticker.Fire(clock.Now())
	}

	// 验证仍然只有 1 次 Status() 调用（single-flight）
	if count := fc.statusCallCount(); count != 1 {
		t.Fatalf("expected 1 Status call during blocked poll (single-flight), got %d", count)
	}

	// 释放 gate，Status() 返回，goroutine 回到 select 等待下一 tick
	gateClose()

	// 触发新 tick，验证 polling 恢复
	ticker.Fire(clock.Now())
	waitForStatusCall(t, statusCh, "poll after gate release")

	if count := fc.statusCallCount(); count < 2 {
		t.Fatalf("expected at least 2 Status calls after gate release, got %d", count)
	}
}

// TestMonitorUnregisterStopsPolling 验证 UnregisterController 停止轮询 goroutine，
// 后续 tick 不再触发 Status()。
//
// spec: Unregister 后该控制器不再采集；Slice 8 在此基础上验证无 goroutine 泄漏。
func TestMonitorUnregisterStopsPolling(t *testing.T) {
	clock := NewFakeClock(time.Now())
	m := NewMotionStatusMonitor(DefaultConfig(), clock, DefaultFreshnessPolicy{
		StaleThreshold: 1 * time.Second,
	})
	fc := newFakeController("c1")
	statusCh := make(chan struct{}, 16)
	fc.statusCalled = statusCh

	m.RegisterController("c1", fc)

	ticker := clock.LatestTicker()
	ticker.Fire(clock.Now())
	waitForStatusCall(t, statusCh, "poll before unregister")

	// Unregister 应同步等待 goroutine 退出
	m.UnregisterController("c1")

	// Unregister 返回后 goroutine 已退出，再触发 tick 不应触发 Status()
	before := fc.statusCallCount()
	for i := 0; i < 5; i++ {
		ticker.Fire(clock.Now())
	}
	// 给 goroutine 一个宽限期（如果还在运行，会处理 tick）；正常情况应立即返回
	// 此处不能用 time.Sleep 断言时序——但可短暂等待让潜在残留 goroutine 暴露
	// 改用 statusCh 探测：若 50ms 内无信号，认为 goroutine 已退出
	select {
	case <-statusCh:
		t.Fatalf("polling continued after Unregister")
	case <-time.After(50 * time.Millisecond):
		// 期望路径：goroutine 已退出，无新 Status() 调用
	}
	after := fc.statusCallCount()
	if after != before {
		t.Fatalf("Status calls increased after Unregister: %d -> %d", before, after)
	}
}

// TestMonitorPublishesSnapshotAfterPoll 验证 pollOnce 完成后 Latest 返回新快照。
//
// spec Interface Contract: Latest 返回深拷贝的聚合视图，包含最新发布的快照。
func TestMonitorPublishesSnapshotAfterPoll(t *testing.T) {
	clock := NewFakeClock(time.Now())
	m := NewMotionStatusMonitor(DefaultConfig(), clock, DefaultFreshnessPolicy{
		StaleThreshold: 1 * time.Second,
	})
	fc := newFakeController("c1")
	fc.setStatus(core.ControllerStatus{
		ID:        "c1",
		Connected: true,
		Axes:      []core.AxisStatus{{Name: core.AxisX, Position: 42.0, Moving: false}},
	})
	statusCh := make(chan struct{}, 16)
	fc.statusCalled = statusCh

	m.RegisterController("c1", fc)
	defer m.UnregisterController("c1")

	// 首帧前 Latest 应返回空
	snap := m.Latest()
	if len(snap.Controllers) != 0 {
		t.Fatalf("before first poll, len(Controllers) = %d, want 0", len(snap.Controllers))
	}

	// 触发采集
	ticker := clock.LatestTicker()
	ticker.Fire(clock.Now())
	waitForStatusCall(t, statusCh, "first poll")

	// 等待发布完成——pollOnce 在 Status() 返回后写入 lastSnap。
	// 由于发布与 Status() 返回不在同一原子步骤，需短暂同步等待。
	// 用 Latest 重试 + 短超时，避免 time.Sleep 固定时序
	deadline := time.After(500 * time.Millisecond)
	for {
		snap = m.Latest()
		if len(snap.Controllers) == 1 && snap.Controllers[0].Status.Axes[0].Position == 42.0 {
			break
		}
		select {
		case <-deadline:
			t.Fatalf("snapshot not published within 500ms: len=%d", len(snap.Controllers))
		case <-time.After(2 * time.Millisecond):
		}
	}

	// 验证快照内容
	if snap.Controllers[0].ControllerID != "c1" {
		t.Fatalf("ControllerID = %q, want c1", snap.Controllers[0].ControllerID)
	}
	if snap.Controllers[0].Status.Axes[0].Position != 42.0 {
		t.Fatalf("Position = %v, want 42.0", snap.Controllers[0].Status.Axes[0].Position)
	}
	if snap.Controllers[0].Sequence != 1 {
		t.Fatalf("Sequence = %d, want 1", snap.Controllers[0].Sequence)
	}
	if snap.Controllers[0].SucceededAt.IsZero() {
		t.Fatalf("SucceededAt should be set on success")
	}
	// 聚合 Sequence 与 PublishedAt 也应在发布时更新
	if snap.Sequence == 0 {
		t.Fatalf("aggregate Sequence should be > 0 after publish")
	}
	if snap.PublishedAt.IsZero() {
		t.Fatalf("PublishedAt should be set after publish")
	}
}

// TestMonitorPreservesSucceededAtOnError 验证采集失败时保留上一轮 SucceededAt。
//
// spec Failure Semantics: "连续失败只更新 AttemptedAt 与 Err，不更新 SucceededAt"。
// 这样单次失败不立即推翻可信状态，消费者通过 Freshness 仍按 SucceededAt 判定新鲜度。
func TestMonitorPreservesSucceededAtOnError(t *testing.T) {
	clock := NewFakeClock(time.Now())
	m := NewMotionStatusMonitor(DefaultConfig(), clock, DefaultFreshnessPolicy{
		StaleThreshold: 1 * time.Second,
	})
	fc := newFakeController("c1")
	statusCh := make(chan struct{}, 16)
	fc.statusCalled = statusCh

	m.RegisterController("c1", fc)
	defer m.UnregisterController("c1")

	ticker := clock.LatestTicker()

	// 首次成功采集
	ticker.Fire(clock.Now())
	waitForStatusCall(t, statusCh, "first success")
	waitForSnapshot(t, m, "c1", 1)

	firstSnap, ok := m.LatestController("c1")
	if !ok {
		t.Fatal("expected first snapshot")
	}
	if firstSnap.Err != nil {
		t.Fatalf("first poll should succeed, got err: %v", firstSnap.Err)
	}
	firstSucceeded := firstSnap.SucceededAt

	// 推进时钟，确保第二次采集的 AttemptedAt 与首次不同
	// FakeClock.Now() 不会自动推进，必须显式 Advance
	clock.Advance(10 * time.Millisecond)

	// 第二次采集失败
	fc.setErr(errFakeStatusFailure)
	ticker.Fire(clock.Now())
	waitForStatusCall(t, statusCh, "second failure")
	waitForSnapshot(t, m, "c1", 2)

	secondSnap, ok := m.LatestController("c1")
	if !ok {
		t.Fatal("expected second snapshot")
	}
	if secondSnap.Err == nil {
		t.Fatal("second poll should have err")
	}
	// SucceededAt 应保留首帧的值
	if !secondSnap.SucceededAt.Equal(firstSucceeded) {
		t.Fatalf("SucceededAt changed on failure: %v -> %v", firstSucceeded, secondSnap.SucceededAt)
	}
	// AttemptedAt 应更新为新时间
	if !secondSnap.AttemptedAt.After(firstSnap.AttemptedAt) {
		t.Fatalf("AttemptedAt should advance: %v -> %v", firstSnap.AttemptedAt, secondSnap.AttemptedAt)
	}
}

// waitForSnapshot 轮询 LatestController 直到目标 sequence 出现或超时。
//
// 用途：pollOnce 在 Status() 返回后异步写入 lastSnap；测试需等待写入完成。
// 轮询而非 time.Sleep 固定时序，避免 flaky。
func waitForSnapshot(t *testing.T, m *MotionStatusMonitor, id string, wantSeq uint64) {
	t.Helper()
	deadline := time.After(500 * time.Millisecond)
	for {
		snap, ok := m.LatestController(id)
		if ok && snap.Sequence >= wantSeq {
			return
		}
		select {
		case <-deadline:
			t.Fatalf("snapshot for %s seq=%d not published within 500ms", id, wantSeq)
		case <-time.After(time.Millisecond):
		}
	}
}

// errFakeStatusFailure 是测试用错误，模拟硬件采集失败。
var errFakeStatusFailure = newFakeStatusError("fake status failure")

type fakeStatusError struct{ msg string }

func newFakeStatusError(msg string) error { return &fakeStatusError{msg: msg} }
func (e *fakeStatusError) Error() string  { return e.msg }
