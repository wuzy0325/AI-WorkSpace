package usecase

import (
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"wind-daq/services/api-go/internal/core/calibration"
	"wind-daq/services/api-go/internal/core/traversal"
)

// ==================== 测试假件 ====================

// calibrationLifecycleRuntime 实现 ports.CalibrationRuntime，用于生命周期测试。
// waitGate 非 nil 时 WaitForMotionComplete 阻塞直到 channel 关闭——
// 模拟 legacy 阻塞式运动等待（不响应 ctx 取消），用于构造 Stop 超时场景。
type calibrationLifecycleRuntime struct {
	waitGate    chan struct{}
	waitEntered chan struct{}
	enteredOnce sync.Once
	stopCalls   atomic.Int32
}

func newCalibrationLifecycleRuntime() *calibrationLifecycleRuntime {
	return &calibrationLifecycleRuntime{waitEntered: make(chan struct{})}
}

// 与 calibrationStatusLatestReader 相同的已知良好通道布局，
// 保证五孔算法在 SamplesPerPoint=1 时能产出数据点。
var calibrationLifecycleChannelValues = []float64{1, 2, 3, 4, 5, 101325, 25, 80, 15}

func (r *calibrationLifecycleRuntime) GetChannelValue(_ string, channelIndex int) (float64, bool) {
	if channelIndex >= 0 && channelIndex < len(calibrationLifecycleChannelValues) {
		return calibrationLifecycleChannelValues[channelIndex], true
	}
	return 0, false
}

func (r *calibrationLifecycleRuntime) GetLatestTimestamp(_ string) (int64, bool) { return 0, false }

func (r *calibrationLifecycleRuntime) MoveToPosition(_ calibration.MotionAxisConfig, _ float64) error {
	return nil
}

func (r *calibrationLifecycleRuntime) WaitForMotionComplete() error {
	r.enteredOnce.Do(func() { close(r.waitEntered) })
	if r.waitGate != nil {
		<-r.waitGate
	}
	return nil
}

func (r *calibrationLifecycleRuntime) StopMotion() error {
	r.stopCalls.Add(1)
	return nil
}

// recordedCalibrationSave 记录一次 store.Save 调用的快照。
type recordedCalibrationSave struct {
	taskID string
	status calibration.Status
}

// recordingCalibrationStore 实现 ports.CalibrationResultStore，记录所有 Save 调用。
type recordingCalibrationStore struct {
	mu    sync.Mutex
	saves []recordedCalibrationSave
}

func (s *recordingCalibrationStore) Save(taskID string, status calibration.Status) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.saves = append(s.saves, recordedCalibrationSave{taskID: taskID, status: status})
	return nil
}

func (s *recordingCalibrationStore) Get(_ string) (calibration.Status, bool) {
	return calibration.Status{}, false
}

func (s *recordingCalibrationStore) snapshot() []recordedCalibrationSave {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]recordedCalibrationSave(nil), s.saves...)
}

// countingCalibrationWriter 实现 ports.CalibrationCsvWriter，统计 Initialize/Flush 次数，
// 用于验证 writer flush 恰好由所属 session 执行一次。
type countingCalibrationWriter struct {
	mu          sync.Mutex
	initCount   int
	flushCount  int
	appendCount int
}

func (w *countingCalibrationWriter) Initialize(_ calibration.Config) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.initCount++
	return nil
}

func (w *countingCalibrationWriter) AppendPoint(_ calibration.DataPoint) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.appendCount++
	return nil
}

func (w *countingCalibrationWriter) Flush() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.flushCount++
	return nil
}

func (w *countingCalibrationWriter) Path() string { return "" }

func (w *countingCalibrationWriter) counts() (init, flush int) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.initCount, w.flushCount
}

// ==================== 测试辅助 ====================

func overrideCalibrationStopJoinTimeout(t *testing.T, d time.Duration) {
	t.Helper()
	old := calibrationStopJoinTimeout
	calibrationStopJoinTimeout = d
	t.Cleanup(func() { calibrationStopJoinTimeout = old })
}

func waitChannelClosed(t *testing.T, ch <-chan struct{}, desc string) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for %s", desc)
	}
}

