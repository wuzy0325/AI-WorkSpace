package api

import (
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"wind-daq/services/api-go/internal/core/calibration"
	"wind-daq/services/api-go/internal/usecase"
)

// =====================================================================
// spec Task 12 测试：handleSevenHolePreview handler + sanitize NaN 清洗
// =====================================================================
//
// 测试前置：
//   - CalibrationManager 通过 NewCalibrationManager(nil, nil, nil, nil) 构造，
//     PreviewSevenHolePoints 是纯计算方法（不依赖 reader/motion/sink/store 注入）
//   - httptest.NewRecorder + http.NewRequest 模拟 HTTP 请求
//   - 完整模式 715 点 = 169 内区 + 546 外区；数据集模式 481 点 = 169 内区 + 312 外区

// newTestCalibrationManager 构造纯计算用的 CalibrationManager。
//
// PreviewSevenHolePoints 不依赖 reader/motion/sink/store 等运行时注入，
// 这些参数传 nil 即可——其他需要这些依赖的方法（Start/Stop/Pause 等）
// 不在本测试覆盖范围内。
func newTestCalibrationManager() *usecase.CalibrationManager {
	return usecase.NewCalibrationManager(nil, nil, nil, nil)
}

// TestHandleSevenHolePreview_FullMode 【P0】完整模式预览返回 715 点
//
// 测试前置：构造完整模式 SevenHoleConfig（内区 [-30,30] 步长 5°；外区 θ [30,60] 步长 5°、φ [0,355] 步长 5°）
//
// 期待结果：HTTP 200，totalCount=715、innerCount=169、outerCount=546
func TestHandleSevenHolePreview_FullMode(t *testing.T) {
	mgr := newTestCalibrationManager()
	body := `{
		"mode": "full",
		"innerAlphaMin": -30, "innerAlphaMax": 30, "innerAlphaStep": 5,
		"innerBetaMin": -30, "innerBetaMax": 30, "innerBetaStep": 5,
		"outerThetaMin": 30, "outerThetaMax": 60, "outerThetaStep": 5,
		"outerPhiMin": 0, "outerPhiMax": 355, "outerPhiStep": 5,
		"serpentine": true
	}`

	req := httptest.NewRequest(http.MethodPost, "/api/calibration/sevenhole-preview", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handleSevenHolePreview(w, req, mgr)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp struct {
		Points     []json.RawMessage `json:"points"`
		TotalCount int               `json:"totalCount"`
		InnerCount int               `json:"innerCount"`
		OuterCount int               `json:"outerCount"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if resp.TotalCount != 715 {
		t.Errorf("totalCount should be 715, got %d", resp.TotalCount)
	}
	if resp.InnerCount != 169 {
		t.Errorf("innerCount should be 169, got %d", resp.InnerCount)
	}
	if resp.OuterCount != 546 {
		t.Errorf("outerCount should be 546, got %d", resp.OuterCount)
	}
	if len(resp.Points) != 715 {
		t.Errorf("points length should be 715, got %d", len(resp.Points))
	}
}

// TestHandleSevenHolePreview_DatasetMode 【P0】数据集模式预览返回 481 点
//
// 测试前置：mode=dataset，外区 θ 取硬编码 [30,35,40,45]，每扇区 60° 跨度步长 5° → 13 点/扇区 × 6 扇区 = 312 外区点
//
// 期待结果：HTTP 200，totalCount=481、innerCount=169、outerCount=312
func TestHandleSevenHolePreview_DatasetMode(t *testing.T) {
	mgr := newTestCalibrationManager()
	body := `{
		"mode": "dataset",
		"innerAlphaMin": -30, "innerAlphaMax": 30, "innerAlphaStep": 5,
		"innerBetaMin": -30, "innerBetaMax": 30, "innerBetaStep": 5,
		"outerPhiMin": 0, "outerPhiMax": 355, "outerPhiStep": 5,
		"serpentine": true
	}`

	req := httptest.NewRequest(http.MethodPost, "/api/calibration/sevenhole-preview", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handleSevenHolePreview(w, req, mgr)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp struct {
		TotalCount int `json:"totalCount"`
		InnerCount int `json:"innerCount"`
		OuterCount int `json:"outerCount"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if resp.TotalCount != 481 {
		t.Errorf("totalCount should be 481, got %d", resp.TotalCount)
	}
	if resp.InnerCount != 169 {
		t.Errorf("innerCount should be 169, got %d", resp.InnerCount)
	}
	if resp.OuterCount != 312 {
		t.Errorf("outerCount should be 312, got %d", resp.OuterCount)
	}
}

// TestHandleSevenHolePreview_InvalidConfig 【P0】配置非法（步长 ≤ 0）返回 400
func TestHandleSevenHolePreview_InvalidConfig(t *testing.T) {
	mgr := newTestCalibrationManager()
	// innerAlphaStep=0 → GenerateSevenHolePoints 返回"内区步长必须 > 0"
	body := `{
		"mode": "full",
		"innerAlphaMin": -30, "innerAlphaMax": 30, "innerAlphaStep": 0,
		"innerBetaMin": -30, "innerBetaMax": 30, "innerBetaStep": 5
	}`

	req := httptest.NewRequest(http.MethodPost, "/api/calibration/sevenhole-preview", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handleSevenHolePreview(w, req, mgr)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid config, got %d: %s", w.Code, w.Body.String())
	}
	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal error response: %v", err)
	}
	if resp["success"] != false {
		t.Errorf("error response should have success=false, got %v", resp["success"])
	}
	if errMsg, ok := resp["error"].(string); !ok || !strings.Contains(errMsg, "步长") {
		t.Errorf("error message should mention 步长, got %v", resp["error"])
	}
}

// TestHandleSevenHolePreview_RangeInvalid 【P1】范围 min > max 返回 400
func TestHandleSevenHolePreview_RangeInvalid(t *testing.T) {
	mgr := newTestCalibrationManager()
	body := `{
		"mode": "full",
		"innerAlphaMin": 30, "innerAlphaMax": -30, "innerAlphaStep": 5,
		"innerBetaMin": -30, "innerBetaMax": 30, "innerBetaStep": 5
	}`

	req := httptest.NewRequest(http.MethodPost, "/api/calibration/sevenhole-preview", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handleSevenHolePreview(w, req, mgr)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for min > max, got %d: %s", w.Code, w.Body.String())
	}
}

// TestHandleSevenHolePreview_MethodNotAllowed 【P1】GET 方法返回 405
func TestHandleSevenHolePreview_MethodNotAllowed(t *testing.T) {
	mgr := newTestCalibrationManager()
	req := httptest.NewRequest(http.MethodGet, "/api/calibration/sevenhole-preview", nil)
	w := httptest.NewRecorder()

	handleSevenHolePreview(w, req, mgr)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405 for GET, got %d", w.Code)
	}
}

