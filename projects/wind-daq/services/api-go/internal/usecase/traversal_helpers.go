// Package usecase — TraversalManager 内部辅助函数（从 traversal.go 拆分）
//
// 包含错误流式构造、任务取消轮询、通道映射、运动轴目标过滤等纯工具方法。
package usecase

import (
	"fmt"
	"log/slog"
	"time"

	"wind-daq/services/api-go/internal/core/device"
	"wind-daq/services/api-go/internal/core/motion"
	"wind-daq/services/api-go/internal/core/traversal"
)

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

func availableAxisTargets(status motion.ControllerStatus, point traversal.Point) map[motion.AxisName]float64 {
	targets := make(map[motion.AxisName]float64, len(status.Axes))
	for _, axis := range status.Axes {
		switch axis.Name {
		case motion.AxisX:
			targets[axis.Name] = point.X
		case motion.AxisY:
			targets[axis.Name] = point.Y
		case motion.AxisZ:
			targets[axis.Name] = point.Z
		// U 轴仅在 motion.ControllerStatus.Axes 含 AxisU 时生效
		// （如旋转台 / 第四轴位移机构），无 U 轴的控制器 profile 会自动跳过此 case
		case motion.AxisU:
			targets[axis.Name] = point.U
		}
	}
	return targets
}

// RunTraversalLoop 主循环：按 dwell 间隔驱动 RunCurrentPoint，直至完成/停止/错误
