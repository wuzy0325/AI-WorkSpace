package calibration

import (
	"sync"
	"testing"
	"time"
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

func (r *pauseTestRuntime) MoveToPosition(axis MotionAxisConfig, position float64) error {
	r.mu.Lock()
	r.moves = append(r.moves, axis.Name)
	r.mu.Unlock()
	return nil
}

func (r *pauseTestRuntime) WaitForMotionComplete() error {
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
	return nil
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
	engine := NewAutomaticCalibration(config, nil, rt, nil)
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
	engine := NewAutomaticCalibration(config, nil, rt, nil)
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
	engine := NewAutomaticCalibration(config, nil, rt, nil)
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
func (r *nonBlockingRuntime) MoveToPosition(axis MotionAxisConfig, _ float64) error {
	r.moves = append(r.moves, axis.Name)
	return nil
}
func (r *nonBlockingRuntime) WaitForMotionComplete() error { return nil }
func (r *nonBlockingRuntime) StopMotion() error {
	r.stopCalls++
	return nil
}