// lifecycleMotionAxesConfig 在已知良好的五孔配置基础上绑定 α/β 两个运动轴，
// 使引擎在 processPoint 中走入 runtime.MoveToPosition + WaitForMotionComplete 路径。
func lifecycleMotionAxesConfig(taskID string) calibration.Config {
	cfg := fiveHoleStatusTestConfig(taskID)
	cfg.MotionAxes = []calibration.MotionAxisConfig{
		{ControllerID: "mc-1", Axis: "α", Name: "α"},
		{ControllerID: "mc-1", Axis: "β", Name: "β"},
	}
	return cfg
}

// ==================== 生命周期测试 ====================

// TestCalibrationManagerLifecycleImmediateStartStopBounded 验证 Start 后立即 Stop
// 能在有界时间内返回（worker 驻留等待可被 ctx 取消），且状态收敛为 Stopped。
func TestCalibrationManagerLifecycleImmediateStartStopBounded(t *testing.T) {
	runtime := newCalibrationLifecycleRuntime()
	manager := NewCalibrationManager(nil, nil, nil, nil)
	manager.SetRuntime(runtime)

	cfg := fiveHoleStatusTestConfig("cal-lifecycle-immediate")
	cfg.DwellTimeMs = 60000 // 长驻留：Stop 必须能打断，而非等驻留自然结束

	if err := manager.Start(cfg); err != nil {
		t.Fatalf("start calibration: %v", err)
	}

	start := time.Now()
	if err := manager.Stop(); err != nil {
		t.Fatalf("stop calibration: %v", err)
	}
	if elapsed := time.Since(start); elapsed > 3*time.Second {
		t.Fatalf("stop took %v, expected bounded return well under 5s join limit", elapsed)
	}

	if status := manager.Status(); status.State != calibration.StateStopped {
		t.Fatalf("expected stopped state, got %s", status.State)
	}
}

// TestCalibrationManagerLifecycleRejectsSecondStartWhileRunning 验证运行中拒绝
// replacement Start；Stop 正常 join 后新任务可被接受。
func TestCalibrationManagerLifecycleRejectsSecondStartWhileRunning(t *testing.T) {
	runtime := newCalibrationLifecycleRuntime()
	manager := NewCalibrationManager(nil, nil, nil, nil)
	manager.SetRuntime(runtime)

	cfgA := fiveHoleStatusTestConfig("cal-lifecycle-running-a")
	cfgA.DwellTimeMs = 60000

	if err := manager.Start(cfgA); err != nil {
		t.Fatalf("start calibration A: %v", err)
	}

	cfgB := fiveHoleStatusTestConfig("cal-lifecycle-running-b")
	if err := manager.Start(cfgB); err == nil {
		t.Fatal("expected replacement start to be rejected while A is running")
	}

	if err := manager.Stop(); err != nil {
		t.Fatalf("stop calibration A: %v", err)
	}

	if err := manager.Start(cfgB); err != nil {
		t.Fatalf("expected start B to succeed after A fully stopped, got %v", err)
	}
	if err := manager.Stop(); err != nil {
		t.Fatalf("stop calibration B: %v", err)
	}
}

