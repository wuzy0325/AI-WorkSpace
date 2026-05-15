package interpolation

import (
	"fmt"
	"math"
	"strings"
	"testing"
)

func genPrbContent(kaScale, kbScale, cpt, cps float64) string {
	var b strings.Builder
	b.WriteString("13 13\n")
	for i := 0; i < GridSize; i++ {
		alpha := float64(AngleMin + i*AngleStep)
		for j := 0; j < GridSize; j++ {
			beta := float64(AngleMin + j*AngleStep)
			ka := alpha * kaScale
			kb := beta * kbScale
			b.WriteString(fmt.Sprintf("%.6f %.6f %.6f %.6f %.0f %.0f\n", ka, kb, cpt, cps, alpha, beta))
		}
	}
	return b.String()
}

func TestParsePrbFile_Valid(t *testing.T) {
	content := genPrbContent(0.02, 0.02, 0.5, 0.8)
	table, err := parsePrbFile(content, "test_0.5Ma.prb")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(table.Rows) != TotalPoints {
		t.Errorf("expected %d rows, got %d", TotalPoints, len(table.Rows))
	}
	if len(table.Index) != TotalPoints {
		t.Errorf("expected %d index entries, got %d", TotalPoints, len(table.Index))
	}
	if table.Mach != 0.5 {
		t.Errorf("expected mach 0.5, got %f", table.Mach)
	}
	// Verify a specific row
	key := fmt.Sprintf("%.0f,%.0f", 0.0, 0.0)
	row, ok := table.Index[key]
	if !ok {
		t.Fatal("missing index entry for (0,0)")
	}
	if row.Alpha != 0 || row.Beta != 0 {
		t.Errorf("expected (0,0), got (%.0f,%.0f)", row.Alpha, row.Beta)
	}
}

func TestParsePrbFile_TooShort(t *testing.T) {
	_, err := parsePrbFile("13 13\nonly one data line", "test.prb")
	if err == nil {
		t.Fatal("expected error for too-short content")
	}
}

func TestParsePrbFile_WrongHeader(t *testing.T) {
	_, err := parsePrbFile("10 10\n0 0 0 0 0 0", "test.prb")
	if err == nil {
		t.Fatal("expected error for wrong header")
	}
}

func TestParsePrbFile_ShortContent(t *testing.T) {
	_, err := parsePrbFile("13", "test.prb")
	if err == nil {
		t.Fatal("expected error for short content")
	}
}

func TestParsePrbFile_WrongRowCount(t *testing.T) {
	lines := []string{"13 13"}
	for i := 0; i < 100; i++ {
		lines = append(lines, "0 0 0 0 0 0")
	}
	_, err := parsePrbFile(strings.Join(lines, "\n"), "test.prb")
	if err == nil {
		t.Fatal("expected error for wrong row count")
	}
}

func TestCalculatePressureCoefficients(t *testing.T) {
	// Known inputs: P4-P5=0.2, P3-P1=0.1, delta=1.0
	input := TraversalInterpolationInput{
		P1: 101325.0, P2: 101325.975,
		P3: 101325.1, P4: 101325.0,
		P5: 101324.8, Patm: 101325, Tatm: 15,
	}
	ka, kb, delta := calculatePressureCoefficients(input)
	if math.Abs(ka-0.2) > 1e-9 {
		t.Errorf("expected ka=0.2, got %f", ka)
	}
	if math.Abs(kb-0.1) > 1e-9 {
		t.Errorf("expected kb=0.1, got %f", kb)
	}
	if math.Abs(delta-1.0) > 1e-9 {
		t.Errorf("expected delta=1.0, got %f", delta)
	}
}

func TestCalculatePressureCoefficients_ClampDelta(t *testing.T) {
	// All P values equal => delta near zero, should be clamped
	input := TraversalInterpolationInput{
		P1: 100, P2: 100, P3: 100, P4: 100, P5: 100,
	}
	_, _, delta := calculatePressureCoefficients(input)
	if math.Abs(delta) < 1e-10 {
		t.Error("expected delta to be clamped away from zero")
	}
}

