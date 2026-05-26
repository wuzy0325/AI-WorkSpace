package calibration

import (
	"math"
	"testing"
)

// 测试精度容差
const epsilon = 1e-6

func TestStdDev(t *testing.T) {
	tests := []struct {
		name     string
		values   []float64
		expected float64
	}{
		{"空切片", []float64{}, 0},
		{"单元素", []float64{5.0}, 0},
		{"相同值", []float64{3.0, 3.0, 3.0}, 0},
		{"两个值", []float64{1.0, 3.0}, math.Sqrt(2)},
		{"多个值", []float64{2.0, 4.0, 4.0, 4.0, 5.0, 5.0, 7.0, 9.0}, 2.14},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := StdDev(tt.values)
			if math.Abs(result-tt.expected) > 0.01 {
				t.Errorf("StdDev(%v) = %v, 期望 %v", tt.values, result, tt.expected)
			}
		})
	}
}

func TestMean(t *testing.T) {
	tests := []struct {
		name     string
		values   []float64
		expected float64
	}{
		{"空切片", []float64{}, 0},
		{"单元素", []float64{5.0}, 5.0},
		{"多个值", []float64{1.0, 2.0, 3.0, 4.0, 5.0}, 3.0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Mean(tt.values)
			if math.Abs(result-tt.expected) > epsilon {
				t.Errorf("Mean(%v) = %v, 期望 %v", tt.values, result, tt.expected)
			}
		})
	}
}

// ==================== 五孔探针系数测试 ====================

func TestCalculateFiveHoleCoefficients(t *testing.T) {
	// 基本测试：对称数据，Kα 和 Kβ 应为0
	pTotal := 500.0
	pStatic := 100.0
	data := FiveHoleRawData{
		P1:      100.0, // 下孔
		P2:      200.0, // 中心孔
		P3:      100.0, // 上孔
		P4:      100.0, // 左孔
		P5:      100.0, // 右孔
		PAtm:    101325.0,
		TAtm:    20.0,
		PTotal:  &pTotal,
		PStatic: &pStatic,
	}

	coeffs := CalculateFiveHoleCoefficients(data)

	// 对称数据：Kα = (P4-P5)/qRef = 0, Kβ = (P3-P1)/qRef = 0
	if math.Abs(coeffs.Kalpha) > epsilon {
		t.Errorf("对称数据 Kalpha 应为0, 实际 %v", coeffs.Kalpha)
	}
	if math.Abs(coeffs.Kbeta) > epsilon {
		t.Errorf("对称数据 Kbeta 应为0, 实际 %v", coeffs.Kbeta)
	}

	// 非对称测试：P4 > P5，Kα 应为正
	data2 := FiveHoleRawData{
		P1:   100.0,
		P2:   200.0,
		P3:   100.0,
		P4:   150.0, // 左孔压力更大
		P5:   50.0,  // 右孔压力更小
		PAtm: 101325.0,
		TAtm: 20.0,
	}

	coeffs2 := CalculateFiveHoleCoefficients(data2)

	// Kα = (150 - 50) / (200 - (100+100+150+50)/4) = 100 / (200 - 100) = 1.0
	if math.Abs(coeffs2.Kalpha-1.0) > epsilon {
		t.Errorf("Kalpha 期望 1.0, 实际 %v", coeffs2.Kalpha)
	}

	// Kβ = (100 - 100) / 100 = 0
	if math.Abs(coeffs2.Kbeta) > epsilon {
		t.Errorf("Kbeta 期望 0, 实际 %v", coeffs2.Kbeta)
	}
}

func TestCalculateFiveHoleCoefficients_WithMachNumber(t *testing.T) {
	pTotal := 106891.0 // 总压（表压+大气压）
	pStatic := 101325.0
	data := FiveHoleRawData{
		P1:      100.0,
		P2:      200.0,
		P3:      100.0,
		P4:      100.0,
		P5:      100.0,
		PAtm:    101325.0,
		TAtm:    20.0,
		PTotal:  &pTotal,
		PStatic: &pStatic,
	}

	coeffs := CalculateFiveHoleCoefficients(data)

	if coeffs.MachNumber == nil {
		t.Fatal("马赫数不应为 nil")
	}

	// Ma = sqrt(5 * ((106891/101325)^(2/7) - 1))
	// 实际计算约 0.197
	ma := *coeffs.MachNumber
	if ma < 0.15 || ma > 0.3 {
		t.Errorf("马赫数应在 0.15~0.3 范围内, 实际 %v", ma)
	}
}

