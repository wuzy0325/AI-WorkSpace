package interpolation

import (
	"math"
	"strings"
	"testing"
)

func TestResolveRegion9UsesBilinearInverseOnWarpedCell(t *testing.T) {
	cell := region9Cell{
		X1: probeTableRow{Alpha: -30, Beta: 25},
		X2: probeTableRow{Alpha: -25, Beta: 25},
		X3: probeTableRow{Alpha: -25, Beta: 30},
		X4: probeTableRow{Alpha: -30, Beta: 30},
		Vertices: [4]point2D{
			{X: -1.4, Y: 0.2},
			{X: -0.5, Y: 0.1},
			{X: -0.2, Y: 1.5},
			{X: -1.8, Y: 1.0},
		},
	}
	point := interpolationPoint{Ka: -0.975, Kb: 0.7}

	result := resolveRegion9(point, []region9Cell{cell})
	if result.Alpha == nil || result.Beta == nil {
		t.Fatal("expected warped cell point to resolve")
	}
	if math.Abs(*result.Alpha-(-27.5)) > 1e-9 || math.Abs(*result.Beta-27.5) > 1e-9 {
		t.Fatalf("resolved angles = (%.12f, %.12f), want (-27.5, 27.5)", *result.Alpha, *result.Beta)
	}
}

func TestPrbOutOfRangeReturnsExplicitInvalidResult(t *testing.T) {
	interpolator := NewPrbInterpolator()
	if err := interpolator.LoadPrbLines(syntheticPrbLines(0.05, 0.01), "0.1Ma.prb"); err != nil {
		t.Fatalf("LoadPrbLines: %v", err)
	}

	result, err := interpolator.Calculate(prbInputForAngles(50, 50))
	if err != nil {
		t.Fatalf("Calculate: %v", err)
	}
	if result.IsValid {
		t.Fatal("out-of-range input must be invalid")
	}
	if result.Alpha != 0 || result.Beta != 0 || result.TotalPressure != 0 || result.StaticPressure != 0 {
		t.Fatalf("out-of-range result must not expose clamped values: %+v", result)
	}
	if !strings.Contains(result.Warning, "不支持外推") {
		t.Fatalf("warning = %q, want explicit unsupported extrapolation warning", result.Warning)
	}
}
