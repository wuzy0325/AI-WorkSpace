// Package usecase — TraversalManager 内部辅助函数（从 traversal.go 拆分）
//
// 包含错误流式构造、任务取消轮询、通道映射、运动轴目标过滤等纯工具方法。
package usecase

import (
	"fmt"
	"log/slog"
	"math"
	"time"

	"wind-daq/services/api-go/internal/core/device"
	"wind-daq/services/api-go/internal/core/motion"
	"wind-daq/services/api-go/internal/core/traversal"
)

// formatCoord 格式化坐标值用于日志输出。
// NaN 表示该轴不参与遍历运动（line/rectangle/sector 模式 markAxesNaN 标记），
// 输出 "N/A" 比 "NaN" 更直观，避免日志排查时误判为浮点异常。
func formatCoord(v float64) string {
	if math.IsNaN(v) {
		return "N/A"
	}
	return fmt.Sprintf("%.2f", v)
}

func (m *TraversalManager) fail(format string, args ...any) error {
	return m.failWithCode(format, traversal.ErrUnknown, args...)
}

// failWithCode 带错误码的失败
func (m *TraversalManager) failWithCode(format string, code traversal.ErrorCode, args ...any) error {
	message := fmt.Sprintf(format, args...)
	m.mu.Lock()
	m.setErrorLocked(message, code)
	taskID := m.status.TaskID
	m.mu.Unlock()

	slog.Error("traversal failed",
		"component", "traversal",
		"task_id", taskID,
		"error_code", string(code),
		"error", message,
	)
	return fmt.Errorf("%s", message)
}

func (m *TraversalManager) setErrorLocked(message string, code traversal.ErrorCode) {
	m.status.State = traversal.StateError
	m.status.LastError = message
	m.status.LastErrorCode = code
	// 清空运动安全故障快照：setErrorLocked 是所有错误路径的公共出口，
	// 非运动安全故障（采集失败/保存失败等）不应残留上一次的故障现场。
	// handleMotionSafetyFailure 在调用 failWithCode 之后会单独写入快照，
	// 此处清空不会覆盖运动安全故障路径的写入。
	m.status.MotionSafetyFailure = nil
}

// isTaskCancelled 检查任务是否已取消
func (m *TraversalManager) isTaskCancelled(taskID string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.status.TaskID != taskID || m.isStopped
}

// sleepWithTaskCheck 带任务取消检查的睡眠
func (m *TraversalManager) sleepWithTaskCheck(taskID string, d time.Duration) {
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		if m.isTaskCancelled(taskID) {
			return
		}
		time.Sleep(cancelCheckPoll)
	}
}

// valuesForChannels 从数据载荷中提取指定通道索引的值
func valuesForChannels(payload device.DataPayload, channels []int) map[int]float64 {
	valuesByIndex := make(map[int]float64, len(payload.Channels))
	for i, value := range payload.Channels {
		chIdx := i
		if i < len(payload.ChannelIndices) {
			chIdx = payload.ChannelIndices[i]
		}
		valuesByIndex[chIdx] = value
	}

	values := make(map[int]float64, len(channels))
	for _, channel := range channels {
		value, ok := valuesByIndex[channel]
		if ok {
			values[channel] = value
		}
	}
	return values
}

