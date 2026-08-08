package interpolation

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"testing"
)

// loadOuterSector loads one synthetic outer sector grid into p via the real
// loader (geometry precompute included). thetaCount=4（默认，匹配历史夹具）；
// 如需其他维度请用 loadOuterSectorTheta。
func loadOuterSector(t *testing.T, p *SevenHolePrbInterpolator, sector int, f func(theta, phi float64) (ka, kb, cpt, cps float64)) {
	t.Helper()
	loadOuterSectorTheta(t, p, sector, defaultTestThetaCount, f)
}

// loadOuterSectorTheta 用指定 thetaCount 加载合成外区扇区网格。
func loadOuterSectorTheta(t *testing.T, p *SevenHolePrbInterpolator, sector, thetaCount int, f func(theta, phi float64) (ka, kb, cpt, cps float64)) {
	t.Helper()
	center := float64(sector-1) * 60
	lines := []string{"ka kb cpt cps a b"}
	for k := 0; k < outerPhiCount; k++ {
		b := normalize360(center + 30 - gridStep*float64(k))
		for it := 0; it < thetaCount; it++ {
			theta := outerThetaMin + gridStep*float64(it)
			ka, kb, cpt, cps := f(theta, b)
			lines = append(lines, fmt.Sprintf("%s %s %s %s %.6f %.6f",
				strconv.FormatFloat(ka, 'g', -1, 64),
				strconv.FormatFloat(kb, 'g', -1, 64),
				strconv.FormatFloat(cpt, 'g', -1, 64),
				strconv.FormatFloat(cps, 'g', -1, 64),
				theta, b))
		}
	}
	if err := p.LoadOuterPrbLines(sector, lines, fmt.Sprintf("synthetic-%d.prb", sector)); err != nil {
		t.Fatalf("load sector %d: %v", sector, err)
	}
}

// buildFullTestInterpolator returns an interpolator with the inner zone and
// all six outer sectors loaded from linear synthetic maps.
func buildFullTestInterpolator(t *testing.T) *SevenHolePrbInterpolator {
	t.Helper()
	p := buildInnerTestInterpolator(t, linearInnerMap)
	for sector := 1; sector <= outerSectorCount; sector++ {
		loadOuterSector(t, p, sector, linearOuterMap(float64(sector-1)*60))
	}
	return p
}

func TestIsLoaded(t *testing.T) {
	p := NewSevenHolePrbInterpolator()
	if p.IsLoaded() {
		t.Error("empty interpolator must not be loaded")
	}
	if err := p.LoadInnerPrbLines(makeInnerLines(), "7.prb"); err != nil {
		t.Fatal(err)
	}
	if p.IsLoaded() {
		t.Error("inner-only interpolator must not be loaded")
	}
	for sector := 1; sector <= 5; sector++ {
		loadOuterSector(t, p, sector, linearOuterMap(float64(sector-1)*60))
	}
	if p.IsLoaded() {
		t.Error("missing sector 6 must not be loaded")
	}
	loadOuterSector(t, p, 6, linearOuterMap(300))
	if !p.IsLoaded() {
		t.Error("full 7-file set must be loaded")
	}
}

func TestGetValidRange(t *testing.T) {
	if got := NewSevenHolePrbInterpolator().GetValidRange(); got != (PrbValidRange{}) {
		t.Errorf("unloaded GetValidRange = %+v, want zero", got)
	}
	p := buildFullTestInterpolator(t)
	got := p.GetValidRange()
	want := PrbValidRange{AlphaMin: -30, AlphaMax: 30, BetaMin: -30, BetaMax: 30, MachMin: 0, MachMax: 0}
	if got != want {
		t.Errorf("GetValidRange = %+v, want %+v", got, want)
	}
}

// TestIdentity verifies the optional identity capability: stable, and
// carrying the inner file plus all six sector file labels.
func TestIdentity(t *testing.T) {
	p := buildFullTestInterpolator(t)
	id := p.Identity()
	if id == "" || id != p.Identity() {
		t.Fatalf("Identity not stable/non-empty: %q", id)
	}
	for _, sub := range []string{"7=synthetic-7.prb", "1=synthetic-1.prb", "2=synthetic-2.prb", "3=synthetic-3.prb",
		"4=synthetic-4.prb", "5=synthetic-5.prb", "6=synthetic-6.prb"} {
		if !strings.Contains(id, sub) {
			t.Errorf("Identity %q missing %q", id, sub)
		}
	}
	if got := NewSevenHolePrbInterpolator().Identity(); got == "" {
		t.Error("unloaded Identity must be non-empty")
	}
}

