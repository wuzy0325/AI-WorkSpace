package usecase

import (
	"strings"
	"testing"

	"wind-daq/services/api-go/internal/core/calibration"
)

// =====================================================================
// spec Task 10 测试：CalibrationManager.PreviewFiveHolePoints
// =====================================================================
//
// 设计要点（plan Slice B4 / spec R-4、R-6）：
//   - 五孔蛇形点位生成原本在 API 层直接调 core.GenerateFiveHoleSnakePoints，
//     违反"API 不直接调用点位生成算法"边界。Task 10 将生成逻辑收口到
//     usecase 层，HTTP/Wails 共用同一入口，确保点位生成逻辑唯一、可观测。
//   - 方法签名与 PreviewSevenHolePoints 对称：纯计算、不启动采集、不创建
//     runtime、不写 CSV；仅依赖 core 公式，不访问 reader/motion/sink/store。
//   - 返回 []calibration.FiveHoleSnakePoint（与 core.GenerateFiveHoleSnakePoints
//     相同的元素类型），保留 bare-array 语义，避免破坏前端 JSON 契约。
//
// 测试覆盖（acceptance：method/invalid step/serpentine order 有行为测试）：
//  1. TestPreviewFiveHolePoints_DefaultRaster：默认 raster 模式每行 α 升序
//  2. TestPreviewFiveHolePoints_SerpentineReversesOddRows：蛇形奇数行 α 降序
//  3. TestPreviewFiveHolePoints_InvalidStep：步长 ≤ 0 透传 core 错误
//  4. TestPreviewFiveHolePoints_BareArrayResponse：返回 []FiveHoleSnakePoint
//     而非包装对象（HTTP 响应契约保持 bare array）
//  5. TestPreviewFiveHolePoints_PreservesCoordinates：点位 Coordinates 含 α/β
//  6. TestPreviewFiveHolePoints_NilManagerSafe：nil receiver 不 panic（防御性，
//     实际生产不会传 nil；与 PreviewSevenHolePoints 行为一致——纯计算不读 receiver 字段）

