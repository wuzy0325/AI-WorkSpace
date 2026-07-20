package interpolation

import "math"

// point2D is a vertex in the (ka,kb) coefficient plane.
type point2D struct {
	x, y float64
}

// dedupPolygon removes duplicate vertices while preserving first-occurrence
// order, mirroring the edge_tuple dedup in Python point_in_polygon
// (SKILL.md section 4). The four boundary chains share corner vertices, so
// the closed ring has 4*(N-1) unique vertices for an N-point chain.
func dedupPolygon(pts []point2D) []point2D {
	out := make([]point2D, 0, len(pts))
	for _, p := range pts {
		dup := false
		for _, q := range out {
			if p == q {
				dup = true
				break
			}
		}
		if !dup {
			out = append(out, p)
		}
	}
	return out
}

// pointInPolygon classifies point (x,y) against a polygon via ray casting
// (Python point_in_polygon, seven_hole.py; SKILL.md section 4). Returns 0
// when the point lies on a boundary segment (treated as inside by the
// caller), 1 when inside, -1 when outside. The iteration sequence
// (including the degenerate first segment and the closing segment) replicates
// the Python loop exactly so signs match bit for bit.
func pointInPolygon(x, y float64, polygon []point2D) int {
	n := len(polygon)
	if n == 0 {
		return -1
	}
	inside := false
	p1 := polygon[0]
	for i := 0; i <= n; i++ {
		p2 := polygon[i%n]
		if y == p1.y && y == p2.y {
			if math.Min(p1.x, p2.x) <= x && x <= math.Max(p1.x, p2.x) {
				return 0 // on boundary
			}
		}
		if math.Min(p1.y, p2.y) < y && y <= math.Max(p1.y, p2.y) {
			if x <= math.Max(p1.x, p2.x) {
				if p1.y != p2.y {
					xInters := (y-p1.y)*(p2.x-p1.x)/(p2.y-p1.y) + p1.x
					if p1.x == p2.x || x <= xInters {
						inside = !inside
					}
				}
			}
		}
		p1 = p2
	}
	if inside {
		return 1
	}
	return -1
}

// quadEdge is one precomputed edge line y = k*x + b of a distorted
// quadrilateral cell in the (ka,kb) plane.
type quadEdge struct {
	k, b float64
}

// distortedQuad is one calibration grid cell mapped into the (ka,kb)
// coefficient plane (Python little_create_square / big_create_square,
// SKILL.md sections 2.3 / 3.4). Edges 1..4 are X1X2, X2X3, X3X4, X4X1 with
// X1 the grid corner (a1,b1); bStep is +5 for the inner zone (X3/X4 above
// X1 in b) and -5 for the outer zone (below).
type distortedQuad struct {
	e     [4]quadEdge // edge line coefficients
	a1    float64     // grid coordinate a of corner X1 (deg)
	b1    float64     // grid coordinate b of corner X1 (deg)
	bStep float64     // +5 inner zone / -5 outer zone
}

// newDistortedQuad precomputes a cell from its four (ka,kb) corners in
// X1..X4 order plus the grid coordinates of X1.
func newDistortedQuad(x1, x2, x3, x4 point2D, a1, b1, bStep float64) distortedQuad {
	return distortedQuad{
		e: [4]quadEdge{
			lineThrough(x1, x2),
			lineThrough(x2, x3),
			lineThrough(x3, x4),
			lineThrough(x4, x1),
		},
		a1: a1, b1: b1, bStep: bStep,
	}
}

// lineThrough returns the line y = k*x + b through two points. For a
// vertical edge k becomes +/-Inf; Python raises ZeroDivisionError there and
// aborts the whole cell search, while Go's Inf/NaN arithmetic simply fails
// the containment comparisons and skips the cell. No real calibration grid
// produces vertical edges, so the behaviors do not diverge in practice
// (Python little_cal_ab / big_cal_ab, SKILL.md sections 2.3 / 3.4).
func lineThrough(p, q point2D) quadEdge {
	k := (q.y - p.y) / (q.x - p.x)
	return quadEdge{k: k, b: p.y - k*p.x}
}

// locateInvertAB finds the distorted quadrilateral containing (ka,kb) and
// inverts to grid coordinates (a,b) via edge-equation containment and
// inverse-distance weighting (Python little_cal_ab / big_cal_ab, SKILL.md
// sections 2.3 / 3.4). Cells are scanned in construction order; found=false
// means no cell contains the point.
func locateInvertAB(ka, kb float64, quads []distortedQuad) (a, b float64, found bool) {
	for i := range quads {
		q := &quads[i]
		y1 := q.e[0].k*ka + q.e[0].b
		y3 := q.e[2].k*ka + q.e[2].b
		x2 := (kb - q.e[1].b) / q.e[1].k
		x4 := (kb - q.e[3].b) / q.e[3].k
		if y1 <= kb && kb <= y3 && x2 >= ka && ka >= x4 {
			d1 := math.Abs((-q.e[0].k*ka + kb - q.e[0].b) / math.Sqrt(q.e[0].k*q.e[0].k+1))
			d2 := math.Abs((-q.e[1].k*ka + kb - q.e[1].b) / math.Sqrt(q.e[1].k*q.e[1].k+1))
			d3 := math.Abs((-q.e[2].k*ka + kb - q.e[2].b) / math.Sqrt(q.e[2].k*q.e[2].k+1))
			d4 := math.Abs((-q.e[3].k*ka + kb - q.e[3].b) / math.Sqrt(q.e[3].k*q.e[3].k+1))
			b = q.b1 + q.bStep*d1/(d1+d3)
			a = q.a1 + 5*d4/(d2+d4)
			return a, b, true
		}
	}
	return 0, 0, false
}
