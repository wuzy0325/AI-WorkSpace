package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHandleFiveHolePointImportReturnsOrderedPoints(t *testing.T) {
	request := httptest.NewRequest(http.MethodPost, "/api/calibration/fivehole-points-import", strings.NewReader(`{"content":"beta,alpha\n-10,5\n0,0\n-10,5\n"}`))
	response := httptest.NewRecorder()

	handleFiveHolePointImport(response, request, newTestFiveHoleCalibrationManager())

	if response.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", response.Code, response.Body.String())
	}
	var points []struct {
		ID          int                `json:"id"`
		Coordinates map[string]float64 `json:"coordinates"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &points); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if len(points) != 3 || points[0].Coordinates["β"] != -10 || points[0].Coordinates["α"] != 5 {
		t.Fatalf("unexpected imported points: %#v", points)
	}
	if points[2].Coordinates["β"] != -10 || points[2].Coordinates["α"] != 5 {
		t.Fatalf("duplicate point order was not preserved: %#v", points)
	}
}

func TestHandleFiveHolePointImportRejectsInvalidFile(t *testing.T) {
	request := httptest.NewRequest(http.MethodPost, "/api/calibration/fivehole-points-import", strings.NewReader(`{"content":"0,61\n"}`))
	response := httptest.NewRecorder()

	handleFiveHolePointImport(response, request, newTestFiveHoleCalibrationManager())

	if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "第 1 行") {
		t.Fatalf("expected row-specific 400, got %d: %s", response.Code, response.Body.String())
	}
}