func TestCalculateFiveHoleAverage(t *testing.T) {
	pTotal1 := 500.0
	pTotal2 := 600.0

	data1 := FiveHoleRawData{P1: 100, P2: 200, P3: 300, P4: 400, P5: 500, PAtm: 101325, TAtm: 20, PTotal: &pTotal1}
	data2 := FiveHoleRawData{P1: 200, P2: 300, P3: 400, P4: 500, P5: 600, PAtm: 101326, TAtm: 22, PTotal: &pTotal2}

	avg := CalculateFiveHoleAverage([]FiveHoleRawData{data1, data2})

	if math.Abs(avg.P1-150) > epsilon {
		t.Errorf("P1 平均值期望 150, 实际 %v", avg.P1)
	}
	if math.Abs(avg.P2-250) > epsilon {
		t.Errorf("P2 平均值期望 250, 实际 %v", avg.P2)
	}
	if avg.PTotal == nil {
		t.Fatal("PTotal 不应为 nil")
	}
	if math.Abs(*avg.PTotal-550) > epsilon {
		t.Errorf("PTotal 平均值期望 550, 实际 %v", *avg.PTotal)
	}
}

// ==================== 三孔探针系数测试 ====================

func TestCalculateThreeHoleCoefficients(t *testing.T) {
	pTotal := 500.0
	data := ThreeHoleRawData{
		P1:     300.0, // 中心孔
		P2:     350.0, // 侧孔1
		P3:     250.0, // 侧孔2
		PAtm:   101325.0,
		PTotal: &pTotal,
	}

	_ = CalculateThreeHoleCoefficients(data)

	// K = (P2 - P3) / (P1 - PAtm) = (350 - 250) / (300 - 101325)
	// 注意：这里 P1 - PAtm 是一个非常大的负数，实际场景中 P1 应该是表压
	// 让我们用表压场景重新测试
}

func TestCalculateThreeHoleCoefficients_GaugePressure(t *testing.T) {
	// 使用表压值（相对大气压）
	pTotal := 500.0
	data := ThreeHoleRawData{
		P1:     300.0, // 中心孔表压
		P2:     350.0, // 侧孔1表压
		P3:     250.0, // 侧孔2表压
		PAtm:   0.0,   // 大气压参考为0（表压模式）
		PTotal: &pTotal,
	}

	coeffs := CalculateThreeHoleCoefficients(data)

	// K = (350 - 250) / (300 - 0) = 100/300 ≈ 0.333
	expectedK := 100.0 / 300.0
	if math.Abs(coeffs.K-expectedK) > epsilon {
		t.Errorf("K 期望 %v, 实际 %v", expectedK, coeffs.K)
	}

	// Cv = (300 - 0) / (500 - 0) = 0.6
	expectedCv := 300.0 / 500.0
	if math.Abs(coeffs.Cv-expectedCv) > epsilon {
		t.Errorf("Cv 期望 %v, 实际 %v", expectedCv, coeffs.Cv)
	}
}

// ==================== 总压探针系数测试 ====================

func TestCalculateTotalPressureCoefficients(t *testing.T) {
	data := TotalPressureRawData{
		PAtm:          101325.0,
		TAtm:          20.0,
		PTunnelTotal:  5000.0, // 风洞总压（表压）
		PTunnelStatic: 3000.0, // 风洞静压（表压）
		TTunnel:       25.0,
		PProbeTotal:   4900.0, // 探针总压（表压）
	}

	coeffs := CalculateTotalPressureCoefficients(data)

	// CPT = PProbeTotal / PTunnelTotal = 4900 / 5000 = 0.98
	if math.Abs(coeffs.CPT-0.98) > epsilon {
		t.Errorf("CPT 期望 0.98, 实际 %v", coeffs.CPT)
	}

	// 误差 = (4900 - 5000) / (5000 + 101325) * 100 = -100 / 106325 * 100 ≈ -0.094%
	expectedError := -100.0 / 106325.0 * 100
	if math.Abs(coeffs.Error-expectedError) > 0.01 {
		t.Errorf("误差期望 %v%%, 实际 %v%%", expectedError, coeffs.Error)
	}

	// 马赫数应大于0
	if coeffs.MachNumber <= 0 {
		t.Errorf("马赫数应大于0, 实际 %v", coeffs.MachNumber)
	}
}

