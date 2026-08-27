package http

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"cal1604/internal/api/dto"
	"cal1604/internal/application/deviceconnect"
	"cal1604/internal/device"
	"cal1604/internal/device/manager"
	"cal1604/internal/domain"
)

type sessionStateResponse struct {
	State string `json:"state"`
}

func TestSessionInitialStateIsIdle(t *testing.T) {
	router := NewRouter()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/sessions/state", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	var resp dto.Response[sessionStateResponse]
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if resp.Data.State != "idle" {
		t.Fatalf("expected state idle, got %s", resp.Data.State)
	}
}

func TestSessionStartStopRestartFlow(t *testing.T) {
	router := newSessionRouterWithMeasureDriver(t)

	call := func(method, path string) sessionStateResponse {
		t.Helper()
		req := httptest.NewRequest(method, path, nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("request %s %s failed with %d", method, path, rec.Code)
		}

		var resp dto.Response[sessionStateResponse]
		if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
			t.Fatalf("decode response: %v", err)
		}

		return resp.Data
	}

	if got := call(http.MethodPost, "/api/v1/sessions/start").State; got != "ready" {
		t.Fatalf("expected start state ready, got %s", got)
	}

	if got := call(http.MethodPost, "/api/v1/sessions/stop").State; got != "stopped" {
		t.Fatalf("expected stop state stopped, got %s", got)
	}

	if got := call(http.MethodPost, "/api/v1/sessions/start").State; got != "ready" {
		t.Fatalf("expected restart state ready, got %s", got)
	}
}

func TestSessionStartWithoutDeviceRejected(t *testing.T) {
	router := NewRouter()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/sessions/start", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("expected status 409, got %d", rec.Code)
	}
}

func newSessionRouterWithMeasureDriver(t *testing.T) http.Handler {
	router, _ := newSessionRouterWithMeasureDriverAndFakeDriver(t)
	return router
}

func newSessionRouterWithMeasureDriverAndFakeDriver(t *testing.T) (http.Handler, *sessionFakeMeasureDriver) {
	t.Helper()

	fakeDriver := &sessionFakeMeasureDriver{
		// 默认阀门处于校准态，满足后端启动门禁。
		// 显式测门禁拒绝的用例可在拿到 fakeDriver 句柄后改写 valveStatus = "measurement"。
		valveStatus:              "calibration",
		unit:                     "kPa",
		calibrateZeroResult:      []float64{0.11},
		calibrateFullScaleResult: []float64{9.99},
	}

	store := manager.NewDeviceManager()
	store.Upsert(domain.Device{ID: "m1", Type: domain.DeviceTypeMeasure, Unit: "kPa", Status: domain.DeviceStatusConnected})
	connector := &sessionTestConnector{
		activeDrivers: map[string]device.ConnectionDriver{
			"m1": fakeDriver,
		},
	}
	router := NewRouterWithDependencies(store, connector)

	body := bytes.NewReader([]byte(`{"measureDeviceId":"m1","pressureDeviceId":""}`))
	req := httptest.NewRequest(http.MethodPost, "/api/v1/session/devices", body)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("bind measure device failed, status=%d", rec.Code)
	}

	configBody := bytes.NewReader([]byte(`{
		"channels":[1],
		"pressurePoints":2,
		"averageCount":1,
		"minPressure":0,
		"maxPressure":10,
		"stableWaitMs":1000,
		"controlMode":"manual"
	}`))
	configReq := httptest.NewRequest(http.MethodPost, "/api/v1/calibration/config", configBody)
	configReq.Header.Set("Content-Type", "application/json")
	configRec := httptest.NewRecorder()
	router.ServeHTTP(configRec, configReq)
	if configRec.Code != http.StatusOK {
		t.Fatalf("set calibration config failed, status=%d", configRec.Code)
	}

	return router, fakeDriver
}

func newSessionRouterWithMeasureDriverAndRuntimeConfig(t *testing.T, runtimeCfg CalibrationRuntimeConfig) http.Handler {
	t.Helper()

	store := manager.NewDeviceManager()
	connector := &sessionTestConnector{
		activeDrivers: map[string]device.ConnectionDriver{
			"m1": &sessionFakeMeasureDriver{
				valveStatus: "measurement",
				unit:        "kPa",
			},
		},
	}
	router := newRouter(store, connector, deviceconnect.DefaultConfig(), runtimeCfg, nil, "", "")

	bindBody := bytes.NewReader([]byte(`{"measureDeviceId":"m1","pressureDeviceId":""}`))
	bindReq := httptest.NewRequest(http.MethodPost, "/api/v1/session/devices", bindBody)
	bindReq.Header.Set("Content-Type", "application/json")
	bindRec := httptest.NewRecorder()
	router.ServeHTTP(bindRec, bindReq)
	if bindRec.Code != http.StatusOK {
		t.Fatalf("bind measure device failed, status=%d", bindRec.Code)
	}

	configBody := bytes.NewReader([]byte(`{
		"channels":[1],
		"pressurePoints":2,
		"averageCount":1,
		"minPressure":0,
		"maxPressure":10,
		"stableWaitMs":1000,
		"controlMode":"manual"
	}`))
	configReq := httptest.NewRequest(http.MethodPost, "/api/v1/calibration/config", configBody)
	configReq.Header.Set("Content-Type", "application/json")
	configRec := httptest.NewRecorder()
	router.ServeHTTP(configRec, configReq)
	if configRec.Code != http.StatusOK {
		t.Fatalf("set calibration config failed, status=%d", configRec.Code)
	}

	return router
}

