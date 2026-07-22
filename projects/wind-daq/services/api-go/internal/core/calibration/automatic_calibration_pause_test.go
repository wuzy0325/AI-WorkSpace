package calibration

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"wind-daq/services/api-go/internal/core/traversal"
)

// pauseTestRuntime 可控运行时：MoveToPosition/WaitForMotionComplete/StopMotion
// 均被记录，并可通过 block 暂停 WaitForMotionComplete 直到外部信号触发，
// 模拟"运动中途被暂停打断"的场景。
type pauseTestRuntime struct {
	mu sync.Mutex

	values      map[string]float64
	moves       []string
	stopCalls   int
	waitStarted chan struct{} // WaitForMotionComplete 进入即关闭（仅一次）
	releaseOnce sync.Once
	releaseWait chan struct{} // 外部关闭以放行 WaitForMotionComplete
	waitBlocked bool
}

func newPauseTestRuntime() *pauseTestRuntime {
	return &pauseTestRuntime{
		values:      make(map[string]float64),
		waitStarted: make(chan struct{}),
		releaseWait: make(chan struct{}),
	}
}

func (r *pauseTestRuntime) GetChannelValue(deviceID string, channelIndex int) (float64, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	v, ok := r.values[deviceID]
	return v, ok
}

func (r *pauseTestRuntime) GetLatestTimestamp(_ string) (int64, bool) { return 0, false }

// IsAcquiring 测试 mock：默认返回 true（在采集），保持既有超时失败行为。
// 需要模拟"用户停采集"场景的测试可覆盖本方法或使用专用 mock。
func (r *pauseTestRuntime) IsAcquiring(_ string) bool { return true }

func (r *pauseTestRuntime) MoveToPosition(axis MotionAxisConfig, position float64) error {
	r.mu.Lock()
	r.moves = append(r.moves, axis.Name)
	r.mu.Unlock()
	return nil
}

func (r *pauseTestRuntime) WaitForMotionComplete() (bool, traversal.MotionInterruptReason, *traversal.MotionSafetyFailure) {
	// 仅在第一次调用时阻塞，模拟运动进行中；后续调用（如重跑）直接返回。
	r.mu.Lock()
	shouldBlock := !r.waitBlocked
	r.waitBlocked = true
	r.mu.Unlock()

	if shouldBlock {
		close(r.waitStarted)
		select {
		case <-r.releaseWait:
		case <-time.After(5 * time.Second):
			// 测试超时保护
		}
	}
	return true, traversal.MotionInterruptNone, nil
}

func (r *pauseTestRuntime) StopMotion() error {
	r.mu.Lock()
	r.stopCalls++
	r.mu.Unlock()
	// 模拟运动停止：放行被阻塞的 WaitForMotionComplete（幂等，仅关闭一次）。
	r.releaseOnce.Do(func() { close(r.releaseWait) })
	return nil
}

func (r *pauseTestRuntime) movesSnapshot() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]string, len(r.moves))
	copy(out, r.moves)
	return out
}

func (r *pauseTestRuntime) stopCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.stopCalls
}

// noopAlgorithm 最小 Algorithm 实现，供暂停测试使用。
type noopAlgorithm struct{}

func (noopAlgorithm) Type() CalibrationType { return TypeTotalPressure }
func (noopAlgorithm) AcquireData(point CalPoint, _ ChannelValueReader, _ int) (DataPoint, error) {
	return &PointResult{PointIndex: point.ID}, nil
}
func (noopAlgorithm) AcquireDataWithConfig(point CalPoint, _ ChannelValueReader, _ Config, _ func() bool, _ func(current, total int)) (DataPoint, error) {
	return &PointResult{PointIndex: point.ID}, nil
}
func (noopAlgorithm) ValidateConfig(Config) error { return nil }

