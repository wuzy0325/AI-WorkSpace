package traversal

import "testing"

func TestInterpolateLinearPathIncludesEndpoints(t *testing.T) {
	path, err := InterpolateLinearPath([]Point{
		{X: 0, Y: 0, Z: 0},
		{X: 10, Y: 0, Z: 0},
	}, 5)
	if err != nil {
		t.Fatalf("InterpolateLinearPath returned error: %v", err)
	}
	if len(path) != 3 {
		t.Fatalf("expected 3 points, got %d", len(path))
	}
	if path[0].X != 0 || path[1].X != 5 || path[2].X != 10 {
		t.Fatalf("expected x coordinates [0 5 10], got [%v %v %v]", path[0].X, path[1].X, path[2].X)
	}
}

func TestInterpolateLinearPathRejectsInvalidStep(t *testing.T) {
	_, err := InterpolateLinearPath([]Point{{X: 0}, {X: 1}}, 0)
	if err == nil {
		t.Fatal("expected invalid step error")
	}
}
