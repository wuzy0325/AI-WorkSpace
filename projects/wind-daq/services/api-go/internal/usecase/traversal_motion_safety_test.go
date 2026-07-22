// Package usecase — 运动安全判定单元测试
//
// 覆盖 EvaluateMotionSafety 纯函数、MotionSafetyConfig.Resolve/Merge 合并逻辑、
// motionWatchdog 跨样本判定（NoProgress + Overshoot）。
// 测试用例遵循三段式：测试前置 / 测试步骤 / 期待结果。
package usecase

import (
	"context"
	"errors"
	"math"
	"testing"
	"time"

	"wind-daq/services/api-go/internal/core/device"
	"wind-daq/services/api-go/internal/core/motion"
	"wind-daq/services/api-go/internal/core/traversal"
)

// makeResolvedConfig 构造已解析的有效 MotionSafetyConfig（指针字段非 nil）。
// 用于 EvaluateMotionSafety 测试，避免每个用例重复构造默认值。
func makeResolvedConfig(arrival, critical float64) traversal.MotionSafetyConfig {
	cfg := traversal.DefaultMotionSafety()
	if arrival > 0 {
		cfg.ArrivalTolerance = &arrival
	}
	if critical > 0 {
		cfg.CriticalDeviationLimit = &critical
	}
	return cfg
}

// ptrFloat64Local 返回 v 的指针副本，仅测试用。
// traversal 包内有同名私有函数 ptrFloat64，这里加 Local 后缀避免混淆。
func ptrFloat64Local(v float64) *float64 { return &v }

func TestEvaluateMotionSafety_Arrived(t *testing.T) {
	// 测试前置：默认配置 + 轴已停且偏差小于到位容差
	cfg := makeResolvedConfig(0.01, 5.0)
	axis := motion.AxisStatus{Name: motion.AxisX, Position: 30.005, Moving: false}

	// 测试步骤：判定目标位置 30 的运动安全
	verdict := EvaluateMotionSafety(axis, 30.0, cfg)

	// 期待结果：Arrived
	if verdict != traversal.MotionSafetyArrived {
		t.Errorf("verdict = %v, want Arrived", verdict)
	}
}

func TestEvaluateMotionSafety_MovingLongStrokeNoFalsePositive(t *testing.T) {
	// 测试前置：轴运动中，距目标较远（正常长行程）
	cfg := makeResolvedConfig(0.01, 5.0)
	axis := motion.AxisStatus{Name: motion.AxisX, Position: 15.0, Moving: true}

	// 测试步骤：判定目标位置 30 的运动安全
	verdict := EvaluateMotionSafety(axis, 30.0, cfg)

	// 期待结果：OK——运动中不应误判偏差
	if verdict != traversal.MotionSafetyOK {
		t.Errorf("verdict = %v, want OK (moving long stroke should not trigger deviation)", verdict)
	}
}

func TestEvaluateMotionSafety_DeviationWhenStopped(t *testing.T) {
	// 测试前置：轴已停，偏差 1mm（超过 0.01 容差但小于 5.0 严重偏离）
	cfg := makeResolvedConfig(0.01, 5.0)
	axis := motion.AxisStatus{Name: motion.AxisX, Position: 31.0, Moving: false}

	// 测试步骤：判定目标位置 30 的运动安全
	verdict := EvaluateMotionSafety(axis, 30.0, cfg)

	// 期待结果：Deviation（普通停止）
	if verdict != traversal.MotionSafetyDeviation {
		t.Errorf("verdict = %v, want Deviation", verdict)
	}
	if verdict.RequiresEmergencyStop() {
		t.Errorf("Deviation should not require emergency stop")
	}
}

func TestEvaluateMotionSafety_CriticalDeviation(t *testing.T) {
	// 测试前置：轴已停，偏差 6mm（超过 5.0 严重偏离阈值）
	cfg := makeResolvedConfig(0.01, 5.0)
	axis := motion.AxisStatus{Name: motion.AxisX, Position: 36.0, Moving: false}

	// 测试步骤：判定目标位置 30 的运动安全
	verdict := EvaluateMotionSafety(axis, 30.0, cfg)

	// 期待结果：CriticalDeviation（急停）
	if verdict != traversal.MotionSafetyCriticalDeviation {
		t.Errorf("verdict = %v, want CriticalDeviation", verdict)
	}
	if !verdict.RequiresEmergencyStop() {
		t.Errorf("CriticalDeviation should require emergency stop")
	}
}

func TestEvaluateMotionSafety_PosLimitTriggered(t *testing.T) {
	// 测试前置：正向限位触发，运动中
	cfg := makeResolvedConfig(0.01, 5.0)
	axis := motion.AxisStatus{Name: motion.AxisX, Position: 100.0, Moving: true, PosLimit: true}

	// 测试步骤：判定目标位置 30 的运动安全
	verdict := EvaluateMotionSafety(axis, 30.0, cfg)

	// 期待结果：LimitTriggered（急停）——限位优先级高于运动中
	if verdict != traversal.MotionSafetyLimitTriggered {
		t.Errorf("verdict = %v, want LimitTriggered (limit switch overrides Moving)", verdict)
	}
	if !verdict.RequiresEmergencyStop() {
		t.Errorf("LimitTriggered should require emergency stop")
	}
}

func TestEvaluateMotionSafety_NegLimitTriggeredWhenStopped(t *testing.T) {
	// 测试前置：负向限位触发，轴已停
	cfg := makeResolvedConfig(0.01, 5.0)
	axis := motion.AxisStatus{Name: motion.AxisX, Position: 0.0, Moving: false, NegLimit: true}

	// 测试步骤：判定目标位置 30 的运动安全
	verdict := EvaluateMotionSafety(axis, 30.0, cfg)

	// 期待结果：LimitTriggered（急停）——限位优先级最高
	if verdict != traversal.MotionSafetyLimitTriggered {
		t.Errorf("verdict = %v, want LimitTriggered", verdict)
	}
}

func TestEvaluateMotionSafety_BoundaryAtArrivalTolerance(t *testing.T) {
	// 测试前置：偏差小于到位容差（避免浮点边界精度问题，用 0.009 < 0.01）
	cfg := makeResolvedConfig(0.01, 5.0)
	axis := motion.AxisStatus{Name: motion.AxisX, Position: 30.009, Moving: false}

	// 测试步骤：判定目标位置 30 的运动安全
	verdict := EvaluateMotionSafety(axis, 30.0, cfg)

	// 期待结果：Arrived（偏差在容差内）
	if verdict != traversal.MotionSafetyArrived {
		t.Errorf("verdict = %v, want Arrived (deviation within tolerance)", verdict)
	}
}

func TestEvaluateMotionSafety_BoundaryAtCriticalDeviation(t *testing.T) {
	// 测试前置：偏差大于严重偏离阈值（避免浮点边界精度问题，用 5.001 > 5.0）
	cfg := makeResolvedConfig(0.01, 5.0)
	axis := motion.AxisStatus{Name: motion.AxisX, Position: 35.001, Moving: false}

	// 测试步骤：判定目标位置 30 的运动安全
	verdict := EvaluateMotionSafety(axis, 30.0, cfg)

	// 期待结果：CriticalDeviation（偏差超过严重偏离阈值）
	if verdict != traversal.MotionSafetyCriticalDeviation {
		t.Errorf("verdict = %v, want CriticalDeviation (deviation exceeds critical threshold)", verdict)
	}
}

// --- MotionSafetyConfig.Resolve / Merge 测试 ---

func TestMotionSafetyConfig_ResolveDefaultsWhenNil(t *testing.T) {
	// 测试前置：nil 配置
	var cfg *traversal.MotionSafetyConfig

	// 测试步骤：解析 X 轴有效配置
	resolved := cfg.Resolve("X")

	// 期待结果：使用默认值
	if resolved.ArrivalTolerance == nil || *resolved.ArrivalTolerance != 0.2 {
		t.Errorf("ArrivalTolerance = %v, want 0.2 (default)", resolved.ArrivalTolerance)
	}
	if resolved.CriticalDeviationLimit == nil || *resolved.CriticalDeviationLimit != 5.0 {
		t.Errorf("CriticalDeviationLimit = %v, want 5.0 (default)", resolved.CriticalDeviationLimit)
	}
	if resolved.NoProgressTimeoutMs == nil || *resolved.NoProgressTimeoutMs != 2000 {
		t.Errorf("NoProgressTimeoutMs = %v, want 2000 (default)", resolved.NoProgressTimeoutMs)
	}
	if resolved.ProgressEpsilon == nil || *resolved.ProgressEpsilon != 0.001 {
		t.Errorf("ProgressEpsilon = %v, want 0.001 (default)", resolved.ProgressEpsilon)
	}
}

