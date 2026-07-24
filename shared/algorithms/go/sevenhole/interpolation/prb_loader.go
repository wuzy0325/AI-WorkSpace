package interpolation

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// Grid geometry constants (spec-seven-hole-traversal section 2.1).
const (
	innerGridSide   = 13                        // a,b in [-30,30] deg, step 5
	innerPointCount = innerGridSide * innerGridSide // 169 data rows of 7.prb
	outerThetaCount = 4                         // theta in {30,35,40,45} deg
	outerPhiCount   = 13                        // sector phi lines: center +/-30 step 5
	outerPointCount = outerThetaCount * outerPhiCount // 52 data rows of n.prb
	outerSectorCount = 6

	innerGridMin  = -30.0
	gridStep      = 5.0
	outerThetaMin = 30.0

	// gridEps is the tolerance (deg) for on-grid coordinate matching,
	// including the sector-center consistency enforced via the phi grid sets.
	gridEps = 1e-9
)

// gridPoint is one calibration node of a .prb grid (SKILL.md section 1.2).
type gridPoint struct {
	ka  float64
	kb  float64
	cpt float64
	cps float64
	a   float64 // inner zone: angle a (deg); outer zone: theta (deg)
	b   float64 // inner zone: angle b (deg); outer zone: phi normalized to [0,360) (deg)
}

// innerGrid is the small-angle 13x13 calibration grid (7.prb).
type innerGrid struct {
	// points is indexed [ia][ib] with a=-30+5*ia, b=-30+5*ib (deg).
	points [innerGridSide][innerGridSide]gridPoint
}

// outerSector is one large-angle 4x13 calibration grid (n.prb for hole n).
type outerSector struct {
	sector    int     // hole number 1..6
	centerPhi float64 // sector center phi in deg: (sector-1)*60 (spec section 2.1)
	// points is indexed [iTheta][iPhi] with theta=30+5*iTheta and
	// phi=normalize360(centerPhi+30-5*iPhi).
	points [outerThetaCount][outerPhiCount]gridPoint
}

// NewSevenHolePrbInterpolator constructs an empty interpolator. The 7-file
// .prb set is loaded via LoadInnerPrbLines plus one LoadOuterPrbLines call
// per sector; the package performs no file I/O.
func NewSevenHolePrbInterpolator() *SevenHolePrbInterpolator {
	return &SevenHolePrbInterpolator{}
}

// LoadInnerPrbLines parses and strictly validates the small-angle grid
// (7.prb) from pre-read text lines. source is an opaque label (usually the
// file path) embedded into error messages; it is never used for I/O.
//
// Header contract (spec section 2.1): the first line must exist and is
// skipped without parsing, accepting both dimension headers ("13 13") and
// column-name headers ("ka kb cpt cps a b"), mirroring Python next(file).
func (p *SevenHolePrbInterpolator) LoadInnerPrbLines(lines []string, source string) error {
	rows, err := parsePrbDataLines(lines, source, innerPointCount)
	if err != nil {
		return err
	}
	grid := &innerGrid{}
	var filled [innerGridSide][innerGridSide]bool
	for i, r := range rows {
		lineNo := i + 2 // header occupies line 1
		ia, ok := innerCoordIndex(r.a)
		if !ok {
			return fmt.Errorf("%s 第%d行: 内区网格点 a=%.6g 越界或非网格点 (期望 -30..30 步长5)", source, lineNo, r.a)
		}
		ib, ok := innerCoordIndex(r.b)
		if !ok {
			return fmt.Errorf("%s 第%d行: 内区网格点 b=%.6g 越界或非网格点 (期望 -30..30 步长5)", source, lineNo, r.b)
		}
		if filled[ia][ib] {
			return fmt.Errorf("%s 第%d行: 重复网格点 (a=%.6g, b=%.6g)", source, lineNo, r.a, r.b)
		}
		filled[ia][ib] = true
		grid.points[ia][ib] = r
	}
	// Exact row count + no duplicates + on-grid already imply completeness;
	// the sweep below is a defensive check and reports the first missing node.
	for ia := 0; ia < innerGridSide; ia++ {
		for ib := 0; ib < innerGridSide; ib++ {
			if !filled[ia][ib] {
				return fmt.Errorf("%s: 内区网格缺失点 (a=%.6g, b=%.6g)", source,
					innerGridMin+gridStep*float64(ia), innerGridMin+gridStep*float64(ib))
			}
		}
	}
	p.inner = grid
	p.innerSource = source
	p.buildInnerGeometry()
	return nil
}

