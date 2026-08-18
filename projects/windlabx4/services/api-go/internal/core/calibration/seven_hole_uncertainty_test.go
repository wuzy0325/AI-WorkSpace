package calibration

import (
	"math"
	"testing"
)

// ==================== 七孔探针不确定度评定测试（spec §5） ====================
//
// 测试覆盖 spec §5 不确定度评定的所有关键算法：
//   - B 类分量 u1~u4（spec §5.1）
//   - A 类分量 u(A) = S/√N（spec §5.1）
//   - 内区 K0/Ks/Kα/Kβ 灵敏系数（spec §5.3）
//   - 合成公式 |Σ cᵢu_B,i|² + Σ cᵢ²u(A,i)²（spec §5.2，B 类 ρ=1，A 类 ρ=0）
//   - 扩展不确定度 U = k·u_c（spec §5.4）
//   - 中心点 7 步算例完整复算（spec §5.5，目标 U(K0) ∈ [0.004, 0.0082]）

// sevenHoleUncEpsilon 不确定度浮点比较容差（相对容差，应对量级跨度大的浮点比较）
const sevenHoleUncEpsilon = 1e-4

// approxEqualRelative 相对容差浮点比较
//
// 用于不确定度计算中量级跨度大的浮点数比较（如 1e-4 vs 1e-7）。
// 当期望值接近 0 时退化为绝对容差比较（避免除零）。
func approxEqualRelative(actual, expected, relTol float64) bool {
	if math.Abs(expected) < 1e-12 {
		return math.Abs(actual) < relTol
	}
	return math.Abs(actual-expected)/math.Abs(expected) < relTol
}

// ==================== B 类分量测试（spec §5.1） ====================

// TestUncertaintyBTypeComponents 验证 B 类分量 u1~u4 数值
//
// 测试前置：构造 UncertaintyCalculator，传入 pTunnel 作为 u2 计算输入
// 测试步骤：分别读取 u1/u2/u3/u4 数值
// 期待结果（spec §5.1 表格，表格值为取整值，实现用精确值，容差 1%）：
//   - u1 = 20.2 Pa（扫描阀误差 70000×0.05%/√3，精确值 20.2073）
//   - u2 = p_t × 0.1% / √3（随 p_t 变化，p_t=4073.07 时精确值 2.3516）
//   - u3 = 3 Pa（位移机构运动误差 5/√3，精确值 2.8868）
//   - u4 = 5.8 Pa（静压扫描阀误差 20000×0.05%/√3，精确值 5.7735）
func TestUncertaintyBTypeComponents(t *testing.T) {
	// spec §5.5 算例输入：p_t = 4073.07
	calc := NewUncertaintyCalculator(4073.07)

	// u1 = 70000 × 0.0005 / √3 = 20.207...
	if !approxEqualRelative(calc.U1(), 20.2073, 0.001) {
		t.Errorf("u1 期望 20.2073, 实际 %.6f", calc.U1())
	}

	// u2 = 4073.07 × 0.001 / √3 = 2.3516...
	if !approxEqualRelative(calc.U2(), 2.3516, 0.001) {
		t.Errorf("u2 期望 2.3516 (p_t=4073.07), 实际 %.6f", calc.U2())
	}

	// u3 = 5 / √3 = 2.886...
	if !approxEqualRelative(calc.U3(), 2.8868, 0.001) {
		t.Errorf("u3 期望 2.8868 (5/√3), 实际 %.6f", calc.U3())
	}

	// u4 = 20000 × 0.0005 / √3 = 5.773...
	if !approxEqualRelative(calc.U4(), 5.7735, 0.001) {
		t.Errorf("u4 期望 5.7735, 实际 %.6f", calc.U4())
	}
}