// TestCalculateInnerEndToEnd runs a uniform-ring pressure input through the
// full inner-zone path and checks every assembled field.
func TestCalculateInnerEndToEnd(t *testing.T) {
	p := buildFullTestInterpolator(t)
	in := InterpolationInput{
		P1: 100, P2: 100, P3: 100, P4: 100, P5: 100, P6: 100, P7: 110,
		PAtm: 101325, TAtm: 20,
	}
	res, err := p.Calculate(in)
	if err != nil {
		t.Fatalf("Calculate: %v", err)
	}
	if !res.IsValid {
		t.Fatalf("IsValid=false, warning=%q", res.Warning)
	}
	// ka=kb=0 -> a=0,b=0; cpt=1.1, cps=-0.5 (linear field at origin);
	// pt=(110*0.5+1.1*100)/1.6=103.125, ps=(110*(-0.5)+100*2.1)/1.6=96.875.
	if math.Abs(res.Alpha) > 1e-9 || math.Abs(res.Beta) > 1e-9 {
		t.Errorf("angles = (%v,%v), want (0,0)", res.Alpha, res.Beta)
	}
	if math.Abs(res.TotalPressure-103.125) > 1e-9 {
		t.Errorf("Pt = %v, want 103.125", res.TotalPressure)
	}
	if math.Abs(res.StaticPressure-96.875) > 1e-9 {
		t.Errorf("Ps = %v, want 96.875", res.StaticPressure)
	}
	if math.Abs(res.DynamicPressure-6.25) > 1e-9 {
		t.Errorf("dynamic = %v, want 6.25", res.DynamicPressure)
	}
	if res.MachNumber <= 0 || res.Velocity <= 0 {
		t.Errorf("Ma/V must be positive, got Ma=%v V=%v", res.MachNumber, res.Velocity)
	}
	if res.Warning != "" {
		t.Errorf("unexpected warning %q", res.Warning)
	}
}

// TestCalculateOuterEndToEnd forces the large-angle path with P2 dominant
// and checks the sector hit, the (theta,phi)->(alpha,beta) transform, and
// the solved pressures.
func TestCalculateOuterEndToEnd(t *testing.T) {
	p := buildFullTestInterpolator(t)
	in := InterpolationInput{
		P1: 100, P2: 1000, P3: 100, P4: 100, P5: 100, P6: 100, P7: 100,
		PAtm: 101325, TAtm: 20,
	}
	res, err := p.Calculate(in)
	if err != nil {
		t.Fatalf("Calculate: %v", err)
	}
	if !res.IsValid {
		t.Fatalf("IsValid=false, warning=%q", res.Warning)
	}
	// Sector 2 map: ka=1.0,kb=0 -> theta=40, phi=40 (s=-20);
	// cpt=1.3, cps=-0.18 -> pt=950/2.12, ps=50/2.12.
	if math.Abs(res.TotalPressure-950.0/2.12) > 1e-6 {
		t.Errorf("Pt = %v, want %v", res.TotalPressure, 950.0/2.12)
	}
	if math.Abs(res.StaticPressure-50.0/2.12) > 1e-6 {
		t.Errorf("Ps = %v, want %v", res.StaticPressure, 50.0/2.12)
	}
	// alpha = -atan(tan40*sin40), beta = atan(tan40*cos40).
	tan40 := math.Tan(40 * math.Pi / 180)
	wantAlpha := -math.Atan(tan40*math.Sin(40*math.Pi/180)) * 180 / math.Pi
	wantBeta := math.Atan(tan40*math.Cos(40*math.Pi/180)) * 180 / math.Pi
	if math.Abs(res.Alpha-wantAlpha) > 1e-9 || math.Abs(res.Beta-wantBeta) > 1e-9 {
		t.Errorf("angles = (%v,%v), want (%v,%v)", res.Alpha, res.Beta, wantAlpha, wantBeta)
	}
	if res.Alpha >= 0 {
		t.Errorf("alpha must be negative for phi=40 (minus sign), got %v", res.Alpha)
	}
}

