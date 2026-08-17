package calibration

import (
	"fmt"
	"math"
)

// ==================== 七孔探针不确定度评定（spec §5） ====================
//
// 实现内容（spec §5.1~5.5）：
//   - B 类分量 u1~u4（spec §5.1 表格）
//   - A 类分量 u(A) = S/√N（spec §5.1）
//   - 内区 K0/Ks/Kα/Kβ 灵敏系数（spec §5.3）
//   - 合成公式 |Σ cᵢu_B,i|² + Σ cᵢ²u(A,i)²（spec §5.2，B 类 ρ=1，A 类 ρ=0）
//   - 扩展不确定度 U = k·u_c（spec §5.4，默认 k=2）
//
// 关键约束（spec §5.2 注释）：
//   - ✅ 正确公式：u_c = √( |Σ cᵢ·u_B,i|² + Σ cᵢ²·u(A,i)² )
//     B 类完全正相关（ρ=1）→ 保留符号求和后取绝对值
//     A 类相互独立（ρ=0）→ 平方和开方
//   - ❌ 错误公式：u_c = √( (Σ|cᵢ|·u_B,i)² + Σ cᵢ²·u(A,i)² )
//     绝对值的和隐含 ρᵢⱼ = sign(cᵢcⱼ)，是保守估计但不是 ρ=1 标准结果，会高估约 46%
//
// 误差源物理归属（spec §5.5 第 1 步，B 类分量按输入归属分组）：
//   - P1~P7：扫描阀误差 u1 + 运动误差 u3（外孔与中心孔同源）
//   - p_t：总压脉动 u2 + 运动误差 u3
//   - p_s：静压扫描阀 u4 + 运动误差 u3
//   - u3 对所有压力输入都影响（运动机构位移引起压力变化）

// ==================== 常数定义（spec §5.1） ====================

const (
	// u1PressureScannerRange 压力扫描阀量程（±35 kPa），用于 u1 计算
	u1PressureScannerRange = 70000.0 // Pa
	// u1ScannerAccuracy 扫描阀精度（0.05%），用于 u1/u4 计算
	u1ScannerAccuracy = 0.0005
	// u2TotalPressureFluctuationRate 总压脉动率（0.1%），用于 u2 计算
	u2TotalPressureFluctuationRate = 0.001
	// u3MotionErrorRange 位移机构运动误差引起的压力变化量程（5 Pa），用于 u3 计算
	u3MotionErrorRange = 5.0
	// u4StaticPressureScannerRange 静压扫描阀量程（±10 kPa），用于 u4 计算
	u4StaticPressureScannerRange = 20000.0 // Pa
	// sqrt3 √3，B 类均匀分布的除数
	// 注：Go 常量不支持 math.Sqrt 调用，使用十进制近似值 1.7320508075688772
	sqrt3 = 1.7320508075688772
	// defaultExpansionFactor 默认扩展因子 k=2（95% 置信区间，spec §5.4）
	defaultExpansionFactor = 2.0
)

// ==================== UncertaintyCalculator 结构体 ====================

// UncertaintyCalculator 七孔探针不确定度计算器
//
// 封装 B 类分量配置和 p_t 上下文（u2 随 p_t 变化）。
// 实例化时通过 NewUncertaintyCalculator(pTunnel) 注入 p_t，
// 后续调用 U1()/U2()/U3()/U4() 返回对应的 B 类分量数值。
//
// 设计权衡：
//   - p_t 作为构造参数而非每次调用传入——避免 SensitivityCoefficientsK0 等多个方法重复传 p_t
//   - 假设单次校准任务中 p_t 稳定（风洞建立后总压不变），u2 在任务期间无需重算
//   - 若未来需要支持 p_t 动态变化，可增加 SetPTunnel 方法重置 u2 缓存
type UncertaintyCalculator struct {
	pTunnel float64 // 风洞参考总压（表压 A 基准），用于 u2 = p_t × 0.1% / √3
}

// NewUncertaintyCalculator 创建不确定度计算器实例
//
// 参数 pTunnel 为风洞参考总压（表压），用于 u2 计算。
// 调用时机：每个校准点位采集前创建一个新实例（p_t 可能不同），或任务级单例（p_t 稳定时）。
func NewUncertaintyCalculator(pTunnel float64) *UncertaintyCalculator {
	return &UncertaintyCalculator{pTunnel: pTunnel}
}