// TestUncertaintyBTypeForInput 验证按误差源分组的 B 类分量（spec §5.5 第 1 步）
//
// 测试前置：构造 UncertaintyCalculator，p_t=4073.07
// 测试步骤：调用 BTypeForInput 获取各输入的 B 类不确定度
// 期待结果（spec §5.5 第 1 步表格用取整值，实现用精确值，容差 2%）：
//   - u_B(P7)  = √(u1² + u3²) = √(20.207² + 2.887²) = 20.4124
//     （spec 表格 20.42 用 u1=20.2, u3=3 取整值算出）
//   - u_B(p_t) = √(u2² + u3²) = √(2.352² + 2.887²) = 3.7232
//     （spec 表格 3.81 用 u2=2.35, u3=3 取整值算出）
//   - u_B(p_s) = √(u4² + u3²) = √(5.774² + 2.887²) = 6.4550
//     （spec 表格 6.53 用 u4=5.8, u3=3 取整值算出）
//   - u_B(P1..P6) = √(u1² + u3²) = 20.4124（与 P7 同源，都是扫描阀 + 运动）
//
// 注：spec §5.5 算例表格使用取整后的 u1~u4 值（如 u3=3 而非 2.887），
// 算例结果用于教学演示；实现用精确值计算，最终 U(K0) 仍落在 spec 容差范围内（见 TestUncertaintyCenterPointK0）。
func TestUncertaintyBTypeForInput(t *testing.T) {
	calc := NewUncertaintyCalculator(4073.07)

	// P7：u1（扫描阀）+ u3（运动），精确值 √(20.2073² + 2.8868²) = 20.4124
	uBP7 := calc.BTypeForInput("P7")
	if !approxEqualRelative(uBP7, 20.4124, 0.002) {
		t.Errorf("u_B(P7) 期望 20.4124 (精确值), 实际 %.4f", uBP7)
	}

	// p_t：u2（总压脉动）+ u3（运动），精确值 √(2.3516² + 2.8868²) = 3.7232
	uBPt := calc.BTypeForInput("p_t")
	if !approxEqualRelative(uBPt, 3.7232, 0.002) {
		t.Errorf("u_B(p_t) 期望 3.7232 (精确值), 实际 %.4f", uBPt)
	}

	// p_s：u4（静压扫描阀）+ u3（运动），精确值 √(5.7735² + 2.8868²) = 6.4550
	uBPs := calc.BTypeForInput("p_s")
	if !approxEqualRelative(uBPs, 6.4550, 0.002) {
		t.Errorf("u_B(p_s) 期望 6.4550 (精确值), 实际 %.4f", uBPs)
	}

	// P1..P6：与 P7 同源
	uBP1 := calc.BTypeForInput("P1")
	if !approxEqualRelative(uBP1, 20.4124, 0.002) {
		t.Errorf("u_B(P1) 期望 20.4124 (与 P7 同源), 实际 %.4f", uBP1)
	}
}

// ==================== A 类分量测试（spec §5.1） ====================

// TestCalculateTypeA 验证 A 类分量 u(A) = S/√N
//
// 测试前置：构造样本 [100, 102, 98, 101, 99]（N=5，样本标准差约 1.581）
// 测试步骤：调用 CalculateTypeA
// 期待结果：uA = S/√5 ≈ 0.707，stdDev ≈ 1.581
func TestCalculateTypeA(t *testing.T) {
	calc := NewUncertaintyCalculator(4073.07)

	samples := []float64{100, 102, 98, 101, 99}
	uA, stdDev := calc.CalculateTypeA(samples)

	// 样本标准差 S = √(Σ(xi-x̄)²/(N-1)) = √(10/4) = 1.5811
	expectedStd := math.Sqrt(10.0 / 4.0)
	if !approxEqualRelative(stdDev, expectedStd, sevenHoleUncEpsilon) {
		t.Errorf("stdDev 期望 %.6f, 实际 %.6f", expectedStd, stdDev)
	}

	// uA = S/√N = 1.5811/√5 = 0.7071
	expectedUA := expectedStd / math.Sqrt(5)
	if !approxEqualRelative(uA, expectedUA, sevenHoleUncEpsilon) {
		t.Errorf("uA 期望 %.6f, 实际 %.6f", expectedUA, uA)
	}
}

// TestCalculateTypeA_SmallSample 验证小样本边界：N<2 时返回 0
//
// 测试前置：构造 N=0、N=1 的样本
// 测试步骤：调用 CalculateTypeA
// 期待结果：uA=0, stdDev=0（样本数不足无法计算样本标准差）
func TestCalculateTypeA_SmallSample(t *testing.T) {
	calc := NewUncertaintyCalculator(4073.07)

	// 空样本
	uA, stdDev := calc.CalculateTypeA([]float64{})
	if uA != 0 || stdDev != 0 {
		t.Errorf("空样本应返回 0,0, 实际 uA=%v stdDev=%v", uA, stdDev)
	}

	// 单样本
	uA, stdDev = calc.CalculateTypeA([]float64{100})
	if uA != 0 || stdDev != 0 {
		t.Errorf("单样本应返回 0,0, 实际 uA=%v stdDev=%v", uA, stdDev)
	}
}

// ==================== 内区 K0 灵敏系数测试（spec §5.3） ====================

