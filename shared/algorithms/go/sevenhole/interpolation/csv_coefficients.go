package interpolation

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

func parseSevenHoleCSVFloat(record []string, column int) (float64, error) {
	value, err := strconv.ParseFloat(strings.TrimSpace(record[column]), 64)
	if err != nil {
		return 0, fmt.Errorf("第%d列不是有效数字 %q", column+1, record[column])
	}
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return 0, fmt.Errorf("第%d列为非有限数值 %q", column+1, record[column])
	}
	return value, nil
}

func parseSevenHoleCSVPressures(record []string) ([7]float64, float64, float64, error) {
	var pressures [7]float64
	for i := range pressures {
		value, err := parseSevenHoleCSVFloat(record, sevenHoleCSVP1Column+i)
		if err != nil {
			return pressures, 0, 0, err
		}
		pressures[i] = value
	}
	pt, err := parseSevenHoleCSVFloat(record, sevenHoleCSVPtColumn)
	if err != nil {
		return pressures, 0, 0, err
	}
	ps, err := parseSevenHoleCSVFloat(record, sevenHoleCSVPsColumn)
	if err != nil {
		return pressures, 0, 0, err
	}
	return pressures, pt, ps, nil
}

// recomputeSevenHoleInnerCoeffs derives full-precision coefficients from the
// pressure columns instead of the historically rounded coefficient columns.
// A rejected row cannot be skipped: every inner grid node is required to form
// the fixed 13x13 interpolation mesh.
func recomputeSevenHoleInnerCoeffs(record []string) (ka, kb, cpt, cps float64, err error) {
	p, pt, ps, err := parseSevenHoleCSVPressures(record)
	if err != nil {
		return 0, 0, 0, 0, err
	}
	pAvg := (p[0] + p[1] + p[2] + p[3] + p[4] + p[5]) / 6
	directionDenom := p[6] - pAvg
	if math.Abs(directionDenom) < 1e-12 {
		return 0, 0, 0, 0, fmt.Errorf("|p7-pAverage|=%.6e < 1e-12", directionDenom)
	}
	cpa := (p[3] - p[0]) / directionDenom
	cpb := (p[4] - p[1]) / directionDenom
	cpc := (p[5] - p[2]) / directionDenom
	ka = (cpb + cpc) / math.Sqrt(3)
	kb = -(2*cpa + cpb - cpc) / 3

	pressureDenom := pt - ps
	if math.Abs(pressureDenom) < 1e-12 {
		return 0, 0, 0, 0, fmt.Errorf("|pt-ps|=%.6e < 1e-12", pressureDenom)
	}
	cpt = (p[6] - pt) / pressureDenom
	cps = (ps - pAvg) / pressureDenom
	return ka, kb, cpt, cps, nil
}

// recomputeSevenHoleOuterCoeffs derives sector-n coefficients from the raw
// pressure columns, using wrap-around neighbors on the six-hole ring.
// A rejected row cannot be skipped because it would leave adjacent quads
// without a corner in the rectangular theta x phi mesh.
func recomputeSevenHoleOuterCoeffs(record []string, sector int) (ka, kb, cpt, cps float64, err error) {
	if sector < 1 || sector > outerSectorCount {
		return 0, 0, 0, 0, fmt.Errorf("扇区编号 %d 非法", sector)
	}
	p, pt, ps, err := parseSevenHoleCSVPressures(record)
	if err != nil {
		return 0, 0, 0, 0, err
	}
	pc := p[sector-1]
	pl := p[(sector-2+outerSectorCount)%outerSectorCount]
	pr := p[sector%outerSectorCount]
	directionDenom := pc - (pl+pr)/2
	if math.Abs(directionDenom) < 1e-12 {
		return 0, 0, 0, 0, fmt.Errorf("|pcenter-(pleft+pright)/2|=%.6e < 1e-12", directionDenom)
	}
	ka = (pc - p[6]) / directionDenom
	kb = (pl - pr) / directionDenom

	pressureDenom := pt - ps
	if math.Abs(pressureDenom) < 1e-12 {
		return 0, 0, 0, 0, fmt.Errorf("|pt-ps|=%.6e < 1e-12", pressureDenom)
	}
	cpt = (pc - pt) / pressureDenom
	cps = (ps - (pl+pr)/2) / pressureDenom
	return ka, kb, cpt, cps, nil
}
