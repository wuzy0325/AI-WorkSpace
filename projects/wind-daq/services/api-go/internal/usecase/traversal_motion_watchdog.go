// Package usecase — 运动安全跨样本看门狗
//
// 维护运动轴的跨样本历史，识别单次快照无法判定的两类异常：
//  1. NoProgress：Moving=true 但位置长时间无有效进展（卡死）
//  2. Overshoot：Moving=true 且位置已穿越目标位置（越过目标后继续冲）
//
// 设计要点：
//   - 每轴独立维护状态（lastPosition/lastSide/lastProgressTime），多轴互不影响
//   - 使用单调时钟避免系统时间回拨
//   - 轴停止时立即清空历史，下次运动重新初始化
//   - Observe 返回 *MotionSafetyFailure 或 nil，调用方无需再判定
package usecase

import (
	"math"
	"time"

	"wind-daq/services/api-go/internal/core/motion"
	"wind-daq/services/api-go/internal/core/traversal"
)

// axisWatchdogState 单轴的看门狗历史状态
//
// 仅在 Moving=true 时维护；Moving=false 时 Reset 清空。
// 位置或目标变化时也需 Reset，避免跨点位误判。
type axisWatchdogState struct {
	// initialized 是否已初始化本次运动观察
	initialized bool

	// lastPosition 上次观察到的位置
	lastPosition float64

	// lastSide 上次位置相对目标的侧向：-1 表示 < target, +1 表示 > target, 0 表示 == target
	// 用于检测穿越目标
	lastSide int

	// lastProgressAt 上次有效进展时刻（单调时钟）
	lastProgressAt time.Time
}

// motionWatchdog 跨样本运动安全看门狗
//
// 使用方式：
//  1. 每次新点位开始前调用 Reset() 清空所有轴状态
//  2. waitForMotionComplete 轮询循环中调用 Observe() 注入快照
//  3. 任一 Observe 返回非 nil failure 即停止遍历
type motionWatchdog struct {
	states map[string]*axisWatchdogState // 键格式: controllerID|axisName
}

// newMotionWatchdog 构造跨样本看门狗
func newMotionWatchdog() *motionWatchdog {
	return &motionWatchdog{
		states: make(map[string]*axisWatchdogState),
	}
}

// watchdogKey 生成 (controllerID, axis) 的唯一键
func watchdogKey(controllerID, axis string) string {
	return controllerID + "|" + axis
}

// Reset 清空所有轴的看门狗历史。
//
// 调用时机：
//   - 新点位开始
//   - 暂停后恢复
//   - 目标位置变化（理论上新点位会触发，但保险起见）
func (w *motionWatchdog) Reset() {
	for k := range w.states {
		delete(w.states, k)
	}
}

