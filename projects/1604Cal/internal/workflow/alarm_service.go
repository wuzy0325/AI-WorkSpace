package workflow

import (
	"fmt"
	"math"

	"cal1604/internal/domain"
)

const (
	// AlarmDecisionContinue 继续采集后续流程。
	AlarmDecisionContinue = "continue"
	// AlarmDecisionSkip 跳过当前压力点。
	AlarmDecisionSkip = "skip"
	// AlarmDecisionRecollect 重新采集当前压力点。
	AlarmDecisionRecollect = "recollect"
	// AlarmDecisionStop 停止自动采集流程。
	AlarmDecisionStop = "stop"
)

// AlarmResult 表示一次报警判定结果。
type AlarmResult struct {
	Triggered        bool
	Deviation        float64
	DeviationPercent float64
	Allowance        float64
}

// MultiChannelAlarmResult 多通道报警判定结果。
type MultiChannelAlarmResult struct {
	Triggered         bool            `json:"triggered"`
	OverLimitChannels []int           `json:"overLimitChannels"`
	MaxDeviation      float64         `json:"maxDeviation"`
	ChannelDetails    map[int]float64 `json:"channelDetails"` // channelIndex -> deviation
}

// AlarmService 负责计算精度超限并校验处置动作。
type AlarmService struct{}

// NewAlarmService 创建报警规则服务。
func NewAlarmService() *AlarmService {
	return &AlarmService{}
}

// Evaluate 根据目标值、实测值和百分比阈值计算报警结果。
func (s *AlarmService) Evaluate(target, actual, levelPercent float64) AlarmResult {
	deviation := math.Abs(actual - target)
	allowance := math.Abs(target) * levelPercent / 100

	deviationPercent := 0.0
	if target != 0 {
		deviationPercent = deviation / math.Abs(target) * 100
	}

	return AlarmResult{
		Triggered:        deviation > allowance,
		Deviation:        deviation,
		DeviationPercent: deviationPercent,
		Allowance:        allowance,
	}
}

// EvaluateMultiChannel 多通道报警判定。
// channelData: channelIndex -> measured value
// 返回超过限值的通道列表、最大偏差和是否触发报警。
func (s *AlarmService) EvaluateMultiChannel(alarmConfig domain.AlarmConfig, target float64, maxPressure float64, minPressure float64, channelData map[int]float64) MultiChannelAlarmResult {
	if !alarmConfig.Enabled {
		return MultiChannelAlarmResult{}
	}

	// 量程引用误差：(maxPressure - minPressure) × precisionThreshold
	span := maxPressure - minPressure
	if span < 0 {
		span = -span
	}
	allowance := span * alarmConfig.PrecisionThreshold
	if allowance < 1e-10 {
		allowance = math.Abs(target) * alarmConfig.PrecisionThreshold
	}

	result := MultiChannelAlarmResult{
		ChannelDetails: make(map[int]float64),
	}

	for _, ch := range alarmConfig.EnabledChannels {
		val, ok := channelData[ch]
		if !ok {
			continue
		}
		deviation := math.Abs(val - target)
		result.ChannelDetails[ch] = deviation

		if deviation > allowance {
			result.OverLimitChannels = append(result.OverLimitChannels, ch)
			if deviation > result.MaxDeviation {
				result.MaxDeviation = deviation
			}
		}
	}

	result.Triggered = len(result.OverLimitChannels) > 0
	return result
}

// ValidateDecision 校验报警后的用户决策动作是否合法。
func (s *AlarmService) ValidateDecision(decision string) error {
	switch decision {
	case AlarmDecisionContinue, AlarmDecisionSkip, AlarmDecisionRecollect, AlarmDecisionStop:
		return nil
	default:
		return fmt.Errorf("invalid alarm decision: %s", decision)
	}
}