// U1 返回扫描阀误差引起的 B 类不确定度（spec §5.1）
//
// 公式：u1 = 70000 × 0.05% / √3 = 20.207 Pa
// 物理含义：压力扫描阀量程 ±35 kPa，精度 0.05%，按均匀分布除以 √3
func (c *UncertaintyCalculator) U1() float64 {
	return u1PressureScannerRange * u1ScannerAccuracy / sqrt3
}

// U2 返回风洞总压脉动引起的 B 类不确定度（spec §5.1）
//
// 公式：u2 = p_t × 0.1% / √3
// 物理含义：风洞气流总压脉动率 0.1%，按均匀分布除以 √3
// 注意：u2 随 p_t 变化，p_t 为表压（A 基准）
func (c *UncertaintyCalculator) U2() float64 {
	return c.pTunnel * u2TotalPressureFluctuationRate / sqrt3
}

// U3 返回位移机构运动误差引起的 B 类不确定度（spec §5.1）
//
// 公式：u3 = 5 / √3 = 2.886 Pa
// 物理含义：位移机构运动误差引起压力变化 5 Pa，按均匀分布除以 √3
func (c *UncertaintyCalculator) U3() float64 {
	return u3MotionErrorRange / sqrt3
}

// U4 返回静压扫描阀误差引起的 B 类不确定度（spec §5.1）
//
// 公式：u4 = 20000 × 0.05% / √3 = 5.773 Pa
// 物理含义：静压扫描阀量程 ±10 kPa，精度 0.05%，按均匀分布除以 √3
func (c *UncertaintyCalculator) U4() float64 {
	return u4StaticPressureScannerRange * u1ScannerAccuracy / sqrt3
}

// BTypeForInput 按误差源物理归属返回各压力输入的 B 类不确定度（spec §5.5 第 1 步）
//
// 输入参数 inputName 标识压力来源：
//   - "P1".."P7"：扫描阀误差 u1 + 运动误差 u3 → √(u1² + u3²)
//   - "p_t"：总压脉动 u2 + 运动误差 u3 → √(u2² + u3²)
//   - "p_s"：静压扫描阀 u4 + 运动误差 u3 → √(u4² + u3²)
//
// 物理依据（spec §5.5 第 1 步注释）：
//   - u1~u4 对各输入的影响按误差源物理归属分组
//   - u3（运动误差）对所有压力输入都影响（运动机构位移改变所有孔的压力）
//   - 同源误差按均匀分布独立假设，取平方和开方（不同误差源相互独立）
//
// 未知输入名返回 0（防御性编程，避免 typo 导致 NaN）
func (c *UncertaintyCalculator) BTypeForInput(inputName string) float64 {
	u3 := c.U3()
	switch inputName {
	case "P1", "P2", "P3", "P4", "P5", "P6", "P7":
		// 扫描阀误差 + 运动误差
		u1 := c.U1()
		return math.Sqrt(u1*u1 + u3*u3)
	case "p_t":
		// 总压脉动 + 运动误差
		u2 := c.U2()
		return math.Sqrt(u2*u2 + u3*u3)
	case "p_s":
		// 静压扫描阀 + 运动误差
		u4 := c.U4()
		return math.Sqrt(u4*u4 + u3*u3)
	default:
		return 0
	}
}

// ==================== A 类不确定度（spec §5.1） ====================

// CalculateTypeA 计算 A 类不确定度 u(A) = S/√N
//
// 输入参数 samples 为同一压力输入的 N 次采样值
// 输出：
//   - uA: 均值的标准差 S/√N（A 类不确定度）
//   - stdDev: 样本标准差 S（n-1 自由度）
//
// 边界处理：
//   - N < 2 时返回 (0, 0)（样本数不足无法计算样本标准差）
//   - 复用 StdDev 函数保持与五孔/三孔/总压同口径
//
// spec §5.1 约束：N ≥ 3（默认 5），此处允许 N=2 仅作降级处理，调用方应保证 N≥3
func (c *UncertaintyCalculator) CalculateTypeA(samples []float64) (uA float64, stdDev float64) {
	n := len(samples)
	if n < 2 {
		return 0, 0
	}
	stdDev = StdDev(samples)
	uA = stdDev / math.Sqrt(float64(n))
	return
}

// ==================== 内区 K0 灵敏系数（spec §5.3） ====================