func TestSessionStartRejectsWhenValveGateEnabled(t *testing.T) {
	router := newSessionRouterWithMeasureDriverAndRuntimeConfig(t, CalibrationRuntimeConfig{
		EnforceValveCalibrationGate: true,
	})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/sessions/start", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("expected status 409 when valve gate enabled, got %d", rec.Code)
	}
}

func TestSessionStartAllowsWhenValveGateDisabled(t *testing.T) {
	router := newSessionRouterWithMeasureDriverAndRuntimeConfig(t, CalibrationRuntimeConfig{
		EnforceValveCalibrationGate: false,
	})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/sessions/start", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200 when valve gate disabled, got %d", rec.Code)
	}
}

func TestSessionValvePutRejectsInvalidStatus(t *testing.T) {
	router := newSessionRouterWithMeasureDriver(t)
	body := bytes.NewReader([]byte(`{"status":"invalid"}`))
	req := httptest.NewRequest(http.MethodPut, "/api/v1/session/valve", body)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400 for invalid valve status, got %d", rec.Code)
	}
}

func TestSessionValvePutUpdatesStatus(t *testing.T) {
	router, _ := newSessionRouterWithMeasureDriverAndFakeDriver(t)
	body := bytes.NewReader([]byte(`{"status":"calibration"}`))
	req := httptest.NewRequest(http.MethodPut, "/api/v1/session/valve", body)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200 for set valve via PUT, got %d", rec.Code)
	}

	getReq := httptest.NewRequest(http.MethodGet, "/api/v1/session/valve", nil)
	getRec := httptest.NewRecorder()
	router.ServeHTTP(getRec, getReq)
	if getRec.Code != http.StatusOK {
		t.Fatalf("expected status 200 for valve get, got %d", getRec.Code)
	}

	var resp dto.Response[map[string]string]
	if err := json.NewDecoder(getRec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Data["status"] != "calibration" {
		t.Fatalf("expected valve status calibration, got %s", resp.Data["status"])
	}
}

func TestSessionCalibrateZeroEndpoint(t *testing.T) {
	router, fakeDriver := newSessionRouterWithMeasureDriverAndFakeDriver(t)
	body := bytes.NewReader([]byte(`{"channels":[1,2]}`))
	req := httptest.NewRequest(http.MethodPost, "/api/v1/session/calibrate-zero", body)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200 for calibrate zero, got %d", rec.Code)
	}

	if len(fakeDriver.calibrateZeroChannels) != 2 || fakeDriver.calibrateZeroChannels[0] != 1 || fakeDriver.calibrateZeroChannels[1] != 2 {
		t.Fatalf("expected calibrate zero channels [1 2], got %v", fakeDriver.calibrateZeroChannels)
	}
}

func TestSessionAggregateDeviceEndpoints(t *testing.T) {
	router, _ := newSessionRouterWithMeasureDriverAndFakeDriver(t)

	get := func(path string) map[string]any {
		t.Helper()
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("GET %s: expected 200, got %d", path, rec.Code)
		}
		var resp dto.Response[map[string]any]
		if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
			t.Fatalf("decode %s: %v", path, err)
		}
		return resp.Data
	}

	valves := get("/api/v1/session/valve/all")
	if _, ok := valves["devices"].(map[string]any)["m1"]; !ok {
		t.Fatalf("expected m1 valve result, got %#v", valves)
	}
	units := get("/api/v1/session/measure-unit/all")
	if _, ok := units["devices"].(map[string]any)["m1"]; !ok {
		t.Fatalf("expected m1 unit result, got %#v", units)
	}
	consistency := get("/api/v1/session/unit-consistency")
	if consistency["consistent"] != true {
		t.Fatalf("expected consistent bound devices, got %#v", consistency)
	}
}

