package http_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	apihttp "cal1604/internal/api/http"
)

func TestHealthHandler(t *testing.T) {
	router := apihttp.NewRouter()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	if rec.Body.String() == "" {
		t.Fatal("expected non-empty response body")
	}
}
