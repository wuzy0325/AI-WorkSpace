package interpolation

import (
	"math"
	"testing"
)

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
		pt, ps, pa, tatm float64
		wantMin, wantMax float64
	}{
		{0, 0, 101325, 20, 0, 0},
		{50000, 40000, 101325, 20, 0, 2},
	}

	for _, tt := range tests {
		mach, err := calcMach(tt.pt, tt.ps, tt.pa, tt.tatm)
		if err != nil {
			t.Errorf("calcMach(%f, %f, %f, %f) 不应返回错误: %v",
				tt.pt, tt.ps, tt.pa, tt.tatm, err)
			continue
		}
		if mach < tt.wantMin || mach > tt.wantMax {
			t.Errorf("calcMach(%f, %f, %f, %f) = %f, want in [%f, %f]",
				tt.pt, tt.ps, tt.pa, tt.tatm, mach, tt.wantMin, tt.wantMax)
		}
	}
}

func TestCalcGamma(t *testing.T) {
	// 20°C 时 gamma 应为 1.4
	g20 := calcGamma(20)
	if math.Abs(g20-1.4) > 1e-9 {
		t.Errorf("calcGamma(20) = %f, want 1.4", g20)
	}

	// 0°C 时 gamma 约为 1.404
	g0 := calcGamma(0)
	if math.Abs(g0-1.404) > 1e-9 {
		t.Errorf("calcGamma(0) = %f, want 1.404", g0)
	}

	// 50°C 时 gamma 约为 1.394
	g50 := calcGamma(50)
	if math.Abs(g50-1.394) > 1e-9 {
		t.Errorf("calcGamma(50) = %f, want 1.394", g50)
	}

	// NaN 或 Inf 应返回默认值 1.4
	gNaN := calcGamma(math.NaN())
	if math.Abs(gNaN-1.4) > 1e-9 {
		t.Errorf("calcGamma(NaN) = %f, want 1.4", gNaN)
	}
}

func TestTemperatureEffect(t *testing.T) {
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

	// P2 偏离 P1/P3 均值以产生非零 ΔP，否则 calcMach 提前返回，温度修正无法生效
	input20 := InterpolationInput{P1: 98000, P2: 100400, P3: 102600, PAtm: 101325, TAtm: 20}
	input50 := InterpolationInput{P1: 98000, P2: 100400, P3: 102600, PAtm: 101325, TAtm: 50}

	res20, err := interp.Calculate(input20)
	if err != nil {
		t.Fatalf("Calculate(20°C) failed: %v", err)
	}
	res50, err := interp.Calculate(input50)
	if err != nil {
		t.Fatalf("Calculate(50°C) failed: %v", err)
	}

	t.Logf("20°C: Mach=%f, 50°C: Mach=%f (diff=%e)", res20.MachNumber, res50.MachNumber, res50.MachNumber-res20.MachNumber)

	if math.Abs(res20.MachNumber-res50.MachNumber) < 1e-10 {
		t.Error("Temperature should affect Mach calculation via gamma")
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
	t.Logf("Extrapolation test: Alpha=%f Mach=%f Warning=%q", res.Alpha, res.MachNumber, res.Warning)
}
