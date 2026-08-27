package calibration

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"windlabx4/services/api-go/internal/core/traversal"
)

// gateTestRuntime 风洞总压范围判定专用测试运行时。
// totalPressureFn 可选：覆盖 fiveHole.pTotal 通道（dev-1:18）的读取，
// 用于模拟总压延迟进入范围；nil 时回退 values map。
type gateTestRuntime struct {
	values          map[string]float64
	moves           []string
	stopCalls       int
	totalPressureFn func() (float64, bool)
}

func (r *gateTestRuntime) GetChannelValue(deviceID string, channelIndex int) (float64, bool) {
	if r.totalPressureFn != nil && deviceID == "dev-1" && channelIndex == 18 {
		return r.totalPressureFn()
	}
	v, ok := r.values[fmt.Sprintf("%s:%d", deviceID, channelIndex)]
	return v, ok
}

func (r *gateTestRuntime) GetLatestTimestamp(_ string) (int64, bool) { return 0, false }

func (r *gateTestRuntime) IsAcquiring(_ string) bool { return true }

func (r *gateTestRuntime) MoveToPosition(axis MotionAxisConfig, position float64) error {
	r.moves = append(r.moves, fmt.Sprintf("%s=%g", axis.Name, position))
	return nil
}

func (r *gateTestRuntime) WaitForMotionComplete() (bool, traversal.MotionInterruptReason, *traversal.MotionSafetyFailure) {
	return true, traversal.MotionInterruptNone, nil
}

func (r *gateTestRuntime) StopMotion() error {
	r.stopCalls++
	return nil
}

func gateEnabledConfig() Config {
	config := completeFiveHoleConfig()
	config.TunnelTotalPressureGate = &TunnelTotalPressureGateConfig{
		Enabled: true, MinTotalPressure: 50, MaxTotalPressure: 120,
	}
	return config
}

// TestTotalPressureGateAllowsAcquisitionWhenInRange 启用判定且总压在范围内 → 正常采集。
func TestTotalPressureGateAllowsAcquisitionWhenInRange(t *testing.T) {
	config := gateEnabledConfig()
	runtime := &gateTestRuntime{values: completeFiveHoleValues()}

	engine := NewAutomaticCalibration(config, nil, runtime, nil, nil)
	if err := engine.Start(NewFiveHoleAlgorithm()); err != nil {
		t.Fatalf("范围内应正常完成校准: %v", err)
	}
	if points := engine.GetDataPoints(); len(points) != 1 {
		t.Fatalf("应采集 1 个点，实际 %d", len(points))
	}
}

// TestTotalPressureGateWaitsUntilInRange 初始总压在范围外 → 等待，进入范围后正常采集。
func TestTotalPressureGateWaitsUntilInRange(t *testing.T) {
	config := gateEnabledConfig()
	config.StopOnError = true

	var inRange atomic.Bool
	// 初始 1000 Pa（范围 [50,120] 之外），~300ms 后翻转为 100 Pa（范围内）
	runtime := &gateTestRuntime{
		values: completeFiveHoleValues(),
		totalPressureFn: func() (float64, bool) {
			if inRange.Load() {
				return 100, true
			}
			return 1000, true
		},
	}
	go func() {
		time.Sleep(300 * time.Millisecond)
		inRange.Store(true)
	}()

	engine := NewAutomaticCalibration(config, nil, runtime, nil, nil)
	start := time.Now()
	if err := engine.Start(NewFiveHoleAlgorithm()); err != nil {
		t.Fatalf("进入范围后应完成校准: %v", err)
	}
	if elapsed := time.Since(start); elapsed < 300*time.Millisecond {
		t.Fatalf("应等待总压进入范围后再采集，实际仅耗时 %v", elapsed)
	}
	if points := engine.GetDataPoints(); len(points) != 1 {
		t.Fatalf("应采集 1 个点，实际 %d", len(points))
	}
}

