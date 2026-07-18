package usecase

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sync"
	"testing"
	"time"

	"wind-daq/services/api-go/internal/core/device"
	"wind-daq/services/api-go/internal/core/motion"
	"wind-daq/services/api-go/internal/core/traversal"
	"wind-daq/services/api-go/pkg/wiring"
)

type delayedLatestDataReader struct {
	calls int
	seq   int64
}

func (r *delayedLatestDataReader) GetLatestData(deviceID string) (device.DataPayload, bool) {
	r.calls++
	if r.calls <= 2 {
		return device.DataPayload{}, false
	}
	r.seq++
	return device.DataPayload{
		DeviceID:       deviceID,
		Timestamp:      r.seq,
		Channels:       []float64{12.5},
		ChannelIndices: []int{0},
	}, true
}

func (r *delayedLatestDataReader) GetLatestTimestamp(_ string) (int64, bool) { return 0, false }

type stoppingLatestDataReader struct {
	calls int
}

func (r *stoppingLatestDataReader) GetLatestData(deviceID string) (device.DataPayload, bool) {
	r.calls++
	return device.DataPayload{
		DeviceID:       deviceID,
		Timestamp:      int64(r.calls),
		Channels:       []float64{float64(r.calls)},
		ChannelIndices: []int{0},
	}, true
}

func (*stoppingLatestDataReader) GetLatestTimestamp(string) (int64, bool) { return 0, false }

type stopAfterFirstSampleController struct {
	reader *stoppingLatestDataReader
}

func (*stopAfterFirstSampleController) IsConnected(string) bool { return true }
func (c *stopAfterFirstSampleController) IsAcquiring(string) bool {
	return c.reader.calls < 1
}
func (*stopAfterFirstSampleController) StartAcquisition(string) error { return nil }

func TestCollectAveragedSamplesWaitsForDelayedFirstData(t *testing.T) {
	reader := &delayedLatestDataReader{}
	manager := NewTraversalManager(reader, nil, nil, nil, nil)
	manager.status = traversal.Status{TaskID: "trav-1"}

	values, err := manager.collectAveragedSamples("trav-1", []deviceChannelGroup{{deviceID: "dev-1", keys: []int{0}, hwIndices: []int{0}}}, 2)
	if err != nil {
		t.Fatalf("collectAveragedSamples returned error: %v", err)
	}
	if got := values[0]; got != 12.5 {
		t.Fatalf("averaged channel 0 = %v, want 12.5", got)
	}
	if reader.calls != 4 {
		t.Fatalf("GetLatestData calls = %d, want 4", reader.calls)
	}
}

func TestCollectAveragedSamplesFailsWhenAcquisitionStopsBeforeTargetCount(t *testing.T) {
	reader := &stoppingLatestDataReader{}
	manager := NewTraversalManager(reader, nil, nil, nil, nil)
	manager.status = traversal.Status{TaskID: "trav-stop-acquisition", State: traversal.StateRunning}
	manager.SetAcquisitionController(&stopAfterFirstSampleController{reader: reader})

	values, err := manager.collectAveragedSamples("trav-stop-acquisition", []deviceChannelGroup{{deviceID: "dev-1", keys: []int{0}, hwIndices: []int{0}}}, 2)
	if err == nil {
		t.Fatalf("expected sampling error, got values %v", values)
	}
	if reader.calls != 1 {
		t.Fatalf("GetLatestData calls = %d, want 1 before acquisition stop", reader.calls)
	}
}

func TestRunCurrentPointDoesNotMoveWhenAcquisitionHasStopped(t *testing.T) {
	motionAccess := &originReturnMotion{moveTargets: make(map[motion.AxisName]float64)}
	manager := NewTraversalManager(&delayedLatestDataReader{}, motionAccess, nil, nil, nil)
	manager.config = traversal.Config{
		TaskID:          "trav-no-move",
		DeviceID:        "dev-1",
		Channels:        []int{0},
		Path:            []traversal.Point{{X: 10}},
		SamplesPerPoint: 1,
		MotionAxes:      []traversal.MotionAxisBinding{{ControllerID: "mc-1", Axis: "X"}},
	}
	manager.status = traversal.Status{TaskID: manager.config.TaskID, State: traversal.StateRunning, TotalPoints: 1}
	manager.SetAcquisitionController(&mockAcquisitionController{
		connected:  map[string]bool{"dev-1": true},
		acquiring:  map[string]bool{"dev-1": false},
		startCalls: nil,
	})

	if err := manager.RunCurrentPoint(); err == nil {
		t.Fatal("expected RunCurrentPoint to fail when acquisition is stopped")
	}
	if len(motionAccess.moveTargets) != 0 {
		t.Fatalf("MoveTo called after acquisition stopped: %v", motionAccess.moveTargets)
	}
}

