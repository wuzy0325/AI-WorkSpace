package http

import (
	"encoding/json"
	"errors"
	"net/http"

	"cal1604/internal/api/dto"
	"cal1604/internal/application/calibration"
	"cal1604/internal/application/session"
	apperrors "cal1604/internal/errors"
	"cal1604/internal/events"
)

func writeError(w http.ResponseWriter, err error) {
	status := http.StatusInternalServerError
	code := "INTERNAL_ERROR"
	message := "internal error"

	if errors.Is(err, apperrors.ErrUnitMismatch) {
		status = http.StatusBadRequest
		code = "UNIT_MISMATCH"
		message = "device units are not consistent"
	} else if errors.Is(err, apperrors.ErrInvalidArgument) {
		status = http.StatusBadRequest
		code = "INVALID_ARGUMENT"
		message = "invalid request argument"
	} else if errors.Is(err, apperrors.ErrNotFound) {
		status = http.StatusNotFound
		code = "NOT_FOUND"
		message = "resource not found"
	} else if errors.Is(err, session.ErrDeviceNotFound) {
		status = http.StatusNotFound
		code = "DEVICE_NOT_FOUND"
		message = "device not found"
	} else if errors.Is(err, session.ErrMeasureDeviceNotSet) || errors.Is(err, calibration.ErrMeasureDeviceNotSet) {
		status = http.StatusConflict
		code = "MEASURE_DEVICE_NOT_SET"
		message = "measure device not set"
	} else if errors.Is(err, session.ErrPressureDeviceNotSet) || errors.Is(err, calibration.ErrPressureDeviceNotSet) {
		status = http.StatusConflict
		code = "PRESSURE_DEVICE_NOT_SET"
		message = "pressure device not set"
	} else if errors.Is(err, apperrors.ErrInvalidStateTransition) {
		status = http.StatusConflict
		code = "INVALID_STATE_TRANSITION"
		message = "invalid session state transition"
	} else if errors.Is(err, apperrors.ErrPrerequisiteNotMet) {
		status = http.StatusConflict
		code = "PREREQUISITE_NOT_MET"
		message = "prerequisite not met"
	} else if errors.Is(err, apperrors.ErrNoData) {
		status = http.StatusNotFound
		code = "NO_DATA"
		message = "no data available for export"
	} else if errors.Is(err, apperrors.ErrNoActiveSession) {
		status = http.StatusConflict
		code = "NO_ACTIVE_SESSION"
		message = "no active calibration session"
	} else if errors.Is(err, apperrors.ErrReportExport) {
		status = http.StatusUnprocessableEntity
		code = "REPORT_EXPORT_FAILED"
		message = "report export failed"
	} else if errors.Is(err, apperrors.ErrWorkflowConflict) {
		status = http.StatusConflict
		code = "WORKFLOW_CONFLICT"
		message = "active workflow conflict"
	} else if errors.Is(err, apperrors.ErrManualInterventionRequired) {
		status = http.StatusLocked
		code = "MANUAL_INTERVENTION_REQUIRED"
		message = "device requires manual intervention before new workflow"
	} else if errors.Is(err, apperrors.ErrStaleWorkflowContext) {
		status = http.StatusConflict
		code = "STALE_WORKFLOW_CONTEXT"
		message = "workflow context is stale"
	} else if errors.Is(err, apperrors.ErrWorkflowOwnerMismatch) {
		status = http.StatusConflict
		code = "WORKFLOW_OWNER_MISMATCH"
		message = "operation not allowed for current workflow owner"
	}

	// 将原始错误信息附加到 message，便于前端诊断具体原因
	if err != nil && err.Error() != message {
		message = message + ": " + err.Error()
	}

	// 广播系统错误事件到前端日志面板
	events.GlobalBus.Publish(events.Event{Type: events.EventSystemError, Data: map[string]any{
		"code":    code,
		"status":  status,
		"message": message,
	}})

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(dto.Response[any]{
		Success: false,
		Code:    code,
		Message: message,
	})
}