func TestBilinear(t *testing.T) {
	tests := []struct {
		name                     string
		v00, v10, v01, v11, t, s float64
		want                     float64
	}{
		{"all equal", 5, 5, 5, 5, 0.5, 0.5, 5},
		{"t=0,s=0", 1, 2, 3, 4, 0, 0, 1},
		{"t=1,s=0", 1, 2, 3, 4, 1, 0, 2},
		{"t=0,s=1", 1, 2, 3, 4, 0, 1, 3},
		{"t=1,s=1", 1, 2, 3, 4, 1, 1, 4},
		{"center", 0, 10, 10, 20, 0.5, 0.5, 10},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := bilinear(tt.v00, tt.v10, tt.v01, tt.v11, tt.t, tt.s)
			if math.Abs(got-tt.want) > 1e-9 {
				t.Errorf("bilinear() = %f, want %f", got, tt.want)
			}
		})
	}
}

func TestClampInt(t *testing.T) {
	tests := []struct {
		v, min, max, want int
	}{
		{-5, 0, 10, 0},
		{15, 0, 10, 10},
		{5, 0, 10, 5},
		{0, 0, 10, 0},
		{10, 0, 10, 10},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprintf("clamp(%d,%d,%d)", tt.v, tt.min, tt.max), func(t *testing.T) {
			got := clampInt(tt.v, tt.min, tt.max)
			if got != tt.want {
				t.Errorf("clampInt(%d,%d,%d) = %d, want %d", tt.v, tt.min, tt.max, got, tt.want)
			}
		})
	}
}

func TestFindNearestMachIndex(t *testing.T) {
	machs := []float64{0.3, 0.5, 0.8}
	tests := []struct {
		target float64
		want   int
	}{
		{0.3, 0}, // exact
		{0.5, 1}, // exact
		{0.8, 2}, // exact
		{0.4, 1}, // between 0.3 and 0.5, FP bias to 0.5
		{0.6, 1}, // nearest to 0.5
		{0.2, 0}, // below range
		{1.0, 2}, // above range
	}
	for _, tt := range tests {
		t.Run(fmt.Sprintf("target=%.1f", tt.target), func(t *testing.T) {
			got := findNearestMachIndex(machs, tt.target)
			if got != tt.want {
				t.Errorf("findNearestMachIndex(%v, %f) = %d, want %d", machs, tt.target, got, tt.want)
			}
		})
	}
}

func TestFindMachInterval(t *testing.T) {
	machs := []float64{0.3, 0.5, 0.8}
	tests := []struct {
		target         float64
		wantLo, wantHi int
	}{
		{0.2, 0, 0}, // below range
		{1.0, 2, 2}, // above range
		{0.3, 0, 0}, // exact first
		{0.8, 2, 2}, // exact last
		{0.4, 0, 1}, // between 0.3 and 0.5
		{0.6, 1, 2}, // between 0.5 and 0.8
	}
	for _, tt := range tests {
		t.Run(fmt.Sprintf("target=%.1f", tt.target), func(t *testing.T) {
			lo, hi := findMachInterval(machs, tt.target)
			if lo != tt.wantLo || hi != tt.wantHi {
				t.Errorf("findMachInterval(%v, %f) = (%d,%d), want (%d,%d)",
					machs, tt.target, lo, hi, tt.wantLo, tt.wantHi)
			}
		})
	}
}

func TestLerp(t *testing.T) {
	tests := []struct {
		a, b, t, want float64
	}{
		{0, 10, 0, 0},
		{0, 10, 1, 10},
		{0, 10, 0.5, 5},
		{10, 0, 0.5, 5},
		{5, 5, 0.5, 5},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprintf("lerp(%.0f,%.0f,%.1f)", tt.a, tt.b, tt.t), func(t *testing.T) {
			got := lerp(tt.a, tt.b, tt.t)
			if math.Abs(got-tt.want) > 1e-9 {
				t.Errorf("lerp(%f,%f,%f) = %f, want %f", tt.a, tt.b, tt.t, got, tt.want)
			}
		})
	}
}

