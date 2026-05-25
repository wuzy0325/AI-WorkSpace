package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"wind-daq/services/api-go/internal/adapters/config"
	"wind-daq/services/api-go/internal/adapters/hardware"
	"wind-daq/services/api-go/internal/core/device"
	"wind-daq/services/api-go/internal/core/motion"
	"wind-daq/services/api-go/internal/ports"
	"wind-daq/services/api-go/internal/usecase"
)

type apiProfileStore struct {
	profiles []device.Profile
}

func (s *apiProfileStore) LoadProfiles() ([]device.Profile, error) {
	return append([]device.Profile(nil), s.profiles...), nil
}

func (s *apiProfileStore) SaveProfiles(profiles []device.Profile) error {
	s.profiles = append([]device.Profile(nil), profiles...)
	return nil
}

type apiDeviceFactory struct{}

func (apiDeviceFactory) Create(profile device.Profile) (ports.Device, error) {
	return hardware.NewSimulatedDevice(profile), nil
}

type apiPublisher struct{}

func (apiPublisher) Publish(string, any) {}

func newTestMotionManager(axisNames ...motion.AxisName) *usecase.MotionManager {
	profileStore := config.NewMemoryMotionProfileStore()
	axes := make([]motion.AxisConfig, len(axisNames))
	for i, name := range axisNames {
		speed := 10.0
		axes[i] = motion.AxisConfig{Name: name, Enabled: true, Kind: motion.AxisKindLinear, MaxSpeed: &speed}
	}
	profiles := []motion.MotionControllerProfile{
		{
			ID:      "test-motion",
			Name:    "Test Controller",
			Type:    motion.ControllerTypeSimulated,
			Address: "127.0.0.1",
			Port:    9000,
			Axes:    axes,
		},
	}
	profileStore.SaveProfiles(profiles)
	mgr := usecase.NewMotionManager(profileStore, func(profile motion.MotionControllerProfile) ports.MotionController {
		return hardware.NewSimulatedMotionController(profile)
	})
	mgr.LoadProfiles()
	return mgr
}

func TestDeviceAcquisitionHTTPFlow(t *testing.T) {
	hub := usecase.NewAcquisitionHub(apiPublisher{}, 20)
	manager, err := usecase.NewDeviceManager(&apiProfileStore{}, apiDeviceFactory{}, hub.OnData)
	if err != nil {
		t.Fatalf("NewDeviceManager returned error: %v", err)
	}
	router := NewRouter(Deps{DeviceManager: manager, AcquisitionHub: hub})

	profile := device.NewDefaultProfile("sim-1", device.DeviceSimulated)
	profileBody, err := json.Marshal(profile)
	if err != nil {
		t.Fatal(err)
	}
	request(t, router, http.MethodPut, "/api/device/profiles", profileBody, http.StatusOK)
	request(t, router, http.MethodPost, "/api/device/sim-1/connect", nil, http.StatusOK)
	request(t, router, http.MethodPost, "/api/device/sim-1/startAcquisition", nil, http.StatusOK)
	defer request(t, router, http.MethodPost, "/api/device/sim-1/stopAcquisition", nil, http.StatusOK)

	deadline := time.After(700 * time.Millisecond)
	for {
		resp := request(t, router, http.MethodGet, "/api/daq/latest/sim-1", nil, http.StatusOK)
		var payload device.DataPayload
		if err := json.Unmarshal(resp.Body.Bytes(), &payload); err != nil {
			t.Fatalf("decode latest response: %v", err)
		}
		if len(payload.Channels) > 0 {
			break
		}
		select {
		case <-deadline:
			t.Fatal("timed out waiting for latest data")
		case <-time.After(20 * time.Millisecond):
		}
	}

	resp := request(t, router, http.MethodGet, "/api/device/sim-1/status", nil, http.StatusOK)
	var status device.Status
	if err := json.Unmarshal(resp.Body.Bytes(), &status); err != nil {
		t.Fatalf("decode status response: %v", err)
	}
	if !status.Acquiring {
		t.Fatalf("expected acquiring status, got %#v", status)
	}
}