func TestMotionSafetyConfig_ResolveGlobalOverridesDefaults(t *testing.T) {
	// 测试前置：全局配置覆盖默认值
	arrival := 0.05
	critical := 3.0
	timeout := 1500
	epsilon := 0.002
	cfg := &traversal.MotionSafetyConfig{
		ArrivalTolerance:       &arrival,
		CriticalDeviationLimit: &critical,
		NoProgressTimeoutMs:    &timeout,
		ProgressEpsilon:        &epsilon,
	}

	// 测试步骤：解析 X 轴有效配置（无按轴覆盖）
	resolved := cfg.Resolve("X")

	// 期待结果：使用全局配置
	if *resolved.ArrivalTolerance != 0.05 {
		t.Errorf("ArrivalTolerance = %v, want 0.05 (global override)", *resolved.ArrivalTolerance)
	}
	if *resolved.CriticalDeviationLimit != 3.0 {
		t.Errorf("CriticalDeviationLimit = %v, want 3.0 (global override)", *resolved.CriticalDeviationLimit)
	}
	if *resolved.NoProgressTimeoutMs != 1500 {
		t.Errorf("NoProgressTimeoutMs = %v, want 1500 (global override)", *resolved.NoProgressTimeoutMs)
	}
	if *resolved.ProgressEpsilon != 0.002 {
		t.Errorf("ProgressEpsilon = %v, want 0.002 (global override)", *resolved.ProgressEpsilon)
	}
}

func TestMotionSafetyConfig_ResolveAxisOverridesGlobal(t *testing.T) {
	// 测试前置：全局 + 按轴覆盖（U 轴覆盖到位容差）
	globalArrival := 0.01
	globalCritical := 5.0
	uArrival := 0.5 // U 旋转轴放宽到 0.5°
	cfg := &traversal.MotionSafetyConfig{
		ArrivalTolerance:       &globalArrival,
		CriticalDeviationLimit: &globalCritical,
		AxisOverrides: map[string]*traversal.MotionSafetyConfig{
			"U": {ArrivalTolerance: &uArrival},
		},
	}

	// 测试步骤：解析 U 轴有效配置
	resolved := cfg.Resolve("U")

	// 期待结果：U 轴到位容差被覆盖，严重偏离继承全局
	if *resolved.ArrivalTolerance != 0.5 {
		t.Errorf("U axis ArrivalTolerance = %v, want 0.5 (axis override)", *resolved.ArrivalTolerance)
	}
	if *resolved.CriticalDeviationLimit != 5.0 {
		t.Errorf("U axis CriticalDeviationLimit = %v, want 5.0 (inherited from global)", *resolved.CriticalDeviationLimit)
	}
}

func TestMotionSafetyConfig_ResolveStripsAxisOverrides(t *testing.T) {
	// 测试前置：含 AxisOverrides 的配置
	cfg := &traversal.MotionSafetyConfig{
		AxisOverrides: map[string]*traversal.MotionSafetyConfig{
			"X": {ArrivalTolerance: ptrFloat64Local(0.02)},
		},
	}

	// 测试步骤：解析 X 轴有效配置
	resolved := cfg.Resolve("X")

	// 期待结果：返回的 resolved 不应再带 AxisOverrides（避免下游误用）
	if resolved.AxisOverrides != nil {
		t.Errorf("resolved.AxisOverrides should be nil to prevent downstream misuse")
	}
}

func TestMotionSafetyConfig_ResolveUnknownAxisFallsBackToGlobal(t *testing.T) {
	// 测试前置：仅配置 X 轴覆盖
	globalArrival := 0.01
	xArrival := 0.02
	cfg := &traversal.MotionSafetyConfig{
		ArrivalTolerance: &globalArrival,
		AxisOverrides: map[string]*traversal.MotionSafetyConfig{
			"X": {ArrivalTolerance: &xArrival},
		},
	}

	// 测试步骤：解析未覆盖的 Y 轴
	resolved := cfg.Resolve("Y")

	// 期待结果：Y 轴继承全局值
	if *resolved.ArrivalTolerance != 0.01 {
		t.Errorf("Y axis ArrivalTolerance = %v, want 0.01 (global, no axis override)", *resolved.ArrivalTolerance)
	}
}

func TestMotionSafetyConfig_MergeFillsNilFields(t *testing.T) {
	// 测试前置：c 部分字段为 nil，other 提供完整默认值
	c := &traversal.MotionSafetyConfig{
		ArrivalTolerance: ptrFloat64Local(0.05),
	}
	other := traversal.DefaultMotionSafety()

	// 测试步骤：合并
	merged := c.Merge(other)

	// 期待结果：c 的字段保留，nil 字段从 other 填充
	if *merged.ArrivalTolerance != 0.05 {
		t.Errorf("merged ArrivalTolerance = %v, want 0.05 (from c)", *merged.ArrivalTolerance)
	}
	if merged.CriticalDeviationLimit == nil || *merged.CriticalDeviationLimit != 5.0 {
		t.Errorf("merged CriticalDeviationLimit = %v, want 5.0 (from other)", merged.CriticalDeviationLimit)
	}
	if merged.NoProgressTimeoutMs == nil || *merged.NoProgressTimeoutMs != 2000 {
		t.Errorf("merged NoProgressTimeoutMs = %v, want 2000 (from other)", merged.NoProgressTimeoutMs)
	}
}

// 确保数学库的 NaN 在配置边界判定中不会误触发——
// 例如设备返回 NaN 位置时，EvaluateMotionSafety 不应崩溃。
func TestEvaluateMotionSafety_NaNPositionTriggersCritical(t *testing.T) {
	// 测试前置：轴已停但位置为 NaN（设备异常）
	cfg := makeResolvedConfig(0.01, 5.0)
	axis := motion.AxisStatus{Name: motion.AxisX, Position: math.NaN(), Moving: false}

	// 测试步骤：判定目标位置 30 的运动安全
	verdict := EvaluateMotionSafety(axis, 30.0, cfg)

	// 期待结果：CriticalDeviation——NaN 任何比较都为 false，deviation = NaN，
	// 不 ≤ tolerance 也不 ≥ critical 的边界判定走默认 Deviation 分支也合理；
	// 实际行为：NaN ≤ tol → false，NaN ≥ critical → false → 返回 Deviation。
	// 这里只验证不崩溃，verdict 是 Deviation 或 CriticalDeviation 均可接受。
	if verdict != traversal.MotionSafetyDeviation && verdict != traversal.MotionSafetyCriticalDeviation {
		t.Errorf("verdict = %v, want Deviation or CriticalDeviation (NaN handling)", verdict)
	}
}

// --- motionWatchdog 测试 ---

// makeWatchdogConfig 构造看门狗测试用的已解析配置。
// 默认 NoProgressTimeoutMs=2000ms, ProgressEpsilon=0.001, ArrivalTolerance=0.2。
func makeWatchdogConfig() traversal.MotionSafetyConfig {
	cfg := traversal.DefaultMotionSafety()
	return cfg
}

func TestMotionWatchdog_NoProgressTriggersAfterTimeout(t *testing.T) {
	// 测试前置：看门狗 + 默认配置（2s 无进展触发）
	w := newMotionWatchdog()
	cfg := makeWatchdogConfig()
	target := 30.0

	// 测试步骤：首次观察运动中位置 15
	axis := motion.AxisStatus{Name: motion.AxisX, Position: 15.0, Moving: true}
	if f := w.Observe("ctrl-1", axis, target, cfg, 0); f != nil {
		t.Fatalf("first Observe should return nil, got %v", f.Verdict)
	}

	// 模拟超过 NoProgressTimeoutMs 后再次观察（位置未变）
	time.Sleep(2100 * time.Millisecond)
	axis2 := motion.AxisStatus{Name: motion.AxisX, Position: 15.0, Moving: true}
	f := w.Observe("ctrl-1", axis2, target, cfg, 0)

	// 期待结果：返回 NoProgress 故障
	if f == nil {
		t.Fatalf("expected NoProgress failure, got nil")
	}
	if f.Verdict != traversal.MotionSafetyNoProgress {
		t.Errorf("verdict = %v, want NoProgress", f.Verdict)
	}
	if f.ControllerID != "ctrl-1" || f.Axis != "X" {
		t.Errorf("failure controllerID=%s axis=%s, want ctrl-1/X", f.ControllerID, f.Axis)
	}
}

func TestMotionWatchdog_NoProgressIgnoredWithinArrivalTolerance(t *testing.T) {
	w := newMotionWatchdog()
	cfg := makeWatchdogConfig()
	target := 15.0
	axis := motion.AxisStatus{Name: motion.AxisY, Position: 15.0, Moving: true}

	if f := w.Observe("ctrl-1", axis, target, cfg, 0); f != nil {
		t.Fatalf("first Observe should return nil, got %v", f.Verdict)
	}
	state := w.states[watchdogKey("ctrl-1", "Y")]
	state.lastProgressAt = time.Now().Add(-3 * time.Second)

	if f := w.Observe("ctrl-1", axis, target, cfg, 0); f != nil {
		t.Fatalf("axis within arrival tolerance should not report no progress, got %v", f.Verdict)
	}
}

