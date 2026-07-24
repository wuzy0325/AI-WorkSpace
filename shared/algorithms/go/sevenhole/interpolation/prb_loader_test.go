package interpolation

import (
	"fmt"
	"strings"
	"testing"
)

// makeInnerLines builds a valid 13x13 inner-zone line set (169 data rows)
// with a column-name header. Coefficients are deterministic functions of the
// grid coordinates so loaded values can be verified exactly.
func makeInnerLines() []string {
	lines := []string{"ka kb cpt cps a b"} // column-name header must be skipped
	for ia := 0; ia < innerGridSide; ia++ {
		for ib := 0; ib < innerGridSide; ib++ {
			a := innerGridMin + gridStep*float64(ia)
			b := innerGridMin + gridStep*float64(ib)
			lines = append(lines, fmt.Sprintf("%.6f %.6f %.6f %.6f %.6f %.6f",
				0.01*a, 0.02*b, 1.5, -0.25, a, b))
		}
	}
	return lines
}

// makeOuterLines builds a valid 4x13 outer-sector line set (52 data rows)
// with a dimension header, in b-outer / a-inner row order.
func makeOuterLines(sector int) []string {
	center := float64(sector-1) * 60
	lines := []string{"4 13"} // dimension header must be skipped
	for k := 0; k < outerPhiCount; k++ { // b outer loop
		b := normalize360(center + 30 - gridStep*float64(k))
		for it := 0; it < outerThetaCount; it++ { // a inner loop
			theta := outerThetaMin + gridStep*float64(it)
			lines = append(lines, fmt.Sprintf("%.6f %.6f %.6f %.6f %.6f %.6f",
				0.1, 0.2, 1.1, -0.3, theta, b))
		}
	}
	return lines
}

func TestLoadInnerPrbLines_Valid(t *testing.T) {
	p := NewSevenHolePrbInterpolator()
	if err := p.LoadInnerPrbLines(makeInnerLines(), "7.prb"); err != nil {
		t.Fatalf("LoadInnerPrbLines: %v", err)
	}
	if p.inner == nil {
		t.Fatal("inner grid not stored")
	}
	// Corners and center must carry the coordinates and coefficients from input.
	checks := []struct{ ia, ib int }{{0, 0}, {12, 12}, {6, 6}, {0, 12}, {12, 0}}
	for _, c := range checks {
		gp := p.inner.points[c.ia][c.ib]
		wantA := innerGridMin + gridStep*float64(c.ia)
		wantB := innerGridMin + gridStep*float64(c.ib)
		if gp.a != wantA || gp.b != wantB {
			t.Errorf("points[%d][%d] coord = (%v,%v), want (%v,%v)", c.ia, c.ib, gp.a, gp.b, wantA, wantB)
		}
		if gp.ka != 0.01*wantA || gp.kb != 0.02*wantB || gp.cpt != 1.5 || gp.cps != -0.25 {
			t.Errorf("points[%d][%d] coeffs = (%v,%v,%v,%v)", c.ia, c.ib, gp.ka, gp.kb, gp.cpt, gp.cps)
		}
	}
}

func TestLoadInnerPrbLines_DimensionHeader(t *testing.T) {
	lines := makeInnerLines()
	lines[0] = "13 13" // dimension header must also be accepted (skipped)
	p := NewSevenHolePrbInterpolator()
	if err := p.LoadInnerPrbLines(lines, "7.prb"); err != nil {
		t.Fatalf("dimension header rejected: %v", err)
	}
}

func TestLoadInnerPrbLines_RowCount(t *testing.T) {
	for _, tc := range []struct {
		name string
		n    int // data rows to keep
	}{
		{"too few", 168},
		{"too many", 170},
	} {
		t.Run(tc.name, func(t *testing.T) {
			lines := makeInnerLines()
			if tc.n < len(lines)-1 {
				lines = lines[:tc.n+1]
			} else {
				lines = append(lines, lines[len(lines)-1])
			}
			err := NewSevenHolePrbInterpolator().LoadInnerPrbLines(lines, "inner-src")
			if err == nil {
				t.Fatal("expected row-count error")
			}
			if !strings.Contains(err.Error(), "inner-src") || !strings.Contains(err.Error(), "169") {
				t.Errorf("error must name source and expected count, got: %v", err)
			}
		})
	}
}

