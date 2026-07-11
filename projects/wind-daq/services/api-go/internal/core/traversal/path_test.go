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

// TestGridPointsFromAxesOrdered 验证主轴顺序切换与归一化：
//   - primaryAxis="x"：外层 Y、内层 X，先沿 X 走完一条线
//   - primaryAxis="y"：外层 X、内层 Y，等价于 GridPointsFromAxes
//   - 空字符串：保旧行为（先走 Y），用于升级前保存的 profile 兼容
//   - 大写 "X"/"Y" 与带空白的 " x "：归一化后等价于小写
func TestGridPointsFromAxesOrdered(t *testing.T) {
	xs := []float64{0, 1, 2}
	ys := []float64{0, 1}

	// 先走 X：(0,0), (1,0), (2,0), (0,1), (1,1), (2,1)
	xFirst := GridPointsFromAxesOrdered(xs, ys, "x")
	if len(xFirst) != 6 {
		t.Fatalf("expected 6 points, got %d", len(xFirst))
	}
	if xFirst[0].X != 0 || xFirst[0].Y != 0 {
		t.Fatalf("x-first: expected point[0] (0,0), got (%v,%v)", xFirst[0].X, xFirst[0].Y)
	}
	if xFirst[1].X != 1 || xFirst[1].Y != 0 {
		t.Fatalf("x-first: expected point[1] (1,0), got (%v,%v)", xFirst[1].X, xFirst[1].Y)
	}
	if xFirst[2].X != 2 || xFirst[2].Y != 0 {
		t.Fatalf("x-first: expected point[2] (2,0), got (%v,%v)", xFirst[2].X, xFirst[2].Y)
	}
	if xFirst[3].X != 0 || xFirst[3].Y != 1 {
		t.Fatalf("x-first: expected point[3] (0,1), got (%v,%v)", xFirst[3].X, xFirst[3].Y)
	}

	// 先走 Y：等价于 GridPointsFromAxes，(0,0), (0,1), (1,0), (1,1), (2,0), (2,1)
	yFirst := GridPointsFromAxesOrdered(xs, ys, "y")
	legacy := GridPointsFromAxes(xs, ys)
	if len(yFirst) != len(legacy) {
		t.Fatalf("y-first length mismatch: %d vs legacy %d", len(yFirst), len(legacy))
	}
	for i := range yFirst {
		if yFirst[i] != legacy[i] {
			t.Fatalf("y-first point[%d] = (%v,%v), expected legacy (%v,%v)",
				i, yFirst[i].X, yFirst[i].Y, legacy[i].X, legacy[i].Y)
		}
	}

	// 空字符串保旧行为（先走 Y），等价于 GridPointsFromAxes
	empty := GridPointsFromAxesOrdered(xs, ys, "")
	if empty[1].X != 0 || empty[1].Y != 1 {
		t.Fatalf("empty primaryAxis: expected legacy point[1] (0,1), got (%v,%v)", empty[1].X, empty[1].Y)
	}

	// 大写 "X" 归一化为 "x"，走先走 X 分支
	upperX := GridPointsFromAxesOrdered(xs, ys, "X")
	if upperX[1].X != 1 || upperX[1].Y != 0 {
		t.Fatalf("upper X: expected point[1] (1,0), got (%v,%v)", upperX[1].X, upperX[1].Y)
	}

	// 带空白的 " x " 归一化为 "x"
	trimmed := GridPointsFromAxesOrdered(xs, ys, " x ")
	if trimmed[1].X != 1 || trimmed[1].Y != 0 {
		t.Fatalf("trimmed ' x ': expected point[1] (1,0), got (%v,%v)", trimmed[1].X, trimmed[1].Y)
	}

	// 大写 "Y" 归一化为 "y"，走 legacy 分支
	upperY := GridPointsFromAxesOrdered(xs, ys, "Y")
	if upperY[1].X != 0 || upperY[1].Y != 1 {
		t.Fatalf("upper Y: expected legacy point[1] (0,1), got (%v,%v)", upperY[1].X, upperY[1].Y)
	}
}

