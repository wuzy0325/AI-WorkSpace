// Package usecase — 校准模块运动安全移植单元测试
//
// 覆盖范围：
//   - validateCalibrationMotionSafetyConfig：calibration.MotionAxisConfig → traversal.MotionAxisBinding 转换 + 委托校验
//   - fallbackRuntime.WaitForMotionComplete：运动安全判定 + 三元组返回值
//   - fallbackRuntime.EmergencyStopMotion：急停成功 / 失败 fallback / 无目标兜底
//   - runtimeAdapter：MotionSafetyAwareRuntime 透传 / EmergencyStopProvider 透传 / 旧接口 fallback
//   - handleCalibrationMotionSafetyFailure：急停类 / 普通停止类分发 + 错误码写入 + 故障快照写入
//
// 测试用例遵循三段式：测试前置 / 测试步骤 / 期待结果。
package usecase

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"shared.local/device-sdk/go/pkg/slog"

	"wind-daq/services/api-go/internal/core/calibration"
	"wind-daq/services/api-go/internal/core/motion"
	"wind-daq/services/api-go/internal/core/traversal"
	"wind-daq/services/api-go/internal/ports"
)

// ==================== 测试辅助：可控 MotionManager ====================

// controllableMotionManager 可编程运动控制器 mock。
//
// 字段语义：
//   - status：固定返回的运动控制器状态快照（含 Position/Moving/PosLimit/NegLimit/Connected）
//   - emergencyStopCalls / stopCalls：调用计数（用于断言急停 vs 普通停止的分发）
//   - emergencyStopErr / stopErr：注入错误（用于测试 fallback 路径）
type controllableMotionManager struct {
	mu               sync.Mutex
	status           []motion.ControllerStatus
	statusSequence   [][]motion.ControllerStatus
	emergencyStopErr error
	stopErr          error
	emergencyStopIDs []string
	stopIDs          []string
}

func (m *controllableMotionManager) setStatus(status []motion.ControllerStatus) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.status = status
}

func (m *controllableMotionManager) LoadProfiles() ([]motion.MotionControllerProfile, error) {
	return m.GetProfiles(), nil
}
func (m *controllableMotionManager) SaveProfiles([]motion.MotionControllerProfile) error { return nil }
func (m *controllableMotionManager) GetProfiles() []motion.MotionControllerProfile {
	return nil
}
func (m *controllableMotionManager) UpsertProfile(motion.MotionControllerProfile) error { return nil }
func (m *controllableMotionManager) DeleteProfile(string) error                         { return nil }
func (m *controllableMotionManager) Connect(context.Context, string) error              { return nil }
func (m *controllableMotionManager) Disconnect(context.Context, string) error           { return nil }
func (m *controllableMotionManager) StatusAll(context.Context) []motion.ControllerStatus {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.statusSequence) > 0 {
		status := m.statusSequence[0]
		m.statusSequence = m.statusSequence[1:]
		return status
	}
	return m.status
}
func (m *controllableMotionManager) MoveTo(_ context.Context, _ string, _ motion.AxisName, _ float64) error {
	return nil
}
func (m *controllableMotionManager) MoveBy(context.Context, string, motion.AxisName, float64) error {
	return nil
}
func (m *controllableMotionManager) Jog(context.Context, string, motion.AxisName, float64) error {
	return nil
}
func (m *controllableMotionManager) Home(context.Context, string, motion.AxisName) error { return nil }
func (m *controllableMotionManager) Stop(_ context.Context, id string, _ motion.AxisName) error {
	m.mu.Lock()
	m.stopIDs = append(m.stopIDs, id)
	m.mu.Unlock()
	return m.stopErr
}
func (m *controllableMotionManager) EmergencyStop(_ context.Context, id string) error {
	m.mu.Lock()
	m.emergencyStopIDs = append(m.emergencyStopIDs, id)
	m.mu.Unlock()
	return m.emergencyStopErr
}
func (m *controllableMotionManager) ResetEmergencyStop(context.Context, string) error { return nil }
func (m *controllableMotionManager) DefinePosition(context.Context, string, motion.AxisName, float64) error {
	return nil
}

func (m *controllableMotionManager) emergencyStopCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.emergencyStopIDs)
}
func (m *controllableMotionManager) stopCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.stopIDs)
}

// makeStoppedAxisStatus 构造"控制器已连接 + 单轴已停于 position"的状态快照。
func makeStoppedAxisStatus(controllerID, axisName string, position float64) []motion.ControllerStatus {
	return []motion.ControllerStatus{{
		ID:        controllerID,
		Connected: true,
		Axes:      []motion.AxisStatus{{Name: motion.AxisName(axisName), Position: position, Moving: false}},
	}}
}

// makeMovingAxisStatus 构造"控制器已连接 + 单轴运动中于 position"的状态快照。
func makeMovingAxisStatus(controllerID, axisName string, position float64) []motion.ControllerStatus {
	return []motion.ControllerStatus{{
		ID:        controllerID,
		Connected: true,
		Axes:      []motion.AxisStatus{{Name: motion.AxisName(axisName), Position: position, Moving: true}},
	}}
}

// makeLimitTriggeredStatus 构造"控制器已连接 + 单轴已停于 position + 限位触发"的状态快照。
func makeLimitTriggeredStatus(controllerID, axisName string, position float64, posLimit bool) []motion.ControllerStatus {
	return []motion.ControllerStatus{{
		ID:        controllerID,
		Connected: true,
		Axes: []motion.AxisStatus{{
			Name:     motion.AxisName(axisName),
			Position: position,
			Moving:   false,
			PosLimit: posLimit,
			NegLimit: !posLimit,
		}},
	}}
}

// makeMotionSafetyConfig 构造已解析的运动安全配置（指针字段非 nil）。
// 参数：arrival=到位容差, critical=严重偏离阈值, noProgressMs=无进展超时, epsilon=进展阈值。
func makeMotionSafetyConfig(arrival, critical float64, noProgressMs int, epsilon float64) *traversal.MotionSafetyConfig {
	return &traversal.MotionSafetyConfig{
		ArrivalTolerance:       &arrival,
		CriticalDeviationLimit: &critical,
		NoProgressTimeoutMs:    &noProgressMs,
		ProgressEpsilon:        &epsilon,
	}
}

// ==================== validateCalibrationMotionSafetyConfig 测试 ====================

func TestValidateCalibrationMotionSafetyConfig_NilCfgReturnsNil(t *testing.T) {
	// 测试前置：nil 配置（旧配置兼容场景）
	axes := []calibration.MotionAxisConfig{{Name: "α", ControllerID: "motion-1", Axis: "X"}}

	// 测试步骤：校验 nil 配置
	err := validateCalibrationMotionSafetyConfig(nil, axes)

	// 期待结果：无错误，下游使用默认值
	if err != nil {
		t.Fatalf("nil cfg 应直接返回 nil，实际: %v", err)
	}
}

