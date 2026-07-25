package interpolation

import (
	"fmt"
	"math"
	"sort"
)

// ringPressures returns the six outer-ring pressures P1..P6 as an array
// (index 0 = hole 1).
func ringPressures(in InterpolationInput) [6]float64 {
	return [6]float64{in.P1, in.P2, in.P3, in.P4, in.P5, in.P6}
}

// holePressure returns the pressure of outer-ring hole n (1..6) with
// wrap-around neighbors (n=1: left=p6; n=6: right=p1), Python
// big_cal_kakb / big_cal_ptps neighbor convention (SKILL.md section 3.1).
func holePressure(in InterpolationInput, n int) float64 {
	p := ringPressures(in)
	return p[(n-1+6)%6]
}

// maxPressureHoles finds the largest (first) and second-largest (second)
// pressure holes among P1..P6 (Python big_max_pressure, SKILL.md section
// 3.2). First occurrence wins on exact ties; first==second is possible when
// the two largest sorted values are equal (Python semantics preserved).
func maxPressureHoles(in InterpolationInput) (first, second int) {
	p := ringPressures(in)
	sorted := p
	sort.Float64s(sorted[:])
	maxV, secondV := sorted[5], sorted[4]
	for i, v := range p {
		if v == maxV {
			first = i + 1
			break
		}
	}
	for i, v := range p {
		if v == secondV {
			second = i + 1
			break
		}
	}
	return first, second
}

// outerKaKb computes the large-angle sector coefficients for candidate
// sector n (Python big_cal_kakb, SKILL.md section 3.1):
// ka=(pc-p7)/denom, kb=(pl-pr)/denom, denom=pc-(pl+pr)/2 with wrap-around
// neighbors. The 1e-12 guard replaces Python's ZeroDivisionError (spec
// section 3.2).
func outerKaKb(in InterpolationInput, n int) (ka, kb float64, err error) {
	pc := holePressure(in, n)
	pl := holePressure(in, n-1)
	pr := holePressure(in, n+1)
	denom := pc - (pl+pr)/2
	if math.Abs(denom) < 1e-12 {
		return 0, 0, fmt.Errorf("大角度模式孔%d: |pcenter-(pleft+pright)/2|=%.6e < 1e-12", n, denom)
	}
	ka = (pc - in.P7) / denom
	kb = (pl - pr) / denom
	return ka, kb, nil
}

// buildOuterGeometry precomputes the boundary polygon and the 36 distorted
// quadrilateral cells of one outer sector (spec section 3.4: grids are
// precomputed at load time; Calculate never rebuilds them).
func (p *SevenHolePrbInterpolator) buildOuterGeometry(sector int) {
	sec := p.outer[sector-1]
	p.outerPolygons[sector-1] = buildOuterPolygon(sec)
	p.outerQuads[sector-1] = buildOuterQuads(sec)
}

// buildOuterPolygon constructs the boundary polygon of one outer sector from
// its boundary calibration points (Python big_create_line, SKILL.md section
// 3.3): right edge (phi=center+30, theta ascending), outer edge (theta=thetaMax,
// phi descending), left edge (phi=center-30, theta descending), inner edge
// (theta=30, phi ascending), then deduplicated into a closed ring.
//
// thetaMax 动态：由 sec.thetaCount 决定（如 thetaCount=4 → thetaMax=45，
// thetaCount=7 → thetaMax=60），不再硬编码 45。
func buildOuterPolygon(sec *outerSector) []point2D {
	thetaCount := sec.thetaCount
	pts := make([]point2D, 0, 2*thetaCount+2*outerPhiCount)
	for it := 0; it < thetaCount; it++ { // right edge: phi=center+30
		pts = append(pts, point2D{sec.points[it][0].ka, sec.points[it][0].kb})
	}
	for ip := 0; ip < outerPhiCount; ip++ { // outer edge: theta=thetaMax
		pts = append(pts, point2D{sec.points[thetaCount-1][ip].ka, sec.points[thetaCount-1][ip].kb})
	}
	for it := thetaCount - 1; it >= 0; it-- { // left edge: phi=center-30
		pts = append(pts, point2D{sec.points[it][outerPhiCount-1].ka, sec.points[it][outerPhiCount-1].kb})
	}
	for ip := outerPhiCount - 1; ip >= 0; ip-- { // inner edge: theta=30
		pts = append(pts, point2D{sec.points[0][ip].ka, sec.points[0][ip].kb})
	}
	return dedupPolygon(pts)
}

