package monitor

import (
	"testing"
	"time"

	"shared.local/device-sdk/go/motion/core"
)

// currentIntervalFromState 是白盒测试辅助，读取指定控制器当前 ticker 间隔。
//
// Slice 7 自适应频率的核心可观测信号：pollLoop 切换 ticker 时更新此字段，
// 测试通过它确定性断言间隔已切换，避免依赖 time.Sleep 或 FakeTicker 类型断言。
func currentIntervalFromState(t *testing.T, m *MotionStatusMonitor, id string) (time.Duration, bool) {
	t.Helper()
	m.mu.RLock()
	cs := m.controllers[id]
	m.mu.RUnlock()
	if cs == nil {
		return 0, false
	}
	cs.mu.Lock()
	defer cs.mu.Unlock()
	return cs.currentInterval, true
}

// waitForInterval 轮询等待控制器当前间隔变为 want，或在超时后失败。
//
// 设计理由：pollLoop 切换 ticker 是异步的——pollOnce 返回后 pollLoop 才检查间隔。
// 测试通过 channel 同步等待 pollOnce 完成（waitForStatusCall/waitForSnapshot），
// 但间隔切换发生在 pollOnce 之后的 pollLoop 循环中，无直接同步信号。
// 短轮询（1ms 间隔）+ 1s 超时是确定性兜底，正常情况下间隔切换在毫秒级完成。
func waitForInterval(t *testing.T, m *MotionStatusMonitor, id string, want time.Duration, desc string) {
	t.Helper()
	deadline := time.After(1 * time.Second)
	for {
		got, ok := currentIntervalFromState(t, m, id)
		if ok && got == want {
			return
		}
		select {
		case <-deadline:
			t.Fatalf("%s: currentInterval did not become %v within 1s, got %v (ok=%v)", desc, want, got, ok)
		case <-time.After(time.Millisecond):
		}
	}
}

// adaptiveTestConfig 返回三档间隔可区分的测试配置。
//
// FastWindowInterval=50ms / MovingInterval=100ms / IdleInterval=500ms：
// 三档值不同便于测试断言切换到了正确的档位，避免默认配置中
// FastWindowInterval 与 MovingInterval 都是 100ms 导致无法区分。
func adaptiveTestConfig() Config {
	return Config{
		MovingInterval:     100 * time.Millisecond,
		IdleInterval:       500 * time.Millisecond,
		FastWindowInterval: 50 * time.Millisecond,
		FastWindowDuration: 2 * time.Second,
	}
}

// TestMonitorAdaptiveInitialIntervalIsIdle 验证注册后初始间隔为 IdleInterval。
//
// spec Polling Policy: 首帧前无 Moving 信息，用保守的 idle 间隔等首帧。
// 首帧后若无 fastWindow 且 Moving=false，间隔保持 idle。
func TestMonitorAdaptiveInitialIntervalIsIdle(t *testing.T) {
	clock := NewFakeClock(time.Now())
	m := NewMotionStatusMonitor(adaptiveTestConfig(), clock, DefaultFreshnessPolicy{
		StaleThreshold: 1 * time.Second,
	})
	fc := newFakeController("c1")
	statusCh := make(chan struct{}, 16)
	fc.statusCalled = statusCh

	m.RegisterController("c1", fc)
	defer m.UnregisterController("c1")

	// 初始间隔应为 idle（首帧前）
	got, ok := currentIntervalFromState(t, m, "c1")
	if !ok {
		t.Fatal("controller not found")
	}
	if got != 500*time.Millisecond {
		t.Fatalf("initial interval = %v, want 500ms (idle)", got)
	}

	// 首帧 Moving=false
	ticker := clock.LatestTicker()
	ticker.Fire(clock.Now())
	waitForStatusCall(t, statusCh, "initial poll")
	waitForSnapshot(t, m, "c1", 1)

	// 首帧后 Moving=false 且无 fastWindow，间隔应保持 idle
	waitForInterval(t, m, "c1", 500*time.Millisecond, "after first frame, still idle (Moving=false)")
}

