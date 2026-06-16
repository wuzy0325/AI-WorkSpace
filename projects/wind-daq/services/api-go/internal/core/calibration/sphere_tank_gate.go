package calibration

import "fmt"

// NormalizeSphereTankGateConfig 标准化球罐闸门配置
// 如果配置为空或未启用，返回 nil
func NormalizeSphereTankGateConfig(config Config) *SphereTankGateConfig {
	if config.SphereTankGate == nil {
		return nil
	}
	return normalizeGate(config.SphereTankGate)
}

// NormalizeSphereTankGate 标准化球罐闸门配置
func NormalizeSphereTankGate(gate *SphereTankGateConfig) *SphereTankGateConfig {
	if gate == nil {
		return nil
	}
	return normalizeGate(gate)
}

// normalizeGate 内部标准化函数
func normalizeGate(gate *SphereTankGateConfig) *SphereTankGateConfig {
	waitTimeSec := gate.WaitTimeSec
	if waitTimeSec < 0 {
		waitTimeSec = 0
	}

	channelIndex := gate.StableTimeChannel.ChannelIndex

	return &SphereTankGateConfig{
		Enabled:     gate.Enabled,
		WaitTimeSec: waitTimeSec,
		StableTimeChannel: ChannelRef{
			DeviceID:     gate.StableTimeChannel.DeviceID,
			ChannelIndex: channelIndex,
		},
	}
}

// ParseSphereTankStableTimeSec 解析球罐稳定时间（秒）
// 从通道读取的原始值转换为稳定时间
func ParseSphereTankStableTimeSec(value float64, ok bool) (float64, error) {
	if !ok {
		return 0, fmt.Errorf("球罐稳定时间通道读取失败")
	}
	if value < 0 {
		return 0, fmt.Errorf("球罐稳定时间不能为负数: %f", value)
	}
	return value, nil
}

// IsSphereTankGateSatisfied 判断球罐闸门条件是否满足
func IsSphereTankGateSatisfied(gate *SphereTankGateConfig, stableTimeSec float64) bool {
	if gate == nil || !gate.Enabled {
		return true
	}
	return stableTimeSec >= gate.WaitTimeSec
}

// ValidateSphereTankGateConfig 验证球罐闸门配置
func ValidateSphereTankGateConfig(gate *SphereTankGateConfig) error {
	if gate == nil || !gate.Enabled {
		return nil
	}
	if gate.StableTimeChannel.DeviceID == "" {
		return fmt.Errorf("球罐判定已启用，但稳定时间通道设备ID未配置")
	}
	if gate.StableTimeChannel.ChannelIndex < 0 {
		return fmt.Errorf("球罐判定已启用，但稳定时间通道索引无效")
	}
	return nil
}
