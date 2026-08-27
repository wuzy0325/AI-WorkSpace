package domain

// AlarmConfig 报警配置，控制精度超限的判定与响应行为。
// 标定与计量流程统一使用量程引用误差（FS%）：
//   允许偏差 = (MaxPressure - MinPressure) × PrecisionThreshold
// 当量程为 0 时降级为按目标值比例计算。
type AlarmConfig struct {
	Enabled            bool    `json:"enabled"`
	PrecisionThreshold float64 `json:"precisionThreshold"` // 满量程百分比（标定模块使用）
	SoundEnabled       bool    `json:"soundEnabled"`
	ConfirmOnAlarm     bool    `json:"confirmOnAlarm"`
	EnabledChannels    []int   `json:"enabledChannels"`
}