func TestValidateCalibrationMotionSafetyConfig_ValidCfgWithLogicalAxisName(t *testing.T) {
	// 测试前置：含 Name 字段的 calibration.MotionAxisConfig + 合法阈值
	arrival, critical := 0.1, 5.0
	cfg := makeMotionSafetyConfig(arrival, critical, 500, 0.001)
	axes := []calibration.MotionAxisConfig{
		{Name: "α", ControllerID: "motion-1", Axis: "X"},
		{Name: "β", ControllerID: "motion-1", Axis: "Y"},
	}

	// 测试步骤：校验（应转换 ControllerID/Axis 到 MotionAxisBinding 后通过）
	err := validateCalibrationMotionSafetyConfig(cfg, axes)

	// 期待结果：通过（Name 字段不参与校验，仅 ControllerID/Axis 用于 axisOverrides 匹配）
	if err != nil {
		t.Fatalf("合法配置应通过校验，实际: %v", err)
	}
}

func TestValidateCalibrationMotionSafetyConfig_CriticalLessThanArrival(t *testing.T) {
	// 测试前置：阈值倒置——criticalDeviationLimit < arrivalTolerance
	arrival, critical := 5.0, 0.1 // 阈值倒置
	cfg := makeMotionSafetyConfig(arrival, critical, 500, 0.001)
	axes := []calibration.MotionAxisConfig{{Name: "α", ControllerID: "motion-1", Axis: "X"}}

	// 测试步骤：校验
	err := validateCalibrationMotionSafetyConfig(cfg, axes)

	// 期待结果：错误（阈值倒置会让偏差 5–10 永远走不到 Deviation 分支）
	if err == nil {
		t.Fatal("阈值倒置应返回错误，实际 nil")
	}
}

// ==================== fallbackRuntime.WaitForMotionComplete 测试 ====================

func TestFallbackRuntimeWaitForMotionComplete_NormalArrival(t *testing.T) {
	// 测试前置：运动中→到位的状态序列，目标 10mm
	mgr := &controllableMotionManager{}
	mgr.setStatus(makeMovingAxisStatus("motion-1", "X", 0))
	rt := &fallbackRuntime{
		motion:       mgr,
		motionSafety: makeMotionSafetyConfig(0.2, 5.0, 2000, 0.001),
	}
	// 通过 MoveToPosition 设置目标，然后切换状态为到位
	_ = rt.MoveToPosition(calibration.MotionAxisConfig{ControllerID: "motion-1", Axis: "X", Name: "α"}, 10)
	// 异步切换：100ms 后改为到位状态
	go func() {
		time.Sleep(120 * time.Millisecond)
		mgr.setStatus(makeStoppedAxisStatus("motion-1", "X", 10))
	}()

	// 测试步骤：等待运动完成
	completed, reason, failure := rt.WaitForMotionComplete()

	// 期待结果：(true, None, nil)
	if !completed || reason != traversal.MotionInterruptNone || failure != nil {
		t.Fatalf("normal arrival: completed=%v reason=%v failure=%v", completed, reason, failure)
	}
}

func TestFallbackRuntimeWaitForMotionComplete_PosLimitTriggered(t *testing.T) {
	// 测试前置：限位触发的状态（PosLimit=true），目标 30mm
	mgr := &controllableMotionManager{}
	mgr.setStatus(makeLimitTriggeredStatus("motion-1", "X", 35, true))
	rt := &fallbackRuntime{
		motion:       mgr,
		motionSafety: makeMotionSafetyConfig(0.2, 5.0, 2000, 0.001),
	}
	_ = rt.MoveToPosition(calibration.MotionAxisConfig{ControllerID: "motion-1", Axis: "X", Name: "α"}, 30)

	// 测试步骤：等待运动完成
	completed, _, failure := rt.WaitForMotionComplete()

	// 期待结果：检测到 LimitTriggered 故障
	if completed {
		t.Fatal("limit triggered should not complete")
	}
	if failure == nil || failure.Verdict != traversal.MotionSafetyLimitTriggered {
		t.Fatalf("failure verdict = %v, want LimitTriggered", failure)
	}
}

func TestFallbackRuntimeWaitForMotionComplete_ControllerDisconnectAfter3Snapshots(t *testing.T) {
	mgr := &controllableMotionManager{
		statusSequence: [][]motion.ControllerStatus{
			makeMovingAxisStatus("motion-1", "X", 0),
			{{ID: "motion-1", Connected: false}},
			{{ID: "motion-1", Connected: false}},
			{{ID: "motion-1", Connected: false}},
		},
	}
	rt := &fallbackRuntime{motion: mgr, motionSafety: makeMotionSafetyConfig(0.2, 5.0, 2000, 0.001)}
	_ = rt.MoveToPosition(calibration.MotionAxisConfig{ControllerID: "motion-1", Axis: "X", Name: "α"}, 30)

	completed, _, failure := rt.WaitForMotionComplete()

	if completed || failure == nil || failure.Verdict != traversal.MotionSafetyStatusUnavailable {
		t.Fatalf("disconnect result: completed=%v failure=%+v", completed, failure)
	}
}

func TestFallbackRuntimeWaitForMotionComplete_EmergencyStopped(t *testing.T) {
	mgr := &controllableMotionManager{}
	mgr.setStatus([]motion.ControllerStatus{{
		ID: "motion-1", Connected: true, EmergencyStopped: true,
		Axes: []motion.AxisStatus{{Name: motion.AxisX, Position: 10}},
	}})
	rt := &fallbackRuntime{motion: mgr, motionSafety: makeMotionSafetyConfig(0.2, 5.0, 2000, 0.001)}
	_ = rt.MoveToPosition(calibration.MotionAxisConfig{ControllerID: "motion-1", Axis: "X", Name: "α"}, 30)

	completed, _, failure := rt.WaitForMotionComplete()

	if completed || failure == nil || failure.Verdict != traversal.MotionSafetyStatusUnavailable {
		t.Fatalf("emergency-stopped result: completed=%v failure=%+v", completed, failure)
	}
}

func TestFallbackRuntimeWaitForMotionComplete_AxisMissingAfter3Snapshots(t *testing.T) {
	missing := []motion.ControllerStatus{{ID: "motion-1", Connected: true}}
	mgr := &controllableMotionManager{statusSequence: [][]motion.ControllerStatus{missing, missing, missing}}
	rt := &fallbackRuntime{motion: mgr, motionSafety: makeMotionSafetyConfig(0.2, 5.0, 2000, 0.001)}
	_ = rt.MoveToPosition(calibration.MotionAxisConfig{ControllerID: "motion-1", Axis: "X", Name: "α"}, 30)

	completed, _, failure := rt.WaitForMotionComplete()

	if completed || failure == nil || failure.Verdict != traversal.MotionSafetyStatusUnavailable {
		t.Fatalf("missing-axis result: completed=%v failure=%+v", completed, failure)
	}
}