// TestSensitivityCoefficientsK0 验证内区 K0 灵敏系数（spec §5.5 第 3 步）
//
// K0 = (P7 - p_t) / (p_t - p_s)
// 测试前置：P7=4075.35, p_t=4073.07, p_s=-32.7（spec §5.5 中心点算例输入）
// 测试步骤：调用 SensitivityCoefficientsK0
// 期待结果（spec §5.5 第 3 步，使用精确值，容差 0.1%）：
//   - c(P7)  =  1 / (p_t - p_s)         = 1/4105.77 = 2.4356e-4
//   - c(p_t) =  (p_s - P7) / (p_t - p_s)² = -4108.05/4105.77² = -2.4369e-4
//   - c(p_s) =  (P7 - p_t) / (p_t - p_s)² = 2.28/4105.77² = 1.3525e-7
//
// 注：spec §5.5 算例值为 2.436/-2.437/1.352（取整到 4 位有效数字），实现用精确值。
func TestSensitivityCoefficientsK0(t *testing.T) {
	calc := NewUncertaintyCalculator(4073.07)

	p7, pTunnel, pStatic := 4075.35, 4073.07, -32.7
	cP7, cPt, cPs := calc.SensitivityCoefficientsK0(p7, pTunnel, pStatic)

	// c(P7) = 1 / (4073.07 - (-32.7)) = 1 / 4105.77 = 2.435597e-4
	if !approxEqualRelative(cP7, 2.435597e-4, 0.001) {
		t.Errorf("c(P7) 期望 2.435597e-4, 实际 %.6e", cP7)
	}

	// c(p_t) = (-32.7 - 4075.35) / 4105.77² = -4108.05 / 16857346 = -2.436949e-4
	if !approxEqualRelative(cPt, -2.436949e-4, 0.001) {
		t.Errorf("c(p_t) 期望 -2.436949e-4, 实际 %.6e", cPt)
	}

	// c(p_s) = (4075.35 - 4073.07) / 4105.77² = 2.28 / 16857346 = 1.352526e-7
	if !approxEqualRelative(cPs, 1.352526e-7, 0.001) {
		t.Errorf("c(p_s) 期望 1.352526e-7, 实际 %.6e", cPs)
	}
}

// ==================== 内区 Ks 灵敏系数测试（spec §5.3） ====================

// TestSensitivityCoefficientsKs 验证内区 Ks 灵敏系数
//
// Ks = (p_s - P̄) / (p_t - p_s)，P̄ = (P1+...+P6)/6
// 测试前置：构造 P1..P6 数据，p_t、p_s 已知
// 测试步骤：调用 SensitivityCoefficientsKs
// 期待结果（spec §5.3 公式）：
//   - c(P1..P6) = -1 / (6·(p_t - p_s))
//   - c(p_t)    = (P1+...+P6 - 6·p_s) / (6·(p_t - p_s)²)
//   - c(p_s)    = (6·p_t - P1-...-P6) / (6·(p_t - p_s)²)
func TestSensitivityCoefficientsKs(t *testing.T) {
	calc := NewUncertaintyCalculator(4073.07)

	// 构造 P1..P6：均值为 1000（任意值，仅验证公式）
	p1, p2, p3, p4, p5, p6 := 1000.0, 1010.0, 990.0, 1005.0, 995.0, 1000.0
	pTunnel, pStatic := 4073.07, -32.7

	cP1toP6, cPt, cPs := calc.SensitivityCoefficientsKs(p1, p2, p3, p4, p5, p6, pTunnel, pStatic)

	// c(P1..P6) = -1 / (6·(p_t - p_s)) = -1 / (6·4105.77) = -4.0595e-5
	expectedCPS := -1.0 / (6.0 * (pTunnel - pStatic))
	if !approxEqualRelative(cP1toP6, expectedCPS, sevenHoleUncEpsilon) {
		t.Errorf("c(P1..P6) 期望 %.6e, 实际 %.6e", expectedCPS, cP1toP6)
	}

	// c(p_t) = (P1+...+P6 - 6·p_s) / (6·(p_t - p_s)²)
	sumP := p1 + p2 + p3 + p4 + p5 + p6
	expectedCPt := (sumP - 6*pStatic) / (6.0 * (pTunnel - pStatic) * (pTunnel - pStatic))
	if !approxEqualRelative(cPt, expectedCPt, sevenHoleUncEpsilon) {
		t.Errorf("c(p_t) 期望 %.6e, 实际 %.6e", expectedCPt, cPt)
	}

	// c(p_s) = (6·p_t - P1-...-P6) / (6·(p_t - p_s)²)
	expectedCPs := (6*pTunnel - sumP) / (6.0 * (pTunnel - pStatic) * (pTunnel - pStatic))
	if !approxEqualRelative(cPs, expectedCPs, sevenHoleUncEpsilon) {
		t.Errorf("c(p_s) 期望 %.6e, 实际 %.6e", expectedCPs, cPs)
	}
}

// ==================== 合成公式测试（spec §5.2） ====================

