package calibration

import (
	"fmt"
	"math"
)

// AtmosphericDataCalculator 飞行大气数据速度计算算法
// 基于文档：《[20251128扫描阀与ADC-9比对]》总结及验证数据
type AtmosphericDataCalculator struct{}

// 大气数据计算常数
const (
	atmR      = 287.05 // 气体常数 J/(kg·K)
	atmP0     = 101325 // 标准海平面气压 Pa
	atmT0     = 288.15 // 标准海平面温度 K
	atmRHO0   = 1.225  // 标准海平面空气密度 kg/m³
	atmGAMMA  = 1.4    // 空气绝热指数
	atmCCOEFF  = 20.047 // 声速计算系数 (文档P10: C=20.047)
	atmRECOVERY = 0.9   // 温度传感器恢复系数 (默认值)
)

// NewAtmosphericDataCalculator 创建飞行大气数据计算器实例
func NewAtmosphericDataCalculator() *AtmosphericDataCalculator {
	return &AtmosphericDataCalculator{}
}

// CalculateMach 计算马赫数
// 依据：文档验证数据表中的马赫数计算逻辑 (亚音速公式)
// 公式: Ma = sqrt( (2/(γ-1)) * ((Pt/Ps)^((γ-1)/γ) - 1) )
func (c *AtmosphericDataCalculator) CalculateMach(Pt, Ps float64) (float64, error) {
	if Ps <= 0 {
		return 0, fmt.Errorf("静压Ps必须大于0")
	}
	if Pt <= Ps {
		return 0, fmt.Errorf("总压Pt必须大于静压Ps")
	}
	ratio := Pt / Ps
	ma := math.Sqrt(
		(2 / (atmGAMMA - 1)) * (math.Pow(ratio, (atmGAMMA-1)/atmGAMMA) - 1),
	)
	return ma, nil
}

// CalculateSAT 计算静温 (SAT)
// 依据：文档总结1 - "总温TAT取开氏温度，为传感器测量值；r为温度传感器恢复系数"
// 公式推导：TAT = SAT * (1 + 0.2 * r * Ma^2) => SAT = TAT / (1 + 0.2 * r * Ma^2)
func (c *AtmosphericDataCalculator) CalculateSAT(TAT, Ma float64, r ...float64) float64 {
	recoveryCoeff := atmRECOVERY
	if len(r) > 0 {
		recoveryCoeff = r[0]
	}
	denominator := 1 + ((atmGAMMA-1)/2)*recoveryCoeff*math.Pow(Ma, 2)
	return TAT / denominator
}

// CalculateQc 计算动压
// 依据：文档总结3注释 - "Qc~动压=总压减去静压"
func (c *AtmosphericDataCalculator) CalculateQc(Pt, Ps float64) float64 {
	return Pt - Ps
}

// CalculateCAS 计算校正空速 (CAS)
// 依据：文档总结2 - CAS计算 (标准大气数据计算公式)
func (c *AtmosphericDataCalculator) CalculateCAS(Qc float64) float64 {
	a0 := math.Sqrt(atmGAMMA * atmP0 / atmRHO0)
	innerTerm := math.Pow(1+Qc/atmP0, (atmGAMMA-1)/atmGAMMA) - 1
	cas := a0 * math.Sqrt((2/(atmGAMMA-1))*innerTerm)
	return cas
}

// CalculateTASByDensity 计算真空速 (TAS) - 方法一：基于气压和静温计算
// 依据：文档总结3 - "注：Ps、Qc单位取Pa；SAT取开氏温度；R=287.05J/（kg.K）为常数"
// 逻辑：利用理想气体状态方程求出空气密度 ρs = Ps / (R * SAT)，再通过 TAS = CAS * sqrt(ρ0 / ρs) 求解
func (c *AtmosphericDataCalculator) CalculateTASByDensity(Ps, Qc, SAT float64) float64 {
	CAS := c.CalculateCAS(Qc)
	rhoS := Ps / (atmR * SAT)
	TAS := CAS * math.Sqrt(atmRHO0/rhoS)
	return TAS
}

// CalculateTASByMach 计算真空速 (TAS) - 方法二：声速马赫数法
// 依据：文档验证数据表 - "C=20.047", "vt(m/s) = Ma * C"
// 逻辑：C = 20.047 * sqrt(SAT)，TAS = Ma * C
func (c *AtmosphericDataCalculator) CalculateTASByMach(Ma, SAT float64) float64 {
	C := atmCCOEFF * math.Sqrt(SAT)
	TAS := Ma * C
	return TAS
}

// AtmosphericDataResult 大气数据完整计算结果
type AtmosphericDataResult struct {
	MachNumber float64 // 马赫数
	SAT        float64 // 静温 (K)
	Qc         float64 // 动压 (Pa)
	CAS        float64 // 校正空速 (m/s)
	TASDensity float64 // 真空速-气压静温法 (m/s)
	TASMach    float64 // 真空速-声速马赫数法 (m/s)
}

// CalculateAll 执行完整的大气数据计算
// Pt: 总压(Pa), Ps: 静压(Pa), TAT: 总温(K), r: 温度传感器恢复系数(默认0.9)
func (c *AtmosphericDataCalculator) CalculateAll(Pt, Ps, TAT float64, r ...float64) (AtmosphericDataResult, error) {
	Ma, err := c.CalculateMach(Pt, Ps)
	if err != nil {
		return AtmosphericDataResult{}, fmt.Errorf("计算马赫数失败: %w", err)
	}

	SAT := c.CalculateSAT(TAT, Ma, r...)
	Qc := c.CalculateQc(Pt, Ps)
	CAS := c.CalculateCAS(Qc)
	TASDensity := c.CalculateTASByDensity(Ps, Qc, SAT)
	TASMach := c.CalculateTASByDensity(Ps, Qc, SAT)

	return AtmosphericDataResult{
		MachNumber: Ma,
		SAT:        SAT,
		Qc:         Qc,
		CAS:        CAS,
		TASDensity: TASDensity,
		TASMach:    TASMach,
	}, nil
}
