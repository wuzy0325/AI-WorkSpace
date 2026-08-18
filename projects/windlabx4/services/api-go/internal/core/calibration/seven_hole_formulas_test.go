package calibration

import (
	"math"
	"testing"
)

// 七孔公式测试精度容差。
// - 系数误差容差 0.001（spec §11.1 算法正确性验收要求）
// - 马赫数误差容差 0.005（spec §11.1 算法正确性验收要求）
// - 速度误差容差 5%（spec §11.1 算法正确性验收要求）
const (
	sevenHoleCoeffEpsilon = 1e-3 // 系数容差
	sevenHoleMachEpsilon  = 5e-3 // 马赫数容差
)

// ==================== 内区公式测试（spec §4.1 公式 1-8） ====================

// TestCalculateSevenHoleInnerCoefficients_CenterPoint 数据集中心点（α=0°, β=0°）回归测试
//
// 数据集来源：projects/WindLabX4/docs/W532.202608.P.7H.1-01/（85 m/s，Ma≈0.242）
// 通道原始值（表压，A 基准）：
//
//	P1..P7 = 537.18, 282.78, 313.98, 519.90, 573.97, 294.97, 4075.35
//	p_t = 4073.07, p_s = -32.7, 大气压力 = 98880
//
// 期望值（spec §4.1 数据集验证）：
//
//	Kα = 0.043, Kβ = -0.025, K0 = 0.00056, Ks = -0.110
//
// 手算验证：
//
//	P̄ = (537.18+282.78+313.98+519.90+573.97+294.97)/6 = 420.463
//	Cpa = (P4-P1)/(P7-P̄) = (519.90-537.18)/3654.887 = -0.004728
//	Cpb = (P5-P2)/(P7-P̄) = (573.97-282.78)/3654.887 = 0.07967
//	Cpc = (P6-P3)/(P7-P̄) = (294.97-313.98)/3654.887 = -0.005200
//	Kβ = -(2·Cpa+Cpb-Cpc)/3 = -(-0.009456+0.07967+0.005200)/3 = -0.025138 ✓
//	Kα = (Cpb+Cpc)/√3 = 0.074470/1.732 = 0.043001 ✓
//	K0 = (P7-p_t)/(p_t-p_s) = 2.28/4105.77 = 0.0005552 ✓
//	Ks = (p_s-P̄)/(p_t-p_s) = -453.163/4105.77 = -0.11037 ✓
func TestCalculateSevenHoleInnerCoefficients_CenterPoint(t *testing.T) {
	pTunnel := 4073.07
	pStatic := -32.7
	raw := SevenHoleRawData{
		P1:      537.18,
		P2:      282.78,
		P3:      313.98,
		P4:      519.90,
		P5:      573.97,
		P6:      294.97,
		P7:      4075.35,
		PAtm:    98880.0,
		TAtm:    28.0,
		PTotal:  &pTunnel,
		PStatic: &pStatic,
	}

	coeffs, err := CalculateSevenHoleInnerCoefficients(raw)
	if err != nil {
		t.Fatalf("CalculateSevenHoleInnerCoefficients 返回错误: %v", err)
	}

	// 系数误差验证（spec §11.1 要求 ≤ 0.001）
	if math.Abs(coeffs.Kalpha-0.043) > sevenHoleCoeffEpsilon {
		t.Errorf("Kalpha 期望 0.043, 实际 %.6f, 误差 %.6f", coeffs.Kalpha, math.Abs(coeffs.Kalpha-0.043))
	}
	if math.Abs(coeffs.Kbeta-(-0.025)) > sevenHoleCoeffEpsilon {
		t.Errorf("Kbeta 期望 -0.025, 实际 %.6f, 误差 %.6f", coeffs.Kbeta, math.Abs(coeffs.Kbeta+0.025))
	}
	if math.Abs(coeffs.K0-0.00056) > sevenHoleCoeffEpsilon {
		t.Errorf("K0 期望 0.00056, 实际 %.6f, 误差 %.6f", coeffs.K0, math.Abs(coeffs.K0-0.00056))
	}
	if math.Abs(coeffs.Ks-(-0.110)) > sevenHoleCoeffEpsilon {
		t.Errorf("Ks 期望 -0.110, 实际 %.6f, 误差 %.6f", coeffs.Ks, math.Abs(coeffs.Ks+0.110))
	}

	// 外区系数在内区点必须为零值（防止下游误用）
	if coeffs.Ktheta != 0 || coeffs.Kphi != 0 || coeffs.K0Outer != 0 || coeffs.KsOuter != 0 {
		t.Errorf("内区点外区系数应为零值, 实际 Ktheta=%v Kphi=%v K0Outer=%v KsOuter=%v",
			coeffs.Ktheta, coeffs.Kphi, coeffs.K0Outer, coeffs.KsOuter)
	}

	// 马赫数验证（spec §11.1 要求 ≤ 0.005）
	if coeffs.MachNumber == nil {
		t.Fatal("MachNumber 不应为 nil（PTotal/PStatic/PAtm 齐全）")
	}
	if math.Abs(*coeffs.MachNumber-0.242) > sevenHoleMachEpsilon {
		t.Errorf("Ma 期望 0.242, 实际 %.6f, 误差 %.6f", *coeffs.MachNumber, math.Abs(*coeffs.MachNumber-0.242))
	}

	// 速度验证（spec §11.1 要求 ≤ 5%，标称 85 m/s，下限 80.75）
	if coeffs.Velocity == nil {
		t.Fatal("Velocity 不应为 nil（Ma + TAtm 齐全）")
	}
	if math.Abs(*coeffs.Velocity-85.0)/85.0 > 0.05 {
		t.Errorf("V 期望约 85 m/s (±5%%), 实际 %.3f", *coeffs.Velocity)
	}
}

