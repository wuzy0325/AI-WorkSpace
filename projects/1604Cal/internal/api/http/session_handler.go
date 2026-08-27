package http

import (
	"fmt"
	"net/http"

	apperrors "cal1604/internal/errors"
	"cal1604/internal/events"
)

type sessionStatePayload struct {
	State string `json:"state"`
}

func (s *apiServer) sessionStateHandler(w http.ResponseWriter, _ *http.Request) {
	writeSuccess(w, http.StatusOK, sessionStatePayload{State: string(s.coordinator.State())})
}

func (s *apiServer) sessionStartHandler(w http.ResponseWriter, r *http.Request) {
	if err := s.calibrationService.ValidateStartPrerequisites(r.Context()); err != nil {
		writeError(w, fmt.Errorf("%w: %s", apperrors.ErrPrerequisiteNotMet, err.Error()))
		return
	}

	if err := s.calibrationService.StartCalibration(r.Context()); err != nil {
		writeError(w, err)
		return
	}

	writeSuccess(w, http.StatusOK, sessionStatePayload{State: string(s.coordinator.State())})
}

func (s *apiServer) sessionPauseHandler(w http.ResponseWriter, _ *http.Request) {
	if err := s.calibrationService.PauseAutoCollection(); err != nil {
		writeError(w, err)
		return
	}

	writeSuccess(w, http.StatusOK, sessionStatePayload{State: string(s.coordinator.State())})
}

func (s *apiServer) sessionResumeHandler(w http.ResponseWriter, r *http.Request) {
	if err := s.calibrationService.ResumeAutoCollection(r.Context()); err != nil {
		writeError(w, err)
		return
	}

	writeSuccess(w, http.StatusOK, sessionStatePayload{State: string(s.coordinator.State())})
}

func (s *apiServer) sessionStopHandler(w http.ResponseWriter, r *http.Request) {
	if err := s.calibrationService.EndCalibration(r.Context()); err != nil {
		writeError(w, err)
		return
	}

	s.coordinator.End()
	s.publishSessionState()

	writeSuccess(w, http.StatusOK, sessionStatePayload{State: string(s.coordinator.State())})
}

func (s *apiServer) publishSessionState() {
	publishEvent(events.EventSessionStateChanged, map[string]any{
		"state": string(s.coordinator.State()),
	})
}