func TestLoadInnerPrbLines_Errors(t *testing.T) {
	corrupt := func(mutate func(lines []string)) []string {
		lines := makeInnerLines()
		mutate(lines)
		return lines
	}
	tests := []struct {
		name    string
		lines   []string
		wantSub []string
	}{
		{
			name:  "wrong column count",
			lines: corrupt(func(l []string) { l[1] = "1 2 3 4 5" }),
			// first data row is line 2 of the original set
			wantSub: []string{"col-src", "第2行", "6 列"},
		},
		{
			name:  "non-numeric field",
			lines: corrupt(func(l []string) { l[3] = "0.1 abc 1.5 -0.25 -30 -25" }),
			wantSub: []string{"col-src", "第4行", "第2列"},
		},
		{
			name:  "NaN field",
			lines: corrupt(func(l []string) { l[1] = "NaN 0 1.5 -0.25 -30 -30" }),
			wantSub: []string{"col-src", "第2行", "非有限"},
		},
		{
			name:  "Inf field",
			lines: corrupt(func(l []string) { l[1] = "0.1 +Inf 1.5 -0.25 -30 -30" }),
			wantSub: []string{"col-src", "第2行", "非有限"},
		},
		{
			name: "duplicate grid point",
			lines: corrupt(func(l []string) {
				l[len(l)-1] = l[1] // duplicate first row over last row
			}),
			wantSub: []string{"col-src", "重复网格点", "第170行"},
		},
		{
			name:  "a out of range",
			lines: corrupt(func(l []string) { l[1] = "0.1 0.2 1.5 -0.25 32 -30" }),
			wantSub: []string{"col-src", "第2行", "越界"},
		},
		{
			name:  "a off grid",
			lines: corrupt(func(l []string) { l[1] = "0.1 0.2 1.5 -0.25 27.5 -30" }),
			wantSub: []string{"col-src", "第2行", "非网格点"},
		},
		{
			name:  "b out of range",
			lines: corrupt(func(l []string) { l[1] = "0.1 0.2 1.5 -0.25 -30 -31" }),
			wantSub: []string{"col-src", "第2行", "越界"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := NewSevenHolePrbInterpolator().LoadInnerPrbLines(tc.lines, "col-src")
			if err == nil {
				t.Fatal("expected error")
			}
			for _, sub := range tc.wantSub {
				if !strings.Contains(err.Error(), sub) {
					t.Errorf("error %q missing %q", err.Error(), sub)
				}
			}
		})
	}
}

func TestLoadInnerPrbLines_Empty(t *testing.T) {
	err := NewSevenHolePrbInterpolator().LoadInnerPrbLines(nil, "empty-src")
	if err == nil || !strings.Contains(err.Error(), "empty-src") {
		t.Fatalf("expected source-named error, got: %v", err)
	}
}

func TestLoadOuterPrbLines_ValidAllSectors(t *testing.T) {
	for sector := 1; sector <= outerSectorCount; sector++ {
		t.Run(fmt.Sprintf("sector%d", sector), func(t *testing.T) {
			p := NewSevenHolePrbInterpolator()
			if err := p.LoadOuterPrbLines(sector, makeOuterLines(sector), fmt.Sprintf("%d.prb", sector)); err != nil {
				t.Fatalf("LoadOuterPrbLines: %v", err)
			}
			sec := p.outer[sector-1]
			if sec == nil {
				t.Fatal("outer sector not stored")
			}
			wantCenter := float64(sector-1) * 60
			if sec.centerPhi != wantCenter {
				t.Errorf("centerPhi = %v, want %v", sec.centerPhi, wantCenter)
			}
			// Verify theta/phi indexing and stored coefficients.
			for k := 0; k < outerPhiCount; k++ {
				for it := 0; it < outerThetaCount; it++ {
					gp := sec.points[it][k]
					wantTheta := outerThetaMin + gridStep*float64(it)
					wantPhi := normalize360(wantCenter + 30 - gridStep*float64(k))
					if gp.a != wantTheta || angularDiffDeg(gp.b, wantPhi) > gridEps {
						t.Errorf("points[%d][%d] coord = (%v,%v), want (%v,%v)", it, k, gp.a, gp.b, wantTheta, wantPhi)
					}
					if gp.ka != 0.1 || gp.kb != 0.2 || gp.cpt != 1.1 || gp.cps != -0.3 {
						t.Errorf("points[%d][%d] coeffs = (%v,%v,%v,%v)", it, k, gp.ka, gp.kb, gp.cpt, gp.cps)
					}
				}
			}
		})
	}
}

