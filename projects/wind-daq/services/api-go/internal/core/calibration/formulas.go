package calibration

import (
	"fmt"
	"math"
)

// ==================== 通用数学工具 ====================

// StdDev 计算标准差（样本标准差，n-1 自由度）
func StdDev(values []float64) float64 {
	n := len(values)
	if n < 2 {
		return 0
	}

	mean := Mean(values)
	sum := 0.0
	for _, v := range values {
		diff := v - mean
		sum += diff * diff
	}
	variance := sum / float64(n-1)
	return math.Sqrt(variance)
}

// Mean 计算平均值
func Mean(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	sum := 0.0
	for _, v := range values {
		sum += v
	}
	return sum / float64(len(values))
}

// ==================== 五孔探针公式 ====================

// CalculateFiveHoleCoefficients 计算五孔探针系数
//
// 探针孔序：孔1=下，孔2=中，孔3=上，孔4=左，孔5=右
// 孔2(中)=中心孔，孔1/3/4/5=四个侧孔
//
// 标准五孔探针公式：
//
//	P_side = (P1 + P3 + P4 + P5) / 4     — 四个侧孔平均静压
//	Kα = (P4 - P5) / (P2 - P_side)        — 左-右孔压差 / 中-侧压差 = 攻角系数
//	Kβ = (P3 - P1) / (P2 - P_side)        — 上-下孔压差 / 中-侧压差 = 侧滑角系数
//	CPT = (P2 - P∞) / (Pt - P∞)           — 中心孔总压恢复系数
//	CPS = (P_side - P∞) / (P2 - P∞)       — 侧孔静压恢复系数
func CalculateFiveHoleCoefficients(data FiveHoleRawData) FiveHoleCoefficients {
	// 四个侧孔平均压力（不含中心孔）
	pSide := (data.P1 + data.P3 + data.P4 + data.P5) / 4

	// 动压参考：中心孔压力 - 侧孔平均
	qRef := data.P2 - pSide
	safeQ := qRef
	if math.Abs(qRef) < 1e-6 {
		safeQ = 1e-6
	}

	// 计算攻角系数 Kα（左孔-右孔 压差）
	Kalpha := (data.P4 - data.P5) / safeQ

	// 计算侧滑角系数 Kβ（上孔-下孔 压差）
	Kbeta := (data.P3 - data.P1) / safeQ

	// 动压是否大于零（用于判断风洞是否运行）
	hasFlow := math.Abs(qRef) > 1e-6

	// 计算总压系数 CPT（中心孔的总压恢复能力）
	var CPT float64
	if data.PTotal != nil && hasFlow {
		CPT = (data.P2 - *data.PTotal) / safeQ
	}

	// 计算静压系数 CPS（侧孔的静压恢复能力）
	var CPS float64
	if data.PStatic != nil && hasFlow {
		CPS = (pSide - *data.PStatic) / safeQ
	}

	// 计算马赫数（使用 AtmosphericDataCalculator）
	var machNumber *float64
	if data.PTotal != nil && data.PStatic != nil && data.PAtm > 0 {
		totalPressureAbs := *data.PTotal + data.PAtm
		staticPressureAbs := *data.PStatic + data.PAtm
		calc := NewAtmosphericDataCalculator()
		if ma, err := calc.CalculateMach(totalPressureAbs, staticPressureAbs); err == nil {
			machNumber = &ma
		}
	}

	return FiveHoleCoefficients{
		Kalpha:     Kalpha,
		Kbeta:      Kbeta,
		CPT:        CPT,
		CPS:        CPS,
		MachNumber: machNumber,
	}
}

// CalculateFiveHoleAverage 计算多次采样的平均值
func CalculateFiveHoleAverage(dataList []FiveHoleRawData) FiveHoleRawData {
	if len(dataList) == 0 {
		return FiveHoleRawData{}
	}

	var sumP1, sumP2, sumP3, sumP4, sumP5, sumPAtm, sumTAtm float64
	var sumPTotal, sumPStatic float64
	var pTotalCount, pStaticCount int

	for _, d := range dataList {
		sumP1 += d.P1
		sumP2 += d.P2
		sumP3 += d.P3
		sumP4 += d.P4
		sumP5 += d.P5
		sumPAtm += d.PAtm
		sumTAtm += d.TAtm
		if d.PTotal != nil {
			sumPTotal += *d.PTotal
			pTotalCount++
		}
		if d.PStatic != nil {
			sumPStatic += *d.PStatic
			pStaticCount++
		}
	}

	n := float64(len(dataList))
	result := FiveHoleRawData{
		P1:   sumP1 / n,
		P2:   sumP2 / n,
		P3:   sumP3 / n,
		P4:   sumP4 / n,
		P5:   sumP5 / n,
		PAtm: sumPAtm / n,
		TAtm: sumTAtm / n,
	}

	if pTotalCount > 0 {
		avg := sumPTotal / float64(pTotalCount)
		result.PTotal = &avg
	}
	if pStaticCount > 0 {
		avg := sumPStatic / float64(pStaticCount)
		result.PStatic = &avg
	}

	return result
}