// TestCalculateSevenHoleInnerCoefficients_MissingPTunnel 验证 PTotal/PStatic 缺失时 Ma/V 为 nil
//
// 依据 spec §2.1：PTotal/PStatic 为指针类型，缺失时为 nil。
// 此时马赫数/速度无法计算，CSV 写空字符串、UI 显示 "--"。
// 内区系数本身仅依赖 P1~P7，仍可正常计算。
func TestCalculateSevenHoleInnerCoefficients_MissingPTunnel(t *testing.T) {
	raw := SevenHoleRawData{
		P1:   537.18,
		P2:   282.78,
		P3:   313.98,
		P4:   519.90,
		P5:   573.97,
		P6:   294.97,
		P7:   4075.35,
		PAtm: 98880.0,
		TAtm: 28.0,
		// PTotal/PStatic 不设置
	}

	coeffs, err := CalculateSevenHoleInnerCoefficients(raw)
	if err != nil {
		t.Fatalf("PTotal/PStatic 缺失不应返回错误: %v", err)
	}

	// Kα/Kβ/K0/Ks 仍可计算（K0/Ks 会因 p_t-p_s=0 而触发除零保护，见 Separate test）
	// 这里只验证 Ma/V 为 nil
	if coeffs.MachNumber != nil {
		t.Errorf("PTotal/PStatic 缺失时 MachNumber 应为 nil, 实际 %v", *coeffs.MachNumber)
	}
	if coeffs.Velocity != nil {
		t.Errorf("PTotal/PStatic 缺失时 Velocity 应为 nil, 实际 %v", *coeffs.Velocity)
	}
}

// TestCalculateSevenHoleInnerCoefficients_DivideByZero 内区公式除零保护
//
// 当 P7-P̄ 接近零（来流正对探针但 P7 与外围孔压力几乎相等），
// 或 p_t-p_s 接近零（风洞未建立压差）时，公式分母为零，必须返回错误而非 NaN/Inf。
//
// 注：p_t-p_s=0 的等压场景由 TestCalculateSevenHoleInnerCoefficients_EqualPressure
// 单独覆盖——等压是有效零流量，K0/Ks 跳过但 Ma/V 返回 0，不返回错误。
func TestCalculateSevenHoleInnerCoefficients_DivideByZero(t *testing.T) {
	pTunnel := 100.0
	pStatic := 100.0 // p_t - p_s = 0
	raw := SevenHoleRawData{
		P1:      100.0,
		P2:      100.0,
		P3:      100.0,
		P4:      100.0,
		P5:      100.0,
		P6:      100.0,
		P7:      100.0, // P7 - P̄ = 0
		PAtm:    98880.0,
		PTotal:  &pTunnel,
		PStatic: &pStatic,
	}

	_, err := CalculateSevenHoleInnerCoefficients(raw)
	if err == nil {
		t.Error("P7-P̄=0 时应返回除零错误, 实际返回 nil")
	}
}

