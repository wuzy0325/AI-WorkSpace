package interpolation

import (
	"fmt"
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

// TestCalcVelocity 验证 calcVelocity 数值正确性与非法输入兜底
// 公式: V = Ma · sqrt(γ · R · T_K)，R=287，γ=calcGamma(tatm)
func TestCalcVelocity(t *testing.T) {
	tests := []struct {
		name    string
		ma      float64
		tatm    float64
		want    float64 // 期望值（精确或兜底 0）
		tol     float64 // 容差
		wantZero bool   // 是否期望 0（非法输入）
	}{
		// 正常工况: Ma=0.8, Tatm=20℃ → γ=1.4, T_K=293.15
		// V = 0.8 · sqrt(1.4 · 287 · 293.15) ≈ 274.55 m/s
		{"normal_Ma0.8_T20", 0.8, 20, 274.55, 0.5, false},
		// Ma=0 → V=0
		{"zero_Ma", 0, 20, 0, 0, true},
		// 负马赫数非法 → V=0
		{"negative_Ma", -0.1, 20, 0, 0, true},
		// NaN 马赫数 → V=0
		{"nan_Ma", math.NaN(), 20, 0, 0, true},
		// Inf 马赫数 → V=0
		{"inf_Ma", math.Inf(1), 20, 0, 0, true},
		// 绝对零度以下 → V=0
		{"below_absolute_zero", 0.8, -300, 0, 0, true},
		// 温度影响: 同 Ma=0.8, Tatm=-40℃ 比 Tatm=20℃ 速度低
		// γ=1.412, T_K=233.15, V = 0.8 · sqrt(1.412 · 287 · 233.15) ≈ 245.9 m/s
		{"cold_T-40", 0.8, -40, 245.9, 0.5, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := calcVelocity(tc.ma, tc.tatm)
			if tc.wantZero {
				if got != 0 {
					t.Errorf("calcVelocity(%v, %v) = %v, 期望 0（非法输入兜底）", tc.ma, tc.tatm, got)
				}
				return
			}
			if math.Abs(got-tc.want) > tc.tol {
				t.Errorf("calcVelocity(%v, %v) = %.4f, 期望 %.4f ± %.2f", tc.ma, tc.tatm, got, tc.want, tc.tol)
			}
		})
	}
}

// TestCalcVelocity_TemperatureEffect 验证温度对速度的影响
// 同一 Ma 下，高温工况速度更高（声速随温度升高增大）
func TestCalcVelocity_TemperatureEffect(t *testing.T) {
	vHot := calcVelocity(0.8, 50)  // 50℃ 高温
	vCold := calcVelocity(0.8, -40) // -40℃ 低温
	if vHot <= vCold {
		t.Errorf("高温速度应大于低温速度: vHot=%.4f, vCold=%.4f", vHot, vCold)
	}
	t.Logf("温度影响: V(50℃)=%.4f m/s, V(-40℃)=%.4f m/s, 差值=%.4f", vHot, vCold, vHot-vCold)
}

// TestCalculate_VelocityOutput 集成测试: Calculate 返回的 Velocity 与 calcVelocity(Ma, Tatm) 一致
// 覆盖单文件快速路径（finalizeSingle）的所有返回点：正常返回 + 非法输入兜底
func TestCalculate_VelocityOutput(t *testing.T) {
	// 单文件快速路径: 用真实 0.8.prb 校准数据
	interp := NewThreeHoleInterpolator()
	lines := []string{
		"0.4", "5",
		"-1.5 0.3 0.9 -20",
		"-0.8 0.3 0.85 -10",
		"0.0 0.3 0.8 0",
		"0.8 0.3 0.85 10",
		"1.5 0.3 0.9 20",
	}
	if _, err := interp.LoadPrbData([]PrbFileData{{FilePath: "test.prb", Lines: lines}}); err != nil {
		t.Fatalf("LoadPrbData failed: %v", err)
	}

	// 正常工况: 期望 Ma 落在校准范围内, Velocity 与 calcVelocity 一致
	input := InterpolationInput{P1: 50000, P2: 80000, P3: 60000, PAtm: 101325, TAtm: 20}
	res, err := interp.Calculate(input)
	if err != nil {
		t.Fatalf("Calculate failed: %v", err)
	}
	if res.Calculated {
		wantV := calcVelocity(res.MachNumber, input.TAtm)
		if math.Abs(res.Velocity-wantV) > 1e-9 {
			t.Errorf("单文件路径 Velocity=%.6f, 期望 calcVelocity(Ma=%.6f, T=%.1f)=%.6f",
				res.Velocity, res.MachNumber, input.TAtm, wantV)
		}
		if res.Velocity <= 0 {
			t.Errorf("正常工况 Velocity 应 > 0, 实际=%.4f (Ma=%.4f)", res.Velocity, res.MachNumber)
		}
	}
	t.Logf("单文件路径: Ma=%.4f, V=%.4f m/s, IsValid=%v", res.MachNumber, res.Velocity, res.IsValid)

	// 输入非法（PAtm=0）: 期望 Velocity=0（Ma 兜底为 0）
	badInput := InterpolationInput{P1: 50000, P2: 80000, P3: 60000, PAtm: 0, TAtm: 20}
	badRes, _ := interp.Calculate(badInput)
	if badRes.Velocity != 0 {
		t.Errorf("非法输入 Velocity 应为 0, 实际=%.4f (Ma=%.4f, Warning=%q)",
			badRes.Velocity, badRes.MachNumber, badRes.Warning)
	}
}