func TestDeviceDisconnectAndDeleteProfileHTTPFlow(t *testing.T) {
	hub := usecase.NewAcquisitionHub(apiPublisher{}, 20)
	store := &apiProfileStore{}
	manager, err := usecase.NewDeviceManager(store, apiDeviceFactory{}, hub.OnData)
	if err != nil {
		t.Fatalf("NewDeviceManager returned error: %v", err)
	}
	router := NewRouter(Deps{DeviceManager: manager, AcquisitionHub: hub})

	profile := device.NewDefaultProfile("sim-1", device.DeviceSimulated)
	profileBody, err := json.Marshal(profile)
	if err != nil {
		t.Fatal(err)
	}
	request(t, router, http.MethodPut, "/api/device/profiles", profileBody, http.StatusOK)
	request(t, router, http.MethodPost, "/api/device/sim-1/connect", nil, http.StatusOK)
	request(t, router, http.MethodPost, "/api/device/sim-1/disconnect", nil, http.StatusOK)
	request(t, router, http.MethodGet, "/api/device/sim-1/status", nil, http.StatusNotFound)

	request(t, router, http.MethodPost, "/api/device/sim-1/connect", nil, http.StatusOK)
	request(t, router, http.MethodDelete, "/api/device/profiles/sim-1", nil, http.StatusOK)
	if len(store.profiles) != 0 {
		t.Fatalf("expected profile store to be empty, got %d", len(store.profiles))
	}
	request(t, router, http.MethodGet, "/api/device/sim-1/status", nil, http.StatusNotFound)
}

func TestDeviceScanHTTPFlow(t *testing.T) {
	hub := usecase.NewAcquisitionHub(apiPublisher{}, 20)
	manager, err := usecase.NewDeviceManager(&apiProfileStore{}, apiDeviceFactory{}, hub.OnData)
	if err != nil {
		t.Fatalf("NewDeviceManager returned error: %v", err)
	}
	manager.SetScanner(apiScanner{results: []device.ScanResult{
		{ID: "sim-1", Name: "Simulator 1", Type: device.DeviceSimulated, Available: true},
	}})
	router := NewRouter(Deps{DeviceManager: manager, AcquisitionHub: hub})

	resp := request(t, router, http.MethodGet, "/api/device/scan", nil, http.StatusOK)
	var results []device.ScanResult
	if err := json.Unmarshal(resp.Body.Bytes(), &results); err != nil {
		t.Fatalf("decode scan response: %v", err)
	}
	if len(results) != 1 || results[0].ID != "sim-1" {
		t.Fatalf("unexpected scan results: %+v", results)
	}
}

func TestDeviceConfigurationHTTPFlow(t *testing.T) {
	hub := usecase.NewAcquisitionHub(apiPublisher{}, 20)
	store := &apiProfileStore{}
	manager, err := usecase.NewDeviceManager(store, apiDeviceFactory{}, hub.OnData)
	if err != nil {
		t.Fatalf("NewDeviceManager returned error: %v", err)
	}
	router := NewRouter(Deps{DeviceManager: manager, AcquisitionHub: hub})

	profile := device.NewDefaultProfile("temp-1", device.DeviceDaqT1603)
	profileBody, err := json.Marshal(profile)
	if err != nil {
		t.Fatal(err)
	}
	request(t, router, http.MethodPut, "/api/device/profiles", profileBody, http.StatusOK)

	request(t, router, http.MethodPut, "/api/device/temp-1/unit", []byte(`{"unit":"degC"}`), http.StatusOK)
	if store.profiles[0].Channels[0].Unit != "degC" {
		t.Fatalf("expected persisted unit degC, got %q", store.profiles[0].Channels[0].Unit)
	}

	request(t, router, http.MethodPut, "/api/device/temp-1/daqT1603Config", []byte(`{"thermocoupleType":"K","coldJunction":"internal","filterHz":50}`), http.StatusOK)
	resp := request(t, router, http.MethodGet, "/api/device/temp-1/daqT1603Config", nil, http.StatusOK)
	var got device.DaqT1603HardwareConfig
	if err := json.Unmarshal(resp.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode daqT1603 config response: %v", err)
	}
	if got.ThermocoupleType != "K" || got.ColdJunction != "internal" || got.FilterHz != 50 {
		t.Fatalf("unexpected daqT1603 config: %+v", got)
	}
}

