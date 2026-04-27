package calibration

import "math"

// ── 五孔校准公式 ──

// CalculateFiveHoleCoefficients 计算五孔探针校准系数
func CalculateFiveHoleCoefficients(raw FiveHoleRawData) FiveHoleCoefficients {
	Pavg := (raw.P2 + raw.P3 + raw.P4 + raw.P5) / 4.0
	denom := raw.P1 - Pavg
	if math.Abs(denom) < 1e-6 {
		denom = 1e-6
	}

	Kalpha := (raw.P2 - raw.P3) / denom
	Kbeta := (raw.P4 - raw.P5) / denom

	// 总压系数
	var CPT float64
	if raw.PTotal != 0 {
		denomCPT := raw.PTotal - raw.PAtm
		if math.Abs(denomCPT) < 1e-6 {
			denomCPT = 1e-6
		}
		CPT = (raw.P1 - raw.PAtm) / denomCPT
	} else {
		if raw.P1 > raw.PAtm {
			CPT = 1.0
		}
	}

	// 静压系数 — 添加 p1 > pAtm 保护（对齐 TS）
	var CPS float64
	if raw.P1 > raw.PAtm {
		CPS = (Pavg - raw.PAtm) / denom
	}

	return FiveHoleCoefficients{
		Kalpha: Kalpha,
		Kbeta:  Kbeta,
		CPT:    CPT,
		CPS:    CPS,
	}
}

// ── 三孔校准公式 ──

// CalculateThreeHoleCoefficients 计算三孔探针校准系数
func CalculateThreeHoleCoefficients(raw ThreeHoleRawData) ThreeHoleCoefficients {
	denom := raw.P1 - raw.PAtm
	if math.Abs(denom) < 1e-6 {
		denom = 1e-6
	}

	K := (raw.P2 - raw.P3) / denom

	var Cv, Cp float64
	if raw.PTotal != 0 {
		denomCv := raw.PTotal - raw.PAtm
		if math.Abs(denomCv) < 1e-6 {
			denomCv = 1e-6
		}
		Cv = (raw.P1 - raw.PAtm) / denomCv
		Cp = Cv
	} else {
		Cp = 1.0
		if math.Abs(raw.P1-raw.PAtm) < 1e-6 {
			Cp = 0
		}
	}

	return ThreeHoleCoefficients{
		K:  K,
		Cv: Cv,
		Cp: Cp,
	}
}

// ── 总压校准公式 ──

// CalculateTotalPressureCoefficients 计算总压校准系数
func CalculateTotalPressureCoefficients(raw TotalPressureRawData) TotalPressureCoefficients {
	// 表压转绝对压力
	PtTunnelAbs := raw.PTunnelTotal + raw.PAtm
	PsTunnelAbs := raw.PTunnelStatic + raw.PAtm

	// 总压系数 (表压比)
	var CPT float64
	if math.Abs(raw.PTunnelTotal) > 1e-6 {
		CPT = raw.PProbeTotal / raw.PTunnelTotal
	}

	// 误差(%)
	var errPct float64
	if math.Abs(PtTunnelAbs) > 1e-6 {
		errPct = (raw.PProbeTotal - raw.PTunnelTotal) / PtTunnelAbs * 100.0
	}

	// 马赫数: Ma = sqrt(2*(Pt-Ps)/(γ*Ps)), γ=1.4
	var machNumber float64
	if PsTunnelAbs > 0 {
		q := PtTunnelAbs - PsTunnelAbs
		if q > 0 {
			machNumber = math.Sqrt(2 * q / (1.4 * PsTunnelAbs))
		}
	}

	return TotalPressureCoefficients{
		CPT:        CPT,
		Error:      errPct,
		MachNumber: machNumber,
	}
}

// ── 总温校准公式 ──

// CalculateMachNumber 从总压/静压计算马赫数
// Ma = sqrt(5 * ((Pt/Ps)^(2/7) - 1))
func CalculateMachNumber(totalPressure, staticPressure float64) float64 {
	if staticPressure <= 0 || totalPressure < staticPressure {
		return 0
	}
	ratio := totalPressure / staticPressure
	if ratio <= 0 {
		return 0
	}
	val := 5.0 * (math.Pow(ratio, 2.0/7.0) - 1.0)
	if val < 0 {
		return 0
	}
	return math.Sqrt(val)
}

// CalculateRecoveryCoefficient 计算恢复系数
// r = T_measured / T_total (开尔文)
func CalculateRecoveryCoefficient(tMeasured, tTotal float64) float64 {
	TmeasK := tMeasured + 273.15
	TtotalK := tTotal + 273.15
	if TtotalK <= 0 {
		return 0
	}
	return TmeasK / TtotalK
}

// CheckTemperatureStability 检查温度稳定性
func CheckTemperatureStability(samples []float64, maxStdDev float64) bool {
	if len(samples) < 2 {
		return true
	}
	return CalculateStdDev(samples) <= maxStdDev
}

// ── 通用辅助函数 ──

// CalculateAverage 计算平均值
func CalculateAverage(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	sum := 0.0
	for _, v := range values {
		sum += v
	}
	return sum / float64(len(values))
}

// CalculateStdDev 计算样本标准差 (贝塞尔校正)
func CalculateStdDev(values []float64) float64 {
	n := len(values)
	if n < 2 {
		return 0
	}
	mean := CalculateAverage(values)
	sumSq := 0.0
	for _, v := range values {
		d := v - mean
		sumSq += d * d
	}
	return math.Sqrt(sumSq / float64(n-1))
}