// resolveMotionAxes 在调用 availableAxisTargets 前对 motionAxes 做容错预处理。
//
// 回退规则：若所有非空 controllerId 都不匹配任何**已连接**控制器的 ID，
// 视为旧配置 / 错误配置（典型表现：前端用别名 "sim-motion-1" 而后端 profile.ID
// 是 UUID），统一把 controllerId 清空，回退到「按轴名匹配」的旧行为。
//
// 这样既保留了「多控制器同时连接时严格按 controllerId 过滤」的能力，
// 又避免了 controllerId 不匹配时 availableAxisTargets 对所有控制器返回 nil、
// 遍历跳过运动直接进入「稳定中」阶段的空转 bug。
//
// 注意：本函数仅做一次全局判断，不会改变部分匹配场景下的行为——
// 只要存在任意一个绑定匹配（或 controllerId 为空通配），原 motionAxes 原样返回。
func resolveMotionAxes(motionAxes []traversal.MotionAxisBinding, statuses []motion.ControllerStatus) []traversal.MotionAxisBinding {
	if len(motionAxes) == 0 {
		return motionAxes
	}
	// 收集已连接控制器的 ID 集合
	connectedIDs := make(map[string]bool, len(statuses))
	for _, s := range statuses {
		if s.Connected {
			connectedIDs[s.ID] = true
		}
	}
	// 任意一个绑定匹配（含空 controllerId 视为通配）→ 不需要回退
	for _, b := range motionAxes {
		if b.ControllerID == "" {
			return motionAxes
		}
		if connectedIDs[b.ControllerID] {
			return motionAxes
		}
	}
	// 全部不匹配：清空 controllerId，回退到按轴名匹配
	slog.Warn("traversal motionAxes controllerId mismatched, falling back to axis-name matching",
		"component", "traversal",
		"connected_controller_count", len(connectedIDs),
	)
	fallback := make([]traversal.MotionAxisBinding, len(motionAxes))
	for i, b := range motionAxes {
		fallback[i] = traversal.MotionAxisBinding{
			Name:         b.Name,
			ControllerID: "",
			Axis:         b.Axis,
		}
	}
	return fallback
}

// validateSectorOrigin 确认扇形机构的径向轴和旋转轴已由操作员在首测点手动置零。
// 扇形路径使用相对目标，未置零时绝对 MoveTo(0) 会把机构拉回控制器原点。
func validateSectorOrigin(statuses []motion.ControllerStatus, motionAxes []traversal.MotionAxisBinding, safety *traversal.MotionSafetyConfig) error {
	if len(motionAxes) == 0 {
		return fmt.Errorf("sector origin check requires X and Y motion axis bindings")
	}

	foundLogical := map[string]bool{"X": false, "Y": false}
	for _, binding := range motionAxes {
		if binding.Name != "X" && binding.Name != "Y" {
			continue
		}
		if binding.ControllerID == "" {
			return fmt.Errorf("sector origin check requires an explicit controller binding for %s target", binding.Name)
		}
		foundLogical[binding.Name] = true
		matched := false
		for _, status := range statuses {
			if !status.Connected || binding.ControllerID != status.ID {
				continue
			}
			for _, axis := range status.Axes {
				if string(axis.Name) != binding.Axis {
					continue
				}
				matched = true
				if axis.Moving {
					return fmt.Errorf("sector origin check failed: controller %s axis %s is moving", status.ID, axis.Name)
				}
				resolved := safety.Resolve(binding.Axis)
				tolerance := *resolved.ArrivalTolerance
				// NaN 位置必须显式判失败：math.Abs(NaN) > tolerance 恒为 false，
				// 不拦截会让位置反馈缺失的轴静默通过原点校验。
				if math.IsNaN(axis.Position) {
					return fmt.Errorf("sector origin check failed: controller %s axis %s current position is NaN (position feedback unavailable); re-home the axis and zero both axes", status.ID, axis.Name)
				}
				if math.Abs(axis.Position) > tolerance {
					return fmt.Errorf("sector origin check failed: controller %s axis %s current position %.6g is not zero (tolerance %.6g); move to the first point and zero both axes",
						status.ID, axis.Name, axis.Position, tolerance)
				}
				break
			}
			if matched {
				break
			}
		}
		if !matched {
			return fmt.Errorf("sector origin check failed: physical axis %s for %s target is unavailable", binding.Axis, binding.Name)
		}
	}
	if !foundLogical["X"] || !foundLogical["Y"] {
		return fmt.Errorf("sector origin check requires both X (radial) and Y (rotation) target bindings")
	}
	return nil
}

