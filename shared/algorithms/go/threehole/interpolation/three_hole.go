package interpolation

import (
	"fmt"
	"math"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

const (
	maxIterations = 20
	convergeTol   = 1e-4
	deltaPTol     = 1e-6
	// alphaGridEps 是多文件加载时 Alpha 网格一致性校验的容差（deg）。
	alphaGridEps = 1e-9

	// 空气比热比温度修正系数（-40°C ~ +60°C 范围内误差 <0.1%）
	gammaRef  = 1.4    // 20°C时的参考比热比（γ）
	tempRef   = 20.0   // 参考温度(°C)
	tempCoeff = 0.0002 // 温度修正系数 γ ≈ gammaRef - tempCoeff*(T-tempRef)
)

type calibrationItem struct {
	Kb    float64 // 角度系数 Kβ（仅孔压，始终可算）
	K0    float64 // 总压系数 K0（反演 Pt = P2 + K0·ΔP）
	Kv    float64 // 速度系数 Kv（反演 Ps = Pt - Kv·ΔP）
	Alpha float64
}

type calibrationData struct {
	CMa    float64
	Nalpha int
	Items  []calibrationItem
	// kbSorted 是 Items 按 Kb 升序排序的副本，加载时预计算。
	// 单文件快速路径（interpolateSingle）直接查表，避免每帧重复排序/分配。
	kbSorted []calibrationItem
}

type ThreeHoleInterpolator struct {
	loaded   bool
	calib    []calibrationData
	alphaSeq []float64
	initMa   float64
	minMa    float64
	maxMa    float64
}

type kbAlphaEntry struct {
	Kb    float64
	Alpha float64
	K0    float64
	Kv    float64
}

func NewThreeHoleInterpolator() *ThreeHoleInterpolator {
	return &ThreeHoleInterpolator{}
}

// validateFiniteInput 校验输入字段的有限性（NaN/Inf）。
//
// 改善项 A1：P1/P2/P3 为 NaN/Inf 时会被 deltaP/Kb 校验拦截，但 Patm=NaN 会一路
// 穿透到 calcMach 产生 NaN 马赫数，而 NaN 与校准范围比较恒为 false，导致
// isValid=true 且 MachNumber=NaN 的静默放行。此处统一在入口拦截。
//
// 错误契约（见 docs/three-hole-interpolation-improvements.md A1）：返回结构化
// 无效结果而非 error——BatchCalculateThreeHole 中 error 会使整批 Success=false，
// 而目标是"个别坏行不使批次失败"。
func validateFiniteInput(in InterpolationInput) error {
	fields := []struct {
		name string
		v    float64
	}{
		{"P1", in.P1}, {"P2", in.P2}, {"P3", in.P3},
		{"Patm", in.PAtm}, {"Tatm", in.TAtm},
	}
	for _, f := range fields {
		if math.IsNaN(f.v) || math.IsInf(f.v, 0) {
			return fmt.Errorf("输入字段 %s 为非有限数值: %v", f.name, f.v)
		}
	}
	return nil
}

func (t *ThreeHoleInterpolator) IsLoaded() bool {
	return t.loaded
}

func (t *ThreeHoleInterpolator) GetValidRange() PrbValidRange {
	if !t.loaded {
		return PrbValidRange{}
	}
	return PrbValidRange{
		AlphaMin: t.alphaSeq[0],
		AlphaMax: t.alphaSeq[len(t.alphaSeq)-1],
		MachMin:  t.minMa,
		MachMax:  t.maxMa,
	}
}

func (t *ThreeHoleInterpolator) Calculate(input InterpolationInput) (InterpolationResult, error) {
	if !t.loaded {
		return InterpolationResult{}, fmt.Errorf("校准数据未加载")
	}

	// A1：输入有限性校验（契约：结构化无效结果，不返回 error）。
	if err := validateFiniteInput(input); err != nil {
		return InterpolationResult{IsValid: false, Warning: err.Error()}, nil
	}

	p1, p2, p3 := input.P1, input.P2, input.P3
	pa := input.PAtm
	tatm := input.TAtm

	deltaP := 2*p2 - p1 - p3
	if math.Abs(deltaP) < deltaPTol {
		return InterpolationResult{
			TotalPressure:  p2,
			StaticPressure: p2,
			IsValid:        false,
			Warning:        "压力差分ΔP接近零，无法计算",
		}, nil
	}

	kbTemp := (p3 - p1) / deltaP
	if math.IsInf(kbTemp, 0) || math.IsNaN(kbTemp) {
		return InterpolationResult{
			Alpha:          0,
			MachNumber:     t.initMa,
			TotalPressure:  p2,
			StaticPressure: p2,
			IsValid:        false,
			Warning:        "Kb值为无穷大或非数值",
		}, nil
	}

	// B1：单文件快速路径。len(calib)==1 时 calib1==calib2、ratio=0，
	// 插值结果与 Ma 无关，迭代循环与每帧排序均为空转；直接单次插值，
	// IterationCount=1，且无 maClamped 警告（C2）。
	if len(t.calib) == 1 {
		return t.finalizeSingle(kbTemp, p2, deltaP, pa, tatm), nil
	}

	return t.calculateMulti(kbTemp, p2, deltaP, pa, tatm), nil
}

// finalizeSingle 单 PRB 文件快速路径：按预排序 Kb 表插值一次并组装结果。
// IterationCount 固定为 1（B1 对外语义变化，见 docs §B1）。
func (t *ThreeHoleInterpolator) finalizeSingle(kbTemp, p2, deltaP, pa, tatm float64) InterpolationResult {
	match, kbExtrapolated := t.interpolateSingle(kbTemp)
	if match == nil {
		return InterpolationResult{
			Alpha:          0,
			MachNumber:     t.initMa,
			TotalPressure:  p2,
			StaticPressure: p2,
			IterationCount: 1,
			IsValid:        false,
			Warning:        "最终插值未能返回有效结果",
		}
	}

	pt := p2 + match.K0*deltaP
	ps := pt - match.Kv*deltaP
	mach, err := calcMach(pt, ps, pa, tatm)
	if err != nil {
		return InterpolationResult{
			TotalPressure:  pt,
			StaticPressure: ps,
			IterationCount: 1,
			IsValid:        false,
			Warning:        err.Error(),
		}
	}

	var warnings []string
	isValid := true

	// A3：超范围警告携带实际值与校准范围，便于定位。
	if mach > t.maxMa+0.01 || mach < t.minMa-0.01 {
		warnings = append(warnings, fmt.Sprintf("计算马赫数超出校准范围: 恢复Ma=%.3f，校准范围[%.3f, %.3f]", mach, t.minMa, t.maxMa))
		isValid = false
	}
	if kbExtrapolated {
		warnings = append(warnings, "Kb值超出校准数据范围，已使用最近边界点外推")
	}

	return InterpolationResult{
		Alpha:          match.Alpha,
		MachNumber:     mach,
		TotalPressure:  pt,
		StaticPressure: ps,
		IterationCount: 1,
		Calculated:     true,
		IsValid:        isValid,
		Warning:        strings.Join(warnings, "; "),
	}
}

// calculateMulti 多 PRB 文件路径：保留原迭代收敛行为（B1 只优化单文件场景）。
func (t *ThreeHoleInterpolator) calculateMulti(kbTemp, p2, deltaP, pa, tatm float64) InterpolationResult {
	currentMa := t.initMa
	iteration := 0
	maClamped := false

	for iteration = 0; iteration < maxIterations; iteration++ {
		match, _ := t.interpolateWithWarning(kbTemp, currentMa)
		if match == nil {
			break
		}

		pt := p2 + match.K0*deltaP
		ps := pt - match.Kv*deltaP
		newMa, err := calcMach(pt, ps, pa, tatm)
		if err != nil {
			return InterpolationResult{
				TotalPressure:  pt,
				StaticPressure: ps,
				IterationCount: iteration + 1,
				IsValid:        false,
				Warning:        err.Error(),
			}
		}

		if math.Abs(newMa-currentMa) < convergeTol {
			currentMa = newMa
			break
		}

		clampedMa := math.Max(t.minMa, math.Min(t.maxMa, newMa))
		if math.Abs(clampedMa-newMa) > 1e-6 {
			maClamped = true
		}
		currentMa = clampedMa
	}

	finalMatch, kbExtrapolated := t.interpolateWithWarning(kbTemp, currentMa)
	if finalMatch == nil {
		return InterpolationResult{
			Alpha:          0,
			MachNumber:     currentMa,
			TotalPressure:  p2,
			StaticPressure: p2,
			IterationCount: iteration,
			IsValid:        false,
			Warning:        "最终插值未能返回有效结果",
		}
	}

	pt := p2 + finalMatch.K0*deltaP
	ps := pt - finalMatch.Kv*deltaP
	mach, err := calcMach(pt, ps, pa, tatm)
	if err != nil {
		return InterpolationResult{
			TotalPressure:  pt,
			StaticPressure: ps,
			IterationCount: iteration + 1,
			IsValid:        false,
			Warning:        err.Error(),
		}
	}

	var warnings []string
	isValid := true

	// A3：超范围警告携带实际值与校准范围，便于定位。
	if mach > t.maxMa+0.01 || mach < t.minMa-0.01 {
		warnings = append(warnings, fmt.Sprintf("计算马赫数超出校准范围: 恢复Ma=%.3f，校准范围[%.3f, %.3f]", mach, t.minMa, t.maxMa))
		isValid = false
	}
	if maClamped {
		warnings = append(warnings, "迭代过程中马赫数被限制到标定边界，结果精度可能降低")
	}
	if kbExtrapolated {
		warnings = append(warnings, "Kb值超出校准数据范围，已使用最近边界点外推")
	}

	return InterpolationResult{
		Alpha:          finalMatch.Alpha,
		MachNumber:     mach,
		TotalPressure:  pt,
		StaticPressure: ps,
		IterationCount: iteration + 1,
		Calculated:     true,
		IsValid:        isValid,
		Warning:        strings.Join(warnings, "; "),
	}
}

// interpolateSingle 单文件插值（B2）：使用加载时预计算的 Kb 排序表直接查表，
// 无分配、无每帧排序。行为与 interpolateWithWarning 在 len(calib)==1 时一致。
func (t *ThreeHoleInterpolator) interpolateSingle(kbMeasured float64) (*calibrationItem, bool) {
	entries := t.calib[0].kbSorted
	if len(entries) == 0 {
		return nil, false
	}
	kbExtrapolated := false

	if kbMeasured <= entries[0].Kb {
		if kbMeasured < entries[0].Kb {
			kbExtrapolated = true
		}
		return &entries[0], kbExtrapolated
	}
	if kbMeasured >= entries[len(entries)-1].Kb {
		if kbMeasured > entries[len(entries)-1].Kb {
			kbExtrapolated = true
		}
		last := &entries[len(entries)-1]
		return last, kbExtrapolated
	}

	for j := 0; j < len(entries)-1; j++ {
		if kbMeasured >= entries[j].Kb && kbMeasured <= entries[j+1].Kb {
			r := (kbMeasured - entries[j].Kb) / (entries[j+1].Kb - entries[j].Kb)
			return &calibrationItem{
				Kb:    kbMeasured,
				K0:    entries[j].K0 + r*(entries[j+1].K0-entries[j].K0),
				Kv:    entries[j].Kv + r*(entries[j+1].Kv-entries[j].Kv),
				Alpha: entries[j].Alpha + r*(entries[j+1].Alpha-entries[j].Alpha),
			}, kbExtrapolated
		}
	}

	return nil, false
}

// interpolateWithWarning 按当前 Ma 在 Kb 表上插值，返回插值项与外推标志。
// 多文件路径使用：从按 CMa 预排序的 calib 中线性扫描最近两个 Ma 档，
// 避免原实现的"每帧拷贝 + 排序"（B2）。单文件场景由 Calculate 走
// interpolateSingle 快速路径，不进入本函数。
func (t *ThreeHoleInterpolator) interpolateWithWarning(kbMeasured, ma float64) (*calibrationItem, bool) {
	kbExtrapolated := false

	if len(t.calib) == 0 {
		return nil, false
	}

	// 线性扫描取距离 ma 最近的两个 Ma 档（等价于原 sort.Slice 取前二，
	// 且交换 calib1/calib2 时 ratio→1-ratio 得到相同的插值混合，结果不变）。
	var calib1, calib2 *calibrationData
	d1, d2 := math.MaxFloat64, math.MaxFloat64
	for i := range t.calib {
		c := &t.calib[i]
		d := math.Abs(c.CMa - ma)
		if d < d1 {
			calib2, d2 = calib1, d1
			calib1, d1 = c, d
		} else if d < d2 {
			calib2, d2 = c, d
		}
	}
	if calib2 == nil {
		calib2 = calib1
	}

	var entries []kbAlphaEntry
	ratio := 0.0
	if math.Abs(calib2.CMa-calib1.CMa) > 1e-6 {
		ratio = (ma - calib1.CMa) / (calib2.CMa - calib1.CMa)
		if ratio < 0 || ratio > 1 {
			kbExtrapolated = true
		}
		ratio = math.Max(0, math.Min(1, ratio))
	}

	for i := 0; i < len(t.alphaSeq); i++ {
		kb := calib1.Items[i].Kb + ratio*(calib2.Items[i].Kb-calib1.Items[i].Kb)
		k0 := calib1.Items[i].K0 + ratio*(calib2.Items[i].K0-calib1.Items[i].K0)
		kv := calib1.Items[i].Kv + ratio*(calib2.Items[i].Kv-calib1.Items[i].Kv)
		entries = append(entries, kbAlphaEntry{
			Kb:    kb,
			Alpha: t.alphaSeq[i],
			K0:    k0,
			Kv:    kv,
		})
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Kb < entries[j].Kb
	})

	if kbMeasured <= entries[0].Kb {
		if kbMeasured < entries[0].Kb {
			kbExtrapolated = true
		}
		return &calibrationItem{
			Kb: entries[0].Kb, K0: entries[0].K0,
			Kv: entries[0].Kv, Alpha: entries[0].Alpha,
		}, kbExtrapolated
	}
	if kbMeasured >= entries[len(entries)-1].Kb {
		if kbMeasured > entries[len(entries)-1].Kb {
			kbExtrapolated = true
		}
		last := entries[len(entries)-1]
		return &calibrationItem{
			Kb: last.Kb, K0: last.K0,
			Kv: last.Kv, Alpha: last.Alpha,
		}, kbExtrapolated
	}

	for j := 0; j < len(entries)-1; j++ {
		if kbMeasured >= entries[j].Kb && kbMeasured <= entries[j+1].Kb {
			r := (kbMeasured - entries[j].Kb) / (entries[j+1].Kb - entries[j].Kb)
			return &calibrationItem{
				Kb:    kbMeasured,
				K0:    entries[j].K0 + r*(entries[j+1].K0-entries[j].K0),
				Kv:    entries[j].Kv + r*(entries[j+1].Kv-entries[j].Kv),
				Alpha: entries[j].Alpha + r*(entries[j+1].Alpha-entries[j].Alpha),
			}, kbExtrapolated
		}
	}

	return nil, false
}