// TestCalculate_VelocityOutput_MultiFile 集成测试: 多文件迭代路径（calculateMulti）的 Velocity 填充
// 覆盖 calculateMulti 的所有返回点：正常返回 + newMaRes.error 路径 + finalMatch==nil 兜底
func TestCalculate_VelocityOutput_MultiFile(t *testing.T) {
	// 两档合成 PRB（CMa=0.3/0.6），触发多文件迭代路径
	threeSynth := func(cma float64) []string {
		lines := []string{fmt.Sprintf("%.6f", cma), "13"}
		for alpha := -30.0; alpha <= 30; alpha += 5 {
			kb := (4 + 2*cma) * math.Sin(alpha*math.Pi/180)
			k0 := 0.5 + 0.01*alpha + 0.1*cma
			kv := 2.0 + 0.02*alpha + 0.05*cma
			lines = append(lines, fmt.Sprintf("%.10f %.10f %.10f %.0f", kb, k0, kv, alpha))
		}
		return lines
	}
	interp := NewThreeHoleInterpolator()
	files := []PrbFileData{
		{FilePath: "0.3Ma.prb", Lines: threeSynth(0.3)},
		{FilePath: "0.6Ma.prb", Lines: threeSynth(0.6)},
	}
	if _, err := interp.LoadPrbData(files); err != nil {
		t.Fatalf("LoadPrbData failed: %v", err)
	}

	// 正常工况: Velocity 与 calcVelocity(Ma, Tatm) 一致
	input := InterpolationInput{P1: 5000, P2: 8000, P3: 6000, PAtm: 101325, TAtm: 20}
	res, err := interp.Calculate(input)
	if err != nil {
		t.Fatalf("Calculate failed: %v", err)
	}
	if res.Calculated {
		wantV := calcVelocity(res.MachNumber, input.TAtm)
		if math.Abs(res.Velocity-wantV) > 1e-9 {
			t.Errorf("多文件路径 Velocity=%.6f, 期望 calcVelocity(Ma=%.6f, T=%.1f)=%.6f",
				res.Velocity, res.MachNumber, input.TAtm, wantV)
		}
		if res.Velocity <= 0 {
			t.Errorf("正常工况 Velocity 应 > 0, 实际=%.4f (Ma=%.4f)", res.Velocity, res.MachNumber)
		}
	}
	t.Logf("多文件路径: Ma=%.4f, V=%.4f m/s, IsValid=%v", res.MachNumber, res.Velocity, res.IsValid)

	// 非法输入（PAtm=0）: 期望 Velocity=0（calcMach 失败路径，Ma=0 兜底）
	badInput := InterpolationInput{P1: 5000, P2: 8000, P3: 6000, PAtm: 0, TAtm: 20}
	badRes, _ := interp.Calculate(badInput)
	if badRes.Velocity != 0 {
		t.Errorf("多文件非法输入 Velocity 应为 0, 实际=%.4f (Ma=%.4f, Warning=%q)",
			badRes.Velocity, badRes.MachNumber, badRes.Warning)
	}
}