// CalculateFiveHoleCoefficientsStdDev 计算五孔探针系数的标准差
func CalculateFiveHoleCoefficientsStdDev(dataList []FiveHoleRawData) (KalphaStd, KbetaStd, CPTStd, CPSStd float64) {
	if len(dataList) < 2 {
		return 0, 0, 0, 0
	}

	coefficientsList := make([]FiveHoleCoefficients, len(dataList))
	for i, d := range dataList {
		coefficientsList[i] = CalculateFiveHoleCoefficients(d)
	}

	kalphaVals := make([]float64, len(coefficientsList))
	kbetaVals := make([]float64, len(coefficientsList))
	cptVals := make([]float64, len(coefficientsList))
	cpsVals := make([]float64, len(coefficientsList))

	for i, c := range coefficientsList {
		kalphaVals[i] = c.Kalpha
		kbetaVals[i] = c.Kbeta
		cptVals[i] = c.CPT
		cpsVals[i] = c.CPS
	}

	return StdDev(kalphaVals), StdDev(kbetaVals), StdDev(cptVals), StdDev(cpsVals)
}

// ==================== 三孔探针公式 ====================

// CalculateThreeHoleCoefficients 计算三孔探针校准系数
//
// 工程命名 Kb(Kβ) / K0 / Kv，口径与插值器 PRB 文件对齐
// （shared/algorithms/go/threehole/interpolation/three_hole.go），
// 使用孔间差压 ΔP 而非对大气压表压，确保校准产物可直接导出为 PRB 供插值器消费：
//
//	ΔP = 2·P2 - P1 - P3            中心孔(P2)与两侧孔(P1/P3)的差压
//	Kb = (P3 - P1) / ΔP            角度系数 Kβ（仅需孔压，始终可算）
//	K0 = (Pt - P2) / ΔP            总压系数 K0（需 PTotal；缺失置 0，不发误导值）
//	Kv = (Pt - Ps) / ΔP            速度系数 Kv（需 PTotal + PStatic；缺失置 0）
//
// 插值时由 K0/Kv 反演：Pt = P2 + K0·ΔP，Ps = Pt - Kv·ΔP。
// 当 |ΔP| < 1e-6（流场未立）时三系数全部置 0，避免除零与误导性输出。
func CalculateThreeHoleCoefficients(data ThreeHoleRawData) ThreeHoleCoefficients {
	deltaP := 2*data.P2 - data.P1 - data.P3
	if math.Abs(deltaP) < 1e-6 {
		return ThreeHoleCoefficients{}
	}

	kb := (data.P3 - data.P1) / deltaP

	var k0 float64
	if data.PTotal != nil {
		k0 = (*data.PTotal - data.P2) / deltaP
	}

	var kv float64
	if data.PTotal != nil && data.PStatic != nil {
		kv = (*data.PTotal - *data.PStatic) / deltaP
	}

	// 马赫数/速度计算：与五孔探针和五孔插值算法口径一致，
	// 通过 AtmosphericDataCalculator.CalculateAll 统一计算。
	// 输入需绝压：Pt_abs = PTotal + PAtm, Ps_abs = PStatic + PAtm；
	// TAT 取大气温度（三孔无独立风洞温度传感器，低速风洞下用大气温度近似）。
	// 任一通道缺失或物理上非法（Pt<=Ps）时不发误导值，置 nil。
	var machNumber *float64
	var velocity *float64
	if data.PTotal != nil && data.PStatic != nil && data.PAtm > 0 {
		ptAbs := *data.PTotal + data.PAtm
		psAbs := *data.PStatic + data.PAtm
		// TAT 转开尔文：CalculateSAT 内部用开氏温度
		tatK := data.TAtm + 273.15
		if tatK > 0 && ptAbs > psAbs && psAbs > 0 {
			calc := NewAtmosphericDataCalculator()
			if result, err := calc.CalculateAll(ptAbs, psAbs, tatK); err == nil {
				ma := result.MachNumber
				v := result.TASMach
				machNumber = &ma
				velocity = &v
			}
		}
	}

	return ThreeHoleCoefficients{Kb: kb, K0: k0, Kv: kv, MachNumber: machNumber, Velocity: velocity}
}

