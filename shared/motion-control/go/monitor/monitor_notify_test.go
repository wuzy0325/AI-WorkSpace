package monitor

import (
	"sync"
	"testing"
	"time"

	"shared.local/device-sdk/go/motion/core"
)

// fastWindowUntilFromState 是白盒测试辅助，读取指定控制器的 fastWindowUntil 字段。
// 同包测试可直接访问 cs.mu 与 cs.fastWindowUntil，但通过 helper 统一上锁避免遗漏。
func fastWindowUntilFromState(t *testing.T, m *MotionStatusMonitor, id string) (time.Time, bool) {
	t.Helper()
	m.mu.RLock()
	cs := m.controllers[id]
	m.mu.RUnlock()
	if cs == nil {
		return time.Time{}, false
	}
	cs.mu.Lock()
	defer cs.mu.Unlock()
	return cs.fastWindowUntil, true
}

// generationFromState 是白盒测试辅助，读取指定控制器的 generation。
func generationFromState(t *testing.T, m *MotionStatusMonitor, id string) (uint64, bool) {
	t.Helper()
	m.mu.RLock()
	cs := m.controllers[id]
	m.mu.RUnlock()
	if cs == nil {
		return 0, false
	}
	cs.mu.Lock()
	defer cs.mu.Unlock()
	return cs.generation, true
}

// TestMonitorNotifyMoveTriggersFastWindowAndRefresh 验证 CmdKindMove 触发 2s 快速窗口 + 一轮额外采集。
//
// spec NotifyCommandExecuted: "运动命令（Move/Jog/Home）... 触发该 controller 的 2s 快速观察窗口 + 一轮额外采集"。
// 测试：Notify(Move) 后 fastWindowUntil 应为 now + 2s，且 Status 调用次数 +1。
func TestMonitorNotifyMoveTriggersFastWindowAndRefresh(t *testing.T) {
	clock := NewFakeClock(time.Now())
	m := NewMotionStatusMonitor(DefaultConfig(), clock, DefaultFreshnessPolicy{
		StaleThreshold: 1 * time.Second,
	})
	fc := newFakeController("c1")
	statusCh := make(chan struct{}, 16)
	fc.statusCalled = statusCh

	m.RegisterController("c1", fc)
	defer m.UnregisterController("c1")

	// 首帧建立
	ticker := clock.LatestTicker()
	ticker.Fire(clock.Now())
	waitForStatusCall(t, statusCh, "initial poll")
	waitForSnapshot(t, m, "c1", 1)

	// 推进时钟，让 Notify 后的 fastWindowUntil 与首帧时间明显不同
	clock.Advance(100 * time.Millisecond)
	nowBeforeNotify := clock.Now()

	// Notify(Move) 应设置 fastWindowUntil = now + FastWindowDuration（默认 2s）
	m.NotifyCommandExecuted("c1", CmdKindMove)

	// 等 refresh 触发的额外采集
	waitForStatusCall(t, statusCh, "notify move refresh")
	waitForSnapshot(t, m, "c1", 2)

	until, ok := fastWindowUntilFromState(t, m, "c1")
	if !ok {
		t.Fatal("controller not found")
	}
	want := nowBeforeNotify.Add(2 * time.Second)
	if !until.Equal(want) {
		t.Fatalf("fastWindowUntil = %v, want %v", until, want)
	}
}