// SensitivityCoefficientsK0 计算内区 K0 = (P7 - p_t) / (p_t - p_s) 的灵敏系数
//
// 实现公式（spec §5.3）：
//
//	C(P7)  =  ∂K0/∂P7  =  1 / (p_t - p_s)
//	C(p_t) =  ∂K0/∂p_t =  (p_s - P7) / (p_t - p_s)²
//	C(p_s) =  ∂K0/∂p_s =  (P7 - p_t) / (p_t - p_s)²
//
// 推导（链式求导）：
//
//	K0 = (P7 - p_t) / D,  D = p_t - p_s
//	∂K0/∂P7 = 1/D
//	∂K0/∂p_t = [(-1)·D - (P7-p_t)·1] / D² = [-D - (P7-p_t)] / D² = [-(p_t-p_s) - (P7-p_t)] / D²
//	         = (p_s - P7) / D²
//	∂K0/∂p_s = [0·D - (P7-p_t)·(-1)] / D² = (P7-p_t) / D²
//
// 物理含义（spec §5.5 第 3 步）：
//   - c(P7) 与 c(p_t) 符号通常相反（p_s - P7 < 0 而 p_t - p_s > 0）
//   - 此特性导致 B 类合成时正负项部分抵消（spec §5.2 关键区别）
//
// 注：此函数不做除零保护——分母 (p_t - p_s) 接近零时风洞未建立压差，
// K0 本身无意义，调用方应在外层检查 p_t > p_s 后再调用。
// 此设计与 CalculateSevenHoleInnerCoefficients 保持一致（错误由公式层返回）。
func (c *UncertaintyCalculator) SensitivityCoefficientsK0(p7, pTunnel, pStatic float64) (cP7, cPt, cPs float64) {
	denom := pTunnel - pStatic
	denomSq := denom * denom

	cP7 = 1.0 / denom
	cPt = (pStatic - p7) / denomSq
	cPs = (p7 - pTunnel) / denomSq
	return
}

// ==================== 内区 Ks 灵敏系数（spec §5.3） ====================

// SensitivityCoefficientsKs 计算内区 Ks = (p_s - P̄) / (p_t - p_s) 的灵敏系数
//
// 实现公式（spec §5.3，P̄ = (P1+P2+P3+P4+P5+P6)/6）：
//
//	C(P1..P6) = -1 / (6·(p_t - p_s))                       外围 6 孔相同（P̄ 线性依赖）
//	C(p_t)    = (P1+P2+P3+P4+P5+P6 - 6·p_s) / (6·(p_t - p_s)²)
//	C(p_s)    = (6·p_t - P1-P2-P3-P4-P5-P6) / (6·(p_t - p_s)²)
//
// 推导（链式求导，注意 P̄ 依赖全部外围孔）：
//
//	Ks = (p_s - P̄) / D,  D = p_t - p_s,  P̄ = (ΣPi)/6
//	∂Ks/∂Pi = (-1/6) / D = -1 / (6D)        (i=1..6)
//	∂Ks/∂p_t = [0·D - (p_s-P̄)·1] / D² = -(p_s-P̄)/D² = (P̄-p_s)/D²
//	          但 P̄ 不依赖 p_t，可继续展开：(P̄-p_s)/D²，P̄ = (ΣPi)/6
//	          = ((ΣPi)/6 - p_s) / D² = (ΣPi - 6·p_s) / (6·D²)
//	∂Ks/∂p_s = [1·D - (p_s-P̄)·(-1)] / D² = [D + (p_s-P̄)] / D²
//	          = [(p_t-p_s) + (p_s-P̄)] / D² = (p_t - P̄) / D²
//	          = (6·p_t - ΣPi) / (6·D²)
//
// 物理含义（spec §5.2 注释）：
//   - c(P1..P6) = -1/(6·(p_t-p_s)) 为负（p_t > p_s 时）
//   - c(p_t) 通常为正（ΣPi > 6·p_s 时，外围孔压力远大于静压）
//   - 此异号特性是 spec §5.2 强调"|Σ cᵢuᵢ| 而非 Σ|cᵢ|uᵢ"的典型场景
//
// 返回值：cP1toP6（外围 6 孔相同，调用方按相同值赋给 P1..P6）
func (c *UncertaintyCalculator) SensitivityCoefficientsKs(p1, p2, p3, p4, p5, p6, pTunnel, pStatic float64) (cP1toP6, cPt, cPs float64) {
	sumP := p1 + p2 + p3 + p4 + p5 + p6
	denom := pTunnel - pStatic
	denomSq := denom * denom
	sixDenomSq := 6.0 * denomSq

	cP1toP6 = -1.0 / (6.0 * denom)
	cPt = (sumP - 6.0*pStatic) / sixDenomSq
	cPs = (6.0*pTunnel - sumP) / sixDenomSq
	return
}