type originReturnMotion struct {
	mu          sync.Mutex
	moveTargets map[motion.AxisName]float64
	statuses    []motion.ControllerStatus
}

func (m *originReturnMotion) StatusAll(context.Context) []motion.ControllerStatus {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.statuses != nil {
		statuses := append([]motion.ControllerStatus(nil), m.statuses...)
		for i := range statuses {
			statuses[i].Axes = append([]motion.AxisStatus(nil), statuses[i].Axes...)
		}
		return statuses
	}
	return []motion.ControllerStatus{{
		ID:        "mc-1",
		Connected: true,
		Axes: []motion.AxisStatus{
			{Name: motion.AxisX, Position: 0, Moving: false},
			{Name: motion.AxisY, Position: 0, Moving: false},
		},
	}}
}

func (m *originReturnMotion) MoveTo(_ context.Context, _ string, axis motion.AxisName, position float64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.moveTargets[axis] = position
	return nil
}

func (*originReturnMotion) Stop(context.Context, string, motion.AxisName) error { return nil }
func (*originReturnMotion) EmergencyStop(context.Context, string) error         { return nil }

func TestReturnToOriginMovesConfiguredAxesToZero(t *testing.T) {
	motionAccess := &originReturnMotion{moveTargets: make(map[motion.AxisName]float64)}
	manager := NewTraversalManager(nil, motionAccess, nil, nil, nil)
	manager.config = traversal.Config{
		TaskID: "trav-return-origin",
		Path:   []traversal.Point{{X: 1, Y: 2, Z: math.NaN(), U: math.NaN()}},
		MotionAxes: []traversal.MotionAxisBinding{
			{ControllerID: "mc-1", Axis: "X"},
			{ControllerID: "mc-1", Axis: "Y"},
		},
	}
	manager.status = traversal.Status{TaskID: manager.config.TaskID, State: traversal.StateRunning}

	if err := manager.returnToOrigin(manager.config.TaskID, 1); err != nil {
		t.Fatalf("returnToOrigin returned error: %v", err)
	}
	for _, axis := range []motion.AxisName{motion.AxisX, motion.AxisY} {
		if got, ok := motionAccess.moveTargets[axis]; !ok || got != 0 {
			t.Fatalf("MoveTo target for %s = %v (called=%v), want 0", axis, got, ok)
		}
	}
}

func TestReturnToOriginSkipsAxesUnusedByPath(t *testing.T) {
	motionAccess := &originReturnMotion{moveTargets: make(map[motion.AxisName]float64)}
	manager := NewTraversalManager(nil, motionAccess, nil, nil, nil)
	manager.config = traversal.Config{
		TaskID: "trav-return-origin-line",
		Path:   []traversal.Point{{X: 10, Y: math.NaN(), Z: math.NaN(), U: math.NaN()}},
	}
	manager.status = traversal.Status{TaskID: manager.config.TaskID, State: traversal.StateRunning}

	if err := manager.returnToOrigin(manager.config.TaskID, 1); err != nil {
		t.Fatalf("returnToOrigin returned error: %v", err)
	}
	if len(motionAccess.moveTargets) != 1 || motionAccess.moveTargets[motion.AxisX] != 0 {
		t.Fatalf("return should only move path axis X, got %#v", motionAccess.moveTargets)
	}
}

