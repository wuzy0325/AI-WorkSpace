package interpolation

import (
	"math"
	"strings"
	"testing"
)

// =====================================================================
// 改善项回归测试（docs/three-hole-interpolation-improvements.md）
//   - A1 输入有限性校验（结构化无效结果契约）
//   - A2 完整气动参数校验（对齐七孔 calVelocityMach 前置条件）
//   - A3 超范围警告携带诊断数值
//   - A4 加载器一致性校验（Alpha 网格一致 + Kb 严格单调无重复）
//   - B1+C2 单文件快速路径（IterationCount=1、无 maClamped 警告）
// =====================================================================

// ==================== A1: 输入有限性校验 ====================

func TestA1_InputFiniteValidation(t *testing.T) {
	interp := loadRealPrbFile(t)

	t.Run("Patm NaN 不再静默放行", func(t *testing.T) {
		// A1 核心复现：此前 Patm=NaN 会穿透到 calcMach 产出 MachNumber=NaN 且
		// isValid=true（NaN 与校准范围比较恒 false）。现在入口应判无效。
		input := calibInputForAlpha(0)
		input.PAtm = math.NaN()
		res, err := interp.Calculate(input)
		if err != nil {
			t.Fatalf("Calculate 不应返回 error（结构化无效结果契约）: %v", err)
		}
		if res.IsValid {
			t.Fatalf("Patm=NaN 应 IsValid=false, 实际=true")
		}
		if !isFinite(res.MachNumber) {
			t.Fatalf("MachNumber 应为有限值: %v", res.MachNumber)
		}
	})

	t.Run("Tatm Inf 判无效", func(t *testing.T) {
		input := calibInputForAlpha(0)
		input.TAtm = math.Inf(1)
		res, err := interp.Calculate(input)
		if err != nil {
			t.Fatalf("Calculate 不应返回 error: %v", err)
		}
		if res.IsValid {
			t.Fatalf("Tatm=Inf 应 IsValid=false, 实际=true")
		}
	})

	t.Run("P1 NaN 保持既有拦截", func(t *testing.T) {
		input := calibInputForAlpha(0)
		input.P1 = math.NaN()
		res, err := interp.Calculate(input)
		if err != nil {
			t.Fatalf("Calculate 不应返回 error: %v", err)
		}
		if res.IsValid {
			t.Fatalf("P1=NaN 应 IsValid=false, 实际=true")
		}
	})
}

// ==================== A2: 完整气动参数校验 ====================

func TestA2_CalcMachPreconditions(t *testing.T) {
	cases := []struct {
		name    string
		pt, ps  float64
		patm    float64
		tatm    float64
		wantErr bool
	}{
		{"合法", 50000, 40000, 101325, 20, false},
		{"压力比=1 返回 0", 0, 0, 101325, 20, false},
		{"pt<ps", 40000, 50000, 101325, 20, true},
		{"patm=0", 50000, 40000, 0, 20, true},
		{"patm<0", 50000, 40000, -1, 20, true},
		{"patm=NaN", 50000, 40000, math.NaN(), 20, true},
		{"tatm=-300 绝对零度下", 50000, 40000, 101325, -300, true},
		{"ps+patm<=0", 100, -200000, 101325, 20, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := calcMach(c.pt, c.ps, c.patm, c.tatm)
			if c.wantErr && err == nil {
				t.Fatalf("calcMach(%v) 应返回错误", c)
			}
			if !c.wantErr && err != nil {
				t.Fatalf("calcMach(%v) 不应返回错误: %v", c, err)
			}
		})
	}
}

func TestA2_NonPhysicalRecovery_ReturnsInvalid(t *testing.T) {
	interp := loadRealPrbFile(t)
	// deltaP<0 时 Kv>0 使 ps = pt - Kv*deltaP > pt，触发 pt<ps 非物理校验。
	// P1=P3=100000, P2=50000 → deltaP = -100000，Kb=0（对称）。
	input := InterpolationInput{P1: 100000, P2: 50000, P3: 100000, PAtm: 101325, TAtm: 20}
	res, err := interp.Calculate(input)
	if err != nil {
		t.Fatalf("Calculate 不应返回 error: %v", err)
	}
	if res.IsValid {
		t.Fatalf("pt<ps 非物理恢复应 IsValid=false, 实际=true")
	}
	if !strings.Contains(res.Warning, "总压低于静压") {
		t.Fatalf("警告应提示总压低于静压: %q", res.Warning)
	}
}