func TestIsPointInsideConvexQuad(t *testing.T) {
	// Square quad: (0,0), (1,0), (1,1), (0,1)
	tests := []struct {
		name   string
		px, py float64
		want   bool
	}{
		{"center inside", 0.5, 0.5, true},
		{"corner on vertex", 0, 0, true},
		{"on edge", 0.5, 0, true},
		{"far outside", 2, 2, false},
		{"near outside x", -0.5, 0.5, false},
		{"near outside y", 0.5, -0.5, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isPointInsideConvexQuad(tt.px, tt.py, 0, 0, 1, 0, 1, 1, 0, 1)
			if got != tt.want {
				t.Errorf("isPointInsideConvexQuad(%f,%f) = %v, want %v", tt.px, tt.py, got, tt.want)
			}
		})
	}
}

func TestSignDistance(t *testing.T) {
	// Line from (0,0) to (1,1)
	tests := []struct {
		name     string
		px, py   float64
		wantSign int // -1, 0, or 1
	}{
		{"on line", 0.5, 0.5, 0},
		// (0,1) relative to directed edge (0,0)->(1,1):
		// cross = (0-0)*(1-0)-(1-0)*(1-0) = -1 < 0
		{"below line", 1, 0, 1},
		// (1,0) relative to directed edge (0,0)->(1,1):
		// cross = (1-0)*(1-0)-(0-0)*(1-0) = 1 > 0
		{"above line", 0, 1, -1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := signDistance(tt.px, tt.py, 0, 0, 1, 1)
			s := 0
			if got > 1e-12 {
				s = 1
			} else if got < -1e-12 {
				s = -1
			}
			if s != tt.wantSign {
				t.Errorf("signDistance(%f,%f) = %f (sign=%d), want sign=%d", tt.px, tt.py, got, s, tt.wantSign)
			}
		})
	}
}

func TestPointToLineDistance(t *testing.T) {
	tests := []struct {
		name           string
		px, py         float64
		x1, y1, x2, y2 float64
		want           float64
	}{
		{"point on line", 0.5, 0.5, 0, 0, 1, 1, 0},
		{"vertical line", 0.5, 0.5, 0, 0, 0, 1, 0.5},
		{"horizontal line", 0.5, 0.5, 0, 0, 1, 0, 0.5},
		{"diagonal line", 1, 0, 0, 0, 1, 1, 1.0 / math.Sqrt2},
		{"degenerate line", 1, 0, 0, 0, 0, 0, 1}, // point-to-point
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := pointToLineDistance(tt.px, tt.py, tt.x1, tt.y1, tt.x2, tt.y2)
			if math.Abs(got-tt.want) > 1e-9 {
				t.Errorf("pointToLineDistance() = %f, want %f", got, tt.want)
			}
		})
	}
}

func TestParseMachFromFileName(t *testing.T) {
	tests := []struct {
		path string
		want float64
	}{
		{"data_0.5Ma.prb", 0.5},
		{"data_1.0Ma.prb", 1.0},
		{"data_02Ma.prb", 2.0},
		{"data_no_match.prb", 0},
		{"0.3Ma.prb", 0.3},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			got := parseMachFromFileName(tt.path)
			if math.Abs(got-tt.want) > 1e-9 {
				t.Errorf("parseMachFromFileName(%q) = %f, want %f", tt.path, got, tt.want)
			}
		})
	}
}

func TestPrbInterpolator_Calculate_NotLoaded(t *testing.T) {
	p := NewPrbInterpolator()
	_, err := p.Calculate(TraversalInterpolationInput{})
	if err == nil {
		t.Fatal("expected error when not loaded")
	}
}