// TestCalculateOutOfGrid verifies the no-extrapolation decision (spec
// section 4): both candidate sectors miss -> IsValid=false + warning, no
// numeric output.
func TestCalculateOutOfGrid(t *testing.T) {
	p := buildInnerTestInterpolator(t, linearInnerMap)
	shifted := func(center float64) func(theta, phi float64) (ka, kb, cpt, cps float64) {
		return func(theta, phi float64) (ka, kb, cpt, cps float64) {
			ka, kb, cpt, cps = linearOuterMap(center)(theta, phi)
			return ka + 100, kb + 100, cpt, cps // sector polygons far from any realistic (ka,kb)
		}
	}
	for sector := 1; sector <= outerSectorCount; sector++ {
		loadOuterSector(t, p, sector, shifted(float64(sector-1)*60))
	}
	in := InterpolationInput{
		P1: 1, P2: 2, P3: 3, P4: 100000, P5: 5, P6: 6, P7: 100,
		PAtm: 101325, TAtm: 20,
	}
	res, err := p.Calculate(in)
	if err != nil {
		t.Fatalf("out-of-grid must not error, got: %v", err)
	}
	if res.IsValid {
		t.Fatal("out-of-grid must be IsValid=false")
	}
	if !strings.Contains(res.Warning, "不支持外推") {
		t.Errorf("warning %q must state no extrapolation", res.Warning)
	}
	if res.Alpha != 0 || res.Beta != 0 || res.TotalPressure != 0 || res.StaticPressure != 0 {
		t.Errorf("out-of-grid must not carry numeric output: %+v", res)
	}
}

// TestCalculatePtBelowPs locks the intentional difference from Python
// (SKILL.md section 5 fabs defect, spec section 4): pt<ps returns an error,
// not a silently-absolutized result and not IsValid=false.
func TestCalculatePtBelowPs(t *testing.T) {
	p := buildFullTestInterpolator(t)
	// p7 < pAvg -> solved pt < ps at the inner origin.
	in := InterpolationInput{
		P1: 200, P2: 200, P3: 200, P4: 200, P5: 200, P6: 200, P7: 100,
		PAtm: 101325, TAtm: 20,
	}
	_, err := p.Calculate(in)
	if err == nil {
		t.Fatal("pt<ps must return an error")
	}
	if !strings.Contains(err.Error(), "总压低于静压") {
		t.Errorf("error %q must name pt<ps", err.Error())
	}
}

func TestCalculateNotLoaded(t *testing.T) {
	in := InterpolationInput{P1: 1, PAtm: 101325, TAtm: 20}
	if _, err := NewSevenHolePrbInterpolator().Calculate(in); err == nil {
		t.Fatal("unloaded interpolator must error")
	}
}

func TestCalculateNonFiniteInput(t *testing.T) {
	p := buildFullTestInterpolator(t)
	base := InterpolationInput{
		P1: 100, P2: 100, P3: 100, P4: 100, P5: 100, P6: 100, P7: 110,
		PAtm: 101325, TAtm: 20,
	}
	for _, tc := range []struct {
		name   string
		mutate func(*InterpolationInput)
	}{
		{"NaN P1", func(in *InterpolationInput) { in.P1 = math.NaN() }},
		{"Inf P7", func(in *InterpolationInput) { in.P7 = math.Inf(1) }},
		{"NaN PAtm", func(in *InterpolationInput) { in.PAtm = math.NaN() }},
		{"Inf Tatm", func(in *InterpolationInput) { in.TAtm = math.Inf(-1) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			in := base
			tc.mutate(&in)
			if _, err := p.Calculate(in); err == nil {
				t.Fatal("non-finite input must error")
			}
		})
	}
}

// TestCalculateSecondCandidateFallback drives the case where the
// largest-pressure hole's sector misses but the second candidate hits
// (Python first/second logic): P1 dominant pushes (ka,kb) for sector 1 far
// outside its polygon, while sector 2 still covers it.
func TestCalculateSecondCandidateFallback(t *testing.T) {
	p := buildInnerTestInterpolator(t, linearInnerMap)
	farSector1 := func(theta, phi float64) (ka, kb, cpt, cps float64) {
		ka, kb, cpt, cps = linearOuterMap(0)(theta, phi)
		return ka + 100, kb + 100, cpt, cps
	}
	loadOuterSector(t, p, 1, farSector1)
	for sector := 2; sector <= outerSectorCount; sector++ {
		loadOuterSector(t, p, sector, linearOuterMap(float64(sector-1)*60))
	}
	// P2 dominant with p7 moderate: sector-2 (ka,kb) lands inside its polygon
	// (same shape as TestCalculateOuterEndToEnd), sector-1's polygon is
	// shifted far away.
	in := InterpolationInput{
		P1: 100, P2: 1000, P3: 100, P4: 100, P5: 100, P6: 100, P7: 100,
		PAtm: 101325, TAtm: 20,
	}
	res, err := p.Calculate(in)
	if err != nil {
		t.Fatalf("Calculate: %v", err)
	}
	if !res.IsValid {
		t.Fatalf("second candidate must hit, warning=%q", res.Warning)
	}
	if math.Abs(res.TotalPressure-950.0/2.12) > 1e-6 {
		t.Errorf("Pt = %v, want %v (sector-2 solve)", res.TotalPressure, 950.0/2.12)
	}
}

