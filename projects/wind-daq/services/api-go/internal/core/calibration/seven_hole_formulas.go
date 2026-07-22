package calibration

import (
	"fmt"
	"math"
)

// ==================== 七孔探针校准公式（spec §4） ====================
//
// 压力基准三分离声明（spec §2.1，types.go 同名注释同步）：
//   - A 基准（通道原始值）：P1~P7、p_t、p_s 全部为表压（相对环境大气压，可正可负）。
//   - B 基准（系数计算值）：与 A 同基准——系数是压差比，分子分母同基准时表压与绝压等价，
//     因此本文件的内区公式 1-8、外区公式 9-12 直接使用通道原始值，不做 A→B 转换。
//   - C 基准（大气计算值）：仅马赫数公式（§4.4）入口处将 p_t、p_s 转绝压，
//     公式 p_abs = p_gauge + 大气压力。禁止在系数计算阶段提前转绝压。
//
// α 公式负号约束（spec §3.3）：外区 θ,φ → 内区 α,β 换算时
//
//	α = -arctan(tan θ × sin φ)        // 负号必须保留：φ 从探针尾部看顺时针递增，与标准极角方向相反
//
// 此负号属点位生成阶段（Task 6 GenerateSevenHolePoints）的实现细节，本文件公式不涉及；
// 本文件公式只负责"给定压力 → 计算系数"，输入压力基准与坐标系均由调用方保证。

// sevenHoleDivTolerance 七孔公式除零容差。
// 当分母绝对值小于此阈值时视为除零，返回错误而非 NaN/Inf。
//
// 选值依据：扫描阀精度 u1≈20 Pa（spec §5.1），分母接近此量级时
// 系数计算结果已不可信（相对误差 > 100%），不如显式报错让上游处理。
const sevenHoleDivTolerance = 1e-6

// ==================== 内区公式（spec §4.1 公式 1-8） ====================

