package workflow

import (
	"math"
	"time"

	"cal1604/internal/events"
)

// StabilityAccumulator 用于累计稳定时间。
type StabilityAccumulator struct {
	tolerance        float64
	requiredDuration time.Duration
	accumulated      time.Duration
}

// StabilityStatus 稳定性判定结果，用于 SSE 事件推送。
type StabilityStatus struct {
	IsStable         bool    `json:"isStable"`
	IsInRange        bool    `json:"isInRange"`
	CurrentValue     float64 `json:"currentValue"`
	TargetValue      float64 `json:"targetValue"`
	Deviation        float64 `json:"deviation"`
	StableDurationMs int64   `json:"stableDurationMs"`
	RequiredDurationMs int64 `json:"requiredDurationMs"`
	Progress         int     `json:"progress"` // 0-100
}

// NewStabilityAccumulator 创建稳定累计器。
func NewStabilityAccumulator(tolerance float64, requiredDuration time.Duration) *StabilityAccumulator {
	return &StabilityAccumulator{
		tolerance:        tolerance,
		requiredDuration: requiredDuration,
	}
}

// AddSample 增加一次采样并返回是否达稳以及当前累计时长。
func (a *StabilityAccumulator) AddSample(target, actual float64, interval time.Duration) (bool, time.Duration) {
	if interval < 0 {
		interval = 0
	}

	deviation := math.Abs(actual - target)
	if deviation <= a.tolerance {
		a.accumulated += interval
	} else {
		a.accumulated = 0
	}

	return a.accumulated >= a.requiredDuration, a.accumulated
}

// StabilityEventPublisher 稳定性事件回调，用于推送 SSE 事件。
type StabilityEventPublisher func(eventType string, data any)

// StabilityMonitor 封装 StabilityAccumulator，在状态变化时发布 SSE 事件。
type StabilityMonitor struct {
	accumulator *StabilityAccumulator
	publisher   StabilityEventPublisher
	interval    time.Duration
	wasInRange  bool
	wasStable   bool
}

// NewStabilityMonitor 创建稳定性监控器。
func NewStabilityMonitor(tolerance float64, requiredDuration time.Duration, publisher StabilityEventPublisher) *StabilityMonitor {
	return &StabilityMonitor{
		accumulator: NewStabilityAccumulator(tolerance, requiredDuration),
		publisher:   publisher,
		interval:    200 * time.Millisecond,
		wasInRange:  false,
		wasStable:   false,
	}
}

// FeedSample 输入一次采样，检测状态变化并发布相应 SSE 事件。
// 返回当前 StabilityStatus。
func (m *StabilityMonitor) FeedSample(target, actual float64) StabilityStatus {
	stable, accumulated := m.accumulator.AddSample(target, actual, m.interval)

	deviation := math.Abs(actual - target)
	isInRange := deviation <= m.accumulator.tolerance
	progress := 0
	if m.accumulator.requiredDuration > 0 {
		progress = int(float64(accumulated) / float64(m.accumulator.requiredDuration) * 100)
		if progress > 100 {
			progress = 100
		}
	}

	status := StabilityStatus{
		IsStable:         stable,
		IsInRange:        isInRange,
		CurrentValue:     actual,
		TargetValue:      target,
		Deviation:        deviation,
		StableDurationMs: accumulated.Milliseconds(),
		RequiredDurationMs: m.accumulator.requiredDuration.Milliseconds(),
		Progress:         progress,
	}

	if m.publisher == nil {
		m.wasInRange = isInRange
		m.wasStable = stable
		return status
	}

	// 检测进入/离开稳定范围
	if isInRange && !m.wasInRange {
		m.publisher(events.EventCalibrationStabilityChanged, status)
	}
	if !isInRange && m.wasInRange && !stable {
		m.publisher(events.EventCalibrationStabilityLost, status)
	}

	// 进度更新（在范围内但未达稳）
	if isInRange && !stable {
		m.publisher(events.EventCalibrationStabilityProgress, status)
	}

	// 达到稳定
	if stable && !m.wasStable {
		m.publisher(events.EventCalibrationStabilityAchieved, status)
	}

	m.wasInRange = isInRange
	m.wasStable = stable
	return status
}