// TestCombinedUncertaintyFormula 验证合成公式：|Σ cᵢu_B,i|（正确）vs Σ|cᵢ|uᵢ（错误）
//
// 测试前置：构造灵敏系数和 B 类不确定度，使 c1、c2 异号
// 测试步骤：调用 CombinedUncertainty
// 期待结果：合成公式正确使用 |Σ cᵢu_B,i|，符号相反的项部分抵消
//          若误用 Σ|cᵢ|uᵢ 会高估 46%（spec §5.5 第 4 步注释）
func TestCombinedUncertaintyFormula(t *testing.T) {
	calc := NewUncertaintyCalculator(4073.07)

	// spec §5.5 第 4 步数据：
	// c(P7) = 2.436e-4, c(p_t) = -2.437e-4, c(p_s) = 1.352e-7
	// u_B(P7) = 20.42, u_B(p_t) = 3.81, u_B(p_s) = 6.53
	// u_A(P7) = 0.894, u_A(p_t) = 1.342, u_A(p_s) = 0.671
	cValues := []float64{2.436e-4, -2.437e-4, 1.352e-7}
	uBValues := []float64{20.42, 3.81, 6.53}
	uAValues := []float64{0.894, 1.342, 0.671}

	uC := calc.CombinedUncertainty(cValues, uBValues, uAValues)

	// 正确公式：
	// Σ c·u_B = 2.436e-4×20.42 + (-2.437e-4)×3.81 + 1.352e-7×6.53
	//        = 4.974e-3 - 9.285e-4 + 8.83e-7 = 4.046e-3
	// |Σ c·u_B| = 4.046e-3
	// Σ c²·u_A² = (2.436e-4×0.894)² + (-2.437e-4×1.342)² + (1.352e-7×0.671)²
	//           = 4.748e-8 + 1.068e-7 + 8.22e-15 = 1.543e-7
	// √(Σ c²·u_A²) = 3.928e-4
	// u_c = √((4.046e-3)² + (3.928e-4)²) = √(1.637e-5 + 1.543e-7) = 4.065e-3
	if !approxEqualRelative(uC, 4.065e-3, 0.01) {
		t.Errorf("u_c 期望 4.065e-3, 实际 %.6e", uC)
	}

	// 验证不等于"绝对值的和"公式
	// Σ|c|·u_B = 4.974e-3 + 9.285e-4 + 8.83e-7 = 5.903e-3
	// u_c_wrong = √((5.903e-3)² + (3.928e-4)²) = 5.916e-3
	wrongSumAbs := math.Abs(cValues[0])*uBValues[0] + math.Abs(cValues[1])*uBValues[1] + math.Abs(cValues[2])*uBValues[2]
	aSquared := 0.0
	for i, c := range cValues {
		aSquared += c * c * uAValues[i] * uAValues[i]
	}
	uCWrong := math.Sqrt(wrongSumAbs*wrongSumAbs + aSquared)

	if approxEqualRelative(uC, uCWrong, 0.01) {
		t.Errorf("正确公式与错误公式结果相同 (%.6e), 应有 46%% 差异", uC)
	}

	// 验证差异约 46%（spec §5.5 第 4 步注释）
	diffPercent := math.Abs(uCWrong-uC) / uC * 100
	if diffPercent < 30 || diffPercent > 60 {
		t.Errorf("正确公式与错误公式差异 %.2f%%, 期望约 46%%", diffPercent)
	}
}

// TestUncertaintyFormulaComparison spec Task 17 Verification 公式对比测试入口
//
// 此测试是 Task 17 Verification 命令 `go test -v -run 'TestUncertaintyFormulaComparison'`
// 的入口，与上面的 TestCombinedUncertaintyFormula 形成互补——
// 上层测试覆盖更全面的边界场景（同号/异号），本测试聚焦 spec Task 17 字面要求：
// "对比验证：误用 Σ|cᵢ|uᵢ 公式会得到 5.903e-3（高估 46%），测试必须区分两种公式"。
//
// 测试前置：spec §5.5 中心点 K0 算例的灵敏系数与 B/A 类分量
// 测试步骤：
//  1. 用正确公式 |Σ cᵢ·u_B,i| 计算 u_c_correct（期望 4.065e-3）
//  2. 用错误公式 Σ|cᵢ|·u_B,i 计算 u_c_wrong（期望 5.903e-3）
//  3. 验证两公式结果差异约 46%（误用会显著高估不确定度）
//
// 期待结果：正确公式 4.065e-3，错误公式 5.903e-3，差异 45-47%
//          若 CombinedUncertainty 实现误用 Σ|cᵢ|uᵢ，正确公式结果会等于错误公式结果
func TestUncertaintyFormulaComparison(t *testing.T) {
	calc := NewUncertaintyCalculator(4073.07)

	// spec §5.5 第 4 步数据（中心点 K0 算例）
	cValues := []float64{2.436e-4, -2.437e-4, 1.352e-7}
	uBValues := []float64{20.42, 3.81, 6.53}
	uAValues := []float64{0.894, 1.342, 0.671}

	// 正确公式 u_c_correct = √(|Σ cᵢ·u_B,i|² + Σ cᵢ²·u_A,i²)
	uCCorrect := calc.CombinedUncertainty(cValues, uBValues, uAValues)

	// 错误公式 u_c_wrong = √((Σ|cᵢ|·u_B,i)² + Σ cᵢ²·u_A,i²)
	// 区别仅在 B 类分量的合成方式：正确公式保留符号求和后取绝对值，
	// 错误公式先取绝对值再求和——异号项不抵消，导致高估
	sumAbsCB := 0.0
	for i, c := range cValues {
		sumAbsCB += math.Abs(c) * uBValues[i]
	}
	aSquared := 0.0
	for i, c := range cValues {
		aSquared += c * c * uAValues[i] * uAValues[i]
	}
	uCWrong := math.Sqrt(sumAbsCB*sumAbsCB + aSquared)

	// 正确公式结果验证：u_c ≈ 4.065e-3（spec §5.5 第 6 步）
	if !approxEqualRelative(uCCorrect, 4.065e-3, 0.01) {
		t.Errorf("正确公式 u_c 期望 4.065e-3, 实际 %.6e", uCCorrect)
	}

	// 错误公式结果验证：u_c_wrong ≈ 5.903e-3（spec §5.5 第 4 步注释）
	// spec Task 17 Acceptance criteria: "误用 Σ|cᵢ|uᵢ 公式会得到 5.903e-3（高估 46%）"
	if !approxEqualRelative(uCWrong, 5.903e-3, 0.01) {
		t.Errorf("错误公式 u_c_wrong 期望 5.903e-3, 实际 %.6e", uCWrong)
	}

	// 公式区分验证：两公式结果必须不同（不能等价）
	if approxEqualRelative(uCCorrect, uCWrong, 0.01) {
		t.Errorf("正确公式 (%.6e) 与错误公式 (%.6e) 结果相同, CombinedUncertainty 可能误用 Σ|cᵢ|uᵢ",
			uCCorrect, uCWrong)
	}

	// 差异百分比验证：约 46%（spec §5.5 第 4 步注释）
	diffPercent := math.Abs(uCWrong-uCCorrect) / uCCorrect * 100
	if diffPercent < 40 || diffPercent > 50 {
		t.Errorf("公式差异 %.2f%%, 期望约 46%% (容差 40-50%%)", diffPercent)
	}

	// 方向性验证：错误公式必须高估（u_c_wrong > u_c_correct）
	// 异号项不抵消导致 Σ|cᵢ|uᵢ ≥ |Σ cᵢuᵢ|，错误公式结果必然 ≥ 正确公式
	if uCWrong <= uCCorrect {
		t.Errorf("错误公式 (%.6e) 应高估 (>正确公式 %.6e), 实际反而低估",
			uCWrong, uCCorrect)
	}
}