// buildOuterQuads builds the (thetaCount-1)×(outerPhiCount-1) distorted
// quadrilateral cells of one outer sector (Python big_create_square,
// SKILL.md section 3.4). Cell (i,j) spans theta in [30+5i, 35+5i] and phi
// in [center+30-5(j+1), center+30-5j] with X1 at (thetaLo, phiHi);
// construction order (j outer, i inner) matches the Python list order so
// first-match edge behavior is identical. bStep=-5: phi decreases away
// from X1 (Python big_cal_ab).
//
// thetaCount 动态：cell 数量随 sec.thetaCount 变化（4→3×12=36，7→6×12=72）。
func buildOuterQuads(sec *outerSector) []distortedQuad {
	thetaCount := sec.thetaCount
	quads := make([]distortedQuad, 0, (thetaCount-1)*(outerPhiCount-1))
	for j := 0; j < outerPhiCount-1; j++ {
		for i := 0; i < thetaCount-1; i++ {
			quads = append(quads, newDistortedQuad(
				point2D{sec.points[i][j].ka, sec.points[i][j].kb},
				point2D{sec.points[i+1][j].ka, sec.points[i+1][j].kb},
				point2D{sec.points[i+1][j+1].ka, sec.points[i+1][j+1].kb},
				point2D{sec.points[i][j+1].ka, sec.points[i][j+1].kb},
				outerThetaMin+gridStep*float64(i), normalize360(sec.centerPhi+30-gridStep*float64(j)), -gridStep,
			))
		}
	}
	return quads
}

// outerZoneTrySector runs the large-angle pipeline for one candidate sector
// through the cpt/cps stage (Python cal_ab big-angle branch): boundary
// polygon test, (theta,phi) inversion via distorted quadrilaterals, then
// cpt/cps interpolation. hit=false (without error) means (ka,kb) lies
// outside the sector polygon; the caller then tries the second candidate
// sector (Python first/second logic). No extrapolation is performed
// (beyond_border is intentionally not implemented, spec section 4).
//
// 网格点匹配兜底（两处）：当 (ka,kb) 正好是网格点（自提取 PRB 反推场景），
// Python 原版的 outerPhiCell/outerCorner 链路会因以下两类边界条件失败：
//   - locateInvertAB 对四边形角点的闭区间判定不包含 → found=false
//   - locateInvertAB 成功但反演的 b 落在扇区右边界（b=centerPhi+30）或
//     0/360 跨界 cell（b∈[-5,0]）→ outerBilinearCptCps 报"角点缺失"
//     （outerPhiCell 对 b=center+30 返回 (b, b+5)，b+5 不在网格中）
//
// 此时直接匹配网格点，返回其 (a, b, cpt, cps)，跳过双线性插值。
func (p *SevenHolePrbInterpolator) outerZoneTrySector(sector int, ka, kb float64) (zoneCoefficients, bool, error) {
	sign := pointInPolygon(ka, kb, p.outerPolygons[sector-1])
	if sign < 0 {
		return zoneCoefficients{}, false, nil
	}
	a, b, found := locateInvertAB(ka, kb, p.outerQuads[sector-1])
	if !found {
		// 兜底 1：locateInvertAB 未找到 quad 时，尝试网格点直命中。
		if gp, ok := outerFindGridPointByKaKb(p.outer[sector-1], ka, kb); ok {
			return zoneCoefficients{a: gp.a, b: gp.b, cpt: gp.cpt, cps: gp.cps}, true, nil
		}
		return zoneCoefficients{}, false, fmt.Errorf("大角度模式孔%d: (ka,kb)=(%.6g,%.6g) 在扇区多边形内但未定位到四边形", sector, ka, kb)
	}
	if math.IsNaN(a) || math.IsNaN(b) {
		return zoneCoefficients{}, false, fmt.Errorf("大角度模式孔%d: 四边形反演结果非有限 (ka=%.6g, kb=%.6g)", sector, ka, kb)
	}
	cpt, cps, err := outerBilinearCptCps(p.outer[sector-1], a, b)
	if err != nil {
		// 兜底 2：locateInvertAB 成功但 outerBilinearCptCps 角点缺失
		// （扇区右边界 b=center+30 或 0/360 跨界 cell），尝试网格点直命中。
		if gp, ok := outerFindGridPointByKaKb(p.outer[sector-1], ka, kb); ok {
			return zoneCoefficients{a: gp.a, b: gp.b, cpt: gp.cpt, cps: gp.cps}, true, nil
		}
		return zoneCoefficients{}, false, err
	}
	return zoneCoefficients{a: a, b: b, cpt: cpt, cps: cps}, true, nil
}

