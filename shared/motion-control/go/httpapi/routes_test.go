package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"shared.local/device-sdk/go/motion/core"
)

func TestProfilesPutIgnoresNULBytes(t *testing.T) {
	service := &stubMotionService{}
	mux := http.NewServeMux()
	RegisterMotionRoutes(mux, service)

	body := "{\x00\"id\":\"ctrl-1\",\"name\":\"B140\",\"type\":\"B140-MC\",\"address\":\"127.0.0.1\",\"port\":23,\"axes\":[]}"
	req := httptest.NewRequest(http.MethodPut, "/api/motion/profiles", strings.NewReader(body))
	rec := httptest.NewRecorder()

	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if service.saved.ID != "ctrl-1" {
		t.Fatalf("saved profile id = %q", service.saved.ID)
	}
}

type stubMotionService struct {
	saved core.MotionControllerProfile
}

func (s *stubMotionService) LoadProfiles() ([]core.MotionControllerProfile, error) { return nil, nil }
func (s *stubMotionService) GetProfiles() []core.MotionControllerProfile           { return nil }
func (s *stubMotionService) UpsertProfile(profile core.MotionControllerProfile) error {
	s.saved = profile
	return nil
}
func (s *stubMotionService) DeleteProfile(id string) error                         { return nil }
func (s *stubMotionService) Connect(ctx context.Context, id string) error          { return nil }
func (s *stubMotionService) Disconnect(ctx context.Context, id string) error       { return nil }
func (s *stubMotionService) StatusAll(ctx context.Context) []core.ControllerStatus { return nil }
func (s *stubMotionService) Home(ctx context.Context, id string, axis core.AxisName) error {
	return nil
}
func (s *stubMotionService) MoveTo(ctx context.Context, id string, axis core.AxisName, position float64) error {
	return nil
}
func (s *stubMotionService) MoveBy(ctx context.Context, id string, axis core.AxisName, delta float64) error {
	return nil
}
func (s *stubMotionService) Jog(ctx context.Context, id string, axis core.AxisName, velocity float64) error {
	return nil
}
func (s *stubMotionService) DefinePosition(ctx context.Context, id string, axis core.AxisName, position float64) error {
	return nil
}
func (s *stubMotionService) Stop(ctx context.Context, id string, axis core.AxisName) error {
	return nil
}
func (s *stubMotionService) EmergencyStop(ctx context.Context, id string) error      { return nil }
func (s *stubMotionService) ResetEmergencyStop(ctx context.Context, id string) error { return nil }