// calcMach 由恢复的总/静压（表压）+ 大气参数计算马赫数。
//
// 改善项 A2：对齐七孔 calVelocityMach（atmosphere.go）的全部前置条件，任一不满足
// 返回错误（由 Calculate 转成 isValid=false + 警告），不再 `return 0`、不再
// `math.Abs` 掩盖非物理状态：
//  1. patm 有限且 > 0；
//  2. tatm+273.15 有限且 > 0（即 tatm > -273.15℃）；
//  3. pt >= ps；
//  4. ps+patm > 0；
//  5. ratio = (pt+patm)/(ps+patm) >= 1；
//  6. 最终 Ma 有限（maSq 非负且有限）。
func calcMach(pt, ps, patm, tatm float64) (float64, error) {
	if math.IsNaN(patm) || math.IsInf(patm, 0) || patm <= 0 {
		return 0, fmt.Errorf("大气压力非法: pa=%.6g", patm)
	}
	tempK := tatm + 273.15
	if math.IsNaN(tempK) || math.IsInf(tempK, 0) || tempK <= 0 {
		return 0, fmt.Errorf("大气温度非法: t=%.6g degC", tatm)
	}
	if pt < ps {
		return 0, fmt.Errorf("总压低于静压 (pt < ps): pt=%.6g, ps=%.6g", pt, ps)
	}
	psAbs := ps + patm
	if psAbs <= 0 {
		return 0, fmt.Errorf("绝对静压非正: ps+pa=%.6g (ps=%.6g, pa=%.6g)", psAbs, ps, patm)
	}
	ratio := (pt + patm) / psAbs
	if ratio < 1 {
		return 0, fmt.Errorf("压力比 %.6g < 1 (pt=%.6g, ps=%.6g, pa=%.6g)", ratio, pt, ps, patm)
	}

	gamma := calcGamma(tatm)
	if math.IsNaN(gamma) || math.IsInf(gamma, 0) || gamma <= 1 {
		return 0, fmt.Errorf("比热比非法: gamma=%.6g", gamma)
	}
	exp := (gamma - 1) / gamma
	coeff := 2 / (gamma - 1)

	powered := math.Pow(ratio, exp)
	maSq := coeff * (powered - 1)
	if math.IsNaN(maSq) || math.IsInf(maSq, 0) || maSq < 0 {
		return 0, fmt.Errorf("马赫数根号内非有限或为负: %.6e", maSq)
	}
	return math.Sqrt(maSq), nil
}

