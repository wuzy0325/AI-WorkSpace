package interpolation

import (
	"math"
	"strconv"
	"testing"
)

func makePrbLines(cma float64, items ...string) []string {
	lines := []string{formatFloat(cma), "4"}
	lines = append(lines, items...)
	return lines
}

func formatFloat(v float64) string {
	return strconv.FormatFloat(v, 'f', -1, 64)
}

func TestParsePrbLines(t *testing.T) {
	lines := []string{
		"0.4",
		"13",
		"-1.736225162  0.405256232  1.144794741  -30",
		"-1.246677624  0.200787637  0.913957869  -25",
		"0.0 0.0 0.0 0",
		"1.246677624  0.200787637  0.913957869  25",
		"1.736225162  0.405256232  1.144794741  30",
		"0.0 0.0 0.0 0",
		"0.0 0.0 0.0 0",
		"0.0 0.0 0.0 0",
		"0.0 0.0 0.0 0",
		"0.0 0.0 0.0 0",
		"0.0 0.0 0.0 0",
		"0.0 0.0 0.0 0",
		"0.0 0.0 0.0 0",
	}

	cal, err := parsePrbLines(lines)
	if err != nil {
		t.Fatalf("parsePrbLines failed: %v", err)
	}

	if cal.CMa != 0.4 {
		t.Errorf("CMa = %f, want 0.4", cal.CMa)
	}
	if cal.Nalpha != 13 {
		t.Errorf("Nalpha = %d, want 13", cal.Nalpha)
	}
	if len(cal.Items) != 13 {
		t.Errorf("Items = %d, want 13", len(cal.Items))
	}
	if cal.Items[0].Kb != -1.736225162 {
		t.Errorf("Items[0].Kb = %f", cal.Items[0].Kb)
	}
}

func TestInterpolator_LoadAndCalculate(t *testing.T) {
	lines1 := []string{
		"0.2",
		"5",
		"-1.5 0.3 0.9 -20",
		"-0.8 0.2 0.5 -10",
		"0.0 0.0 0.0 0",
		"0.8 0.2 0.5 10",
		"1.5 0.3 0.9 20",
	}
	lines2 := []string{
		"0.6",
		"5",
		"-1.5 0.3 0.9 -20",
		"-0.8 0.2 0.5 -10",
		"0.0 0.0 0.0 0",
		"0.8 0.2 0.5 10",
		"1.5 0.3 0.9 20",
	}

	interp := NewThreeHoleInterpolator()
	result, err := interp.LoadPrbData([]PrbFileData{
		{FilePath: "test_0.2.prb", Lines: lines1},
		{FilePath: "test_0.6.prb", Lines: lines2},
	})
	if err != nil {
		t.Fatalf("LoadPrbData failed: %v", err)
	}

	if !interp.IsLoaded() {
		t.Fatal("interpolator should be loaded")
	}

	if len(result.Files) != 2 {
		t.Errorf("Files = %d, want 2", len(result.Files))
	}

	minMa, maxMa := interp.GetMachRange()
	if minMa != 0.2 || maxMa != 0.6 {
		t.Errorf("MachRange = [%f, %f], want [0.2, 0.6]", minMa, maxMa)
	}

	input := InterpolationInput{
		P1: 98000, P2: 101000, P3: 102000,
		PAtm: 101325, TAtm: 20,
	}

	res, err := interp.Calculate(input)
	if err != nil {
		t.Fatalf("Calculate failed: %v", err)
	}

	t.Logf("Result: Alpha=%f, Mach=%f, Pt=%f, Ps=%f, Iterations=%d, Warning=%q",
		res.Alpha, res.MachNumber, res.TotalPressure, res.StaticPressure, res.IterationCount, res.Warning)

	if res.MachNumber < 0 || res.MachNumber > 2 {
		t.Errorf("MachNumber out of range: %f", res.MachNumber)
	}
	if res.IterationCount <= 0 {
		t.Errorf("IterationCount should be > 0, got %d", res.IterationCount)
	}
}

func TestCalcMach(t *testing.T) {
	tests := []struct {
		pt, ps, pa float64
		wantMin, wantMax float64
	}{
		{0, 0, 101325, 0, 0},
		{50000, 40000, 101325, 0, 2},
	}

	for _, tt := range tests {
		mach := calcMach(tt.pt, tt.ps, tt.pa)
		if mach < tt.wantMin || mach > tt.wantMax {
			t.Errorf("calcMach(%f, %f, %f) = %f, want in [%f, %f]",
				tt.pt, tt.ps, tt.pa, mach, tt.wantMin, tt.wantMax)
		}
	}
}

func TestDeltaPNearZero(t *testing.T) {
	lines := []string{
		"0.4",
		"2",
		"-1.0 0.5 0.8 -10",
		"1.0 0.5 0.8 10",
	}

	interp := NewThreeHoleInterpolator()
	_, err := interp.LoadPrbData([]PrbFileData{
		{FilePath: "test.prb", Lines: lines},
	})
	if err != nil {
		t.Fatalf("LoadPrbData failed: %v", err)
	}

	input := InterpolationInput{
		P1: 100000, P2: 100000, P3: 100000,
		PAtm: 101325, TAtm: 20,
	}

	res, err := interp.Calculate(input)
	if err != nil {
		t.Fatalf("Calculate failed: %v", err)
	}
	if res.TotalPressure != 100000 || res.StaticPressure != 100000 {
		t.Errorf("Expected fallback values P2=%f, got Pt=%f Ps=%f", 100000.0, res.TotalPressure, res.StaticPressure)
	}
}

