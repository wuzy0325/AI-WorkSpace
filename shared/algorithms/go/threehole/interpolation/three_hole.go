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

	// 空气比热比温度修正系数（-40°C ~ +60°C 范围内误差 <0.1%）
	k0        = 1.4    // 20°C时的参考比热比
	tempRef   = 20.0   // 参考温度(°C)
	tempCoeff = 0.0002 // 温度修正系数 k ≈ k0 - tempCoeff*(T-tempRef)
)

type calibrationItem struct {
	Kb    float64
	Kt    float64
	Sb    float64
	Alpha float64
}

type calibrationData struct {
	CMa    float64
	Nalpha int
	Items  []calibrationItem
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
	Kt    float64
	Sb    float64
}

func NewThreeHoleInterpolator() *ThreeHoleInterpolator {
	return &ThreeHoleInterpolator{}
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

	currentMa := t.initMa
	iteration := 0
	maClamped := false

	for iteration = 0; iteration < maxIterations; iteration++ {
		match, _ := t.interpolateWithWarning(kbTemp, currentMa)
		if match == nil {
			break
		}

		pt := p2 + match.Kt*deltaP
		ps := pt - match.Sb*deltaP
		newMa := calcMach(pt, ps, pa, tatm)

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
		}, nil
	}

	pt := p2 + finalMatch.Kt*deltaP
	ps := pt - finalMatch.Sb*deltaP
	mach := calcMach(pt, ps, pa, tatm)

	var warnings []string
	isValid := true

	if mach > t.maxMa+0.01 || mach < t.minMa-0.01 {
		warnings = append(warnings, "计算马赫数超出校准范围")
		isValid = false
	}
	if maClamped {
		warnings = append(warnings, "迭代过程中马赫数被限制到标定边界，结果精度可能降低")
		if isValid {
			isValid = true // 不改变有效性，仅提示
		}
	}
	if kbExtrapolated {
		warnings = append(warnings, "Kb值超出校准数据范围，已使用最近边界点外推")
	}

	warning := strings.Join(warnings, "; ")

	return InterpolationResult{
		Alpha:          finalMatch.Alpha,
		MachNumber:     mach,
		TotalPressure:  pt,
		StaticPressure: ps,
		IterationCount: iteration + 1,
		IsValid:        isValid,
		Warning:        warning,
	}, nil
}

func (t *ThreeHoleInterpolator) interpolate(kbMeasured, ma float64) *calibrationItem {
	match, _ := t.interpolateWithWarning(kbMeasured, ma)
	return match
}

func (t *ThreeHoleInterpolator) interpolateWithWarning(kbMeasured, ma float64) (*calibrationItem, bool) {
	kbExtrapolated := false

	if len(t.calib) == 0 {
		return nil, false
	}

	sorted := make([]calibrationData, len(t.calib))
	copy(sorted, t.calib)
	sort.Slice(sorted, func(i, j int) bool {
		return math.Abs(sorted[i].CMa-ma) < math.Abs(sorted[j].CMa-ma)
	})

	calib1 := sorted[0]
	calib2 := sorted[0]
	if len(sorted) > 1 {
		calib2 = sorted[1]
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
		kt := calib1.Items[i].Kt + ratio*(calib2.Items[i].Kt-calib1.Items[i].Kt)
		sb := calib1.Items[i].Sb + ratio*(calib2.Items[i].Sb-calib1.Items[i].Sb)
		entries = append(entries, kbAlphaEntry{
			Kb:    kb,
			Alpha: t.alphaSeq[i],
			Kt:    kt,
			Sb:    sb,
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
			Kb: entries[0].Kb, Kt: entries[0].Kt,
			Sb: entries[0].Sb, Alpha: entries[0].Alpha,
		}, kbExtrapolated
	}
	if kbMeasured >= entries[len(entries)-1].Kb {
		if kbMeasured > entries[len(entries)-1].Kb {
			kbExtrapolated = true
		}
		last := entries[len(entries)-1]
		return &calibrationItem{
			Kb: last.Kb, Kt: last.Kt,
			Sb: last.Sb, Alpha: last.Alpha,
		}, kbExtrapolated
	}

	for j := 0; j < len(entries)-1; j++ {
		if kbMeasured >= entries[j].Kb && kbMeasured <= entries[j+1].Kb {
			r := (kbMeasured - entries[j].Kb) / (entries[j+1].Kb - entries[j].Kb)
			return &calibrationItem{
				Kb:    kbMeasured,
				Kt:    entries[j].Kt + r*(entries[j+1].Kt-entries[j].Kt),
				Sb:    entries[j].Sb + r*(entries[j+1].Sb-entries[j].Sb),
				Alpha: entries[j].Alpha + r*(entries[j+1].Alpha-entries[j].Alpha),
			}, kbExtrapolated
		}
	}

	return nil, false
}

func calcMach(pt, ps, pa, tatm float64) float64 {
	absPs := ps + pa
	if absPs < deltaPTol {
		return 0
	}
	absPt := pt + pa
	ratio := absPt / absPs

	gamma := calcGamma(tatm)
	exp := (gamma - 1) / gamma
	coeff := 2 / (gamma - 1)

	powered := math.Pow(ratio, exp)
	return math.Sqrt(coeff * math.Abs(powered-1))
}

func calcGamma(tatm float64) float64 {
	// 空气比热比随温度近似变化 k ≈ k0 - tempCoeff*(T-tempRef)
	// 在 -40°C ~ +60°C 范围内误差 <0.1%
	if math.IsNaN(tatm) || math.IsInf(tatm, 0) {
		return k0
	}
	return k0 - tempCoeff*(tatm-tempRef)
}

func (t *ThreeHoleInterpolator) LoadPrbData(fileData []PrbFileData) (*LoadPrbResult, error) {
	t.loaded = false
	t.calib = nil
	t.alphaSeq = nil

	var calibList []calibrationData
	var firstNalpha *int

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
			return calibrationData{}, fmt.Errorf("第%d行需要4列数值(Ka Kt Sb Alpha)", i+3)
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
			Kt:    vals[1],
			Sb:    vals[2],
			Alpha: vals[3],
		})
	}

	return cal, nil
}