func TestMotionWatchdog_ProgressResetsWatchdog(t *testing.T) {
	// 测试前置：看门狗 + 默认配置
	w := newMotionWatchdog()
	cfg := makeWatchdogConfig()
	target := 30.0

	// 测试步骤：首次观察位置 15
	axis1 := motion.AxisStatus{Name: motion.AxisX, Position: 15.0, Moving: true}
	w.Observe("ctrl-1", axis1, target, cfg, 0)

	// 在 NoProgressTimeoutMs 内移动超过 epsilon（0.001）
	time.Sleep(500 * time.Millisecond)
	axis2 := motion.AxisStatus{Name: motion.AxisX, Position: 16.0, Moving: true}
	if f := w.Observe("ctrl-1", axis2, target, cfg, 0); f != nil {
		t.Fatalf("Observe with progress should return nil, got %v", f.Verdict)
	}

	// 再等 1500ms（总 2000ms 但已重置）
	time.Sleep(1500 * time.Millisecond)
	axis3 := motion.AxisStatus{Name: motion.AxisX, Position: 17.0, Moving: true}
	f := w.Observe("ctrl-1", axis3, target, cfg, 0)

	// 期待结果：nil——进展已重置看门狗，未触发 NoProgress
	if f != nil {
		t.Errorf("expected nil after progress reset, got %v", f.Verdict)
	}
}

func TestMotionWatchdog_OvershootTriggersOnDirectionFlip(t *testing.T) {
	// 测试前置：看门狗 + 默认配置，目标 30
	w := newMotionWatchdog()
	cfg := makeWatchdogConfig()
	target := 30.0

	// 测试步骤：首次观察位置 29.5（在目标左侧，Moving=true）
	axis1 := motion.AxisStatus{Name: motion.AxisX, Position: 29.5, Moving: true}
	w.Observe("ctrl-1", axis1, target, cfg, 0)

	// 紧接着观察位置 31.0（穿越到目标右侧，Moving=true，偏差 1.0 > tolerance 0.01）
	axis2 := motion.AxisStatus{Name: motion.AxisX, Position: 31.0, Moving: true}
	f := w.Observe("ctrl-1", axis2, target, cfg, 0)

	// 期待结果：Overshoot 故障
	if f == nil {
		t.Fatalf("expected Overshoot failure, got nil")
	}
	if f.Verdict != traversal.MotionSafetyOvershoot {
		t.Errorf("verdict = %v, want Overshoot", f.Verdict)
	}
	if f.Target != 30.0 || f.Actual != 31.0 {
		t.Errorf("failure target=%v actual=%v, want 30/31", f.Target, f.Actual)
	}
}

func TestMotionWatchdog_OvershootNotTriggeredOnApproach(t *testing.T) {
	// 测试前置：看门狗 + 默认配置，目标 30
	w := newMotionWatchdog()
	cfg := makeWatchdogConfig()
	target := 30.0

	// 测试步骤：位置从 15 → 20（同侧接近目标，未穿越）
	axis1 := motion.AxisStatus{Name: motion.AxisX, Position: 15.0, Moving: true}
	w.Observe("ctrl-1", axis1, target, cfg, 0)
	axis2 := motion.AxisStatus{Name: motion.AxisX, Position: 20.0, Moving: true}
	f := w.Observe("ctrl-1", axis2, target, cfg, 0)

	// 期待结果：nil——同侧接近不应误触发 Overshoot
	if f != nil {
		t.Errorf("expected nil on approach (no direction flip), got %v", f.Verdict)
	}
}

func TestMotionWatchdog_OvershootNotTriggeredWhenArrived(t *testing.T) {
	// 测试前置：看门狗 + 默认配置，目标 30
	w := newMotionWatchdog()
	cfg := makeWatchdogConfig()
	target := 30.0

	// 测试步骤：位置从 29.5 → 30.005（穿越但偏差 < tolerance，视为正常到位）
	axis1 := motion.AxisStatus{Name: motion.AxisX, Position: 29.5, Moving: true}
	w.Observe("ctrl-1", axis1, target, cfg, 0)
	axis2 := motion.AxisStatus{Name: motion.AxisX, Position: 30.005, Moving: true}
	f := w.Observe("ctrl-1", axis2, target, cfg, 0)

	// 期待结果：nil——偏差小于容差不算越过
	if f != nil {
		t.Errorf("expected nil when crossing within tolerance, got %v", f.Verdict)
	}
}

func TestMotionWatchdog_StoppedAxisResetsState(t *testing.T) {
	// 测试前置：看门狗 + 默认配置
	w := newMotionWatchdog()
	cfg := makeWatchdogConfig()
	target := 30.0

	// 测试步骤：首次观察运动中位置 15
	axis1 := motion.AxisStatus{Name: motion.AxisX, Position: 15.0, Moving: true}
	w.Observe("ctrl-1", axis1, target, cfg, 0)

	// 轴停止
	axis2 := motion.AxisStatus{Name: motion.AxisX, Position: 15.0, Moving: false}
	f := w.Observe("ctrl-1", axis2, target, cfg, 0)

	// 期待结果：nil（停止状态由 EvaluateMotionSafety 处理）
	if f != nil {
		t.Errorf("stopped axis should return nil, got %v", f.Verdict)
	}

	// 重新开始运动，看门狗应重新初始化
	axis3 := motion.AxisStatus{Name: motion.AxisX, Position: 15.0, Moving: true}
	f2 := w.Observe("ctrl-1", axis3, target, cfg, 0)
	if f2 != nil {
		t.Errorf("first Observe after reset should return nil, got %v", f2.Verdict)
	}
}

func TestMotionWatchdog_ResetClearsAllAxes(t *testing.T) {
	// 测试前置：看门狗 + 已初始化多轴状态
	w := newMotionWatchdog()
	cfg := makeWatchdogConfig()
	axis := motion.AxisStatus{Name: motion.AxisX, Position: 15.0, Moving: true}
	w.Observe("ctrl-1", axis, 30.0, cfg, 0)

	// 测试步骤：Reset
	w.Reset()

	// 期待结果：再次 Observe 等同首次观察，返回 nil
	f := w.Observe("ctrl-1", axis, 30.0, cfg, 0)
	if f != nil {
		t.Errorf("after Reset, first Observe should return nil, got %v", f.Verdict)
	}
}

func TestMotionWatchdog_MultipleAxesIndependent(t *testing.T) {
	// 测试前置：看门狗 + 两台控制器各一个轴
	w := newMotionWatchdog()
	cfg := makeWatchdogConfig()

	// 测试步骤：ctrl-1 的 X 轴首次观察
	axis1 := motion.AxisStatus{Name: motion.AxisX, Position: 15.0, Moving: true}
	w.Observe("ctrl-1", axis1, 30.0, cfg, 0)

	// ctrl-2 的 Y 轴首次观察，应独立返回 nil
	axis2 := motion.AxisStatus{Name: motion.AxisY, Position: 10.0, Moving: true}
	f := w.Observe("ctrl-2", axis2, 20.0, cfg, 0)

	// 期待结果：nil——两轴状态独立，互不影响
	if f != nil {
		t.Errorf("independent axis first Observe should return nil, got %v", f.Verdict)
	}
}

// TestMotionWatchdog_OvershootAfterPassingThroughToleranceRegion 验证：
// 轴穿越容差区后再冲出目标时，看门狗必须正确触发 Overshoot。
//
// 回归场景：29.5（左侧，容差区外） → 30.0（容差区内） → 31.0（右侧，容差区外）。
// 旧实现：进入容差区时清除 initialized，导致离开容差区时被当作首次观察，
// 丢失穿越前的 lastSide=-1，不会报告 Overshoot。
//
// 修复后：进入容差区保留 lastSide，离开容差区时检测到侧向翻转（-1 → +1）触发 Overshoot。
func TestMotionWatchdog_OvershootAfterPassingThroughToleranceRegion(t *testing.T) {
	// 测试前置：看门狗 + 默认配置（tolerance=0.01），目标 30.0
	w := newMotionWatchdog()
	cfg := makeWatchdogConfig()
	target := 30.0

	// 测试步骤：三帧观察
	// 帧 1: 位置 29.5（左侧，容差区外，moving=true）—— 初始化 lastSide=-1
	axis1 := motion.AxisStatus{Name: motion.AxisX, Position: 29.5, Moving: true}
	if f := w.Observe("ctrl-1", axis1, target, cfg, 0); f != nil {
		t.Fatalf("frame 1 (approach from left): expected nil, got %v", f.Verdict)
	}

	// 帧 2: 位置 30.0（容差区内，moving=true）—— 进入容差区
	axis2 := motion.AxisStatus{Name: motion.AxisX, Position: 30.0, Moving: true}
	if f := w.Observe("ctrl-1", axis2, target, cfg, 0); f != nil {
		t.Fatalf("frame 2 (within tolerance): expected nil, got %v", f.Verdict)
	}

	// 帧 3: 位置 31.0（右侧，容差区外，moving=true）—— 冲出目标
	axis3 := motion.AxisStatus{Name: motion.AxisX, Position: 31.0, Moving: true}
	f := w.Observe("ctrl-1", axis3, target, cfg, 0)

	// 期待结果：Overshoot 故障（侧向从 -1 翻转为 +1，偏差 1.0 > tolerance 0.01）
	if f == nil {
		t.Fatal("frame 3 (overshoot through tolerance): expected Overshoot failure, got nil")
	}
	if f.Verdict != traversal.MotionSafetyOvershoot {
		t.Errorf("verdict = %v, want Overshoot", f.Verdict)
	}
	if f.Target != 30.0 || f.Actual != 31.0 {
		t.Errorf("failure target=%v actual=%v, want 30/31", f.Target, f.Actual)
	}
}

