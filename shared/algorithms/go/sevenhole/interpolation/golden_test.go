package interpolation

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// goldenEntry mirrors one record of testdata/golden/*.json. Calibration-node
// entries are generated from their defining grid nodes; constructed non-node
// entries are produced by the authoritative Python seven_hole.cal_ab. Regenerate
// the fixtures (and review the diff) whenever the .prb format, the algorithm,
// the seven_hole.py API, or the source dataset changes.
type goldenEntry struct {
	Index    int    `json:"index"`
	Mode     string `json:"mode"` // "little" | "big" | "out" (out = Python beyond_border path)
	Sector   int    `json:"sector"`
	Fallback bool   `json:"fallback"`
	Why      string `json:"why"`
	Input    struct {
		P1 float64 `json:"p1"`
		P2 float64 `json:"p2"`
		P3 float64 `json:"p3"`
		P4 float64 `json:"p4"`
		P5 float64 `json:"p5"`
		P6 float64 `json:"p6"`
		P7 float64 `json:"p7"`
		Pa float64 `json:"pa"`
		T  float64 `json:"t"`
	} `json:"input"`
	Output struct {
		Alpha float64 `json:"alpha"`
		Beta  float64 `json:"beta"`
		Pt    float64 `json:"pt"`
		Ps    float64 `json:"ps"`
		Ma    float64 `json:"ma"`
		V     float64 `json:"v"`
	} `json:"output"`
}

// Golden tolerances (spec section 7.1): angles absolute, pressures
// max(rel,abs), Ma/V relative.
const (
	goldenAngleTol = 1e-4
	goldenRelTol   = 1e-6
	goldenPressAbs = 1e-4
	goldenPressRel = 1e-6
)

func readGoldenLines(t *testing.T, name string) []string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", "prb", name))
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	var lines []string
	for _, line := range strings.Split(string(data), "\n") {
		if line = strings.TrimSpace(line); line != "" {
			lines = append(lines, line)
		}
	}
	return lines
}

// loadGoldenInterpolator loads the 7-file .prb fixture set through the real
// line-based loaders (no I/O in the algorithm package itself).
func loadGoldenInterpolator(t *testing.T) *SevenHolePrbInterpolator {
	t.Helper()
	p := NewSevenHolePrbInterpolator()
	if err := p.LoadInnerPrbLines(readGoldenLines(t, "7.prb"), "testdata/prb/7.prb"); err != nil {
		t.Fatalf("load inner: %v", err)
	}
	for sector := 1; sector <= outerSectorCount; sector++ {
		name := fmt.Sprintf("%d.prb", sector)
		if err := p.LoadOuterPrbLines(sector, readGoldenLines(t, name), "testdata/prb/"+name); err != nil {
			t.Fatalf("load sector %d: %v", sector, err)
		}
	}
	if !p.IsLoaded() {
		t.Fatal("interpolator not loaded after 7-file fixture set")
	}
	return p
}

func loadGoldenEntries(t *testing.T, file string) []goldenEntry {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", "golden", file))
	if err != nil {
		t.Fatalf("read golden %s: %v", file, err)
	}
	var entries []goldenEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		t.Fatalf("parse golden %s: %v", file, err)
	}
	return entries
}

func pressureOK(got, want float64) bool {
	return math.Abs(got-want) <= math.Max(math.Abs(want)*goldenPressRel, goldenPressAbs)
}

func relOK(got, want float64) bool {
	if want == 0 {
		return got == 0
	}
	return math.Abs(got-want)/math.Abs(want) <= goldenRelTol
}