// TestCombinedUncertainty_SameSignCoefficients 验证灵敏系数同号时两种公式结果相同
//
// 测试前置：构造所有灵敏系数同号（全正）的数据
// 测试步骤：调用 CombinedUncertainty，并手算错误公式结果
// 期待结果：两公式结果相同（同号时 |Σ cᵢuᵢ| = Σ|cᵢ|uᵢ）
func TestCombinedUncertainty_SameSignCoefficients(t *testing.T) {
	calc := NewUncertaintyCalculator(4073.07)

	// 全正灵敏系数
	cValues := []float64{0.001, 0.002, 0.003}
	uBValues := []float64{10.0, 20.0, 30.0}
	uAValues := []float64{0.5, 0.6, 0.7}

	uC := calc.CombinedUncertainty(cValues, uBValues, uAValues)

	// 手算正确公式
	sumCB := 0.0
	for i, c := range cValues {
		sumCB += c * uBValues[i]
	}
	sumAbsCB := 0.0
	for i, c := range cValues {
		sumAbsCB += math.Abs(c) * uBValues[i]
	}
	aSq := 0.0
	for i, c := range cValues {
		aSq += c * c * uAValues[i] * uAValues[i]
	}
	expectedUC := math.Sqrt(sumCB*sumCB + aSq)

	if !approxEqualRelative(uC, expectedUC, sevenHoleUncEpsilon) {
		t.Errorf("u_c 期望 %.6e, 实际 %.6e", expectedUC, uC)
	}

	// 同号时正确公式 = 错误公式
	if !approxEqualRelative(sumCB, sumAbsCB, sevenHoleUncEpsilon) {
		t.Errorf("同号时 |Σ cᵢuᵢ| (%.6e) 应等于 Σ|cᵢ|uᵢ (%.6e)", sumCB, sumAbsCB)
	}
}

// ==================== 扩展不确定度测试（spec §5.4） ====================

// TestExpandedUncertainty 验证 U = k·u_c
//
// 测试前置：构造 u_c 和 k
// 测试步骤：调用 ExpandedUncertainty
// 期待结果：U = k × u_c
func TestExpandedUncertainty(t *testing.T) {
	calc := NewUncertaintyCalculator(4073.07)

	// 默认 k=2
	u := calc.ExpandedUncertainty(4.065e-3, 2.0)
	if !approxEqualRelative(u, 8.13e-3, sevenHoleUncEpsilon) {
		t.Errorf("U 期望 8.13e-3 (k=2), 实际 %.6e", u)
	}

	// k=1
	u1 := calc.ExpandedUncertainty(4.065e-3, 1.0)
	if !approxEqualRelative(u1, 4.065e-3, sevenHoleUncEpsilon) {
		t.Errorf("U 期望 4.065e-3 (k=1), 实际 %.6e", u1)
	}

	// k=3
	u3 := calc.ExpandedUncertainty(4.065e-3, 3.0)
	if !approxEqualRelative(u3, 1.2195e-2, sevenHoleUncEpsilon) {
		t.Errorf("U 期望 1.2195e-2 (k=3), 实际 %.6e", u3)
	}
}