// CalculateSevenHoleInnerCoefficients 计算七孔探针内区系数（P7 为最大压力孔时）
//
// 实现公式（spec §4.1，A→B 不转换——直接用通道原始表压）：
//
//	P̄  = (P1 + P2 + P3 + P4 + P5 + P6) / 6                ...(4)
//	Cpa = (P4 - P1) / (P7 - P̄)                              ...(1)
//	Cpb = (P5 - P2) / (P7 - P̄)                              ...(2)
//	Cpc = (P6 - P3) / (P7 - P̄)                              ...(3)
//	Kβ  = -(2·Cpa + Cpb - Cpc) / 3                          ...(5)
//	Kα  = (Cpb + Cpc) / √3                                  ...(6)
//	K0  = (P7 - p_t) / (p_t - p_s)                          ...(7)
//	Ks  = (p_s - P̄) / (p_t - p_s)                           ...(8)
//
// 输入约束：
//   - raw.PTotal / raw.PStatic 为指针，缺失（nil）时跳过 K0/Ks/Ma/V 计算，
//     仅返回 Kα/Kβ（这两个系数只依赖 P1~P7）。此语义与五孔/三孔/总压指针字段保持一致。
//
// 错误返回：
//   - P7-P̄ 接近零（来流方向无意义）：返回除零错误
//   - p_t-p_s 接近零（风洞未建立压差，仅当 PTotal/PStatic 同时非 nil 时检查）：返回除零错误
//
// 返回系数中外区字段（Ktheta/Kphi/K0Outer/KsOuter）保持零值，便于下游按 region 区分使用。
func CalculateSevenHoleInnerCoefficients(raw SevenHoleRawData) (SevenHoleCoefficients, error) {
	// 公式 (4)：外围 6 孔平均压力 P̄
	pBar := (raw.P1 + raw.P2 + raw.P3 + raw.P4 + raw.P5 + raw.P6) / 6.0

	// 公式 (1)(2)(3) 分母：P7 - P̄
	// 此分母同时用于 Cpa/Cpb/Cpc 三个中间系数，接近零时整个内区算法无意义
	denomAlphaBeta := raw.P7 - pBar
	if math.Abs(denomAlphaBeta) < sevenHoleDivTolerance {
		return SevenHoleCoefficients{}, fmt.Errorf("内区公式分母 P7-P̄ 接近零 (%.6f), 无法计算 Kα/Kβ", denomAlphaBeta)
	}

	// 中间压力系数 Cpa/Cpb/Cpc（公式 1-3）
	cpa := (raw.P4 - raw.P1) / denomAlphaBeta
	cpb := (raw.P5 - raw.P2) / denomAlphaBeta
	cpc := (raw.P6 - raw.P3) / denomAlphaBeta

	// 目标系数 Kα/Kβ（公式 5-6）
	// Kβ = -(2·Cpa + Cpb - Cpc)/3
	// Kα = (Cpb + Cpc)/√3
	kBeta := -(2.0*cpa + cpb - cpc) / 3.0
	kAlpha := (cpb + cpc) / math.Sqrt(3.0)

	coeffs := SevenHoleCoefficients{
		Kalpha: kAlpha,
		Kbeta:  kBeta,
	}

	// K0/Ks 公式 (7)(8) 依赖 p_t、p_s，PTotal/PStatic 任一缺失时跳过
	// 此设计保持与五孔/三孔/总压的指针字段语义一致：缺失通道不发误导值
	if raw.PTotal != nil && raw.PStatic != nil {
		denomK0Ks := *raw.PTotal - *raw.PStatic
		if math.Abs(denomK0Ks) < sevenHoleDivTolerance {
			// 等压（p_t == p_s）是有效零流量场景：K0/Ks 分母为零无物理意义，
			// 保持零值跳过（与缺失通道同语义，CSV 写空/0，下游插值器不消费）；
			// 但 Ma/V 必须继续计算返回 0，与 live physics（五孔/三孔/总压同走
			// CalculateAll）零流量口径一致，避免七孔 CSV 在零流量场景下显示空值。
			// 旧实现此处直接返回错误，导致等压时 Ma/V 也无法计算。
		} else {
			coeffs.K0 = (raw.P7 - *raw.PTotal) / denomK0Ks
			coeffs.Ks = (*raw.PStatic - pBar) / denomK0Ks
		}
	}

	// 马赫数/速度计算（A→C 边界，仅在此处转绝压）
	calcMachAndVelocity(&coeffs, raw)

	return coeffs, nil
}

// ==================== 外区公式（spec §4.2 公式 9-12，含环形取模） ====================

