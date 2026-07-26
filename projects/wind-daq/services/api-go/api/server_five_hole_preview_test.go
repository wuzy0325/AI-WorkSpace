package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"wind-daq/services/api-go/internal/usecase"
)

// =====================================================================
// spec Task 10 测试：handleFiveHolePreview HTTP handler
// =====================================================================
//
// 测试前置：
//   - CalibrationManager 通过 NewCalibrationManager(nil, nil, nil, nil) 构造，
//     PreviewFiveHolePoints 是纯计算方法（不依赖 reader/motion/sink/store 注入）
//   - httptest.NewRecorder + http.NewRequest 模拟 HTTP 请求
//
// 验收覆盖（acceptance：HTTP 调用 manager usecase；method/invalid step/serpentine order
// 有行为测试；bare-array 响应不变）：
//  1. TestHandleFiveHolePreview_RasterOrder：raster 模式每行 α 升序
//  2. TestHandleFiveHolePreview_SerpentineOrder：蛇形奇数行 α 降序
//  3. TestHandleFiveHolePreview_InvalidStep：步长 ≤ 0 返回 400
//  4. TestHandleFiveHolePreview_BareArrayResponse：响应是 bare array（非 {points:[...]}）
//  5. TestHandleFiveHolePreview_MethodNotAllowed：GET 返回 405
//  6. TestHandleFiveHolePreview_MalformedJSON：JSON 解析失败返回 400

// newTestFiveHoleCalibrationManager 构造纯计算用的 CalibrationManager。
//
// PreviewFiveHolePoints 不依赖 reader/motion/sink/store 等运行时注入，
// 这些参数传 nil 即可——与 newTestCalibrationManager（七孔预览测试）一致。
func newTestFiveHoleCalibrationManager() *usecase.CalibrationManager {
	return usecase.NewCalibrationManager(nil, nil, nil, nil)
}

// TestHandleFiveHolePreview_RasterOrder 【P0】raster 模式每行 α 升序
//
// 测试前置：3×3 网格（α/β 均 0..10 步长 5），Serpentine=false
// 测试步骤：POST /api/calibration/fivehole 带 layout JSON
// 期待结果：HTTP 200，响应为 9 元素 bare array；第二行 α 升序 [0,5,10]
func TestHandleFiveHolePreview_RasterOrder(t *testing.T) {
	mgr := newTestFiveHoleCalibrationManager()
	body := `{
		"alphaMin": 0, "alphaMax": 10, "alphaStep": 5,
		"betaMin": 0, "betaMax": 10, "betaStep": 5
	}`

	req := httptest.NewRequest(http.MethodPost, "/api/calibration/fivehole", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handleFiveHolePreview(w, req, mgr)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// 响应应为 bare array（顶层是 [），而非 {points:[...]}
	trimmed := strings.TrimSpace(w.Body.String())
	if !strings.HasPrefix(trimmed, "[") {
		// Go 1.20 无内置 min（Go 1.21+），手动三元避免 vet 报错
		n := 50
		if len(trimmed) < n {
			n = len(trimmed)
		}
		t.Fatalf("response should be bare array (start with '['), got: %s", trimmed[:n])
	}

	var points []struct {
		ID          int                `json:"id"`
		Coordinates map[string]float64 `json:"coordinates"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &points); err != nil {
		t.Fatalf("unmarshal bare array: %v", err)
	}
	if len(points) != 9 {
		t.Fatalf("expected 9 points, got %d", len(points))
	}
	// 第二行（β=5）应为升序 0,5,10
	for i, wantAlpha := range []float64{0, 5, 10} {
		got := points[3+i].Coordinates["α"]
		if got != wantAlpha {
			t.Errorf("raster row 2 idx %d: expected α=%v, got %v", i, wantAlpha, got)
		}
	}
}

// TestHandleFiveHolePreview_SerpentineOrder 【P0】蛇形奇数行 α 降序
//
// 测试前置：3×3 网格，serpentine=true
// 测试步骤：POST /api/calibration/fivehole
// 期待结果：HTTP 200；第二行 α 降序 [10,5,0]；首行升序
func TestHandleFiveHolePreview_SerpentineOrder(t *testing.T) {
	mgr := newTestFiveHoleCalibrationManager()
	body := `{
		"alphaMin": 0, "alphaMax": 10, "alphaStep": 5,
		"betaMin": 0, "betaMax": 10, "betaStep": 5,
		"serpentine": true
	}`

	req := httptest.NewRequest(http.MethodPost, "/api/calibration/fivehole", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handleFiveHolePreview(w, req, mgr)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var points []struct {
		ID          int                `json:"id"`
		Coordinates map[string]float64 `json:"coordinates"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &points); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(points) != 9 {
		t.Fatalf("expected 9 points, got %d", len(points))
	}
	// 第二行（β=5，奇数行）应为降序 10,5,0
	for i, wantAlpha := range []float64{10, 5, 0} {
		got := points[3+i].Coordinates["α"]
		if got != wantAlpha {
			t.Errorf("serpentine row 2 idx %d: expected α=%v, got %v", i, wantAlpha, got)
		}
	}
	// 首行升序
	if points[0].Coordinates["α"] != 0 || points[2].Coordinates["α"] != 10 {
		t.Errorf("serpentine first row should be ascending, got %v, %v",
			points[0].Coordinates["α"], points[2].Coordinates["α"])
	}
}

// TestHandleFiveHolePreview_InvalidStep 【P0】步长 ≤ 0 返回 400
//
// 测试前置：alphaStep=0（core.GenerateFiveHoleSnakePoints 返回 "step must be positive"）
// 测试步骤：POST /api/calibration/fivehole
// 期待结果：HTTP 400；响应 success=false，error 含 "step"
func TestHandleFiveHolePreview_InvalidStep(t *testing.T) {
	mgr := newTestFiveHoleCalibrationManager()
	body := `{
		"alphaMin": 0, "alphaMax": 10, "alphaStep": 0,
		"betaMin": 0, "betaMax": 10, "betaStep": 5
	}`

	req := httptest.NewRequest(http.MethodPost, "/api/calibration/fivehole", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handleFiveHolePreview(w, req, mgr)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid step, got %d: %s", w.Code, w.Body.String())
	}
	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal error response: %v", err)
	}
	if resp["success"] != false {
		t.Errorf("error response should have success=false, got %v", resp["success"])
	}
	errMsg, _ := resp["error"].(string)
	if !strings.Contains(errMsg, "step") {
		t.Errorf("error message should mention step, got %q", errMsg)
	}
}