// TestCalibrationManagerStopTimeoutRejectsNewStartUntilSessionDone 验证：
//  1. worker 卡在不可取消的 legacy 运动等待时，Stop 在有界时间后返回明确超时错误；
//  2. 超时后旧 session 未 done 之前，新 Start 持续被拒绝；
//  3. 旧 worker 退出后新 Start 恢复可用。
func TestCalibrationManagerStopTimeoutRejectsNewStartUntilSessionDone(t *testing.T) {
	overrideCalibrationStopJoinTimeout(t, 300*time.Millisecond)

	runtime := newCalibrationLifecycleRuntime()
	runtime.waitGate = make(chan struct{}) // 阻塞直到测试释放
	manager := NewCalibrationManager(nil, nil, nil, nil)
	manager.SetRuntime(runtime)

	cfgA := lifecycleMotionAxesConfig("cal-lifecycle-timeout-a")
	if err := manager.Start(cfgA); err != nil {
		t.Fatalf("start calibration A: %v", err)
	}
	waitChannelClosed(t, runtime.waitEntered, "worker A to enter blocking motion wait")

	start := time.Now()
	err := manager.Stop()
	if err == nil {
		t.Fatal("expected stop to return a timeout error while worker is stuck")
	}
	if !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("expected stop error to mention timed out, got %v", err)
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("stop took %v, expected bounded return near the 300ms join timeout", elapsed)
	}

	// 旧 session 未 done：新 Start 必须被拒绝
	cfgB := fiveHoleStatusTestConfig("cal-lifecycle-timeout-b")
	if startErr := manager.Start(cfgB); startErr == nil {
		t.Fatal("expected start B to be rejected while old session is still finalizing")
	}

	// 释放旧 worker，等待其完全退出后新 Start 恢复可用
	close(runtime.waitGate)
	deadline := time.Now().Add(2 * time.Second)
	for {
		startErr := manager.Start(cfgB)
		if startErr == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("start B still rejected after old worker released: %v", startErr)
		}
		time.Sleep(20 * time.Millisecond)
	}
	if err := manager.Stop(); err != nil {
		t.Fatalf("stop calibration B: %v", err)
	}
}

// TestCalibrationManagerSessionStaleWorkerFinalizeIsolated 验证旧 session 的
// finalize（writer flush / 结果保存 / 状态写入）只执行一次且不会污染新 session：
//   - Stop 超时时刻旧 worker 尚未 finalize（store 无记录、writer 未 flush）；
//   - 旧 worker 退出后恰好保存一次 taskA(Stopped)、flush 一次；
//   - 新 session 正常完成后保存 taskB(Completed)，终态属于新 session。
func TestCalibrationManagerSessionStaleWorkerFinalizeIsolated(t *testing.T) {
	overrideCalibrationStopJoinTimeout(t, 300*time.Millisecond)

	runtime := newCalibrationLifecycleRuntime()
	runtime.waitGate = make(chan struct{})
	store := &recordingCalibrationStore{}
	writer := &countingCalibrationWriter{}
	manager := NewCalibrationManager(nil, nil, nil, store)
	manager.SetRuntime(runtime)
	manager.SetCsvWriter(writer)

	cfgA := lifecycleMotionAxesConfig("cal-lifecycle-stale-a")
	cfgA.SavePath = filepath.Join(t.TempDir(), "stale-a.csv")
	if err := manager.Start(cfgA); err != nil {
		t.Fatalf("start calibration A: %v", err)
	}
	waitChannelClosed(t, runtime.waitEntered, "worker A to enter blocking motion wait")

	if err := manager.Stop(); err == nil {
		t.Fatal("expected stop timeout error while worker A is stuck")
	}

	// Stop 超时返回时旧 worker 仍卡在运动等待中：不得有任何 finalize 副作用
	if saves := store.snapshot(); len(saves) != 0 {
		t.Fatalf("expected no store save before old worker exits, got %d", len(saves))
	}
	if _, flush := writer.counts(); flush != 0 {
		t.Fatalf("expected no writer flush before old worker exits, got %d", flush)
	}

	// 释放旧 worker → 旧 session finalize 恰好执行一次 → 新 Start 被接受
	close(runtime.waitGate)
	cfgB := fiveHoleStatusTestConfig("cal-lifecycle-stale-b")
	cfgB.Points = cfgB.Points[:1]
	deadline := time.Now().Add(2 * time.Second)
	for {
		if err := manager.Start(cfgB); err == nil {
			break
		} else if time.Now().After(deadline) {
			t.Fatalf("start B still rejected after old worker released: %v", err)
		}
		time.Sleep(20 * time.Millisecond)
	}

	// 新 session 启动即意味着旧 session 已完全 finalize：
	// taskA 恰好保存一次且状态为 Stopped，writer flush 恰好一次。
	saves := store.snapshot()
	if len(saves) != 1 || saves[0].taskID != "cal-lifecycle-stale-a" {
		t.Fatalf("expected exactly one save for stale task A, got %+v", saves)
	}
	if saves[0].status.State != calibration.StateStopped {
		t.Fatalf("expected stale task A saved with stopped state, got %s", saves[0].status.State)
	}
	if init, flush := writer.counts(); init != 1 || flush != 1 {
		t.Fatalf("expected writer init=1 flush=1 after stale finalize, got init=%d flush=%d", init, flush)
	}

	// 新 session 单点快速完成，终态必须属于新 session
	waitForStatus(t, manager, func(status calibration.Status) bool {
		return status.State == calibration.StateCompleted
	}, "calibration B to complete")

	saves = store.snapshot()
	if len(saves) != 2 || saves[1].taskID != "cal-lifecycle-stale-b" {
		t.Fatalf("expected second save for task B, got %+v", saves)
	}
	if saves[1].status.State != calibration.StateCompleted {
		t.Fatalf("expected task B saved with completed state, got %s", saves[1].status.State)
	}
	status := manager.Status()
	if status.TaskID != "cal-lifecycle-stale-b" || status.State != calibration.StateCompleted {
		t.Fatalf("expected final status to belong to B (completed), got taskID=%s state=%s", status.TaskID, status.State)
	}
}

