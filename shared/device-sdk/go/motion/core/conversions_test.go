package core

import (
	"testing"
)

// linearAxis 直线轴：步距角 1.8°, 细分 4, 导程 4mm。
// ppu = (360/1.8)*4/4 = 200 脉冲/mm，脉冲当量 = 0.005 mm/脉冲。
func linearAxis(scale float64) AxisConfig {
	return AxisConfig{
		Name:           AxisX,
		Enabled:        true,
		Kind:           AxisKindLinear,
		StepsPerRev:    PtrFloat64(1.8),
		MicroSteps:     PtrInt(4),
		Lead:           PtrFloat64(4),
		PositionSource: PositionSourceEncoder,
		EncoderScale:   &scale,
	}
}

func validComp() AxisEncoderCompensationConfig {
	return AxisEncoderCompensationConfig{
		Enabled:   true,
		Tolerance: 0.01,
		MaxCycles: 3,
		SettleMs:  100,
		MinStep:   0.001,
		TimeoutMs: 5000,
	}
}

func hasWarningField(warns []CompensationWarning, field string) bool {
	for _, w := range warns {
		if w.Field == field {
			return true
		}
	}
	return false
}

// tolerance < encoderScale → error
func TestValidateCompensationToleranceBelowScale(t *testing.T) {
	axis := linearAxis(0.01) // scale 0.01
	cfg := validComp()
	cfg.Tolerance = 0.005 // < scale
	warns := ValidateCompensationConfig(cfg, axis)
	found := false
	for _, w := range warns {
		if w.Field == "tolerance" && w.Severity == "error" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected tolerance error, got: %v", warns)
	}
}

// encoderScale 粗于脉冲当量（0.005）→ encoderScale warning
func TestValidateCompensationScaleCoarserThanPulseQuantum(t *testing.T) {
	axis := linearAxis(0.02) // scale 0.02 > 脉冲当量 0.005
	cfg := validComp()
	cfg.Tolerance = 0.03 // > scale，不触发 tolerance error
	warns := ValidateCompensationConfig(cfg, axis)
	if !hasWarningField(warns, "encoderScale") {
		t.Fatalf("expected encoderScale warning, got: %v", warns)
	}
}

// encoderScale 等于脉冲当量 → 不报（边界，恰好可分辨）
func TestValidateCompensationScaleEqualsPulseQuantum(t *testing.T) {
	axis := linearAxis(0.005) // scale == 脉冲当量 0.005
	cfg := validComp()
	cfg.Tolerance = 0.01
	warns := ValidateCompensationConfig(cfg, axis)
	if hasWarningField(warns, "encoderScale") {
		t.Fatalf("expected no encoderScale warning at boundary, got: %v", warns)
	}
}

// encoderScale 优于脉冲当量 → 不报
func TestValidateCompensationScaleFinerThanPulseQuantum(t *testing.T) {
	axis := linearAxis(0.001) // scale 0.001 < 脉冲当量 0.005
	cfg := validComp()
	cfg.Tolerance = 0.002
	warns := ValidateCompensationConfig(cfg, axis)
	if hasWarningField(warns, "encoderScale") {
		t.Fatalf("expected no encoderScale warning for fine scale, got: %v", warns)
	}
}

// 禁用时无任何告警
func TestValidateCompensationDisabledNoWarnings(t *testing.T) {
	axis := linearAxis(0.02)
	cfg := validComp()
	cfg.Enabled = false
	warns := ValidateCompensationConfig(cfg, axis)
	if len(warns) > 0 {
		t.Fatalf("expected no warnings when disabled, got: %v", warns)
	}
}

// 确认 error 级告警在阻断路径（UpsertProfile）中被检查（间接：severity 字段）
func TestValidateCompensationWarningSeverity(t *testing.T) {
	axis := linearAxis(0.02)
	cfg := validComp()
	cfg.Tolerance = 0.03
	warns := ValidateCompensationConfig(cfg, axis)
	for _, w := range warns {
		if w.Field == "encoderScale" && w.Severity != "warning" {
			t.Fatalf("encoderScale should be warning severity, got %s", w.Severity)
		}
	}
}
