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

// tolerance < 脉冲当量（0.005）→ tolerance warning（电机走一步即越过容差，会振荡）
func TestValidateCompensationToleranceBelowPulseQuantum(t *testing.T) {
	axis := linearAxis(0.001) // scale 0.001，远小于脉冲当量，避免触发 tolerance < scale
	cfg := validComp()
	cfg.Tolerance = 0.003 // < 脉冲当量 0.005
	warns := ValidateCompensationConfig(cfg, axis)
	if !hasWarningField(warns, "tolerance") {
		t.Fatalf("expected tolerance warning, got: %v", warns)
	}
}

// tolerance 等于脉冲当量 → 不报（边界，恰好不过冲）
func TestValidateCompensationToleranceEqualsPulseQuantum(t *testing.T) {
	axis := linearAxis(0.001)
	cfg := validComp()
	cfg.Tolerance = 0.005 // == 脉冲当量 0.005
	warns := ValidateCompensationConfig(cfg, axis)
	// 仅检查无 tolerance<pulseQuantum 的 warning（可能有其他 warning，不在此测试关注范围）
	for _, w := range warns {
		if w.Field == "tolerance" && w.Severity == "warning" {
			t.Fatalf("expected no tolerance<pulseQuantum warning at boundary, got: %v", warns)
		}
	}
}

// tolerance > 脉冲当量 → 不报 tolerance warning
func TestValidateCompensationToleranceAbovePulseQuantum(t *testing.T) {
	axis := linearAxis(0.001)
	cfg := validComp()
	cfg.Tolerance = 0.01 // > 脉冲当量 0.005
	warns := ValidateCompensationConfig(cfg, axis)
	for _, w := range warns {
		if w.Field == "tolerance" && w.Severity == "warning" {
			t.Fatalf("expected no tolerance warning when tolerance > pulseQuantum, got: %v", warns)
		}
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

// 确认 tolerance<pulseQuantum 的告警为 warning 级别（可忽略，非阻断）
func TestValidateCompensationWarningSeverity(t *testing.T) {
	axis := linearAxis(0.001) // scale 0.001，避免触发 tolerance < scale
	cfg := validComp()
	cfg.Tolerance = 0.003 // < 脉冲当量 0.005，触发 warning
	warns := ValidateCompensationConfig(cfg, axis)
	found := false
	for _, w := range warns {
		if w.Field == "tolerance" && w.Severity == "warning" {
			found = true
		}
		if w.Field == "tolerance" && w.Severity != "warning" {
			t.Fatalf("tolerance<pulseQuantum should be warning severity, got %s", w.Severity)
		}
	}
	if !found {
		t.Fatalf("expected tolerance warning, got: %v", warns)
	}
}
