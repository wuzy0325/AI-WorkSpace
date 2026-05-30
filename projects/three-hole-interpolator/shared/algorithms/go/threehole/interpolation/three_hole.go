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
	maxIterations   = 20
	convergeTol     = 1e-4
	deltaPTol       = 1e-6
	k               = 1.4
	isentropicPower = (k - 1) / k
	machCoeff       = 2 / (k - 1)
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
	loaded     bool
	calib      []calibrationData
	alphaSeq   []float64
	initMa     float64
	minMa      float64
	maxMa      float64
	kbSorted   bool
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
		return InterpolationResult{}, fmt.Errorf("calibration data not loaded")
	}

	p1, p2, p3 := input.P1, input.P2, input.P3
	pa := input.PAtm

	if pa < 0 {
		return InterpolationResult{}, fmt.Errorf("atmospheric pressure must be non-negative")
	}

	deltaP := 2*p2 - p1 - p3
	if math.Abs(deltaP) < deltaPTol {
		return InterpolationResult{
			TotalPressure:  p2,
			StaticPressure: p2,
			IsValid:        false,
			Warning:        "pressure difference near zero, cannot calculate",
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
			Warning:        "Kb is Inf or NaN",
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
		newMa := calcMach(pt, ps, pa)

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
			Warning:        "final interpolation failed",
		}, nil
	}

	pt := p2 + finalMatch.Kt*deltaP
	ps := pt - finalMatch.Sb*deltaP
	mach := calcMach(pt, ps, pa)

	var warnings []string
	isValid := true

	if mach > t.maxMa+0.01 || mach < t.minMa-0.01 {
		warnings = append(warnings, "Mach number out of calibration range")
	}
	if maClamped {
		warnings = append(warnings, "Mach clamped to calibration boundary, accuracy may be reduced")
	}
	if kbExtrapolated {
		warnings = append(warnings, "Kb outside calibration data range, using nearest boundary point")
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

	var entries []calibrationItem
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
		entries = append(entries, calibrationItem{
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

func calcMach(pt, ps, pa float64) float64 {
	absPs := ps + pa
	absPt := pt + pa

	if absPs <= 0 {
		return 0
	}
	if absPt <= absPs {
		return 0
	}

	ratio := absPt / absPs
	powered := math.Pow(ratio, isentropicPower)
	return math.Sqrt(machCoeff * (powered - 1))
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
			return nil, fmt.Errorf("parse %s failed: %w", filepath.Base(fd.FilePath), err)
		}

		if firstNalpha == nil {
			firstNalpha = &cal.Nalpha
		} else if cal.Nalpha != *firstNalpha {
			return nil, fmt.Errorf("Nalpha mismatch in %s: %d vs %d",
				filepath.Base(fd.FilePath), cal.Nalpha, *firstNalpha)
		}

		calibList = append(calibList, cal)
	}

	if len(calibList) == 0 {
		return nil, fmt.Errorf("no valid calibration data loaded")
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

	for i := 1; i < len(alphaSeq); i++ {
		if alphaSeq[i] <= alphaSeq[i-1] {
			return nil, fmt.Errorf("Alpha values must be in strictly ascending order")
		}
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
		return calibrationData{}, fmt.Errorf("PRB file needs at least 2 lines")
	}

	cma, err := strconv.ParseFloat(nonEmpty[0], 64)
	if err != nil {
		return calibrationData{}, fmt.Errorf("line 1 CMa parse error: %w", err)
	}

	nalpha, err := strconv.Atoi(nonEmpty[1])
	if err != nil || nalpha <= 0 {
		return calibrationData{}, fmt.Errorf("line 2 Nalpha should be a positive integer")
	}

	dataLines := nonEmpty[2:]
	if len(dataLines) < nalpha {
		return calibrationData{}, fmt.Errorf("insufficient data lines: need %d, got %d", nalpha, len(dataLines))
	}

	cal := calibrationData{CMa: cma, Nalpha: nalpha}

	for i := 0; i < nalpha; i++ {
		parts := strings.Fields(dataLines[i])
		if len(parts) != 4 {
			return calibrationData{}, fmt.Errorf("line %d needs 4 columns (Kb Kt Sb Alpha)", i+3)
		}

		vals := make([]float64, 4)
		for j, p := range parts {
			v, err := strconv.ParseFloat(p, 64)
			if err != nil || math.IsNaN(v) || math.IsInf(v, 0) {
				return calibrationData{}, fmt.Errorf("line %d col %d invalid value", i+3, j+1)
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