func TestPrbInterpolator_IsLoaded(t *testing.T) {
	p := NewPrbInterpolator()
	if p.IsLoaded() {
		t.Error("expected not loaded initially")
	}
	content := genPrbContent(0.02, 0.02, 0.5, 0.8)
	err := p.LoadPrbFile(content, "test_0.5Ma.prb")
	if err != nil {
		t.Fatalf("LoadPrbFile failed: %v", err)
	}
	if !p.IsLoaded() {
		t.Error("expected loaded after LoadPrbFile")
	}
}

func TestPrbInterpolator_GetValidRange(t *testing.T) {
	p := NewPrbInterpolator()
	aMin, aMax, bMin, bMax := p.GetValidRange()
	if aMin != -30 || aMax != 30 || bMin != -30 || bMax != 30 {
		t.Errorf("GetValidRange() = (%f,%f,%f,%f), want (-30,30,-30,30)", aMin, aMax, bMin, bMax)
	}
}

func TestPrbInterpolator_Calculate_HappyPath(t *testing.T) {
	content := genPrbContent(0.02, 0.02, 0.5, 0.8)
	p := NewPrbInterpolator()
	if err := p.LoadPrbFile(content, "test_0.5Ma.prb"); err != nil {
		t.Fatalf("LoadPrbFile failed: %v", err)
	}

	input := TraversalInterpolationInput{
		P1: 101325.0, P2: 101325.975,
		P3: 101325.1, P4: 101325.0,
		P5: 101324.8, Patm: 101325, Tatm: 15,
	}

	result, err := p.Calculate(input)
	if err != nil {
		t.Fatalf("Calculate failed: %v", err)
	}
	if !result.IsValid {
		t.Fatalf("expected valid result, got warning: %s", result.Warning)
	}
	if math.Abs(result.Alpha-10) > 0.1 {
		t.Errorf("expected Alpha≈10, got %f", result.Alpha)
	}
	if math.Abs(result.Beta-5) > 0.1 {
		t.Errorf("expected Beta≈5, got %f", result.Beta)
	}
}

func TestMultiPrbInterpolator_Calculate_SingleFile(t *testing.T) {
	content := genPrbContent(0.02, 0.02, 0.5, 0.8)
	m := NewMultiPrbInterpolator(MultiPrbNearest)
	if err := m.AddPrbFile(content, "test_0.5Ma.prb"); err != nil {
		t.Fatalf("AddPrbFile failed: %v", err)
	}

	input := TraversalInterpolationInput{
		P1: 101325.0, P2: 101325.975,
		P3: 101325.1, P4: 101325.0,
		P5: 101324.8, Patm: 101325, Tatm: 15,
	}

	result, err := m.Calculate(input)
	if err != nil {
		t.Fatalf("Calculate failed: %v", err)
	}
	if !result.IsValid {
		t.Errorf("expected valid result")
	}
}

func TestMultiPrbInterpolator_AddPrbFile_SortsByMach(t *testing.T) {
	m := NewMultiPrbInterpolator(MultiPrbNearest)

	content := genPrbContent(0.02, 0.02, 0.5, 0.8)
	if err := m.AddPrbFile(content, "test_1.0Ma.prb"); err != nil {
		t.Fatalf("AddPrbFile 1.0Ma failed: %v", err)
	}
	if err := m.AddPrbFile(content, "test_0.5Ma.prb"); err != nil {
		t.Fatalf("AddPrbFile 0.5Ma failed: %v", err)
	}

	machs := m.GetMachNumbers()
	if len(machs) != 2 {
		t.Fatalf("expected 2 mach numbers, got %d", len(machs))
	}
	if machs[0] != 0.5 || machs[1] != 1.0 {
		t.Errorf("expected [0.5, 1.0], got %v", machs)
	}
}

func TestMultiPrbInterpolator_Calculate_NotLoaded(t *testing.T) {
	m := NewMultiPrbInterpolator(MultiPrbNearest)
	_, err := m.Calculate(TraversalInterpolationInput{})
	if err == nil {
		t.Fatal("expected error when not loaded")
	}
}