func TestFallbackRuntimeWaitForMotionComplete_CriticalDeviation(t *testing.T) {
	// 测试前置：轴已停但偏差 6mm（>5.0 严重偏离阈值），目标 30mm
	mgr := &controllableMotionManager{}
	mgr.setStatus(makeStoppedAxisStatus("motion-1", "X", 36)) // 偏差 6mm
	rt := &fallbackRuntime{
		motion:       mgr,
		motionSafety: makeMotionSafetyConfig(0.2, 5.0, 2000, 0.001),
	}
	_ = rt.MoveToPosition(calibration.MotionAxisConfig{ControllerID: "motion-1", Axis: "X", Name: "α"}, 30)

	// 测试步骤：等待运动完成
	completed, _, failure := rt.WaitForMotionComplete()

	// 期待结果：检测到 CriticalDeviation 故障
	if completed {
		t.Fatal("critical deviation should not complete")
	}
	if failure == nil || failure.Verdict != traversal.MotionSafetyCriticalDeviation {
		t.Fatalf("failure verdict = %v, want CriticalDeviation", failure)
	}
}

func TestFallbackRuntimeWaitForMotionComplete_Deviation(t *testing.T) {
	// 测试前置：轴已停但偏差 1mm（>0.2 容差但 <5.0 严重偏离），目标 30mm
	mgr := &controllableMotionManager{}
	mgr.setStatus(makeStoppedAxisStatus("motion-1", "X", 31)) // 偏差 1mm
	rt := &fallbackRuntime{
		motion:       mgr,
		motionSafety: makeMotionSafetyConfig(0.2, 5.0, 2000, 0.001),
	}
	_ = rt.MoveToPosition(calibration.MotionAxisConfig{ControllerID: "motion-1", Axis: "X", Name: "α"}, 30)

	// 测试步骤：等待运动完成
	completed, _, failure := rt.WaitForMotionComplete()

	// 期待结果：检测到 Deviation 故障
	if completed {
		t.Fatal("deviation should not complete")
	}
	if failure == nil || failure.Verdict != traversal.MotionSafetyDeviation {
		t.Fatalf("failure verdict = %v, want Deviation", failure)
	}
}

func TestFallbackRuntimeWaitForMotionComplete_NoProgress(t *testing.T) {
	// 测试前置：运动中无进展——位置长时间不变，noProgressTimeoutMs=300ms
	mgr := &controllableMotionManager{}
	mgr.setStatus(makeMovingAxisStatus("motion-1", "X", 5)) // 位置始终 5，目标 30
	rt := &fallbackRuntime{
		motion:       mgr,
		motionSafety: makeMotionSafetyConfig(0.2, 5.0, 300, 0.001),
	}
	_ = rt.MoveToPosition(calibration.MotionAxisConfig{ControllerID: "motion-1", Axis: "X", Name: "α"}, 30)

	// 测试步骤：等待运动完成
	completed, _, failure := rt.WaitForMotionComplete()

	// 期待结果：检测到 NoProgress 故障
	if completed {
		t.Fatal("no progress should not complete")
	}
	if failure == nil || failure.Verdict != traversal.MotionSafetyNoProgress {
		t.Fatalf("failure verdict = %v, want NoProgress", failure)
	}
}

func TestFallbackRuntimeWaitForMotionComplete_Overshoot(t *testing.T) {
	// 测试前置：位置穿越目标——从负侧到正侧（5 → 35），目标 30
	mgr := &controllableMotionManager{}
	mgr.setStatus(makeMovingAxisStatus("motion-1", "X", 5)) // 起始负侧
	rt := &fallbackRuntime{
		motion:       mgr,
		motionSafety: makeMotionSafetyConfig(0.2, 5.0, 2000, 0.001),
	}
	_ = rt.MoveToPosition(calibration.MotionAxisConfig{ControllerID: "motion-1", Axis: "X", Name: "α"}, 30)
	// 异步切换：100ms 后改为已穿越到正侧
	go func() {
		time.Sleep(120 * time.Millisecond)
		mgr.setStatus(makeMovingAxisStatus("motion-1", "X", 35)) // 仍在运动但已穿越
	}()

	// 测试步骤：等待运动完成
	completed, _, failure := rt.WaitForMotionComplete()

	// 期待结果：检测到 Overshoot 故障
	if completed {
		t.Fatal("overshoot should not complete")
	}
	if failure == nil || failure.Verdict != traversal.MotionSafetyOvershoot {
		t.Fatalf("failure verdict = %v, want Overshoot", failure)
	}
}

func TestFallbackRuntimeWaitForMotionComplete_Paused(t *testing.T) {
	// 测试前置：运动中 + isPaused 返回 true
	mgr := &controllableMotionManager{}
	mgr.setStatus(makeMovingAxisStatus("motion-1", "X", 5))
	rt := &fallbackRuntime{
		motion:       mgr,
		motionSafety: makeMotionSafetyConfig(0.2, 5.0, 20000, 0.001), // 长 noProgress 避免误触发
		isPaused:     func() bool { return true },
	}
	_ = rt.MoveToPosition(calibration.MotionAxisConfig{ControllerID: "motion-1", Axis: "X", Name: "α"}, 30)

	// 测试步骤：等待运动完成
	completed, reason, failure := rt.WaitForMotionComplete()

	// 期待结果：(false, Paused, nil) — 暂停为非故障中断
	if completed {
		t.Fatal("paused should not complete")
	}
	if reason != traversal.MotionInterruptPaused {
		t.Fatalf("reason = %v, want Paused", reason)
	}
	if failure != nil {
		t.Fatalf("paused should not produce failure, got %v", failure)
	}
}

func TestFallbackRuntimeWaitForMotionComplete_NoMotionManager(t *testing.T) {
	// 测试前置：motion=nil（未注入运动控制器）
	rt := &fallbackRuntime{motion: nil}

	// 测试步骤：等待运动完成
	completed, reason, failure := rt.WaitForMotionComplete()

	// 期待结果：(true, None, nil) — 无运动控制器视为已到位
	if !completed || reason != traversal.MotionInterruptNone || failure != nil {
		t.Fatalf("no motion: completed=%v reason=%v failure=%v", completed, reason, failure)
	}
}

func TestFallbackRuntimeWaitForMotionComplete_EmptyTargets(t *testing.T) {
	// 测试前置：有 motion 但无目标（未调 MoveToPosition）
	mgr := &controllableMotionManager{}
	mgr.setStatus(makeStoppedAxisStatus("motion-1", "X", 0))
	rt := &fallbackRuntime{
		motion:       mgr,
		motionSafety: makeMotionSafetyConfig(0.2, 5.0, 2000, 0.001),
	}

	// 测试步骤：等待运动完成
	completed, reason, failure := rt.WaitForMotionComplete()

	// 期待结果：(true, None, nil) — 空目标视为无需等待
	if !completed || reason != traversal.MotionInterruptNone || failure != nil {
		t.Fatalf("empty targets: completed=%v reason=%v failure=%v", completed, reason, failure)
	}
}

