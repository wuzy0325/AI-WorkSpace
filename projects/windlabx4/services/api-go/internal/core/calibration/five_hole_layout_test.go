package calibration

import (
	"strings"
	"testing"
)

// 验证默认（Serpentine=false）按逐行 raster 扫描：每行 α 都从 AlphaMin 升序排列
func TestGenerateFiveHoleSnakePointsDefaultRaster(t *testing.T) {
	layout := FiveHolePointLayout{
		AlphaMin: 0, AlphaMax: 10, AlphaStep: 5, // 3 个 α：0,5,10
		BetaMin: 0, BetaMax: 10, BetaStep: 5, // 3 个 β：0,5,10
	}
	points, err := GenerateFiveHoleSnakePoints(layout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(points) != 9 {
		t.Fatalf("expected 9 points, got %d", len(points))
	}
	// 第二行（β=5）应为升序 0,5,10
	for i, wantAlpha := range []float64{0, 5, 10} {
		got := points[3+i].Coordinates["α"]
		if got != wantAlpha {
			t.Fatalf("raster row 2 idx %d: expected α=%v, got %v", i, wantAlpha, got)
		}
	}
}

// 验证开启蛇形后奇数行反向遍历 α
func TestGenerateFiveHoleSnakePointsSerpentineReversesOddRows(t *testing.T) {
	layout := FiveHolePointLayout{
		AlphaMin: 0, AlphaMax: 10, AlphaStep: 5,
		BetaMin: 0, BetaMax: 10, BetaStep: 5,
		Serpentine: true,
	}
	points, err := GenerateFiveHoleSnakePoints(layout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// 第二行（β=5，奇数行）应为降序 10,5,0
	for i, wantAlpha := range []float64{10, 5, 0} {
		got := points[3+i].Coordinates["α"]
		if got != wantAlpha {
			t.Fatalf("serpentine row 2 idx %d: expected α=%v, got %v", i, wantAlpha, got)
		}
	}
	// 首行与末行仍为升序
	if points[0].Coordinates["α"] != 0 || points[2].Coordinates["α"] != 10 {
		t.Fatalf("serpentine first row should be ascending, got %v, %v", points[0].Coordinates["α"], points[2].Coordinates["α"])
	}
}

// =====================================================================
// spec-five-hole-angle-convert：α 轴角度换算开关（angleConvert）测试
// =====================================================================
//
// 断言精度约定（spec）：黄金表"1 位小数"列为断言基准；测试侧独立计算期望值
// 后同样 roundTo1Decimal 再比较——对齐 seven_hole_points_test.go 既有模式，
// 高精度中间值存在 ±0.001 量级浮点噪声，禁止做 exact 断言。

// TestConvertFiveHoleAlpha 【P0】换算辅助函数黄金用例
//
// 测试前置：spec 黄金表 7 组 (α, β) 输入
// 测试步骤：调 convertFiveHoleAlpha 并 roundTo1Decimal
// 期待结果：与 spec"α' (1 位小数)"列一致；β=0 不改变 α、α=0 恒为 0、负数对称
func TestConvertFiveHoleAlpha(t *testing.T) {
	cases := []struct {
		name     string
		alpha    float64
		beta     float64
		expected float64
	}{
		{"30/30", 30, 30, 26.6},
		{"30/25", 30, 25, 27.6},
		{"25/30", 25, 30, 22.0},
		{"30/10", 30, 10, 29.6},
		{"30/0", 30, 0, 30.0},
		{"0/30", 0, 30, 0.0},
		{"-30/30", -30, 30, -26.6},
	}
	for _, tc := range cases {
		got := roundTo1Decimal(convertFiveHoleAlpha(tc.alpha, tc.beta))
		if got != tc.expected {
			t.Errorf("%s: convertFiveHoleAlpha(%v, %v) round1=%v, want %v",
				tc.name, tc.alpha, tc.beta, got, tc.expected)
		}
	}
}

// TestGenerateFiveHoleSnakePointsAngleConvert 【P0】开关开启：α 换算、β 与 ID/顺序不变
//
// 测试前置：网格 α ∈ {-30,0,30}（步 30）、β ∈ {0,30}（步 30），AngleConvert=true
// 测试步骤：调 GenerateFiveHoleSnakePoints
// 期待结果：β 慢轴（外层）→ 第一行 β=0（α 不变），第二行 β=30（α 换算 26.6/-26.6）；
//
//	β 全部不变，ID 连续 1..6
func TestGenerateFiveHoleSnakePointsAngleConvert(t *testing.T) {
	layout := FiveHolePointLayout{
		AlphaMin: -30, AlphaMax: 30, AlphaStep: 30,
		BetaMin: 0, BetaMax: 30, BetaStep: 30,
		AngleConvert: true,
	}
	points, err := GenerateFiveHoleSnakePoints(layout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []struct {
		id    int
		alpha float64
		beta  float64
	}{
		{1, -30, 0}, {2, 0, 0}, {3, 30, 0}, // β=0：换算不改变 α
		{4, -26.6, 30}, {5, 0, 30}, {6, 26.6, 30}, // β=30：α 换算（spec 黄金值）
	}
	if len(points) != len(want) {
		t.Fatalf("expected %d points, got %d", len(want), len(points))
	}
	for i, w := range want {
		p := points[i]
		if p.ID != w.id {
			t.Errorf("point %d: expected ID=%d, got %d", i, w.id, p.ID)
		}
		if p.Coordinates["α"] != w.alpha {
			t.Errorf("point %d: expected α=%v, got %v", i, w.alpha, p.Coordinates["α"])
		}
		if p.Coordinates["β"] != w.beta {
			t.Errorf("point %d: expected β=%v, got %v", i, w.beta, p.Coordinates["β"])
		}
	}
}

// TestGenerateFiveHoleSnakePointsAngleConvertOff 【P0】开关关闭（默认）：回归，输出与现状逐点一致
//
// 测试前置：与 AngleConvert 测试同一网格，不设 AngleConvert（零值 false）
// 测试步骤：调 GenerateFiveHoleSnakePoints
// 期待结果：α 为原始网格角（无换算），保证旧请求体/旧配置行为不变
func TestGenerateFiveHoleSnakePointsAngleConvertOff(t *testing.T) {
	layout := FiveHolePointLayout{
		AlphaMin: -30, AlphaMax: 30, AlphaStep: 30,
		BetaMin: 0, BetaMax: 30, BetaStep: 30,
	}
	points, err := GenerateFiveHoleSnakePoints(layout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	wantAlpha := []float64{-30, 0, 30, -30, 0, 30}
	wantBeta := []float64{0, 0, 0, 30, 30, 30}
	if len(points) != 6 {
		t.Fatalf("expected 6 points, got %d", len(points))
	}
	for i := range points {
		if points[i].Coordinates["α"] != wantAlpha[i] {
			t.Errorf("point %d: expected α=%v, got %v", i, wantAlpha[i], points[i].Coordinates["α"])
		}
		if points[i].Coordinates["β"] != wantBeta[i] {
			t.Errorf("point %d: expected β=%v, got %v", i, wantBeta[i], points[i].Coordinates["β"])
		}
	}
}

// TestGenerateFiveHoleSnakePointsAngleConvert_TraversalOrderBetaSlowAlphaFast 【P0】遍历顺序锁定（R-Order）
//
// 测试前置：3 个 α × 3 个 β 网格，AngleConvert=true（raster 与 serpentine 两组）
// 测试步骤：调 GenerateFiveHoleSnakePoints，按行（每行 alphaCount 个点）检查
// 期待结果：β 为慢轴（外层）——行间 β 严格递增、行内 β 恒定；α 为快轴（内层）——
//
//	每行覆盖全区间（raster 升序；serpentine 奇数行降序）；
//	换算保持 α 单调（dα'/dα = cosβ > 0），不因开关改变循环结构
func TestGenerateFiveHoleSnakePointsAngleConvert_TraversalOrderBetaSlowAlphaFast(t *testing.T) {
	const alphaCount = 3
	layout := FiveHolePointLayout{
		AlphaMin: -30, AlphaMax: 30, AlphaStep: 30,
		BetaMin: 0, BetaMax: 30, BetaStep: 15, // β: 0,15,30
		AngleConvert: true,
	}
	points, err := GenerateFiveHoleSnakePoints(layout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(points) != 9 {
		t.Fatalf("expected 9 points, got %d", len(points))
	}

	// β 慢轴：行间严格递增、行内恒定（β 步进只发生在整行 α 扫完之后）
	for row := 0; row < 3; row++ {
		beta := points[row*alphaCount].Coordinates["β"]
		if row > 0 && beta <= points[(row-1)*alphaCount].Coordinates["β"] {
			t.Fatalf("row %d β=%v 未严格递增（β 应为慢轴/外层）", row, beta)
		}
		for col := 1; col < alphaCount; col++ {
			if points[row*alphaCount+col].Coordinates["β"] != beta {
				t.Fatalf("row %d col %d β 与行首不一致（α 应为快轴/内层）", row, col)
			}
		}
	}

	// raster 行内换算后 α 仍升序（cosβ>0 保证单调）
	for row := 0; row < 3; row++ {
		for col := 1; col < alphaCount; col++ {
			if points[row*alphaCount+col].Coordinates["α"] <= points[row*alphaCount+col-1].Coordinates["α"] {
				t.Fatalf("raster row %d α 未升序: %v", row, points[row*alphaCount+col].Coordinates["α"])
			}
		}
	}

	// serpentine：奇数行（第二行 β=15）α 降序，且换算 α 集合与 raster 同行一致（仅顺序不同）
	serp := layout
	serp.Serpentine = true
	sPoints, err := GenerateFiveHoleSnakePoints(serp)
	if err != nil {
		t.Fatalf("unexpected error (serpentine): %v", err)
	}
	for col := 1; col < alphaCount; col++ {
		if sPoints[alphaCount+col].Coordinates["α"] >= sPoints[alphaCount+col-1].Coordinates["α"] {
			t.Fatalf("serpentine odd row α 未降序: col %d", col)
		}
	}
	for col := 0; col < alphaCount; col++ {
		a := sPoints[alphaCount+col].Coordinates["α"]
		b := sPoints[alphaCount+col].Coordinates["β"]
		found := false
		for c2 := 0; c2 < alphaCount; c2++ {
			if points[alphaCount+c2].Coordinates["α"] == a && points[alphaCount+c2].Coordinates["β"] == b {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("serpentine row2 (%v,%v) 不在 raster row2 集合中（应仅顺序不同）", a, b)
		}
	}
}

// TestGenerateFiveHoleSnakePointsAngleConvert_AngleRangeGuard 【P1】α/β 范围触及 ±90° 时防呆拒绝
//
// 测试前置：分别构造 α 或 β 触及 ±90° 的网格，AngleConvert=true；对照组关闭开关
// 测试步骤：分别调 GenerateFiveHoleSnakePoints
// 期待结果：开启时返回明确错误（含轴名与"90"）且不产出点位；关闭时同区间合法（现状不变）
func TestGenerateFiveHoleSnakePointsAngleConvert_AngleRangeGuard(t *testing.T) {
	base := FiveHolePointLayout{
		AlphaMin: -30, AlphaMax: 30, AlphaStep: 30,
		BetaMin: 0, BetaMax: 30, BetaStep: 30,
		AngleConvert: true,
	}
	// 同构防呆：α 与 β 任一轴越界都必须被拦截（cosβ 在 ±90° 同样退化/变号）
	cases := []struct {
		name   string
		mutate func(*FiveHolePointLayout)
		want   string
	}{
		{"alpha 触及 ±90", func(l *FiveHolePointLayout) { l.AlphaMin, l.AlphaMax = -90, 90 }, "α 范围"},
		{"beta 触及 ±90", func(l *FiveHolePointLayout) { l.BetaMin, l.BetaMax = -90, 90 }, "β 范围"},
	}
	for _, tc := range cases {
		layout := base
		tc.mutate(&layout)
		points, err := GenerateFiveHoleSnakePoints(layout)
		if err == nil {
			t.Fatalf("%s: expected error with angleConvert, got points=%v", tc.name, points)
		}
		if !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: error should mention %q, got %q", tc.name, tc.want, err.Error())
		}

		off := layout
		off.AngleConvert = false
		if _, err := GenerateFiveHoleSnakePoints(off); err != nil {
			t.Fatalf("%s: angleConvert=false should allow |·|≥90 (现状不变), got %v", tc.name, err)
		}
	}
}