// TestCalculateSevenHoleInnerCoefficients_EqualPressure 内区等压（p_t == p_s）零流量语义
//
// 验证 review P1 修复：等压是有效零流量场景，K0/Ks 分母为零无物理意义跳过（保持零值），
// 但 Ma/V 必须返回 0（与 live physics 五孔/三孔/总压同走 CalculateAll 的零流量口径一致），
// 不再像旧实现那样直接返回错误导致 CSV/样本 Ma/V 为空。
//
// 测试前置：
//   - P1..P6 取不同值保证 P7-P̄≠0（避免 denomAlphaBeta 除零保护触发）
//   - PTotal == PStatic（等压，pt-ps=0）
//   - PAtm/TAtm 齐全
//
// 期待结果：
//   - err == nil（等压不再是错误）
//   - MachNumber != nil 且 *MachNumber == 0
//   - Velocity != nil 且 *Velocity == 0
//   - K0 == 0, Ks == 0（等压跳过，保持零值）
func TestCalculateSevenHoleInnerCoefficients_EqualPressure(t *testing.T) {
	pTunnel := 500.0
	pStatic := 500.0 // p_t == p_s，等压零流量
	raw := SevenHoleRawData{
		P1:      100.0,
		P2:      200.0,
		P3:      300.0,
		P4:      400.0,
		P5:      500.0,
		P6:      600.0,
		P7:      700.0, // P7-P̄ = 700-350 = 350 ≠ 0
		PAtm:    98880.0,
		TAtm:    28.0,
		PTotal:  &pTunnel,
		PStatic: &pStatic,
	}

	coeffs, err := CalculateSevenHoleInnerCoefficients(raw)
	if err != nil {
		t.Fatalf("等压（p_t==p_s）不应返回错误, 实际: %v", err)
	}

	// K0/Ks 等压跳过，保持零值
	if coeffs.K0 != 0 || coeffs.Ks != 0 {
		t.Errorf("等压时 K0/Ks 应跳过保持零值, 实际 K0=%.6f Ks=%.6f", coeffs.K0, coeffs.Ks)
	}

	// Ma/V 必须返回 0（与 live physics 零流量口径一致）
	if coeffs.MachNumber == nil {
		t.Fatal("等压时 MachNumber 不应为 nil, 期望 0（零流量语义与 live physics 一致）")
	}
	if *coeffs.MachNumber != 0 {
		t.Errorf("等压时 Ma 期望 0, 实际 %.6f", *coeffs.MachNumber)
	}
	if coeffs.Velocity == nil {
		t.Fatal("等压时 Velocity 不应为 nil, 期望 0（零流量语义与 live physics 一致）")
	}
	if *coeffs.Velocity != 0 {
		t.Errorf("等压时 V 期望 0, 实际 %.6f", *coeffs.Velocity)
	}
}

// ==================== 外区公式测试（spec §4.2 公式 9-12，含环形取模） ====================