func TestReturnToOriginWaitsForResume(t *testing.T) {
	motionAccess := &originReturnMotion{
		moveTargets: make(map[motion.AxisName]float64),
		statuses: []motion.ControllerStatus{{
			ID: "mc-1", Connected: true,
			Axes: []motion.AxisStatus{{Name: motion.AxisX, Position: 1, Moving: true}},
		}},
	}
	manager := NewTraversalManager(nil, motionAccess, nil, nil, nil)
	manager.config = traversal.Config{
		TaskID:     "trav-return-origin-pause",
		Path:       []traversal.Point{{X: 1, Y: math.NaN(), Z: math.NaN(), U: math.NaN()}},
		MotionAxes: []traversal.MotionAxisBinding{{ControllerID: "mc-1", Axis: "X"}},
	}
	manager.status = traversal.Status{TaskID: manager.config.TaskID, State: traversal.StateRunning}

	done := make(chan error, 1)
	go func() { done <- manager.returnToOrigin(manager.config.TaskID, 1) }()
	time.Sleep(2 * motionCompletePoll)
	if err := manager.Pause(); err != nil {
		t.Fatalf("Pause failed: %v", err)
	}
	time.Sleep(2 * motionCompletePoll)
	select {
	case err := <-done:
		t.Fatalf("returnToOrigin exited while paused: %v", err)
	default:
	}

	motionAccess.mu.Lock()
	motionAccess.statuses[0].Axes[0] = motion.AxisStatus{Name: motion.AxisX, Position: 0}
	motionAccess.mu.Unlock()
	time.Sleep(2 * motionCompletePoll)
	select {
	case err := <-done:
		t.Fatalf("returnToOrigin completed while paused at origin: %v", err)
	default:
	}
	if err := manager.Resume(); err != nil {
		t.Fatalf("Resume failed: %v", err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("returnToOrigin after resume: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("returnToOrigin did not continue after resume")
	}
}

// TestRunCurrentPointSkipLastPersistsCompletedState 回归测试（B1）：
// 最后一个点被 skip（validation OnInvalid=skip 且校验失败）时，
// 持久化到 result store 的最终状态必须是 StateCompleted，
// 不得是置 Completed 之前捕获的过期快照（StateSaving/StateAcquiring 等）。
//
// 修复前 skip 分支在锁内先捕获 saveStatus，returnToOrigin 之后才置 Completed，
// 导致 store.Save 写入的是旧状态快照；修复后镜像正常分支，先置 Completed 再捕获。
func TestRunCurrentPointSkipLastPersistsCompletedState(t *testing.T) {
	reader := &mockLatestDataReader{data: device.DataPayload{Channels: []float64{1, 2, 3, 4, 5}}}
	motionAccess := &mockMotionAccess{}
	sink := &mockTraversalPointSink{}
	store := newMockTraversalResultStore()
	manager := NewTraversalManager(reader, motionAccess, sink, store, wiring.NewFileCheckpointStore())

	config := traversal.Config{
		TaskID:          "trav-skip-last-point",
		DeviceID:        "sim-1",
		Channels:        []int{0, 1, 2, 3, 4},
		ChannelLabels:   map[int]string{0: "P1", 1: "P2", 2: "P3", 3: "P4", 4: "P5"},
		Path:            []traversal.Point{{X: 0, Y: 0, Z: 0}},
		DwellTimeMs:     1,
		SamplesPerPoint: 1,
		SavePath:        t.TempDir(),
		SaveFileName:    "skip-last",
	}
	// 校验恒失败（P1 读数 1 不在 [1000,2000]）+ OnInvalid=skip → 唯一（最后）点走 skip 分支
	manager.SetValidation(&traversal.DataValidationConfig{
		Enabled:   true,
		OnInvalid: "skip",
		PressureRange: map[string]*traversal.PressureRange{
			"P1": {Min: 1000, Max: 2000},
		},
	})

	if err := manager.Start(config); err != nil {
		t.Fatalf("Start returned error: %v", err)
	}
	t.Cleanup(func() { _ = manager.Stop() })

	if err := manager.RunCurrentPoint(); err != nil {
		t.Fatalf("RunCurrentPoint returned error: %v", err)
	}

	saved, ok := store.Get(config.TaskID)
	if !ok {
		t.Fatal("expected final status persisted to result store when last point is skipped")
	}
	if saved.State != traversal.StateCompleted {
		t.Fatalf("persisted state = %q, want %q (skip 末点不得持久化置 Completed 之前的过期快照)",
			saved.State, traversal.StateCompleted)
	}
}

// returnOriginFailMotion 仅在目标为 0（回零）时让 MoveTo 失败，
// 非零目标的点位运动正常，用于构造"采集成功但回零失败"的场景。
type returnOriginFailMotion struct {
	mockMotionAccess
}

func (m *returnOriginFailMotion) MoveTo(ctx context.Context, id string, axis motion.AxisName, position float64) error {
	if position == 0 {
		return fmt.Errorf("simulated return-to-origin failure")
	}
	return m.mockMotionAccess.MoveTo(ctx, id, axis, position)
}

// TestRunCurrentPointReturnToOriginFailureStillCompletes 回归测试：
// 数据全部采完后回零失败（MoveTo(0) 注入错误）只应记录 Status.Warning，
// 最终状态仍为 Completed，LastError/LastErrorCode 清空，
// result store 持久化的也是完成态——回零失败不得把已采完的测试判为失败。
func TestRunCurrentPointReturnToOriginFailureStillCompletes(t *testing.T) {
	reader := &mockLatestDataReader{data: device.DataPayload{Channels: []float64{1, 2, 3, 4, 5}}}
	motionAccess := &returnOriginFailMotion{mockMotionAccess{
		statuses: []motion.ControllerStatus{{
			ID: "mc-1", Connected: true,
			Axes: []motion.AxisStatus{
				{Name: motion.AxisX, Position: 10, Homed: true, Moving: false},
				{Name: motion.AxisY, Position: 0, Homed: true, Moving: false},
				{Name: motion.AxisZ, Position: 0, Homed: true, Moving: false},
			},
		}},
	}}
	sink := &mockTraversalPointSink{}
	store := newMockTraversalResultStore()
	manager := NewTraversalManager(reader, motionAccess, sink, store, wiring.NewFileCheckpointStore())

	config := traversal.Config{
		TaskID:          "trav-return-origin-fail",
		DeviceID:        "sim-1",
		Channels:        []int{0, 1, 2, 3, 4},
		ChannelLabels:   map[int]string{0: "P1", 1: "P2", 2: "P3", 3: "P4", 4: "P5"},
		Path:            []traversal.Point{{X: 10, Y: math.NaN(), Z: math.NaN(), U: math.NaN()}},
		DwellTimeMs:     1,
		SamplesPerPoint: 1,
		SavePath:        t.TempDir(),
		SaveFileName:    "return-origin-fail",
	}

	if err := manager.Start(config); err != nil {
		t.Fatalf("Start returned error: %v", err)
	}
	t.Cleanup(func() { _ = manager.Stop() })

	if err := manager.RunCurrentPoint(); err != nil {
		t.Fatalf("RunCurrentPoint returned error: %v（回零失败应降级为 warning，不应报错）", err)
	}

	saved, ok := store.Get(config.TaskID)
	if !ok {
		t.Fatal("expected final status persisted to result store")
	}
	if saved.State != traversal.StateCompleted {
		t.Fatalf("persisted state = %q, want %q（数据已采完，回零失败不判测试失败）",
			saved.State, traversal.StateCompleted)
	}
	if saved.Warning == "" {
		t.Fatal("expected Warning recorded for failed return-to-origin")
	}
	if saved.LastError != "" || saved.LastErrorCode != "" {
		t.Fatalf("LastError/LastErrorCode should be cleared, got %q / %q", saved.LastError, saved.LastErrorCode)
	}
}

// TestCompleteAfterReturnToOriginSemantics 单元测试：
// 用户停止/取消（errReturnToOriginAborted）原样透传；运动失败降级为 Warning。
func TestCompleteAfterReturnToOriginSemantics(t *testing.T) {
	manager := NewTraversalManager(nil, nil, nil, nil, nil)
	manager.status = traversal.Status{TaskID: "t", State: traversal.StateRunning}

	if err := manager.completeAfterReturnToOrigin("t", nil); err != nil {
		t.Fatalf("nil error should pass through, got %v", err)
	}
	if err := manager.completeAfterReturnToOrigin("t", errReturnToOriginAborted); !errors.Is(err, errReturnToOriginAborted) {
		t.Fatalf("abort should pass through, got %v", err)
	}
	if manager.status.Warning != "" {
		t.Fatalf("abort path must not record Warning, got %q", manager.status.Warning)
	}

	manager.setErrorLocked("return to origin timed out", traversal.ErrMotionTimeout)
	if err := manager.completeAfterReturnToOrigin("t", fmt.Errorf("return to origin timed out")); err != nil {
		t.Fatalf("motion failure should be downgraded to warning, got %v", err)
	}
	if manager.status.Warning != "return to origin timed out" {
		t.Fatalf("Warning = %q, want %q", manager.status.Warning, "return to origin timed out")
	}
	if manager.status.LastError != "" || manager.status.LastErrorCode != "" {
		t.Fatalf("error state should be cleared, got %q / %q", manager.status.LastError, manager.status.LastErrorCode)
	}
}

// slowFrameLatestDataReader 按真实时间节流产帧：每 frameInterval 产出一帧新数据，
// 模拟低频设备（如 2Hz 帧率）。帧时间戳 = 已产出帧序号，保证帧去重语义生效。
type slowFrameLatestDataReader struct {
	frameInterval time.Duration
	start         time.Time
	value         float64
}

func (r *slowFrameLatestDataReader) GetLatestData(deviceID string) (device.DataPayload, bool) {
	frame := int64(time.Since(r.start) / r.frameInterval)
	return device.DataPayload{
		DeviceID:       deviceID,
		Timestamp:      frame,
		Channels:       []float64{r.value},
		ChannelIndices: []int{0},
	}, true
}

func (*slowFrameLatestDataReader) GetLatestTimestamp(string) (int64, bool) { return 0, false }

// TestCollectAveragedSamplesLowFrameRateNotKilledByOverallTimeout 回归测试：
// 低频设备（800ms 一帧）采 4 个样本需要约 2.4s——旧的 2s 固定总体超时会必然失败；
// 改为停滞超时后，只要持续有新帧/新样本就不判超时，应正常完成。
func TestCollectAveragedSamplesLowFrameRateNotKilledByOverallTimeout(t *testing.T) {
	reader := &slowFrameLatestDataReader{frameInterval: 800 * time.Millisecond, start: time.Now(), value: 42}
	manager := NewTraversalManager(reader, nil, nil, nil, nil)
	manager.status = traversal.Status{TaskID: "trav-low-fps"}

	start := time.Now()
	values, err := manager.collectAveragedSamples("trav-low-fps",
		[]deviceChannelGroup{{deviceID: "dev-1", keys: []int{0}, hwIndices: []int{0}}}, 4)
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("collectAveragedSamples returned error: %v（低频设备多样本采集不应被总体超时杀死）", err)
	}
	if got := values[0]; got != 42 {
		t.Fatalf("averaged channel 0 = %v, want 42", got)
	}
	if elapsed < 2*time.Second {
		t.Fatalf("elapsed = %v, want >= 2s（4 帧 × 800ms 应超过旧的 2s 总体超时，证明语义已改）", elapsed)
	}
}

// oneFrameThenSilentReader 只产出一帧后永远不再出新帧，用于验证停滞超时仍会触发。
type oneFrameThenSilentReader struct{}

func (*oneFrameThenSilentReader) GetLatestData(deviceID string) (device.DataPayload, bool) {
	return device.DataPayload{
		DeviceID:       deviceID,
		Timestamp:      1,
		Channels:       []float64{1},
		ChannelIndices: []int{0},
	}, true
}

func (*oneFrameThenSilentReader) GetLatestTimestamp(string) (int64, bool) { return 0, false }

// TestCollectAveragedSamplesStallTimeoutWhenNoNewSample 回归测试：
// 凑不齐新样本（设备静默/帧不再更新）时，停滞超时仍应在 acquisitionStallTimeout 后报错，
// 而不是因为"完成样本重置"语义退化为永不超时。
func TestCollectAveragedSamplesStallTimeoutWhenNoNewSample(t *testing.T) {
	reader := &oneFrameThenSilentReader{}
	manager := NewTraversalManager(reader, nil, nil, nil, nil)
	manager.status = traversal.Status{TaskID: "trav-stall"}

	start := time.Now()
	_, err := manager.collectAveragedSamples("trav-stall",
		[]deviceChannelGroup{{deviceID: "dev-1", keys: []int{0}, hwIndices: []int{0}}}, 2)
	elapsed := time.Since(start)
	if err == nil {
		t.Fatal("expected stall timeout error when no new complete sample arrives")
	}
	if elapsed < acquisitionStallTimeout || elapsed > acquisitionStallTimeout+2*time.Second {
		t.Fatalf("elapsed = %v, want ≈ acquisitionStallTimeout(%v)", elapsed, acquisitionStallTimeout)
	}
}