// TestExpandedUncertainty_DefaultK 验证 k=0 或负数时使用默认 k=2
//
// spec §5.4 默认 k=2（95% 置信区间），传入非法 k 时回退到默认值
func TestExpandedUncertainty_DefaultK(t *testing.T) {
	calc := NewUncertaintyCalculator(4073.07)

	// k=0 应回退到默认 2.0
	u0 := calc.ExpandedUncertainty(4.065e-3, 0)
	if !approxEqualRelative(u0, 8.13e-3, sevenHoleUncEpsilon) {
		t.Errorf("k=0 时应回退到 k=2, U 期望 8.13e-3, 实际 %.6e", u0)
	}

	// k=-1 应回退到默认 2.0
	uNeg := calc.ExpandedUncertainty(4.065e-3, -1.0)
	if !approxEqualRelative(uNeg, 8.13e-3, sevenHoleUncEpsilon) {
		t.Errorf("k=-1 时应回退到 k=2, U 期望 8.13e-3, 实际 %.6e", uNeg)
	}
}

// ==================== 中心点完整算例测试（spec §5.5） ====================

// TestUncertaintyCenterPointK0 中心点 K0 不确定度 7 步算例复算（spec §5.5）
//
// 测试前置：spec §5.5 中心点输入
//   - P7 = 4075.35, p_t = 4073.07, p_s = -32.7, 大气压力 = 98880
//   - 样本标准差 S(P7)=2.0, S(p_t)=3.0, S(p_s)=1.5, N=5
//
// 测试步骤：完整执行 7 步算例（使用精确值计算，spec §5.5 表格值为取整值仅作参考）
// 期待结果：U(K0) 四舍五入到 3 位小数后 ∈ [0.004, 0.0082]（Task 4 验收规则）
//          精确计算值约 8.17e-3，四舍五入为 0.008，落在容差范围内
func TestUncertaintyCenterPointK0(t *testing.T) {
	// 输入数据（spec §5.5）
	p7, pTunnel, pStatic := 4075.35, 4073.07, -32.7
	samplesN := 5
	stdP7, stdPt, stdPs := 2.0, 3.0, 1.5

	calc := NewUncertaintyCalculator(pTunnel)

	// 第 1 步：B 类不确定度（按误差源分组，精确值）
	uBP7 := calc.BTypeForInput("P7")    // √(u1² + u3²) = 20.4124
	uBPt := calc.BTypeForInput("p_t")   // √(u2² + u3²) = 3.7232
	uBPs := calc.BTypeForInput("p_s")   // √(u4² + u3²) = 6.4550

	if !approxEqualRelative(uBP7, 20.4124, 0.002) {
		t.Errorf("第 1 步 u_B(P7) 期望 20.4124, 实际 %.4f", uBP7)
	}
	if !approxEqualRelative(uBPt, 3.7232, 0.002) {
		t.Errorf("第 1 步 u_B(p_t) 期望 3.7232, 实际 %.4f", uBPt)
	}
	if !approxEqualRelative(uBPs, 6.4550, 0.002) {
		t.Errorf("第 1 步 u_B(p_s) 期望 6.4550, 实际 %.4f", uBPs)
	}

	// 第 2 步：A 类不确定度（u(A,i) = S/√N）
	uAP7 := stdP7 / math.Sqrt(float64(samplesN)) // 2.0/√5 = 0.8944
	uAPt := stdPt / math.Sqrt(float64(samplesN)) // 3.0/√5 = 1.3416
	uAPs := stdPs / math.Sqrt(float64(samplesN)) // 1.5/√5 = 0.6708

	if !approxEqualRelative(uAP7, 0.8944, 0.001) {
		t.Errorf("第 2 步 u_A(P7) 期望 0.8944, 实际 %.4f", uAP7)
	}
	if !approxEqualRelative(uAPt, 1.3416, 0.001) {
		t.Errorf("第 2 步 u_A(p_t) 期望 1.3416, 实际 %.4f", uAPt)
	}
	if !approxEqualRelative(uAPs, 0.6708, 0.001) {
		t.Errorf("第 2 步 u_A(p_s) 期望 0.6708, 实际 %.4f", uAPs)
	}

	// 第 3 步：K0 灵敏系数（精确值）
	cP7, cPt, cPs := calc.SensitivityCoefficientsK0(p7, pTunnel, pStatic)
	if !approxEqualRelative(cP7, 2.435597e-4, 0.001) {
		t.Errorf("第 3 步 c(P7) 期望 2.435597e-4, 实际 %.6e", cP7)
	}
	if !approxEqualRelative(cPt, -2.436949e-4, 0.001) {
		t.Errorf("第 3 步 c(p_t) 期望 -2.436949e-4, 实际 %.6e", cPt)
	}
	if !approxEqualRelative(cPs, 1.352526e-7, 0.001) {
		t.Errorf("第 3 步 c(p_s) 期望 1.352526e-7, 实际 %.6e", cPs)
	}

	// 第 4-6 步：合成标准不确定度
	cValues := []float64{cP7, cPt, cPs}
	uBValues := []float64{uBP7, uBPt, uBPs}
	uAValues := []float64{uAP7, uAPt, uAPs}

	uC := calc.CombinedUncertainty(cValues, uBValues, uAValues)
	// 精确计算 u_c ≈ 4.083e-3（spec §5.5 算例用取整值得 4.065e-3）
	if !approxEqualRelative(uC, 4.083e-3, 0.01) {
		t.Errorf("第 6 步 u_c(K0) 期望 4.083e-3 (精确值), 实际 %.6e", uC)
	}

	// 第 7 步：扩展不确定度 U = k·u_c (k=2)
	uK0 := calc.ExpandedUncertainty(uC, 2.0)
	// 精确计算 U ≈ 8.166e-3

	// Task 4 验收规则：四舍五入到 3 位小数后比较
	// 8.166e-3 → 0.008，应落在 [0.004, 0.0082] 范围内
	roundedUK0 := math.Round(uK0*1000) / 1000
	if roundedUK0 < 0.004 || roundedUK0 > 0.0082 {
		t.Errorf("U(K0)=%.6e (四舍五入到 3 位小数=%.4f) 超出容差 [0.004, 0.0082]",
			uK0, roundedUK0)
	}

	// 验证 U(K0) 接近 spec §5.5 算例目标值 8.13e-3（容差 2%）
	if !approxEqualRelative(uK0, 8.13e-3, 0.02) {
		t.Errorf("U(K0) 期望接近 8.13e-3 (容差 2%%), 实际 %.6e", uK0)
	}
}