// CalculateThreeHoleAverage 计算三孔探针多次采样平均值
func CalculateThreeHoleAverage(dataList []ThreeHoleRawData) ThreeHoleRawData {
	if len(dataList) == 0 {
		return ThreeHoleRawData{}
	}

	var sumP1, sumP2, sumP3, sumPAtm, sumTAtm, sumPTotal, sumPStatic float64
	var pTotalCount, pStaticCount int

	for _, d := range dataList {
		sumP1 += d.P1
		sumP2 += d.P2
		sumP3 += d.P3
		sumPAtm += d.PAtm
		sumTAtm += d.TAtm
		if d.PTotal != nil {
			sumPTotal += *d.PTotal
			pTotalCount++
		}
		if d.PStatic != nil {
			sumPStatic += *d.PStatic
			pStaticCount++
		}
	}

	n := float64(len(dataList))
	result := ThreeHoleRawData{
		P1:   sumP1 / n,
		P2:   sumP2 / n,
		P3:   sumP3 / n,
		PAtm: sumPAtm / n,
		TAtm: sumTAtm / n,
	}

	if pTotalCount > 0 {
		avg := sumPTotal / float64(pTotalCount)
		result.PTotal = &avg
	}
	if pStaticCount > 0 {
		avg := sumPStatic / float64(pStaticCount)
		result.PStatic = &avg
	}

	return result
}

// CalculateThreeHoleCoefficientsStdDev 计算三孔探针系数的标准差
func CalculateThreeHoleCoefficientsStdDev(dataList []ThreeHoleRawData) (KbStd, K0Std, KvStd float64) {
	if len(dataList) < 2 {
		return 0, 0, 0
	}

	coefficientsList := make([]ThreeHoleCoefficients, len(dataList))
	for i, d := range dataList {
		coefficientsList[i] = CalculateThreeHoleCoefficients(d)
	}

	kbVals := make([]float64, len(coefficientsList))
	k0Vals := make([]float64, len(coefficientsList))
	kvVals := make([]float64, len(coefficientsList))

	for i, c := range coefficientsList {
		kbVals[i] = c.Kb
		k0Vals[i] = c.K0
		kvVals[i] = c.Kv
	}

	return StdDev(kbVals), StdDev(k0Vals), StdDev(kvVals)
}

// ==================== 总压探针公式 ====================

// CalculateTotalPressureCoefficients 计算总压探针系数
//
// 输入的所有压力均为表压（相对大气压），统一转换为绝对压力后计算，保证量纲一致：
//
//	Pt_probe_abs   = pProbeTotal   + pAtm
//	Pt_tunnel_abs  = pTunnelTotal  + pAtm
//	Ps_tunnel_abs  = pTunnelStatic + pAtm
//
//	CPT     = Pt_probe_abs / Pt_tunnel_abs
//	          （探针总压恢复系数，绝对总压相除）
//	误差(%) = (Pt_probe_abs - Pt_tunnel_abs) / Pt_tunnel_abs × 100
//	          （探针总压相对于风洞总压的相对偏差，量纲一致）
//
// 马赫数/速度通过 AtmosphericDataCalculator.CalculateAll 统一计算：
//
//	Ma = √[2/(γ-1) · ((Pt_abs/Ps_abs)^((γ-1)/γ) - 1)]
//	SAT = TAT / (1 + 0.2 · r · Ma²)   （TAT 取风洞温度，需转开尔文）
//	V  = Ma · 20.047 · √SAT
//
// TAT 选取优先级：风洞温度 TTunnel > 大气温度 TAtm（与三孔一致性兜底）。
// 阈值 pressureValidThresholdPa 用于判断"风洞是否建立有效压差"：
// 检查对象是表压 pTunnelTotal（即风洞总压相对于大气压的差值），
// 而非绝对压力 pTunnelTotalAbs——因为后者包含 pAtm（~101325 Pa），
// 即使风洞未运行也会远超阈值，导致 CPT=1.0、误差=0% 的误导输出。
const pressureValidThresholdPa = 100.0 // 100 Pa 表压阈值：风洞总压表压低于此值视为未建立有效压差

