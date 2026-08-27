package workflow_test

import (
	"testing"

	"cal1604/internal/domain"
	"cal1604/internal/workflow"
)

func TestSessionStateTransition(t *testing.T) {
	m := workflow.NewSessionMachine()

	if got := m.State(); got != domain.SessionStateIdle {
		t.Fatalf("expected initial state idle, got %s", got)
	}

	if err := m.Transition(domain.SessionStateReady); err != nil {
		t.Fatalf("unexpected transition error: %v", err)
	}

	if got := m.State(); got != domain.SessionStateReady {
		t.Fatalf("expected state ready, got %s", got)
	}
}

func TestSessionStateTransitionRejectsIllegalPath(t *testing.T) {
	m := workflow.NewSessionMachine()

	if err := m.Transition(domain.SessionStateCompleted); err == nil {
		t.Fatal("expected illegal transition to fail")
	}
}