// TestPreviewFiveHolePoints_DefaultRaster 【P0】默认 raster 模式每行 α 升序
//
// 测试前置：构造 3×3 网格（α/β 均 0..10 步长 5），Serpentine=false
// 测试步骤：调 mgr.PreviewFiveHolePoints(layout)
// 期待结果：9 点；第二行（β=5）α 升序 [0,5,10]
func TestPreviewFiveHolePoints_DefaultRaster(t *testing.T) {
	mgr := newTestFiveHoleManager()
	layout := calibration.FiveHolePointLayout{
		AlphaMin: 0, AlphaMax: 10, AlphaStep: 5,
		BetaMin: 0, BetaMax: 10, BetaStep: 5,
	}

	points, err := mgr.PreviewFiveHolePoints(layout)
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

// TestPreviewFiveHolePoints_SerpentineReversesOddRows 【P0】蛇形奇数行 α 降序
//
// 测试前置：3×3 网格，Serpentine=true
// 测试步骤：调 mgr.PreviewFiveHolePoints(layout)
// 期待结果：第二行（β=5，奇数行）α 降序 [10,5,0]；首末行升序
func TestPreviewFiveHolePoints_SerpentineReversesOddRows(t *testing.T) {
	mgr := newTestFiveHoleManager()
	layout := calibration.FiveHolePointLayout{
		AlphaMin: 0, AlphaMax: 10, AlphaStep: 5,
		BetaMin: 0, BetaMax: 10, BetaStep: 5,
		Serpentine: true,
	}

	points, err := mgr.PreviewFiveHolePoints(layout)
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
	// 首行仍为升序
	if points[0].Coordinates["α"] != 0 || points[2].Coordinates["α"] != 10 {
		t.Fatalf("serpentine first row should be ascending, got %v, %v",
			points[0].Coordinates["α"], points[2].Coordinates["α"])
	}
}

// TestPreviewFiveHolePoints_InvalidStep 【P0】步长 ≤ 0 透传 core 错误
//
// 测试前置：AlphaStep=0（core.GenerateFiveHoleSnakePoints 返回 "step must be positive"）
// 测试步骤：调 mgr.PreviewFiveHolePoints(layout)
// 期待结果：返回非 nil 错误，错误信息包含 "step"
func TestPreviewFiveHolePoints_InvalidStep(t *testing.T) {
	mgr := newTestFiveHoleManager()
	layout := calibration.FiveHolePointLayout{
		AlphaMin: 0, AlphaMax: 10, AlphaStep: 0, // 非法
		BetaMin:  0, BetaMax: 10, BetaStep: 5,
	}

	points, err := mgr.PreviewFiveHolePoints(layout)
	if err == nil {
		t.Fatalf("expected error for invalid step, got nil; points=%v", points)
	}
	if !strings.Contains(err.Error(), "step") {
		t.Errorf("error should mention step, got %q", err.Error())
	}
	if points != nil {
		t.Errorf("points should be nil on error, got %v", points)
	}
}

// TestPreviewFiveHolePoints_BareArrayResponse 【P1】返回 []FiveHoleSnakePoint（bare array 语义）
//
// 测试前置：最小 1×1 网格
// 测试步骤：调 mgr.PreviewFiveHolePoints(layout)
// 期待结果：返回类型为 []calibration.FiveHoleSnakePoint，长度=1；
//           无包装对象（HTTP handler 直接 writeJSON(points)，前端收到 bare array）
func TestPreviewFiveHolePoints_BareArrayResponse(t *testing.T) {
	mgr := newTestFiveHoleManager()
	layout := calibration.FiveHolePointLayout{
		AlphaMin: 0, AlphaMax: 0, AlphaStep: 5,
		BetaMin: 0, BetaMax: 0, BetaStep: 5,
	}

	points, err := mgr.PreviewFiveHolePoints(layout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(points) != 1 {
		t.Fatalf("expected 1 point, got %d", len(points))
	}
	// bare array 语义：返回值本身是切片，不是 struct{Points []...}
	// HTTP handler 直接 writeJSON(w, 200, points)，前端收到 [...]
	if points[0].ID != 1 {
		t.Errorf("first point ID should be 1, got %d", points[0].ID)
	}
}

// TestPreviewFiveHolePoints_PreservesCoordinates 【P1】点位 Coordinates 含 α/β
//
// 测试前置：3×3 网格
// 测试步骤：调 mgr.PreviewFiveHolePoints(layout)
// 期待结果：每个点 Coordinates 含 α 和 β 两个键，值为 float64
func TestPreviewFiveHolePoints_PreservesCoordinates(t *testing.T) {
	mgr := newTestFiveHoleManager()
	layout := calibration.FiveHolePointLayout{
		AlphaMin: -5, AlphaMax: 5, AlphaStep: 5,
		BetaMin: -5, BetaMax: 5, BetaStep: 5,
	}

	points, err := mgr.PreviewFiveHolePoints(layout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(points) != 9 {
		t.Fatalf("expected 9 points, got %d", len(points))
	}
	for i, p := range points {
		alpha, ok := p.Coordinates["α"]
		if !ok {
			t.Errorf("point %d missing α", i)
			continue
		}
		if _, ok := p.Coordinates["β"]; !ok {
			t.Errorf("point %d missing β", i)
		}
		// α 应在 [-5, 5] 范围内
		if alpha < -5 || alpha > 5 {
			t.Errorf("point %d α=%v out of [-5,5]", i, alpha)
		}
	}
}

// TestPreviewFiveHolePoints_NilManagerSafe 【P2】nil receiver 不 panic
//
// 测试前置：构造 nil *CalibrationManager
// 测试步骤：调 nilMgr.PreviewFiveHolePoints(layout)
// 期待结果：不 panic，正常返回点位（纯计算不读 receiver 字段）
//
// 设计依据：PreviewSevenHolePoints 也是 nil-safe——纯计算方法不依赖
// manager 内部状态（reader/motion/sink/store），nil receiver 调用
// 仅在方法访问 m.xxx 字段时 panic。
func TestPreviewFiveHolePoints_NilManagerSafe(t *testing.T) {
	var nilMgr *CalibrationManager
	layout := calibration.FiveHolePointLayout{
		AlphaMin: 0, AlphaMax: 0, AlphaStep: 5,
		BetaMin: 0, BetaMax: 0, BetaStep: 5,
	}

	// 不应 panic
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("nil receiver should not panic, got %v", r)
		}
	}()

	points, err := nilMgr.PreviewFiveHolePoints(layout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(points) != 1 {
		t.Errorf("expected 1 point from nil manager, got %d", len(points))
	}
}

// newTestFiveHoleManager 构造纯计算用的 CalibrationManager。
//
// PreviewFiveHolePoints 不依赖 reader/motion/sink/store 等运行时注入，
// 这些参数传 nil 即可——与 server_seven_hole_preview_test.go 中
// newTestCalibrationManager 保持一致。
func newTestFiveHoleManager() *CalibrationManager {
	return NewCalibrationManager(nil, nil, nil, nil)
}
