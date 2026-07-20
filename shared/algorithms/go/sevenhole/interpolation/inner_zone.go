package interpolation

import (
	"fmt"
	"math"
)

// zoneCoefficients carries the products of one zone pipeline up to the
// cpt/cps stage: the inverted grid coordinates (a,b) and the interpolated
// total/static pressure coefficients.
type zoneCoefficients struct {
	a, b     float64 // inverted angle coordinates (deg)
	cpt, cps float64 // interpolated cpt/cps (dimensionless)
}

// innerKaKb computes the small-angle direction coefficients from the seven
// probe pressures (Python little_cal_kakb, SKILL.md section 2.1). The
// symmetrized pairs are (p4-p1), (p5-p2), (p6-p3); the 1e-12 denominator
// guard replaces Python's ZeroDivisionError (spec section 3.2).
func innerKaKb(in InterpolationInput) (ka, kb float64, err error) {
	pAvg := (in.P1 + in.P2 + in.P3 + in.P4 + in.P5 + in.P6) / 6
	denom := in.P7 - pAvg
	if math.Abs(denom) < 1e-12 {
		return 0, 0, fmt.Errorf("小角度模式: |p7-pAverage|=%.6e < 1e-12", denom)
	}
	cpa := (in.P4 - in.P1) / denom
	cpb := (in.P5 - in.P2) / denom
	cpc := (in.P6 - in.P3) / denom
	ka = (cpb + cpc) / math.Sqrt(3)
	kb = -(2*cpa + cpb - cpc) / 3
	return ka, kb, nil
}

// buildInnerGeometry precomputes the boundary polygon and the 144 distorted
// quadrilateral cells of the inner grid (spec section 3.4: grids are
// precomputed at load time; Calculate never rebuilds them).
func (p *SevenHolePrbInterpolator) buildInnerGeometry() {
	p.innerPolygon = buildInnerPolygon(p.inner)
	p.innerQuads = buildInnerQuads(p.inner)
}

// buildInnerPolygon constructs the boundary polygon of the inner grid from
// its outermost calibration points (Python little_create_line, SKILL.md
// section 4): bottom row (b=-30, a ascending), right column (a=30, b
// ascending), top row (b=30, a descending), left column (a=-30, b
// descending), then deduplicated into a closed ring.
func buildInnerPolygon(g *innerGrid) []point2D {
	pts := make([]point2D, 0, 4*innerGridSide)
	for ia := 0; ia < innerGridSide; ia++ { // bottom edge: b=-30
		pts = append(pts, point2D{g.points[ia][0].ka, g.points[ia][0].kb})
	}
	for ib := 0; ib < innerGridSide; ib++ { // right edge: a=30
		pts = append(pts, point2D{g.points[innerGridSide-1][ib].ka, g.points[innerGridSide-1][ib].kb})
	}
	for ia := innerGridSide - 1; ia >= 0; ia-- { // top edge reversed: b=30
		pts = append(pts, point2D{g.points[ia][innerGridSide-1].ka, g.points[ia][innerGridSide-1].kb})
	}
	for ib := innerGridSide - 1; ib >= 0; ib-- { // left edge reversed: a=-30
		pts = append(pts, point2D{g.points[0][ib].ka, g.points[0][ib].kb})
	}
	return dedupPolygon(pts)
}

// buildInnerQuads builds the 12x12 distorted quadrilateral cells of the
// inner grid (Python little_create_square, SKILL.md section 2.3). Cell (i,j)
// spans a in [5i-30, 5i-25] and b in [5j-30, 5j-25] with X1 at the
// (aLo,bLo) corner; construction order (j outer, i inner) matches the Python
// list order so first-match edge behavior is identical.
func buildInnerQuads(g *innerGrid) []distortedQuad {
	quads := make([]distortedQuad, 0, (innerGridSide-1)*(innerGridSide-1))
	for j := 0; j < innerGridSide-1; j++ {
		for i := 0; i < innerGridSide-1; i++ {
			quads = append(quads, newDistortedQuad(
				point2D{g.points[i][j].ka, g.points[i][j].kb},
				point2D{g.points[i+1][j].ka, g.points[i+1][j].kb},
				point2D{g.points[i+1][j+1].ka, g.points[i+1][j+1].kb},
				point2D{g.points[i][j+1].ka, g.points[i][j+1].kb},
				innerGridMin+gridStep*float64(i), innerGridMin+gridStep*float64(j), +gridStep,
			))
		}
	}
	return quads
}