// TestUncertaintyCenterPointK0_StepByStep 验证中心点算例的中间值精度
//
// 比 TestUncertaintyCenterPointK0 更严格：验证第 4 步、第 5 步的中间值
//   - 第 4 步 |Σ c·u_B| = 4.046e-3
//   - 第 5 步 √(Σ c²·u_A²) = 3.928e-4
func TestUncertaintyCenterPointK0_StepByStep(t *testing.T) {
	calc := NewUncertaintyCalculator(4073.07)

	// spec §5.5 数据
	cValues := []float64{2.436e-4, -2.437e-4, 1.352e-7}
	uBValues := []float64{20.42, 3.81, 6.53}
	uAValues := []float64{0.894, 1.342, 0.671}

	// 第 4 步：Σ c·u_B = 4.046e-3（保留符号求和）
	sumCB := 0.0
	for i, c := range cValues {
		sumCB += c * uBValues[i]
	}
	if !approxEqualRelative(sumCB, 4.046e-3, 0.01) {
		t.Errorf("第 4 步 Σ c·u_B 期望 4.046e-3, 实际 %.6e", sumCB)
	}
	if sumCB < 0 {
		t.Errorf("第 4 步 Σ c·u_B 应为正值 (主项 c(P7)·u_B(P7) 占优), 实际 %.6e", sumCB)
	}

	// 第 5 步：√(Σ c²·u_A²) = 3.928e-4
	aSquared := 0.0
	for i, c := range cValues {
		aSquared += c * c * uAValues[i] * uAValues[i]
	}
	sqrtASq := math.Sqrt(aSquared)
	if !approxEqualRelative(sqrtASq, 3.928e-4, 0.01) {
		t.Errorf("第 5 步 √(Σ c²·u_A²) 期望 3.928e-4, 实际 %.6e", sqrtASq)
	}

	// 第 6 步：u_c = √(|Σ c·u_B|² + Σ c²·u_A²²) = 4.065e-3
	uC := calc.CombinedUncertainty(cValues, uBValues, uAValues)
	expectedUC := math.Sqrt(sumCB*sumCB + aSquared)
	if !approxEqualRelative(uC, expectedUC, sevenHoleUncEpsilon) {
		t.Errorf("第 6 步 u_c 期望 %.6e, 实际 %.6e", expectedUC, uC)
	}
}

// ==================== Kα/Kβ 灵敏系数基础测试 ====================

