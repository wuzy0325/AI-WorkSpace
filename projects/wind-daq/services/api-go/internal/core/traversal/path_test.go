package traversal

import (
	"math"
	"testing"
)

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

// 注：SectorPointsFromRadiiAngles / SectorPointsFromRadiiAnglesSnake 及其测试已删除（B4）——
// 扇形布点改走 GridPointsFromAxes 后这两个函数仅余自身测试引用，无生产消费方；
// 扇形相对目标行为由下面的 TestPointsFromLayoutSectorUsesRelativeRadiusAndAngleTargets 覆盖。

// 扇形机构由一个径向平移轴和一个旋转轴组成。第一个测点是当前机械位置，
// 后续目标必须表达为相对首点的 (半径增量, 角度增量)，不能把预览图的笛卡尔坐标发给运动轴。
func TestPointsFromLayoutSectorUsesRelativeRadiusAndAngleTargets(t *testing.T) {
	points := PointsFromLayout(LayoutConfig{
		Pattern: "sector",
		Sector: &SectorLayout{
			RadiusMin: 100, RadiusMax: 150,
			RadialStepSegments: []StepSegment{{Start: 100, End: 150, Step: 50}},
			AngleStart:         -30, AngleEnd: 30,
			AngularStepSegments: []StepSegment{{Start: -30, End: 30, Step: 30}},
		},
	})

	expected := []Point{
		{X: 0, Y: 0}, {X: 0, Y: 30}, {X: 0, Y: 60},
		{X: 50, Y: 0}, {X: 50, Y: 30}, {X: 50, Y: 60},
	}
	if len(points) != len(expected) {
		t.Fatalf("expected %d points, got %d", len(expected), len(points))
	}
	for i, want := range expected {
		if points[i].X != want.X || points[i].Y != want.Y {
			t.Fatalf("point[%d] = (%v,%v), want relative radius/angle target (%v,%v)",
				i, points[i].X, points[i].Y, want.X, want.Y)
		}
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

// TestPointsFromLayoutLinePrimaryX 验证 line 布局仅沿 X 轴布点：
// Y/Z/U 标记为 NaN 表示"不参与遍历运动"（markAxesNaN），availableAxisTargets 会跳过 NaN 轴。
// 旧实现 Y 固定为 0，配合 motionAxes 默认含 Y 会把 Y 轴强制归零——已修复。
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
	// 单行 X 坐标：0, 1, 2
	expectedX := []float64{0, 1, 2}
	for i, expX := range expectedX {
		if points[i].X != expX {
			t.Fatalf("point[%d].X expected %v, got %v", i, expX, points[i].X)
		}
		// Y/Z/U 应为 NaN：line 模式仅沿 X 运动，未配置轴标记 NaN 避免被强制归零
		if !math.IsNaN(points[i].Y) {
			t.Fatalf("point[%d].Y expected NaN, got %v", i, points[i].Y)
		}
		if !math.IsNaN(points[i].Z) {
			t.Fatalf("point[%d].Z expected NaN, got %v", i, points[i].Z)
		}
		if !math.IsNaN(points[i].U) {
			t.Fatalf("point[%d].U expected NaN, got %v", i, points[i].U)
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
			// Y 应为 NaN（line 模式仅沿 X 运动），不再检查 Y==0
			for i, p := range points {
				if !math.IsNaN(p.Y) {
					t.Fatalf("%s: point[%d].Y expected NaN, got %v", c.name, i, p.Y)
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

// === StepValues 重写后的测试（2026-07-12）===

// TestStepValuesAscending 验证递增方向步进
func TestStepValuesAscending(t *testing.T) {
	values := StepValues(0, 10, []StepSegment{{Start: 0, End: 10, Step: 2}})
	expected := []float64{0, 2, 4, 6, 8, 10}
	if len(values) != len(expected) {
		t.Fatalf("expected %d values, got %d: %v", len(expected), len(values), values)
	}
	for i, exp := range expected {
		if math.Abs(values[i]-exp) > 1e-9 {
			t.Fatalf("values[%d] expected %v, got %v", i, exp, values[i])
		}
	}
}

// TestStepValuesDescending 验证递减方向步进（P0-1 回归测试）
// 旧实现循环条件 `value <= actualEnd` 在 start > end 时立即退出导致丢点
func TestStepValuesDescending(t *testing.T) {
	values := StepValues(180, 0, []StepSegment{{Start: 180, End: 0, Step: 30}})
	expected := []float64{180, 150, 120, 90, 60, 30, 0}
	if len(values) != len(expected) {
		t.Fatalf("expected %d values, got %d: %v", len(expected), len(values), values)
	}
	for i, exp := range expected {
		if math.Abs(values[i]-exp) > 1e-9 {
			t.Fatalf("values[%d] expected %v, got %v", i, exp, values[i])
		}
	}
}

// TestStepValuesDedup 验证量化 map 去重（P1-1 回归测试）
// 多个 segment 重叠时去重，O(N) 而非 O(N²)
func TestStepValuesDedup(t *testing.T) {
	values := StepValues(0, 10, []StepSegment{
		{Start: 0, End: 5, Step: 1},
		{Start: 3, End: 10, Step: 1},
	})
	// 0,1,2,3,4,5,3,4,5,6,7,8,9,10 → 去重后 0..10 共 11 个
	expected := []float64{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10}
	if len(values) != len(expected) {
		t.Fatalf("expected %d values after dedup, got %d: %v", len(expected), len(values), values)
	}
	for i, exp := range expected {
		if math.Abs(values[i]-exp) > 1e-9 {
			t.Fatalf("values[%d] expected %v, got %v", i, exp, values[i])
		}
	}
}

// TestStepValuesNoAccumulationError 验证整数索引步进避免浮点累加误差（P1-3 回归测试）
// 旧实现 0.1 累加 100 次得到 9.99999999999998，新实现用 float64(i)*step 得到精确 10.0
func TestStepValuesNoAccumulationError(t *testing.T) {
	values := StepValues(0, 1, []StepSegment{{Start: 0, End: 1, Step: 0.1}})
	if len(values) != 11 {
		t.Fatalf("expected 11 values, got %d: %v", len(values), values)
	}
	// 最后一个值应为 1.0 而非 0.99999999999999
	if math.Abs(values[10]-1.0) > 1e-9 {
		t.Fatalf("last value expected 1.0, got %v", values[10])
	}
}

// TestStepValuesSegmentsUnorderedSorted 验证 segments 乱序时最终排序（P0-3 回归测试）
// 旧实现按 segments 顺序追加不排序，导致点位跳走
func TestStepValuesSegmentsUnorderedSorted(t *testing.T) {
	values := StepValues(0, 10, []StepSegment{
		{Start: 5, End: 10, Step: 1},
		{Start: 0, End: 4, Step: 1},
	})
	// 最终应单调递增 0..10，而非 [5,6,7,8,9,10,0,1,2,3,4]
	for i := 1; i < len(values); i++ {
		if values[i] < values[i-1]-1e-9 {
			t.Fatalf("values not monotonically increasing at index %d: %v", i, values)
		}
	}
}

// TestStepValuesEmptySegmentsFallback 验证空 segments 回退为 [start, end]
func TestStepValuesEmptySegmentsFallback(t *testing.T) {
	values := StepValues(0, 10, nil)
	if len(values) != 2 {
		t.Fatalf("expected 2 values [start, end], got %d: %v", len(values), values)
	}
	if values[0] != 0 || values[1] != 10 {
		t.Fatalf("expected [0, 10], got %v", values)
	}
}

// TestStepValuesSameStartEnd 验证 start == end 返回单点
func TestStepValuesSameStartEnd(t *testing.T) {
	values := StepValues(5, 5, nil)
	if len(values) != 1 || values[0] != 5 {
		t.Fatalf("expected [5], got %v", values)
	}
}

// TestPointsFromLayoutRectangleZUNaN 验证 rectangle 模式 Z/U 标记为 NaN（P0-2 回归测试）
func TestPointsFromLayoutRectangleZUNaN(t *testing.T) {
	points := PointsFromLayout(LayoutConfig{
		Pattern: "rectangle",
		Rectangle: &RectangleLayout{
			XMin: 0, XMax: 1, XStepSegments: []StepSegment{{Start: 0, End: 1, Step: 1}},
			YMin: 0, YMax: 1, YStepSegments: []StepSegment{{Start: 0, End: 1, Step: 1}},
		},
	})
	if len(points) != 4 {
		t.Fatalf("expected 4 points, got %d", len(points))
	}
	for i, p := range points {
		if !math.IsNaN(p.Z) {
			t.Fatalf("point[%d].Z expected NaN, got %v", i, p.Z)
		}
		if !math.IsNaN(p.U) {
			t.Fatalf("point[%d].U expected NaN, got %v", i, p.U)
		}
	}
}

// TestPointsFromLayoutSectorZUNaN 验证 sector 模式 Z/U 标记为 NaN（P0-2 回归测试）
func TestPointsFromLayoutSectorZUNaN(t *testing.T) {
	points := PointsFromLayout(LayoutConfig{
		Pattern: "sector",
		Sector: &SectorLayout{
			CenterX: 0, CenterY: 0,
			RadiusMin: 1, RadiusMax: 2,
			RadialStepSegments: []StepSegment{{Start: 1, End: 2, Step: 1}},
			AngleStart:         0, AngleEnd: 90,
			AngularStepSegments: []StepSegment{{Start: 0, End: 90, Step: 90}},
		},
	})
	if len(points) != 4 {
		t.Fatalf("expected 4 points, got %d", len(points))
	}
	for i, p := range points {
		if !math.IsNaN(p.Z) {
			t.Fatalf("point[%d].Z expected NaN, got %v", i, p.Z)
		}
		if !math.IsNaN(p.U) {
			t.Fatalf("point[%d].U expected NaN, got %v", i, p.U)
		}
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