// outerFindGridPointByKaKb 在扇区网格中查找 (ka,kb) 近似匹配的网格点。
//
// 容差选择 1e-6（远大于 gridEps=1e-9）：兜底路径用于自提取 PRB 反推场景，
// (ka,kb) 来自同一份数据反算应 bit-for-bit 相等，1e-9 即可；但若 ka/kb
// 来自外部 CSV 校准（浮点 16 位有效数字序列化，如 0.571 vs 0.5710000009999999），
// 1e-9 会漏匹配。1e-6 同时覆盖两类来源且不会误跨网格（gridStep=5°，相邻
// 网格点 ka/kb 差远大于 1e-6）。
func outerFindGridPointByKaKb(sec *outerSector, ka, kb float64) (gridPoint, bool) {
	const findGridPointEps = 1e-6
	if sec == nil {
		return gridPoint{}, false
	}
	for it := 0; it < sec.thetaCount; it++ {
		for ip := 0; ip < outerPhiCount; ip++ {
			gp := &sec.points[it][ip]
			if math.Abs(gp.ka-ka) < findGridPointEps && math.Abs(gp.kb-kb) < findGridPointEps {
				return *gp, true
			}
		}
	}
	return gridPoint{}, false
}

// outerThetaCellLo mirrors the a-axis cell branch of big_cptcps_square
// (SKILL.md section 3.5): k=int(a/5); cell [5k,5k+5], except a==thetaMax
// where the cell shrinks to [thetaMax-5,thetaMax].
//
// thetaCount 动态：原 Python 实现硬编码 k==9（thetaMax=45 时 45/5=9）。
// 动态化后通过 thetaMax = outerThetaMin + gridStep*(thetaCount-1) 推导
// kMax = thetaMax/gridStep = 6 + (thetaCount-1)，使任意 thetaMax
// （45/50/60/...）的最外层 cell 都能正确收缩。
//
// kMax 表达式拆分：int(outerThetaMin/gridStep) 显式计算 outerThetaMin/gridStep=30/5=6，
// 加上 thetaCount-1 即最外层 theta 的索引；避免隐式除法让读者反推魔法数字。
func outerThetaCellLo(a float64, thetaCount int) float64 {
	k := int(a / gridStep)
	kMax := int(outerThetaMin/gridStep) + thetaCount - 1 // 6 + (thetaCount-1)
	if k != kMax {
		return gridStep * float64(k)
	}
	return gridStep * float64(kMax-1)
}

// outerPhiCell mirrors the b-axis cell branch of big_cptcps_square (SKILL.md
// section 3.5): l=int(b/5) (Python truncation toward zero); the l==71 wrap
// branch returns (355,0) verbatim (the -355 denominator quirk of the
// reference implementation is preserved); negative b from the sector-1 wrap
// square is truncated toward zero like Python.
//
// 注：l==71 wrap 与 outerPhiCount=13 物理设计绑定（13 条 phi 网格线跨越
// 0/360 边界），不随 thetaCount 变化，故保持硬编码。
func outerPhiCell(b float64) (lo, hi float64) {
	l := int(b / gridStep)
	if l == 71 {
		return 355, 0
	}
	return gridStep * float64(l), gridStep * float64(l+1)
}