// motionTargetsReachedWithTolerance 判断当前状态快照中的所有有效运动目标是否到位。
//
// 到位容差从 MotionSafetyConfig 读取，支持按轴覆盖。当 cfg 为 nil 时回退到默认值（0.2mm）。
// 必须至少检查到一个已连接控制器上的目标轴；零目标不等于全部完成，避免状态缺失时提前进入稳定阶段。
// 当 checkedTargets==0 且 motionAxes 非空时，说明轴名配置与已连接控制器不匹配，
// 打 warning 日志暴露根因，避免 120s 静默超时。
// 返回 (allReached, checkedTargets)：
//   - allReached: 所有目标轴均到位
//   - checkedTargets: 实际检查的目标轴数（用于空配置检测）
func motionTargetsReachedWithTolerance(
	statuses []motion.ControllerStatus,
	point traversal.Point,
	motionAxes []traversal.MotionAxisBinding,
	cfg *traversal.MotionSafetyConfig,
) (bool, int) {
	checkedTargets := 0
	for _, status := range statuses {
		if !status.Connected {
			continue
		}
		targets := availableAxisTargets(status, point, motionAxes)
		for _, axis := range status.Axes {
			target, hasTarget := targets[axis.Name]
			if !hasTarget {
				continue
			}
			checkedTargets++
			resolved := cfg.Resolve(string(axis.Name))
			tolerance := *resolved.ArrivalTolerance
			if axis.Moving || math.Abs(axis.Position-target) > tolerance {
				return false, checkedTargets
			}
		}
	}
	if checkedTargets == 0 && len(motionAxes) > 0 {
		slog.Warn("traversal motionTargetsReached: no axis matched motionAxes config, will timeout",
			"component", "traversal",
			"motion_axes", motionAxes,
		)
	}
	return checkedTargets > 0, checkedTargets
}

// EvaluateMotionSafety 运动安全判定纯函数
//
// 根据**单次快照**的轴状态、目标位置和已解析的 MotionSafetyConfig，
// 返回运动安全判定结果。函数无副作用、不访问硬件，全部判定逻辑可单测覆盖。
//
// 判定优先级（spec §EvaluateMotionSafety）：
//  1. 撞限位（PosLimit/NegLimit）→ LimitTriggered（急停）
//  2. 运动中（Moving=true）→ OK（不判偏差，交给看门狗检测卡死/越过）
//  3. 轴已停 → 检查偏差：
//     - deviation ≤ ArrivalTolerance → Arrived
//     - deviation ≥ CriticalDeviationLimit → CriticalDeviation（急停）
//     - 其他 → Deviation（普通停止）
//
// 参数 cfg 必须是 Resolve() 后的有效配置（指针字段非 nil）。
// 调用方负责按轴名解析覆盖项，本函数不做覆盖合并。
//
// NoProgress 和 Overshoot 不在本函数职责内——它们需要跨样本历史，
// 由 motionWatchdog.Observe 维护。
func EvaluateMotionSafety(
	axis motion.AxisStatus,
	target float64,
	cfg traversal.MotionSafetyConfig,
) traversal.MotionSafetyVerdict {
	// 1. 撞限位优先级最高，无论运动状态如何都立即判定为急停场景
	if axis.PosLimit || axis.NegLimit {
		return traversal.MotionSafetyLimitTriggered
	}

	// 2. 运动中不判偏差——运动中距目标远是正常现象，不能误判为超差。
	// 卡死和越过目标由跨样本看门狗识别。
	if axis.Moving {
		return traversal.MotionSafetyOK
	}

	// 3. 轴已停，独立验证是否到位
	deviation := math.Abs(axis.Position - target)
	tolerance := 0.0
	if cfg.ArrivalTolerance != nil {
		tolerance = *cfg.ArrivalTolerance
	}
	if deviation <= tolerance {
		return traversal.MotionSafetyArrived
	}

	critical := 0.0
	if cfg.CriticalDeviationLimit != nil {
		critical = *cfg.CriticalDeviationLimit
	}
	if deviation >= critical {
		return traversal.MotionSafetyCriticalDeviation
	}

	// 4. 偏差超过到位容差但未达严重偏离阈值——普通超差
	return traversal.MotionSafetyDeviation
}