func CalculateTotalPressureCoefficients(rawData TotalPressureRawData) TotalPressureCoefficients {
	pAtm := rawData.PAtm

	// 转换为绝对压力，避免表压/绝对压力混用导致的量纲不一致
	pProbeTotalAbs := rawData.PProbeTotal + pAtm
	pTunnelTotalAbs := rawData.PTunnelTotal + pAtm
	pTunnelStaticAbs := rawData.PTunnelStatic + pAtm

	// 阈值过滤：检查表压（风洞建立的压差），而非绝对压力。
	// 风洞未运行时 pTunnelTotal≈0，CPT/误差无物理意义，置 0 避免误导。
	tunnelTotalGaugeValid := rawData.PTunnelTotal > pressureValidThresholdPa

	// 计算 CPT：仅在风洞建立有效压差时才有意义
	var CPT float64
	if tunnelTotalGaugeValid {
		CPT = pProbeTotalAbs / pTunnelTotalAbs
	}

	// 计算误差(%)：分子分母均为绝对压力，量纲一致
	var err float64
	if tunnelTotalGaugeValid {
		err = ((pProbeTotalAbs - pTunnelTotalAbs) / pTunnelTotalAbs) * 100
	}

	// 计算马赫数 + 速度（与三孔一致的指针语义，缺失通道或物理非法时返回 nil）
	// TAT 选取：优先风洞温度 TTunnel，未配置该通道时（read_probe_channels.go 对未映射通道返回 0）
	// 回退到大气温度 TAtm。低速风洞下两者温差小，误差可接受。
	// 注：0°C 是物理合法温度，理论上应改 *float64 区分"未映射"与"0°C"，
	// 但 TotalPressureRawData.TTunnel 为非指针 float64，改动涉及 Average/CSV schema/tests 多处，
	// 且风洞运行温度通常 15-30°C，0°C 极端罕见，权衡后保留 0 哨兵 + TAtm 兜底。
	var machNumber *float64
	var velocity *float64
	if pAtm > 0 && pTunnelStaticAbs > 0 && pTunnelTotalAbs > pTunnelStaticAbs {
		tatC := rawData.TTunnel
		if tatC == 0 {
			tatC = rawData.TAtm
		}
		tatK := tatC + 273.15
		if tatK > 0 {
			calc := NewAtmosphericDataCalculator()
			if result, calcErr := calc.CalculateAll(pTunnelTotalAbs, pTunnelStaticAbs, tatK); calcErr == nil {
				ma := result.MachNumber
				v := result.TASMach
				machNumber = &ma
				velocity = &v
			}
		}
	}

	return TotalPressureCoefficients{
		CPT:        CPT,
		Error:      err,
		MachNumber: machNumber,
		Velocity:   velocity,
	}
}

// CalculateTotalPressureAverage 计算总压探针多次采样平均值
func CalculateTotalPressureAverage(samples []TotalPressureRawData) TotalPressureRawData {
	if len(samples) == 0 {
		return TotalPressureRawData{}
	}

	var sum TotalPressureRawData
	for _, s := range samples {
		sum.PAtm += s.PAtm
		sum.TAtm += s.TAtm
		sum.PTunnelTotal += s.PTunnelTotal
		sum.PTunnelStatic += s.PTunnelStatic
		sum.TTunnel += s.TTunnel
		sum.PProbeTotal += s.PProbeTotal
	}

	n := float64(len(samples))
	return TotalPressureRawData{
		PAtm:          sum.PAtm / n,
		TAtm:          sum.TAtm / n,
		PTunnelTotal:  sum.PTunnelTotal / n,
		PTunnelStatic: sum.PTunnelStatic / n,
		TTunnel:       sum.TTunnel / n,
		PProbeTotal:   sum.PProbeTotal / n,
	}
}

// CalculateTotalPressureStdDev 计算总压探针多次采样中探针总压的样本标准差（Pa）。
//
// 选择 PProbeTotal 作为稳定性指标：
//   - 探针总压是校准的核心被测量，其波动直接反映流场稳定性与采样质量
//   - 与 FiveHole/ThreeHole 使用 P1 标准差的口径保持一致（核心被测量）
//   - StdDev 使用样本标准差（n-1 自由度），n<2 时返回 0
func CalculateTotalPressureStdDev(samples []TotalPressureRawData) float64 {
	if len(samples) < 2 {
		return 0
	}
	values := make([]float64, len(samples))
	for i, s := range samples {
		values[i] = s.PProbeTotal
	}
	return StdDev(values)
}

// ==================== 总温探针公式 ====================

// CalculateMachNumber 计算马赫数（委托给 AtmosphericDataCalculator）
// 公式: Ma = sqrt( (2/(γ-1)) * ((Pt/Ps)^((γ-1)/γ) - 1) )
func CalculateMachNumber(totalPressure, staticPressure float64) (float64, error) {
	calc := NewAtmosphericDataCalculator()
	return calc.CalculateMach(totalPressure, staticPressure)
}

// CalculateRecoveryCoefficient 计算恢复系数
//
// 公式: r = T_measured / T_total
func CalculateRecoveryCoefficient(testProbeTemp, standardProbeTemp float64) (float64, error) {
	if standardProbeTemp <= -273.15 {
		return 0, fmt.Errorf("标准探针温度必须高于绝对零度")
	}

	// 转换为开尔文温度
	TMeasured := testProbeTemp + 273.15
	TTotal := standardProbeTemp + 273.15

	r := TMeasured / TTotal
	return r, nil
}

// CheckTemperatureStability 检测温度稳定性
func CheckTemperatureStability(samples []float64, maxStdDev float64) (stable bool, stdDev float64) {
	if len(samples) < 2 {
		return false, 0
	}

	sd := StdDev(samples)
	return sd <= maxStdDev, sd
}
