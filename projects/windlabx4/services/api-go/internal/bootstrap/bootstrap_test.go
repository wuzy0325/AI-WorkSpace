package bootstrap

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"windlabx4/services/api-go/internal/core/device"
)

func TestBuildAPIServerInitializesDefaultProfilesAndRouter(t *testing.T) {
	profilePath := filepath.Join(t.TempDir(), "profiles.json")

	server, err := BuildAPIServer(Config{Address: ":9090", ProfileStorePath: profilePath})
	if err != nil {
		t.Fatalf("BuildAPIServer returned error: %v", err)
	}
	if server.Address != ":9090" {
		t.Fatalf("expected address :9090, got %q", server.Address)
	}
	if server.Handler == nil {
		t.Fatal("expected handler to be initialized")
	}

	req := httptest.NewRequest(http.MethodGet, "/api/device/profiles", nil)
	resp := httptest.NewRecorder()
	server.Handler.ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d body=%s", resp.Code, resp.Body.String())
	}

	var profiles []device.Profile
	if err := json.Unmarshal(resp.Body.Bytes(), &profiles); err != nil {
		t.Fatalf("decode profiles response: %v", err)
	}
	if len(profiles) != 1 || profiles[0].ID != "sim-1" || profiles[0].Type != device.DeviceSimulated {
		t.Fatalf("unexpected default profiles: %+v", profiles)
	}
}