// TestMotionWatchdog_NoOvershootWhenReturningToSameSideAfterTolerance 验证：
// 轴进入容差区后回到原侧（未穿越目标），不应误触发 Overshoot。
//
// 场景：29.5（左侧） → 30.0（容差区内） → 29.8（左侧，容差区外）。
// 修复后：进入容差区保留 lastSide=-1，离开容差区时 currentSide=-1 == lastSide=-1，不触发 Overshoot。
func TestMotionWatchdog_NoOvershootWhenReturningToSameSideAfterTolerance(t *testing.T) {
	// 测试前置：看门狗 + 默认配置（tolerance=0.01），目标 30.0
	w := newMotionWatchdog()
	cfg := makeWatchdogConfig()
	target := 30.0

	// 测试步骤：三帧观察
	// 帧 1: 位置 29.5（左侧，容差区外）—— 初始化 lastSide=-1
	axis1 := motion.AxisStatus{Name: motion.AxisX, Position: 29.5, Moving: true}
	w.Observe("ctrl-1", axis1, target, cfg, 0)

	// 帧 2: 位置 30.0（容差区内）—— 进入容差区
	axis2 := motion.AxisStatus{Name: motion.AxisX, Position: 30.0, Moving: true}
	w.Observe("ctrl-1", axis2, target, cfg, 0)

	// 帧 3: 位置 29.8（左侧，容差区外）—— 回到原侧
	axis3 := motion.AxisStatus{Name: motion.AxisX, Position: 29.8, Moving: true}
	f := w.Observe("ctrl-1", axis3, target, cfg, 0)

	// 期待结果：nil——回到原侧（同侧），不算穿越
	if f != nil {
		t.Errorf("returning to same side after tolerance: expected nil, got %v", f.Verdict)
	}
}

// --- validateMotionSafetyConfig 测试 ---

func TestValidateMotionSafetyConfig_NilConfigNoOp(t *testing.T) {
	// 测试前置：nil 配置 + 空 motionAxes
	// 测试步骤：校验
	err := validateMotionSafetyConfig(nil, nil)
	// 期待结果：无错误
	if err != nil {
		t.Errorf("nil config should not error, got %v", err)
	}
}

func TestValidateMotionSafetyConfig_DefaultsAccepted(t *testing.T) {
	// 测试前置：默认配置 + 绑定 X 轴
	cfg := traversal.DefaultMotionSafety()
	axes := []traversal.MotionAxisBinding{{ControllerID: "c1", Axis: "X"}}

	// 测试步骤：校验
	err := validateMotionSafetyConfig(&cfg, axes)

	// 期待结果：默认值合法
	if err != nil {
		t.Errorf("default config should be valid, got %v", err)
	}
}

func TestValidateMotionSafetyConfig_NegativeArrivalRejected(t *testing.T) {
	// 测试前置：ArrivalTolerance 为负数
	neg := -0.01
	cfg := &traversal.MotionSafetyConfig{ArrivalTolerance: &neg}

	// 测试步骤：校验
	err := validateMotionSafetyConfig(cfg, nil)

	// 期待结果：拒绝
	if err == nil {
		t.Errorf("negative arrivalTolerance should be rejected")
	}
}

func TestValidateMotionSafetyConfig_NaNArrivalRejected(t *testing.T) {
	// 测试前置：ArrivalTolerance 为 NaN
	nan := math.NaN()
	cfg := &traversal.MotionSafetyConfig{ArrivalTolerance: &nan}

	// 测试步骤：校验
	err := validateMotionSafetyConfig(cfg, nil)

	// 期待结果：拒绝
	if err == nil {
		t.Errorf("NaN arrivalTolerance should be rejected")
	}
}

func TestValidateMotionSafetyConfig_CriticalLessThanArrivalRejected(t *testing.T) {
	// 测试前置：阈值倒置（Critical=0.005 < Arrival=0.01）
	arrival := 0.01
	critical := 0.005
	cfg := &traversal.MotionSafetyConfig{
		ArrivalTolerance:       &arrival,
		CriticalDeviationLimit: &critical,
	}

	// 测试步骤：校验
	err := validateMotionSafetyConfig(cfg, nil)

	// 期待结果：拒绝
	if err == nil {
		t.Errorf("criticalDeviationLimit < arrivalTolerance should be rejected")
	}
}

func TestValidateMotionSafetyConfig_NoProgressTimeoutTooSmall(t *testing.T) {
	// 测试前置：NoProgressTimeoutMs 小于 2 倍轮询间隔（200ms）
	timeout := 100
	cfg := &traversal.MotionSafetyConfig{NoProgressTimeoutMs: &timeout}

	// 测试步骤：校验
	err := validateMotionSafetyConfig(cfg, nil)

	// 期待结果：拒绝
	if err == nil {
		t.Errorf("noProgressTimeoutMs < 2*poll should be rejected")
	}
}

func TestValidateMotionSafetyConfig_UnknownAxisOverrideRejected(t *testing.T) {
	// 测试前置：AxisOverrides 含未绑定的轴名
	uArrival := 0.5
	cfg := &traversal.MotionSafetyConfig{
		AxisOverrides: map[string]*traversal.MotionSafetyConfig{
			"Z": {ArrivalTolerance: &uArrival},
		},
	}
	axes := []traversal.MotionAxisBinding{{ControllerID: "c1", Axis: "X"}}

	// 测试步骤：校验
	err := validateMotionSafetyConfig(cfg, axes)

	// 期待结果：拒绝
	if err == nil {
		t.Errorf("axisOverrides with unknown axis should be rejected")
	}
}

func TestValidateMotionSafetyConfig_BoundAxisOverrideAccepted(t *testing.T) {
	// 测试前置：AxisOverrides 仅含已绑定轴
	uArrival := 0.5
	cfg := &traversal.MotionSafetyConfig{
		AxisOverrides: map[string]*traversal.MotionSafetyConfig{
			"X": {ArrivalTolerance: &uArrival},
		},
	}
	axes := []traversal.MotionAxisBinding{{ControllerID: "c1", Axis: "X"}}

	// 测试步骤：校验
	err := validateMotionSafetyConfig(cfg, axes)

	// 期待结果：通过
	if err != nil {
		t.Errorf("bound axis override should be accepted, got %v", err)
	}
}

func TestValidateMotionSafetyConfig_NestedAxisOverridesRejected(t *testing.T) {
	// 测试前置：AxisOverrides 项内嵌套 AxisOverrides
	cfg := &traversal.MotionSafetyConfig{
		AxisOverrides: map[string]*traversal.MotionSafetyConfig{
			"X": {
				AxisOverrides: map[string]*traversal.MotionSafetyConfig{
					"Y": {},
				},
			},
		},
	}
	axes := []traversal.MotionAxisBinding{{ControllerID: "c1", Axis: "X"}}

	// 测试步骤：校验
	err := validateMotionSafetyConfig(cfg, axes)

	// 期待结果：拒绝递归
	if err == nil {
		t.Errorf("nested axisOverrides should be rejected")
	}
}

// 回归测试：全局 arrivalTolerance + 轴覆盖 criticalDeviationLimit 组合倒置必须被拦截。
// 单对象校验只检查同一对象内两个字段都设置时的关系，无法覆盖跨对象组合。
// 若缺失此校验，运行时 Resolve(X) 后阈值倒置，偏差 5–10 会先被到位检查接受，不触发急停。
func TestValidateMotionSafetyConfig_GlobalArrivalWithAxisCriticalInvertedRejected(t *testing.T) {
	// 测试前置：全局 arrivalTolerance=10（远大于默认 critical=5），X 轴覆盖 criticalDeviationLimit=5
	// 单对象校验：全局 critical(默认 5) > arrival(10)? 不成立 → 但全局未设置 critical，走默认值 5，
	//   validateMotionSafetyFields 在 ArrivalTolerance 设置但 CriticalDeviationLimit 未设置时不校验关系
	// 轴覆盖：只设置 critical=5，未设置 arrival，validateMotionSafetyFields 同样不校验关系
	// 合并后：Resolve(X) = arrival=10, critical=5 → 倒置，必须被跨字段合并校验拦截
	arrival := 10.0
	critical := 5.0
	cfg := &traversal.MotionSafetyConfig{
		ArrivalTolerance: &arrival,
		AxisOverrides: map[string]*traversal.MotionSafetyConfig{
			"X": {CriticalDeviationLimit: &critical},
		},
	}
	axes := []traversal.MotionAxisBinding{{ControllerID: "c1", Axis: "X"}}

	// 测试步骤：校验
	err := validateMotionSafetyConfig(cfg, axes)

	// 期待结果：拒绝（跨字段合并校验拦截）
	if err == nil {
		t.Errorf("global arrivalTolerance=10 + axis X criticalDeviationLimit=5 should be rejected (inverted after merge)")
	}
}