func TestDaqStreamSendsServerSentEvents(t *testing.T) {
	hub := usecase.NewAcquisitionHub(apiPublisher{}, 20)
	manager, err := usecase.NewDeviceManager(&apiProfileStore{}, apiDeviceFactory{}, hub.OnData)
	if err != nil {
		t.Fatalf("NewDeviceManager returned error: %v", err)
	}
	router := NewRouter(Deps{DeviceManager: manager, AcquisitionHub: hub})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	req := httptest.NewRequestWithContext(ctx, http.MethodGet, "/api/daq/stream/dev-1", nil)
	resp := httptest.NewRecorder()
	done := make(chan struct{})
	go func() {
		router.ServeHTTP(resp, req)
		close(done)
	}()

	headerDeadline := time.After(500 * time.Millisecond)
	for resp.Header().Get("Content-Type") != "text/event-stream" {
		select {
		case <-time.After(10 * time.Millisecond):
		case <-headerDeadline:
			t.Fatal("timed out waiting for SSE response headers")
		}
	}
	hub.OnData(device.DataPayload{DeviceID: "dev-1", Timestamp: 123, Channels: []float64{42}, ChannelIndices: []int{0}})
	deadline := time.After(500 * time.Millisecond)
	for {
		if strings.Contains(resp.Body.String(), "event: payload") && strings.Contains(resp.Body.String(), `"deviceId":"dev-1"`) {
			cancel()
			<-done
			return
		}
		select {
		case <-deadline:
			cancel()
			<-done
			t.Fatalf("timed out waiting for SSE payload, body=%q", resp.Body.String())
		case <-time.After(10 * time.Millisecond):
		}
	}
}

func TestMotionHTTPFlow(t *testing.T) {
	hub := usecase.NewAcquisitionHub(apiPublisher{}, 20)
	motionMgr := newTestMotionManager(motion.AxisX, motion.AxisY, motion.AxisZ)
	router := NewRouter(Deps{
		DeviceManager:  newTestDeviceManager(t, hub),
		AcquisitionHub: hub,
		MotionManager:  motionMgr,
	})

	request(t, router, http.MethodPost, "/api/motion/connect", []byte(`{"id":"test-motion"}`), http.StatusOK)
	resp := request(t, router, http.MethodGet, "/api/motion/status", nil, http.StatusOK)
	var statuses []struct {
		Connected bool              `json:"connected"`
		Axes      []json.RawMessage `json:"axes"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &statuses); err != nil {
		t.Fatalf("decode motion status: %v", err)
	}
	if len(statuses) != 1 {
		t.Fatalf("expected 1 controller status, got %d", len(statuses))
	}
	if !statuses[0].Connected {
		t.Fatal("expected connected after connect")
	}
	if len(statuses[0].Axes) != 3 {
		t.Fatalf("expected 3 axes, got %d", len(statuses[0].Axes))
	}

	request(t, router, http.MethodPost, "/api/motion/moveTo", []byte(`{"id":"test-motion","axis":"X","position":50}`), http.StatusOK)
	request(t, router, http.MethodPost, "/api/motion/jog", []byte(`{"id":"test-motion","axis":"Y","velocity":2}`), http.StatusOK)
	resp2 := request(t, router, http.MethodGet, "/api/motion/status", nil, http.StatusOK)
	if strings.Contains(resp2.Body.String(), `"moving":true`) {
		request(t, router, http.MethodPost, "/api/motion/stop", []byte(`{"id":"test-motion","axis":"Y"}`), http.StatusOK)
	}
	request(t, router, http.MethodPost, "/api/motion/emergencyStop", []byte(`{"id":"test-motion"}`), http.StatusOK)
	request(t, router, http.MethodPost, "/api/motion/disconnect", []byte(`{"id":"test-motion"}`), http.StatusOK)
}

func TestMotionHTTPValidation(t *testing.T) {
	hub := usecase.NewAcquisitionHub(apiPublisher{}, 20)
	motionMgr := newTestMotionManager(motion.AxisX)
	router := NewRouter(Deps{
		DeviceManager:  newTestDeviceManager(t, hub),
		AcquisitionHub: hub,
		MotionManager:  motionMgr,
	})

	request(t, router, http.MethodGet, "/api/motion/connect", nil, http.StatusMethodNotAllowed)
	request(t, router, http.MethodPost, "/api/motion/status", nil, http.StatusMethodNotAllowed)
	request(t, router, http.MethodPost, "/api/motion/moveTo", []byte(`{"axis":"","position":0}`), http.StatusBadRequest)
	request(t, router, http.MethodPost, "/api/motion/jog", []byte(`{"axis":"","velocity":0}`), http.StatusBadRequest)
}

func TestCalibrationHTTPFlow(t *testing.T) {
	hub := usecase.NewAcquisitionHub(apiPublisher{}, 20)
	motionMgr := newTestMotionManager(motion.AxisX, motion.AxisY, motion.AxisZ)
	calMgr := usecase.NewCalibrationManager(hub, motionMgr, nil, nil)
	router := NewRouter(Deps{
		DeviceManager:      newTestDeviceManager(t, hub),
		AcquisitionHub:     hub,
		MotionManager:      motionMgr,
		CalibrationManager: calMgr,
	})

	request(t, router, http.MethodGet, "/api/calibration/status", nil, http.StatusOK)
	request(t, router, http.MethodPost, "/api/calibration/start", []byte(`{"taskId":"cal-1","deviceId":"sim-1","type":"five-hole","channels":[0,1],"pressurePoints":[0,50,100],"averageSamples":5}`), http.StatusOK)
	resp := request(t, router, http.MethodGet, "/api/calibration/status", nil, http.StatusOK)
	var st struct {
		State       string `json:"state"`
		TotalPoints int    `json:"totalPoints"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &st); err != nil {
		t.Fatalf("decode calibration status: %v", err)
	}
	if st.State != "running" {
		t.Fatalf("expected running state, got %q", st.State)
	}
	if st.TotalPoints != 3 {
		t.Fatalf("expected 3 total points, got %d", st.TotalPoints)
	}

	request(t, router, http.MethodPost, "/api/calibration/pause", nil, http.StatusOK)
	resp2 := request(t, router, http.MethodGet, "/api/calibration/status", nil, http.StatusOK)
	if !strings.Contains(resp2.Body.String(), `"paused"`) {
		t.Fatalf("expected paused state, got %s", resp2.Body.String())
	}

	request(t, router, http.MethodPost, "/api/calibration/resume", nil, http.StatusOK)
	resp3 := request(t, router, http.MethodGet, "/api/calibration/status", nil, http.StatusOK)
	if !strings.Contains(resp3.Body.String(), `"running"`) {
		t.Fatalf("expected running after resume, got %s", resp3.Body.String())
	}

	request(t, router, http.MethodPost, "/api/calibration/collect", nil, http.StatusBadRequest)
	request(t, router, http.MethodPost, "/api/calibration/stop", nil, http.StatusOK)
}