// TestTotalPressureGateTimeoutStopsCalibration 总压始终在范围外 + 短超时 → 停止校准并返回超时错误。
func TestTotalPressureGateTimeoutStopsCalibration(t *testing.T) {
	config := gateEnabledConfig()
	config.StopOnError = true
	config.TunnelTotalPressureGate.TimeoutSec = 1
	runtime := &gateTestRuntime{
		values: completeFiveHoleValues(),
		totalPressureFn: func() (float64, bool) {
			return 1000, true // 永不在范围 [50,120] 内
		},
	}

	engine := NewAutomaticCalibration(config, nil, runtime, nil, nil)
	err := engine.Start(NewFiveHoleAlgorithm())
	if err == nil || !strings.Contains(err.Error(), "等待超时") {
		t.Fatalf("应返回总压范围等待超时错误，实际: %v", err)
	}
	if engine.IsRunning() {
		t.Fatal("超时后校准应已停止")
	}
	if points := engine.GetDataPoints(); len(points) != 0 {
		t.Fatalf("超时前不应采集任何点，实际 %d", len(points))
	}
}

// TestTotalPressureGateTimeoutStopsCalibrationUnconditionally 回归测试（code-review Critical 1）：
// StopOnError=false（前端默认）+ 多点配置时，门控超时仍必须返回错误并停止校准，
// 不得因 a.Stop() 置位后主循环以 !IsRunning()→nil 退出，被 Manager 误判为 StateCompleted。
func TestTotalPressureGateTimeoutStopsCalibrationUnconditionally(t *testing.T) {
	config := gateEnabledConfig()
	config.StopOnError = false
	config.Points = []CalPoint{
		{ID: 1, Coordinates: map[string]float64{"α": 0, "β": 0}},
		{ID: 2, Coordinates: map[string]float64{"α": 5, "β": 0}},
	}
	config.TunnelTotalPressureGate.TimeoutSec = 1
	runtime := &gateTestRuntime{
		values: completeFiveHoleValues(),
		totalPressureFn: func() (float64, bool) {
			return 1000, true // 永不在范围 [50,120] 内
		},
	}

	engine := NewAutomaticCalibration(config, nil, runtime, nil, nil)
	err := engine.Start(NewFiveHoleAlgorithm())
	if err == nil {
		t.Fatal("StopOnError=false 时门控超时也必须返回错误，实际返回 nil（会被 Manager 误判为成功完成）")
	}
	if !errors.Is(err, ErrGateConditionFailed) {
		t.Fatalf("应包含 ErrGateConditionFailed 哨兵错误，实际: %v", err)
	}
	if engine.IsRunning() {
		t.Fatal("超时后校准应已停止")
	}
	if points := engine.GetDataPoints(); len(points) != 0 {
		t.Fatalf("超时前不应采集任何点，实际 %d", len(points))
	}
}

// TestSphereTankGateTimeoutStopsCalibrationUnconditionally 与总压门控同类的回归测试：
// 球罐门控超时在 StopOnError=false 下同样必须返回错误（同一哨兵 ErrGateConditionFailed）。
func TestSphereTankGateTimeoutStopsCalibrationUnconditionally(t *testing.T) {
	config := completeFiveHoleConfig()
	config.StopOnError = false
	config.SphereTankGate = &SphereTankGateConfig{
		Enabled:           true,
		WaitTimeSec:       100,
		TimeoutSec:        1,
		StableTimeChannel: ChannelRef{DeviceID: "dev-gate", ChannelIndex: 0},
	}
	// 稳定时间通道返回合法但不满足条件的值（0s < WaitTimeSec=100s），
	// 使球罐闸门循环走到超时判定分支（读失败路径会 continue 跳过超时检查）。
	values := completeFiveHoleValues()
	values["dev-gate:0"] = 0
	runtime := &gateTestRuntime{values: values}

	engine := NewAutomaticCalibration(config, nil, runtime, nil, nil)
	err := engine.Start(NewFiveHoleAlgorithm())
	if err == nil {
		t.Fatal("球罐门控超时在 StopOnError=false 下也必须返回错误，实际返回 nil")
	}
	if !errors.Is(err, ErrGateConditionFailed) {
		t.Fatalf("应包含 ErrGateConditionFailed 哨兵错误，实际: %v", err)
	}
	if engine.IsRunning() {
		t.Fatal("超时后校准应已停止")
	}
}