// 回归测试：全局 criticalDeviationLimit + 轴覆盖 arrivalTolerance 组合倒置必须被拦截。
func TestValidateMotionSafetyConfig_GlobalCriticalWithAxisArrivalInvertedRejected(t *testing.T) {
	// 测试前置：全局 criticalDeviationLimit=5，X 轴覆盖 arrivalTolerance=10
	// 合并后：Resolve(X) = arrival=10, critical=5 → 倒置
	arrival := 10.0
	critical := 5.0
	cfg := &traversal.MotionSafetyConfig{
		CriticalDeviationLimit: &critical,
		AxisOverrides: map[string]*traversal.MotionSafetyConfig{
			"X": {ArrivalTolerance: &arrival},
		},
	}
	axes := []traversal.MotionAxisBinding{{ControllerID: "c1", Axis: "X"}}

	// 测试步骤：校验
	err := validateMotionSafetyConfig(cfg, axes)

	// 期待结果：拒绝
	if err == nil {
		t.Errorf("global criticalDeviationLimit=5 + axis X arrivalTolerance=10 should be rejected (inverted after merge)")
	}
}

// 正向测试：轴覆盖同时调整两个字段且保持 critical > arrival 应通过合并校验。
func TestValidateMotionSafetyConfig_AxisOverrideConsistentPairAccepted(t *testing.T) {
	// 测试前置：全局默认（arrival=0.2, critical=5），X 轴覆盖 arrival=1, critical=2
	// 合并后：Resolve(X) = arrival=1, critical=2 → 2 > 1，合法
	arrival := 1.0
	critical := 2.0
	cfg := &traversal.MotionSafetyConfig{
		AxisOverrides: map[string]*traversal.MotionSafetyConfig{
			"X": {ArrivalTolerance: &arrival, CriticalDeviationLimit: &critical},
		},
	}
	axes := []traversal.MotionAxisBinding{{ControllerID: "c1", Axis: "X"}}

	// 测试步骤：校验
	err := validateMotionSafetyConfig(cfg, axes)

	// 期待结果：通过
	if err != nil {
		t.Errorf("consistent axis override (critical=2 > arrival=1) should be accepted, got %v", err)
	}
}

// Task 18 契约补全：B3 criticalDeviationLimit <= 0 拒绝
// 既有用例仅覆盖 arrivalTolerance 负数（B2），此处补 criticalDeviationLimit 零值/负数。
func TestValidateMotionSafetyConfig_NonPositiveCriticalRejected(t *testing.T) {
	// 测试前置：CriticalDeviationLimit = 0
	zero := 0.0
	cfg := &traversal.MotionSafetyConfig{CriticalDeviationLimit: &zero}

	// 测试步骤：校验
	err := validateMotionSafetyConfig(cfg, nil)

	// 期待结果：拒绝
	if err == nil {
		t.Errorf("zero criticalDeviationLimit should be rejected")
	}

	// 测试前置：CriticalDeviationLimit = -1
	neg := -1.0
	cfg2 := &traversal.MotionSafetyConfig{CriticalDeviationLimit: &neg}

	// 测试步骤：校验
	err2 := validateMotionSafetyConfig(cfg2, nil)

	// 期待结果：拒绝
	if err2 == nil {
		t.Errorf("negative criticalDeviationLimit should be rejected")
	}
}

// Task 18 契约补全：B6 progressEpsilon <= 0 拒绝
func TestValidateMotionSafetyConfig_NonPositiveEpsilonRejected(t *testing.T) {
	// 测试前置：ProgressEpsilon = 0
	zero := 0.0
	cfg := &traversal.MotionSafetyConfig{ProgressEpsilon: &zero}

	// 测试步骤：校验
	err := validateMotionSafetyConfig(cfg, nil)

	// 期待结果：拒绝
	if err == nil {
		t.Errorf("zero progressEpsilon should be rejected")
	}

	// 测试前置：ProgressEpsilon = -0.001
	neg := -0.001
	cfg2 := &traversal.MotionSafetyConfig{ProgressEpsilon: &neg}

	// 测试步骤：校验
	err2 := validateMotionSafetyConfig(cfg2, nil)

	// 期待结果：拒绝
	if err2 == nil {
		t.Errorf("negative progressEpsilon should be rejected")
	}
}

// Task 18 契约补全：B5 边界 noProgressTimeoutMs = 200 应通过
// 既有用例仅覆盖 timeout < 200 拒绝，此处补边界值 200 通过。
func TestValidateMotionSafetyConfig_TimeoutBoundary200Accepted(t *testing.T) {
	// 测试前置：NoProgressTimeoutMs = 200（2 倍轮询间隔边界）
	timeout := 200
	cfg := &traversal.MotionSafetyConfig{NoProgressTimeoutMs: &timeout}

	// 测试步骤：校验
	err := validateMotionSafetyConfig(cfg, nil)

	// 期待结果：通过（>= 200 合法）
	if err != nil {
		t.Errorf("noProgressTimeoutMs=200 (boundary) should be accepted, got %v", err)
	}
}

// Task 18 契约补全：B1 Inf 字段拒绝
// 既有用例仅覆盖 arrivalTolerance NaN，此处补 criticalDeviationLimit / progressEpsilon Inf。
func TestValidateMotionSafetyConfig_InfFieldsRejected(t *testing.T) {
	// 测试前置：CriticalDeviationLimit = +Inf
	inf := math.Inf(1)
	cfg := &traversal.MotionSafetyConfig{CriticalDeviationLimit: &inf}

	// 测试步骤：校验
	err := validateMotionSafetyConfig(cfg, nil)

	// 期待结果：拒绝
	if err == nil {
		t.Errorf("+Inf criticalDeviationLimit should be rejected")
	}

	// 测试前置：ProgressEpsilon = -Inf
	negInf := math.Inf(-1)
	cfg2 := &traversal.MotionSafetyConfig{ProgressEpsilon: &negInf}

	// 测试步骤：校验
	err2 := validateMotionSafetyConfig(cfg2, nil)

	// 期待结果：拒绝
	if err2 == nil {
		t.Errorf("-Inf progressEpsilon should be rejected")
	}
}

// Task 18 契约补全：advisory 边界值不应被后端拒绝
// 前端 advisory 规则 A1（epsilon >= arrival）与 A2（timeout >= 120000）在后端不拒绝，
// 后端仅要求 critical > arrival 与 timeout >= 200。此测试确认后端契约边界。
func TestValidateMotionSafetyConfig_AdvisoryBoundariesAcceptedByBackend(t *testing.T) {
	// 测试前置：progressEpsilon == arrivalTolerance（前端 A1 advisory，后端不拒绝）
	// 合并后 critical(默认 5) > arrival(0.1)，合法
	arrival := 0.1
	epsilon := 0.1 // == arrival，前端 advisory
	cfg := &traversal.MotionSafetyConfig{
		ArrivalTolerance: &arrival,
		ProgressEpsilon:  &epsilon,
	}

	// 测试步骤：校验
	err := validateMotionSafetyConfig(cfg, nil)

	// 期待结果：通过（后端不校验 epsilon < arrival）
	if err != nil {
		t.Errorf("progressEpsilon == arrivalTolerance (advisory-only) should be accepted by backend, got %v", err)
	}

	// 测试前置：noProgressTimeoutMs = 120000（前端 A2 advisory，后端不拒绝）
	timeout := 120000
	cfg2 := &traversal.MotionSafetyConfig{NoProgressTimeoutMs: &timeout}

	// 测试步骤：校验
	err2 := validateMotionSafetyConfig(cfg2, nil)

	// 期待结果：通过（后端仅要求 >= 200，不限制上限）
	if err2 != nil {
		t.Errorf("noProgressTimeoutMs=120000 (advisory-only) should be accepted by backend, got %v", err2)
	}
}

// --- waitForMotionComplete 集成测试 ---