func calcGamma(tatm float64) float64 {
	// 空气比热比随温度近似变化 γ ≈ gammaRef - tempCoeff*(T-tempRef)
	// 在 -40°C ~ +60°C 范围内误差 <0.1%
	if math.IsNaN(tatm) || math.IsInf(tatm, 0) {
		return gammaRef
	}
	return gammaRef - tempCoeff*(tatm-tempRef)
}

func (t *ThreeHoleInterpolator) LoadPrbData(fileData []PrbFileData) (*LoadPrbResult, error) {
	t.loaded = false
	t.calib = nil
	t.alphaSeq = nil

	var calibList []calibrationData
	var firstNalpha *int
	var firstCal *calibrationData

	for _, fd := range fileData {
		cal, err := parsePrbLines(fd.Lines)
		if err != nil {
			return nil, fmt.Errorf("解析 %s 失败: %w", filepath.Base(fd.FilePath), err)
		}

		if firstNalpha == nil {
			firstNalpha = &cal.Nalpha
		} else if cal.Nalpha != *firstNalpha {
			return nil, fmt.Errorf("文件 %s 的Nalpha(%d)与其他文件(%d)不一致",
				filepath.Base(fd.FilePath), cal.Nalpha, *firstNalpha)
		}

		// A4：各档 Alpha 网格必须与首档完全一致（含顺序）。
		// 否则 interpolateWithWarning 按下标混合各 Ma 档时会静默错配角度。
		if firstCal == nil {
			firstCal = &cal
		} else {
			for i := range cal.Items {
				if math.Abs(cal.Items[i].Alpha-firstCal.Items[i].Alpha) > alphaGridEps {
					return nil, fmt.Errorf("文件 %s 的Alpha网格与其他文件不一致(第%d点: %g vs %g)",
						filepath.Base(fd.FilePath), i+1, cal.Items[i].Alpha, firstCal.Items[i].Alpha)
				}
			}
		}

		// A4：每档 Kb 必须按行序严格单调递增且无重复。
		// 否则区间插值 r 的分母可能为零（K0/Kv/Alpha 变 NaN/Inf）或角度错配。
		for i := 1; i < len(cal.Items); i++ {
			if cal.Items[i].Kb <= cal.Items[i-1].Kb {
				return nil, fmt.Errorf("文件 %s 的Kb非严格单调(第%d点 %g <= 第%d点 %g)",
					filepath.Base(fd.FilePath), i+1, cal.Items[i].Kb, i, cal.Items[i-1].Kb)
			}
		}

		// B2：预计算按 Kb 排序的插值表，单文件快速路径免去每帧排序/分配。
		cal.kbSorted = make([]calibrationItem, len(cal.Items))
		copy(cal.kbSorted, cal.Items)
		sort.Slice(cal.kbSorted, func(i, j int) bool {
			return cal.kbSorted[i].Kb < cal.kbSorted[j].Kb
		})

		calibList = append(calibList, cal)
	}

	if len(calibList) == 0 {
		return nil, fmt.Errorf("未加载任何有效校准数据")
	}

	var sumMa float64
	minMa := calibList[0].CMa
	maxMa := calibList[0].CMa

	for _, c := range calibList {
		sumMa += c.CMa
		if c.CMa < minMa {
			minMa = c.CMa
		}
		if c.CMa > maxMa {
			maxMa = c.CMa
		}
	}

	alphaSeq := make([]float64, len(calibList[0].Items))
	for i, item := range calibList[0].Items {
		alphaSeq[i] = item.Alpha
	}

	t.calib = calibList
	t.alphaSeq = alphaSeq
	t.initMa = sumMa / float64(len(calibList))
	t.minMa = minMa
	t.maxMa = maxMa
	t.loaded = true

	var machNumbers []float64
	files := make([]PrbFileInfo, 0, len(fileData))
	for i, fd := range fileData {
		machNumbers = append(machNumbers, calibList[i].CMa)
		files = append(files, PrbFileInfo{
			FilePath:   fd.FilePath,
			FileName:   filepath.Base(fd.FilePath),
			MachNumber: calibList[i].CMa,
			ValidRange: PrbValidRange{
				AlphaMin: alphaSeq[0],
				AlphaMax: alphaSeq[len(alphaSeq)-1],
				MachMin:  minMa,
				MachMax:  maxMa,
			},
		})
	}

	return &LoadPrbResult{
		Files:       files,
		MachNumbers: machNumbers,
	}, nil
}