// TestMonitorAdaptiveFastWindowAfterMove 验证 Notify(Move) 后间隔切换为 FastWindowInterval。
//
// spec NotifyCommandExecuted: "快速窗口期间轮询频率按 Polling Policy 的 moving 间隔执行"。
// spec Polling Policy: "命令后快速观察窗口 100ms，持续 2s"。
// 测试：Notify(Move) 设置 fastWindowUntil = now + 2s，refresh 触发的 pollOnce 完成后，
// pollLoop 检测到 fastWindow 激活，间隔切换为 FastWindowInterval。
func TestMonitorAdaptiveFastWindowAfterMove(t *testing.T) {
	clock := NewFakeClock(time.Now())
	m := NewMotionStatusMonitor(adaptiveTestConfig(), clock, DefaultFreshnessPolicy{
		StaleThreshold: 1 * time.Second,
	})
	fc := newFakeController("c1")
	statusCh := make(chan struct{}, 32)
	fc.statusCalled = statusCh

	m.RegisterController("c1", fc)
	defer m.UnregisterController("c1")

	// 首帧 Moving=false，间隔 idle
	ticker := clock.LatestTicker()
	ticker.Fire(clock.Now())
	waitForStatusCall(t, statusCh, "initial poll")
	waitForSnapshot(t, m, "c1", 1)
	waitForInterval(t, m, "c1", 500*time.Millisecond, "initial idle")

	// Notify(Move) 激活 fastWindow（2s）+ 触发 refresh
	m.NotifyCommandExecuted("c1", CmdKindMove)
	waitForStatusCall(t, statusCh, "move refresh poll")

	// refresh 触发的 pollOnce 完成后，pollLoop 检测到 fastWindow 激活，
	// 间隔切换为 FastWindowInterval（50ms，与 MovingInterval 100ms 区分）
	waitForInterval(t, m, "c1", 50*time.Millisecond, "fast window active after Move")
}

// TestMonitorAdaptiveFastWindowExpires 验证 fastWindow 过期后间隔回落到 IdleInterval。
//
// spec Polling Policy: "窗口结束后回落到 idle 间隔"。
// 测试：Notify(Move) 后间隔为 FastWindowInterval；推进时钟超过 FastWindowDuration，
// 触发新一轮 pollOnce，pollLoop 检测到 fastWindow 已过期，间隔回落到 IdleInterval。
func TestMonitorAdaptiveFastWindowExpires(t *testing.T) {
	cfg := adaptiveTestConfig()
	cfg.FastWindowDuration = 100 * time.Millisecond // 缩短到 100ms 便于测试
	clock := NewFakeClock(time.Now())
	m := NewMotionStatusMonitor(cfg, clock, DefaultFreshnessPolicy{
		StaleThreshold: 1 * time.Second,
	})
	fc := newFakeController("c1")
	statusCh := make(chan struct{}, 32)
	fc.statusCalled = statusCh

	m.RegisterController("c1", fc)
	defer m.UnregisterController("c1")

	// 首帧
	ticker := clock.LatestTicker()
	ticker.Fire(clock.Now())
	waitForStatusCall(t, statusCh, "initial poll")
	waitForSnapshot(t, m, "c1", 1)
	waitForInterval(t, m, "c1", 500*time.Millisecond, "initial idle")

	// Notify(Move) 激活 fastWindow（100ms）
	m.NotifyCommandExecuted("c1", CmdKindMove)
	waitForStatusCall(t, statusCh, "move refresh poll")
	waitForInterval(t, m, "c1", 50*time.Millisecond, "fast window active")

	// 推进时钟超过 fastWindowDuration
	clock.Advance(200 * time.Millisecond)

	// 触发新一轮 pollOnce——此时 fastWindow 已过期
	// ticker 现在是 FastWindowInterval 的，Fire 它触发 pollOnce
	ticker2 := clock.LatestTicker()
	if ticker2 == nil {
		t.Fatal("expected ticker after fast window switch")
	}
	ticker2.Fire(clock.Now())
	waitForStatusCall(t, statusCh, "poll after fast window expired")

	// fastWindow 已过期，Moving=false，间隔应回落到 IdleInterval
	waitForInterval(t, m, "c1", 500*time.Millisecond, "fast window expired, back to idle")
}