// innerCellLo returns the lower grid-line coordinate (deg) of the cpt/cps
// interpolation cell containing v, mirroring little_cptcps_square (SKILL.md
// section 2.4) branch logic exactly, including Python int() truncation
// toward zero: v in [0,30) or v <= -30 picks the cell starting at 5*trunc(v/5);
// otherwise the cell ends at 5*trunc(v/5).
func innerCellLo(v float64) float64 {
	k := int(v / gridStep)
	if (0 <= v && v < 30) || v <= -30 {
		return gridStep * float64(k)
	}
	return gridStep * float64(k-1)
}

// innerBilinearCptCps interpolates cpt/cps at angle (a,b) on the regular
// inner angle grid (Python little_cptcps_square + little_cal_cptcps,
// SKILL.md section 2.4): slopes along a at the two b rows, then linear along
// b.
func (p *SevenHolePrbInterpolator) innerBilinearCptCps(a, b float64) (cpt, cps float64) {
	aLo := innerCellLo(a)
	bLo := innerCellLo(b)
	ia := int(math.Round((aLo - innerGridMin) / gridStep))
	ib := int(math.Round((bLo - innerGridMin) / gridStep))
	g := p.inner
	x1 := g.points[ia][ib]     // (aLo, bLo)
	x2 := g.points[ia+1][ib]   // (aLo+5, bLo)
	x3 := g.points[ia+1][ib+1] // (aLo+5, bLo+5)
	x4 := g.points[ia][ib+1]   // (aLo, bLo+5)
	cpt = bilinearXFirst(a, b, aLo, bLo, x1.cpt, x2.cpt, x3.cpt, x4.cpt)
	cps = bilinearXFirst(a, b, aLo, bLo, x1.cps, x2.cps, x3.cps, x4.cps)
	return cpt, cps
}

// bilinearXFirst implements little_cal_cptcps (SKILL.md section 2.4): slope
// along x at row yLo (X1->X2) and at row yLo+5 (X3->X4), then blend along y.
func bilinearXFirst(x, y, xLo, yLo, vX1, vX2, vX3, vX4 float64) float64 {
	k1 := (vX2 - vX1) / gridStep
	cp1 := vX1 + k1*(x-xLo)
	k3 := (vX3 - vX4) / gridStep
	cp2 := vX3 + k3*(x-(xLo+gridStep))
	return (cp2-cp1)*(y-yLo)/gridStep + cp1
}

// innerZoneInterpolate runs the small-angle pipeline through the cpt/cps
// stage (Python cal_ab inner branch, SKILL.md sections 2.3-2.4). inZone is
// false (without error) when (ka,kb) falls outside the boundary polygon; the
// caller then tries the large-angle pipeline (Python branch order: inner
// zone first, boundary sign 0 counts as inside).
func (p *SevenHolePrbInterpolator) innerZoneInterpolate(ka, kb float64) (zoneCoefficients, bool, error) {
	sign := pointInPolygon(ka, kb, p.innerPolygon)
	if sign < 0 {
		return zoneCoefficients{}, false, nil
	}
	a, b, found := locateInvertAB(ka, kb, p.innerQuads)
	if !found {
		return zoneCoefficients{}, false, fmt.Errorf("小角度模式: (ka,kb)=(%.6g,%.6g) 在边界多边形内但未定位到四边形", ka, kb)
	}
	if math.IsNaN(a) || math.IsNaN(b) {
		return zoneCoefficients{}, false, fmt.Errorf("小角度模式: 四边形反演结果非有限 (ka=%.6g, kb=%.6g)", ka, kb)
	}
	cpt, cps := p.innerBilinearCptCps(a, b)
	return zoneCoefficients{a: a, b: b, cpt: cpt, cps: cps}, true, nil
}
