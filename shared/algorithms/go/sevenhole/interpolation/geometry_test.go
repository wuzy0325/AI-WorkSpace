package interpolation

import (
	"math"
	"testing"
)

func TestPointInPolygon(t *testing.T) {
	// Axis-aligned unit square scaled by 4 for deterministic signs.
	poly := []point2D{{0, 0}, {4, 0}, {4, 4}, {0, 4}}
	tests := []struct {
		name string
		x, y float64
		want int
	}{
		{"interior", 2, 2, 1},
		{"outside right", 5, 2, -1},
		{"outside left", -1, 2, -1},
		{"outside above", 2, 5, -1},
		{"horizontal bottom edge", 2, 0, 0},
		{"horizontal top edge", 2, 4, 0},
		// Vertex (0,0): the degenerate first segment (v0,v0) triggers the
		// boundary check (Python parity).
		{"vertex origin", 0, 0, 0},
		// Vertex (4,4): the following horizontal segment flags boundary.
		{"vertex corner", 4, 4, 0},
		// Ray-cast asymmetry on vertical edges (Python behavior preserved):
		// right edge classifies inside, left edge classifies outside.
		{"vertical right edge", 4, 2, 1},
		{"vertical left edge", 0, 2, -1},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := pointInPolygon(tc.x, tc.y, poly); got != tc.want {
				t.Errorf("pointInPolygon(%v,%v) = %d, want %d", tc.x, tc.y, got, tc.want)
			}
		})
	}
}

func TestPointInPolygonEmpty(t *testing.T) {
	if got := pointInPolygon(0, 0, nil); got != -1 {
		t.Errorf("empty polygon = %d, want -1", got)
	}
}

// TestPointInPolygon_VertexMatch 覆盖"测试点=多边形非极值顶点"边界条件：
// 射线法对非极值顶点会因前后两段都触发条件 2（y 等于端点 y）导致 inside
// 双重翻转，把顶点误判为外部。入口的顶点匹配前置检查命中后返回 0
// （on boundary），是自提取 PRB 反推场景的关键修复（θ=30 内边顶点）。
//
// 用菱形多边形构造 4 个"一坐标极值、另一坐标非极值"的顶点
// （左/右/上/下尖角），每个都应返回 0。
func TestPointInPolygon_VertexMatch(t *testing.T) {
	// 菱形：(2,0)下 (4,2)右 (2,4)上 (0,2)左
	diamond := []point2D{{2, 0}, {4, 2}, {2, 4}, {0, 2}}
	// 4 个顶点都是非极值顶点（一坐标极值 + 另一坐标中间值）
	for i, v := range diamond {
		if got := pointInPolygon(v.x, v.y, diamond); got != 0 {
			t.Errorf("顶点[%d]=(%v,%v) 返回 %d，期望 0（on boundary）", i, v.x, v.y, got)
		}
	}
	// 容差测试：1e-12 偏移仍应命中顶点匹配（gridEps=1e-9）
	for i, v := range diamond {
		if got := pointInPolygon(v.x+1e-12, v.y-1e-12, diamond); got != 0 {
			t.Errorf("顶点[%d]容差偏移=(%v,%v) 返回 %d，期望 0", i, v.x+1e-12, v.y-1e-12, got)
		}
	}
	// 内部点（菱形中心）仍应返回 1
	if got := pointInPolygon(2, 2, diamond); got != 1 {
		t.Errorf("菱形中心 (2,2) 返回 %d，期望 1（inside）", got)
	}
	// 外部点（菱形右上方）应返回 -1
	if got := pointInPolygon(5, 5, diamond); got != -1 {
		t.Errorf("外部点 (5,5) 返回 %d，期望 -1（outside）", got)
	}
}

func TestDedupPolygon(t *testing.T) {
	in := []point2D{{0, 0}, {1, 0}, {1, 0}, {2, 0}, {0, 0}, {3, 3}}
	want := []point2D{{0, 0}, {1, 0}, {2, 0}, {3, 3}}
	got := dedupPolygon(in)
	if len(got) != len(want) {
		t.Fatalf("dedupPolygon len = %d, want %d (%v)", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("dedupPolygon[%d] = %v, want %v", i, got[i], want[i])
		}
	}
}

// TestLocateInvertAB verifies the edge-equation location and inverse-distance
// inversion against a hand-computed distorted parallelogram
// (Python little_cal_ab, SKILL.md section 2.3).
func TestLocateInvertAB(t *testing.T) {
	// Parallelogram X1=(0,0) X2=(2,1) X3=(3,3) X4=(1,2); center (1.5,1.5).
	// Hand computation: all four distance ratios are 1/2, so the point inverts
	// to the cell center: a = a1+2.5, b = b1+bStep/2.
	quad := newDistortedQuad(
		point2D{0, 0}, point2D{2, 1}, point2D{3, 3}, point2D{1, 2},
		-10, -20, +gridStep,
	)
	a, b, found := locateInvertAB(1.5, 1.5, []distortedQuad{quad})
	if !found {
		t.Fatal("center point not located")
	}
	if math.Abs(a-(-7.5)) > 1e-12 || math.Abs(b-(-17.5)) > 1e-12 {
		t.Errorf("inversion = (%v,%v), want (-7.5,-17.5)", a, b)
	}

	// Quarter offset: point (0.75,0.75) is the (1/4,1/4) affine position of
	// the parallelogram -> a = a1+1.25, b = b1+1.25.
	a, b, found = locateInvertAB(0.75, 0.75, []distortedQuad{quad})
	if !found {
		t.Fatal("quarter point not located")
	}
	if math.Abs(a-(-8.75)) > 1e-12 || math.Abs(b-(-18.75)) > 1e-12 {
		t.Errorf("inversion = (%v,%v), want (-8.75,-18.75)", a, b)
	}

	// A point outside the cell is not found.
	if _, _, found := locateInvertAB(5, 5, []distortedQuad{quad}); found {
		t.Error("outside point must not be located")
	}
}

// TestLocateInvertABOuterDirection pins the outer-zone b direction (bStep=-5,
// Python big_cal_ab): b decreases from X1.b by the weighted ratio.
func TestLocateInvertABOuterDirection(t *testing.T) {
	quad := newDistortedQuad(
		point2D{0, 0}, point2D{2, 1}, point2D{3, 3}, point2D{1, 2},
		35, 120, -gridStep,
	)
	a, b, found := locateInvertAB(1.5, 1.5, []distortedQuad{quad})
	if !found {
		t.Fatal("center point not located")
	}
	if math.Abs(a-37.5) > 1e-12 || math.Abs(b-117.5) > 1e-12 {
		t.Errorf("inversion = (%v,%v), want (37.5,117.5)", a, b)
	}
}