// TestMonitorAdaptiveMovingAxisSwitchesToMovingInterval 验证轴 Moving=true 切换到 MovingInterval。
//
// spec Polling Policy: "任一轴 Moving 或 Compensating 100ms"。
// 测试：fakeController 返回 Moving=true，首帧后 pollLoop 检测到 Moving，
// 间隔切换为 MovingInterval（100ms）。注意 fastWindow 未激活，所以不是 FastWindowInterval（50ms）。
func TestMonitorAdaptiveMovingAxisSwitchesToMovingInterval(t *testing.T) {
	clock := NewFakeClock(time.Now())
	m := NewMotionStatusMonitor(adaptiveTestConfig(), clock, DefaultFreshnessPolicy{
		StaleThreshold: 1 * time.Second,
	})
	fc := newFakeController("c1")
	statusCh := make(chan struct{}, 32)
	fc.statusCalled = statusCh

	// 设置 Status 返回 Moving=true
	fc.setStatus(core.ControllerStatus{
		ID:        "c1",
		Name:      "c1",
		Connected: true,
		Axes:      []core.AxisStatus{{Name: core.AxisX, Position: 0, Moving: true}},
	})

	m.RegisterController("c1", fc)
	defer m.UnregisterController("c1")

	// 首帧 Moving=true
	ticker := clock.LatestTicker()
	ticker.Fire(clock.Now())
	waitForStatusCall(t, statusCh, "initial poll with Moving=true")
	waitForSnapshot(t, m, "c1", 1)

	// Moving=true，间隔应切换为 MovingInterval（100ms）
	// 注意 fastWindow 未激活（无 Notify Move），所以不是 FastWindowInterval（50ms）
	waitForInterval(t, m, "c1", 100*time.Millisecond, "moving axis")
}

// TestMonitorAdaptiveMovingFalseKeepsIdle 验证 Moving=false 且无 fastWindow 保持 idle。
//
// spec Polling Policy: "全部连接控制器空闲 500ms"。
// 测试：首帧 Moving=false，间隔保持 IdleInterval；多次 Fire 不改变。
func TestMonitorAdaptiveMovingFalseKeepsIdle(t *testing.T) {
	clock := NewFakeClock(time.Now())
	m := NewMotionStatusMonitor(adaptiveTestConfig(), clock, DefaultFreshnessPolicy{
		StaleThreshold: 1 * time.Second,
	})
	fc := newFakeController("c1")
	statusCh := make(chan struct{}, 32)
	fc.statusCalled = statusCh

	m.RegisterController("c1", fc)
	defer m.UnregisterController("c1")

	// 首帧 Moving=false
	ticker := clock.LatestTicker()
	ticker.Fire(clock.Now())
	waitForStatusCall(t, statusCh, "initial poll")
	waitForSnapshot(t, m, "c1", 1)
	waitForInterval(t, m, "c1", 500*time.Millisecond, "idle after first frame")

	// 再触发几轮，间隔应保持 idle
	for i := 0; i < 3; i++ {
		ticker.Fire(clock.Now())
		waitForStatusCall(t, statusCh, "subsequent poll")
	}
	waitForInterval(t, m, "c1", 500*time.Millisecond, "still idle after multiple polls")
}

// TestMonitorAdaptiveFastWindowPriorityOverMoving 验证 fastWindow 优先级高于 Moving。
//
// spec Polling Policy 优先级（隐含）：fastWindow 激活时用 FastWindowInterval，
// 即使 Moving=false 也用 FastWindowInterval；fastWindow 过期后才看 Moving。
// 测试：Moving=false + Notify(Move) 激活 fastWindow，间隔应为 FastWindowInterval（50ms），
// 而非 IdleInterval（500ms）。
func TestMonitorAdaptiveFastWindowPriorityOverMoving(t *testing.T) {
	clock := NewFakeClock(time.Now())
	m := NewMotionStatusMonitor(adaptiveTestConfig(), clock, DefaultFreshnessPolicy{
		StaleThreshold: 1 * time.Second,
	})
	fc := newFakeController("c1")
	statusCh := make(chan struct{}, 32)
	fc.statusCalled = statusCh

	// Moving=false
	fc.setStatus(core.ControllerStatus{
		ID:        "c1",
		Name:      "c1",
		Connected: true,
		Axes:      []core.AxisStatus{{Name: core.AxisX, Position: 0, Moving: false}},
	})

	m.RegisterController("c1", fc)
	defer m.UnregisterController("c1")

	// 首帧 Moving=false，间隔 idle
	ticker := clock.LatestTicker()
	ticker.Fire(clock.Now())
	waitForStatusCall(t, statusCh, "initial poll")
	waitForSnapshot(t, m, "c1", 1)
	waitForInterval(t, m, "c1", 500*time.Millisecond, "idle (Moving=false)")

	// Notify(Move) 激活 fastWindow
	m.NotifyCommandExecuted("c1", CmdKindMove)
	waitForStatusCall(t, statusCh, "move refresh poll")

	// fastWindow 优先：间隔应为 FastWindowInterval（50ms），不是 IdleInterval（500ms）
	// 即使 Moving=false，fastWindow 激活期间用 FastWindowInterval
	waitForInterval(t, m, "c1", 50*time.Millisecond, "fast window takes priority over idle")
}

// 编译期断言：core 包被使用（setStatus 调用）。
var _ = core.AxisX