// checkGoldenEntry compares one golden point against the Go implementation.
// mode "little"/"big" (inside the legal grid): numeric parity within
// tolerance. mode "out": Python takes its beyond_border extrapolation there;
// Go deliberately returns IsValid=false without extrapolating (intentional
// difference, spec section 4 / 7.2) and the assertion covers that contract.
func checkGoldenEntry(t *testing.T, p *SevenHolePrbInterpolator, e goldenEntry) {
	t.Helper()
	in := InterpolationInput{
		P1: e.Input.P1, P2: e.Input.P2, P3: e.Input.P3, P4: e.Input.P4,
		P5: e.Input.P5, P6: e.Input.P6, P7: e.Input.P7,
		PAtm: e.Input.Pa, TAtm: e.Input.T,
	}
	res, err := p.Calculate(in)

	if e.Mode == "out" {
		if err != nil {
			t.Errorf("out-of-grid must return invalid (not error), got: %v", err)
			return
		}
		if res.IsValid {
			t.Errorf("out-of-grid must be IsValid=false (no extrapolation, spec section 4)")
		}
		if !strings.Contains(res.Warning, "外推") {
			t.Errorf("warning %q must state that extrapolation is unsupported", res.Warning)
		}
		return
	}

	if err != nil {
		ka, kb, kaErr := innerKaKb(in)
		t.Errorf("Calculate error: %v\n  intermediates: inner ka=%.6g kb=%.6g (err=%v)", err, ka, kb, kaErr)
		return
	}
	if !res.IsValid {
		ka, kb, _ := innerKaKb(in)
		t.Fatalf("unexpected invalid result: warning=%q\n  intermediates: inner ka=%.6g kb=%.6g", res.Warning, ka, kb)
	}
	if math.Abs(res.Alpha-e.Output.Alpha) > goldenAngleTol ||
		math.Abs(res.Beta-e.Output.Beta) > goldenAngleTol ||
		!pressureOK(res.TotalPressure, e.Output.Pt) ||
		!pressureOK(res.StaticPressure, e.Output.Ps) ||
		!relOK(res.MachNumber, e.Output.Ma) ||
		!relOK(res.Velocity, e.Output.V) {
		ka, kb, _ := innerKaKb(in)
		t.Errorf("mismatch (mode=%s sector=%d fallback=%v)\n  got:  alpha=%.8f beta=%.8f pt=%.6f ps=%.6f ma=%.8f v=%.6f\n  want: alpha=%.8f beta=%.8f pt=%.6f ps=%.6f ma=%.8f v=%.6f\n  intermediates: inner ka=%.6g kb=%.6g",
			e.Mode, e.Sector, e.Fallback,
			res.Alpha, res.Beta, res.TotalPressure, res.StaticPressure, res.MachNumber, res.Velocity,
			e.Output.Alpha, e.Output.Beta, e.Output.Pt, e.Output.Ps, e.Output.Ma, e.Output.V,
			ka, kb)
	}
}

// TestGolden is the 481-point calibration round-trip gate (spec section 7.1).
// Every source row is an exact calibration node: 169 inner nodes and 312
// outer nodes must reproduce the generated full-precision golden result.
func TestGolden(t *testing.T) {
	p := loadGoldenInterpolator(t)
	entries := loadGoldenEntries(t, "golden.json")
	if len(entries) != 481 {
		t.Fatalf("golden entry count = %d, want 481", len(entries))
	}
	counts := map[string]int{}
	for _, e := range entries {
		counts[e.Mode]++
		t.Run(fmt.Sprintf("idx%03d_%s", e.Index, e.Mode), func(t *testing.T) {
			checkGoldenEntry(t, p, e)
		})
	}
	t.Logf("golden modes: %v", counts)
}

// TestBoundary runs calibration boundaries, guard cases, and independently
// generated non-node Python parity cases, all inside the legal grid.
func TestBoundary(t *testing.T) {
	p := loadGoldenInterpolator(t)
	entries := loadGoldenEntries(t, "boundary.json")
	if len(entries) < 8 {
		t.Fatalf("boundary entry count = %d, want >= 8", len(entries))
	}
	for _, e := range entries {
		t.Run(fmt.Sprintf("idx%03d_%s_%s", e.Index, e.Mode, e.Why), func(t *testing.T) {
			if e.Mode == "out" {
				t.Fatalf("boundary case must be inside the legal grid, got mode=out")
			}
			checkGoldenEntry(t, p, e)
		})
	}
}

func TestPythonParityNonNodeInputs(t *testing.T) {
	p := loadGoldenInterpolator(t)
	entries := loadGoldenEntries(t, "boundary.json")
	want := map[string]bool{"inner": false, "outer": false}
	for _, e := range entries {
		for kind := range want {
			if e.Why == "python parity non-node "+kind {
				checkGoldenEntry(t, p, e)
				want[kind] = true
			}
		}
	}
	for kind, found := range want {
		if !found {
			t.Errorf("boundary golden missing Python parity non-node %s case", kind)
		}
	}
}

