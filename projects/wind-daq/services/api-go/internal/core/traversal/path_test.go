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

func TestGridPointsFromAxesNormal(t *testing.T) {
	xs := []float64{0, 1, 2}
	ys := []float64{0, 1}
	points := GridPointsFromAxes(xs, ys)
	expected := 6 // 3 * 2
	if len(points) != expected {
		t.Fatalf("expected %d points, got %d", expected, len(points))
	}
	// 第一个点
	if points[0].X != 0 || points[0].Y != 0 {
		t.Fatalf("expected first point (0,0), got (%v,%v)", points[0].X, points[0].Y)
	}
}

func TestGridPointsFromAxesSnake(t *testing.T) {
	xs := []float64{0, 1, 2}
	ys := []float64{0, 1}
	points := GridPointsFromAxesSnake(xs, ys)
	if len(points) != 6 {
		t.Fatalf("expected 6 points, got %d", len(points))
	}
	// 第0行（偶数行）：正常顺序 (0,0), (0,1)
	if points[0].X != 0 || points[0].Y != 0 {
		t.Fatalf("expected point[0] (0,0), got (%v,%v)", points[0].X, points[0].Y)
	}
	if points[1].X != 0 || points[1].Y != 1 {
		t.Fatalf("expected point[1] (0,1), got (%v,%v)", points[1].X, points[1].Y)
	}
	// 第1行（奇数行）：反转顺序 (1,1), (1,0)
	if points[2].X != 1 || points[2].Y != 1 {
		t.Fatalf("expected point[2] (1,1), got (%v,%v)", points[2].X, points[2].Y)
	}
	if points[3].X != 1 || points[3].Y != 0 {
		t.Fatalf("expected point[3] (1,0), got (%v,%v)", points[3].X, points[3].Y)
	}
	// 第2行（偶数行）：正常顺序 (2,0), (2,1)
	if points[4].X != 2 || points[4].Y != 0 {
		t.Fatalf("expected point[4] (2,0), got (%v,%v)", points[4].X, points[4].Y)
	}
	if points[5].X != 2 || points[5].Y != 1 {
		t.Fatalf("expected point[5] (2,1), got (%v,%v)", points[5].X, points[5].Y)
	}
}

func TestSectorPointsFromRadiiAngles(t *testing.T) {
	radii := []float64{1}
	angles := []float64{0, 90}
	points := SectorPointsFromRadiiAngles(0, 0, radii, angles)
	if len(points) != 2 {
		t.Fatalf("expected 2 points, got %d", len(points))
	}
	// 0度: (1, 0)
	if points[0].X < 0.99 || points[0].Y > 0.01 {
		t.Fatalf("expected point[0] near (1,0), got (%v,%v)", points[0].X, points[0].Y)
	}
	// 90度: (0, 1)
	if points[1].X > 0.01 || points[1].Y < 0.99 {
		t.Fatalf("expected point[1] near (0,1), got (%v,%v)", points[1].X, points[1].Y)
	}
}

func TestSectorPointsFromRadiiAnglesSnake(t *testing.T) {
	radii := []float64{1, 2}
	angles := []float64{0, 90}
	points := SectorPointsFromRadiiAnglesSnake(0, 0, radii, angles)
	if len(points) != 4 {
		t.Fatalf("expected 4 points, got %d", len(points))
	}
	// 第0行（偶数行）：0度, 90度
	// 第1行（奇数行）：90度, 0度（反转）
	if points[2].X > 0.01 || points[2].Y < 1.99 {
		t.Fatalf("expected point[2] near (0,2) [90deg], got (%v,%v)", points[2].X, points[2].Y)
	}
	if points[3].X < 1.99 || points[3].Y > 0.01 {
		t.Fatalf("expected point[3] near (2,0) [0deg], got (%v,%v)", points[3].X, points[3].Y)
	}
}

func TestPointsFromLayoutRectangle(t *testing.T) {
	points := PointsFromLayout(LayoutConfig{
		Pattern: "rectangle",
		Rectangle: &RectangleLayout{
			XMin: 0, XMax: 2, XStepSegments: []StepSegment{{Start: 0, End: 2, Step: 1}},
			YMin: 0, YMax: 1, YStepSegments: []StepSegment{{Start: 0, End: 1, Step: 1}},
		},
	})
	if len(points) != 6 { // 3 * 2
		t.Fatalf("expected 6 points, got %d", len(points))
	}
}