// ==================== A3: 超范围警告诊断信息 ====================

func TestA3_OutOfRangeWarningCarriesValues(t *testing.T) {
	interp := loadRealPrbFile(t)
	// 小压差输入 → 恢复 Ma≈0.236 < 0.791（0.8.prb 下限），触发超范围。
	input := InterpolationInput{P1: 1000, P2: 2000, P3: 1000, PAtm: 101325, TAtm: 20}
	res, err := interp.Calculate(input)
	if err != nil {
		t.Fatalf("Calculate 不应返回 error: %v", err)
	}
	if res.IsValid {
		t.Fatalf("恢复 Ma 超出校准范围应 IsValid=false, 实际=true")
	}
	if !res.Calculated {
		t.Fatal("恢复 Ma 超出校准范围时已完成计算，Calculated 应为 true")
	}
	if !strings.Contains(res.Warning, "恢复Ma=") || !strings.Contains(res.Warning, "校准范围[") {
		t.Fatalf("警告应携带恢复Ma与校准范围: %q", res.Warning)
	}
}

func TestResultCalculatedDistinguishesReferenceFromFailure(t *testing.T) {
	interp := loadRealPrbFile(t)

	t.Run("校准范围内完整结果", func(t *testing.T) {
		res, err := interp.Calculate(calibInputForAlpha(0))
		if err != nil {
			t.Fatalf("Calculate 不应返回 error: %v", err)
		}
		if !res.Calculated || !res.IsValid {
			t.Fatalf("完整有效结果应 Calculated=true, IsValid=true: %+v", res)
		}
	})

	t.Run("零压差未完成计算", func(t *testing.T) {
		res, err := interp.Calculate(InterpolationInput{PAtm: 101325, TAtm: 20})
		if err != nil {
			t.Fatalf("Calculate 不应返回 error: %v", err)
		}
		if res.Calculated || res.IsValid {
			t.Fatalf("零压差应 Calculated=false, IsValid=false: %+v", res)
		}
	})

	t.Run("非物理恢复未完成完整结果", func(t *testing.T) {
		input := InterpolationInput{P1: 100000, P2: 50000, P3: 100000, PAtm: 101325, TAtm: 20}
		res, err := interp.Calculate(input)
		if err != nil {
			t.Fatalf("Calculate 不应返回 error: %v", err)
		}
		if res.Calculated || res.IsValid {
			t.Fatalf("pt<ps 应 Calculated=false, IsValid=false: %+v", res)
		}
	})
}

// ==================== A4: 加载器一致性校验 ====================

func prbLinesNalpha5() []string {
	return []string{
		"0.4",
		"5",
		"-1.5 0.3 0.9 -20",
		"-0.8 0.2 0.5 -10",
		"0.0 0.0 0.0 0",
		"0.8 0.2 0.5 10",
		"1.5 0.3 0.9 20",
	}
}

func TestA4_LoaderAlphaGridMismatch(t *testing.T) {
	base := prbLinesNalpha5()
	// 第二档 Alpha 网格与首档不一致（末点 20 → 25），应拒绝加载。
	mismatch := []string{
		"0.6",
		"5",
		"-1.5 0.3 0.9 -20",
		"-0.8 0.2 0.5 -10",
		"0.0 0.0 0.0 0",
		"0.8 0.2 0.5 10",
		"1.5 0.3 0.9 25",
	}
	interp := NewThreeHoleInterpolator()
	_, err := interp.LoadPrbData([]PrbFileData{
		{FilePath: "a.prb", Lines: base},
		{FilePath: "b.prb", Lines: mismatch},
	})
	if err == nil {
		t.Fatal("Alpha 网格不一致应拒绝加载")
	}
	if !strings.Contains(err.Error(), "Alpha网格") {
		t.Fatalf("错误信息应指明 Alpha 网格: %v", err)
	}
}

func TestA4_LoaderDuplicateKb(t *testing.T) {
	dup := []string{
		"0.4",
		"3",
		"-1.0 0.5 0.8 -10",
		"-1.0 0.3 0.4 0",
		"1.0 0.5 0.8 10",
	}
	interp := NewThreeHoleInterpolator()
	_, err := interp.LoadPrbData([]PrbFileData{
		{FilePath: "dup.prb", Lines: dup},
	})
	if err == nil {
		t.Fatal("Kb 重复应拒绝加载")
	}
}