// TestCalibrationManagerLifecyclePreStartValidationFailureLeavesIdle 验证所有
// 预启动校验失败都不会留下 Running 状态（校验全部通过后才发布 running）。
func TestCalibrationManagerLifecyclePreStartValidationFailureLeavesIdle(t *testing.T) {
	manager := NewCalibrationManager(nil, nil, nil, nil)

	// 用例 1：未知校准类型——旧实现先发布 Running 再创建算法，
	// 失败后状态卡死在 Running，后续合法 Start 被永久拒绝。
	badType := fiveHoleStatusTestConfig("cal-lifecycle-bad-type")
	badType.Type = "not-a-calibration-type"
	if err := manager.Start(badType); err == nil {
		t.Fatal("expected start with unknown type to fail")
	}
	if status := manager.Status(); status.State == calibration.StateRunning {
		t.Fatal("expected no running state after failed start (unknown type)")
	}

	// 用例 2：非法运动安全配置（ArrivalTolerance 必须 > 0）
	badSafety := fiveHoleStatusTestConfig("cal-lifecycle-bad-safety")
	negativeTolerance := -1.0
	badSafety.MotionSafety = &traversal.MotionSafetyConfig{ArrivalTolerance: &negativeTolerance}
	if err := manager.Start(badSafety); err == nil {
		t.Fatal("expected start with invalid motion safety config to fail")
	}
	if status := manager.Status(); status.State == calibration.StateRunning {
		t.Fatal("expected no running state after failed start (invalid motion safety)")
	}

	// 校验失败后合法任务仍可正常启动并运行
	valid := fiveHoleStatusTestConfig("cal-lifecycle-valid")
	valid.DwellTimeMs = 60000
	if err := manager.Start(valid); err != nil {
		t.Fatalf("expected valid start to succeed after failed validations, got %v", err)
	}
	if status := manager.Status(); status.State != calibration.StateRunning {
		t.Fatalf("expected running state for valid task, got %s", status.State)
	}
	if err := manager.Stop(); err != nil {
		t.Fatalf("stop valid calibration: %v", err)
	}
}

// TestCalibrationManagerStatusConcurrentAccessRaceFree 在 Start/Stop 期间并发
// 轮询 Status()，配合 -race 检测 m.autoEngine 的无同步读竞态。
func TestCalibrationManagerStatusConcurrentAccessRaceFree(t *testing.T) {
	runtime := newCalibrationLifecycleRuntime()
	manager := NewCalibrationManager(nil, nil, nil, nil)
	manager.SetRuntime(runtime)

	stopHammer := make(chan struct{})
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stopHammer:
					return
				default:
					_ = manager.Status()
				}
			}
		}()
	}

	cfg := fiveHoleStatusTestConfig("cal-lifecycle-race")
	cfg.DwellTimeMs = 30000
	if err := manager.Start(cfg); err != nil {
		close(stopHammer)
		wg.Wait()
		t.Fatalf("start calibration: %v", err)
	}
	time.Sleep(50 * time.Millisecond)
	if err := manager.Stop(); err != nil {
		t.Fatalf("stop calibration: %v", err)
	}
	close(stopHammer)
	wg.Wait()
}