// LoadOuterPrbLines parses and strictly validates one large-angle sector
// grid (n.prb for hole n; sector must be in 1..6). The phi grid lines are
// table-driven per sector (spec section 2.1: sector centers 0,60,...,300 deg,
// pairwise distinct, union covering 360 deg); a row whose phi does not match
// the expected set of the given sector is rejected, which also enforces
// center-angle consistency across the six files. Row order convention is
// b-outer / a-inner, but points are indexed by coordinate so file order is
// not significant.
func (p *SevenHolePrbInterpolator) LoadOuterPrbLines(sector int, lines []string, source string) error {
	if sector < 1 || sector > outerSectorCount {
		return fmt.Errorf("%s: 扇区编号 %d 非法，必须在 1..%d 之间", source, sector, outerSectorCount)
	}
	rows, err := parsePrbDataLines(lines, source, outerPointCount)
	if err != nil {
		return err
	}
	sec := &outerSector{sector: sector, centerPhi: float64(sector-1) * 60}
	var filled [outerThetaCount][outerPhiCount]bool
	for i, r := range rows {
		lineNo := i + 2 // header occupies line 1
		it, ok := outerThetaIndex(r.a)
		if !ok {
			return fmt.Errorf("%s 第%d行: 外区网格点 theta=%.6g 越界或非网格点 (期望 30..45 步长5)", source, lineNo, r.a)
		}
		ip, ok := outerPhiIndex(sector, r.b)
		if !ok {
			return fmt.Errorf("%s 第%d行: 外区扇区%d 网格点 phi=%.6g 越界或非网格点", source, lineNo, sector, r.b)
		}
		if filled[it][ip] {
			return fmt.Errorf("%s 第%d行: 重复网格点 (theta=%.6g, phi=%.6g)", source, lineNo, r.a, r.b)
		}
		filled[it][ip] = true
		r.b = normalize360(r.b) // store phi normalized for later geometry
		sec.points[it][ip] = r
	}
	// Defensive completeness sweep (implied by count + uniqueness + on-grid).
	for it := 0; it < outerThetaCount; it++ {
		for ip := 0; ip < outerPhiCount; ip++ {
			if !filled[it][ip] {
				return fmt.Errorf("%s: 外区扇区%d 网格缺失点 (theta=%.6g, phi=%.6g)", source, sector,
					outerThetaMin+gridStep*float64(it), normalize360(sec.centerPhi+30-gridStep*float64(ip)))
			}
		}
	}
	p.outer[sector-1] = sec
	p.outerSources[sector-1] = source
	p.buildOuterGeometry(sector)
	return nil
}

// parsePrbDataLines skips the header line and parses exactly wantRows data
// lines of the form "ka kb cpt cps a b" (6 whitespace-separated finite
// numbers). Any failure returns an error naming source and the 1-based line
// number of the original line set.
func parsePrbDataLines(lines []string, source string, wantRows int) ([]gridPoint, error) {
	if len(lines) == 0 {
		return nil, fmt.Errorf("%s: 文件为空，缺少表头行", source)
	}
	data := lines[1:] // header skipped without parsing (spec section 2.1)
	if len(data) != wantRows {
		return nil, fmt.Errorf("%s: 必须包含 %d 行数据，实际 %d 行", source, wantRows, len(data))
	}
	rows := make([]gridPoint, 0, wantRows)
	for i, line := range data {
		lineNo := i + 2
		fields := strings.Fields(line)
		if len(fields) != 6 {
			return nil, fmt.Errorf("%s 第%d行: 必须包含 6 列 (ka kb cpt cps a b)，实际 %d 列", source, lineNo, len(fields))
		}
		var v [6]float64
		for j, f := range fields {
			x, err := strconv.ParseFloat(f, 64)
			if err != nil {
				return nil, fmt.Errorf("%s 第%d行第%d列: 不是有效数字 %q", source, lineNo, j+1, f)
			}
			if math.IsNaN(x) || math.IsInf(x, 0) {
				return nil, fmt.Errorf("%s 第%d行第%d列: 非有限数值 %q", source, lineNo, j+1, f)
			}
			v[j] = x
		}
		rows = append(rows, gridPoint{ka: v[0], kb: v[1], cpt: v[2], cps: v[3], a: v[4], b: v[5]})
	}
	return rows, nil
}

// innerCoordIndex maps an inner-zone coordinate (deg) to its grid index.
func innerCoordIndex(v float64) (int, bool) {
	idx := int(math.Round((v - innerGridMin) / gridStep))
	if idx < 0 || idx >= innerGridSide {
		return 0, false
	}
	if math.Abs(v-(innerGridMin+gridStep*float64(idx))) > gridEps {
		return 0, false
	}
	return idx, true
}

// outerThetaIndex maps an outer-zone theta (deg) to its grid index.
func outerThetaIndex(theta float64) (int, bool) {
	idx := int(math.Round((theta - outerThetaMin) / gridStep))
	if idx < 0 || idx >= outerThetaCount {
		return 0, false
	}
	if math.Abs(theta-(outerThetaMin+gridStep*float64(idx))) > gridEps {
		return 0, false
	}
	return idx, true
}

// outerPhiIndex matches phi (deg) against the sector's 13 grid lines
// (center+30 down to center-30, step 5, normalized to [0,360)).
func outerPhiIndex(sector int, phi float64) (int, bool) {
	center := float64(sector-1) * 60
	for k := 0; k < outerPhiCount; k++ {
		line := normalize360(center + 30 - gridStep*float64(k))
		if angularDiffDeg(phi, line) <= gridEps {
			return k, true
		}
	}
	return 0, false
}

// normalize360 maps an angle in degrees into [0, 360).
func normalize360(deg float64) float64 {
	d := math.Mod(deg, 360)
	if d < 0 {
		d += 360
	}
	return d
}

// angularDiffDeg returns the smallest absolute angular difference in [0,180].
func angularDiffDeg(a, b float64) float64 {
	d := math.Abs(normalize360(a) - normalize360(b))
	if d > 180 {
		d = 360 - d
	}
	return d
}
