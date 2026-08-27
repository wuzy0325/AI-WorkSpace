package workflow

import (
	"errors"
	"testing"

	"cal1604/internal/domain"
	apperrors "cal1604/internal/errors"
)

func TestNewCoordinatorStartsIdle(t *testing.T) {
	c := NewWorkflowCoordinator()
	if c.State() != domain.SessionStateIdle {
		t.Fatalf("expected idle, got %s", c.State())
	}
	if c.HasActiveWorkflow() {
		t.Fatal("new coordinator should not have active workflow")
	}
	if c.Owner() != "" {
		t.Fatalf("expected empty owner, got %s", c.Owner())
	}
}

func TestBeginSetsOwnerAndTransitionsToReady(t *testing.T) {
	c := NewWorkflowCoordinator()
	if err := c.Begin(OwnerMeasurement); err != nil {
		t.Fatalf("Begin: %v", err)
	}
	if c.State() != domain.SessionStateReady {
		t.Fatalf("expected ready, got %s", c.State())
	}
	if c.Owner() != OwnerMeasurement {
		t.Fatalf("expected owner measurement, got %s", c.Owner())
	}
	if c.CtxID() == "" {
		t.Fatal("expected non-empty ctxID")
	}
}

func TestBeginSameOwnerIsIdempotent(t *testing.T) {
	c := NewWorkflowCoordinator()
	if err := c.Begin(OwnerCalibration); err != nil {
		t.Fatalf("first Begin: %v", err)
	}
	firstCtx := c.CtxID()
	if err := c.Begin(OwnerCalibration); err != nil {
		t.Fatalf("second Begin (same owner) should be idempotent: %v", err)
	}
	if c.CtxID() != firstCtx {
		t.Fatal("ctxID should not change on idempotent Begin")
	}
}

func TestBeginDifferentOwnerRejectedWhenActive(t *testing.T) {
	c := NewWorkflowCoordinator()
	if err := c.Begin(OwnerMeasurement); err != nil {
		t.Fatalf("first Begin: %v", err)
	}
	err := c.Begin(OwnerCalibration)
	if err == nil {
		t.Fatal("expected error for different owner")
	}
	if !errors.Is(err, apperrors.ErrWorkflowConflict) {
		t.Fatalf("expected ErrWorkflowConflict, got %v", err)
	}
}

func TestBeginFromErrorSucceeds(t *testing.T) {
	c := NewWorkflowCoordinator()
	if err := c.Begin(OwnerMeasurement); err != nil {
		t.Fatalf("Begin: %v", err)
	}
	if err := c.Fail(); err != nil {
		t.Fatalf("Fail: %v", err)
	}
	if err := c.Begin(OwnerCalibration); err != nil {
		t.Fatalf("Begin after Fail should succeed: %v", err)
	}
}

func TestBeginRejectedWhenManualInterventionRequired(t *testing.T) {
	c := NewWorkflowCoordinator()
	if err := c.Begin(OwnerMeasurement); err != nil {
		t.Fatalf("Begin: %v", err)
	}
	if err := c.Fail(); err != nil {
		t.Fatalf("Fail: %v", err)
	}
	if err := c.NotifySafetyReleaseFailed(); err != nil {
		t.Fatalf("NotifySafetyReleaseFailed: %v", err)
	}
	err := c.Begin(OwnerMeasurement)
	if err == nil {
		t.Fatal("expected error when manual intervention required")
	}
	if !errors.Is(err, apperrors.ErrManualInterventionRequired) {
		t.Fatalf("expected ErrManualInterventionRequired, got %v", err)
	}
}

func TestEndClearsState(t *testing.T) {
	c := NewWorkflowCoordinator()
	if err := c.Begin(OwnerMeasurement); err != nil {
		t.Fatalf("Begin: %v", err)
	}
	c.End()
	if c.State() != domain.SessionStateStopped {
		t.Fatalf("expected stopped after End, got %s", c.State())
	}
	if c.Owner() != "" {
		t.Fatalf("expected empty owner after End, got %s", c.Owner())
	}
	if c.CtxID() != "" {
		t.Fatal("expected empty ctxID after End")
	}
	if c.HasActiveWorkflow() {
		t.Fatal("should not have active workflow after End")
	}
}

func TestEndFromIdleIsSafe(t *testing.T) {
	c := NewWorkflowCoordinator()
	c.End()
	if c.State() != domain.SessionStateIdle {
		t.Fatalf("expected idle, got %s", c.State())
	}
}

