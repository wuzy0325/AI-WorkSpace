package core

import "math"

const (
	DefaultStepDeg   = 1.8
	DefaultScale     = 0.005
	DefaultMaxSpeed  = 10.0
	DefaultLead      = 1.0
	DefaultGearRatio = 1.0
	DefaultMicroStep = 1
)

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
