package core

import (
	"fmt"
	"math"
)

const (
	DefaultStepDeg   = 1.8
	DefaultScale     = 0.005
	DefaultMaxSpeed  = 10.0
	DefaultLead      = 1.0
	DefaultGearRatio = 1.0
	DefaultMicroStep = 1
)

// 编码器补偿默认参数。
// 与参考实现 (Cursor DAQ constants.ts) 对齐，确保跨项目行为一致。
const (
	// DefaultEncoderCompensationTolerance 补偿容差（工程单位）。
	// 误差绝对值 <= tolerance 即视为补偿到位。
	DefaultEncoderCompensationTolerance = 0.01

	// DefaultEncoderCompensationMaxCycles 最大补偿循环次数。
	// 取 3 而非 10：保守策略，避免追尾/振荡；如遇精度不足由用户在 UI 上调。
	DefaultEncoderCompensationMaxCycles = 3

	// DefaultEncoderCompensationSettleMs 运动停止后等待机械震荡衰减的稳定时间（毫秒）。
	DefaultEncoderCompensationSettleMs = 100

	// DefaultEncoderCompensationMinStep 单次补偿最小步长（工程单位）。
	// 误差 < minStep 时不再补偿，避免无穷小步进导致振荡。
	DefaultEncoderCompensationMinStep = 0.001

	// DefaultEncoderCompensationTimeoutMs 单次补偿任务总超时（毫秒）。
	// 超时后任务标记为失败，避免长时间阻塞状态轮询。
	DefaultEncoderCompensationTimeoutMs = 5000
)

// DefaultEncoderCompensationConfig 返回带默认值的编码器补偿配置。
// 当 profile 未显式配置时调用，避免零值导致补偿逻辑行为异常。
func DefaultEncoderCompensationConfig() AxisEncoderCompensationConfig {
	return AxisEncoderCompensationConfig{
		Enabled:   false, // 项目默认关闭：避免静默改变现有运动语义，需用户显式开启
		Tolerance: DefaultEncoderCompensationTolerance,
		MaxCycles: DefaultEncoderCompensationMaxCycles,
		SettleMs:  DefaultEncoderCompensationSettleMs,
		MinStep:   DefaultEncoderCompensationMinStep,
		TimeoutMs: DefaultEncoderCompensationTimeoutMs,
	}
}

// ResolveEncoderCompensation 返回解析后的补偿配置。
// 若 profile 显式提供了配置则用 profile 值，否则返回默认配置。
// 始终返回非 nil，简化调用方判空。
func ResolveEncoderCompensation(cfg *AxisEncoderCompensationConfig) AxisEncoderCompensationConfig {
	if cfg == nil {
		return DefaultEncoderCompensationConfig()
	}
	resolved := *cfg
	if resolved.MaxCycles <= 0 {
		resolved.MaxCycles = DefaultEncoderCompensationMaxCycles
	}
	if resolved.SettleMs <= 0 {
		resolved.SettleMs = DefaultEncoderCompensationSettleMs
	}
	if resolved.TimeoutMs <= 0 {
		resolved.TimeoutMs = DefaultEncoderCompensationTimeoutMs
	}
	if resolved.Tolerance <= 0 {
		resolved.Tolerance = DefaultEncoderCompensationTolerance
	}
	if resolved.MinStep <= 0 {
		resolved.MinStep = DefaultEncoderCompensationMinStep
	}
	return resolved
}

// CompensationWarning 参数校验告警/错误。
type CompensationWarning struct {
	Field    string // 参数名，如 "tolerance"
	Message  string
	Severity string // "error"（不可忽略）或 "warning"（可忽略）
}