// TestHandleSevenHolePreview_MalformedJSON 【P1】JSON 解析失败返回 400
func TestHandleSevenHolePreview_MalformedJSON(t *testing.T) {
	mgr := newTestCalibrationManager()
	req := httptest.NewRequest(http.MethodPost, "/api/calibration/sevenhole-preview", strings.NewReader("{invalid json"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handleSevenHolePreview(w, req, mgr)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for malformed JSON, got %d", w.Code)
	}
}

// TestHandleSevenHolePreview_PointStructure 【P1】点位结构含 Coordinates/MotionCoordinates/Region/Sector
//
// 验证 sanitize 后的点位 JSON 结构：内区点 Region="inner" Sector=7，外区点 Region="outer" Sector=1~6
func TestHandleSevenHolePreview_PointStructure(t *testing.T) {
	mgr := newTestCalibrationManager()
	// 用最小配置生成少量点位便于断言：内区 [-5,5] 步长 5° → 3×3=9 内区点
	body := `{
		"mode": "full",
		"innerAlphaMin": -5, "innerAlphaMax": 5, "innerAlphaStep": 5,
		"innerBetaMin": -5, "innerBetaMax": 5, "innerBetaStep": 5,
		"outerThetaMin": 30, "outerThetaMax": 30, "outerThetaStep": 5,
		"outerPhiMin": 0, "outerPhiMax": 0, "outerPhiStep": 5,
		"serpentine": false
	}`

	req := httptest.NewRequest(http.MethodPost, "/api/calibration/sevenhole-preview", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handleSevenHolePreview(w, req, mgr)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp struct {
		Points []struct {
			ID                int            `json:"id"`
			Coordinates       map[string]any `json:"coordinates"`
			MotionCoordinates map[string]any `json:"motionCoordinates"`
			Region            string         `json:"region"`
			Sector            int            `json:"sector"`
		} `json:"points"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(resp.Points) == 0 {
		t.Fatal("expected at least 1 point")
	}

	// 第一个点应为内区点（内区在前）
	first := resp.Points[0]
	if first.Region != "inner" {
		t.Errorf("first point region should be inner, got %s", first.Region)
	}
	if first.Sector != 7 {
		t.Errorf("inner point sector should be 7, got %d", first.Sector)
	}
	if first.Coordinates == nil {
		t.Error("first point coordinates should not be nil")
	}
	if first.MotionCoordinates == nil {
		t.Error("inner point motionCoordinates should not be nil")
	}
	// 内区点 Coordinates 应含 α 和 β
	if _, ok := first.Coordinates["α"]; !ok {
		t.Errorf("inner point coordinates should contain α, got %v", first.Coordinates)
	}
	if _, ok := first.Coordinates["β"]; !ok {
		t.Errorf("inner point coordinates should contain β, got %v", first.Coordinates)
	}

	// 验证 JSON 字段为 number 类型而非 string（NaN 会变成 "NaN" 字符串）
	for k, v := range first.Coordinates {
		switch v.(type) {
		case float64, nil:
			// 正常
		default:
			t.Errorf("coordinate %s should be float64 or nil, got %T: %v", k, v, v)
		}
	}
}

// TestSanitizeSevenHolePreview_NaNValues 【P0】NaN/Inf 值替换为 nil
//
// 测试前置：构造含 NaN/Inf 的 SevenHolePreviewResult
//
// 期待结果：sanitize 后 Coordinates 中 NaN/Inf 对应值为 nil
func TestSanitizeSevenHolePreview_NaNValues(t *testing.T) {
	result := calibration.SevenHolePreviewResult{
		Points: []calibration.CalPoint{
			{
				ID:                1,
				Coordinates:       map[string]float64{"α": math.NaN(), "β": 5.0, "θ": math.Inf(1)},
				MotionCoordinates: map[string]float64{"α": math.Inf(-1), "β": 0.0},
				Region:            "outer",
				Sector:            3,
			},
		},
		TotalCount: 1,
		InnerCount: 0,
		OuterCount: 1,
	}

	sanitized := sanitizeSevenHolePreview(result)
	points, ok := sanitized["points"].([]sanitizedCalPoint)
	if !ok {
		t.Fatalf("points should be []sanitizedCalPoint, got %T", sanitized["points"])
	}
	if len(points) != 1 {
		t.Fatalf("expected 1 point, got %d", len(points))
	}

	p := points[0]
	// α=NaN 应为 nil
	if v, ok := p.Coordinates["α"]; !ok || v != nil {
		t.Errorf("NaN α should be nil, got %v (ok=%v)", v, ok)
	}
	// β=5 应为 float64(5)
	if v, ok := p.Coordinates["β"]; !ok || v != 5.0 {
		t.Errorf("β=5 should be 5.0, got %v (ok=%v)", v, ok)
	}
	// θ=+Inf 应为 nil
	if v, ok := p.Coordinates["θ"]; !ok || v != nil {
		t.Errorf("+Inf θ should be nil, got %v (ok=%v)", v, ok)
	}
	// MotionCoordinates α=-Inf 应为 nil
	if v, ok := p.MotionCoordinates["α"]; !ok || v != nil {
		t.Errorf("-Inf α should be nil, got %v (ok=%v)", v, ok)
	}
	// MotionCoordinates β=0 应为 float64(0)
	if v, ok := p.MotionCoordinates["β"]; !ok || v != 0.0 {
		t.Errorf("β=0 should be 0.0, got %v (ok=%v)", v, ok)
	}

	// 验证可被 json.Marshal 序列化（NaN/Inf 会导致默认 Marshal 失败）
	if _, err := json.Marshal(sanitized); err != nil {
		t.Fatalf("sanitized result should be JSON-serializable: %v", err)
	}
}

// TestSanitizeFloatMap_NilInput 【P1】nil 输入返回 nil
func TestSanitizeFloatMap_NilInput(t *testing.T) {
	if got := sanitizeFloatMap(nil); got != nil {
		t.Errorf("nil input should return nil, got %v", got)
	}
}

// TestSanitizeFloatMap_EmptyInput 【P1】空 map 返回空 map
func TestSanitizeFloatMap_EmptyInput(t *testing.T) {
	got := sanitizeFloatMap(map[string]float64{})
	if got == nil {
		t.Error("empty map should return non-nil empty map")
	}
	if len(got) != 0 {
		t.Errorf("empty map should return empty map, got %v", got)
	}
}

// TestSanitizeFloatMap_NormalValues 【P1】正常值原样保留为 float64
func TestSanitizeFloatMap_NormalValues(t *testing.T) {
	input := map[string]float64{"α": 1.5, "β": -2.5, "θ": 30.0}
	got := sanitizeFloatMap(input)
	if len(got) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(got))
	}
	for k, want := range input {
		if v, ok := got[k]; !ok {
			t.Errorf("key %s missing", k)
		} else if v != want {
			t.Errorf("key %s should be %v, got %v", k, want, v)
		}
	}
}

// TestSanitizeSevenHolePreview_PreservesCounts 【P1】聚合统计字段保留
func TestSanitizeSevenHolePreview_PreservesCounts(t *testing.T) {
	result := calibration.SevenHolePreviewResult{
		Points:     []calibration.CalPoint{},
		TotalCount: 715,
		InnerCount: 169,
		OuterCount: 546,
	}
	sanitized := sanitizeSevenHolePreview(result)
	if sanitized["totalCount"] != 715 {
		t.Errorf("totalCount should be 715, got %v", sanitized["totalCount"])
	}
	if sanitized["innerCount"] != 169 {
		t.Errorf("innerCount should be 169, got %v", sanitized["innerCount"])
	}
	if sanitized["outerCount"] != 546 {
		t.Errorf("outerCount should be 546, got %v", sanitized["outerCount"])
	}
}