// CalculateSevenHoleOuterCoefficients 计算七孔探针外区系数（Pn 为最大压力孔时，n ∈ {1..6}）
//
// 实现公式（spec §4.2，环形取模——n=1 时 n-1=6，n=6 时 n+1=1）：
//
//	Kθ[n] = (Pn - P7)     / (Pn - (Pn+1 + Pn-1)/2)        ...(9)
//	Kφ[n] = (Pn-1 - Pn+1) / (Pn - (Pn+1 + Pn-1)/2)        ...(10)
//	K0[n] = (Pn - p_t)    / (p_t - p_s)                    ...(11)
//	Ks[n] = (p_s - (Pn+1 + Pn-1)/2) / (p_t - p_s)         ...(12)
//
// 关键约束（spec §4.3）：
//   - Kφ 边界符号反转不取绝对值——分区边界处 Kφ 在 ±2 之间跳变是七孔算法固有特性，
//     取绝对值会破坏 Kφ-φ 曲线单调性，导致实测反算时角度解算错误。
//
// 输入约束：
//   - n ∈ {1..6}，超出范围返回错误
//   - raw.PTotal / raw.PStatic 缺失时跳过 K0[n]/Ks[n] 与 Ma/V 计算
//
// 错误返回：
//   - n 不在 {1..6} 范围
//   - Pn-(Pn+1+Pn-1)/2 接近零（相邻孔压力均等，无方向性）
//   - p_t-p_s 接近零（仅当 PTotal/PStatic 同时非 nil 时检查）
func CalculateSevenHoleOuterCoefficients(raw SevenHoleRawData, n int) (SevenHoleCoefficients, error) {
	// 扇区编号校验
	if n < 1 || n > 6 {
		return SevenHoleCoefficients{}, fmt.Errorf("外区扇区编号 n=%d 无效, 必须在 1..6 范围内", n)
	}

	// 环形取模：n=1 时 n-1=6；n=6 时 n+1=1
	// next = n%6 + 1   (n=1→2, n=6→1)
	// prev = (n-2+6)%6 + 1   (n=1→6, n=2→1, n=6→5)
	next := n%6 + 1
	prev := (n-2+6)%6 + 1

	// 从 raw 中按编号取出对应孔压力
	// pressures[0..5] 对应 P1..P6；P7 是中心孔单独处理
	pressures := [6]float64{raw.P1, raw.P2, raw.P3, raw.P4, raw.P5, raw.P6}
	pn := pressures[n-1]      // Pn（最大压力孔）
	pnNext := pressures[next-1] // Pn+1（顺时针相邻）
	pnPrev := pressures[prev-1] // Pn-1（逆时针相邻）

	// 公式 (9)(10) 分母：Pn - (Pn+1 + Pn-1)/2
	denomThetaPhi := pn - (pnNext+pnPrev)/2.0
	if math.Abs(denomThetaPhi) < sevenHoleDivTolerance {
		return SevenHoleCoefficients{}, fmt.Errorf("外区公式分母 Pn-(Pn+1+Pn-1)/2 接近零 (%.6f), 无法计算 Kθ/Kφ", denomThetaPhi)
	}

	// 目标系数 Kθ[n]/Kφ[n]（公式 9-10）
	// 注意：Kφ 不取绝对值（spec §4.3 约束）
	kTheta := (pn - raw.P7) / denomThetaPhi
	kPhi := (pnPrev - pnNext) / denomThetaPhi

	coeffs := SevenHoleCoefficients{
		Ktheta: kTheta,
		Kphi:   kPhi,
	}

	// K0[n]/Ks[n] 公式 (11)(12) 依赖 p_t、p_s
	if raw.PTotal != nil && raw.PStatic != nil {
		denomK0Ks := *raw.PTotal - *raw.PStatic
		if math.Abs(denomK0Ks) < sevenHoleDivTolerance {
			// 等压（p_t == p_s）是有效零流量场景：K0[n]/Ks[n] 分母为零无物理意义，
			// 保持零值跳过；Ma/V 继续计算返回 0，与内区及 live physics 零流量口径一致。
			// 旧实现此处直接返回错误，导致外区等压时 Ma/V 也无法计算。
		} else {
			coeffs.K0Outer = (pn - *raw.PTotal) / denomK0Ks
			coeffs.KsOuter = (*raw.PStatic - (pnNext+pnPrev)/2.0) / denomK0Ks
		}
	}

	// 马赫数/速度计算（与内区共用，A→C 边界转换）
	calcMachAndVelocity(&coeffs, raw)

	return coeffs, nil
}

// ==================== 马赫数与速度公式（spec §4.4） ====================