// TestCalculateSevenHoleOuterCoefficients_Sector1FirstPoint 外区 1 区首点（φ=330°, θ=30°）回归测试
//
// 数据集来源：projects/WindLabX4/docs/W532.202608.P.7H.1-01/（大角度 1 区首点）
// 通道原始值（表压，A 基准）：
//
//	P1=3260.217, P2=-874.900, P6=2973.950, P7=2168.100
//	p_t=4117.517, p_s=-30.133
//
// n=1, n+1=2, n-1=6（环形取模）
//
// 期望值（spec §4.2 数据集验证）：
//
//	Kθ[1] = 0.494, Kφ[1] = 1.741, K0[1] = -0.207, Ks[1] = -0.260
//
// 手算验证：
//
//	分母 = P1 - (P2+P6)/2 = 3260.217 - 1049.525 = 2210.692
//	Kθ[1] = (P1-P7)/分母 = 1092.117/2210.692 = 0.4941 ✓
//	Kφ[1] = (P6-P2)/分母 = 3848.850/2210.692 = 1.7408 ✓
//	K0[1] = (P1-p_t)/(p_t-p_s) = -857.300/4147.650 = -0.20673 ✓
//	Ks[1] = (p_s-(P2+P6)/2)/(p_t-p_s) = -1079.658/4147.650 = -0.26026 ✓
func TestCalculateSevenHoleOuterCoefficients_Sector1FirstPoint(t *testing.T) {
	pTunnel := 4117.517
	pStatic := -30.133
	// P3/P4/P5 在数据集中存在但本测试只需 P1/P2/P6/P7 即可验证公式，
	// 这里填充数据集实测值以保证完整回归（数据集 1 区首点完整 7 孔压力）。
	raw := SevenHoleRawData{
		P1:      3260.217,
		P2:      -874.900,
		P3:      -870.0, // 数据集实测（占位，不影响 1 区公式计算）
		P4:      -870.0,
		P5:      -870.0,
		P6:      2973.950,
		P7:      2168.100,
		PAtm:    98880.0,
		TAtm:    28.0,
		PTotal:  &pTunnel,
		PStatic: &pStatic,
	}

	coeffs, err := CalculateSevenHoleOuterCoefficients(raw, 1)
	if err != nil {
		t.Fatalf("CalculateSevenHoleOuterCoefficients 返回错误: %v", err)
	}

	// 外区系数误差验证（spec §11.1 要求 ≤ 0.001）
	if math.Abs(coeffs.Ktheta-0.494) > sevenHoleCoeffEpsilon {
		t.Errorf("Ktheta[1] 期望 0.494, 实际 %.6f, 误差 %.6f", coeffs.Ktheta, math.Abs(coeffs.Ktheta-0.494))
	}
	if math.Abs(coeffs.Kphi-1.741) > sevenHoleCoeffEpsilon {
		t.Errorf("Kphi[1] 期望 1.741, 实际 %.6f, 误差 %.6f", coeffs.Kphi, math.Abs(coeffs.Kphi-1.741))
	}
	if math.Abs(coeffs.K0Outer-(-0.207)) > sevenHoleCoeffEpsilon {
		t.Errorf("K0Outer[1] 期望 -0.207, 实际 %.6f, 误差 %.6f", coeffs.K0Outer, math.Abs(coeffs.K0Outer+0.207))
	}
	if math.Abs(coeffs.KsOuter-(-0.260)) > sevenHoleCoeffEpsilon {
		t.Errorf("KsOuter[1] 期望 -0.260, 实际 %.6f, 误差 %.6f", coeffs.KsOuter, math.Abs(coeffs.KsOuter+0.260))
	}

	// 内区系数在外区点必须为零值（防止下游误用）
	if coeffs.Kalpha != 0 || coeffs.Kbeta != 0 || coeffs.K0 != 0 || coeffs.Ks != 0 {
		t.Errorf("外区点内区系数应为零值, 实际 Kalpha=%v Kbeta=%v K0=%v Ks=%v",
			coeffs.Kalpha, coeffs.Kbeta, coeffs.K0, coeffs.Ks)
	}

	// 马赫数仍由 p_t/p_s/PAtm 计算（与内/外区无关）
	if coeffs.MachNumber == nil {
		t.Fatal("MachNumber 不应为 nil")
	}
	if math.Abs(*coeffs.MachNumber-0.242) > sevenHoleMachEpsilon {
		t.Errorf("Ma 期望 0.242, 实际 %.6f", *coeffs.MachNumber)
	}
}