func TestCalibrationHTTPValidation(t *testing.T) {
	hub := usecase.NewAcquisitionHub(apiPublisher{}, 20)
	calMgr := usecase.NewCalibrationManager(hub, nil, nil, nil)
	router := NewRouter(Deps{
		DeviceManager:      newTestDeviceManager(t, hub),
		AcquisitionHub:     hub,
		MotionManager:      usecase.NewMotionManager(nil, nil),
		CalibrationManager: calMgr,
	})

	request(t, router, http.MethodPost, "/api/calibration/start", []byte(`{}`), http.StatusBadRequest)
	request(t, router, http.MethodPost, "/api/calibration/start", []byte(`{"taskId":"cal-1","deviceId":"sim-1","channels":[0],"pressurePoints":[0],"averageSamples":1}`), http.StatusBadRequest)
	request(t, router, http.MethodGet, "/api/calibration/nonexistent", nil, http.StatusNotFound)
}

func TestTraversalHTTPFlow(t *testing.T) {
	hub := usecase.NewAcquisitionHub(apiPublisher{}, 20)
	motionMgr := newTestMotionManager(motion.AxisX, motion.AxisY, motion.AxisZ)
	travMgr := usecase.NewTraversalManager(hub, motionMgr, nil, nil)
	router := NewRouter(Deps{
		DeviceManager:    newTestDeviceManager(t, hub),
		AcquisitionHub:   hub,
		MotionManager:    motionMgr,
		TraversalManager: travMgr,
	})

	request(t, router, http.MethodGet, "/api/traversal/status", nil, http.StatusOK)
	pathPayload := []byte(`{"taskId":"trav-1","deviceId":"sim-1","channels":[0],"path":[{"x":0,"y":0,"z":0},{"x":50,"y":25,"z":10}]}`)
	request(t, router, http.MethodPost, "/api/traversal/start", pathPayload, http.StatusOK)
	resp := request(t, router, http.MethodGet, "/api/traversal/status", nil, http.StatusOK)
	var st struct {
		State       string `json:"state"`
		TotalPoints int    `json:"totalPoints"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &st); err != nil {
		t.Fatalf("decode traversal status: %v", err)
	}
	if st.State != "running" {
		t.Fatalf("expected running state, got %q", st.State)
	}
	if st.TotalPoints != 2 {
		t.Fatalf("expected 2 total points, got %d", st.TotalPoints)
	}

	request(t, router, http.MethodPost, "/api/traversal/pause", nil, http.StatusOK)
	request(t, router, http.MethodPost, "/api/traversal/resume", nil, http.StatusOK)
	request(t, router, http.MethodPost, "/api/traversal/stop", nil, http.StatusOK)
}

func TestTraversalHTTPValidation(t *testing.T) {
	hub := usecase.NewAcquisitionHub(apiPublisher{}, 20)
	travMgr := usecase.NewTraversalManager(hub, nil, nil, nil)
	router := NewRouter(Deps{
		DeviceManager:    newTestDeviceManager(t, hub),
		AcquisitionHub:   hub,
		MotionManager:    usecase.NewMotionManager(nil, nil),
		TraversalManager: travMgr,
	})

	request(t, router, http.MethodPost, "/api/traversal/start", []byte(`{}`), http.StatusBadRequest)
	request(t, router, http.MethodPost, "/api/traversal/start", []byte(`{"taskId":"trav-1","deviceId":"sim-1","channels":[],"path":[]}`), http.StatusBadRequest)
}

func TestStorageHTTPFlow(t *testing.T) {
	hub := usecase.NewAcquisitionHub(apiPublisher{}, 20)
	recorder := usecase.NewStorageRecorder(nil)
	router := NewRouter(Deps{
		DeviceManager:   newTestDeviceManager(t, hub),
		AcquisitionHub:  hub,
		StorageRecorder: recorder,
	})

	request(t, router, http.MethodGet, "/api/storage/status", nil, http.StatusOK)
	request(t, router, http.MethodPost, "/api/storage/start", []byte(`{"outputDir":"testdata","filePrefix":"run-1"}`), http.StatusBadRequest)
	request(t, router, http.MethodPost, "/api/storage/start", []byte(`{}`), http.StatusBadRequest)
	request(t, router, http.MethodPost, "/api/storage/stop", nil, http.StatusOK)
}

func TestPublishRateHTTPFlow(t *testing.T) {
	hub := usecase.NewAcquisitionHub(apiPublisher{}, 20)
	router := NewRouter(Deps{
		DeviceManager:   newTestDeviceManager(t, hub),
		AcquisitionHub:  hub,
		StorageRecorder: usecase.NewStorageRecorder(nil),
	})

	request(t, router, http.MethodGet, "/api/daq/publishRate", nil, http.StatusOK)
	request(t, router, http.MethodPut, "/api/daq/publishRate", []byte(`{"hz":10}`), http.StatusOK)
	request(t, router, http.MethodPut, "/api/daq/publishRate", []byte(`{"hz":0}`), http.StatusBadRequest)
}

func TestReportHTTPFlow(t *testing.T) {
	hub := usecase.NewAcquisitionHub(apiPublisher{}, 20)
	reportMgr := usecase.NewReportManager(nil)
	router := NewRouter(Deps{
		DeviceManager:  newTestDeviceManager(t, hub),
		AcquisitionHub: hub,
		ReportManager:  reportMgr,
	})

	request(t, router, http.MethodGet, "/api/report/status", nil, http.StatusOK)
	request(t, router, http.MethodPost, "/api/report/generate", []byte(`{}`), http.StatusBadRequest)
	request(t, router, http.MethodPost, "/api/report/generate", []byte(`{"outputDir":"/tmp","filePrefix":"test","deviceId":"sim-1"}`), http.StatusBadRequest)
}

func newTestDeviceManager(t *testing.T, hub *usecase.AcquisitionHub) *usecase.DeviceManager {
	t.Helper()
	mgr, err := usecase.NewDeviceManager(&apiProfileStore{}, apiDeviceFactory{}, hub.OnData)
	if err != nil {
		t.Fatalf("NewDeviceManager returned error: %v", err)
	}
	return mgr
}

type apiScanner struct {
	results []device.ScanResult
}

func (s apiScanner) Scan() ([]device.ScanResult, error) {
	return append([]device.ScanResult(nil), s.results...), nil
}

func request(t *testing.T, handler http.Handler, method string, path string, body []byte, wantStatus int) *httptest.ResponseRecorder {
	t.Helper()
	var reader *bytes.Reader
	if body == nil {
		reader = bytes.NewReader(nil)
	} else {
		reader = bytes.NewReader(body)
	}
	req := httptest.NewRequest(method, path, reader)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp := httptest.NewRecorder()
	handler.ServeHTTP(resp, req)
	if resp.Code != wantStatus {
		t.Fatalf("%s %s: expected status %d, got %d body=%s", method, path, wantStatus, resp.Code, resp.Body.String())
	}
	return resp
}
