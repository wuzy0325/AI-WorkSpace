package interpolation

import (
	"fmt"
	"math"
	"strings"
	"testing"
)

func TestFiveHoleNewInterpolatorFindsOriginalGridCell(t *testing.T) {
	interpolator := NewFiveHoleNewInterpolator()
	if err := interpolator.LoadPrbLines(syntheticFiveHoleCsv([]float64{-20, 0, 20}, []float64{-20, 0, 20})); err != nil {
		t.Fatalf("LoadPrbLines returned error: %v", err)
	}

	result, err := interpolator.Calculate(inputForAngles(10, 10))
	if err != nil {
		t.Fatalf("Calculate returned error: %v", err)
	}
	if !result.IsValid {
		t.Fatalf("expected valid interpolation, got warning %q", result.Warning)
	}
	assertNear(t, "alpha", result.Alpha, 10, 0.25)
	assertNear(t, "beta", result.Beta, 10, 0.25)
}

func TestFiveHoleNewInterpolatorUsesExtendedGrid(t *testing.T) {
	interpolator := NewFiveHoleNewInterpolator()
	if err := interpolator.LoadPrbLines(syntheticFiveHoleCsv([]float64{-20, 0, 20}, []float64{-20, 0, 20})); err != nil {
		t.Fatalf("LoadPrbLines returned error: %v", err)
	}

	result, err := interpolator.Calculate(inputForAngles(30, 30))
	if err != nil {
		t.Fatalf("Calculate returned error: %v", err)
	}
	if !result.IsValid {
		t.Fatalf("expected valid extended interpolation, got warning %q", result.Warning)
	}
	if result.Alpha <= 20 || result.Beta <= 20 {
		t.Fatalf("expected extended interpolation beyond original grid, got alpha=%v beta=%v", result.Alpha, result.Beta)
	}
	if result.Warning == "" {
		t.Fatalf("expected extended-grid warning")
	}
}

func TestFiveHoleNewInterpolatorRejectsIncompleteGrid(t *testing.T) {
	interpolator := NewFiveHoleNewInterpolator()
	err := interpolator.LoadPrbLines([]string{
		"alpha,beta,p1,p2,p3,p4,p5",
		"-10,-10,90,110,110,90,110",
		"-10,10,110,110,90,90,110",
		"10,-10,90,110,110,110,90",
	})
	if err == nil {
		t.Fatalf("expected incomplete grid error")
	}
}

func TestMultiPrbLinearKeepsCasSatAndMachRange(t *testing.T) {
	low := NewPrbInterpolator()
	if err := low.LoadPrbLines(syntheticPrbLines(0.05, 0.01), "low.prb"); err != nil {
		t.Fatalf("load low PRB: %v", err)
	}
	high := NewPrbInterpolator()
	if err := high.LoadPrbLines(syntheticPrbLines(0.15, 0.02), "high.prb"); err != nil {
		t.Fatalf("load high PRB: %v", err)
	}
	multi := &MultiPrbInterpolator{
		prbFiles: []prbFileWithInterpolator{
			{MachNumber: 0.2, FileInfo: PrbFileInfo{ValidRange: PrbValidRange{MachMin: 0.2, MachMax: 0.2}}, Interpolator: low},
			{MachNumber: 0.4, FileInfo: PrbFileInfo{ValidRange: PrbValidRange{MachMin: 0.4, MachMax: 0.4}}, Interpolator: high},
		},
		sortedMachNumbers: []float64{0.2, 0.4},
		loaded:            true,
		mode:              ModeLinear,
	}

	result, err := multi.calculateWithLinear(0.3, InterpolationInput{
		P1: 100, P2: 300, P3: 100, P4: 100, P5: 100,
		PAtm: 101325, TAtm: 20,
	})
	if err != nil {
		t.Fatalf("calculateWithLinear returned error: %v", err)
	}
	if result.CAS <= 0 {
		t.Fatalf("expected interpolated CAS to be preserved, got %v", result.CAS)
	}
	if result.SAT <= 0 {
		t.Fatalf("expected interpolated SAT to be preserved, got %v", result.SAT)
	}
	if !strings.Contains(result.Warning, "Ma=0.300") {
		t.Fatalf("expected linear interpolation warning, got %q", result.Warning)
	}

	validRange := multi.GetValidRange()
	assertNear(t, "MachMin", validRange.MachMin, 0.2, 1e-12)
	assertNear(t, "MachMax", validRange.MachMax, 0.4, 1e-12)
}

func syntheticFiveHoleCsv(alphas, betas []float64) []string {
	lines := []string{"alpha,beta,p1,p2,p3,p4,p5"}
	for _, alpha := range alphas {
		for _, beta := range betas {
			lines = append(lines, fmt.Sprintf("%.0f,%.0f,%.6f,%.6f,%.6f,%.6f,%.6f",
				alpha, beta, 100-beta, 200.0, 100+beta, 100+alpha, 100-alpha))
		}
	}
	return lines
}

func inputForAngles(alpha, beta float64) InterpolationInput {
	return InterpolationInput{
		P1: 100 - beta,
		P2: 200,
		P3: 100 + beta,
		P4: 100 + alpha,
		P5: 100 - alpha,
	}
}

func syntheticPrbLines(cpt, cps float64) []string {
	lines := []string{"13 13"}
	for alpha := -30.0; alpha <= 30; alpha += 5 {
		for beta := -30.0; beta <= 30; beta += 5 {
			lines = append(lines, fmt.Sprintf("%.6f %.6f %.6f %.6f %.0f %.0f",
				alpha/100, beta/100, cpt, cps, alpha, beta))
		}
	}
	return lines
}

func assertNear(t *testing.T, name string, got, want, tolerance float64) {
	t.Helper()
	if math.Abs(got-want) > tolerance {
		t.Fatalf("%s = %v, want %v +/- %v", name, got, want, tolerance)
	}
}