// outerCorner finds the grid point of sec with exact coordinates (aC,bC)
// (big_cptcps_square corner lookup by float equality, as in Python). A
// computed cell coordinate that does not exist in the file grid (e.g. b=-5
// for the sector-1 wrap) is reported as missing, matching the Python
// KeyError failure path.
func outerCorner(sec *outerSector, aC, bC float64) (gridPoint, bool) {
	for it := 0; it < sec.thetaCount; it++ {
		for ip := 0; ip < outerPhiCount; ip++ {
			gp := &sec.points[it][ip]
			if gp.a == aC && gp.b == bC {
				return *gp, true
			}
		}
	}
	return gridPoint{}, false
}

// outerBilinearCptCps interpolates cpt/cps at grid coordinates (a=theta,
// b=phi) on the regular outer angle grid of one sector (Python
// big_cptcps_square + big_cal_cptcps, SKILL.md section 3.5): slopes along b
// at the two theta columns, then linear along theta — the direction opposite
// to the inner zone.
func outerBilinearCptCps(sec *outerSector, a, b float64) (cpt, cps float64, err error) {
	aLo := outerThetaCellLo(a, sec.thetaCount)
	aHi := aLo + gridStep
	bLo, bHi := outerPhiCell(b)
	x1, ok1 := outerCorner(sec, aLo, bLo) // (aLo, bLo)
	x2, ok2 := outerCorner(sec, aLo, bHi) // (aLo, bHi)
	x3, ok3 := outerCorner(sec, aHi, bHi) // (aHi, bHi)
	x4, ok4 := outerCorner(sec, aHi, bLo) // (aHi, bLo)
	if !ok1 || !ok2 || !ok3 || !ok4 {
		return 0, 0, fmt.Errorf("大角度模式扇区%d: cpt/cps 单元格角点缺失 (a=%.6g, b=%.6g, 单元 a=[%.6g,%.6g] b=[%.6g,%.6g])",
			sec.sector, a, b, aLo, aHi, bLo, bHi)
	}
	cpt = bilinearBFirst(a, b, aLo, bLo, bHi, x1.cpt, x2.cpt, x3.cpt, x4.cpt)
	cps = bilinearBFirst(a, b, aLo, bLo, bHi, x1.cps, x2.cps, x3.cps, x4.cps)
	return cpt, cps, nil
}

// bilinearBFirst implements big_cal_cptcps (SKILL.md section 3.5): slope
// along b at column aLo (X1->X2) and at column aHi (X3->X4), then blend
// along a. Corner order: X1=(aLo,bLo), X2=(aLo,bHi), X3=(aHi,bHi),
// X4=(aHi,bLo).
func bilinearBFirst(a, b, aLo, bLo, bHi, vX1, vX2, vX3, vX4 float64) float64 {
	k1 := (vX2 - vX1) / (bHi - bLo)
	cp1 := vX1 + k1*(b-bLo)
	k3 := (vX3 - vX4) / (bHi - bLo)
	cp2 := vX3 + k3*(b-bHi)
	return (cp2-cp1)*(a-aLo)/gridStep + cp1
}

// convertThetaPhiToAlphaBeta maps outer-zone grid coordinates (theta,phi) to
// output angles (alpha,beta) (Python big_ab_convert, SKILL.md section 3.7):
//
//	beta  =  deg(atan(tan(rad(theta)) * cos(rad(phi))))
//	alpha = -deg(atan(tan(rad(theta)) * sin(rad(phi))))
//
// The minus sign on alpha is load-bearing (phi grows clockwise seen from the
// probe tail, spec section 3.3); removing it flips the alpha sign and is
// caught by tests. Degenerate branch |theta| >= 89.5 deg (unreachable for
// the standard theta<=45 grid, kept for parity): alpha=-theta, beta=phi.
func convertThetaPhiToAlphaBeta(theta, phi float64) (alpha, beta float64) {
	if math.Abs(theta) >= 89.5 {
		return -theta, phi
	}
	tTheta := math.Tan(theta * math.Pi / 180)
	radPhi := phi * math.Pi / 180
	alpha = -math.Atan(tTheta*math.Sin(radPhi)) * 180 / math.Pi
	beta = math.Atan(tTheta*math.Cos(radPhi)) * 180 / math.Pi
	return alpha, beta
}