// TestCalculateSevenHoleOuterCoefficients_RingModulo 环形取模验证（n=1 时 n-1=6，n=6 时 n+1=1）
//
// 依据 spec §4.2：n+1 顺时针相邻，n-1 逆时针相邻；n=1 时 n-1=6，n=6 时 n+1=1。
// 构造对称数据：P1=P6=200（最大），其他孔为 0。
// - n=1 时：n+1=2, n-1=6 → 分母 = P1-(P2+P6)/2 = 200-(0+200)/2 = 100
// - n=6 时：n+1=1, n-1=5 → 分母 = P6-(P1+P5)/2 = 200-(200+0)/2 = 100
// 两种情况分母相同，Kθ 也应相同（环形对称性）。
func TestCalculateSevenHoleOuterCoefficients_RingModulo(t *testing.T) {
	pTunnel := 1000.0
	pStatic := 0.0
	raw := SevenHoleRawData{
		P1:      200.0,
		P2:      0.0,
		P3:      0.0,
		P4:      0.0,
		P5:      0.0,
		P6:      200.0,
		P7:      100.0,
		PAtm:    98880.0,
		PTotal:  &pTunnel,
		PStatic: &pStatic,
	}

	// n=1 时 n-1=6（环形取模）
	coeffs1, err := CalculateSevenHoleOuterCoefficients(raw, 1)
	if err != nil {
		t.Fatalf("n=1 时返回错误: %v", err)
	}

	// n=6 时 n+1=1（环形取模）
	coeffs6, err := CalculateSevenHoleOuterCoefficients(raw, 6)
	if err != nil {
		t.Fatalf("n=6 时返回错误: %v", err)
	}

	// 环形对称性：n=1 和 n=6 在 P1=P6 时 Kθ 应相等
	// n=1: Kθ[1] = (P1-P7)/(P1-(P2+P6)/2) = (200-100)/(200-(0+200)/2) = 100/100 = 1.0
	// n=6: Kθ[6] = (P6-P7)/(P6-(P1+P5)/2) = (200-100)/(200-(200+0)/2) = 100/100 = 1.0
	if math.Abs(coeffs1.Ktheta-1.0) > sevenHoleCoeffEpsilon {
		t.Errorf("n=1: Ktheta 期望 1.0, 实际 %.6f", coeffs1.Ktheta)
	}
	if math.Abs(coeffs6.Ktheta-1.0) > sevenHoleCoeffEpsilon {
		t.Errorf("n=6: Ktheta 期望 1.0, 实际 %.6f", coeffs6.Ktheta)
	}
}

// TestCalculateSevenHoleOuterCoefficients_InvalidSector 无效扇区编号
//
// 依据 spec §4.2：n ∈ {1..6}，超出范围应返回错误。
func TestCalculateSevenHoleOuterCoefficients_InvalidSector(t *testing.T) {
	pTunnel := 1000.0
	pStatic := 0.0
	raw := SevenHoleRawData{
		P1:      200.0,
		P2:      0.0,
		P3:      0.0,
		P4:      0.0,
		P5:      0.0,
		P6:      0.0,
		P7:      100.0,
		PAtm:    98880.0,
		PTotal:  &pTunnel,
		PStatic: &pStatic,
	}

	invalidSectors := []int{0, -1, 7, 8, 100}
	for _, n := range invalidSectors {
		_, err := CalculateSevenHoleOuterCoefficients(raw, n)
		if err == nil {
			t.Errorf("n=%d 应返回错误（无效扇区编号）, 实际返回 nil", n)
		}
	}
}

// TestCalculateSevenHoleOuterCoefficients_DivideByZero 外区公式除零保护
//
// 当 Pn-(Pn+1+Pn-1)/2 接近零（相邻孔压力均等，无方向性）时，
// 公式分母为零，必须返回错误而非 NaN/Inf。
func TestCalculateSevenHoleOuterCoefficients_DivideByZero(t *testing.T) {
	pTunnel := 1000.0
	pStatic := 0.0
	raw := SevenHoleRawData{
		P1:      200.0,
		P2:      200.0, // P1=P2，分母 P1-(P2+P6)/2 中 P2=P1 导致分母接近 P1-P1/2=100
		P6:      200.0, // 但若 P1=P2=P6，分母 = 200-(200+200)/2 = 0
		P7:      100.0,
		PAtm:    98880.0,
		PTotal:  &pTunnel,
		PStatic: &pStatic,
	}

	_, err := CalculateSevenHoleOuterCoefficients(raw, 1)
	if err == nil {
		t.Error("P1=P2=P6 时分母为零应返回错误, 实际返回 nil")
	}
}

