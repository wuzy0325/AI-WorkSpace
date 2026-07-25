package interpolation

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// Grid geometry constants (spec-seven-hole-traversal section 2.1).
//
// 物理设计硬约束（不可动态化）：
//   - innerGridSide=13：内区 a,b ∈ [-30,30] 步长 5，共 13×13=169 点
//   - outerPhiCount=13：扇区 phi 网格 center±30 步长 5（13 条线，含 0/360 跨界）
//   - outerSectorCount=6：六孔探针外围 6 孔按 60° 等分
//   - gridStep=5、outerThetaMin=30：步长与外区 theta 起点
//
// 可动态化的维度：
//   - 外区 theta 维度（outerSector.thetaCount）：不同校准数据集可能覆盖
//     30..45（4 点）、30..60（7 点）等不同范围。loader 按实际数据行数推断
//     thetaCount = 数据行数 / outerPhiCount，校验整除且 ≥2（至少两点才能
//     形成插值单元格）。算法核心（polygon/quad/cptCps）全部基于 thetaCount
//     动态计算，不再硬编码 4 点。
const (
	innerGridSide    = 13                            // a,b in [-30,30] deg, step 5
	innerPointCount  = innerGridSide * innerGridSide // 169 data rows of 7.prb
	outerPhiCount    = 13                            // sector phi lines: center +/-30 step 5
	outerSectorCount = 6

	innerGridMin  = -30.0
	gridStep      = 5.0
	outerThetaMin = 30.0

	// gridEps is the tolerance (deg) for on-grid coordinate matching,
	// including the sector-center consistency enforced via the phi grid sets.
	gridEps = 1e-9

	// 导出物理设计常量别名：供跨包引用（如 adapters 层 CSV 起点与步长校验、
	// 跨包网格点匹配容差）。包内代码继续使用小写形式，无需改动；包外通过
	// seveninterp.OuterThetaMin / GridStep / GridEps 引用，确保物理常量单点定义。
	OuterThetaMin = outerThetaMin
	GridStep      = gridStep
	GridEps       = gridEps
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

// outerSector is one large-angle thetaCount×13 calibration grid (n.prb for hole n).
//
// thetaCount 是 theta 维度的实际网格点数（动态：≥2，步长 5，从 outerThetaMin
// 起）。points 第一维按 theta 升序索引（0=30°，1=35°，...），第二维按
// phi 网格序索引（center+30 → center-30，步长 -5，含 0/360 跨界归一化）。
type outerSector struct {
	sector     int     // hole number 1..6
	centerPhi  float64 // sector center phi in deg: (sector-1)*60 (spec section 2.1)
	thetaCount int     // theta 维度实际网格点数（动态，≥2）
	// points is indexed [iTheta][iPhi] with theta=30+5*iTheta and
	// phi=normalize360(centerPhi+30-5*iPhi).
	points [][]gridPoint
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
	rows, err := parsePrbDataLines(lines, source)
	if err != nil {
		return err
	}
	// 内区固定 13×13=169 点（物理设计：a,b ∈ [-30,30] 步长 5）。
	if len(rows) != innerPointCount {
		return fmt.Errorf("%s: 内区数据行数必须为 %d（13×13），实际 %d 行", source, innerPointCount, len(rows))
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
//
// 动态 theta 维度：数据行数 = thetaCount × outerPhiCount（phi 固定 13 条线，
// 物理设计）。loader 按实际数据行数推断 thetaCount，校验整除且 ≥2
// （至少两个 theta 点才能形成 cpt/cps 插值单元格）。不再强制 52 行。
func (p *SevenHolePrbInterpolator) LoadOuterPrbLines(sector int, lines []string, source string) error {
	if sector < 1 || sector > outerSectorCount {
		return fmt.Errorf("%s: 扇区编号 %d 非法，必须在 1..%d 之间", source, sector, outerSectorCount)
	}
	rows, err := parsePrbDataLines(lines, source)
	if err != nil {
		return err
	}
	// 动态推断 thetaCount：数据行数必须能被 outerPhiCount（13）整除，且
	// thetaCount ≥ 2（至少两个 theta 点才能形成 quad 单元格）。
	if len(rows) < outerPhiCount*2 || len(rows)%outerPhiCount != 0 {
		return fmt.Errorf("%s: 数据行数 %d 必须是 %d 的整数倍且 ≥%d（thetaCount×phiCount，phi 固定 %d 条）",
			source, len(rows), outerPhiCount, outerPhiCount*2, outerPhiCount)
	}
	thetaCount := len(rows) / outerPhiCount
	sec := &outerSector{
		sector:     sector,
		centerPhi:  float64(sector-1) * 60,
		thetaCount: thetaCount,
		points:     make([][]gridPoint, thetaCount),
	}
	filled := make([][]bool, thetaCount)
	for it := 0; it < thetaCount; it++ {
		sec.points[it] = make([]gridPoint, outerPhiCount)
		filled[it] = make([]bool, outerPhiCount)
	}
	for i, r := range rows {
		lineNo := i + 2 // header occupies line 1
		it, ok := outerThetaIndex(r.a, thetaCount)
		if !ok {
			return fmt.Errorf("%s 第%d行: 外区网格点 theta=%.6g 越界或非网格点 (期望 %g..%g 步长5)",
				source, lineNo, r.a, outerThetaMin, outerThetaMin+gridStep*float64(thetaCount-1))
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
	for it := 0; it < thetaCount; it++ {
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

// parsePrbDataLines skips the header line and parses all data lines of the
// form "ka kb cpt cps a b" (6 whitespace-separated finite numbers). 行数校验
// 由调用方按各自维度约束（内区 169，外区动态 thetaCount×13）执行——本函数
// 只保证"至少 1 行数据 + 6 列 + 有限数值"。Any failure returns an error
// naming source and the 1-based line number of the original line set.
func parsePrbDataLines(lines []string, source string) ([]gridPoint, error) {
	if len(lines) == 0 {
		return nil, fmt.Errorf("%s: 文件为空，缺少表头行", source)
	}
	data := lines[1:] // header skipped without parsing (spec section 2.1)
	if len(data) == 0 {
		return nil, fmt.Errorf("%s: 缺少数据行（仅表头）", source)
	}
	rows := make([]gridPoint, 0, len(data))
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
// thetaCount 是该扇区实际的 theta 网格点数（动态），theta 必须落在
// [outerThetaMin, outerThetaMin+5*(thetaCount-1)] 且为网格点。
func outerThetaIndex(theta float64, thetaCount int) (int, bool) {
	idx := int(math.Round((theta - outerThetaMin) / gridStep))
	if idx < 0 || idx >= thetaCount {
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