func TestPointsFromLayoutRectangleSnake(t *testing.T) {
	points := PointsFromLayout(LayoutConfig{
		Pattern:    "rectangle",
		SnakeOrder: true,
		Rectangle: &RectangleLayout{
			XMin: 0, XMax: 2, XStepSegments: []StepSegment{{Start: 0, End: 2, Step: 1}},
			YMin: 0, YMax: 1, YStepSegments: []StepSegment{{Start: 0, End: 1, Step: 1}},
		},
	})
	if len(points) != 6 {
		t.Fatalf("expected 6 points, got %d", len(points))
	}
	// 第1行（奇数行）反转：x=1时 y=1 在前
	if points[2].Y != 1 {
		t.Fatalf("expected snake order: point[2].Y=1, got %v", points[2].Y)
	}
}

func TestPointsFromLayoutCustom(t *testing.T) {
	points := PointsFromLayout(LayoutConfig{
		Pattern: "custom",
		Custom: &CustomLayout{
			Points: []struct {
				X float64 `json:"x"`
				Y float64 `json:"y"`
			}{{X: 1, Y: 2}, {X: 3, Y: 4}},
		},
	})
	if len(points) != 2 {
		t.Fatalf("expected 2 points, got %d", len(points))
	}
	if points[0].X != 1 || points[0].Y != 2 {
		t.Fatalf("expected (1,2), got (%v,%v)", points[0].X, points[0].Y)
	}
}

func TestPointsFromLayoutUnknown(t *testing.T) {
	points := PointsFromLayout(LayoutConfig{Pattern: "unknown"})
	if points != nil {
		t.Fatalf("expected nil for unknown pattern, got %v", points)
	}
}

func TestValidatePressuresDisabled(t *testing.T) {
	valid, warnings := ValidatePressures(map[int]float64{0: 100}, &DataValidationConfig{Enabled: false}, nil)
	if !valid {
		t.Fatal("expected valid when validation is disabled")
	}
	if warnings != nil {
		t.Fatalf("expected no warnings when disabled, got %v", warnings)
	}
}

func TestValidatePressuresInRange(t *testing.T) {
	valid, warnings := ValidatePressures(map[int]float64{0: 50}, &DataValidationConfig{
		Enabled: true,
		PressureRange: map[string]*PressureRange{
			"P1": {Min: 0, Max: 100},
		},
	}, nil)
	if !valid {
		t.Fatal("expected valid for in-range value")
	}
	if len(warnings) != 0 {
		t.Fatalf("expected no warnings, got %v", warnings)
	}
}

func TestValidatePressuresOutOfRange(t *testing.T) {
	valid, warnings := ValidatePressures(map[int]float64{0: 200}, &DataValidationConfig{
		Enabled: true,
		PressureRange: map[string]*PressureRange{
			"P1": {Min: 0, Max: 100},
		},
		OnInvalid: "continue",
	}, nil)
	if !valid {
		t.Fatal("expected valid for continue mode even with out-of-range")
	}
	if len(warnings) != 1 {
		t.Fatalf("expected 1 warning, got %d", len(warnings))
	}
}

func TestValidatePressuresSkipMode(t *testing.T) {
	valid, _ := ValidatePressures(map[int]float64{0: 200}, &DataValidationConfig{
		Enabled: true,
		PressureRange: map[string]*PressureRange{
			"P1": {Min: 0, Max: 100},
		},
		OnInvalid: "skip",
	}, nil)
	if valid {
		t.Fatal("expected invalid for skip mode with out-of-range")
	}
}

func TestValidatePressuresWithLabels(t *testing.T) {
	// 显式映射模式：通道 17 是 P1，应触发 out-of-range
	valid, warnings := ValidatePressures(map[int]float64{17: 200, 0: 50}, &DataValidationConfig{
		Enabled: true,
		PressureRange: map[string]*PressureRange{
			"P1": {Min: 0, Max: 100},
		},
		OnInvalid: "continue",
	}, map[int]string{17: "P1", 0: "P2"})
	if !valid {
		t.Fatal("expected valid in continue mode")
	}
	if len(warnings) != 1 {
		t.Fatalf("expected 1 warning for P1 out-of-range, got %d", len(warnings))
	}
}

func TestStateIsTerminal(t *testing.T) {
	tests := []struct {
		state    State
		expected bool
	}{
		{StateCompleted, true},
		{StateStopped, true},
		{StateError, true},
		{StateRunning, false},
		{StatePaused, false},
		{StateIdle, false},
		{StateMoving, false},
		{StateStabilizing, false},
		{StateAcquiring, false},
		{StateSaving, false},
	}
	for _, tt := range tests {
		if got := tt.state.IsTerminal(); got != tt.expected {
			t.Errorf("State(%s).IsTerminal() = %v, want %v", tt.state, got, tt.expected)
		}
	}
}