// TestTotalPressureGateDisabledHasNoEffect 未启用时行为与无门控一致（正常采集）。
func TestTotalPressureGateDisabledHasNoEffect(t *testing.T) {
	config := completeFiveHoleConfig()
	config.TunnelTotalPressureGate = &TunnelTotalPressureGateConfig{Enabled: false}
	runtime := &gateTestRuntime{values: completeFiveHoleValues()}

	engine := NewAutomaticCalibration(config, nil, runtime, nil, nil)
	if err := engine.Start(NewFiveHoleAlgorithm()); err != nil {
		t.Fatalf("未启用门控应正常完成校准: %v", err)
	}
	if points := engine.GetDataPoints(); len(points) != 1 {
		t.Fatalf("应采集 1 个点，实际 %d", len(points))
	}
}

// TestTotalPressureGateSkippedForNonFiveHole 非五孔类型即使配置了门控也应忽略。
// 直接调用 waitForTotalPressureGateIfNeeded 验证类型分支，避免构造三孔完整配置。
func TestTotalPressureGateSkippedForNonFiveHole(t *testing.T) {
	config := completeFiveHoleConfig()
	config.Type = string(TypeThreeHole)
	// 非五孔类型 + 门控启用（故意缺 pTotal 通道，若误判则会校验报错）
	config.ProbeChannels = config.ProbeChannels[:6]
	config.TunnelTotalPressureGate = &TunnelTotalPressureGateConfig{
		Enabled: true, MinTotalPressure: 50, MaxTotalPressure: 120,
	}
	engine := NewAutomaticCalibration(config, nil, nil, nil, nil)

	if err := engine.waitForTotalPressureGateIfNeeded(context.Background()); err != nil {
		t.Fatalf("非五孔类型应跳过门控判定，实际: %v", err)
	}
}

// TestTotalPressureGateInvalidConfigReturnsError 启用但配置非法（min>max）→ 返回校验错误。
func TestTotalPressureGateInvalidConfigReturnsError(t *testing.T) {
	config := gateEnabledConfig()
	config.TunnelTotalPressureGate.MinTotalPressure = 200
	config.TunnelTotalPressureGate.MaxTotalPressure = 100
	engine := NewAutomaticCalibration(config, nil, nil, nil, nil)

	err := engine.waitForTotalPressureGateIfNeeded(context.Background())
	if err == nil || !strings.Contains(err.Error(), "范围非法") {
		t.Fatalf("min>max 应返回范围非法错误，实际: %v", err)
	}
}

// TestTotalPressureGateContextCancelStopsWaiting 等待期间 ctx 取消 → 立即返回 context.Canceled。
func TestTotalPressureGateContextCancelStopsWaiting(t *testing.T) {
	config := gateEnabledConfig()
	runtime := &gateTestRuntime{
		values: completeFiveHoleValues(),
		totalPressureFn: func() (float64, bool) {
			return 1000, true // 永不在范围
		},
	}
	engine := NewAutomaticCalibration(config, nil, runtime, nil, nil)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- engine.StartWithContext(ctx, NewFiveHoleAlgorithm()) }()

	// 等待引擎进入总压范围等待（无运动轴，启动后立即到达 gate；300ms 足够越过首个 100ms 轮询间隔）
	time.Sleep(300 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("ctx 取消应返回 context.Canceled，实际: %v", err)
		}
	case <-time.After(2 * time.Second):
		engine.Stop()
		t.Fatal("ctx 取消后未及时退出等待")
	}
}