// ==================== fallbackRuntime.EmergencyStopMotion 测试 ====================

func TestFallbackRuntimeEmergencyStopMotion_Success(t *testing.T) {
	// 测试前置：单控制器目标，EmergencyStop 成功
	mgr := &controllableMotionManager{}
	rt := &fallbackRuntime{
		motion: mgr,
		targets: map[calibrationMotionAxis]float64{
			{controllerID: "motion-1", axis: motion.AxisX}: 30,
		},
	}

	// 测试步骤：急停
	err := rt.EmergencyStopMotion()

	// 期待结果：无错误，急停调用 1 次，目标清空
	if err != nil {
		t.Fatalf("emergency stop should succeed, got %v", err)
	}
	if got := mgr.emergencyStopCount(); got != 1 {
		t.Fatalf("emergency stop calls = %d, want 1", got)
	}
	if len(rt.targets) != 0 {
		t.Fatalf("targets should be cleared, got %v", rt.targets)
	}
}

func TestFallbackRuntimeEmergencyStopMotion_FallbackStopOnError(t *testing.T) {
	// 测试前置：EmergencyStop 注入错误，应 fallback 到 stopAllMotion
	mgr := &controllableMotionManager{}
	mgr.emergencyStopErr = errors.New("injected emergency stop failure")
	rt := &fallbackRuntime{
		motion: mgr,
		targets: map[calibrationMotionAxis]float64{
			{controllerID: "motion-1", axis: motion.AxisX}: 30,
		},
	}
	// 同时设置运动中的状态，让 stopAllMotion 实际停止某轴
	mgr.setStatus(makeMovingAxisStatus("motion-1", "X", 15))

	// 测试步骤：急停
	err := rt.EmergencyStopMotion()

	// 期待结果：返回聚合错误，急停调用 1 次（失败），stop 调用 1 次（fallback）
	if err == nil {
		t.Fatal("emergency stop failure should return aggregated error")
	}
	if got := mgr.emergencyStopCount(); got != 1 {
		t.Fatalf("emergency stop calls = %d, want 1", got)
	}
	if got := mgr.stopCount(); got != 1 {
		t.Fatalf("fallback stop calls = %d, want 1", got)
	}
}

func TestFallbackRuntimeEmergencyStopMotion_NoTargetsStopsAllConnected(t *testing.T) {
	// 测试前置：targets 为空，但有两个已连接控制器——应全部急停
	mgr := &controllableMotionManager{}
	mgr.setStatus([]motion.ControllerStatus{
		{ID: "motion-1", Connected: true, Axes: []motion.AxisStatus{{Name: motion.AxisX, Moving: false}}},
		{ID: "motion-2", Connected: true, Axes: []motion.AxisStatus{{Name: motion.AxisY, Moving: false}}},
	})
	rt := &fallbackRuntime{motion: mgr}

	// 测试步骤：急停
	err := rt.EmergencyStopMotion()

	// 期待结果：无错误，急停调用 2 次（两个已连接控制器）
	if err != nil {
		t.Fatalf("emergency stop should succeed, got %v", err)
	}
	if got := mgr.emergencyStopCount(); got != 2 {
		t.Fatalf("emergency stop calls = %d, want 2 (all connected controllers)", got)
	}
}

// ==================== runtimeAdapter 测试 ====================

// safetyAwareRuntime 实现 MotionSafetyAwareRuntime，返回固定三元组。
type safetyAwareRuntime struct {
	completed bool
	reason    traversal.MotionInterruptReason
	failure   *traversal.MotionSafetyFailure
}

func (s *safetyAwareRuntime) GetChannelValue(string, int) (float64, bool) { return 0, false }
func (s *safetyAwareRuntime) GetLatestTimestamp(string) (int64, bool)     { return 0, false }
func (s *safetyAwareRuntime) MoveToPosition(calibration.MotionAxisConfig, float64) error {
	return nil
}
func (s *safetyAwareRuntime) WaitForMotionComplete() error { return nil }
func (s *safetyAwareRuntime) StopMotion() error            { return nil }
func (s *safetyAwareRuntime) WaitForMotionCompleteWithSafety() (bool, traversal.MotionInterruptReason, *traversal.MotionSafetyFailure) {
	return s.completed, s.reason, s.failure
}

func TestRuntimeAdapter_PassesThroughMotionSafetyAwareRuntime(t *testing.T) {
	// 测试前置：被包装 runtime 实现 MotionSafetyAwareRuntime，返回特定三元组
	inner := &safetyAwareRuntime{
		completed: false,
		reason:    traversal.MotionInterruptPaused,
		failure:   &traversal.MotionSafetyFailure{Axis: "X", Verdict: traversal.MotionSafetyOvershoot},
	}
	adapter := &runtimeAdapter{runtime: inner}

	// 测试步骤：等待运动完成
	completed, reason, failure := adapter.WaitForMotionComplete()

	// 期待结果：透传被包装对象的三元组
	if completed != false || reason != traversal.MotionInterruptPaused || failure != inner.failure {
		t.Fatalf("passthrough: completed=%v reason=%v failure=%v", completed, reason, failure)
	}
}

// legacyRuntime 仅实现基础 CalibrationRuntime（不实现 MotionSafetyAwareRuntime / EmergencyStopProvider）。
type legacyRuntime struct {
	waitErr error
	stopErr error
}

func (l *legacyRuntime) GetChannelValue(string, int) (float64, bool) { return 0, false }
func (l *legacyRuntime) GetLatestTimestamp(string) (int64, bool)     { return 0, false }
func (l *legacyRuntime) MoveToPosition(calibration.MotionAxisConfig, float64) error {
	return nil
}
func (l *legacyRuntime) WaitForMotionComplete() error { return l.waitErr }
func (l *legacyRuntime) StopMotion() error            { return l.stopErr }

func TestRuntimeAdapter_FallbackToLegacyWaitForMotionCompleteOnError(t *testing.T) {
	// 测试前置：被包装 runtime 不实现 MotionSafetyAwareRuntime，WaitForMotionComplete 返回错误
	inner := &legacyRuntime{waitErr: errors.New("motion timeout")}
	adapter := &runtimeAdapter{runtime: inner}

	// 测试步骤：等待运动完成
	completed, reason, failure := adapter.WaitForMotionComplete()

	// 期待结果：(false, Timeout, nil) — 旧实现返回 error 映射为 Timeout
	if completed || reason != traversal.MotionInterruptTimeout || failure != nil {
		t.Fatalf("fallback on error: completed=%v reason=%v failure=%v", completed, reason, failure)
	}
}