func (t *ThreeHoleInterpolator) GetMachRange() (float64, float64) {
	return t.minMa, t.maxMa
}

func parsePrbLines(lines []string) (calibrationData, error) {
	nonEmpty := make([]string, 0, len(lines))
	for _, l := range lines {
		trimmed := strings.TrimSpace(l)
		if trimmed != "" {
			nonEmpty = append(nonEmpty, trimmed)
		}
	}

	if len(nonEmpty) < 2 {
		return calibrationData{}, fmt.Errorf("PRB文件至少需要2行")
	}

	cma, err := strconv.ParseFloat(nonEmpty[0], 64)
	if err != nil {
		return calibrationData{}, fmt.Errorf("第1行CMa格式错误: %w", err)
	}

	nalpha, err := strconv.Atoi(nonEmpty[1])
	if err != nil || nalpha <= 0 {
		return calibrationData{}, fmt.Errorf("第2行Nalpha应为正整数")
	}

	dataLines := nonEmpty[2:]
	if len(dataLines) < nalpha {
		return calibrationData{}, fmt.Errorf("数据行不足: 需要%d行，实际%d行", nalpha, len(dataLines))
	}

	cal := calibrationData{CMa: cma, Nalpha: nalpha}

	for i := 0; i < nalpha; i++ {
		parts := strings.Fields(dataLines[i])
		if len(parts) != 4 {
			return calibrationData{}, fmt.Errorf("第%d行需要4列数值(Kb K0 Kv Alpha)", i+3)
		}

		vals := make([]float64, 4)
		for j, p := range parts {
			v, err := strconv.ParseFloat(p, 64)
			if err != nil || math.IsNaN(v) || math.IsInf(v, 0) {
				return calibrationData{}, fmt.Errorf("第%d行第%d列数值无效", i+3, j+1)
			}
			vals[j] = v
		}

		cal.Items = append(cal.Items, calibrationItem{
			Kb:    vals[0],
			K0:    vals[1],
			Kv:    vals[2],
			Alpha: vals[3],
		})
	}

	return cal, nil
}