// freshGateRuntime 带时间戳能力的总压门控测试运行时：
// GetLatestTimestamp 返回 ts 计数，测试可通过 advance 模拟新帧、冻结模拟设备停止采集。
type freshGateRuntime struct {
	values    map[string]float64
	ts        atomic.Int64
	stopCalls int
}

func (r *freshGateRuntime) GetChannelValue(deviceID string, channelIndex int) (float64, bool) {
	v, ok := r.values[fmt.Sprintf("%s:%d", deviceID, channelIndex)]
	return v, ok
}

func (r *freshGateRuntime) GetLatestTimestamp(_ string) (int64, bool) { return r.ts.Load(), true }

func (r *freshGateRuntime) IsAcquiring(_ string) bool { return true }

func (r *freshGateRuntime) MoveToPosition(axis MotionAxisConfig, position float64) error {
	return nil
}

func (r *freshGateRuntime) WaitForMotionComplete() (bool, traversal.MotionInterruptReason, *traversal.MotionSafetyFailure) {
	return true, traversal.MotionInterruptNone, nil
}

func (r *freshGateRuntime) StopMotion() error {
	r.stopCalls++
	return nil
}

// TestTotalPressureGateRejectsStaleCachedValue 回归测试（code-review Critical 3）：
// 设备停止采集（时间戳冻结）但缓存总压值恰好在范围内时，门控不得接受该陈旧值，
// 必须持续等待直至超时——避免基于运动前缓存的旧数据采集。
func TestTotalPressureGateRejectsStaleCachedValue(t *testing.T) {
	config := gateEnabledConfig()
	config.StopOnError = true
	config.TunnelTotalPressureGate.TimeoutSec = 1
	runtime := &freshGateRuntime{
		values: completeFiveHoleValues(), // dev-1:18 pTotal = 80，在范围 [50,120] 内
	}
	runtime.ts.Store(1000) // 时间戳冻结：设备停止采集，无新帧

	engine := NewAutomaticCalibration(config, nil, runtime, nil, nil)
	err := engine.Start(NewFiveHoleAlgorithm())
	if err == nil || !strings.Contains(err.Error(), "等待超时") {
		t.Fatalf("陈旧缓存总压不应被接受，应等待超时并返回错误，实际: %v", err)
	}
	if engine.IsRunning() {
		t.Fatal("超时后校准应已停止")
	}
	if points := engine.GetDataPoints(); len(points) != 0 {
		t.Fatalf("陈旧缓存值不应触发采集，实际采集 %d 点", len(points))
	}
}

// TestTotalPressureGateAcceptsFreshTimestampValue 带时间戳能力的正常路径：
// 设备持续产新帧（时间戳前进）且总压在范围内 → 门控放行，正常采集。
func TestTotalPressureGateAcceptsFreshTimestampValue(t *testing.T) {
	config := gateEnabledConfig()
	runtime := &freshGateRuntime{values: completeFiveHoleValues()}
	runtime.ts.Store(1000)

	engine := NewAutomaticCalibration(config, nil, runtime, nil, nil)
	done := make(chan error, 1)
	go func() { done <- engine.Start(NewFiveHoleAlgorithm()) }()

	// 模拟 DAQ 持续产新帧：时间戳每 50ms 前进
	stop := make(chan struct{})
	go func() {
		ticker := time.NewTicker(50 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				runtime.ts.Add(1)
			case <-stop:
				return
			}
		}
	}()

	select {
	case err := <-done:
		close(stop)
		if err != nil {
			t.Fatalf("时间戳前进 + 值在范围内应完成校准: %v", err)
		}
		if points := engine.GetDataPoints(); len(points) != 1 {
			t.Fatalf("应采集 1 个点，实际 %d", len(points))
		}
	case <-time.After(3 * time.Second):
		close(stop)
		engine.Stop()
		t.Fatal("新帧到达后门控应放行采集，3 秒内未完成")
	}
}