// ==================== 内区 Kα 灵敏系数（spec §5.3，链式求导） ====================

// SevenHoleAlphaBetaSensitivity Kα/Kβ 对各输入的灵敏系数集合
//
// Kα = (Cpb + Cpc) / √3
// Kβ = -(2·Cpa + Cpb - Cpc) / 3
// 其中 Cpa = (P4 - P1) / D, Cpb = (P5 - P2) / D, Cpc = (P6 - P3) / D, D = P7 - P̄
//
// 链式求导涉及：
//   - P1, P4（Cpa 依赖）
//   - P2, P5（Cpb 依赖）
//   - P3, P6（Cpc 依赖）
//   - P7（D 依赖）
//   - P1..P6（P̄ 依赖，间接影响 D）
//
// 所有外围孔 P1..P6 通过 P̄ 进入 D 影响所有 C 系数，因此全部 7 个孔的灵敏系数都非零。
type SevenHoleAlphaBetaSensitivity struct {
	CP1, CP2, CP3, CP4, CP5, CP6, CP7 float64
}

// SensitivityCoefficientsKAlpha 计算内区 Kα 对各输入的灵敏系数
//
// Kα = (Cpb + Cpc) / √3
//    = ((P5-P2) + (P6-P3)) / (√3 · D)
//
// 其中 D = P7 - P̄, P̄ = (P1+P2+P3+P4+P5+P6)/6
//
// 链式求导（∂Kα/∂Pi）：
//
//	∂Kα/∂P2 = -1 / (√3 · D)                                  （P2 在分子中带负号）
//	∂Kα/∂P3 = -1 / (√3 · D)                                  （P3 在分子中带负号）
//	∂Kα/∂P5 = 1 / (√3 · D)                                   （P5 在分子中带正号）
//	∂Kα/∂P6 = 1 / (√3 · D)                                   （P6 在分子中带正号）
//	∂Kα/∂P1 = -[(P5-P2)+(P6-P3)] / (√3 · D²) · ∂D/∂P1
//	        = -[(P5-P2)+(P6-P3)] / (√3 · D²) · (-1/6)
//	        = [(P5-P2)+(P6-P3)] / (6·√3 · D²)
//	∂Kα/∂P4 = [(P5-P2)+(P6-P3)] / (6·√3 · D²)                （P4 与 P1 同样只通过 D 影响）
//	∂Kα/∂P7 = -[(P5-P2)+(P6-P3)] / (√3 · D²) · 1             （P7 直接进 D）
//	        = -[(P5-P2)+(P6-P3)] / (√3 · D²)
//
// 推导关键：分子中 (P5-P2)+(P6-P3) 不依赖 P1/P4/P7，故 ∂分子/∂P1=0，∂分子/∂P2=-1 等
// P1/P4 仅通过 P̄ 进入 D 影响 Kα，链式求导系数为 ∂D/∂P1 = -1/6
func (c *UncertaintyCalculator) SensitivityCoefficientsKAlpha(raw SevenHoleRawData) (SevenHoleAlphaBetaSensitivity, error) {
	pBar := (raw.P1 + raw.P2 + raw.P3 + raw.P4 + raw.P5 + raw.P6) / 6.0
	denom := raw.P7 - pBar

	// 除零保护（与 CalculateSevenHoleInnerCoefficients 一致）
	if math.Abs(denom) < sevenHoleDivTolerance {
		return SevenHoleAlphaBetaSensitivity{}, fmt.Errorf("Kα 灵敏系数分母 P7-P̄ 接近零 (%.6f)", denom)
	}

	denomSq := denom * denom
	sqrt3Val := math.Sqrt(3.0)
	sixSqrt3DenomSq := 6.0 * sqrt3Val * denomSq

	// 分子部分 numerator = (P5-P2) + (P6-P3)，不依赖 P1/P4/P7
	numerator := (raw.P5 - raw.P2) + (raw.P6 - raw.P3)

	// 对分子中显式出现的压力输入（P2/P3/P5/P6）
	invSqrt3D := 1.0 / (sqrt3Val * denom)
	cP2 := -invSqrt3D
	cP3 := -invSqrt3D
	cP5 := invSqrt3D
	cP6 := invSqrt3D

	// 仅通过 P̄ 进入 D 影响的输入（P1/P4），∂D/∂Pi = -1/6
	// ∂Kα/∂Pi = -numerator / (√3 · D²) · ∂D/∂Pi = -numerator / (√3 · D²) · (-1/6) = numerator / (6·√3·D²)
	cP1 := numerator / sixSqrt3DenomSq
	cP4 := numerator / sixSqrt3DenomSq

	// P7 直接进 D（∂D/∂P7 = 1）
	// ∂Kα/∂P7 = -numerator / (√3 · D²) · 1
	cP7 := -numerator / (sqrt3Val * denomSq)

	return SevenHoleAlphaBetaSensitivity{
		CP1: cP1, CP2: cP2, CP3: cP3, CP4: cP4, CP5: cP5, CP6: cP6, CP7: cP7,
	}, nil
}

