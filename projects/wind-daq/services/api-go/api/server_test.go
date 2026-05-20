package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"wind-daq/services/api-go/internal/adapters/hardware"
	"wind-daq/services/api-go/internal/core/device"
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
