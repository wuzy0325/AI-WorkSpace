package backend

import (
	three_interp "ai-workspace/shared/algorithms/go/threehole/interpolation"
	"testing"
)

func TestToCoreInputGauge(t *testing.T) {
	input := InterpolationInput{
		P1: 100000, P2: 101000, P3: 102000,
		Patm: 101325, Tatm: 20,
		PressureMode: "gauge",
	}

	core := toCoreInput(input)
	if core.P1 != 100000 || core.P2 != 101000 || core.P3 != 102000 {
		t.Errorf("Gauge mode should pass through values: got %f, %f, %f", core.P1, core.P2, core.P3)
	}
}

func TestToCoreInputAbsolute(t *testing.T) {
	input := InterpolationInput{
		P1: 201325, P2: 202325, P3: 203325,
		Patm: 101325, Tatm: 20,
		PressureMode: "absolute",
	}

	core := toCoreInput(input)
	if core.P1 != 100000 || core.P2 != 101000 || core.P3 != 102000 {
		t.Errorf("Absolute mode should subtract Patm: got %f, %f, %f", core.P1, core.P2, core.P3)
	}
}

func TestToCoreInputDefaultGauge(t *testing.T) {
	input := InterpolationInput{
		P1: 100000, P2: 101000, P3: 102000,
		Patm: 101325, Tatm: 20,
	}

	core := toCoreInput(input)
	if core.P1 != 100000 || core.P2 != 101000 || core.P3 != 102000 {
		t.Errorf("Default mode should be gauge: got %f, %f, %f", core.P1, core.P2, core.P3)
	}
}

func TestToAppResult(t *testing.T) {
	core := three_interp.InterpolationResult{
		Alpha:          5.0,
		MachNumber:     0.5,
		TotalPressure:  150000,
		StaticPressure: 100000,
		IterationCount: 5,
		IsValid:        true,
	}

	r := toAppResult(core)
	if r.Alpha != 5.0 {
		t.Errorf("Alpha = %f, want 5.0", r.Alpha)
	}
	if r.MachNumber != 0.5 {
		t.Errorf("MachNumber = %f, want 0.5", r.MachNumber)
	}
	if !r.IsValid {
		t.Error("IsValid should be true")
	}
	if r.IterationCount != 5 {
		t.Errorf("IterationCount = %d, want 5", r.IterationCount)
	}
}