// TestGoldenNegativeGaugePressure documents that negative gauge pressures
// are legal inputs (spec section 7.2): the dataset point with P3 ~= -2771 Pa
// (large-angle sector 1, first row) is part of the golden set and must be
// computed, not rejected.
func TestGoldenNegativeGaugePressure(t *testing.T) {
	entries := loadGoldenEntries(t, "golden.json")
	var neg *goldenEntry
	for i := range entries {
		if entries[i].Input.P3 < -2700 {
			neg = &entries[i]
			break
		}
	}
	if neg == nil {
		t.Fatal("golden set must contain the P3 ~= -2771 Pa dataset point")
	}
	p := loadGoldenInterpolator(t)
	in := InterpolationInput{
		P1: neg.Input.P1, P2: neg.Input.P2, P3: neg.Input.P3, P4: neg.Input.P4,
		P5: neg.Input.P5, P6: neg.Input.P6, P7: neg.Input.P7,
		PAtm: neg.Input.Pa, TAtm: neg.Input.T,
	}
	res, err := p.Calculate(in)
	if err != nil {
		t.Fatalf("negative gauge pressure must be accepted, got error: %v", err)
	}
	if !res.IsValid {
		t.Fatalf("negative gauge pressure must be accepted, got invalid: %q", res.Warning)
	}
}

// TestInnerCalibrationNodesStayInner is the regression gate for the
// self-extracted PRB reverse-inference fix: all 169 small-angle calibration
// grid nodes (the first 169 golden entries, in dataset row order) must resolve
// in the INNER zone, never being routed to the large-angle zone (spec: inner
// zone tried first; a calibration grid node is by construction inside the
// inner grid).
//
// Before the fix, the reference PRB carried 3-decimal-rounded Kalpha/Kbeta
// (~5e-4 error vs same-source recompute), so 21 boundary nodes were classified
// outside the inner polygon and fell through to the big-angle zone. Golden now
// records them as mode=little (grid-node direct hits), so this test asserts
// that contract directly.
func TestInnerCalibrationNodesStayInner(t *testing.T) {
	entries := loadGoldenEntries(t, "golden.json")
	if len(entries) < 169 {
		t.Fatalf("golden entries = %d, want >= 169 inner rows", len(entries))
	}
	p := loadGoldenInterpolator(t)
	for i := 0; i < 169; i++ {
		e := entries[i]
		in := InterpolationInput{
			P1: e.Input.P1, P2: e.Input.P2, P3: e.Input.P3, P4: e.Input.P4,
			P5: e.Input.P5, P6: e.Input.P6, P7: e.Input.P7,
			PAtm: e.Input.Pa, TAtm: e.Input.T,
		}
		res, err := p.Calculate(in)
		if err != nil {
			t.Errorf("inner node idx%d: error %v", i, err)
			continue
		}
		if !res.IsValid {
			t.Errorf("inner node idx%d: invalid %q", i, res.Warning)
			continue
		}
		// Inner mode: Theta==Alpha and Phi==Beta (spec: inner zone theta=alpha).
		if res.Theta != res.Alpha || res.Phi != res.Beta {
			t.Errorf("inner node idx%d: routed to outer zone (theta=%.6f phi=%.6f)", i, res.Theta, res.Phi)
		}
	}
}

