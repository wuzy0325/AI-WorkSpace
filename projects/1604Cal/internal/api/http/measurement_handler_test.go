package http

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"cal1604/internal/api/dto"
	"cal1604/internal/application/deviceconnect"
	"cal1604/internal/config"
)

type measurementStateResponse struct {
	State string `json:"state"`
}

type measurementPointResponse struct {
	ID             string  `json:"id"`
	Index          int     `json:"index"`
	TargetPressure float64 `json:"targetPressure"`
	Direction      string  `json:"direction"`
	Status         string  `json:"status"`
}

func TestMeasurementStartCreatesWorkflowSession(t *testing.T) {
	router, _ := newSessionRouterWithMeasureDriverAndFakeDriver(t)

	configReq := httptest.NewRequest(http.MethodPost, "/api/v1/config/measurement", bytes.NewReader([]byte(`{
		"minPressure": 0,
		"maxPressure": 20,
		"pointCount": 3,
		"precision": 2,
		"averageCount": 1,
		"stableDurationMs": 1000,
		"precisionLevel": 0.05,
		"pressureMode": "single",
		"controlMode": "manual"
	}`)))
	configReq.Header.Set("Content-Type", "application/json")
	configRec := httptest.NewRecorder()
	router.ServeHTTP(configRec, configReq)
	if configRec.Code != http.StatusOK {
		t.Fatalf("expected config update status 200, got %d", configRec.Code)
	}

	generateReq := httptest.NewRequest(http.MethodPost, "/api/v1/measurement/points/generate", nil)
	generateRec := httptest.NewRecorder()
	router.ServeHTTP(generateRec, generateReq)
	if generateRec.Code != http.StatusOK {
		t.Fatalf("expected points generate status 200, got %d", generateRec.Code)
	}

	// StartWorkflow 完成后状态变为 ready，自动采集是异步的
	startState := callMeasurementStateEndpoint(t, router, http.MethodPost, "/api/v1/measurement/start", `{"channels":[1,2]}`)
	if startState != "ready" {
		t.Fatalf("expected start state ready, got %s", startState)
	}

	if state := callMeasurementStateEndpoint(t, router, http.MethodGet, "/api/v1/measurement/state", ""); state != "ready" {
		t.Fatalf("expected current measurement state ready, got %s", state)
	}

	listReq := httptest.NewRequest(http.MethodGet, "/api/v1/measurement/points", nil)
	listRec := httptest.NewRecorder()
	router.ServeHTTP(listRec, listReq)
	if listRec.Code != http.StatusOK {
		t.Fatalf("expected points list status 200, got %d", listRec.Code)
	}

	var listResp dto.Response[[]measurementPointResponse]
	if err := json.NewDecoder(listRec.Body).Decode(&listResp); err != nil {
		t.Fatalf("decode measurement point list response: %v", err)
	}
	if len(listResp.Data) != 3 {
		t.Fatalf("expected 3 measurement points, got %d", len(listResp.Data))
	}

	if state := callMeasurementStateEndpoint(t, router, http.MethodPost, "/api/v1/measurement/stop", ""); state != "idle" {
		t.Fatalf("expected stop state idle, got %s", state)
	}
}

func TestMeasurementStartRejectsEmptyChannels(t *testing.T) {
	router := newSessionRouterWithMeasureDriver(t)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/measurement/start", bytes.NewReader([]byte(`{"channels":[]}`)))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400 for empty channels, got %d", rec.Code)
	}
}

func TestMeasurementStartRequiresBoundMeasureDevice(t *testing.T) {
	router := NewRouter()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/measurement/start", bytes.NewReader([]byte(`{"channels":[1]}`)))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("expected status 409 when measure device not bound, got %d", rec.Code)
	}
}

func TestMeasurementStartRequiresGeneratedPoints(t *testing.T) {
	router := newSessionRouterWithMeasureDriver(t)

	// handler 内部会调用 GeneratePressurePoints 自动生成测点，不再返回 400
	req := httptest.NewRequest(http.MethodPost, "/api/v1/measurement/start", bytes.NewReader([]byte(`{"channels":[1]}`)))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200 when points auto-generated, got %d", rec.Code)
	}
}

func TestMeasurementGeneratePointsEndpointUsesMeasurementConfig(t *testing.T) {
	appCfg := config.Default()
	router := NewRouterWithRuntimeConfig(
		nil,
		deviceconnect.DefaultConfig(),
		CalibrationRuntimeConfig{},
		"",
		appCfg,
	)

	updatePayload := []byte(`{
		"minPressure": 0,
		"maxPressure": 100,
		"pointCount": 5,
		"precision": 2,
		"averageCount": 1,
		"stableDurationMs": 5000,
		"precisionLevel": 0.05,
		"pressureMode": "roundTrip",
		"controlMode": "auto"
	}`)

	configReq := httptest.NewRequest(http.MethodPost, "/api/v1/config/measurement", bytes.NewReader(updatePayload))
	configReq.Header.Set("Content-Type", "application/json")
	configRec := httptest.NewRecorder()
	router.ServeHTTP(configRec, configReq)

	if configRec.Code != http.StatusOK {
		t.Fatalf("expected config update status 200, got %d", configRec.Code)
	}

	generateReq := httptest.NewRequest(http.MethodPost, "/api/v1/measurement/points/generate", nil)
	generateRec := httptest.NewRecorder()
	router.ServeHTTP(generateRec, generateReq)

	if generateRec.Code != http.StatusOK {
		t.Fatalf("expected generate points status 200, got %d", generateRec.Code)
	}

	var resp dto.Response[[]measurementPointResponse]
	if err := json.NewDecoder(generateRec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode measurement points response: %v", err)
	}

	// roundTrip 生成完整正程+回程：5 forward + 5 backward = 10 points
	if len(resp.Data) != 10 {
		t.Fatalf("expected 10 measurement points (5 forward + 5 backward), got %d", len(resp.Data))
	}

	if resp.Data[0].Direction != "forward" || resp.Data[len(resp.Data)-1].Direction != "backward" {
		t.Fatalf("unexpected point directions: %+v", resp.Data)
	}

	// verify forward: [0, 25, 50, 75, 100], backward: [100, 75, 50, 25, 0]
	expected := []float64{0, 25, 50, 75, 100, 100, 75, 50, 25, 0}
	for i, exp := range expected {
		if resp.Data[i].TargetPressure != exp {
			t.Fatalf("points[%d].TargetPressure = %v, want %v", i, resp.Data[i].TargetPressure, exp)
		}
	}

	listReq := httptest.NewRequest(http.MethodGet, "/api/v1/measurement/points", nil)
	listRec := httptest.NewRecorder()
	router.ServeHTTP(listRec, listReq)

	if listRec.Code != http.StatusOK {
		t.Fatalf("expected list points status 200, got %d", listRec.Code)
	}

	var listResp dto.Response[[]measurementPointResponse]
	if err := json.NewDecoder(listRec.Body).Decode(&listResp); err != nil {
		t.Fatalf("decode listed measurement points response: %v", err)
	}

	if len(listResp.Data) != len(resp.Data) {
		t.Fatalf("expected listed points count %d, got %d", len(resp.Data), len(listResp.Data))
	}
}