// Observe 注入单次轮询快照，返回该轴的故障判定或 nil。
//
// 参数：
//   - controllerID: 控制器 ID
//   - axis: 轴状态快照
//   - target: 该轴目标位置
//   - cfg: 已解析的运动安全配置（指针字段非 nil）
//   - pointIndex: 当前点位索引（用于故障快照）
//
// 返回 nil 表示无故障，继续等待；返回 *MotionSafetyFailure 表示需立即停止。
// 轴已停时立即返回 nil——停止状态由 EvaluateMotionSafety 处理。
func (w *motionWatchdog) Observe(
	controllerID string,
	axis motion.AxisStatus,
	target float64,
	cfg traversal.MotionSafetyConfig,
	pointIndex int,
) *traversal.MotionSafetyFailure {
	key := watchdogKey(controllerID, string(axis.Name))
	state, exists := w.states[key]
	if !exists {
		state = &axisWatchdogState{}
		w.states[key] = state
	}

	// 轴已停：清空历史，交给 EvaluateMotionSafety 判定
	if !axis.Moving {
		state.initialized = false
		return nil
	}

	now := time.Now()
	currentSide := signPosition(axis.Position - target)
	tolerance := resolveFloat64Ptr(cfg.ArrivalTolerance, 0.0)

	// 驱动状态可能晚于位置到位清零。轴已进入到位容差时继续等待 Moving=false，
	// 但不能把目标位置上的静止误判为卡死；离开容差后再重新开始计时。
	//
	// 关键约束：不能清除 initialized——清除会丢失穿越前的 lastSide，
	// 导致 29.5 → 30.0（容差区内） → 31.0 这类连续运动在第三帧被当作首次观察，
	// 不会报告 Overshoot。修复方案：保留 lastSide 用于后续穿越检测，
	// 仅重置 lastProgressAt 避免静止在目标位被误判为 NoProgress。
	// 若首次观察就落在容差区内（如 30.005），先初始化基线（currentSide 可能为 0 或 ±1）。
	if math.Abs(axis.Position-target) <= tolerance {
		if !state.initialized {
			state.initialized = true
			state.lastPosition = axis.Position
			state.lastSide = currentSide
		}
		state.lastProgressAt = now
		return nil
	}

	// 首次观察运动：初始化基线
	if !state.initialized {
		state.initialized = true
		state.lastPosition = axis.Position
		state.lastSide = currentSide
		state.lastProgressAt = now
		return nil
	}

	// 检测越过目标：侧向翻转且偏差大于到位容差
	// 仅当 lastSide 非零（之前确实在目标某一侧）且 currentSide 也非零（确实穿越到另一侧）时触发
	if state.lastSide != 0 && currentSide != 0 && state.lastSide != currentSide {
		deviation := math.Abs(axis.Position - target)
		if deviation > tolerance {
			return &traversal.MotionSafetyFailure{
				ControllerID: controllerID,
				Axis:         string(axis.Name),
				Verdict:      traversal.MotionSafetyOvershoot,
				Target:       target,
				Actual:       axis.Position,
				PointIndex:   pointIndex,
			}
		}
	}

	// 检测无进展：位置变化达到 ProgressEpsilon 视为有进展，重置计时
	epsilon := resolveFloat64Ptr(cfg.ProgressEpsilon, 0.0)
	if math.Abs(axis.Position-state.lastPosition) >= epsilon {
		state.lastPosition = axis.Position
		state.lastProgressAt = now
		state.lastSide = currentSide
		return nil
	}

	// 位置无有效进展：检查是否超过 NoProgressTimeoutMs
	timeoutMs := resolveIntPtr(cfg.NoProgressTimeoutMs, 0)
	if timeoutMs > 0 && now.Sub(state.lastProgressAt) >= time.Duration(timeoutMs)*time.Millisecond {
		return &traversal.MotionSafetyFailure{
			ControllerID: controllerID,
			Axis:         string(axis.Name),
			Verdict:      traversal.MotionSafetyNoProgress,
			Target:       target,
			Actual:       axis.Position,
			PointIndex:   pointIndex,
		}
	}

	// 更新侧向（即使无进展也跟踪，避免误判下次小变化为穿越）
	state.lastSide = currentSide
	return nil
}

// resolveFloat64Ptr 解引用 *float64，nil 时返回 fallback。
// 用于看门狗 Observe 中"取值或零值 fallback"场景，统一 cfg 指针字段的解引用模板。
func resolveFloat64Ptr(p *float64, fallback float64) float64 {
	if p != nil {
		return *p
	}
	return fallback
}

// resolveIntPtr 同 resolveFloat64Ptr，用于 *int 字段。
func resolveIntPtr(p *int, fallback int) int {
	if p != nil {
		return *p
	}
	return fallback
}

// signPosition 返回 v 的符号：-1 / 0 / +1
//
// 用于判定位置相对目标的侧向。
// NaN 输入返回 0（保守处理：不触发越过判定，避免 NaN 误报）。
func signPosition(v float64) int {
	if math.IsNaN(v) || v == 0 {
		return 0
	}
	if v > 0 {
		return 1
	}
	return -1
}
