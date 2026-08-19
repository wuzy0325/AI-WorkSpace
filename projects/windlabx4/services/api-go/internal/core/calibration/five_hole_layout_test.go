package calibration

import (
	"testing"
)

// 验证默认（Serpentine=false）按逐行 raster 扫描：每行 α 都从 AlphaMin 升序排列
func TestGenerateFiveHoleSnakePointsDefaultRaster(t *testing.T) {
	layout := FiveHolePointLayout{
		AlphaMin: 0, AlphaMax: 10, AlphaStep: 5, // 3 个 α：0,5,10
		BetaMin:  0, BetaMax: 10, BetaStep: 5,  // 3 个 β：0,5,10
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
		BetaMin:  0, BetaMax: 10, BetaStep: 5,
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