// ==================== 总温探针公式测试 ====================

func TestCalculateMachNumber(t *testing.T) {
	// 标准大气条件下的马赫数计算
	// Ma = sqrt(5 * ((Pt/Ps)^(2/7) - 1))
	totalPressure := 106891.0  // Pa (约1.055atm)
	staticPressure := 101325.0 // Pa (1atm)

	ma, err := CalculateMachNumber(totalPressure, staticPressure)
	if err != nil {
		t.Fatalf("计算马赫数失败: %v", err)
	}

	// 预期马赫数约0.197
	if ma < 0.15 || ma > 0.3 {
		t.Errorf("马赫数应在 0.15~0.3 范围内, 实际 %v", ma)
	}
}

func TestCalculateMachNumber_InvalidInput(t *testing.T) {
	// 静压为0
	_, err := CalculateMachNumber(101325, 0)
	if err == nil {
		t.Error("静压为0时应返回错误")
	}

	// 总压小于静压
	_, err = CalculateMachNumber(50000, 101325)
	if err == nil {
		t.Error("总压小于静压时应返回错误")
	}
}

func TestCalculateRecoveryCoefficient(t *testing.T) {
	// 测试温度和标准温度相同 → r = 1.0
	r, err := CalculateRecoveryCoefficient(20.0, 20.0)
	if err != nil {
		t.Fatalf("计算恢复系数失败: %v", err)
	}
	if math.Abs(r-1.0) > epsilon {
		t.Errorf("相同温度时恢复系数应为1.0, 实际 %v", r)
	}

	// 测试温度低于标准温度 → r < 1.0
	r2, err := CalculateRecoveryCoefficient(19.0, 20.0)
	if err != nil {
		t.Fatalf("计算恢复系数失败: %v", err)
	}
	if r2 >= 1.0 {
		t.Errorf("测试温度低于标准温度时恢复系数应小于1.0, 实际 %v", r2)
	}
}

func TestCheckTemperatureStability(t *testing.T) {
	// 稳定数据
	stable, stdDev := CheckTemperatureStability([]float64{20.0, 20.01, 19.99, 20.0, 20.005}, 0.1)
	if !stable {
		t.Error("稳定数据应判定为稳定")
	}
	if stdDev > 0.1 {
		t.Errorf("标准差应小于0.1, 实际 %v", stdDev)
	}

	// 不稳定数据
	stable2, _ := CheckTemperatureStability([]float64{18.0, 22.0, 19.0, 21.0, 20.0}, 0.1)
	if stable2 {
		t.Error("不稳定数据应判定为不稳定")
	}

	// 数据不足
	stable3, _ := CheckTemperatureStability([]float64{20.0}, 0.1)
	if stable3 {
		t.Error("单点数据应判定为不稳定")
	}
}

// ==================== 大气数据计算器测试 ====================

func TestAtmosphericDataCalculator_Mach(t *testing.T) {
	calc := NewAtmosphericDataCalculator()

	// 文档验证数据：Pt=95934, Ps=95495.4 → Ma≈0.084
	ma, err := calc.CalculateMach(95934, 95495.4)
	if err != nil {
		t.Fatalf("计算马赫数失败: %v", err)
	}
	if ma < 0.07 || ma > 0.10 {
		t.Errorf("马赫数应在 0.07~0.10 范围内, 实际 %v", ma)
	}

	// 标准大气条件
	ma2, err := calc.CalculateMach(106891, 101325)
	if err != nil {
		t.Fatalf("计算马赫数失败: %v", err)
	}
	if ma2 < 0.15 || ma2 > 0.3 {
		t.Errorf("马赫数应在 0.15~0.3 范围内, 实际 %v", ma2)
	}
}