// TestMonitorNotifyMoveExtendsFastWindow 验证连续 Move 命令延长快速窗口。
//
// spec NotifyCommandExecuted: 快速窗口语义——每次 Move 都应重置 fastWindowUntil = now + 2s，
// 而非取 max 或保持首次设置。这样连续运动命令不会让窗口意外过期。
func TestMonitorNotifyMoveExtendsFastWindow(t *testing.T) {
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
	ticker.Fire(clock.Now())
	waitForStatusCall(t, statusCh, "initial poll")
	waitForSnapshot(t, m, "c1", 1)

	// 第一次 Move
	clock.Advance(100 * time.Millisecond)
	firstNow := clock.Now()
	m.NotifyCommandExecuted("c1", CmdKindMove)
	waitForStatusCall(t, statusCh, "first move refresh")
	waitForSnapshot(t, m, "c1", 2)
	firstUntil, _ := fastWindowUntilFromState(t, m, "c1")

	// 第二次 Move，时钟推进 1s
	clock.Advance(1 * time.Second)
	secondNow := clock.Now()
	m.NotifyCommandExecuted("c1", CmdKindMove)
	waitForStatusCall(t, statusCh, "second move refresh")
	waitForSnapshot(t, m, "c1", 3)
	secondUntil, _ := fastWindowUntilFromState(t, m, "c1")

	if !firstUntil.Equal(firstNow.Add(2 * time.Second)) {
		t.Fatalf("first fastWindowUntil = %v, want %v", firstUntil, firstNow.Add(2*time.Second))
	}
	if !secondUntil.Equal(secondNow.Add(2 * time.Second)) {
		t.Fatalf("second fastWindowUntil = %v, want %v (should reset, not max)", secondUntil, secondNow.Add(2*time.Second))
	}
	if !secondUntil.After(firstUntil) {
		t.Fatalf("second fastWindowUntil (%v) should be after first (%v)", secondUntil, firstUntil)
	}
}

// TestMonitorNotifyStopOnlyRefresh 验证 CmdKindStop 仅触发单轮 refresh，不进入快速窗口。
//
// spec NotifyCommandExecuted: "Stop/EStop/Reset 命令仅触发单轮 refresh（不进入快速窗口）"。
// 测试：Notify(Stop) 不修改 fastWindowUntil（保持零值），但触发 1 轮额外采集。
func TestMonitorNotifyStopOnlyRefresh(t *testing.T) {
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
	ticker.Fire(clock.Now())
	waitForStatusCall(t, statusCh, "initial poll")
	waitForSnapshot(t, m, "c1", 1)

	// 记录 Notify 前的 fastWindowUntil
	untilBefore, _ := fastWindowUntilFromState(t, m, "c1")
	if !untilBefore.IsZero() {
		t.Fatalf("fastWindowUntil should be zero before any Move, got %v", untilBefore)
	}

	m.NotifyCommandExecuted("c1", CmdKindStop)
	waitForStatusCall(t, statusCh, "stop refresh")
	waitForSnapshot(t, m, "c1", 2)

	// fastWindowUntil 应保持零值（未进入快速窗口）
	untilAfter, _ := fastWindowUntilFromState(t, m, "c1")
	if !untilAfter.IsZero() {
		t.Fatalf("fastWindowUntil should remain zero after Stop, got %v", untilAfter)
	}
}

// TestMonitorNotifyConfigResetsGeneration 验证 CmdKindConfig 触发 generation 重置 + 单轮 refresh。
//
// spec NotifyCommandExecuted: "ApplyConfig 触发 generation 重置 + 单轮 refresh"。
// spec Data Model: "Generation 在 Disconnect/ApplyConfig 时单调递增；重连后 Sequence 重置为 0；
// monitor 必须在重连完成时清空旧 generation 的快照缓存"。
// 测试：首帧后 Notify(Config) 应递增 generation、清空 lastSnap、重置 sequence，
// 然后 refresh 触发新 generation 的首帧（seq=1）。
func TestMonitorNotifyConfigResetsGeneration(t *testing.T) {
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
	ticker.Fire(clock.Now())
	waitForStatusCall(t, statusCh, "initial poll")
	waitForSnapshot(t, m, "c1", 1)

	genBefore, _ := generationFromState(t, m, "c1")
	if genBefore != 0 {
		t.Fatalf("initial generation should be 0, got %d", genBefore)
	}

	// 设置 gate 阻塞后续 Status() 调用，确保 Notify(Config) 触发的 refresh pollOnce
	// 不会在测试检查 LatestController 前完成并写入新 lastSnap。
	// 不设置 gate 时存在 flaky：refresh pollOnce 可能在测试检查 LatestController 前完成，
	// 导致 lastSnap 已被新 generation 首帧填充，LatestController 返回 true，
	// 违反"清空旧 generation 快照缓存"的断言意图。
	fc.gate = make(chan struct{})

	// gateClose 用 sync.Once 保护，避免 defer 兜底与测试中途显式 close 重复 close panic。
	// 与 TestMonitorNotifyConfigDropsInFlightOldGenResult / TestMonitorRequestRefreshMergesWhileInFlight
	// 的 gate 关闭风格保持一致。
	var gateOnce sync.Once
	gateClose := func() { gateOnce.Do(func() { close(fc.gate) }) }
	// defer 兜底：测试中途失败时也释放 gate，防止 Unregister 阻塞在 Status() 上
	defer gateClose()

	// Notify(Config) 应递增 generation 到 1，清空 lastSnap，重置 sequence
	m.NotifyCommandExecuted("c1", CmdKindConfig)

	genAfter, _ := generationFromState(t, m, "c1")
	if genAfter != 1 {
		t.Fatalf("generation after Config should be 1, got %d", genAfter)
	}

	// 等待 refresh pollOnce 进入 Status()（阻塞在 gate 上）。
	// 这证明 refresh 已被触发，且 pollOnce 正在阻塞——此时 lastSnap 必然仍为空。
	waitForStatusCall(t, statusCh, "config refresh (blocked on gate)")

	// 此时 lastSnap 应已清空——LatestController 应返回 false。
	// gate 阻塞了 refresh pollOnce，确定性保证 lastSnap 仍为空。
	if _, ok := m.LatestController("c1"); ok {
		t.Fatal("LatestController should return false immediately after Config (lastSnap cleared)")
	}

	// 释放 gate，让 refresh pollOnce 完成，写入新 generation 的首帧
	gateClose()

	// refresh 应触发新 generation 的首帧（seq=1）
	deadline := time.After(500 * time.Millisecond)
	for {
		snap, ok := m.LatestController("c1")
		if ok && snap.Generation == 1 && snap.Sequence == 1 {
			return
		}
		select {
		case <-deadline:
			t.Fatalf("new generation first frame not published within 500ms: ok=%v", ok)
		case <-time.After(time.Millisecond):
		}
	}
}