// ── 球罐闸门判定 ──

// IsSphereTankGateSatisfied 球罐闸门判定
func IsSphereTankGateSatisfied(gate *SphereTankGateConfig, stableTimeSec float64) bool {
	if gate == nil || !gate.Enabled {
		return true // 未启用直接放行
	}
	return stableTimeSec >= gate.WaitTimeSec
}

// ── 五孔结构体平均 ──

// CalculateFiveHoleAverage 计算多组五孔原始数据的平均值
func CalculateFiveHoleAverage(samples []FiveHoleRawData) FiveHoleRawData {
	n := len(samples)
	if n == 0 {
		return FiveHoleRawData{}
	}
	var sum FiveHoleRawData
	for _, s := range samples {
		sum.P1 += s.P1
		sum.P2 += s.P2
		sum.P3 += s.P3
		sum.P4 += s.P4
		sum.P5 += s.P5
		sum.PAtm += s.PAtm
		sum.TAtm += s.TAtm
		sum.PTotal += s.PTotal
	}
	return FiveHoleRawData{
		P1: sum.P1 / float64(n), P2: sum.P2 / float64(n),
		P3: sum.P3 / float64(n), P4: sum.P4 / float64(n),
		P5:     sum.P5 / float64(n),
		PAtm:   sum.PAtm / float64(n),
		TAtm:   sum.TAtm / float64(n),
		PTotal: sum.PTotal / float64(n),
	}
}

// ── 三孔结构体平均 ──

// CalculateThreeHoleAverage 计算多组三孔原始数据的平均值
func CalculateThreeHoleAverage(samples []ThreeHoleRawData) ThreeHoleRawData {
	n := len(samples)
	if n == 0 {
		return ThreeHoleRawData{}
	}
	var sum ThreeHoleRawData
	for _, s := range samples {
		sum.P1 += s.P1
		sum.P2 += s.P2
		sum.P3 += s.P3
		sum.PAtm += s.PAtm
		sum.PTotal += s.PTotal
	}
	return ThreeHoleRawData{
		P1: sum.P1 / float64(n), P2: sum.P2 / float64(n), P3: sum.P3 / float64(n),
		PAtm:   sum.PAtm / float64(n),
		PTotal: sum.PTotal / float64(n),
	}
}

// ── 系数标准差 ──

// FiveHoleCoefficientsStdDev 五孔系数标准差结果
type FiveHoleCoefficientsStdDev struct {
	Kalpha float64 `json:"Kalpha"`
	Kbeta  float64 `json:"Kbeta"`
	CPT    float64 `json:"CPT"`
	CPS    float64 `json:"CPS"`
}

// CalculateCoefficientsStdDev 计算多组五孔系数的标准差
func CalculateCoefficientsStdDev(coeffs []FiveHoleCoefficients) FiveHoleCoefficientsStdDev {
	n := len(coeffs)
	if n < 2 {
		return FiveHoleCoefficientsStdDev{}
	}
	kalphaVals := make([]float64, n)
	kbetaVals := make([]float64, n)
	cptVals := make([]float64, n)
	cpsVals := make([]float64, n)
	for i, c := range coeffs {
		kalphaVals[i] = c.Kalpha
		kbetaVals[i] = c.Kbeta
		cptVals[i] = c.CPT
		cpsVals[i] = c.CPS
	}
	return FiveHoleCoefficientsStdDev{
		Kalpha: CalculateStdDev(kalphaVals),
		Kbeta:  CalculateStdDev(kbetaVals),
		CPT:    CalculateStdDev(cptVals),
		CPS:    CalculateStdDev(cpsVals),
	}
}

// ThreeHoleCoefficientsStdDev 三孔系数标准差结果
type ThreeHoleCoefficientsStdDev struct {
	K  float64 `json:"K"`
	Cv float64 `json:"Cv"`
	Cp float64 `json:"Cp"`
}

// CalculateThreeHoleCoefficientsStdDev 计算多组三孔系数的标准差
func CalculateThreeHoleCoefficientsStdDev(coeffs []ThreeHoleCoefficients) ThreeHoleCoefficientsStdDev {
	n := len(coeffs)
	if n < 2 {
		return ThreeHoleCoefficientsStdDev{}
	}
	kVals := make([]float64, n)
	cvVals := make([]float64, n)
	cpVals := make([]float64, n)
	for i, c := range coeffs {
		kVals[i] = c.K
		cvVals[i] = c.Cv
		cpVals[i] = c.Cp
	}
	return ThreeHoleCoefficientsStdDev{
		K:  CalculateStdDev(kVals),
		Cv: CalculateStdDev(cvVals),
		Cp: CalculateStdDev(cpVals),
	}
}

// ── 温度稳定性（返回 stdDev）──

// TemperatureStabilityResult 温度稳定性检查结果
type TemperatureStabilityResult struct {
	Stable bool    `json:"stable"`
	StdDev float64 `json:"stdDev"`
}

// CheckTemperatureStabilityWithResult 检查温度稳定性，返回详细结果
func CheckTemperatureStabilityWithResult(samples []float64, maxStdDev float64) TemperatureStabilityResult {
	if len(samples) < 2 {
		return TemperatureStabilityResult{Stable: true, StdDev: 0}
	}
	stdDev := CalculateStdDev(samples)
	return TemperatureStabilityResult{
		Stable: stdDev <= maxStdDev,
		StdDev: stdDev,
	}
}