// SensitivityCoefficientsKBeta 计算内区 Kβ 对各输入的灵敏系数
//
// Kβ = -(2·Cpa + Cpb - Cpc) / 3
//    = -(2·(P4-P1) + (P5-P2) - (P6-P3)) / (3·D)
//
// 链式求导：
//
//	∂Kβ/∂P1 = -(-2) / (3·D) = 2 / (3·D)                    （P1 在 Cpa 中带负号，Cpa 系数 -2）
//	∂Kβ/∂P2 = -(-1) / (3·D) = 1 / (3·D)                     （P2 在 Cpb 中带负号，Cpb 系数 1）
//	∂Kβ/∂P3 = -(-(-1)) / (3·D) = -1 / (3·D)                 （P3 在 Cpc 中带负号，Cpc 系数 -1 → 整体符号反）
//	∂Kβ/∂P4 = -(2) / (3·D) = -2 / (3·D)                     （P4 在 Cpa 中带正号，Cpa 系数 2）
//	∂Kβ/∂P5 = -(1) / (3·D) = -1 / (3·D)                     （P5 在 Cpb 中带正号，Cpb 系数 1）
//	∂Kβ/∂P6 = -(-1) / (3·D) = 1 / (3·D)                     （P6 在 Cpc 中带正号，Cpc 系数 -1 → 整体符号反）
//	∂Kβ/∂P7 = -numerator / (3·D²) · 1                       （P7 直接进 D）
//	         其中 numerator = 2·(P4-P1) + (P5-P2) - (P6-P3)
//
// 推导细节：
//   - Kβ = -N / (3·D)，N = 2·(P4-P1) + (P5-P2) - (P6-P3)
//   - ∂N/∂P1 = -2, ∂N/∂P2 = -1, ∂N/∂P3 = 1, ∂N/∂P4 = 2, ∂N/∂P5 = 1, ∂N/∂P6 = -1
//   - ∂Kβ/∂Pi = -∂N/∂Pi / (3·D) （对显式出现的 P1..P6）
//   - P1..P6 同时通过 P̄ 进入 D，但 N 不依赖 P7，∂Kβ/∂Pi 的 D 部分需用乘积法则：
//     ∂Kβ/∂Pi = -∂N/∂Pi / (3·D) - N · ∂(1/(3·D))/∂Pi
//              = -∂N/∂Pi / (3·D) + N · ∂D/∂Pi / (3·D²)
//     其中 ∂D/∂Pi = -1/6（i=1..6）, ∂D/∂P7 = 1
//   - 显式项 -∂N/∂Pi/(3·D) 是主项，D 相关项 N·∂D/∂Pi/(3·D²) 是修正项
func (c *UncertaintyCalculator) SensitivityCoefficientsKBeta(raw SevenHoleRawData) (SevenHoleAlphaBetaSensitivity, error) {
	pBar := (raw.P1 + raw.P2 + raw.P3 + raw.P4 + raw.P5 + raw.P6) / 6.0
	denom := raw.P7 - pBar

	if math.Abs(denom) < sevenHoleDivTolerance {
		return SevenHoleAlphaBetaSensitivity{}, fmt.Errorf("Kβ 灵敏系数分母 P7-P̄ 接近零 (%.6f)", denom)
	}

	denomSq := denom * denom
	threeD := 3.0 * denom
	threeDSq := 3.0 * denomSq

	// numerator N = 2·(P4-P1) + (P5-P2) - (P6-P3)
	numerator := 2.0*(raw.P4-raw.P1) + (raw.P5-raw.P2) - (raw.P6-raw.P3)

	// 显式项主部：∂N/∂Pi
	// ∂N/∂P1 = -2, ∂N/∂P2 = -1, ∂N/∂P3 = 1, ∂N/∂P4 = 2, ∂N/∂P5 = 1, ∂N/∂P6 = -1
	dN_dP := [6]float64{-2.0, -1.0, 1.0, 2.0, 1.0, -1.0}

	// 修正项：N·∂D/∂Pi / (3·D²)，∂D/∂Pi = -1/6 (i=1..6)
	correctionPerOuter := -numerator / (6.0 * threeDSq)

	cP1 := -dN_dP[0]/threeD + correctionPerOuter
	cP2 := -dN_dP[1]/threeD + correctionPerOuter
	cP3 := -dN_dP[2]/threeD + correctionPerOuter
	cP4 := -dN_dP[3]/threeD + correctionPerOuter
	cP5 := -dN_dP[4]/threeD + correctionPerOuter
	cP6 := -dN_dP[5]/threeD + correctionPerOuter

	// P7：仅通过 D 影响，∂D/∂P7 = 1
	// ∂Kβ/∂P7 = -N · 1 / (3·D²)
	cP7 := -numerator / threeDSq

	return SevenHoleAlphaBetaSensitivity{
		CP1: cP1, CP2: cP2, CP3: cP3, CP4: cP4, CP5: cP5, CP6: cP6, CP7: cP7,
	}, nil
}