// TestPauseTriggersStopMotion 验证暂停时立刻下发 StopMotion。
// 通过让 WaitForMotionComplete 阻塞模拟运动进行中，暂停后应观察到 stopCalls>0。
func TestPauseTriggersStopMotion(t *testing.T) {
	rt := newPauseTestRuntime()
	config := Config{
		TaskID: "task-pause-stop",
		Type:   string(TypeTotalPressure),
		MotionAxes: []MotionAxisConfig{
			{Name: "α", ControllerID: "motion-1", Axis: "X"},
		},
		Points: []CalPoint{{ID: 1, Coordinates: map[string]float64{"α": 10}}},
	}
	engine := NewAutomaticCalibration(config, nil, rt, nil, nil)
	engine.SetTaskID(config.TaskID)

	go func() { _ = engine.Start(noopAlgorithm{}) }()

	// 等到 WaitForMotionComplete 被调用（运动进行中）
	select {
	case <-rt.waitStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("WaitForMotionComplete 未被调用")
	}

	engine.Pause()

	// Pause 应立即下发 StopMotion
	if got := rt.stopCount(); got < 1 {
		t.Fatalf("暂停后应调用 StopMotion 至少 1 次，实际 %d", got)
	}
	if !engine.IsPaused() {
		t.Fatal("暂停后 IsPaused 应为 true")
	}

	// 等待 WaitForMotionComplete 返回后，processPoint 第一个检查点会捕获暂停，
	// runCalibrationLoop 进入 waitWhilePaused。这里直接 Stop 收尾，避免 goroutine 泄漏。
	engine.Stop()
}

// TestPauseMidPointRerunsPointOnResume 验证点中暂停后恢复会重跑同一点。
// 第一点运动中暂停 → StopMotion 放行 wait → processPoint 返回 errPointAborted →
// 循环回退重跑第一点（第二次 WaitForMotionComplete 直接返回 nil）→ 采集成功 → 进入第二点。
func TestPauseMidPointRerunsPointOnResume(t *testing.T) {
	rt := newPauseTestRuntime()
	config := Config{
		TaskID: "task-pause-rerun",
		Type:   string(TypeTotalPressure),
		MotionAxes: []MotionAxisConfig{
			{Name: "α", ControllerID: "motion-1", Axis: "X"},
		},
		Points: []CalPoint{
			{ID: 1, Coordinates: map[string]float64{"α": 10}},
			{ID: 2, Coordinates: map[string]float64{"α": 20}},
		},
	}
	engine := NewAutomaticCalibration(config, nil, rt, nil, nil)
	engine.SetTaskID(config.TaskID)

	done := make(chan error, 1)
	go func() { done <- engine.Start(noopAlgorithm{}) }()

	// 等待第一点运动开始
	select {
	case <-rt.waitStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("第一点 WaitForMotionComplete 未被调用")
	}

	engine.Pause()

	// 给一点时间让 waitWhilePaused 生效
	time.Sleep(100 * time.Millisecond)
	engine.Resume()

	// 等待校准完成
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("校准应成功完成，实际错误: %v", err)
		}
	case <-time.After(3 * time.Second):
		engine.Stop()
		t.Fatal("校准未在超时前完成")
	}

	// 第一点应被移动两次（初次 + 重跑），第二点移动一次，共 3 次 MoveToPosition
	moves := rt.movesSnapshot()
	// 每个 processPoint 对每轴调一次 MoveToPosition，五孔以外的类型走默认 moveToPoint。
	// 重跑第一点 → α 出现 2 次；第二点 → α 出现 1 次。
	wantMoves := 3
	if len(moves) != wantMoves {
		t.Fatalf("MoveToPosition 调用次数应为 %d（重跑第1点×2 + 第2点×1），实际 %d 次: %v", wantMoves, len(moves), moves)
	}

	dps := engine.GetDataPoints()
	if len(dps) != 2 {
		t.Fatalf("应采集 2 个数据点，实际 %d", len(dps))
	}
}