// availableAxisTargets 根据遍历点坐标和参与运动的轴绑定，生成 (轴名→目标位置) 映射。
// motionAxes 为参与遍历运动的「控制器+轴」绑定；仅匹配当前 status.ID 的轴会生成目标。
// 为空时（旧配置兼容）保持原行为：对 status.Axes 中所有轴生成目标。
//
// NaN 目标跳过：line/rectangle/sector 模式会通过 markAxesNaN 将未配置的轴标记为 NaN，
// 表示"该轴不参与遍历运动"。此函数跳过 NaN 目标轴，既不发 MoveTo 也不参与到位判定，
// 避免把 Z/U 等未配置轴强制归零（曾导致 9ed66bb 后遍历卡死 120s 超时）。
//
// 控制器过滤：绑定了 controllerId 时，仅对对应控制器生成目标。否则若模拟控制器与
// 真实 WTNMC4A 同时连接，会对真实控制器也等待到位，表现为「模拟轴已到位仍超时」。
//
// 调用方应先调用 resolveMotionAxes 预处理 motionAxes：当配置的所有 controllerId
// 都不匹配任何已连接控制器时，回退到按轴名匹配，避免遍历空转。
func availableAxisTargets(status motion.ControllerStatus, point traversal.Point, motionAxes []traversal.MotionAxisBinding) map[motion.AxisName]float64 {
	// 按物理轴名索引当前控制器生效的绑定：跳过 Axis 为空的绑定，
	// 以及 ControllerID 非空且不匹配本控制器的绑定；ControllerID 为空的绑定表示「任意控制器的该轴」
	bindingsByAxis := make(map[string]traversal.MotionAxisBinding, len(motionAxes))
	for _, binding := range motionAxes {
		if binding.Axis == "" {
			continue
		}
		if binding.ControllerID != "" && binding.ControllerID != status.ID {
			continue
		}
		bindingsByAxis[binding.Axis] = binding
	}
	// 配置了绑定，但当前控制器不在任何绑定中 → 不生成目标（跳过该控制器）
	if len(motionAxes) > 0 && len(bindingsByAxis) == 0 {
		return nil
	}
	targets := make(map[motion.AxisName]float64, len(status.Axes))
	for _, axis := range status.Axes {
		// motionAxes 非空时，仅对白名单中的轴生成目标，避免对未配置/未接硬件的轴强制归零
		var target float64
		logicalAxis := string(axis.Name)
		if len(motionAxes) > 0 {
			binding, ok := bindingsByAxis[string(axis.Name)]
			if !ok {
				continue
			}
			// Name 是 UI 中的逻辑目标（X方向/Y方向）；旧配置无 Name 时按物理轴名取值。
			if binding.Name != "" {
				logicalAxis = binding.Name
			}
		}
		switch logicalAxis {
		case string(motion.AxisX):
			target = point.X
		case string(motion.AxisY):
			target = point.Y
		case string(motion.AxisZ):
			target = point.Z
		// U 轴仅在 motion.ControllerStatus.Axes 含 AxisU 时生效
		// （如旋转台 / 第四轴位移机构），无 U 轴的控制器 profile 会自动跳过此 case
		case string(motion.AxisU):
			target = point.U
		default:
			continue
		}
		// NaN 目标表示该轴不参与遍历运动（如 line 模式的 Y/Z/U），跳过不发 MoveTo
		if math.IsNaN(target) {
			continue
		}
		targets[axis.Name] = target
	}
	return targets
}