// CalculateSevenHoleMachNumber 计算七孔探针马赫数
//
// 实现公式（spec §4.4，A→C 边界——仅在此处把表压转绝压）：
//
//	p_t_abs = p_t + 大气压力
//	p_s_abs = p_s + 大气压力
//	Ma = √( (2/(γ-1)) × ((p_t_abs / p_s_abs)^((γ-1)/γ) - 1) )     γ = 1.4
//
// 输入参数（全部为表压 A 基准）：
//   - pTunnel: 风洞参考总压（表压）
//   - pStatic: 风洞参考静压（表压，可为负）
//   - atmPressure: 大气压力（绝压，必须 > 0）
//
// 错误返回：
//   - atmPressure ≤ 0：无法转绝压
//   - p_s_abs ≤ 0：物理无意义
//   - p_t_abs < p_s_abs：亚音速风洞中总压不可能小于静压
//
// Task 12 零流量语义：p_t_abs == p_s_abs 视为有效零流量，返回 Ma=0, nil；
// 详见 AtmosphericDataCalculator.CalculateMach。
//
// 内部委托给 AtmosphericDataCalculator.CalculateMach 复用既有马赫数实现（与五孔/三孔/总压一致）。
func CalculateSevenHoleMachNumber(pTunnel, pStatic, atmPressure float64) (float64, error) {
	// A→C 边界转换：仅在马赫数计算入口把表压转绝压（spec §2.1 关键规则 2）
	if atmPressure <= 0 {
		return 0, fmt.Errorf("大气压力必须 > 0, 实际 %.6f", atmPressure)
	}
	ptAbs := pTunnel + atmPressure
	psAbs := pStatic + atmPressure

	// 委托给既有 AtmosphericDataCalculator（与五孔/三孔/总压同一实现路径，保证 Ma 口径一致）
	calc := NewAtmosphericDataCalculator()
	return calc.CalculateMach(ptAbs, psAbs)
}

// calcMachAndVelocity 共用的马赫数/速度计算逻辑
//
// 内区与外区公式都需要计算 Ma/V，且规则相同：
//   - PTotal/PStatic/PAtm 任一缺失（PTotal/PStatic 为 nil 或 PAtm ≤ 0）→ Ma/V 保持 nil
//   - 物理非法（pt < ps）→ Ma/V 保持 nil（CalculateMach 返回错误时不赋值）
//   - 等压（pt == ps）→ 有效零流量，CalculateMach 返回 Ma=0, V=0（Task 12 零流量语义）
//   - TAT 优先级：TTunnel > TAtm（spec §4.4）
//   - 速度公式：V = Ma × 20.047 × √SAT（由 AtmosphericDataCalculator.CalculateTASByMach 实现）
//
// 此函数直接修改 coeffs 的 MachNumber/Velocity 指针字段，避免内/外区公式重复实现。
func calcMachAndVelocity(coeffs *SevenHoleCoefficients, raw SevenHoleRawData) {
	// 通道完整性检查
	if raw.PTotal == nil || raw.PStatic == nil || raw.PAtm <= 0 {
		return
	}

	// A→C 边界：表压转绝压
	ptAbs := *raw.PTotal + raw.PAtm
	psAbs := *raw.PStatic + raw.PAtm
	// 等压（ptAbs == psAbs）是有效零流量，必须放行到 CalculateAll 返回 Ma=0, V=0；
	// 仅 ptAbs < psAbs 为物理非法（亚音速风洞总压不可能小于静压）。
	// 旧实现 `ptAbs <= psAbs` 一律拦截，导致七孔 CSV/样本在零流量场景下 Ma/V 为空，
	// 而 live physics（五孔/三孔/总压同走 CalculateAll）却返回 0，造成前后端口径不一致。
	if psAbs <= 0 || ptAbs < psAbs {
		return
	}

	// TAT 选取：优先风洞温度 TTunnel，缺失时回退大气温度 TAtm
	// 注：TTunnel=0 视为未配置（与总压模块 TotalPressureCoefficients 同口径，
	// 详见 formulas.go 中 TotalPressure 的同名注释）
	tatC := raw.TAtm
	if raw.TTunnel != nil && *raw.TTunnel != 0 {
		tatC = *raw.TTunnel
	}
	tatK := tatC + 273.15
	if tatK <= 0 {
		return
	}

	// 调用既有 AtmosphericDataCalculator.CalculateAll 一次性算出 Ma+SAT+V
	// 复用而非重写，保证与五孔/三孔/总压同口径
	calc := NewAtmosphericDataCalculator()
	result, err := calc.CalculateAll(ptAbs, psAbs, tatK)
	if err != nil {
		return
	}

	ma := result.MachNumber
	v := result.TASMach
	coeffs.MachNumber = &ma
	coeffs.Velocity = &v
}

