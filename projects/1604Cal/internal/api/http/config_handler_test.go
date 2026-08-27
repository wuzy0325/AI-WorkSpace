package http

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"cal1604/internal/api/dto"
	"cal1604/internal/application/deviceconnect"
	"cal1604/internal/config"
	"cal1604/internal/domain"
)

type deviceConnectConfigResponse struct {
	ConnectAttemptTimeoutMs    int `json:"connectAttemptTimeoutMs"`
	ConnectMaxAttempts         int `json:"connectMaxAttempts"`
	ConnectInitialBackoffMs    int `json:"connectInitialBackoffMs"`
	ConnectMaxBackoffMs        int `json:"connectMaxBackoffMs"`
	DisconnectAttemptTimeoutMs int `json:"disconnectAttemptTimeoutMs"`
	DisconnectMaxAttempts      int `json:"disconnectMaxAttempts"`
	DisconnectInitialBackoffMs int `json:"disconnectInitialBackoffMs"`
	DisconnectMaxBackoffMs     int `json:"disconnectMaxBackoffMs"`
}

func TestGetDeviceConnectConfig(t *testing.T) {
	router := NewRouterWithConnectConfig(nil, deviceconnect.Config{
		ConnectAttemptTimeout:    1800 * time.Millisecond,
		ConnectMaxAttempts:       5,
		ConnectInitialBackoff:    90 * time.Millisecond,
		ConnectMaxBackoff:        500 * time.Millisecond,
		DisconnectAttemptTimeout: 1100 * time.Millisecond,
		DisconnectMaxAttempts:    4,
		DisconnectInitialBackoff: 50 * time.Millisecond,
		DisconnectMaxBackoff:     250 * time.Millisecond,
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/config/device-connect", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var resp dto.Response[deviceConnectConfigResponse]
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if !resp.Success {
		t.Fatalf("expected success response, got %+v", resp)
	}

	if resp.Data.ConnectAttemptTimeoutMs != 1800 || resp.Data.ConnectMaxAttempts != 5 {
		t.Fatalf("unexpected connect config payload: %+v", resp.Data)
	}

	if resp.Data.DisconnectAttemptTimeoutMs != 1100 || resp.Data.DisconnectMaxAttempts != 4 {
		t.Fatalf("unexpected disconnect config payload: %+v", resp.Data)
	}
}

func TestMeasurementConfigIsIndependentFromCalibrationConfig(t *testing.T) {
	appCfg := config.Default()
	router := NewRouterWithRuntimeConfig(
		nil,
		deviceconnect.DefaultConfig(),
		CalibrationRuntimeConfig{},
		"",
		appCfg,
	)

	getMeasurementReq := httptest.NewRequest(http.MethodGet, "/api/v1/config/measurement", nil)
	getMeasurementRec := httptest.NewRecorder()
	router.ServeHTTP(getMeasurementRec, getMeasurementReq)

	if getMeasurementRec.Code != http.StatusOK {
		t.Fatalf("expected measurement config status 200, got %d", getMeasurementRec.Code)
	}

	var measurementResp dto.Response[config.MeasurementParamsConfig]
	if err := json.NewDecoder(getMeasurementRec.Body).Decode(&measurementResp); err != nil {
		t.Fatalf("decode measurement config response: %v", err)
	}

	if measurementResp.Data.PointCount != 6 {
		t.Fatalf("expected default measurement pointCount 6, got %d", measurementResp.Data.PointCount)
	}

	updatePayload := []byte(`{
		"minPressure": 10,
		"maxPressure": 250,
		"pointCount": 9,
		"precision": 3,
		"averageCount": 2,
		"stableDurationMs": 7000,
		"precisionLevel": 0.1,
		"pressureMode": "roundTrip",
		"controlMode": "manual"
	}`)

	postMeasurementReq := httptest.NewRequest(http.MethodPost, "/api/v1/config/measurement", bytes.NewReader(updatePayload))
	postMeasurementReq.Header.Set("Content-Type", "application/json")
	postMeasurementRec := httptest.NewRecorder()
	router.ServeHTTP(postMeasurementRec, postMeasurementReq)

	if postMeasurementRec.Code != http.StatusOK {
		t.Fatalf("expected update measurement config status 200, got %d", postMeasurementRec.Code)
	}

	getMeasurementAfterReq := httptest.NewRequest(http.MethodGet, "/api/v1/config/measurement", nil)
	getMeasurementAfterRec := httptest.NewRecorder()
	router.ServeHTTP(getMeasurementAfterRec, getMeasurementAfterReq)

	if getMeasurementAfterRec.Code != http.StatusOK {
		t.Fatalf("expected updated measurement config status 200, got %d", getMeasurementAfterRec.Code)
	}

	var measurementAfterResp dto.Response[config.MeasurementParamsConfig]
	if err := json.NewDecoder(getMeasurementAfterRec.Body).Decode(&measurementAfterResp); err != nil {
		t.Fatalf("decode updated measurement config response: %v", err)
	}

	if measurementAfterResp.Data.PointCount != 9 || measurementAfterResp.Data.ControlMode != domain.ControlModeManual {
		t.Fatalf("unexpected updated measurement config payload: %+v", measurementAfterResp.Data)
	}

	getCalibrationReq := httptest.NewRequest(http.MethodGet, "/api/v1/config/calibration", nil)
	getCalibrationRec := httptest.NewRecorder()
	router.ServeHTTP(getCalibrationRec, getCalibrationReq)

	if getCalibrationRec.Code != http.StatusOK {
		t.Fatalf("expected calibration config status 200, got %d", getCalibrationRec.Code)
	}

	var calibrationResp dto.Response[config.CalibrationParamsConfig]
	if err := json.NewDecoder(getCalibrationRec.Body).Decode(&calibrationResp); err != nil {
		t.Fatalf("decode calibration config response: %v", err)
	}

	if calibrationResp.Data.PointCount != 5 {
		t.Fatalf("expected calibration pointCount remain 5, got %d", calibrationResp.Data.PointCount)
	}
}

func TestMeasurementConfigPersistsWhenConfigPathProvided(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "app.json")
	appCfg := config.Default()
	if err := appCfg.SaveToFile(configPath); err != nil {
		t.Fatalf("seed config file: %v", err)
	}

	router := NewRouterWithRuntimeConfig(
		nil,
		deviceconnect.DefaultConfig(),
		CalibrationRuntimeConfig{},
		configPath,
		appCfg,
	)

	updatePayload := []byte(`{
		"minPressure": 10,
		"maxPressure": 250,
		"pointCount": 9,
		"precision": 3,
		"averageCount": 2,
		"stableDurationMs": 7000,
		"precisionLevel": 0.1,
		"pressureMode": "roundTrip",
		"controlMode": "manual"
	}`)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/config/measurement", bytes.NewReader(updatePayload))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected config update status 200, got %d", rec.Code)
	}

	raw, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read persisted config: %v", err)
	}

	var persisted config.AppConfig
	if err := json.Unmarshal(raw, &persisted); err != nil {
		t.Fatalf("decode persisted config: %v", err)
	}

	if persisted.MeasurementParams.PointCount != 9 || persisted.MeasurementParams.ControlMode != domain.ControlModeManual {
		t.Fatalf("unexpected persisted measurement params: %+v", persisted.MeasurementParams)
	}
}
