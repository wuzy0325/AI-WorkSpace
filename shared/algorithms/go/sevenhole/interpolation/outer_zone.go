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
	pl := holePressure(in, n - 1)
	pr := holePressure(in, n + 1)
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
// 3.3): right edge (phi=center+30, theta ascending), outer edge (theta=45,
// phi descending), left edge (phi=center-30, theta descending), inner edge
// (theta=30, phi ascending), then deduplicated into a closed ring.
func buildOuterPolygon(sec *outerSector) []point2D {
	pts := make([]point2D, 0, 2*outerThetaCount+2*outerPhiCount)
	for it := 0; it < outerThetaCount; it++ { // right edge: phi=center+30
		pts = append(pts, point2D{sec.points[it][0].ka, sec.points[it][0].kb})
	}
	for ip := 0; ip < outerPhiCount; ip++ { // outer edge: theta=45
		pts = append(pts, point2D{sec.points[outerThetaCount-1][ip].ka, sec.points[outerThetaCount-1][ip].kb})
	}
	for it := outerThetaCount - 1; it >= 0; it-- { // left edge: phi=center-30
		pts = append(pts, point2D{sec.points[it][outerPhiCount-1].ka, sec.points[it][outerPhiCount-1].kb})
	}
	for ip := outerPhiCount - 1; ip >= 0; ip-- { // inner edge: theta=30
		pts = append(pts, point2D{sec.points[0][ip].ka, sec.points[0][ip].kb})
	}
	return dedupPolygon(pts)
}

// buildOuterQuads builds the 3x12 distorted quadrilateral cells of one outer
// sector (Python big_create_square, SKILL.md section 3.4). Cell (i,j) spans
// theta in [30+5i, 35+5i] and phi in [center+30-5(j+1), center+30-5j] with
// X1 at (thetaLo, phiHi); construction order (j outer, i inner) matches the
// Python list order so first-match edge behavior is identical. bStep=-5:
// phi decreases away from X1 (Python big_cal_ab).
func buildOuterQuads(sec *outerSector) []distortedQuad {
	quads := make([]distortedQuad, 0, (outerThetaCount-1)*(outerPhiCount-1))
	for j := 0; j < outerPhiCount-1; j++ {
		for i := 0; i < outerThetaCount-1; i++ {
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
func (p *SevenHolePrbInterpolator) outerZoneTrySector(sector int, ka, kb float64) (zoneCoefficients, bool, error) {
	sign := pointInPolygon(ka, kb, p.outerPolygons[sector-1])
	if sign < 0 {
		return zoneCoefficients{}, false, nil
	}
	a, b, found := locateInvertAB(ka, kb, p.outerQuads[sector-1])
	if !found {
		return zoneCoefficients{}, false, fmt.Errorf("大角度模式孔%d: (ka,kb)=(%.6g,%.6g) 在扇区多边形内但未定位到四边形", sector, ka, kb)
	}
	if math.IsNaN(a) || math.IsNaN(b) {
		return zoneCoefficients{}, false, fmt.Errorf("大角度模式孔%d: 四边形反演结果非有限 (ka=%.6g, kb=%.6g)", sector, ka, kb)
	}
	cpt, cps, err := outerBilinearCptCps(p.outer[sector-1], a, b)
	if err != nil {
		return zoneCoefficients{}, false, err
	}
	return zoneCoefficients{a: a, b: b, cpt: cpt, cps: cps}, true, nil
}

// outerThetaCellLo mirrors the a-axis cell branch of big_cptcps_square
// (SKILL.md section 3.5): k=int(a/5); cell [5k,5k+5], except k==9 (a=45)
// where the cell is [40,45].
func outerThetaCellLo(a float64) float64 {
	k := int(a / gridStep)
	if k != 9 {
		return gridStep * float64(k)
	}
	return 40
}

// outerPhiCell mirrors the b-axis cell branch of big_cptcps_square (SKILL.md
// section 3.5): l=int(b/5) (Python truncation toward zero); the l==71 wrap
// branch returns (355,0) verbatim (the -355 denominator quirk of the
// reference implementation is preserved); negative b from the sector-1 wrap
// square is truncated toward zero like Python.
func outerPhiCell(b float64) (lo, hi float64) {
	l := int(b / gridStep)
	if l == 71 {
		return 355, 0
	}
	return gridStep * float64(l), gridStep * float64(l + 1)
}

// outerCorner finds the grid point of sec with exact coordinates (aC,bC)
// (big_cptcps_square corner lookup by float equality, as in Python). A
// computed cell coordinate that does not exist in the file grid (e.g. b=-5
// for the sector-1 wrap) is reported as missing, matching the Python
// KeyError failure path.
func outerCorner(sec *outerSector, aC, bC float64) (gridPoint, bool) {
	for it := 0; it < outerThetaCount; it++ {
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
	aLo := outerThetaCellLo(a)
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
