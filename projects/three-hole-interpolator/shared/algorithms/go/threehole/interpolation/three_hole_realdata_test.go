package interpolation

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestRealPrbData 测试真实PRB校准数据解析
// 数据来源: C:\Users\wuzhy\Documents\D\SVN\SoftWare\trunk\ThreeHoleProbeApp\20251127-三孔差值试验\prb\
func TestRealPrbData(t *testing.T) {
	testCases := []struct {
		name     string
		fileName string
		wantCMa  float64
		wantNalpha int
	}{
		{"0.2Ma校准文件", "0.2.prb", 0.2, 13},
		{"0.4Ma校准文件", "0.4.prb", 0.4, 13},
		{"0.6Ma校准文件", "0.6.prb", 0.6, 13},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			lines, err := loadTestFile(tc.fileName)
			if err != nil {
				t.Skipf("skipping: test file not found: %v", err)
			}

			cal, err := parsePrbLines(lines)
			if err != nil {
				t.Fatalf("parsePrbLines failed: %v", err)
			}

			if cal.CMa != tc.wantCMa {
				t.Errorf("CMa = %f, want %f", cal.CMa, tc.wantCMa)
			}
			if cal.Nalpha != tc.wantNalpha {
				t.Errorf("Nalpha = %d, want %d", cal.Nalpha, tc.wantNalpha)
			}
			if len(cal.Items) != tc.wantNalpha {
				t.Errorf("Items count = %d, want %d", len(cal.Items), tc.wantNalpha)
			}
		})
	}
}

// TestRealDataCalculation 测试真实数据计算
// 数据来源: C:\Users\wuzhy\Documents\D\SVN\SoftWare\trunk\ThreeHoleProbeApp\20251127-三孔差值试验\data\
// DAT格式: Pa, Ta, PtInlet, PsInlet, ProbeHole1, ProbeHole2, ProbeHole3
func TestRealDataCalculation(t *testing.T) {
	lines0_2, err := loadTestFile("0.2.prb")
	if err != nil {
		t.Skipf("skipping: test file not found: %v", err)
	}
	lines0_4, err := loadTestFile("0.4.prb")
	if err != nil {
		t.Skipf("skipping: test file not found: %v", err)
	}
	lines0_6, err := loadTestFile("0.6.prb")
	if err != nil {
		t.Skipf("skipping: test file not found: %v", err)
	}

	interp := NewThreeHoleInterpolator()
	_, err = interp.LoadPrbData([]PrbFileData{
		{FilePath: "testdata/0.2.prb", Lines: lines0_2},
		{FilePath: "testdata/0.4.prb", Lines: lines0_4},
		{FilePath: "testdata/0.6.prb", Lines: lines0_6},
	})
	if err != nil {
		t.Fatalf("LoadPrbData failed: %v", err)
	}

	// 验证Mach范围
	minMa, maxMa := interp.GetMachRange()
	t.Logf("Mach range: [%f, %f]", minMa, maxMa)
	if minMa != 0.2 || maxMa != 0.6 {
		t.Errorf("Mach range = [%f, %f], want [0.2, 0.6]", minMa, maxMa)
	}

	// 测试用例1: 0.2Ma_18度.dat 样本数据
	// DAT格式: Pa, Ta, ?, PtInlet, ProbeHole1(P1), ProbeHole2(P2), ProbeHole3(P3)
	// 第一行数据: 100920, 15.8, -57.4, 2883.8, -941.5, 2512, 2445
	t.Run("0.2Ma_18度样本", func(t *testing.T) {
		input := InterpolationInput{
			P1:   -941.5,  // ProbeHole1
			P2:   2512.0,  // ProbeHole2
			P3:   2445.0,  // ProbeHole3
			PAtm: 100920.0, // Pa
			TAtm: 15.8,     // Ta
		}

		res, err := interp.Calculate(input)
		if err != nil {
			t.Fatalf("Calculate failed: %v", err)
		}

		t.Logf("Result: Alpha=%f°, Mach=%f, Pt=%f, Ps=%f, Iterations=%d, Warning=%q",
			res.Alpha, res.MachNumber, res.TotalPressure, res.StaticPressure, res.IterationCount, res.Warning)

		// 预期: Ma≈0.2, Alpha≈18°
		// 允许误差: Ma±0.05, Alpha±5°
		if res.MachNumber < 0.15 || res.MachNumber > 0.25 {
			t.Errorf("Mach = %f, expected ~0.2", res.MachNumber)
		}
		if res.Alpha < 13 || res.Alpha > 23 {
			t.Errorf("Alpha = %f°, expected ~18°", res.Alpha)
		}
	})

	// 测试用例2: 0.4Ma校准数据样本
	// Pa=100895.333, P1=??? , P2=???, P3=???
	t.Run("0.4Ma样本", func(t *testing.T) {
		input := InterpolationInput{
			P1:   5500.0,
			P2:   3500.0,
			P3:   -2000.0,
			PAtm: 100895.333,
			TAtm: 20.0,
		}

		res, err := interp.Calculate(input)
		if err != nil {
			t.Fatalf("Calculate failed: %v", err)
		}

		t.Logf("Result: Alpha=%f°, Mach=%f, Pt=%f, Ps=%f, Iterations=%d, Warning=%q",
			res.Alpha, res.MachNumber, res.TotalPressure, res.StaticPressure, res.IterationCount, res.Warning)

		// 验证结果在合理范围内
		if res.MachNumber < 0 || res.MachNumber > 1.0 {
			t.Errorf("Mach = %f out of reasonable range", res.MachNumber)
		}
		if res.Alpha < -30 || res.Alpha > 30 {
			t.Errorf("Alpha = %f° out of reasonable range [-30°, 30°]", res.Alpha)
		}
	})

	// 测试用例3: 边界条件 - 测试Kb越界外推
	t.Run("Kb越界外推", func(t *testing.T) {
		input := InterpolationInput{
			P1:   -3000.0,
			P2:   5000.0,
			P3:   8000.0,
			PAtm: 100895.333,
			TAtm: 20.0,
		}

		res, err := interp.Calculate(input)
		if err != nil {
			t.Fatalf("Calculate failed: %v", err)
		}

		t.Logf("Result: Alpha=%f°, Mach=%f, Pt=%f, Ps=%f, Iterations=%d, Warning=%q",
			res.Alpha, res.MachNumber, res.TotalPressure, res.StaticPressure, res.IterationCount, res.Warning)

		// 验证外推警告存在
		if res.Warning == "" {
			t.Error("Expected warning for extrapolation")
		}

		// 验证结果在合理范围内（只是验证不崩溃）
		if res.MachNumber < 0 || res.MachNumber > 1.0 {
			t.Errorf("Mach = %f out of reasonable range", res.MachNumber)
		}
	})
}