// TestSensitivityCoefficientsKAlpha_KnownCase 验证 Kα 灵敏系数可调用且不返回 NaN
//
// Kα = (Cpb + Cpc) / √3
// 其中 Cpb = (P5 - P2) / (P7 - P̄), Cpc = (P6 - P3) / (P7 - P̄)
// 链式求导涉及 P2/P3/P5/P6/P7/P1/P4（P̄ 依赖全部外围孔）
//
// 测试前置：构造对称数据（P1=P3=P5, P2=P4=P6=P7），Kα 应为 0
// 测试步骤：调用 SensitivityCoefficientsKAlpha
// 期待结果：不返回 NaN/Inf，且对称数据下系数有物理合理的对称性
func TestSensitivityCoefficientsKAlpha_KnownCase(t *testing.T) {
	calc := NewUncertaintyCalculator(4073.07)

	// 构造对称数据：P1=P3=P5, P2=P4=P6=P7
	// P̄ = (P1+P2+P3+P4+P5+P6)/6 = (P1+P2+P1+P2+P1+P2)/6 = (P1+P2)/2
	// denomAlphaBeta = P7 - P̄ = P7 - (P1+P2)/2
	// 假设 P7=2000, P1=P3=P5=1000, P2=P4=P6=2000
	// P̄ = (1000+2000+1000+2000+1000+2000)/6 = 9000/6 = 1500
	// denomAlphaBeta = 2000 - 1500 = 500
	// Cpb = (P5-P2)/denom = (1000-2000)/500 = -2
	// Cpc = (P6-P3)/denom = (2000-1000)/500 = 2
	// Kα = (Cpb+Cpc)/√3 = 0/√3 = 0
	raw := SevenHoleRawData{
		P1: 1000, P2: 2000, P3: 1000, P4: 2000, P5: 1000, P6: 2000, P7: 2000,
	}

	coeffs, err := calc.SensitivityCoefficientsKAlpha(raw)
	if err != nil {
		t.Fatalf("SensitivityCoefficientsKAlpha 返回错误: %v", err)
	}

	// 对称数据下 Kα=0，所有灵敏系数应为有限值
	if math.IsNaN(coeffs.CP2) || math.IsInf(coeffs.CP2, 0) {
		t.Errorf("c(P2) 不应为 NaN/Inf, 实际 %v", coeffs.CP2)
	}
	if math.IsNaN(coeffs.CP3) || math.IsInf(coeffs.CP3, 0) {
		t.Errorf("c(P3) 不应为 NaN/Inf, 实际 %v", coeffs.CP3)
	}
	if math.IsNaN(coeffs.CP5) || math.IsInf(coeffs.CP5, 0) {
		t.Errorf("c(P5) 不应为 NaN/Inf, 实际 %v", coeffs.CP5)
	}
	if math.IsNaN(coeffs.CP6) || math.IsInf(coeffs.CP6, 0) {
		t.Errorf("c(P6) 不应为 NaN/Inf, 实际 %v", coeffs.CP6)
	}
}

// TestSensitivityCoefficientsKBeta_KnownCase 验证 Kβ 灵敏系数可调用
//
// Kβ = -(2·Cpa + Cpb - Cpc) / 3
// 其中 Cpa = (P4 - P1) / (P7 - P̄), Cpb = (P5 - P2) / (P7 - P̄), Cpc = (P6 - P3) / (P7 - P̄)
func TestSensitivityCoefficientsKBeta_KnownCase(t *testing.T) {
	calc := NewUncertaintyCalculator(4073.07)

	raw := SevenHoleRawData{
		P1: 1000, P2: 2000, P3: 1000, P4: 2000, P5: 1000, P6: 2000, P7: 2000,
	}

	coeffs, err := calc.SensitivityCoefficientsKBeta(raw)
	if err != nil {
		t.Fatalf("SensitivityCoefficientsKBeta 返回错误: %v", err)
	}

	// 验证所有字段为有限值
	fields := map[string]float64{
		"c(P1)": coeffs.CP1, "c(P2)": coeffs.CP2, "c(P3)": coeffs.CP3,
		"c(P4)": coeffs.CP4, "c(P5)": coeffs.CP5, "c(P6)": coeffs.CP6,
		"c(P7)": coeffs.CP7,
	}
	for name, val := range fields {
		if math.IsNaN(val) || math.IsInf(val, 0) {
			t.Errorf("%s 不应为 NaN/Inf, 实际 %v", name, val)
		}
	}
}

// TestSensitivityCoefficientsKAlpha_DivZero 验证 P7-P̄≈0 时返回错误
//
// 测试前置：构造 P7 = (P1+P2+P3+P4+P5+P6)/6（分母为零）
// 测试步骤：调用 SensitivityCoefficientsKAlpha
// 期待结果：返回错误（避免除零）
func TestSensitivityCoefficientsKAlpha_DivZero(t *testing.T) {
	calc := NewUncertaintyCalculator(4073.07)

	// P7 = P̄ = 1500 → denomAlphaBeta = 0
	raw := SevenHoleRawData{
		P1: 1000, P2: 2000, P3: 1000, P4: 2000, P5: 1000, P6: 2000, P7: 1500,
	}

	_, err := calc.SensitivityCoefficientsKAlpha(raw)
	if err == nil {
		t.Error("P7-P̄≈0 时应返回错误, 实际无错误")
	}
}