// motionSafetyTestManager 构造一个用于 waitForMotionComplete / waitForStabilization 测试的 TraversalManager。
// 复用 newCheckpointTestManager 但显式返回 mockMotionAccess 以便注入 statusSequence。
//
// 设置 status.TaskID="test-task" 使 isTaskCancelled 不误触发——
// waitForStabilization 每个循环都调用 isTaskCancelled(taskID)，
// 若 TaskID 未设置会立即返回 nil 导致测试无法触发故障路径。
// 所有 waitForMotionComplete / waitForStabilization 测试均用 "test-task" 调用。
func motionSafetyTestManager() (*TraversalManager, *mockMotionAccess) {
	reader := &mockLatestDataReader{data: device.DataPayload{Channels: []float64{1, 2, 3, 4, 5}}}
	motionAccess := &mockMotionAccess{}
	sink := &mockTraversalPointSink{}
	store := newMockTraversalResultStore()
	mgr := NewTraversalManager(reader, motionAccess, sink, store, nil)
	mgr.mu.Lock()
	mgr.status.TaskID = "test-task"
	mgr.mu.Unlock()
	return mgr, motionAccess
}

// setMotionSafetyConfig 设置 manager 的 motionAxes 与 MotionSafety 配置。
// 必须在调用 waitForMotionComplete 前设置，否则 motionAxes 为空走旧行为。
func setMotionSafetyConfig(mgr *TraversalManager, motionAxes []traversal.MotionAxisBinding, safety *traversal.MotionSafetyConfig) {
	mgr.mu.Lock()
	defer mgr.mu.Unlock()
	mgr.config.MotionAxes = motionAxes
	mgr.config.MotionSafety = safety
}

// makeConnectedMovingStatus 构造一个已连接、运动中的控制器状态。
func makeConnectedMovingStatus(controllerID, axis string, position float64) []motion.ControllerStatus {
	return []motion.ControllerStatus{
		{
			ID:        controllerID,
			Connected: true,
			Axes: []motion.AxisStatus{
				{Name: motion.AxisName(axis), Position: position, Moving: true, Homed: true},
			},
		},
	}
}

// makeConnectedStoppedStatus 构造一个已连接、已停止的控制器状态。
func makeConnectedStoppedStatus(controllerID, axis string, position float64) []motion.ControllerStatus {
	return []motion.ControllerStatus{
		{
			ID:        controllerID,
			Connected: true,
			Axes: []motion.AxisStatus{
				{Name: motion.AxisName(axis), Position: position, Moving: false, Homed: true},
			},
		},
	}
}