func TestRuntimeAdapter_FallbackToLegacyWaitForMotionCompleteOnSuccess(t *testing.T) {
	// 测试前置：被包装 runtime 不实现 MotionSafetyAwareRuntime，WaitForMotionComplete 返回 nil
	inner := &legacyRuntime{waitErr: nil}
	adapter := &runtimeAdapter{runtime: inner}

	// 测试步骤：等待运动完成
	completed, reason, failure := adapter.WaitForMotionComplete()

	// 期待结果：(true, None, nil) — 旧实现返回 nil 视为已到位
	if !completed || reason != traversal.MotionInterruptNone || failure != nil {
		t.Fatalf("fallback on success: completed=%v reason=%v failure=%v", completed, reason, failure)
	}
}

// emergencyStopProvider 实现 EmergencyStopProvider。
type emergencyStopProvider struct {
	legacyRuntime
	esCalled bool
	esErr    error
}

func (e *emergencyStopProvider) EmergencyStopMotion() error {
	e.esCalled = true
	return e.esErr
}

func TestRuntimeAdapter_PassesThroughEmergencyStopProvider(t *testing.T) {
	// 测试前置：被包装 runtime 实现 EmergencyStopProvider
	inner := &emergencyStopProvider{}
	adapter := &runtimeAdapter{runtime: inner}

	// 测试步骤：急停
	err := adapter.EmergencyStopMotion()

	// 期待结果：透传到被包装对象的 EmergencyStopMotion
	if err != nil {
		t.Fatalf("emergency stop should succeed, got %v", err)
	}
	if !inner.esCalled {
		t.Fatal("inner EmergencyStopMotion should be called")
	}
}

func TestRuntimeAdapter_FallbackEmergencyStopToStopMotion(t *testing.T) {
	// 测试前置：被包装 runtime 不实现 EmergencyStopProvider，应 fallback 到 StopMotion
	inner := &legacyRuntime{stopErr: nil}
	adapter := &runtimeAdapter{runtime: inner}

	// 测试步骤：急停
	err := adapter.EmergencyStopMotion()

	// 期待结果：调用 StopMotion（无错误）
	if err != nil {
		t.Fatalf("fallback to StopMotion should succeed, got %v", err)
	}
}

// ==================== handleCalibrationMotionSafetyFailure 测试 ====================

// emergencyStopCapableRuntime 实现 ports.CalibrationRuntime + ports.EmergencyStopProvider，
// 用于 handleCalibrationMotionSafetyFailure 测试。
//
// 设计动机：fallbackRuntime 实现的是 calibration.RuntimeAccess（三元组签名 WaitForMotionComplete），
// 无法直接赋值给 m.runtime（ports.CalibrationRuntime 类型，要求 WaitForMotionComplete() error）。
// 本 mock 复刻 fallbackRuntime 的 EmergencyStopMotion / StopMotion 转发行为（急停/停止都委托 controllableMotionManager），
// 便于通过计数器断言 handleCalibrationMotionSafetyFailure 的分发逻辑。
//
// 与 fallbackRuntime 的差异：
//   - WaitForMotionComplete 实现 ports.CalibrationRuntime 旧签名（直接返回 nil，handleCalibrationMotionSafetyFailure 不依赖此方法）
//   - 无 motionSafety / targets / isPaused 字段（handleCalibrationMotionSafetyFailure 测试不需要这些）
type emergencyStopCapableRuntime struct {
	motion ports.MotionManager
}

func newEmergencyStopCapableRuntime(motion ports.MotionManager) *emergencyStopCapableRuntime {
	return &emergencyStopCapableRuntime{motion: motion}
}

func (r *emergencyStopCapableRuntime) GetChannelValue(string, int) (float64, bool) { return 0, false }
func (r *emergencyStopCapableRuntime) GetLatestTimestamp(string) (int64, bool)     { return 0, false }
func (r *emergencyStopCapableRuntime) MoveToPosition(calibration.MotionAxisConfig, float64) error {
	return nil
}
func (r *emergencyStopCapableRuntime) WaitForMotionComplete() error { return nil }

// StopMotion 停止所有运动轴（与 fallbackRuntime.StopMotion 行为一致：转发到 stopAllMotion）。
func (r *emergencyStopCapableRuntime) StopMotion() error {
	if r.motion == nil {
		return nil
	}
	return stopAllMotion(r.motion)
}

