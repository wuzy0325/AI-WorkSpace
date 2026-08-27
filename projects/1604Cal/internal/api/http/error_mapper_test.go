package http

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"cal1604/internal/application/session"
	apperrors "cal1604/internal/errors"
)

func TestWriteErrorResponse(t *testing.T) {
	rec := httptest.NewRecorder()

	writeError(rec, apperrors.ErrUnitMismatch)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", rec.Code)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "UNIT_MISMATCH") {
		t.Fatalf("expected UNIT_MISMATCH in body, got %q", body)
	}
}

func TestWriteErrorResponseForMeasureDeviceNotSet(t *testing.T) {
	rec := httptest.NewRecorder()

	writeError(rec, session.ErrMeasureDeviceNotSet)

	if rec.Code != http.StatusConflict {
		t.Fatalf("expected status 409, got %d", rec.Code)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "MEASURE_DEVICE_NOT_SET") {
		t.Fatalf("expected MEASURE_DEVICE_NOT_SET in body, got %q", body)
	}
}