// TestHandleFiveHolePreview_BareArrayResponse 【P1】响应是 bare array
//
// 测试前置：最小 1×1 网格
// 测试步骤：POST /api/calibration/fivehole
// 期待结果：HTTP 200；响应顶层是 [，不是 {points:...}
//           spec Task 10 acceptance：bare-array 响应不变
func TestHandleFiveHolePreview_BareArrayResponse(t *testing.T) {
	mgr := newTestFiveHoleCalibrationManager()
	body := `{
		"alphaMin": 0, "alphaMax": 0, "alphaStep": 5,
		"betaMin": 0, "betaMax": 0, "betaStep": 5
	}`

	req := httptest.NewRequest(http.MethodPost, "/api/calibration/fivehole", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handleFiveHolePreview(w, req, mgr)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	trimmed := strings.TrimSpace(w.Body.String())
	if !strings.HasPrefix(trimmed, "[") {
		t.Fatalf("response should be bare array (start with '['), got: %s", trimmed)
	}
	if strings.HasPrefix(trimmed, "{") {
		t.Fatalf("response should NOT be wrapped object, got: %s", trimmed)
	}
}

// TestHandleFiveHolePreview_MethodNotAllowed 【P1】GET 方法返回 405
func TestHandleFiveHolePreview_MethodNotAllowed(t *testing.T) {
	mgr := newTestFiveHoleCalibrationManager()
	req := httptest.NewRequest(http.MethodGet, "/api/calibration/fivehole", nil)
	w := httptest.NewRecorder()

	handleFiveHolePreview(w, req, mgr)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405 for GET, got %d", w.Code)
	}
}

// TestHandleFiveHolePreview_MalformedJSON 【P1】JSON 解析失败返回 400
func TestHandleFiveHolePreview_MalformedJSON(t *testing.T) {
	mgr := newTestFiveHoleCalibrationManager()
	req := httptest.NewRequest(http.MethodPost, "/api/calibration/fivehole", strings.NewReader("{invalid json"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handleFiveHolePreview(w, req, mgr)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for malformed JSON, got %d", w.Code)
	}
}