func TestLoadSingleFile(t *testing.T) {
	lines := []string{
		"0.4",
		"3",
		"-1.0 0.5 0.8 -10",
		"0.0 0.0 0.0 0",
		"1.0 0.5 0.8 10",
	}

	interp := NewThreeHoleInterpolator()
	_, err := interp.LoadPrbData([]PrbFileData{
		{FilePath: "single.prb", Lines: lines},
	})
	if err != nil {
		t.Fatalf("LoadPrbData failed: %v", err)
	}

	input := InterpolationInput{
		P1: 99000, P2: 100500, P3: 102000,
		PAtm: 101325, TAtm: 20,
	}

	res, err := interp.Calculate(input)
	if err != nil {
		t.Fatalf("Calculate failed: %v", err)
	}
	if !res.IsValid && math.Abs(res.MachNumber) < 1e-10 {
		t.Logf("Single file: Alpha=%f Mach=%f", res.Alpha, res.MachNumber)
	}
}

func TestInterpolate_ExactMatch(t *testing.T) {
	lines := []string{
		"0.4",
		"3",
		"-1.0 0.5 0.8 -10",
		"0.0 0.3 0.4 0",
		"1.0 0.5 0.8 10",
	}

	interp := NewThreeHoleInterpolator()
	_, err := interp.LoadPrbData([]PrbFileData{
		{FilePath: "exact.prb", Lines: lines},
	})
	if err != nil {
		t.Fatalf("LoadPrbData failed: %v", err)
	}

	minMa, maxMa := interp.GetMachRange()
	if minMa != 0.4 || maxMa != 0.4 {
		t.Errorf("Single file range: [%f, %f]", minMa, maxMa)
	}

	input := InterpolationInput{
		P1: 100000, P2: 100000, P3: 100000,
		PAtm: 101325, TAtm: 20,
	}
	res, err := interp.Calculate(input)
	if err != nil {
		t.Fatalf("Calculate failed: %v", err)
	}
	_ = res
}

func TestExtrapolationWarning(t *testing.T) {
	lines := []string{
		"0.4",
		"3",
		"-1.0 0.5 0.8 -10",
		"0.0 0.3 0.4 0",
		"1.0 0.5 0.8 10",
	}

	interp := NewThreeHoleInterpolator()
	_, err := interp.LoadPrbData([]PrbFileData{
		{FilePath: "exact.prb", Lines: lines},
	})
	if err != nil {
		t.Fatalf("LoadPrbData failed: %v", err)
	}

	input := InterpolationInput{
		P1: 50000, P2: 101000, P3: 150000,
		PAtm: 101325, TAtm: 20,
	}
	res, err := interp.Calculate(input)
	if err != nil {
		t.Fatalf("Calculate failed: %v", err)
	}
	
	t.Logf("Extrapolation test: Alpha=%f Mach=%f IsValid=%t Warning=%q", 
		res.Alpha, res.MachNumber, res.IsValid, res.Warning)
	
	if !res.IsValid {
		t.Error("Extrapolation should not mark result as invalid")
	}
}

func TestNegativePressure(t *testing.T) {
	lines := []string{
		"0.4",
		"2",
		"-1.0 0.5 0.8 -10",
		"1.0 0.5 0.8 10",
	}

	interp := NewThreeHoleInterpolator()
	_, err := interp.LoadPrbData([]PrbFileData{
		{FilePath: "test.prb", Lines: lines},
	})
	if err != nil {
		t.Fatalf("LoadPrbData failed: %v", err)
	}

	// 负探测孔压力是允许的（差压测量），不应该返回错误
	input := InterpolationInput{
		P1:   -100,
		P2:   100000,
		P3:   100000,
		PAtm: 101325,
		TAtm: 20,
	}

	_, err = interp.Calculate(input)
	if err != nil {
		t.Errorf("Probe hole negative pressure should be allowed: %v", err)
	}
}

func TestNegativeAtmosphericPressure(t *testing.T) {
	lines := []string{
		"0.4",
		"2",
		"-1.0 0.5 0.8 -10",
		"1.0 0.5 0.8 10",
	}

	interp := NewThreeHoleInterpolator()
	_, err := interp.LoadPrbData([]PrbFileData{
		{FilePath: "test.prb", Lines: lines},
	})
	if err != nil {
		t.Fatalf("LoadPrbData failed: %v", err)
	}

	// 负大气压应该返回错误
	input := InterpolationInput{
		P1:   -100,
		P2:   100000,
		P3:   100000,
		PAtm: -101325, // 负大气压
		TAtm: 20,
	}

	_, err = interp.Calculate(input)
	if err == nil {
		t.Error("Expected error for negative atmospheric pressure")
	}
}

func TestAlphaNotSorted(t *testing.T) {
	lines := []string{
		"0.4",
		"3",
		"1.0 0.5 0.8 10",    // 乱序：10, 0, -10 而不是 -10, 0, 10
		"0.0 0.3 0.4 0",
		"-1.0 0.5 0.8 -10",
	}

	interp := NewThreeHoleInterpolator()
	_, err := interp.LoadPrbData([]PrbFileData{
		{FilePath: "unsorted.prb", Lines: lines},
	})
	if err == nil {
		t.Error("Expected error for unsorted Alpha values")
	}
}

func TestCalcMachInvalidRatio(t *testing.T) {
	result := calcMach(100000, 200000, 101325)
	if result != 0 {
		t.Errorf("Expected 0 when Pt < Ps, got %f", result)
	}

	result = calcMach(100000, 100000, 101325)
	if result != 0 {
		t.Errorf("Expected 0 when Pt == Ps, got %f", result)
	}

	result = calcMach(100000, 50000, -101325)
	if result != 0 {
		t.Errorf("Expected 0 when Pa negative, got %f", result)
	}

	result = calcMach(50000, 100000, 0)
	if result != 0 {
		t.Errorf("Expected 0 when Pt < Ps with Pa=0, got %f", result)
	}
}