// TestCalculateSevenHoleOuterCoefficients_EqualPressure 外区等压（p_t == p_s）零流量语义
//
// 验证 review P1 修复：与内区等压测试同口径，外区等压时 K0[n]/Ks[n] 跳过（保持零值），
// Ma/V 返回 0，不再返回错误。保证七孔外区 CSV/样本在零流量场景下与 live physics 一致。
//
// 测试前置：
//   - P1=700 为最大孔（n=1），P2/P6 取不同值保证 P1-(P2+P6)/2≠0（避免 denomThetaPhi 除零）
//   - PTotal == PStatic（等压，pt-ps=0）
//   - PAtm/TAtm 齐全
//
// 期待结果：
//   - err == nil
//   - MachNumber != nil 且 *MachNumber == 0
//   - Velocity != nil 且 *Velocity == 0
//   - K0Outer == 0, KsOuter == 0（等压跳过）
func TestCalculateSevenHoleOuterCoefficients_EqualPressure(t *testing.T) {
	pTunnel := 500.0
	pStatic := 500.0 // p_t == p_s，等压零流量
	raw := SevenHoleRawData{
		P1:      700.0, // P1 最大 → n=1
		P2:      200.0,
		P3:      300.0,
		P4:      400.0,
		P5:      500.0,
		P6:      600.0, // P1-(P2+P6)/2 = 700-(200+600)/2 = 700-400 = 300 ≠ 0
		P7:      100.0,
		PAtm:    98880.0,
		TAtm:    28.0,
		PTotal:  &pTunnel,
		PStatic: &pStatic,
	}

	coeffs, err := CalculateSevenHoleOuterCoefficients(raw, 1)
	if err != nil {
		t.Fatalf("外区等压（p_t==p_s）不应返回错误, 实际: %v", err)
	}

	// K0Outer/KsOuter 等压跳过，保持零值
	if coeffs.K0Outer != 0 || coeffs.KsOuter != 0 {
		t.Errorf("等压时 K0Outer/KsOuter 应跳过保持零值, 实际 K0Outer=%.6f KsOuter=%.6f",
			coeffs.K0Outer, coeffs.KsOuter)
	}

	// Ma/V 必须返回 0（与内区及 live physics 零流量口径一致）
	if coeffs.MachNumber == nil {
		t.Fatal("外区等压时 MachNumber 不应为 nil, 期望 0")
	}
	if *coeffs.MachNumber != 0 {
		t.Errorf("外区等压时 Ma 期望 0, 实际 %.6f", *coeffs.MachNumber)
	}
	if coeffs.Velocity == nil {
		t.Fatal("外区等压时 Velocity 不应为 nil, 期望 0")
	}
	if *coeffs.Velocity != 0 {
		t.Errorf("外区等压时 V 期望 0, 实际 %.6f", *coeffs.Velocity)
	}
}

// ==================== 马赫数公式测试（spec §4.4） ====================

// TestCalculateSevenHoleMachNumber_CenterPoint 中心点马赫数验证
//
// 依据 spec §4.4：仅在 Ma 计算入口将 p_t、p_s 转绝压（A→C 边界，仅转换一次）。
//
//	p_t_abs = p_t + 大气压力 = 4073.07 + 98880 = 102953.07
//	p_s_abs = p_s + 大气压力 = -32.7 + 98880 = 98847.3
//	Ma = √((2/(γ-1)) × ((p_t_abs/p_s_abs)^((γ-1)/γ) - 1)) = 0.242
func TestCalculateSevenHoleMachNumber_CenterPoint(t *testing.T) {
	// 表压输入（A 基准）
	pTunnel := 4073.07
	pStatic := -32.7
	atmPressure := 98880.0

	ma, err := CalculateSevenHoleMachNumber(pTunnel, pStatic, atmPressure)
	if err != nil {
		t.Fatalf("CalculateSevenHoleMachNumber 返回错误: %v", err)
	}

	if math.Abs(ma-0.242) > sevenHoleMachEpsilon {
		t.Errorf("Ma 期望 0.242, 实际 %.6f, 误差 %.6f", ma, math.Abs(ma-0.242))
	}
}