// ==================== 多次采样统计工具（spec Task 5） ====================

// CalculateSevenHoleAverage 计算七孔探针多次采样的算术平均值
//
// 用于 AcquireDataWithChannels 采样循环结束后的均值计算——
// 后续的 DetermineRegion 与系数计算都基于此均值，而非单次样本。
//
// 指针字段（PTotal/PStatic/TTunnel）处理：
//   - 全部样本都缺失（nil）→ 平均值也为 nil
//   - 部分样本缺失 → 仅对非 nil 样本求平均（避免零值拉低均值）
//
// 此设计与五孔/三孔/总压的指针字段语义保持一致：缺失通道不发误导值。
func CalculateSevenHoleAverage(samples []SevenHoleRawData) SevenHoleRawData {
	if len(samples) == 0 {
		return SevenHoleRawData{}
	}

	var sumP1, sumP2, sumP3, sumP4, sumP5, sumP6, sumP7, sumPAtm, sumTAtm float64
	var sumPTotal, sumPStatic, sumTTunnel float64
	countPTotal, countPStatic, countTTunnel := 0, 0, 0

	for _, s := range samples {
		sumP1 += s.P1
		sumP2 += s.P2
		sumP3 += s.P3
		sumP4 += s.P4
		sumP5 += s.P5
		sumP6 += s.P6
		sumP7 += s.P7
		sumPAtm += s.PAtm
		sumTAtm += s.TAtm
		if s.PTotal != nil {
			sumPTotal += *s.PTotal
			countPTotal++
		}
		if s.PStatic != nil {
			sumPStatic += *s.PStatic
			countPStatic++
		}
		if s.TTunnel != nil {
			sumTTunnel += *s.TTunnel
			countTTunnel++
		}
	}

	n := float64(len(samples))
	result := SevenHoleRawData{
		P1:   sumP1 / n,
		P2:   sumP2 / n,
		P3:   sumP3 / n,
		P4:   sumP4 / n,
		P5:   sumP5 / n,
		P6:   sumP6 / n,
		P7:   sumP7 / n,
		PAtm: sumPAtm / n,
		TAtm: sumTAtm / n,
	}

	// 指针字段：仅当至少有一个样本非 nil 时才填充，否则保持 nil
	if countPTotal > 0 {
		v := sumPTotal / float64(countPTotal)
		result.PTotal = &v
	}
	if countPStatic > 0 {
		v := sumPStatic / float64(countPStatic)
		result.PStatic = &v
	}
	if countTTunnel > 0 {
		v := sumTTunnel / float64(countTTunnel)
		result.TTunnel = &v
	}

	return result
}

// CalculateSevenHoleStdDev 计算七孔探针 P7（中心孔）多次采样的样本标准差
//
// 选 P7 作为稳定性指标的原因：
//   - P7 是中心孔，正对来流方向，压力值最大且对来流偏角变化最敏感
//   - 与五孔（P1）、三孔（P1）、总压（PProbeTotal）保持"选代表性通道做稳定性指标"的一致模式
//   - P7 抖动小 → 整个七孔阵列采样都稳定（外围孔压力更小，相对噪声更大）
//
// 样本数 < 2 时返回 0（与 StdDev 实现一致）。
func CalculateSevenHoleStdDev(samples []SevenHoleRawData) float64 {
	if len(samples) < 2 {
		return 0
	}
	values := make([]float64, len(samples))
	for i, s := range samples {
		values[i] = s.P7
	}
	return StdDev(values)
}

