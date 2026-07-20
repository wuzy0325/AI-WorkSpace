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

// goldenEntry mirrors one record of testdata/golden/*.json, produced by
// device-lab/skills/seven-hole-probe/tools/gen_traversal_fixtures.py from
// the authoritative Python implementation (seven_hole.cal_ab). Regenerate
// the fixtures (and review the diff) whenever the .prb format, the
// algorithm, the seven_hole.py API, or the source dataset changes.
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

// TestGolden is the Go<->Python cross-check gate (spec section 7.1): 481
// calibration points. 435 inside the legal grid must match Python within
// tolerance; the 46 out-of-grid points (Python beyond_border path) assert
// the Go no-extrapolation contract instead (spec section 4).
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

// TestBoundary runs the constructed boundary cases (all inside the legal
// grid): inner +/-30 grid lines, the theta=30 junction, the theta=45 outer
// boundary, ka=kb=0 origin, pure sideslip, pure attack, a second-candidate
// fallback, a general in-sector point, and P7=0 as a legal input.
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
