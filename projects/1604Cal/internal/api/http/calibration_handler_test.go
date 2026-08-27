package http

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"cal1604/internal/api/dto"
)

type calibrationPointResponse struct {
	Index          int     `json:"index"`
	TargetPressure float64 `json:"targetPressure"`
	Status         string  `json:"status"`
}

func TestCalibrationConfigAcceptsControlAndPressureMode(t *testing.T) {
	router := NewRouter()

	payload := map[string]any{
		"channels":       []int{1, 2},
		"pressurePoints": 5,
		"averageCount":   3,
		"minPressure":    0,
		"maxPressure":    100,
		"stableWaitMs":   3000,
		"controlMode":    "auto",
	}

	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/calibration/config", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	var resp dto.Response[map[string]string]
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if !resp.Success {
		t.Fatalf("expected success response, got %+v", resp)
	}

	generateReq := httptest.NewRequest(http.MethodPost, "/api/v1/calibration/points/generate", nil)
	generateRec := httptest.NewRecorder()
	router.ServeHTTP(generateRec, generateReq)

	if generateRec.Code != http.StatusOK {
		t.Fatalf("expected generate status 200, got %d", generateRec.Code)
	}

	var generateResp dto.Response[[]calibrationPointResponse]
	if err := json.NewDecoder(generateRec.Body).Decode(&generateResp); err != nil {
		t.Fatalf("decode generate response: %v", err)
	}

	if !generateResp.Success {
		t.Fatalf("expected generate success response, got %+v", generateResp)
	}

	if len(generateResp.Data) != 5 {
		t.Fatalf("expected 5 generated points, got %d", len(generateResp.Data))
	}
}

func TestCalibrationRoutesDoNotExposeMeasurementSessionEndpoints(t *testing.T) {
	router := NewRouter()
	testCases := []struct {
		name   string
		method string
		path   string
		body   []byte
	}{
		{name: "measure device bind", method: http.MethodPost, path: "/api/v1/calibration/measure-device", body: []byte(`{"measureDeviceId":"m1"}`)},
		{name: "read pressure", method: http.MethodGet, path: "/api/v1/calibration/pressure"},
		{name: "read stability", method: http.MethodGet, path: "/api/v1/calibration/stability"},
		{name: "read measure data", method: http.MethodGet, path: "/api/v1/calibration/measure-data"},
		{name: "read valve", method: http.MethodGet, path: "/api/v1/calibration/valve"},
		{name: "set valve", method: http.MethodPost, path: "/api/v1/calibration/valve", body: []byte(`{"status":"calibration"}`)},
		{name: "read measure unit", method: http.MethodGet, path: "/api/v1/calibration/measure-unit"},
		{name: "set measure unit", method: http.MethodPost, path: "/api/v1/calibration/measure-unit", body: []byte(`{"unit":"kPa"}`)},
		{name: "read device info", method: http.MethodGet, path: "/api/v1/calibration/device-info"},
		{name: "reset device", method: http.MethodPost, path: "/api/v1/calibration/reset"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, tc.path, bytes.NewReader(tc.body))
			if len(tc.body) > 0 {
				req.Header.Set("Content-Type", "application/json")
			}
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)

			if rec.Code != http.StatusNotFound {
				t.Fatalf("expected status 404 for deprecated calibration endpoint %s %s, got %d", tc.method, tc.path, rec.Code)
			}
		})
	}
}