func TestSessionTargetedZeroAndReset(t *testing.T) {
	router, fakeDriver := newSessionRouterWithMeasureDriverAndFakeDriver(t)

	zeroBody := bytes.NewReader([]byte(`{"deviceId":"m1","channels":[1]}`))
	zeroReq := httptest.NewRequest(http.MethodPost, "/api/v1/session/calibrate-zero", zeroBody)
	zeroReq.Header.Set("Content-Type", "application/json")
	zeroRec := httptest.NewRecorder()
	router.ServeHTTP(zeroRec, zeroReq)
	if zeroRec.Code != http.StatusOK {
		t.Fatalf("targeted zero: expected 200, got %d", zeroRec.Code)
	}

	resetBody := bytes.NewReader([]byte(`{"deviceId":"m1"}`))
	resetReq := httptest.NewRequest(http.MethodPost, "/api/v1/session/reset", resetBody)
	resetReq.Header.Set("Content-Type", "application/json")
	resetRec := httptest.NewRecorder()
	router.ServeHTTP(resetRec, resetReq)
	if resetRec.Code != http.StatusOK {
		t.Fatalf("targeted reset: expected 200, got %d", resetRec.Code)
	}
	if len(fakeDriver.calibrateZeroChannels) != 1 {
		t.Fatalf("expected targeted zero call, got %v", fakeDriver.calibrateZeroChannels)
	}
}

func TestSessionCalibrateFullScaleEndpoint(t *testing.T) {
	router, fakeDriver := newSessionRouterWithMeasureDriverAndFakeDriver(t)
	body := bytes.NewReader([]byte(`{"channels":[3],"fullScaleValue":100}`))
	req := httptest.NewRequest(http.MethodPost, "/api/v1/session/calibrate-full-scale", body)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200 for calibrate full scale, got %d", rec.Code)
	}

	if len(fakeDriver.calibrateFullScaleChannels) != 1 || fakeDriver.calibrateFullScaleChannels[0] != 3 {
		t.Fatalf("expected calibrate full scale channels [3], got %v", fakeDriver.calibrateFullScaleChannels)
	}
	if fakeDriver.calibrateFullScaleValue != 100 {
		t.Fatalf("expected full scale value 100, got %v", fakeDriver.calibrateFullScaleValue)
	}
}

// sessionTestConnector 提供测试用的活动驱动，用于模拟已连接设备。
type sessionTestConnector struct {
	activeDrivers map[string]device.ConnectionDriver
}

func (c *sessionTestConnector) Connect(_ context.Context, id string) (domain.Device, error) {
	return domain.Device{ID: id, Status: domain.DeviceStatusConnected}, nil
}

func (c *sessionTestConnector) Disconnect(_ context.Context, id string) (domain.Device, error) {
	return domain.Device{ID: id, Status: domain.DeviceStatusDisconnected}, nil
}

func (c *sessionTestConnector) Remove(_ context.Context, id string) error {
	return nil
}

func (c *sessionTestConnector) GetActiveDriver(id string) device.ConnectionDriver {
	if c.activeDrivers == nil {
		return nil
	}
	return c.activeDrivers[id]
}

// sessionFakeMeasureDriver 仅实现会话启动/停止测试所需的计量驱动能力。
type sessionFakeMeasureDriver struct {
	valveStatus                string
	unit                       string
	calibrateZeroChannels      []int
	calibrateFullScaleChannels []int
	calibrateFullScaleValue    float64
	calibrateZeroResult        []float64
	calibrateFullScaleResult   []float64
}

func (d *sessionFakeMeasureDriver) Connect(_ context.Context) error {
	return nil
}

func (d *sessionFakeMeasureDriver) Disconnect(_ context.Context) error {
	return nil
}

func (d *sessionFakeMeasureDriver) ReadValveStatus(_ context.Context) (string, error) {
	if d.valveStatus == "" {
		// 默认返回校准态，满足启动门禁的必要条件。
		// 需要验证门禁失败的用例可显式设置 valveStatus = "measurement"。
		return "calibration", nil
	}
	return d.valveStatus, nil
}

func (d *sessionFakeMeasureDriver) SetValveStatus(_ context.Context, status string) error {
	d.valveStatus = status
	return nil
}

func (d *sessionFakeMeasureDriver) ReadUnit(_ context.Context) (string, error) {
	if d.unit == "" {
		return "kPa", nil
	}
	return d.unit, nil
}

func (d *sessionFakeMeasureDriver) SetUnit(_ context.Context, unit string) error {
	d.unit = unit
	return nil
}

func (d *sessionFakeMeasureDriver) CollectData(_ context.Context, channels []int) ([]float64, error) {
	result := make([]float64, len(channels))
	for i := range channels {
		result[i] = 0
	}
	return result, nil
}

func (d *sessionFakeMeasureDriver) ReadDeviceInfo(_ context.Context) (map[string]string, error) {
	return map[string]string{
		"model": "fake-measure",
	}, nil
}

func (d *sessionFakeMeasureDriver) Reset(_ context.Context) error {
	return nil
}

func (d *sessionFakeMeasureDriver) CalibrateZero(_ context.Context, channels []int) ([]float64, error) {
	d.calibrateZeroChannels = append([]int(nil), channels...)
	return append([]float64(nil), d.calibrateZeroResult...), nil
}

func (d *sessionFakeMeasureDriver) CalibrateFullScale(_ context.Context, channels []int, fullScaleValue float64) ([]float64, error) {
	d.calibrateFullScaleChannels = append([]int(nil), channels...)
	d.calibrateFullScaleValue = fullScaleValue
	return append([]float64(nil), d.calibrateFullScaleResult...), nil
}