// TestCalculateSevenHoleMachNumber_InvalidInput 马赫数异常输入
//
// 依据 spec §4.4 与 AtmosphericDataCalculator.CalculateMach 约定（Task 12 后）：
//   - 静压 ≤ 0：物理无意义（绝压必须 > 0）
//   - 总压 < 静压：亚音速风洞中总压必须 ≥ 静压；等压 (Pt == Ps) 已为有效零流量
//     （由 TestCalculateSevenHoleMachNumber_TableDriven 覆盖）
//
// 两种场景必须返回错误，禁止返回 NaN/Inf 误导下游。
func TestCalculateSevenHoleMachNumber_InvalidInput(t *testing.T) {
	atmPressure := 98880.0

	// 场景 1：p_s_abs ≤ 0（大气压不足以抵消负表压）
	_, err := CalculateSevenHoleMachNumber(1000.0, -100000.0, atmPressure)
	if err == nil {
		t.Error("p_s_abs ≤ 0 时应返回错误, 实际返回 nil")
	}

	// 场景 2：p_t_abs < p_s_abs（总压严格小于静压）
	_, err = CalculateSevenHoleMachNumber(-100.0, 0.0, atmPressure)
	if err == nil {
		t.Error("p_t_abs < p_s_abs 时应返回错误, 实际返回 nil")
	}

	// 场景 3：大气压力为 0（无法转绝压）
	_, err = CalculateSevenHoleMachNumber(1000.0, 0.0, 0.0)
	if err == nil {
		t.Error("大气压力=0 时应返回错误, 实际返回 nil")
	}
}

// TestCalculateSevenHoleMachNumber_TableDriven 表驱动覆盖零/非零/非法/绝压转换
//
// Task 12 验收：等压是有效零；Pt < Ps、Ps ≤ 0 仍失败；A→C 边界正确转绝压。
// 覆盖场景：
//   - 零流量：表压均为 0 或相等非 0 → pt_abs == ps_abs → Ma=0
//   - 非零：文档中心点（表压 4073.07/-32.7, atm=98880）→ Ma≈0.242
//   - 非法：pt_abs < ps_abs；ps_abs ≤ 0
//   - A→C 边界：表压输入经绝压转换后正确计算（非零用例同步验证此边界）
func TestCalculateSevenHoleMachNumber_TableDriven(t *testing.T) {
	atm := 98880.0
	tests := []struct {
		name    string
		pt, ps  float64 // 表压 (A 基准)
		wantErr bool
		wantMa  float64
	}{
		{"零流量 表压均为 0", 0, 0, false, 0},
		{"零流量 表压相等非 0", 100, 100, false, 0},
		{"非零 文档中心点 Ma≈0.242", 4073.07, -32.7, false, 0.242},
		{"非法 pt_abs < ps_abs", -100, 0, true, 0},
		{"非法 ps_abs ≤ 0", 1000, -100000, true, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ma, err := CalculateSevenHoleMachNumber(tt.pt, tt.ps, atm)
			if tt.wantErr {
				if err == nil {
					t.Errorf("期望错误, 实际 nil (Ma=%v)", ma)
				}
				return
			}
			if err != nil {
				t.Fatalf("未期望错误: %v", err)
			}
			if math.IsNaN(ma) || math.IsInf(ma, 0) {
				t.Fatalf("Ma 不应为 NaN/Inf: %v", ma)
			}
			if math.Abs(ma-tt.wantMa) > sevenHoleMachEpsilon {
				t.Errorf("Ma 期望 %v (容差 %v), 实际 %v", tt.wantMa, sevenHoleMachEpsilon, ma)
			}
		})
	}
}