func TestAtmosphericDataCalculator_Mach_InvalidInput(t *testing.T) {
	calc := NewAtmosphericDataCalculator()

	_, err := calc.CalculateMach(101325, 0)
	if err == nil {
		t.Error("静压为0时应返回错误")
	}

	_, err = calc.CalculateMach(50000, 101325)
	if err == nil {
		t.Error("总压小于静压时应返回错误")
	}
}

func TestAtmosphericDataCalculator_SAT(t *testing.T) {
	calc := NewAtmosphericDataCalculator()

	// TAT=295.35K, Ma≈0.084, r=1.0 → SAT≈294.94K
	sat := calc.CalculateSAT(295.35, 0.084)
	if sat < 294.0 || sat > 296.0 {
		t.Errorf("静温应在 294~296 K 范围内, 实际 %v", sat)
	}

	// Ma=0 时 SAT = TAT
	sat2 := calc.CalculateSAT(300.0, 0.0)
	if math.Abs(sat2-300.0) > epsilon {
		t.Errorf("Ma=0时静温应等于总温, 实际 %v", sat2)
	}
}

func TestAtmosphericDataCalculator_Qc(t *testing.T) {
	calc := NewAtmosphericDataCalculator()

	// Qc = Pt - Ps = 95934 - 95495.4 ≈ 438.6
	qc := calc.CalculateQc(95934, 95495.4)
	if math.Abs(qc-438.6) > 1.0 {
		t.Errorf("动压期望约438.6, 实际 %v", qc)
	}
}

func TestAtmosphericDataCalculator_CAS(t *testing.T) {
	calc := NewAtmosphericDataCalculator()

	// 小动压下的CAS计算
	qc := calc.CalculateQc(95934, 95495.4)
	cas := calc.CalculateCAS(qc)
	if cas <= 0 {
		t.Errorf("校正空速应大于0, 实际 %v", cas)
	}
}

func TestAtmosphericDataCalculator_TAS_Method1(t *testing.T) {
	calc := NewAtmosphericDataCalculator()

	// 文档验证数据
	Pt := 95934.0
	Ps := 95495.4
	TAT := 295.35

	ma, _ := calc.CalculateMach(Pt, Ps)
	sat := calc.CalculateSAT(TAT, ma)
	qc := calc.CalculateQc(Pt, Ps)
	tas := calc.CalculateTASByDensity(Ps, qc, sat)

	// 文档验证值: vt(m/s) ≈ 28.92
	if tas < 25.0 || tas > 35.0 {
		t.Errorf("真空速(方法1)应在 25~35 m/s 范围内, 实际 %v", tas)
	}
}

func TestAtmosphericDataCalculator_TAS_Method2(t *testing.T) {
	calc := NewAtmosphericDataCalculator()

	// 文档验证数据
	Pt := 95934.0
	Ps := 95495.4
	TAT := 295.35

	ma, _ := calc.CalculateMach(Pt, Ps)
	sat := calc.CalculateSAT(TAT, ma)
	tas := calc.CalculateTASByMach(ma, sat)

	// 文档验证值: vt(m/s) ≈ 28.92
	if tas < 25.0 || tas > 35.0 {
		t.Errorf("真空速(方法2)应在 25~35 m/s 范围内, 实际 %v", tas)
	}
}

func TestAtmosphericDataCalculator_CalculateAll(t *testing.T) {
	calc := NewAtmosphericDataCalculator()

	result, err := calc.CalculateAll(95934, 95495.4, 295.35)
	if err != nil {
		t.Fatalf("完整计算失败: %v", err)
	}

	if result.MachNumber < 0.07 || result.MachNumber > 0.10 {
		t.Errorf("马赫数异常: %v", result.MachNumber)
	}
	if result.SAT < 294.0 || result.SAT > 296.0 {
		t.Errorf("静温异常: %v", result.SAT)
	}
	if result.Qc < 400 || result.Qc > 500 {
		t.Errorf("动压异常: %v", result.Qc)
	}
	if result.CAS <= 0 {
		t.Errorf("校正空速应大于0: %v", result.CAS)
	}
	if result.TASDensity < 25 || result.TASDensity > 35 {
		t.Errorf("真空速(方法1)异常: %v", result.TASDensity)
	}
	if result.TASMach < 25 || result.TASMach > 35 {
		t.Errorf("真空速(方法2)异常: %v", result.TASMach)
	}
}