// EmergencyStopMotion 急停所有已连接控制器（与 fallbackRuntime.EmergencyStopMotion 行为一致）。
// 任一控制器急停失败时聚合错误返回，由 handleCalibrationMotionSafetyFailure 决定是否升级错误码。
func (r *emergencyStopCapableRuntime) EmergencyStopMotion() error {
	if r.motion == nil {
		return nil
	}
	ctx := context.Background()
	var errs []error
	for _, s := range r.motion.StatusAll(ctx) {
		if !s.Connected {
			continue
		}
		if err := r.motion.EmergencyStop(ctx, s.ID); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func TestHandleCalibrationMotionSafetyFailure_CriticalDeviationTriggersEmergencyStop(t *testing.T) {
	// 测试前置：CriticalDeviation 故障，runtime 实现 EmergencyStopProvider
	mgr := &controllableMotionManager{}
	mgr.setStatus(makeStoppedAxisStatus("motion-1", "X", 36))
	// m.runtime 注入 emergencyStopCapableRuntime（实现 ports.CalibrationRuntime + EmergencyStopProvider）；
	// m.motion 注入同一 mgr，便于 fallback stopAllMotion 路径调用 mgr.Stop。
	m := NewCalibrationManager(nil, mgr, nil, nil)
	m.runtime = newEmergencyStopCapableRuntime(mgr)
	m.currentStatus = calibration.Status{TaskID: "test"}

	failure := &traversal.MotionSafetyFailure{
		ControllerID: "motion-1",
		Axis:         "X",
		Verdict:      traversal.MotionSafetyCriticalDeviation,
		Target:       30,
		Actual:       36,
		PointIndex:   0,
	}

	// 测试步骤：处理故障
	err := m.handleCalibrationMotionSafetyFailure(m.createRuntime(nil), failure)

	// 期待结果：调用 EmergencyStop，写入 ErrCriticalPositionDeviation 错误码
	if err == nil {
		t.Fatal("should return error for critical deviation")
	}
	if got := mgr.emergencyStopCount(); got != 1 {
		t.Fatalf("emergency stop calls = %d, want 1", got)
	}
	if m.currentStatus.LastErrorCode != string(traversal.ErrCriticalPositionDeviation) {
		t.Fatalf("LastErrorCode = %v, want %v", m.currentStatus.LastErrorCode, traversal.ErrCriticalPositionDeviation)
	}
	if m.currentStatus.State != calibration.StateError {
		t.Fatalf("State = %v, want Error", m.currentStatus.State)
	}
}

func TestHandleCalibrationMotionSafetyFailure_UsesCurrentFallbackRuntime(t *testing.T) {
	mgr := &controllableMotionManager{}
	mgr.setStatus(makeStoppedAxisStatus("motion-1", "X", 36))
	m := NewCalibrationManager(nil, mgr, nil, nil)
	runtime := m.createRuntime(nil)
	fallback, ok := runtime.(*fallbackRuntime)
	if !ok {
		t.Fatalf("runtime type = %T, want *fallbackRuntime", runtime)
	}
	fallback.targets = map[calibrationMotionAxis]float64{
		{controllerID: "motion-1", axis: motion.AxisX}: 30,
	}
	failure := &traversal.MotionSafetyFailure{
		ControllerID: "motion-1", Axis: "X", Verdict: traversal.MotionSafetyCriticalDeviation,
		Target: 30, Actual: 36,
	}

	err := m.handleCalibrationMotionSafetyFailure(runtime, failure)

	if err == nil {
		t.Fatal("should return error for critical deviation")
	}
	if got := mgr.emergencyStopCount(); got != 1 {
		t.Fatalf("emergency stop calls = %d, want 1", got)
	}
}

func TestHandleCalibrationMotionSafetyFailure_LimitTriggeredTriggersEmergencyStop(t *testing.T) {
	// 测试前置：LimitTriggered 故障
	mgr := &controllableMotionManager{}
	mgr.setStatus(makeLimitTriggeredStatus("motion-1", "X", 35, true))
	m := NewCalibrationManager(nil, mgr, nil, nil)
	m.runtime = newEmergencyStopCapableRuntime(mgr)
	m.currentStatus = calibration.Status{TaskID: "test"}

	failure := &traversal.MotionSafetyFailure{
		ControllerID: "motion-1",
		Axis:         "X",
		Verdict:      traversal.MotionSafetyLimitTriggered,
		Target:       30,
		Actual:       35,
		PointIndex:   0,
	}

	// 测试步骤：处理故障
	err := m.handleCalibrationMotionSafetyFailure(m.createRuntime(nil), failure)

	// 期待结果：调用 EmergencyStop，写入 ErrLimitSwitchTriggered 错误码
	if err == nil {
		t.Fatal("should return error for limit triggered")
	}
	if got := mgr.emergencyStopCount(); got != 1 {
		t.Fatalf("emergency stop calls = %d, want 1", got)
	}
	if m.currentStatus.LastErrorCode != string(traversal.ErrLimitSwitchTriggered) {
		t.Fatalf("LastErrorCode = %v, want %v", m.currentStatus.LastErrorCode, traversal.ErrLimitSwitchTriggered)
	}
}

func TestHandleCalibrationMotionSafetyFailure_DeviationCallsStopNotEmergency(t *testing.T) {
	// 测试前置：Deviation 故障（普通停止类，不应急停）
	mgr := &controllableMotionManager{}
	mgr.setStatus(makeStoppedAxisStatus("motion-1", "X", 31)) // 偏差 1mm
	m := NewCalibrationManager(nil, mgr, nil, nil)
	m.runtime = newEmergencyStopCapableRuntime(mgr)
	m.currentStatus = calibration.Status{TaskID: "test"}

	failure := &traversal.MotionSafetyFailure{
		ControllerID: "motion-1",
		Axis:         "X",
		Verdict:      traversal.MotionSafetyDeviation,
		Target:       30,
		Actual:       31,
		PointIndex:   0,
	}

	// 测试步骤：处理故障
	err := m.handleCalibrationMotionSafetyFailure(m.createRuntime(nil), failure)

	// 期待结果：调用 StopMotion 而非 EmergencyStop，写入 ErrPositionDeviation
	if err == nil {
		t.Fatal("should return error for deviation")
	}
	if got := mgr.emergencyStopCount(); got != 0 {
		t.Fatalf("emergency stop calls = %d, want 0 (deviation is not emergency)", got)
	}
	if m.currentStatus.LastErrorCode != string(traversal.ErrPositionDeviation) {
		t.Fatalf("LastErrorCode = %v, want %v", m.currentStatus.LastErrorCode, traversal.ErrPositionDeviation)
	}
}

func TestHandleCalibrationMotionSafetyFailure_NoProgressCallsStopNotEmergency(t *testing.T) {
	// 测试前置：NoProgress 故障（普通停止类）
	mgr := &controllableMotionManager{}
	m := NewCalibrationManager(nil, mgr, nil, nil)
	m.runtime = newEmergencyStopCapableRuntime(mgr)
	m.currentStatus = calibration.Status{TaskID: "test"}

	failure := &traversal.MotionSafetyFailure{
		ControllerID: "motion-1",
		Axis:         "X",
		Verdict:      traversal.MotionSafetyNoProgress,
		Target:       30,
		Actual:       5,
		PointIndex:   0,
	}

	// 测试步骤：处理故障
	err := m.handleCalibrationMotionSafetyFailure(m.createRuntime(nil), failure)

	// 期待结果：调用 StopMotion 而非 EmergencyStop，写入 ErrMotionNoProgress
	if err == nil {
		t.Fatal("should return error for no progress")
	}
	if got := mgr.emergencyStopCount(); got != 0 {
		t.Fatalf("emergency stop calls = %d, want 0 (no progress is not emergency)", got)
	}
	if m.currentStatus.LastErrorCode != string(traversal.ErrMotionNoProgress) {
		t.Fatalf("LastErrorCode = %v, want %v", m.currentStatus.LastErrorCode, traversal.ErrMotionNoProgress)
	}
}

func TestHandleCalibrationMotionSafetyFailure_EmergencyStopFailFallbackToStop(t *testing.T) {
	// 测试前置：CriticalDeviation 故障，但 EmergencyStop 注入错误
	mgr := &controllableMotionManager{}
	mgr.emergencyStopErr = errors.New("emergency stop hardware failure")
	mgr.setStatus(makeMovingAxisStatus("motion-1", "X", 36))
	m := NewCalibrationManager(nil, mgr, nil, nil)
	m.runtime = newEmergencyStopCapableRuntime(mgr)
	m.currentStatus = calibration.Status{TaskID: "test"}

	failure := &traversal.MotionSafetyFailure{
		ControllerID: "motion-1",
		Axis:         "X",
		Verdict:      traversal.MotionSafetyCriticalDeviation,
		Target:       30,
		Actual:       36,
		PointIndex:   0,
	}

	// 测试步骤：处理故障
	err := m.handleCalibrationMotionSafetyFailure(m.createRuntime(nil), failure)

	// 期待结果：急停失败 → fallback 到 StopMotion + 错误码升级为 ErrEmergencyStopFailed
	if err == nil {
		t.Fatal("should return error for emergency stop failure")
	}
	if got := mgr.emergencyStopCount(); got != 1 {
		t.Fatalf("emergency stop calls = %d, want 1 (attempted before fallback)", got)
	}
	if got := mgr.stopCount(); got < 1 {
		t.Fatalf("fallback stop calls = %d, want >=1 (stopAllMotion on moving axes)", got)
	}
	if m.currentStatus.LastErrorCode != string(traversal.ErrEmergencyStopFailed) {
		t.Fatalf("LastErrorCode = %v, want %v", m.currentStatus.LastErrorCode, traversal.ErrEmergencyStopFailed)
	}
}

func TestHandleCalibrationMotionSafetyFailure_WritesFailureSnapshotAfterFailWithCode(t *testing.T) {
	// 测试前置：任意故障——验证 recordMotionSafetyFailure 在 failWithCode 之后写入快照
	mgr := &controllableMotionManager{}
	m := NewCalibrationManager(nil, mgr, nil, nil)
	m.runtime = newEmergencyStopCapableRuntime(mgr)
	m.currentStatus = calibration.Status{TaskID: "test"}

	failure := &traversal.MotionSafetyFailure{
		ControllerID: "motion-1",
		Axis:         "X",
		Verdict:      traversal.MotionSafetyDeviation,
		Target:       30,
		Actual:       31,
		PointIndex:   2,
	}

	// 测试步骤：处理故障
	_ = m.handleCalibrationMotionSafetyFailure(m.createRuntime(nil), failure)

	// 期待结果：status.MotionSafetyFailure 已写入快照（failWithCode 内部清空，recordMotionSafetyFailure 之后重新写入）
	snapshot := m.currentStatus.MotionSafetyFailure
	if snapshot == nil {
		t.Fatal("MotionSafetyFailure should be written after failWithCode clears it")
	}
	if snapshot.Axis != "X" || snapshot.Verdict != traversal.MotionSafetyDeviation || snapshot.Target != 30 || snapshot.Actual != 31 || snapshot.PointIndex != 2 {
		t.Fatalf("snapshot mismatch: %+v", snapshot)
	}
}

func TestHandleCalibrationMotionSafetyFailure_NilFailureReturnsNil(t *testing.T) {
	// 测试前置：nil 故障（防御性边界场景）
	m := NewCalibrationManager(nil, nil, nil, nil)

	// 测试步骤：处理 nil 故障
	err := m.handleCalibrationMotionSafetyFailure(m.createRuntime(nil), nil)

	// 期待结果：无错误返回（nil 故障视为无操作）
	if err != nil {
		t.Fatalf("nil failure should return nil, got %v", err)
	}
}

// ==================== C-3：急停 + fallback 双错误保留测试 ====================

// 测试级 sentinel 错误：用于 errors.Is 断言返回错误 cause 链中的急停/回退停止根因。
var (
	errSentinelEmergencyStop = errors.New("sentinel: emergency stop bus offline")
	errSentinelFallbackStop  = errors.New("sentinel: fallback stop valve stuck")
)

// scriptedSafetyRuntime 可编程 runtime：直接注入急停/普通停止的返回错误并计数，
// 便于用 sentinel error 精确断言 handleCalibrationMotionSafetyFailure 返回错误的 cause 链。
//
// 与 emergencyStopCapableRuntime 的差异：不经过 controllableMotionManager 转发，
// EmergencyStopMotion / StopMotion 原样返回注入的 sentinel，保证 errors.Is 直接命中。
type scriptedSafetyRuntime struct {
	esErr     error
	stopErr   error
	esCalls   int
	stopCalls int
}

func (s *scriptedSafetyRuntime) GetChannelValue(string, int) (float64, bool) { return 0, false }
func (s *scriptedSafetyRuntime) GetLatestTimestamp(string) (int64, bool)     { return 0, false }
func (s *scriptedSafetyRuntime) MoveToPosition(calibration.MotionAxisConfig, float64) error {
	return nil
}
func (s *scriptedSafetyRuntime) WaitForMotionComplete() error { return nil }
func (s *scriptedSafetyRuntime) StopMotion() error {
	s.stopCalls++
	return s.stopErr
}
func (s *scriptedSafetyRuntime) EmergencyStopMotion() error {
	s.esCalls++
	return s.esErr
}

// 顺序断言说明：测试无法 hook CalibrationManager 内部方法调用顺序，
// 但 failWithCode 内部会清空 MotionSafetyFailure（calibration.go），
// 因此调用结束后快照非 nil 即证明 recordMotionSafetyFailure 在 failWithCode 之后执行；
// LastErrorCode/LastError 非空则证明 failWithCode 已执行。二者合并等价于顺序断言。
// 既有测试 TestHandleCalibrationMotionSafetyFailure_WritesFailureSnapshotAfterFailWithCode 采用同一推断方式。

func TestHandleCalibrationMotionSafetyFailure_EmergencyStopFailureKeepsCauseInChain(t *testing.T) {
	// 测试前置：CriticalDeviation 故障；急停返回 sentinel 错误，fallback StopMotion 成功
	rt := &scriptedSafetyRuntime{esErr: errSentinelEmergencyStop}
	m := NewCalibrationManager(nil, nil, nil, nil)
	m.runtime = rt
	m.currentStatus = calibration.Status{TaskID: "test"}

	failure := &traversal.MotionSafetyFailure{
		ControllerID: "motion-1",
		Axis:         "X",
		Verdict:      traversal.MotionSafetyCriticalDeviation,
		Target:       30,
		Actual:       36,
		PointIndex:   1,
	}

	// 测试步骤：处理故障
	err := m.handleCalibrationMotionSafetyFailure(m.createRuntime(nil), failure)

	// 期待结果：返回 error 且 cause 链可识别急停根因
	if err == nil {
		t.Fatal("急停失败应返回 error")
	}
	if !errors.Is(err, errSentinelEmergencyStop) {
		t.Fatalf("返回错误应可通过 errors.Is 识别急停根因，实际: %v", err)
	}
	// 急停尝试 1 次 + fallback 普通停止 1 次
	if rt.esCalls != 1 || rt.stopCalls != 1 {
		t.Fatalf("esCalls=%d stopCalls=%d, want 1/1", rt.esCalls, rt.stopCalls)
	}
	// 错误码升级 + 快照已记录（顺序断言见上方说明）
	if m.currentStatus.State != calibration.StateError {
		t.Fatalf("State = %v, want Error", m.currentStatus.State)
	}
	if m.currentStatus.LastErrorCode != string(traversal.ErrEmergencyStopFailed) {
		t.Fatalf("LastErrorCode = %v, want %v", m.currentStatus.LastErrorCode, traversal.ErrEmergencyStopFailed)
	}
	snapshot := m.currentStatus.MotionSafetyFailure
	if snapshot == nil {
		t.Fatal("MotionSafetyFailure 快照应已记录（failWithCode 之后）")
	}
	if snapshot.Verdict != traversal.MotionSafetyCriticalDeviation || snapshot.Axis != "X" || snapshot.PointIndex != 1 {
		t.Fatalf("快照内容不符: %+v", snapshot)
	}
}

func TestHandleCalibrationMotionSafetyFailure_FallbackAlsoFailsPreservesBothCauses(t *testing.T) {
	// 测试前置：CriticalDeviation 故障；急停与 fallback StopMotion 都返回 sentinel 错误
	rt := &scriptedSafetyRuntime{esErr: errSentinelEmergencyStop, stopErr: errSentinelFallbackStop}
	m := NewCalibrationManager(nil, nil, nil, nil)
	m.runtime = rt
	m.currentStatus = calibration.Status{TaskID: "test"}

	failure := &traversal.MotionSafetyFailure{
		ControllerID: "motion-1",
		Axis:         "X",
		Verdict:      traversal.MotionSafetyCriticalDeviation,
		Target:       30,
		Actual:       36,
		PointIndex:   1,
	}

	// 测试步骤：处理故障
	err := m.handleCalibrationMotionSafetyFailure(m.createRuntime(nil), failure)

	// 期待结果：返回 error 的 cause 链中急停与 fallback 两个根因都可识别
	if err == nil {
		t.Fatal("急停与 fallback 双失败应返回 error")
	}
	if !errors.Is(err, errSentinelEmergencyStop) {
		t.Fatalf("返回错误应可通过 errors.Is 识别急停根因，实际: %v", err)
	}
	if !errors.Is(err, errSentinelFallbackStop) {
		t.Fatalf("返回错误应可通过 errors.Is 识别 fallback 停止根因（当前被吞），实际: %v", err)
	}
	// 用户可见消息同时包含两个根因，避免 fallback 失败被静默吞掉
	if !strings.Contains(m.currentStatus.LastError, errSentinelEmergencyStop.Error()) ||
		!strings.Contains(m.currentStatus.LastError, errSentinelFallbackStop.Error()) {
		t.Fatalf("LastError 应同时包含急停与 fallback 根因，实际: %q", m.currentStatus.LastError)
	}
	// 错误码升级 + 快照已记录（顺序断言见上方说明）
	if m.currentStatus.State != calibration.StateError {
		t.Fatalf("State = %v, want Error", m.currentStatus.State)
	}
	if m.currentStatus.LastErrorCode != string(traversal.ErrEmergencyStopFailed) {
		t.Fatalf("LastErrorCode = %v, want %v", m.currentStatus.LastErrorCode, traversal.ErrEmergencyStopFailed)
	}
	snapshot := m.currentStatus.MotionSafetyFailure
	if snapshot == nil {
		t.Fatal("MotionSafetyFailure 快照应已记录（failWithCode 之后）")
	}
	if snapshot.Verdict != traversal.MotionSafetyCriticalDeviation || snapshot.Axis != "X" {
		t.Fatalf("快照内容不符: %+v", snapshot)
	}
	if rt.esCalls != 1 || rt.stopCalls != 1 {
		t.Fatalf("esCalls=%d stopCalls=%d, want 1/1", rt.esCalls, rt.stopCalls)
	}
}

func TestHandleCalibrationMotionSafetyFailure_EmergencyStopSuccessKeepsStandardSemantics(t *testing.T) {
	// 测试前置：CriticalDeviation 故障；急停成功（无注入错误）
	rt := &scriptedSafetyRuntime{}
	m := NewCalibrationManager(nil, nil, nil, nil)
	m.runtime = rt
	m.currentStatus = calibration.Status{TaskID: "test"}

	failure := &traversal.MotionSafetyFailure{
		ControllerID: "motion-1",
		Axis:         "X",
		Verdict:      traversal.MotionSafetyCriticalDeviation,
		Target:       30,
		Actual:       36,
		PointIndex:   1,
	}

	// 测试步骤：处理故障
	err := m.handleCalibrationMotionSafetyFailure(m.createRuntime(nil), failure)

	// 期待结果：急停成功路径语义不回归——返回 error 标识运动安全故障，
	// 错误码保持 ErrCriticalPositionDeviation（不升级为 ErrEmergencyStopFailed），不调用普通停止
	if err == nil {
		t.Fatal("运动安全故障应返回 error")
	}
	if errors.Is(err, errSentinelEmergencyStop) || errors.Is(err, errSentinelFallbackStop) {
		t.Fatalf("急停成功路径不应携带急停/停止根因，实际: %v", err)
	}
	if rt.esCalls != 1 || rt.stopCalls != 0 {
		t.Fatalf("esCalls=%d stopCalls=%d, want 1/0（急停成功无需 fallback）", rt.esCalls, rt.stopCalls)
	}
	if m.currentStatus.LastErrorCode != string(traversal.ErrCriticalPositionDeviation) {
		t.Fatalf("LastErrorCode = %v, want %v", m.currentStatus.LastErrorCode, traversal.ErrCriticalPositionDeviation)
	}
	if m.currentStatus.State != calibration.StateError {
		t.Fatalf("State = %v, want Error", m.currentStatus.State)
	}
	if m.currentStatus.MotionSafetyFailure == nil {
		t.Fatal("MotionSafetyFailure 快照应已记录（failWithCode 之后）")
	}
}

// ==================== spec Task 22：结构化日志可检索性测试 ====================

// TestHandleCalibrationMotionSafetyFailure_EmitsStructuredSlogError 验证 motion safety failure
// 触发 slog.Error 含可检索的 'motion safety failure' 消息（spec Task 22：关键 safety 字段可检索）。
//
// 测试前置：withRecordingLogger 临时替换全局 logger（defer restore 保证恢复，不污染后续测试）
// 测试步骤：调用 handleCalibrationMotionSafetyFailure（CriticalDeviation 故障）
// 期待结果：slog.Error 记录被捕获，消息含 'motion safety failure' 子串
func TestHandleCalibrationMotionSafetyFailure_EmitsStructuredSlogError(t *testing.T) {
	// 测试前置：recordingSlogHandler 替换 slog.Default()，defer restore 恢复
	handler, restore := withRecordingLogger(t)
	defer restore()

	rt := &scriptedSafetyRuntime{}
	m := NewCalibrationManager(nil, nil, nil, nil)
	m.runtime = rt
	m.currentStatus = calibration.Status{TaskID: "test"}

	failure := &traversal.MotionSafetyFailure{
		ControllerID: "motion-1",
		Axis:         "X",
		Verdict:      traversal.MotionSafetyCriticalDeviation,
		Target:       30,
		Actual:       36,
		PointIndex:   1,
	}

	// 测试步骤：处理故障（返回值已由前序测试覆盖，此处只验证 slog 副作用）
	_ = m.handleCalibrationMotionSafetyFailure(m.createRuntime(nil), failure)

	// 期待结果：slog.Error 含 'motion safety failure' 消息被记录
	if !handler.hasLevelMessage(slog.LevelError, "motion safety failure") {
		t.Fatal("应记录 slog.Error 含 'motion safety failure' 消息，实际未记录")
	}
}