// TestCalibrationNodesRoundTripExact verifies the defining self-consistency
// contract of calibration data: pressures from every committed calibration
// row must resolve to that row's own grid node. Exact-node recognition is
// intentionally tighter than the interpolation fallback so nearby runtime
// measurements remain continuous rather than snapping to a node.
func TestCalibrationNodesRoundTripExact(t *testing.T) {
	entries := loadGoldenEntries(t, "golden.json")
	if len(entries) != 481 {
		t.Fatalf("golden entries = %d, want 481", len(entries))
	}
	p := loadGoldenInterpolator(t)
	for i, e := range entries {
		in := InterpolationInput{
			P1: e.Input.P1, P2: e.Input.P2, P3: e.Input.P3, P4: e.Input.P4,
			P5: e.Input.P5, P6: e.Input.P6, P7: e.Input.P7,
			PAtm: e.Input.Pa, TAtm: e.Input.T,
		}
		res, err := p.Calculate(in)
		if err != nil {
			t.Errorf("calibration row %d: Calculate: %v", i, err)
			continue
		}
		if !res.IsValid {
			t.Errorf("calibration row %d: invalid: %s", i, res.Warning)
			continue
		}

		if i < innerPointCount {
			ia := i % innerGridSide
			ib := i / innerGridSide
			gp := p.inner.points[ia][ib]
			wantPt, wantPs, solveErr := solveInnerPtPs(in, gp.cpt, gp.cps)
			if solveErr != nil {
				t.Fatalf("inner row %d: solve expected pressures: %v", i, solveErr)
			}
			assertCalibrationResult(t, i, res, gp.a, gp.b, wantPt, wantPs)
			continue
		}

		outerIndex := i - innerPointCount
		sector := outerIndex/52 + 1
		ka, kb, coeffErr := outerKaKb(in, sector)
		if coeffErr != nil {
			t.Fatalf("outer row %d sector %d: coefficients: %v", i, sector, coeffErr)
		}
		gp, ok := outerFindGridPointByKaKbWithin(p.outer[sector-1], ka, kb, gridEps)
		if !ok {
			t.Fatalf("outer row %d sector %d: pressure coefficients do not match its calibration grid", i, sector)
		}
		wantPt, wantPs, solveErr := solveOuterPtPs(in, sector, gp.cpt, gp.cps)
		if solveErr != nil {
			t.Fatalf("outer row %d sector %d: solve expected pressures: %v", i, sector, solveErr)
		}
		assertCalibrationResult(t, i, res, gp.a, gp.b, wantPt, wantPs)
	}
}

func assertCalibrationResult(t *testing.T, row int, got InterpolationResult, theta, phi, pt, ps float64) {
	t.Helper()
	const angleTolerance = 1e-8
	const pressureTolerance = 1e-6
	if math.Abs(got.Theta-theta) > angleTolerance || math.Abs(got.Phi-phi) > angleTolerance {
		t.Errorf("calibration row %d: theta/phi=(%.10g,%.10g), want (%.10g,%.10g)",
			row, got.Theta, got.Phi, theta, phi)
	}
	if math.Abs(got.TotalPressure-pt) > pressureTolerance || math.Abs(got.StaticPressure-ps) > pressureTolerance {
		t.Errorf("calibration row %d: pt/ps=(%.10g,%.10g), want (%.10g,%.10g)",
			row, got.TotalPressure, got.StaticPressure, pt, ps)
	}
}

// TestGuardCases re-asserts the Go product guards against the REAL fixture
// grids (spec section 4 / 7.2): pt<ps returns an error (not Python's fabs),
// NaN/Inf inputs error out, and the 1e-12 denominator guards hold.
func TestGuardCases(t *testing.T) {
	p := loadGoldenInterpolator(t)

	// pt<ps with p7 below the ring average (inner-zone solve) -> error, not
	// IsValid=false and not a silently absolutized result.
	lowCenter := InterpolationInput{
		P1: 200, P2: 200, P3: 200, P4: 200, P5: 200, P6: 200, P7: 100,
		PAtm: 101325, TAtm: 20,
	}
	if _, err := p.Calculate(lowCenter); err == nil {
		t.Error("pt<ps must return an error")
	} else if !strings.Contains(err.Error(), "总压低于静压") {
		t.Errorf("pt<ps error %q must name the cause", err.Error())
	}

	base := InterpolationInput{
		P1: 100, P2: 100, P3: 100, P4: 100, P5: 100, P6: 100, P7: 110,
		PAtm: 101325, TAtm: 20,
	}
	nanIn := base
	nanIn.P3 = math.NaN()
	if _, err := p.Calculate(nanIn); err == nil {
		t.Error("NaN input must error")
	}
	infIn := base
	infIn.P7 = math.Inf(1)
	if _, err := p.Calculate(infIn); err == nil {
		t.Error("Inf input must error")
	}

	// Inner denominator guard with the real grid: p7 == pAvg.
	eqAvg := InterpolationInput{
		P1: 100, P2: 100, P3: 100, P4: 100, P5: 100, P6: 100, P7: 100,
		PAtm: 101325, TAtm: 20,
	}
	if _, err := p.Calculate(eqAvg); err == nil {
		t.Error("p7 == pAvg must trigger the 1e-12 denominator guard")
	}
}
