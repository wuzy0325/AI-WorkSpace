package http

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"cal1604/internal/api/dto"
)

type reportTemplatePayload struct {
	Filename string `json:"filename"`
}

func TestSelectReportTemplate(t *testing.T) {
	router := NewRouter()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/reports/templates/select?points=5&mode=single", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	var resp dto.Response[reportTemplatePayload]
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if resp.Data.Filename != "5s.xlsx" {
		t.Fatalf("expected 5s.xlsx, got %s", resp.Data.Filename)
	}
}

func TestSelectReportTemplateWithInvalidParams(t *testing.T) {
	router := NewRouter()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/reports/templates/select?points=1&mode=single", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", rec.Code)
	}

	if !strings.Contains(rec.Body.String(), "INVALID_ARGUMENT") {
		t.Fatalf("expected INVALID_ARGUMENT code, got %q", rec.Body.String())
	}
}

func TestListTemplatesReturnsArray(t *testing.T) {
	router := NewRouter()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/reports/templates", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	var resp dto.Response[map[string]any]
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	templates, ok := resp.Data["templates"].([]any)
	if !ok {
		t.Fatalf("expected templates array, got %T", resp.Data["templates"])
	}

	if len(templates) != 0 {
		t.Fatalf("expected empty templates for default router, got %d", len(templates))
	}
}