// TestGridPointsFromAxesSnakeOrdered 验证蛇形主轴顺序切换：
//   - primaryAxis="x"（默认）：外层 Y、内层 X，奇数行反转 X 顺序
//   - primaryAxis="y"：等价于 GridPointsFromAxesSnake
func TestGridPointsFromAxesSnakeOrdered(t *testing.T) {
	xs := []float64{0, 1, 2}
	ys := []float64{0, 1}

	// 先走 X 蛇形：(0,0), (1,0), (2,0), (2,1), (1,1), (0,1)
	xFirst := GridPointsFromAxesSnakeOrdered(xs, ys, "x")
	if len(xFirst) != 6 {
		t.Fatalf("expected 6 points, got %d", len(xFirst))
	}
	if xFirst[0].X != 0 || xFirst[0].Y != 0 {
		t.Fatalf("x-first snake: expected point[0] (0,0), got (%v,%v)", xFirst[0].X, xFirst[0].Y)
	}
	if xFirst[1].X != 1 || xFirst[1].Y != 0 {
		t.Fatalf("x-first snake: expected point[1] (1,0), got (%v,%v)", xFirst[1].X, xFirst[1].Y)
	}
	if xFirst[2].X != 2 || xFirst[2].Y != 0 {
		t.Fatalf("x-first snake: expected point[2] (2,0), got (%v,%v)", xFirst[2].X, xFirst[2].Y)
	}
	// 奇数行（Y=1）反转 X：(2,1), (1,1), (0,1)
	if xFirst[3].X != 2 || xFirst[3].Y != 1 {
		t.Fatalf("x-first snake: expected point[3] (2,1), got (%v,%v)", xFirst[3].X, xFirst[3].Y)
	}
	if xFirst[4].X != 1 || xFirst[4].Y != 1 {
		t.Fatalf("x-first snake: expected point[4] (1,1), got (%v,%v)", xFirst[4].X, xFirst[4].Y)
	}
	if xFirst[5].X != 0 || xFirst[5].Y != 1 {
		t.Fatalf("x-first snake: expected point[5] (0,1), got (%v,%v)", xFirst[5].X, xFirst[5].Y)
	}

	// 先走 Y 蛇形：等价于 GridPointsFromAxesSnake
	yFirst := GridPointsFromAxesSnakeOrdered(xs, ys, "y")
	legacy := GridPointsFromAxesSnake(xs, ys)
	if len(yFirst) != len(legacy) {
		t.Fatalf("y-first snake length mismatch: %d vs legacy %d", len(yFirst), len(legacy))
	}
	for i := range yFirst {
		if yFirst[i] != legacy[i] {
			t.Fatalf("y-first snake point[%d] = (%v,%v), expected legacy (%v,%v)",
				i, yFirst[i].X, yFirst[i].Y, legacy[i].X, legacy[i].Y)
		}
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
	// 显式指定 PrimaryAxis="y"，保持原始“外层 X、内层 Y”蛇形预期：
	// (0,0), (0,1), (1,1), (1,0), (2,0), (2,1)
	points := PointsFromLayout(LayoutConfig{
		Pattern:     "rectangle",
		SnakeOrder:  true,
		PrimaryAxis: "y",
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

// TestPointsFromLayoutRectangleSnakeLegacy 验证空字符串（缺省）保旧行为：
// 等价于 PrimaryAxis="y"，外层 X、内层 Y，奇数行反转 Y。
// 此用例保护升级前保存的 profile（无 primaryAxis 字段）行为不变。
func TestPointsFromLayoutRectangleSnakeLegacy(t *testing.T) {
	points := PointsFromLayout(LayoutConfig{
		Pattern:    "rectangle",
		SnakeOrder: true,
		// PrimaryAxis 留空，验证缺省 = legacy（先走 Y）
		Rectangle: &RectangleLayout{
			XMin: 0, XMax: 2, XStepSegments: []StepSegment{{Start: 0, End: 2, Step: 1}},
			YMin: 0, YMax: 1, YStepSegments: []StepSegment{{Start: 0, End: 1, Step: 1}},
		},
	})
	// legacy 蛇形：(0,0), (0,1), (1,1), (1,0), (2,0), (2,1)
	if points[0].X != 0 || points[0].Y != 0 {
		t.Fatalf("legacy: expected point[0] (0,0), got (%v,%v)", points[0].X, points[0].Y)
	}
	if points[1].X != 0 || points[1].Y != 1 {
		t.Fatalf("legacy: expected point[1] (0,1), got (%v,%v)", points[1].X, points[1].Y)
	}
	if points[2].X != 1 || points[2].Y != 1 {
		t.Fatalf("legacy: expected point[2] (1,1), got (%v,%v)", points[2].X, points[2].Y)
	}
}

// TestPointsFromLayoutRectangleSnakePrimaryX 验证显式主轴 "x" 下的蛇形顺序：
// 外层 Y、内层 X，奇数行反转 X，结果应为 (0,0), (1,0), (2,0), (2,1), (1,1), (0,1)
func TestPointsFromLayoutRectangleSnakePrimaryX(t *testing.T) {
	points := PointsFromLayout(LayoutConfig{
		Pattern:     "rectangle",
		SnakeOrder:  true,
		PrimaryAxis: "x", // 显式指定，缺省不再走此分支
		Rectangle: &RectangleLayout{
			XMin: 0, XMax: 2, XStepSegments: []StepSegment{{Start: 0, End: 2, Step: 1}},
			YMin: 0, YMax: 1, YStepSegments: []StepSegment{{Start: 0, End: 1, Step: 1}},
		},
	})
	if len(points) != 6 {
		t.Fatalf("expected 6 points, got %d", len(points))
	}
	// 第0行（Y=0）：(0,0), (1,0), (2,0)
	if points[0].X != 0 || points[0].Y != 0 {
		t.Fatalf("expected point[0] (0,0), got (%v,%v)", points[0].X, points[0].Y)
	}
	if points[1].X != 1 || points[1].Y != 0 {
		t.Fatalf("expected point[1] (1,0), got (%v,%v)", points[1].X, points[1].Y)
	}
	if points[2].X != 2 || points[2].Y != 0 {
		t.Fatalf("expected point[2] (2,0), got (%v,%v)", points[2].X, points[2].Y)
	}
	// 第1行（Y=1）反转 X：(2,1), (1,1), (0,1)
	if points[3].X != 2 || points[3].Y != 1 {
		t.Fatalf("expected point[3] (2,1), got (%v,%v)", points[3].X, points[3].Y)
	}
	if points[4].X != 1 || points[4].Y != 1 {
		t.Fatalf("expected point[4] (1,1), got (%v,%v)", points[4].X, points[4].Y)
	}
	if points[5].X != 0 || points[5].Y != 1 {
		t.Fatalf("expected point[5] (0,1), got (%v,%v)", points[5].X, points[5].Y)
	}
}

// TestPointsFromLayoutRectanglePrimaryX 验证显式主轴 "x" 下的普通（非蛇形）顺序：
// 外层 Y、内层 X，结果应为 (0,0), (1,0), (2,0), (0,1), (1,1), (2,1)
func TestPointsFromLayoutRectanglePrimaryX(t *testing.T) {
	points := PointsFromLayout(LayoutConfig{
		Pattern:     "rectangle",
		PrimaryAxis: "x", // 显式指定，缺省不再走此分支
		Rectangle: &RectangleLayout{
			XMin: 0, XMax: 2, XStepSegments: []StepSegment{{Start: 0, End: 2, Step: 1}},
			YMin: 0, YMax: 1, YStepSegments: []StepSegment{{Start: 0, End: 1, Step: 1}},
		},
	})
	if len(points) != 6 {
		t.Fatalf("expected 6 points, got %d", len(points))
	}
	if points[0].X != 0 || points[0].Y != 0 {
		t.Fatalf("expected point[0] (0,0), got (%v,%v)", points[0].X, points[0].Y)
	}
	if points[1].X != 1 || points[1].Y != 0 {
		t.Fatalf("expected point[1] (1,0), got (%v,%v)", points[1].X, points[1].Y)
	}
	if points[2].X != 2 || points[2].Y != 0 {
		t.Fatalf("expected point[2] (2,0), got (%v,%v)", points[2].X, points[2].Y)
	}
	if points[3].X != 0 || points[3].Y != 1 {
		t.Fatalf("expected point[3] (0,1), got (%v,%v)", points[3].X, points[3].Y)
	}
}

// TestPointsFromLayoutLinePrimaryX 验证 line 布局简化后仅沿 X 轴布点：
// Y 恒为 0，不再消费 PrimaryAxis/SnakeOrder/YStepSegments。
// 覆盖 PointsFromLayout 的 line 分支 + GridPointsFromAxesOrdered 单行路径。
func TestPointsFromLayoutLinePrimaryX(t *testing.T) {
	points := PointsFromLayout(LayoutConfig{
		Pattern:     "line",
		PrimaryAxis: "x",
		Line: &LineLayout{
			StartX: 0, EndX: 2, XStepSegments: []StepSegment{{Start: 0, End: 2, Step: 1}},
		},
	})
	if len(points) != 3 {
		t.Fatalf("expected 3 points (single line), got %d", len(points))
	}
	// 单行：(0,0), (1,0), (2,0)
	expected := []struct{ x, y float64 }{{0, 0}, {1, 0}, {2, 0}}
	for i, exp := range expected {
		if points[i].X != exp.x || points[i].Y != exp.y {
			t.Fatalf("point[%d] expected (%v,%v), got (%v,%v)", i, exp.x, exp.y, points[i].X, points[i].Y)
		}
	}
}

// TestPointsFromLayoutLineIgnoresSnakeAndPrimaryY 验证 line 布局对 SnakeOrder / PrimaryAxis="y"
// 不再产生差异化结果：单行点位无走线方向概念，所有组合应得到相同的 3 点单行结果。
func TestPointsFromLayoutLineIgnoresSnakeAndPrimaryY(t *testing.T) {
	cases := []struct {
		name        string
		snakeOrder  bool
		primaryAxis string
	}{
		{"plain_ordered", false, ""},
		{"snake_x", true, "x"},
		{"snake_y_ignored", true, "y"},
		{"plain_y_ignored", false, "y"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			points := PointsFromLayout(LayoutConfig{
				Pattern:     "line",
				SnakeOrder:  c.snakeOrder,
				PrimaryAxis: c.primaryAxis,
				Line: &LineLayout{
					StartX: 0, EndX: 2, XStepSegments: []StepSegment{{Start: 0, End: 2, Step: 1}},
				},
			})
			if len(points) != 3 {
				t.Fatalf("expected 3 points for %s, got %d", c.name, len(points))
			}
			for i, p := range points {
				if p.Y != 0 {
					t.Fatalf("%s: point[%d].Y expected 0, got %v", c.name, i, p.Y)
				}
			}
		})
	}
}

func TestPointsFromLayoutCustom(t *testing.T) {
	points := PointsFromLayout(LayoutConfig{
		Pattern: "custom",
		Custom: &CustomLayout{
			Points: []struct {
				X float64 `json:"x"`
				Y float64 `json:"y"`
				Z float64 `json:"z"`
				U float64 `json:"u"`
			}{{X: 1, Y: 2, Z: 3, U: 4}, {X: 5, Y: 6, Z: 7, U: 8}},
		},
	})
	if len(points) != 2 {
		t.Fatalf("expected 2 points, got %d", len(points))
	}
	if points[0].X != 1 || points[0].Y != 2 || points[0].Z != 3 || points[0].U != 4 {
		t.Fatalf("expected (1,2,3,4), got (%v,%v,%v,%v)", points[0].X, points[0].Y, points[0].Z, points[0].U)
	}
	if points[1].X != 5 || points[1].Y != 6 || points[1].Z != 7 || points[1].U != 8 {
		t.Fatalf("expected (5,6,7,8), got (%v,%v,%v,%v)", points[1].X, points[1].Y, points[1].Z, points[1].U)
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