// TestPauseBetweenPointsAdvancesOnResume 验证点间暂停（当前点已完成）
// 恢复后正常进入下一点，不重跑已完成的点。
func TestPauseBetweenPointsAdvancesOnResume(t *testing.T) {
	// 使用不阻塞的 runtime：WaitForMotionComplete 立即返回，
	// 这样第一点会完整跑完，随后在主循环顶部 waitWhilePaused 阻塞。
	rt := &nonBlockingRuntime{}
	config := Config{
		TaskID: "task-pause-between",
		Type:   string(TypeTotalPressure),
		MotionAxes: []MotionAxisConfig{
			{Name: "α", ControllerID: "motion-1", Axis: "X"},
		},
		Points: []CalPoint{
			{ID: 1, Coordinates: map[string]float64{"α": 10}},
			{ID: 2, Coordinates: map[string]float64{"α": 20}},
		},
	}
	engine := NewAutomaticCalibration(config, nil, rt, nil, nil)
	engine.SetTaskID(config.TaskID)

	done := make(chan error, 1)
	go func() { done <- engine.Start(noopAlgorithm{}) }()

	// 等待第一点完成并进入 waitWhilePaused：观察数据点数量到达 1。
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if len(engine.GetDataPoints()) >= 1 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if len(engine.GetDataPoints()) < 1 {
		engine.Stop()
		t.Fatal("第一点未在超时前完成")
	}

	engine.Pause()
	// 点间暂停不应触发 StopMotion（无运动轴 Moving），但允许调用——这里主要验证不重跑。
	time.Sleep(100 * time.Millisecond)
	engine.Resume()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("校准应成功完成，实际错误: %v", err)
		}
	case <-time.After(3 * time.Second):
		engine.Stop()
		t.Fatal("校准未在超时前完成")
	}

	// 第一点 + 第二点各移动一次，共 2 次（未重跑）
	if got := len(rt.moves); got != 2 {
		t.Fatalf("MoveToPosition 应为 2 次（无重跑），实际 %d", got)
	}
	if len(engine.GetDataPoints()) != 2 {
		t.Fatalf("应采集 2 个数据点，实际 %d", len(engine.GetDataPoints()))
	}
}

// nonBlockingRuntime Wait 立即返回，用于点间暂停场景。
type nonBlockingRuntime struct {
	moves     []string
	stopCalls int
}

func (r *nonBlockingRuntime) GetChannelValue(string, int) (float64, bool) { return 0, false }
func (r *nonBlockingRuntime) GetLatestTimestamp(string) (int64, bool)   { return 0, false }
func (r *nonBlockingRuntime) IsAcquiring(_ string) bool                 { return true }
func (r *nonBlockingRuntime) MoveToPosition(axis MotionAxisConfig, _ float64) error {
	r.moves = append(r.moves, axis.Name)
	return nil
}
func (r *nonBlockingRuntime) WaitForMotionComplete() (bool, traversal.MotionInterruptReason, *traversal.MotionSafetyFailure) {
	return true, traversal.MotionInterruptNone, nil
}
func (r *nonBlockingRuntime) StopMotion() error {
	r.stopCalls++
	return nil
}

// waitForCondition 轮询 cond 直到为 true，超时则 fail。用于替代长时间 sleep 猜测状态。
func waitForCondition(t *testing.T, timeout time.Duration, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("等待条件超时")
}

// TestStartWithContextAlreadyCancelledReturnsImmediately 验证 ctx 在启动前已取消时
// StartWithContext 立即返回 context.Canceled，不进入运行态、不处理任何测点。
func TestStartWithContextAlreadyCancelledReturnsImmediately(t *testing.T) {
	rt := &nonBlockingRuntime{}
	config := Config{
		TaskID: "task-ctx-precancel",
		Type:   string(TypeTotalPressure),
		MotionAxes: []MotionAxisConfig{
			{Name: "α", ControllerID: "motion-1", Axis: "X"},
		},
		Points: []CalPoint{{ID: 1, Coordinates: map[string]float64{"α": 10}}},
	}
	engine := NewAutomaticCalibration(config, nil, rt, nil, nil)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // 启动前已取消

	start := time.Now()
	err := engine.StartWithContext(ctx, noopAlgorithm{})
	elapsed := time.Since(start)

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("应返回 context.Canceled，实际: %v", err)
	}
	if elapsed > time.Second {
		t.Fatalf("已取消的 ctx 应立即返回，实际耗时 %v", elapsed)
	}
	if engine.IsRunning() {
		t.Fatal("ctx 已取消时不应进入运行态")
	}
	if len(rt.moves) != 0 {
		t.Fatalf("ctx 已取消时不应发生任何运动，实际 moves=%v", rt.moves)
	}
}

