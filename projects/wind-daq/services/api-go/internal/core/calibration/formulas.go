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

	// 计算马赫数（需要总压和静压）
	var machNumber *float64
	if data.PTotal != nil && data.PStatic != nil && data.PAtm > 0 {
		totalPressureAbs := *data.PTotal + data.PAtm
		staticPressureAbs := *data.PStatic + data.PAtm
		if staticPressureAbs > 0 && totalPressureAbs > staticPressureAbs {
			pressureRatio := totalPressureAbs / staticPressureAbs
			ma := math.Sqrt(5 * (math.Pow(pressureRatio, 2.0/7.0) - 1))
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

// CalculateThreeHoleCoefficients 计算三孔探针系数
//
// 公式：
//
//	K = (P2 - P3) / (P1 - P∞)  —— 方向系数
//	Cv = (P1 - P∞) / (Pt - P∞)  —— 速度系数（需要总压）
//	Cp = (P1 - P∞) / (Pt - P∞)  —— 总压系数
func CalculateThreeHoleCoefficients(data ThreeHoleRawData) ThreeHoleCoefficients {
	// 计算方向系数 K
	denominator := data.P1 - data.PAtm
	if math.Abs(denominator) < 1e-6 {
		denominator = 1e-6
	}
	K := (data.P2 - data.P3) / denominator

	// 计算速度系数 Cv
	var Cv float64
	if data.PTotal != nil && math.Abs(*data.PTotal-data.PAtm) >= 1e-6 {
		Cv = (data.P1 - data.PAtm) / (*data.PTotal - data.PAtm)
	}

	// 计算总压系数 Cp
	var Cp float64
	if data.PTotal != nil && math.Abs(*data.PTotal-data.PAtm) >= 1e-6 {
		Cp = Cv
	} else {
		dynamicPressure := math.Abs(data.P1 - data.PAtm)
		if dynamicPressure > 1e-6 {
			Cp = 1.0
		}
	}

	return ThreeHoleCoefficients{K: K, Cv: Cv, Cp: Cp}
}

// CalculateThreeHoleAverage 计算三孔探针多次采样平均值
func CalculateThreeHoleAverage(dataList []ThreeHoleRawData) ThreeHoleRawData {
	if len(dataList) == 0 {
		return ThreeHoleRawData{}
	}

	var sumP1, sumP2, sumP3, sumPAtm, sumPTotal float64
	var pTotalCount int

	for _, d := range dataList {
		sumP1 += d.P1
		sumP2 += d.P2
		sumP3 += d.P3
		sumPAtm += d.PAtm
		if d.PTotal != nil {
			sumPTotal += *d.PTotal
			pTotalCount++
		}
	}

	n := float64(len(dataList))
	result := ThreeHoleRawData{
		P1:   sumP1 / n,
		P2:   sumP2 / n,
		P3:   sumP3 / n,
		PAtm: sumPAtm / n,
	}

	if pTotalCount > 0 {
		avg := sumPTotal / float64(pTotalCount)
		result.PTotal = &avg
	}

	return result
}

// CalculateThreeHoleCoefficientsStdDev 计算三孔探针系数的标准差
func CalculateThreeHoleCoefficientsStdDev(dataList []ThreeHoleRawData) (KStd, CvStd, CpStd float64) {
	if len(dataList) < 2 {
		return 0, 0, 0
	}

	coefficientsList := make([]ThreeHoleCoefficients, len(dataList))
	for i, d := range dataList {
		coefficientsList[i] = CalculateThreeHoleCoefficients(d)
	}

	kVals := make([]float64, len(coefficientsList))
	cvVals := make([]float64, len(coefficientsList))
	cpVals := make([]float64, len(coefficientsList))

	for i, c := range coefficientsList {
		kVals[i] = c.K
		cvVals[i] = c.Cv
		cpVals[i] = c.Cp
	}

	return StdDev(kVals), StdDev(cvVals), StdDev(cpVals)
}

// ==================== 总压探针公式 ====================

// CalculateTotalPressureCoefficients 计算总压探针系数
//
// 所有压力值均为表压，需要转换为绝对压力进行计算
//
//	CPT = Pt_probe / Pt_tunnel（简化公式）
//	误差(%) = (Pt_probe - Pt_tunnel) / (Pt_tunnel + P∞) × 100%
//	Ma = √[2×(Pt_tunnel_abs - Ps_tunnel_abs) / (γ × Ps_tunnel_abs)]
func CalculateTotalPressureCoefficients(rawData TotalPressureRawData) TotalPressureCoefficients {
	pAtm := rawData.PAtm
	pTunnelTotal := rawData.PTunnelTotal
	pTunnelStatic := rawData.PTunnelStatic
	pProbeTotal := rawData.PProbeTotal

	// 将表压转换为绝对压力
	pTunnelTotalAbs := pTunnelTotal + pAtm
	pTunnelStaticAbs := pTunnelStatic + pAtm

	// 计算 CPT
	var CPT float64
	if math.Abs(pTunnelTotal) > 1e-6 {
		CPT = pProbeTotal / pTunnelTotal
	}

	// 计算误差
	var err float64
	if math.Abs(pTunnelTotalAbs) > 1e-6 {
		err = ((pProbeTotal - pTunnelTotal) / pTunnelTotalAbs) * 100
	}

	// 计算马赫数
	var machNumber float64
	gamma := 1.4 // 空气比热比
	if pTunnelStaticAbs > 0 {
		pressureDiff := pTunnelTotalAbs - pTunnelStaticAbs
		if pressureDiff >= 0 {
			machNumber = math.Sqrt((2 * pressureDiff) / (gamma * pTunnelStaticAbs))
		}
	}

	return TotalPressureCoefficients{
		CPT:        CPT,
		Error:      err,
		MachNumber: machNumber,
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

// ==================== 总温探针公式 ====================

// CalculateMachNumber 计算马赫数
//
// 公式: Ma = sqrt(5 * ((Pt / Ps)^(2/7) - 1))
func CalculateMachNumber(totalPressure, staticPressure float64) (float64, error) {
	if staticPressure <= 0 {
		return 0, fmt.Errorf("静压必须大于0")
	}

	pressureRatio := totalPressure / staticPressure
	if pressureRatio < 1 {
		return 0, fmt.Errorf("总压必须大于静压")
	}

	ma := math.Sqrt(5 * (math.Pow(pressureRatio, 2.0/7.0) - 1))
	return ma, nil
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