// ValidateCompensationConfig 校验编码器补偿参数与轴配置的物理合理性。
// 返回的切片为空表示全部合理。调用方应在 UI 中展示每条告警。
func ValidateCompensationConfig(cfg AxisEncoderCompensationConfig, axisCfg AxisConfig) []CompensationWarning {
	if !cfg.Enabled {
		return nil
	}
	var warns []CompensationWarning

	scale := ValueOrFloat(axisCfg.EncoderScale, DefaultScale)
	ppu := PulsesPerUnit(axisCfg)

	// tolerance < 编码器分辨率 => 永远无法收敛
	if cfg.Tolerance < scale {
		warns = append(warns, CompensationWarning{
			Field:    "tolerance",
			Severity: "error",
			Message:  fmt.Sprintf("容差(%.4f)小于编码器分辨率(%.4f)，误差永远无法降到容差以下", cfg.Tolerance, scale),
		})
	}

	// 编码器分辨率粗于电机脉冲当量 => 编码器无法分辨最小步进，补偿盲区大、精度受限。
	// 脉冲当量 = 1 / PulsesPerUnit（每脉冲工程位移）。
	if ppu > 0 {
		pulseQuantum := 1.0 / ppu
		if scale > pulseQuantum {
			warns = append(warns, CompensationWarning{
				Field:    "encoderScale",
				Severity: "warning",
				Message:  fmt.Sprintf("编码器分辨率(%.6f)粗于脉冲当量(%.6f)，电机最小步进无法被编码器分辨，补偿精度受限", scale, pulseQuantum),
			})
		}
	}

	// minStep → 0 脉冲 => 修正无效果
	if cfg.MinStep > 0 {
		minPulse := EngineeringToPulse(axisCfg, cfg.MinStep)
		if minPulse == 0 {
			warns = append(warns, CompensationWarning{
				Field:    "minStep",
				Severity: "warning",
				Message:  fmt.Sprintf("最小步长(%.4f)对应的脉冲数为 0（脉冲当量 %.2f 脉冲/单位），一次修正不会产生任何移动", cfg.MinStep, ppu),
			})
		}
	}

	// minStep >= tolerance => 修正可能过头，导致振荡
	if cfg.MinStep >= cfg.Tolerance {
		warns = append(warns, CompensationWarning{
			Field:    "minStep",
			Severity: "warning",
			Message:  fmt.Sprintf("最小步长(%.4f) >= 容差(%.4f)，修正可能过头导致在目标两侧反复", cfg.MinStep, cfg.Tolerance),
		})
	}

	// timeout 不够跑完 maxCycles
	if cfg.MaxCycles > 0 && cfg.SettleMs > 0 && cfg.TimeoutMs > 0 {
		// 保守估计：单次循环 = settle + 50ms（poll + 命令开销）
		cycleEstimate := cfg.SettleMs + 50
		needed := cfg.MaxCycles * cycleEstimate
		if cfg.TimeoutMs < needed {
			warns = append(warns, CompensationWarning{
				Field:    "timeoutMs",
				Severity: "warning",
				Message:  fmt.Sprintf("超时(%dms)可能不够跑完 %d 次循环(估计需 %dms = %d×(settle %dms + 50ms))", cfg.TimeoutMs, cfg.MaxCycles, needed, cfg.MaxCycles, cfg.SettleMs),
			})
		}
	}

	return warns
}

func ClampInt64(value, min, max int64) int64 {
	if value < min {
		return min
	}
	if value > max {
		return max
	}
	return value
}

// PulsesPerUnit returns pulses per engineering unit (mm for LINEAR, ° for ROTARY).
// stepsPerRev is the step angle in degrees (e.g. 1.8°); 360/stepAngleDeg = steps per full revolution.
func PulsesPerUnit(axisCfg AxisConfig) float64 {
	stepAngleDeg := ValueOrFloat(axisCfg.StepsPerRev, DefaultStepDeg)
	if stepAngleDeg == 0 {
		stepAngleDeg = DefaultStepDeg
	}
	microSteps := ValueOrInt(axisCfg.MicroSteps, DefaultMicroStep)
	stepsPerRev := 360 / stepAngleDeg
	if axisCfg.Kind == AxisKindRotary {
		gearRatio := ValueOrFloat(axisCfg.GearRatio, DefaultGearRatio)
		if gearRatio == 0 {
			gearRatio = DefaultGearRatio
		}
		return (stepsPerRev * float64(microSteps) * gearRatio) / 360
	}
	lead := ValueOrFloat(axisCfg.Lead, DefaultLead)
	if lead == 0 {
		lead = DefaultLead
	}
	return (stepsPerRev * float64(microSteps)) / lead
}

func EngineeringToPulse(axisCfg AxisConfig, value float64) int64 {
	return int64(math.Round(value * PulsesPerUnit(axisCfg)))
}

func PulseToEngineering(axisCfg AxisConfig, pulse float64) float64 {
	ppu := PulsesPerUnit(axisCfg)
	if ppu == 0 {
		return 0
	}
	return pulse / ppu
}

func EngineeringToEncoderCount(axisCfg AxisConfig, value float64) int64 {
	scale := ValueOrFloat(axisCfg.EncoderScale, DefaultScale)
	if scale == 0 {
		return 0
	}
	return int64(math.Round(value / scale))
}

func EncoderCountToEngineering(axisCfg AxisConfig, count float64) float64 {
	return count * ValueOrFloat(axisCfg.EncoderScale, DefaultScale)
}

func ValueOrFloat(value *float64, fallback float64) float64 {
	if value == nil {
		return fallback
	}
	return *value
}

func ValueOrInt(value *int, fallback int) int {
	if value == nil {
		return fallback
	}
	return *value
}

// homedThreshold tolerance for "at home" position (engineering units).
// 0.01 mm for linear axes, 0.01° for rotary axes. Mechanical homing
// typically settles within this range.
const homedThreshold = 0.01

func IsHomed(position float64, axisCfg AxisConfig) bool {
	if math.Abs(position) >= homedThreshold {
		return false
	}
	if axisCfg.MinLimit != nil && position < *axisCfg.MinLimit {
		return false
	}
	if axisCfg.MaxLimit != nil && position > *axisCfg.MaxLimit {
		return false
	}
	return true
}