func TestFailTransitionsToError(t *testing.T) {
	c := NewWorkflowCoordinator()
	if err := c.Begin(OwnerMeasurement); err != nil {
		t.Fatalf("Begin: %v", err)
	}
	if err := c.Fail(); err != nil {
		t.Fatalf("Fail: %v", err)
	}
	if c.State() != domain.SessionStateError {
		t.Fatalf("expected error, got %s", c.State())
	}
	if c.Owner() != "" {
		t.Fatalf("expected empty owner after Fail, got %s", c.Owner())
	}
	if c.HasActiveWorkflow() {
		t.Fatal("error should not be active")
	}
}

func TestNotifySafetyReleaseFailedFromError(t *testing.T) {
	c := NewWorkflowCoordinator()
	if err := c.Begin(OwnerMeasurement); err != nil {
		t.Fatalf("Begin: %v", err)
	}
	if err := c.Fail(); err != nil {
		t.Fatalf("Fail: %v", err)
	}
	if err := c.NotifySafetyReleaseFailed(); err != nil {
		t.Fatalf("NotifySafetyReleaseFailed from error: %v", err)
	}
	if c.State() != domain.SessionStateRequiresManualIntervention {
		t.Fatalf("expected requires_manual_intervention, got %s", c.State())
	}
	if !c.IsManualInterventionRequired() {
		t.Fatal("IsManualInterventionRequired should be true")
	}
}

func TestConfirmManualIntervention(t *testing.T) {
	c := NewWorkflowCoordinator()
	if err := c.Begin(OwnerMeasurement); err != nil {
		t.Fatalf("Begin: %v", err)
	}
	if err := c.Fail(); err != nil {
		t.Fatalf("Fail: %v", err)
	}
	if err := c.NotifySafetyReleaseFailed(); err != nil {
		t.Fatalf("NotifySafetyReleaseFailed: %v", err)
	}
	if err := c.ConfirmManualIntervention(); err != nil {
		t.Fatalf("ConfirmManualIntervention: %v", err)
	}
	if c.State() != domain.SessionStateIdle {
		t.Fatalf("expected idle after confirm, got %s", c.State())
	}
	if c.Owner() != "" {
		t.Fatalf("expected empty owner after confirm, got %s", c.Owner())
	}
}

func TestConfirmManualInterventionRejectedWhenNotInState(t *testing.T) {
	c := NewWorkflowCoordinator()
	err := c.ConfirmManualIntervention()
	if err == nil {
		t.Fatal("expected error when not in manual intervention state")
	}
}

func TestBeginAfterEnd(t *testing.T) {
	c := NewWorkflowCoordinator()
	if err := c.Begin(OwnerCalibration); err != nil {
		t.Fatalf("Begin: %v", err)
	}
	c.End()
	if err := c.Begin(OwnerCalibration); err != nil {
		t.Fatalf("Begin after End should succeed: %v", err)
	}
}

func TestBeginAfterConfirmManualIntervention(t *testing.T) {
	c := NewWorkflowCoordinator()
	if err := c.Begin(OwnerMeasurement); err != nil {
		t.Fatalf("Begin: %v", err)
	}
	if err := c.Fail(); err != nil {
		t.Fatalf("Fail: %v", err)
	}
	if err := c.NotifySafetyReleaseFailed(); err != nil {
		t.Fatalf("NotifySafetyReleaseFailed: %v", err)
	}
	if err := c.ConfirmManualIntervention(); err != nil {
		t.Fatalf("ConfirmManualIntervention: %v", err)
	}
	if err := c.Begin(OwnerMeasurement); err != nil {
		t.Fatalf("Begin after confirm should succeed: %v", err)
	}
}

func TestIsActiveState(t *testing.T) {
	activeStates := []domain.SessionState{
		domain.SessionStateReady,
		domain.SessionStatePaused,
		domain.SessionStatePressurizing,
		domain.SessionStateStabilizing,
		domain.SessionStateCollecting,
		domain.SessionStateAwaitAlarmResolution,
		domain.SessionStateAwaitManualCollect,
		domain.SessionStatePointDone,
		domain.SessionStateFitting,
	}
	for _, st := range activeStates {
		if !isActiveState(st) {
			t.Fatalf("expected %s to be active", st)
		}
	}
	inactiveStates := []domain.SessionState{
		domain.SessionStateIdle,
		domain.SessionStateStopped,
		domain.SessionStateCompleted,
		domain.SessionStateError,
		domain.SessionStateRequiresManualIntervention,
	}
	for _, st := range inactiveStates {
		if isActiveState(st) {
			t.Fatalf("expected %s to be inactive", st)
		}
	}
}