// ==================== 合成不确定度（spec §5.2） ====================

// CombinedUncertainty 计算合成标准不确定度
//
// 实现公式（spec §5.2）：
//
//	u_c(y) = √( |Σᵢ cᵢ·u_B,i|²  +  Σᵢ cᵢ²·u(A,i)² )
//	               ↑                  ↑
//	           B 类完全正相关       A 类各输入独立
//	           （取和的绝对值）     （平方和开方）
//
// 关键约束（spec §5.2 注释）：
//   - ✅ 正确公式：B 类用 |Σ cᵢ·u_B,i|（保留符号求和后取绝对值）
//     反映 ρ=1 时误差同向传播的真实物理意义，异号项部分抵消
//   - ❌ 错误公式：B 类用 Σ|cᵢ|·u_B,i（绝对值的和）
//     隐含 ρᵢⱼ = sign(cᵢcⱼ)，是保守估计但不是 ρ=1 标准结果，会高估约 46%
//
// 参数：
//   - cValues: 各输入的灵敏系数 cᵢ
//   - uBValues: 各输入的 B 类不确定度 u_B,i
//   - uAValues: 各输入的 A 类不确定度 u(A,i)
//
// 三个切片长度必须相同（同一组输入），否则返回 0。
func (c *UncertaintyCalculator) CombinedUncertainty(cValues, uBValues, uAValues []float64) float64 {
	n := len(cValues)
	if n != len(uBValues) || n != len(uAValues) || n == 0 {
		return 0
	}

	// B 类合成：保留符号求和后取绝对值（spec §5.2 正确公式）
	sumCB := 0.0
	for i, cVal := range cValues {
		sumCB += cVal * uBValues[i]
	}
	bSquared := sumCB * sumCB

	// A 类合成：平方和（spec §5.2，各输入独立 ρ=0）
	aSquared := 0.0
	for i, cVal := range cValues {
		ca := cVal * uAValues[i]
		aSquared += ca * ca
	}

	return math.Sqrt(bSquared + aSquared)
}

// ==================== 扩展不确定度（spec §5.4） ====================

// ExpandedUncertainty 计算扩展不确定度 U = k·u_c
//
// 公式（spec §5.4）：U = k × u_c
//
// 参数：
//   - uC: 合成标准不确定度
//   - k: 包含因子，默认 2.0（95% 置信区间）；传入 0 或负数时回退到默认值
//
// spec §5.4 约定 k=2 为默认值，对应 95% 置信区间。
// 物理含义：U 表示被测量的真值有约 95% 概率落在 [结果 - U, 结果 + U] 范围内。
func (c *UncertaintyCalculator) ExpandedUncertainty(uC, k float64) float64 {
	if k <= 0 {
		k = defaultExpansionFactor
	}
	return k * uC
}