func TestA4_LoaderNonMonotonicKb(t *testing.T) {
	nonMonotonic := []string{
		"0.4",
		"3",
		"-1.0 0.5 0.8 -10",
		"0.2 0.3 0.4 0",
		"0.1 0.5 0.8 10",
	}
	interp := NewThreeHoleInterpolator()
	_, err := interp.LoadPrbData([]PrbFileData{
		{FilePath: "nmono.prb", Lines: nonMonotonic},
	})
	if err == nil {
		t.Fatal("Kb 非严格单调应拒绝加载")
	}
	if !strings.Contains(err.Error(), "Kb") {
		t.Fatalf("错误信息应指明 Kb: %v", err)
	}
}

// ==================== B1+C2: 单文件快速路径 ====================

func TestB1_SingleFileFastPath(t *testing.T) {
	interp := loadRealPrbFile(t)

	t.Run("合法输入 IterationCount=1 且无 maClamped 警告", func(t *testing.T) {
		res, err := interp.Calculate(calibInputForAlpha(0))
		if err != nil {
			t.Fatalf("Calculate 不应返回 error: %v", err)
		}
		if res.IterationCount != 1 {
			t.Fatalf("单文件快速路径 IterationCount 应为 1, 实际 %d", res.IterationCount)
		}
		if strings.Contains(res.Warning, "标定边界") {
			t.Fatalf("单文件路径不应出现 maClamped 警告: %q", res.Warning)
		}
		if !res.IsValid {
			t.Fatalf("校准一致性输入应 IsValid=true, Warning=%q", res.Warning)
		}
		if math.Abs(res.MachNumber-0.8) > 0.05 {
			t.Fatalf("Mach 应≈0.8, 实际 %.4f", res.MachNumber)
		}
	})

	t.Run("角度与全量迭代结果一致（Alpha 回归）", func(t *testing.T) {
		for _, alpha := range []float64{-30, -15, 0, 15, 30} {
			res, err := interp.Calculate(calibInputForAlpha(alpha))
			if err != nil {
				t.Fatalf("alpha=%v Calculate 错误: %v", alpha, err)
			}
			if math.Abs(res.Alpha-alpha) > 0.5 {
				t.Errorf("Alpha 应≈%v, 实际 %.4f", alpha, res.Alpha)
			}
		}
	})
}

func TestB1_SingleFileOutOfRangeKeepsRejection(t *testing.T) {
	// 回归现场结论：恢复 Ma 与标定不符时必须保持 isValid=false。
	interp := loadRealPrbFile(t)
	input := InterpolationInput{P1: 1000, P2: 2000, P3: 1000, PAtm: 101325, TAtm: 20}
	res, err := interp.Calculate(input)
	if err != nil {
		t.Fatalf("Calculate 不应返回 error: %v", err)
	}
	if res.IsValid {
		t.Fatal("恢复 Ma 超出校准范围应 IsValid=false")
	}
}

func TestB1_MultiFileStillIterates(t *testing.T) {
	// 多文件路径必须保留迭代（B1 只优化单文件场景）。
	lines1 := []string{"0.2", "5", "-1.5 0.3 0.9 -20", "-0.8 0.2 0.5 -10", "0.0 0.0 0.0 0", "0.8 0.2 0.5 10", "1.5 0.3 0.9 20"}
	lines2 := []string{"0.6", "5", "-1.5 0.3 0.9 -20", "-0.8 0.2 0.5 -10", "0.0 0.0 0.0 0", "0.8 0.2 0.5 10", "1.5 0.3 0.9 20"}
	interp := NewThreeHoleInterpolator()
	if _, err := interp.LoadPrbData([]PrbFileData{
		{FilePath: "a.prb", Lines: lines1},
		{FilePath: "b.prb", Lines: lines2},
	}); err != nil {
		t.Fatalf("LoadPrbData failed: %v", err)
	}
	input := InterpolationInput{P1: 98000, P2: 101000, P3: 102000, PAtm: 101325, TAtm: 20}
	res, err := interp.Calculate(input)
	if err != nil {
		t.Fatalf("Calculate 不应返回 error: %v", err)
	}
	if res.IterationCount <= 0 {
		t.Fatalf("多文件路径 IterationCount 应 > 0, 实际 %d", res.IterationCount)
	}
}