// ==================== 角度坐标系换算（spec §3.3） ====================
//
// 两套坐标系几何关系（spec §3.3）：
//   - 内区 (α, β)：直角坐标系下的来流偏转角（α=侧滑角，β=迎角）
//   - 外区 (θ, φ)：球坐标系下的来流偏转角（θ=俯仰角，φ=滚转角）
//
// φ 方向约定（spec §3.3 关键约束）：从探针尾部看（-Z 方向看向 +Z）顺时针递增。
// 此约定与标准数学极坐标方向相反，因此 α 公式必须带负号。
//
// 推导（spec §3.3）：
//   - 来流方向单位向量 v=(vx,vy,vz)，vz=cosθ，|v_xy|=sinθ
//   - vy = sinθ·cosφ（φ=0° 时 vy=sinθ 对应 +Y）
//   - vx = -sinθ·sinφ（φ=270° 时 vx=+sinθ 对应 +X；负号源于 φ 方向与标准极角相反）
//   - 由 tanα=vx/vz、tanβ=vy/vz 得出以下公式

// ConvertThetaPhiToAlphaBeta 外区 (θ,φ) → 内区 (α,β) 正向换算（spec §3.3）
//
// 输入：theta/phi 单位为角度（°），与 CSV/前端展示口径一致
// 输出：alpha/beta 单位为角度（°），与运动控制器下发口径一致
//
// 公式（spec §3.3 正向公式，弧度计算后转回角度）：
//
//	α = -arctan(tan θ × sin φ)        // 负号必须保留：φ 从探针尾部看顺时针递增
//	β = arctan(tan θ × cos φ)
//
// α 负号约束（spec §3.3 Critical 1）：
//   - 若误删负号，会导致外区运动控制器下发的 X 轴方向相反
//   - 来流偏 +X（φ=270°）时探针反而向 -X 运动
//   - 黄金用例 G2/G4（spec §3.3 表）专门验证此符号
//
// 黄金用例（spec §3.3 表，单位：度）：
//   - G1: θ=30°, φ=0°   → α=0°,    β=+30°
//   - G2: θ=30°, φ=90°  → α=-30°,  β=0°
//   - G3: θ=30°, φ=180° → α=0°,    β=-30°
//   - G4: θ=30°, φ=270° → α=+30°,  β=0°
//   - G5: θ=30°, φ=330° → α=+16.1°,β=+26.6°
func ConvertThetaPhiToAlphaBeta(thetaDeg, phiDeg float64) (alphaDeg, betaDeg float64) {
	thetaRad := thetaDeg * math.Pi / 180.0
	phiRad := phiDeg * math.Pi / 180.0

	// tan θ × sin φ / cos φ（弧度）
	tanTheta := math.Tan(thetaRad)
	alphaRad := -math.Atan(tanTheta * math.Sin(phiRad))
	betaRad := math.Atan(tanTheta * math.Cos(phiRad))

	// 弧度转角度
	alphaDeg = alphaRad * 180.0 / math.Pi
	betaDeg = betaRad * 180.0 / math.Pi
	return
}

// ConvertAlphaBetaToThetaPhi 内区 (α,β) → 外区 (θ,φ) 反向换算（spec §3.3）
//
// 用于内区数据回算到 θ-φ 平面或结果统一展示。
//
// 公式（spec §3.3 反向公式）：
//
//	θ = arctan( √(tan²α + tan²β) )
//	φ = atan2(-tan α, tan β)          // atan2(y, x) 参数顺序：y=-tanα，x=tanβ
//
// φ 范围归一化到 [0°, 360°)——atan2 返回 [-π, π]，负值加 2π 转正。
func ConvertAlphaBetaToThetaPhi(alphaDeg, betaDeg float64) (thetaDeg, phiDeg float64) {
	alphaRad := alphaDeg * math.Pi / 180.0
	betaRad := betaDeg * math.Pi / 180.0

	tanAlpha := math.Tan(alphaRad)
	tanBeta := math.Tan(betaRad)

	thetaRad := math.Atan(math.Sqrt(tanAlpha*tanAlpha + tanBeta*tanBeta))
	phiRad := math.Atan2(-tanAlpha, tanBeta)

	// 归一化 φ 到 [0, 2π)
	if phiRad < 0 {
		phiRad += 2 * math.Pi
	}

	thetaDeg = thetaRad * 180.0 / math.Pi
	phiDeg = phiRad * 180.0 / math.Pi
	return
}