// TestStartWithContextCancelsDuringDwell 验证长驻留（dwell）等待中取消 ctx 能及时退出。
// DwellTimeMs 设为 60s：若驻留仍是不可取消的无条件 sleep，本测试必然超时失败。
func TestStartWithContextCancelsDuringDwell(t *testing.T) {
	rt := &nonBlockingRuntime{}
	config := Config{
		TaskID:      "task-ctx-dwell",
		Type:        string(TypeTotalPressure),
		DwellTimeMs: 60_000, // 长驻留：取消前不会自然结束
		Points:      []CalPoint{{ID: 1, Coordinates: map[string]float64{"α": 10}}},
	}
	engine := NewAutomaticCalibration(config, nil, rt, nil, nil)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- engine.StartWithContext(ctx, noopAlgorithm{}) }()

	// 等引擎进入运行态：无运动轴、无闸门，processPoint 随即进入 60s 驻留
	waitForCondition(t, 2*time.Second, engine.IsRunning)
	cancel()

	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("驻留中取消应返回 context.Canceled，实际: %v", err)
		}
	case <-time.After(2 * time.Second):
		engine.Stop()
		t.Fatal("驻留等待未在取消后及时退出")
	}
	if len(engine.GetDataPoints()) != 0 {
		t.Fatalf("驻留中取消不应采集到数据点，实际 %d 个", len(engine.GetDataPoints()))
	}
}

// TestStartWithContextCancelsWhilePaused 验证暂停等待（waitWhilePaused）中取消 ctx 能及时退出。
// 用 pauseTestRuntime 阻塞首次 WaitForMotionComplete 构造确定性的暂停窗口：
// 运动中暂停 → StopMotion 放行 → ErrPointAborted → 循环顶部进入 waitWhilePaused。
func TestStartWithContextCancelsWhilePaused(t *testing.T) {
	rt := newPauseTestRuntime()
	config := Config{
		TaskID: "task-ctx-pause",
		Type:   string(TypeTotalPressure),
		MotionAxes: []MotionAxisConfig{
			{Name: "α", ControllerID: "motion-1", Axis: "X"},
		},
		Points: []CalPoint{{ID: 1, Coordinates: map[string]float64{"α": 10}}},
	}
	engine := NewAutomaticCalibration(config, nil, rt, nil, nil)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- engine.StartWithContext(ctx, noopAlgorithm{}) }()

	// 等运动开始（WaitForMotionComplete 阻塞中），随后暂停：
	// StopMotion 放行 wait → processPoint 返回 ErrPointAborted → 循环顶部 waitWhilePaused 阻塞
	select {
	case <-rt.waitStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("WaitForMotionComplete 未被调用")
	}
	engine.Pause()
	waitForCondition(t, 2*time.Second, engine.IsPaused)

	cancel()

	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("暂停等待中取消应返回 context.Canceled，实际: %v", err)
		}
	case <-time.After(2 * time.Second):
		engine.Stop()
		t.Fatal("暂停等待未在取消后及时退出")
	}
}

// TestStartWithContextCancelsDuringGateWait 验证球罐闸门等待中取消 ctx 能及时退出。
// 稳定时间通道读取恒失败（nonBlockingRuntime 返回 ok=false），闸门条件永不满足；
// 若 gate 轮询不可取消，只能在 TimeoutSec=300s 后以超时错误结束，本测试将失败。
func TestStartWithContextCancelsDuringGateWait(t *testing.T) {
	rt := &nonBlockingRuntime{}
	config := Config{
		TaskID: "task-ctx-gate",
		Type:   string(TypeTotalPressure),
		Points: []CalPoint{{ID: 1, Coordinates: map[string]float64{"α": 10}}},
		SphereTankGate: &SphereTankGateConfig{
			Enabled:           true,
			WaitTimeSec:       100,
			TimeoutSec:        300,
			StableTimeChannel: ChannelRef{DeviceID: "dev-gate", ChannelIndex: 0},
		},
	}
	engine := NewAutomaticCalibration(config, nil, rt, nil, nil)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- engine.StartWithContext(ctx, noopAlgorithm{}) }()

	// 等进入运行态并给 gate 轮询（100ms 周期）一个生效窗口
	waitForCondition(t, 2*time.Second, engine.IsRunning)
	time.Sleep(150 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("球罐闸门等待中取消应返回 context.Canceled，实际: %v", err)
		}
	case <-time.After(2 * time.Second):
		engine.Stop()
		t.Fatal("球罐闸门等待未在取消后及时退出")
	}
}