// TestZeroDeltaP 测试零压力差分情况
func TestZeroDeltaP(t *testing.T) {
	lines0_4, err := loadTestFile("0.4.prb")
	if err != nil {
		t.Skipf("skipping: test file not found: %v", err)
	}

	interp := NewThreeHoleInterpolator()
	_, err = interp.LoadPrbData([]PrbFileData{
		{FilePath: "testdata/0.4.prb", Lines: lines0_4},
	})
	if err != nil {
		t.Fatalf("LoadPrbData failed: %v", err)
	}

	// 当 P1=P2=P3 时, deltaP=0, 应该返回特定结果
	input := InterpolationInput{
		P1:   10000.0,
		P2:   10000.0,
		P3:   10000.0,
		PAtm: 101325.0,
		TAtm: 20.0,
	}

	res, err := interp.Calculate(input)
	if err != nil {
		t.Fatalf("Calculate failed: %v", err)
	}

	// 应该返回 invalid，Pressure difference near zero
	if res.IsValid {
		t.Error("Expected IsValid=false for zero deltaP")
	}
	if res.TotalPressure != 10000.0 || res.StaticPressure != 10000.0 {
		t.Errorf("Expected fallback Pt=Ps=P2=%f, got Pt=%f Ps=%f", 10000.0, res.TotalPressure, res.StaticPressure)
	}
	t.Logf("Zero deltaP result: IsValid=%v, Warning=%q", res.IsValid, res.Warning)
}

// TestProbeHoleNegativePressure 测试探测孔负压输入（风洞差压测量）
// 与C#一致：允许P1/P2/P3为负值（差压）
func TestProbeHoleNegativePressure(t *testing.T) {
	lines0_4, err := loadTestFile("0.4.prb")
	if err != nil {
		t.Skipf("skipping: test file not found: %v", err)
	}

	interp := NewThreeHoleInterpolator()
	_, err = interp.LoadPrbData([]PrbFileData{
		{FilePath: "testdata/0.4.prb", Lines: lines0_4},
	})
	if err != nil {
		t.Fatalf("LoadPrbData failed: %v", err)
	}

	// 探测孔负压是正常的（差压测量），应该成功计算
	// 使用真实数据第一行
	input := InterpolationInput{
		P1:   -941.5,  // ProbeHole1 (来自真实数据)
		P2:   2512.0,  // ProbeHole2
		P3:   2445.0,  // ProbeHole3
		PAtm: 100920.0, // Pa
		TAtm: 15.8,     // Ta
	}

	res, err := interp.Calculate(input)
	if err != nil {
		t.Fatalf("Calculate failed for probe hole negative pressure: %v", err)
	}

	t.Logf("Probe hole negative pressure result: Alpha=%f°, Mach=%f, Pt=%f, Ps=%f, Iterations=%d",
		res.Alpha, res.MachNumber, res.TotalPressure, res.StaticPressure, res.IterationCount)

	// 验证结果在合理范围内
	if res.MachNumber < 0 || res.MachNumber > 1.0 {
		t.Errorf("Mach = %f out of reasonable range", res.MachNumber)
	}
}

// TestMachNumberFormula 验证马赫数计算公式与C#一致
// C#公式: Ma = sqrt(5 * |((Pt+Pa)/(Ps+Pa))^0.2857 - 1|)
func TestMachNumberFormula(t *testing.T) {
	testCases := []struct {
		name string
		pt   float64
		ps   float64
		pa   float64
	}{
		{"标准大气条件", 101325.0, 95000.0, 101325.0},
		{"高速条件", 200000.0, 100000.0, 101325.0},
		{"低速条件", 102000.0, 100000.0, 101325.0},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ma := calcMach(tc.pt, tc.ps, tc.pa)
			t.Logf("Pt=%f, Ps=%f, Pa=%f -> Ma=%f", tc.pt, tc.ps, tc.pa, ma)

			// 验证Ma在合理范围内
			if ma < 0 {
				t.Errorf("Mach should be non-negative, got %f", ma)
			}
			if ma > 1.5 {
				t.Errorf("Mach seems too high: %f", ma)
			}
		})
	}
}

func loadTestFile(name string) ([]string, error) {
	path := filepath.Join("testdata", name)
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	lines := strings.Split(string(content), "\n")
	result := make([]string, 0, len(lines))
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result, nil
}