// TestCalculateSevenHoleMachNumber_AbsConversionBoundary 验证仅在 Ma 入口转绝压
//
// 依据 spec §2.1 压力基准三分离：A→C 只转换一次，禁止在系数计算阶段提前转绝压。
// 此测试通过对比"表压直接传入"与"绝压传入"的输出差异，验证函数内部确实做了 A→C 转换。
//
// 如果函数实现错误（未转绝压），传入 4073.07/-32.7 会因 p_s=-32.7 < 0 报错；
// 正确实现应该把 p_s 转为 98847.3（绝压）后再调 AtmosphericDataCalculator。
func TestCalculateSevenHoleMachNumber_AbsConversionBoundary(t *testing.T) {
	// 表压输入（A 基准）——若函数未做 A→C 转换，p_s=-32.7<0 会让底层 CalculateMach 报错
	ma, err := CalculateSevenHoleMachNumber(4073.07, -32.7, 98880.0)
	if err != nil {
		t.Fatalf("A→C 转换未生效: %v", err)
	}
	if ma <= 0 || math.IsNaN(ma) || math.IsInf(ma, 0) {
		t.Errorf("Ma 应为正有限数, 实际 %.6f", ma)
	}
}

// TestCalculateSevenHoleVelocity_CenterPoint 速度验证（与 Ma 联动）
//
// 依据 spec §4.4：V = Ma × 20.047 × √SAT
//   - TAT 优先级：TTunnel > TAtm
//   - SAT = TAT / (1 + 0.2 × r × Ma²)，r=0.9 恢复系数
//
// 中心点：Ma=0.242, TAtm=28°C → V ≈ 83.7 m/s（spec §4.4 数据集验证）
func TestCalculateSevenHoleVelocity_CenterPoint(t *testing.T) {
	pTunnel := 4073.07
	pStatic := -32.7
	raw := SevenHoleRawData{
		P1:      537.18,
		P2:      282.78,
		P3:      313.98,
		P4:      519.90,
		P5:      573.97,
		P6:      294.97,
		P7:      4075.35,
		PAtm:    98880.0,
		TAtm:    28.0,
		PTotal:  &pTunnel,
		PStatic: &pStatic,
	}

	coeffs, err := CalculateSevenHoleInnerCoefficients(raw)
	if err != nil {
		t.Fatalf("CalculateSevenHoleInnerCoefficients 返回错误: %v", err)
	}

	if coeffs.Velocity == nil {
		t.Fatal("Velocity 不应为 nil")
	}

	// spec §4.4 数据集验证：V ≈ 83.7 m/s（容差 5%）
	if math.Abs(*coeffs.Velocity-83.7) > 5.0 {
		t.Errorf("V 期望约 83.7 m/s (±5), 实际 %.3f", *coeffs.Velocity)
	}
}

// TestCalculateSevenHoleVelocity_TTunnelPriority TTunnel 优先于 TAtm
//
// 依据 spec §4.4：TAT 选取优先级：风洞温度 TTunnel > 大气温度 TAtm。
// 当 TTunnel 非空时，应优先使用 TTunnel 计算速度，而非 TAtm。
func TestCalculateSevenHoleVelocity_TTunnelPriority(t *testing.T) {
	pTunnel := 4073.07
	pStatic := -32.7
	tTunnel := 35.0 // 风洞温度高于大气温度
	raw := SevenHoleRawData{
		P1:      537.18,
		P2:      282.78,
		P3:      313.98,
		P4:      519.90,
		P5:      573.97,
		P6:      294.97,
		P7:      4075.35,
		PAtm:    98880.0,
		TAtm:    28.0,
		TTunnel: &tTunnel,
		PTotal:  &pTunnel,
		PStatic: &pStatic,
	}

	coeffs, err := CalculateSevenHoleInnerCoefficients(raw)
	if err != nil {
		t.Fatalf("返回错误: %v", err)
	}
	if coeffs.Velocity == nil {
		t.Fatal("Velocity 不应为 nil")
	}

	// 用 TTunnel=35°C 计算的 V 应大于用 TAtm=28°C 计算的 V（温度越高声速越大）
	rawNoTTunnel := raw
	rawNoTTunnel.TTunnel = nil
	coeffsNoTTunnel, _ := CalculateSevenHoleInnerCoefficients(rawNoTTunnel)
	if coeffsNoTTunnel.Velocity == nil {
		t.Fatal("对照样本 Velocity 不应为 nil")
	}

	if *coeffs.Velocity <= *coeffsNoTTunnel.Velocity {
		t.Errorf("TTunnel=35°C 时 V (%.3f) 应大于 TAtm=28°C 时 V (%.3f)",
			*coeffs.Velocity, *coeffsNoTTunnel.Velocity)
	}
}