// TestMonitorNotifyConfigDropsInFlightOldGenResult 验证 ApplyConfig 期间在途的旧 generation 结果被丢弃。
//
// spec Data Model: "generation 不匹配的在途采集结果直接丢弃，不更新时间或状态"。
// 测试：阻塞首帧 Status()，期间 Notify(Config) 递增 generation；释放后首帧结果应被丢弃，
// refresh 触发的新一轮 Status() 才是新 generation 的首帧。
func TestMonitorNotifyConfigDropsInFlightOldGenResult(t *testing.T) {
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
	ticker.Fire(clock.Now())
	waitForStatusCall(t, statusCh, "first poll (blocked on gate)")

	// Status() 阻塞在 gate 上（旧 generation=0 的在途采集）
	// 期间 Notify(Config) 递增 generation 到 1
	m.NotifyCommandExecuted("c1", CmdKindConfig)

	genAfter, _ := generationFromState(t, m, "c1")
	if genAfter != 1 {
		t.Fatalf("generation after Config should be 1, got %d", genAfter)
	}

	// 释放 gate——旧 generation 的 Status() 返回，但 pollOnce 应丢弃（generation 不匹配）
	gateClose()

	// refresh（由 Notify(Config) 触发）应产生新 generation=1 的首帧
	waitForStatusCall(t, statusCh, "config refresh after gate release")
	deadline := time.After(500 * time.Millisecond)
	for {
		snap, ok := m.LatestController("c1")
		if ok && snap.Generation == 1 && snap.Sequence == 1 {
			return
		}
		select {
		case <-deadline:
			t.Fatalf("new generation first frame not published within 500ms: ok=%v snap=%+v", ok, snap)
		case <-time.After(time.Millisecond):
		}
	}
}

// TestMonitorNotifyUnknownControllerIsNoop 验证未注册 ID 静默返回。
func TestMonitorNotifyUnknownControllerIsNoop(t *testing.T) {
	clock := NewFakeClock(time.Now())
	m := NewMotionStatusMonitor(DefaultConfig(), clock, DefaultFreshnessPolicy{
		StaleThreshold: 1 * time.Second,
	})

	// 未注册 ID 应静默返回，不 panic
	m.NotifyCommandExecuted("nonexistent", CmdKindMove)
	m.NotifyCommandExecuted("nonexistent", CmdKindStop)
	m.NotifyCommandExecuted("nonexistent", CmdKindConfig)
}

// 编译期断言：CmdKind 常量已定义。
var _ = []CommandKind{CmdKindMove, CmdKindStop, CmdKindConfig}

// 编译期断言：core 包被使用（setStatus 调用）。
var _ = core.AxisX
