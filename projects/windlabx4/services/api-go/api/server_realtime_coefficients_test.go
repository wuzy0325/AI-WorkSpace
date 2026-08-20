package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"windlabx4/services/api-go/internal/usecase"
)

func TestRealtimeCoefficientsRouteCalculatesWithoutCalibrationTask(t *testing.T) {
	router := NewRouter(Deps{CalibrationManager: usecase.NewCalibrationManager(nil, nil, nil, nil)})
	body := `{"type":"five-hole","input":{"P1":100,"P2":200,"P3":100,"P4":110,"P5":90,"PAtm":101325,"TAtm":20,"PTotal":80,"PStatic":15}}`
	req := httptest.NewRequest(http.MethodPost, "/api/calibration/realtime-coefficients", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()

	router.ServeHTTP(response, req)

	if response.Code != http.StatusOK {
		t.Fatalf("realtime-coefficients status = %d, body = %s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), `"Kalpha"`) {
		t.Fatalf("realtime-coefficients response missing Kalpha: %s", response.Body.String())
	}
}

func TestRealtimeCoefficientsRouteRejectsUnsupportedType(t *testing.T) {
	router := NewRouter(Deps{CalibrationManager: usecase.NewCalibrationManager(nil, nil, nil, nil)})
	req := httptest.NewRequest(http.MethodPost, "/api/calibration/realtime-coefficients", strings.NewReader(`{"type":"unknown","input":{}}`))
	response := httptest.NewRecorder()

	router.ServeHTTP(response, req)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("unsupported type status = %d, want 400", response.Code)
	}
}