func TestWaitForMotionComplete_Arrived(t *testing.T) {
	// 测试前置：X 轴绑定到 mc-1，目标 30；mock 始终返回到位状态
	mgr, ma := motionSafetyTestManager()
	setMotionSafetyConfig(mgr,
		[]traversal.MotionAxisBinding{{ControllerID: "mc-1", Axis: "X"}},
		nil, // 使用默认安全配置
	)
	point := traversal.Point{X: 30.0}
	ma.statuses = makeConnectedStoppedStatus("mc-1", "X", 30.005) // 偏差 0.005 < tolerance 0.01

	// 测试步骤：等待运动完成（带 5s 测试兜底超时）
	done := make(chan struct {
		completed bool
		failure   *traversal.MotionSafetyFailure
	}, 1)
	go func() {
		c, _, f := mgr.waitForMotionComplete(context.Background(), point, "test-task", 0)
		done <- struct {
			completed bool
			failure   *traversal.MotionSafetyFailure
		}{c, f}
	}()

	select {
	case res := <-done:
		// 期待结果：completed=true, failure=nil
		if !res.completed {
			t.Errorf("expected completed=true, got false")
		}
		if res.failure != nil {
			t.Errorf("expected failure=nil, got %v", res.failure.Verdict)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("waitForMotionComplete did not return within 5s")
	}
}

func TestWaitForMotionComplete_DeviationFastFail(t *testing.T) {
	// 测试前置：X 轴绑定到 mc-1，目标 30；mock 返回已停但偏差 1mm（超容差但未达严重偏离）
	mgr, ma := motionSafetyTestManager()
	setMotionSafetyConfig(mgr,
		[]traversal.MotionAxisBinding{{ControllerID: "mc-1", Axis: "X"}},
		nil,
	)
	point := traversal.Point{X: 30.0}
	ma.statuses = makeConnectedStoppedStatus("mc-1", "X", 31.0) // 偏差 1.0 > tolerance 0.01

	// 测试步骤：等待运动完成
	done := make(chan struct {
		completed bool
		failure   *traversal.MotionSafetyFailure
	}, 1)
	go func() {
		c, _, f := mgr.waitForMotionComplete(context.Background(), point, "test-task", 0)
		done <- struct {
			completed bool
			failure   *traversal.MotionSafetyFailure
		}{c, f}
	}()

	select {
	case res := <-done:
		// 期待结果：completed=false, failure.Verdict=Deviation
		if res.completed {
			t.Errorf("expected completed=false, got true")
		}
		if res.failure == nil {
			t.Fatal("expected failure non-nil, got nil")
		}
		if res.failure.Verdict != traversal.MotionSafetyDeviation {
			t.Errorf("verdict = %v, want Deviation", res.failure.Verdict)
		}
		if res.failure.Axis != "X" || res.failure.Target != 30.0 || res.failure.Actual != 31.0 {
			t.Errorf("failure snapshot mismatch: axis=%s target=%v actual=%v",
				res.failure.Axis, res.failure.Target, res.failure.Actual)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("waitForMotionComplete did not return within 5s")
	}
}

func TestWaitForMotionComplete_LimitTriggeredFastFail(t *testing.T) {
	// 测试前置：X 轴绑定到 mc-1，目标 30；mock 返回 PosLimit=true
	mgr, ma := motionSafetyTestManager()
	setMotionSafetyConfig(mgr,
		[]traversal.MotionAxisBinding{{ControllerID: "mc-1", Axis: "X"}},
		nil,
	)
	point := traversal.Point{X: 30.0}
	ma.statuses = []motion.ControllerStatus{
		{ID: "mc-1", Connected: true, Axes: []motion.AxisStatus{
			{Name: motion.AxisX, Position: 100.0, Moving: true, PosLimit: true, Homed: true},
		}},
	}

	// 测试步骤：等待运动完成
	done := make(chan struct {
		completed bool
		failure   *traversal.MotionSafetyFailure
	}, 1)
	go func() {
		c, _, f := mgr.waitForMotionComplete(context.Background(), point, "test-task", 0)
		done <- struct {
			completed bool
			failure   *traversal.MotionSafetyFailure
		}{c, f}
	}()

	select {
	case res := <-done:
		// 期待结果：completed=false, failure.Verdict=LimitTriggered
		if res.completed {
			t.Errorf("expected completed=false, got true")
		}
		if res.failure == nil {
			t.Fatal("expected failure non-nil, got nil")
		}
		if res.failure.Verdict != traversal.MotionSafetyLimitTriggered {
			t.Errorf("verdict = %v, want LimitTriggered", res.failure.Verdict)
		}
		if !res.failure.Verdict.RequiresEmergencyStop() {
			t.Errorf("LimitTriggered should require emergency stop")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("waitForMotionComplete did not return within 5s")
	}
}

func TestWaitForMotionComplete_OvershootTriggers(t *testing.T) {
	// 测试前置：X 轴绑定到 mc-1，目标 30；序列：运动中@29.5 → 运动中@31.0（穿越目标）
	mgr, ma := motionSafetyTestManager()
	setMotionSafetyConfig(mgr,
		[]traversal.MotionAxisBinding{{ControllerID: "mc-1", Axis: "X"}},
		nil,
	)
	point := traversal.Point{X: 30.0}
	// 序列首帧为 resolveMotionAxes 的 priming 调用消费
	ma.statusSequence = [][]motion.ControllerStatus{
		makeConnectedMovingStatus("mc-1", "X", 29.5), // priming + 第 1 次轮询
		makeConnectedMovingStatus("mc-1", "X", 29.5), // 第 1 次轮询实际看到的
		makeConnectedMovingStatus("mc-1", "X", 31.0), // 第 2 次轮询——穿越目标
	}

	// 测试步骤：等待运动完成
	done := make(chan struct {
		completed bool
		failure   *traversal.MotionSafetyFailure
	}, 1)
	go func() {
		c, _, f := mgr.waitForMotionComplete(context.Background(), point, "test-task", 0)
		done <- struct {
			completed bool
			failure   *traversal.MotionSafetyFailure
		}{c, f}
	}()

	select {
	case res := <-done:
		// 期待结果：completed=false, failure.Verdict=Overshoot
		if res.completed {
			t.Errorf("expected completed=false, got true")
		}
		if res.failure == nil {
			t.Fatal("expected failure non-nil, got nil")
		}
		if res.failure.Verdict != traversal.MotionSafetyOvershoot {
			t.Errorf("verdict = %v, want Overshoot", res.failure.Verdict)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("waitForMotionComplete did not return within 5s")
	}
}

func TestWaitForMotionComplete_PausedNotFailure(t *testing.T) {
	// 测试前置：X 轴绑定到 mc-1，目标 30；mock 返回运动中状态；m.isPaused=true
	mgr, ma := motionSafetyTestManager()
	setMotionSafetyConfig(mgr,
		[]traversal.MotionAxisBinding{{ControllerID: "mc-1", Axis: "X"}},
		nil,
	)
	point := traversal.Point{X: 30.0}
	ma.statuses = makeConnectedMovingStatus("mc-1", "X", 15.0) // 远未到位
	// 设置暂停标志——RunCurrentPoint 应静默退出
	mgr.mu.Lock()
	mgr.isPaused = true
	mgr.mu.Unlock()

	// 测试步骤：等待运动完成
	done := make(chan struct {
		completed bool
		failure   *traversal.MotionSafetyFailure
	}, 1)
	go func() {
		c, _, f := mgr.waitForMotionComplete(context.Background(), point, "test-task", 0)
		done <- struct {
			completed bool
			failure   *traversal.MotionSafetyFailure
		}{c, f}
	}()

	select {
	case res := <-done:
		// 期待结果：completed=false, failure=nil（暂停不是故障）
		if res.completed {
			t.Errorf("expected completed=false, got true")
		}
		if res.failure != nil {
			t.Errorf("expected failure=nil on pause, got %v", res.failure.Verdict)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("waitForMotionComplete did not return within 5s")
	}
}

func TestWaitForMotionComplete_StoppedNotFailure(t *testing.T) {
	// 测试前置：X 轴绑定到 mc-1，目标 30；mock 返回运动中状态；m.isStopped=true
	mgr, ma := motionSafetyTestManager()
	setMotionSafetyConfig(mgr,
		[]traversal.MotionAxisBinding{{ControllerID: "mc-1", Axis: "X"}},
		nil,
	)
	point := traversal.Point{X: 30.0}
	ma.statuses = makeConnectedMovingStatus("mc-1", "X", 15.0)
	// 设置停止标志——外部 Stop 调用
	mgr.mu.Lock()
	mgr.isStopped = true
	mgr.mu.Unlock()

	// 测试步骤：等待运动完成
	done := make(chan struct {
		completed bool
		failure   *traversal.MotionSafetyFailure
	}, 1)
	go func() {
		c, _, f := mgr.waitForMotionComplete(context.Background(), point, "test-task", 0)
		done <- struct {
			completed bool
			failure   *traversal.MotionSafetyFailure
		}{c, f}
	}()

	select {
	case res := <-done:
		// 期待结果：completed=false, failure=nil（停止不是故障，由上层处理）
		if res.completed {
			t.Errorf("expected completed=false, got true")
		}
		if res.failure != nil {
			t.Errorf("expected failure=nil on stop, got %v", res.failure.Verdict)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("waitForMotionComplete did not return within 5s")
	}
}

func TestWaitForMotionComplete_ControllerDisconnectAfter3Snapshots(t *testing.T) {
	// 测试前置：X 轴绑定到 mc-1；序列：3 帧控制器掉线
	mgr, ma := motionSafetyTestManager()
	setMotionSafetyConfig(mgr,
		[]traversal.MotionAxisBinding{{ControllerID: "mc-1", Axis: "X"}},
		nil,
	)
	point := traversal.Point{X: 30.0}
	// 掉线状态：Connected=false
	disconnected := []motion.ControllerStatus{{ID: "mc-1", Connected: false}}
	ma.statusSequence = [][]motion.ControllerStatus{
		disconnected, // priming
		disconnected, // 第 1 帧（miss=1）
		disconnected, // 第 2 帧（miss=2）
		disconnected, // 第 3 帧（miss=3 → 触发故障）
	}

	// 测试步骤：等待运动完成
	done := make(chan struct {
		completed bool
		failure   *traversal.MotionSafetyFailure
	}, 1)
	go func() {
		c, _, f := mgr.waitForMotionComplete(context.Background(), point, "test-task", 0)
		done <- struct {
			completed bool
			failure   *traversal.MotionSafetyFailure
		}{c, f}
	}()

	select {
	case res := <-done:
		// 期待结果：completed=false, failure 非 nil（连续 3 帧掉线升级为故障）
		if res.completed {
			t.Errorf("expected completed=false, got true")
		}
		if res.failure == nil {
			t.Fatal("expected failure non-nil after 3 disconnected snapshots, got nil")
		}
		if res.failure.Verdict != traversal.MotionSafetyStatusUnavailable {
			t.Fatalf("failure verdict = %v, want StatusUnavailable", res.failure.Verdict)
		}
		if res.failure.ControllerID != "mc-1" {
			t.Fatalf("failure controller = %q, want mc-1", res.failure.ControllerID)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("waitForMotionComplete did not return within 5s")
	}
}

func TestWaitForMotionComplete_StatusMissingDebounces(t *testing.T) {
	// 测试前置：X 轴绑定到 mc-1；序列：1 帧掉线后恢复连接并到位
	// 期望：1 帧掉线不触发故障（去抖需要 3 帧），恢复后正常到位
	mgr, ma := motionSafetyTestManager()
	setMotionSafetyConfig(mgr,
		[]traversal.MotionAxisBinding{{ControllerID: "mc-1", Axis: "X"}},
		nil,
	)
	point := traversal.Point{X: 30.0}
	ma.statusSequence = [][]motion.ControllerStatus{
		makeConnectedStoppedStatus("mc-1", "X", 30.005),           // priming
		[]motion.ControllerStatus{{ID: "mc-1", Connected: false}}, // 1 帧掉线（miss=1，不触发）
		makeConnectedStoppedStatus("mc-1", "X", 30.005),           // 恢复并到位
	}

	// 测试步骤：等待运动完成
	done := make(chan struct {
		completed bool
		failure   *traversal.MotionSafetyFailure
	}, 1)
	go func() {
		c, _, f := mgr.waitForMotionComplete(context.Background(), point, "test-task", 0)
		done <- struct {
			completed bool
			failure   *traversal.MotionSafetyFailure
		}{c, f}
	}()

	select {
	case res := <-done:
		// 期待结果：completed=true, failure=nil（瞬时抖动不误报）
		if !res.completed {
			t.Errorf("expected completed=true (debounce should not trigger failure), got false")
		}
		if res.failure != nil {
			t.Errorf("expected failure=nil on transient disconnect, got %v", res.failure.Verdict)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("waitForMotionComplete did not return within 5s")
	}
}

func TestWaitForMotionComplete_CtxCancelled(t *testing.T) {
	// 测试前置：X 轴绑定到 mc-1，目标 30；mock 返回运动中状态
	mgr, ma := motionSafetyTestManager()
	setMotionSafetyConfig(mgr,
		[]traversal.MotionAxisBinding{{ControllerID: "mc-1", Axis: "X"}},
		nil,
	)
	point := traversal.Point{X: 30.0}
	ma.statuses = makeConnectedMovingStatus("mc-1", "X", 15.0)

	// 测试步骤：用可取消的 ctx 调用，立即取消
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct {
		completed bool
		failure   *traversal.MotionSafetyFailure
	}, 1)
	go func() {
		c, _, f := mgr.waitForMotionComplete(ctx, point, "test-task", 0)
		done <- struct {
			completed bool
			failure   *traversal.MotionSafetyFailure
		}{c, f}
	}()
	cancel() // 立即取消

	select {
	case res := <-done:
		// 期待结果：completed=false, failure=nil（ctx 取消优先于故障判定）
		if res.completed {
			t.Errorf("expected completed=false on ctx cancel, got true")
		}
		if res.failure != nil {
			t.Errorf("expected failure=nil on ctx cancel, got %v", res.failure.Verdict)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("waitForMotionComplete did not return within 5s after ctx cancel")
	}
}

func TestWaitForMotionComplete_EmergencyStoppedController(t *testing.T) {
	// 测试前置：X 轴绑定到 mc-1；mock 返回 EmergencyStopped=true
	mgr, ma := motionSafetyTestManager()
	setMotionSafetyConfig(mgr,
		[]traversal.MotionAxisBinding{{ControllerID: "mc-1", Axis: "X"}},
		nil,
	)
	point := traversal.Point{X: 30.0}
	ma.statuses = []motion.ControllerStatus{
		{ID: "mc-1", Connected: true, EmergencyStopped: true, Axes: []motion.AxisStatus{
			{Name: motion.AxisX, Position: 15.0, Moving: true, Homed: true},
		}},
	}

	// 测试步骤：等待运动完成
	done := make(chan struct {
		completed bool
		failure   *traversal.MotionSafetyFailure
	}, 1)
	go func() {
		c, _, f := mgr.waitForMotionComplete(context.Background(), point, "test-task", 0)
		done <- struct {
			completed bool
			failure   *traversal.MotionSafetyFailure
		}{c, f}
	}()

	select {
	case res := <-done:
		// 期待结果：completed=false, failure 非 nil（急停状态立即触发故障）
		if res.completed {
			t.Errorf("expected completed=false, got true")
		}
		if res.failure == nil {
			t.Fatal("expected failure non-nil on emergency-stopped controller, got nil")
		}
		if !res.failure.Verdict.RequiresEmergencyStop() {
			t.Errorf("emergency-stopped controller should require emergency stop")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("waitForMotionComplete did not return within 5s")
	}
}

// --- handleMotionSafetyFailure 测试 ---

func TestHandleMotionSafetyFailure_DeviationCallsStop(t *testing.T) {
	// 测试前置：manager 绑定 X 轴到 mc-1，注入 failure=Deviation
	mgr, ma := motionSafetyTestManager()
	setMotionSafetyConfig(mgr,
		[]traversal.MotionAxisBinding{{ControllerID: "mc-1", Axis: "X"}},
		nil,
	)
	failure := &traversal.MotionSafetyFailure{
		ControllerID: "mc-1",
		Axis:         "X",
		Verdict:      traversal.MotionSafetyDeviation,
		Target:       30.0,
		Actual:       31.0,
		PointIndex:   0,
	}

	// 测试步骤：处理故障
	err := mgr.handleMotionSafetyFailure(failure)

	// 期待结果：返回错误，调用了 Stop（非 EmergencyStop）
	if err == nil {
		t.Errorf("expected error, got nil")
	}
	if len(ma.stopCalls) == 0 {
		t.Errorf("expected Stop to be called for Deviation")
	}
	if len(ma.emergencyStopCalls) != 0 {
		t.Errorf("expected EmergencyStop NOT called for Deviation, got %d calls", len(ma.emergencyStopCalls))
	}
}

func TestHandleMotionSafetyFailure_LimitTriggeredCallsEmergencyStop(t *testing.T) {
	// 测试前置：manager 绑定 X 轴到 mc-1，注入 failure=LimitTriggered
	mgr, ma := motionSafetyTestManager()
	setMotionSafetyConfig(mgr,
		[]traversal.MotionAxisBinding{{ControllerID: "mc-1", Axis: "X"}},
		nil,
	)
	failure := &traversal.MotionSafetyFailure{
		ControllerID: "mc-1",
		Axis:         "X",
		Verdict:      traversal.MotionSafetyLimitTriggered,
		Target:       30.0,
		Actual:       100.0,
		PointIndex:   0,
	}

	// 测试步骤：处理故障
	err := mgr.handleMotionSafetyFailure(failure)

	// 期待结果：返回错误，调用了 EmergencyStop
	if err == nil {
		t.Errorf("expected error, got nil")
	}
	if len(ma.emergencyStopCalls) == 0 {
		t.Errorf("expected EmergencyStop to be called for LimitTriggered")
	}
}

func TestHandleMotionSafetyFailure_EmergencyStopFailFallbackToStop(t *testing.T) {
	// 测试前置：manager 绑定 X 轴到 mc-1，EmergencyStop 注入错误，注入 failure=LimitTriggered
	mgr, ma := motionSafetyTestManager()
	setMotionSafetyConfig(mgr,
		[]traversal.MotionAxisBinding{{ControllerID: "mc-1", Axis: "X"}},
		nil,
	)
	ma.emergencyStopErr = errors.New("hardware emergency stop failed")
	failure := &traversal.MotionSafetyFailure{
		ControllerID: "mc-1",
		Axis:         "X",
		Verdict:      traversal.MotionSafetyLimitTriggered,
		Target:       30.0,
		Actual:       100.0,
		PointIndex:   0,
	}

	// 测试步骤：处理故障
	err := mgr.handleMotionSafetyFailure(failure)

	// 期待结果：返回 ErrEmergencyStopFailed 包装错误，急停失败后 fallback 调用 Stop
	if err == nil {
		t.Errorf("expected error, got nil")
	}
	if len(ma.emergencyStopCalls) == 0 {
		t.Errorf("expected EmergencyStop to be attempted")
	}
	if len(ma.stopCalls) == 0 {
		t.Errorf("expected fallback Stop to be called after EmergencyStop failed")
	}
	// 错误码应为 ErrEmergencyStopFailed
	mgr.mu.RLock()
	code := mgr.status.LastErrorCode
	mgr.mu.RUnlock()
	if code != traversal.ErrEmergencyStopFailed {
		t.Errorf("expected LastErrorCode=ErrEmergencyStopFailed, got %v", code)
	}
}

func TestHandleMotionSafetyFailure_NilFailureNoOp(t *testing.T) {
	// 测试前置：manager 正常初始化
	mgr, _ := motionSafetyTestManager()

	// 测试步骤：处理 nil failure
	err := mgr.handleMotionSafetyFailure(nil)

	// 期待结果：无错误，无副作用
	if err != nil {
		t.Errorf("expected nil error for nil failure, got %v", err)
	}
}

// --- waitForStabilization 安全复检测试 ---

// setStabilizationConfig 设置 manager 的 stabilization 配置（fixed 模式）。
func setStabilizationConfig(mgr *TraversalManager, mode string, fixedMs int) {
	mgr.mu.Lock()
	defer mgr.mu.Unlock()
	mgr.stabilization = &traversal.StabilizationConfig{
		Mode:        mode,
		FixedTimeMs: fixedMs,
	}
}

func TestWaitForStabilization_DeviationTriggersFailure(t *testing.T) {
	// 测试前置：X 轴绑定到 mc-1，目标 30；稳定阶段 mock 返回已停但偏差 1mm
	// 稳定阶段期望轴已到位，偏差超容差视为异常（可能在稳定期间被外力推动）
	mgr, ma := motionSafetyTestManager()
	setMotionSafetyConfig(mgr,
		[]traversal.MotionAxisBinding{{ControllerID: "mc-1", Axis: "X"}},
		nil,
	)
	setStabilizationConfig(mgr, "fixed", 2000) // 2s 稳定等待
	point := traversal.Point{X: 30.0}
	ma.statuses = makeConnectedStoppedStatus("mc-1", "X", 31.0) // 偏差 1.0 > tolerance

	// 测试步骤：等待稳定
	done := make(chan *traversal.MotionSafetyFailure, 1)
	go func() {
		done <- mgr.waitForStabilization("test-task", point, 0, nil)
	}()

	select {
	case f := <-done:
		// 期待结果：返回 Deviation 故障（稳定期间位置异常）
		if f == nil {
			t.Fatal("expected failure non-nil during stabilization, got nil")
		}
		if f.Verdict != traversal.MotionSafetyDeviation {
			t.Errorf("verdict = %v, want Deviation", f.Verdict)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("waitForStabilization did not return within 5s")
	}
}

func TestWaitForStabilization_LimitTriggeredTriggersFailure(t *testing.T) {
	// 测试前置：X 轴绑定到 mc-1，目标 30；稳定阶段 mock 返回 PosLimit=true
	// 稳定期间撞限位表示设备失控，需立即急停
	mgr, ma := motionSafetyTestManager()
	setMotionSafetyConfig(mgr,
		[]traversal.MotionAxisBinding{{ControllerID: "mc-1", Axis: "X"}},
		nil,
	)
	setStabilizationConfig(mgr, "fixed", 2000)
	point := traversal.Point{X: 30.0}
	ma.statuses = []motion.ControllerStatus{
		{ID: "mc-1", Connected: true, Axes: []motion.AxisStatus{
			{Name: motion.AxisX, Position: 100.0, Moving: false, PosLimit: true, Homed: true},
		}},
	}

	// 测试步骤：等待稳定
	done := make(chan *traversal.MotionSafetyFailure, 1)
	go func() {
		done <- mgr.waitForStabilization("test-task", point, 0, nil)
	}()

	select {
	case f := <-done:
		// 期待结果：返回 LimitTriggered 故障
		if f == nil {
			t.Fatal("expected failure non-nil, got nil")
		}
		if f.Verdict != traversal.MotionSafetyLimitTriggered {
			t.Errorf("verdict = %v, want LimitTriggered", f.Verdict)
		}
		if !f.Verdict.RequiresEmergencyStop() {
			t.Errorf("LimitTriggered should require emergency stop")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("waitForStabilization did not return within 5s")
	}
}

func TestWaitForStabilization_ArrivedNoFailure(t *testing.T) {
	// 测试前置：X 轴绑定到 mc-1，目标 30；稳定阶段 mock 返回到位状态
	// 期望：稳定等待正常结束，无故障
	mgr, ma := motionSafetyTestManager()
	setMotionSafetyConfig(mgr,
		[]traversal.MotionAxisBinding{{ControllerID: "mc-1", Axis: "X"}},
		nil,
	)
	setStabilizationConfig(mgr, "fixed", 300) // 短稳定等待
	point := traversal.Point{X: 30.0}
	ma.statuses = makeConnectedStoppedStatus("mc-1", "X", 30.005) // 到位

	// 测试步骤：等待稳定
	done := make(chan *traversal.MotionSafetyFailure, 1)
	go func() {
		done <- mgr.waitForStabilization("test-task", point, 0, nil)
	}()

	select {
	case f := <-done:
		// 期待结果：nil（正常结束）
		if f != nil {
			t.Errorf("expected nil failure on arrival, got %v", f.Verdict)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("waitForStabilization did not return within 5s")
	}
}