func TestMeasurementGeneratePointsRejectsInvalidConfig(t *testing.T) {
	appCfg := config.Default()
	router := NewRouterWithRuntimeConfig(
		nil,
		deviceconnect.DefaultConfig(),
		CalibrationRuntimeConfig{},
		"",
		appCfg,
	)

	invalidPayload := []byte(`{
		"minPressure": 0,
		"maxPressure": 100,
		"pointCount": 1,
		"precision": 2,
		"averageCount": 1,
		"stableDurationMs": 5000,
		"precisionLevel": 0.05,
		"pressureMode": "single",
		"controlMode": "auto"
	}`)

	configReq := httptest.NewRequest(http.MethodPost, "/api/v1/config/measurement", bytes.NewReader(invalidPayload))
	configReq.Header.Set("Content-Type", "application/json")
	configRec := httptest.NewRecorder()
	router.ServeHTTP(configRec, configReq)

	if configRec.Code != http.StatusBadRequest {
		t.Fatalf("expected invalid config status 400, got %d", configRec.Code)
	}
}

// TestMeasurementPendingEndpointsShapeWhenIdle 验证页面刷新恢复用的两个
// 查询端点在空闲时返回规范形状（pending=false、无报警详情）。
func TestMeasurementPendingEndpointsShapeWhenIdle(t *testing.T) {
	router := NewRouter()

	alarmReq := httptest.NewRequest(http.MethodGet, "/api/v1/measurement/alarm/pending", nil)
	alarmRec := httptest.NewRecorder()
	router.ServeHTTP(alarmRec, alarmReq)
	if alarmRec.Code != http.StatusOK {
		t.Fatalf("expected alarm pending status 200, got %d", alarmRec.Code)
	}

	var alarmResp dto.Response[struct {
		Pending bool             `json:"pending"`
		Alarm   *measurementAlarmDTO `json:"alarm"`
	}]
	if err := json.NewDecoder(alarmRec.Body).Decode(&alarmResp); err != nil {
		t.Fatalf("decode alarm pending response: %v", err)
	}
	if alarmResp.Data.Pending {
		t.Fatal("expected no alarm pending on fresh router")
	}
	if alarmResp.Data.Alarm != nil {
		t.Fatalf("expected nil alarm detail when not pending, got %+v", alarmResp.Data.Alarm)
	}

	timeoutReq := httptest.NewRequest(http.MethodGet, "/api/v1/measurement/stability-timeout/pending", nil)
	timeoutRec := httptest.NewRecorder()
	router.ServeHTTP(timeoutRec, timeoutReq)
	if timeoutRec.Code != http.StatusOK {
		t.Fatalf("expected stability timeout pending status 200, got %d", timeoutRec.Code)
	}

	var timeoutResp dto.Response[struct {
		Pending    bool `json:"pending"`
		PointIndex int  `json:"pointIndex"`
	}]
	if err := json.NewDecoder(timeoutRec.Body).Decode(&timeoutResp); err != nil {
		t.Fatalf("decode stability timeout pending response: %v", err)
	}
	if timeoutResp.Data.Pending || timeoutResp.Data.PointIndex != 0 {
		t.Fatalf("expected no stability timeout pending, got %+v", timeoutResp.Data)
	}
}

type measurementAlarmDTO struct {
	PointID           string  `json:"pointId"`
	DeviceID          string  `json:"deviceId,omitempty"`
	TargetPressure    float64 `json:"targetPressure"`
	ActualPressure    float64 `json:"actualPressure"`
	Threshold         float64 `json:"threshold"`
	MaxDeviation      float64 `json:"maxDeviation"`
	OverLimitChannels []int   `json:"overLimitChannels"`
}

func callMeasurementStateEndpoint(t *testing.T, router http.Handler, method, path, body string) string {
	t.Helper()

	var reqBody *bytes.Reader
	if body == "" {
		reqBody = bytes.NewReader(nil)
	} else {
		reqBody = bytes.NewReader([]byte(body))
	}

	req := httptest.NewRequest(method, path, reqBody)
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("request %s %s failed with status %d", method, path, rec.Code)
	}

	var resp dto.Response[measurementStateResponse]
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode measurement state response: %v", err)
	}

	return resp.Data.State
}