func TestCalculateExactOuterNodeMustBePressureCandidate(t *testing.T) {
	shifted := func(center float64) func(theta, phi float64) (ka, kb, cpt, cps float64) {
		return func(theta, phi float64) (ka, kb, cpt, cps float64) {
			ka, kb, cpt, cps = linearOuterMap(center)(theta, phi)
			return ka + 100, kb + 100, cpt, cps
		}
	}
	p := buildInnerTestInterpolator(t, func(a, b float64) (ka, kb, cpt, cps float64) {
		ka, kb, cpt, cps = linearInnerMap(a, b)
		return ka + 100, kb + 100, cpt, cps
	})
	for sector := 1; sector <= outerSectorCount; sector++ {
		mapping := shifted(float64(sector-1) * 60)
		if sector == 2 {
			mapping = linearOuterMap(60)
		}
		loadOuterSector(t, p, sector, mapping)
	}

	// Sector 2 retains the exact coefficients from TestCalculateOuterEndToEnd,
	// but unrelated P4/P5 spikes make sectors 4 and 5 the pressure candidates.
	in := InterpolationInput{
		P1: 100, P2: 1000, P3: 100, P4: 3000, P5: 2000, P6: 100, P7: 100,
		PAtm: 101325, TAtm: 20,
	}
	res, err := p.Calculate(in)
	if err != nil {
		t.Fatalf("Calculate: %v", err)
	}
	if res.IsValid {
		t.Fatalf("non-candidate sector 2 must not route through its exact node: %+v", res)
	}
}

// TestGetPointCount 验证 GetInnerPointCount/GetOuterPointCount 在动态
// thetaCount 下返回真实点数：内区固定 169，外区随 thetaCount 变化。
func TestGetPointCount(t *testing.T) {
	// 未加载 → 全部返回 0
	empty := NewSevenHolePrbInterpolator()
	if got := empty.GetInnerPointCount(); got != 0 {
		t.Errorf("unloaded inner count = %d, want 0", got)
	}
	if got := empty.GetOuterPointCount(1); got != 0 {
		t.Errorf("unloaded outer count = %d, want 0", got)
	}
	// 越界 sector → 返回 0
	if got := empty.GetOuterPointCount(0); got != 0 {
		t.Errorf("sector 0 count = %d, want 0", got)
	}
	if got := empty.GetOuterPointCount(7); got != 0 {
		t.Errorf("sector 7 count = %d, want 0", got)
	}

	// 默认 thetaCount=4：内区 169，外区 52
	p := buildFullTestInterpolator(t)
	if got := p.GetInnerPointCount(); got != 169 {
		t.Errorf("inner count = %d, want 169", got)
	}
	for sector := 1; sector <= outerSectorCount; sector++ {
		if got := p.GetOuterPointCount(sector); got != defaultTestThetaCount*outerPhiCount {
			t.Errorf("sector %d outer count = %d, want %d", sector, got, defaultTestThetaCount*outerPhiCount)
		}
	}
}

// TestGetPointCount_DynamicThetaCount 验证动态 thetaCount 下点数查询正确：
// thetaCount=7 时外区点数应为 7×13=91。
func TestGetPointCount_DynamicThetaCount(t *testing.T) {
	const thetaCount = 7
	p := buildInnerTestInterpolator(t, linearInnerMap)
	for sector := 1; sector <= outerSectorCount; sector++ {
		loadOuterSectorTheta(t, p, sector, thetaCount, linearOuterMap(float64(sector-1)*60))
	}
	if got := p.GetInnerPointCount(); got != 169 {
		t.Errorf("inner count = %d, want 169", got)
	}
	for sector := 1; sector <= outerSectorCount; sector++ {
		want := thetaCount * outerPhiCount // 7×13=91
		if got := p.GetOuterPointCount(sector); got != want {
			t.Errorf("sector %d outer count = %d, want %d", sector, got, want)
		}
	}
}