func TestLoadOuterPrbLines_Sector1WrapAround(t *testing.T) {
	// Sector 1 is centered at phi=0: its grid lines wrap across 0/360
	// (30,25,...,0,355,...,330). Values must be stored normalized.
	p := NewSevenHolePrbInterpolator()
	if err := p.LoadOuterPrbLines(1, makeOuterLines(1), "1.prb"); err != nil {
		t.Fatalf("LoadOuterPrbLines: %v", err)
	}
	sec := p.outer[0]
	if got := sec.points[0][0].b; got != 30 {
		t.Errorf("iPhi=0 b = %v, want 30", got)
	}
	if got := sec.points[0][6].b; got != 0 {
		t.Errorf("iPhi=6 b = %v, want 0", got)
	}
	if got := sec.points[0][7].b; got != 355 {
		t.Errorf("iPhi=7 b = %v, want 355 (normalized)", got)
	}
	if got := sec.points[0][12].b; got != 330 {
		t.Errorf("iPhi=12 b = %v, want 330 (normalized)", got)
	}
}

func TestLoadOuterPrbLines_Errors(t *testing.T) {
	tests := []struct {
		name    string
		sector  int
		mutate  func(l []string)
		wantSub []string
	}{
		{
			name:    "theta out of range",
			sector:  2,
			mutate:  func(l []string) { l[1] = "0.1 0.2 1.1 -0.3 50 90" },
			wantSub: []string{"out-src", "第2行", "越界"},
		},
		{
			name:    "phi not in sector grid",
			sector:  2,
			mutate:  func(l []string) { l[1] = "0.1 0.2 1.1 -0.3 30 47.5" },
			wantSub: []string{"out-src", "第2行", "非网格点"},
		},
		{
			name:    "duplicate grid point",
			sector:  3,
			mutate:  func(l []string) { l[len(l)-1] = l[1] },
			wantSub: []string{"out-src", "重复网格点", "第53行"},
		},
		{
			name:    "wrong column count",
			sector:  4,
			mutate:  func(l []string) { l[10] = "1 2 3" },
			wantSub: []string{"out-src", "第11行", "6 列"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			lines := makeOuterLines(tc.sector)
			tc.mutate(lines)
			err := NewSevenHolePrbInterpolator().LoadOuterPrbLines(tc.sector, lines, "out-src")
			if err == nil {
				t.Fatal("expected error")
			}
			for _, sub := range tc.wantSub {
				if !strings.Contains(err.Error(), sub) {
					t.Errorf("error %q missing %q", err.Error(), sub)
				}
			}
		})
	}
}

func TestLoadOuterPrbLines_RowCount(t *testing.T) {
	for _, n := range []int{51, 53} {
		lines := makeOuterLines(2)
		if n < len(lines)-1 {
			lines = lines[:n+1]
		} else {
			lines = append(lines, lines[len(lines)-1])
		}
		err := NewSevenHolePrbInterpolator().LoadOuterPrbLines(2, lines, "row-src")
		if err == nil {
			t.Fatalf("n=%d: expected row-count error", n)
		}
		if !strings.Contains(err.Error(), "row-src") || !strings.Contains(err.Error(), "52") {
			t.Errorf("n=%d: error must name source and expected count, got: %v", n, err)
		}
	}
}

func TestLoadOuterPrbLines_InvalidSector(t *testing.T) {
	for _, sector := range []int{0, 7, -1} {
		err := NewSevenHolePrbInterpolator().LoadOuterPrbLines(sector, makeOuterLines(1), "sec-src")
		if err == nil || !strings.Contains(err.Error(), "sec-src") {
			t.Errorf("sector=%d: expected source-named error, got: %v", sector, err)
		}
	}
}

// TestLoadOuterPrbLines_SectorGridEnforced loads one sector's lines under a
// different sector number: the phi grid lines are table-driven per sector
// (centers 0,60,...,300 deg), so the mismatch must be rejected. This is the
// loader-level guarantee that the six sector centers stay distinct and their
// union covers 360 deg (spec section 2.1).
func TestLoadOuterPrbLines_SectorGridEnforced(t *testing.T) {
	err := NewSevenHolePrbInterpolator().LoadOuterPrbLines(3, makeOuterLines(2), "swap-src")
	if err == nil {
		t.Fatal("sector-2 lines loaded as sector 3 must fail")
	}
	if !strings.Contains(err.Error(), "swap-src") {
		t.Errorf("error must name source, got: %v", err)
	}
}

func TestLoadOuterPrbLines_ColumnNameHeader(t *testing.T) {
	lines := makeOuterLines(5)
	lines[0] = "ka kb cpt cps a b" // column-name header must also be accepted
	if err := NewSevenHolePrbInterpolator().LoadOuterPrbLines(5, lines, "5.prb"); err != nil {
		t.Fatalf("column-name header rejected: %v", err)
	}
}
