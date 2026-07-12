package usecase

import (
	"context"
	"testing"
	"time"

	"wind-daq/services/api-go/internal/core/traversal"
)

func TestTraversalSessionCancelAndDoneAreIdempotent(t *testing.T) {
	session := newTraversalRunSession(context.Background(), "task-1", traversal.TraversalRunSnapshot{})
	session.Cancel()
	session.Cancel()
	session.MarkDone()
	session.MarkDone()
	select {
	case <-session.Context().Done():
	default:
		t.Fatal("session context must be cancelled")
	}
	select {
	case <-session.Done():
	default:
		t.Fatal("session done channel must be closed")
	}
}

func TestTraversalOwnershipRejectsStaleTaskUpdate(t *testing.T) {
	manager := NewTraversalManager(nil, nil, nil, nil, nil)
	manager.mu.Lock()
	manager.session = newTraversalRunSession(context.Background(), "task-new", traversal.TraversalRunSnapshot{})
	manager.status = traversal.Status{TaskID: "task-new", State: traversal.StateRunning}
	manager.mu.Unlock()

	manager.updatePhase("task-old", traversal.StateMoving, traversal.PhaseMoving, 3, 10)
	status := manager.Status()
	if status.State != traversal.StateRunning || status.CurrentPointIndex != 0 {
		t.Fatalf("stale task changed active state: %+v", status)
	}
}

func TestTraversalStateDoesNotOverwriteProtectedState(t *testing.T) {
	protected := []traversal.State{traversal.StatePaused, traversal.StateStopped, traversal.StateError}
	for _, state := range protected {
		t.Run(string(state), func(t *testing.T) {
			manager := NewTraversalManager(nil, nil, nil, nil, nil)
			manager.mu.Lock()
			manager.session = newTraversalRunSession(context.Background(), "task-1", traversal.TraversalRunSnapshot{})
			manager.status = traversal.Status{TaskID: "task-1", State: state}
			manager.mu.Unlock()

			manager.updatePhase("task-1", traversal.StateAcquiring, traversal.PhaseAcquiring, 2, 5)
			if got := manager.Status().State; got != state {
				t.Fatalf("protected state changed from %s to %s", state, got)
			}
		})
	}
}

func TestTraversalSessionAllowsOnlyOneActiveOwner(t *testing.T) {
	manager := NewTraversalManager(nil, nil, nil, nil, nil)
	first, err := manager.beginSession(context.Background(), "task-1", traversal.TraversalRunSnapshot{})
	if err != nil {
		t.Fatalf("begin first session: %v", err)
	}
	if _, err := manager.beginSession(context.Background(), "task-2", traversal.TraversalRunSnapshot{}); err == nil {
		t.Fatal("expected second active session to be rejected")
	}
	first.MarkDone()
	if _, err := manager.beginSession(context.Background(), "task-2", traversal.TraversalRunSnapshot{}); err != nil {
		t.Fatalf("begin replacement session after done: %v", err)
	}
}

func TestStartRejectsActiveSessionEvenWhenPublicStateIsTerminal(t *testing.T) {
	manager := NewTraversalManager(nil, nil, nil, nil, nil)
	manager.mu.Lock()
	active := newTraversalRunSession(context.Background(), "task-active", traversal.TraversalRunSnapshot{})
	manager.session = active
	manager.status = traversal.Status{TaskID: "task-active", State: traversal.StateStopped}
	manager.mu.Unlock()

	err := manager.Start(traversal.Config{
		TaskID:   "task-next",
		DeviceID: "device-1",
		Channels: []int{0},
		Path:     []traversal.Point{{X: 1}},
	})
	if err == nil {
		t.Fatal("expected Start to reject while previous session is still active")
	}
	manager.mu.RLock()
	current := manager.session
	manager.mu.RUnlock()
	if current != active {
		t.Fatal("Start replaced the active session")
	}
}

func TestRunTraversalLoopMarksSessionDoneOnEveryExit(t *testing.T) {
	manager := NewTraversalManager(nil, nil, nil, nil, nil)
	session := newTraversalRunSession(context.Background(), "task-1", traversal.TraversalRunSnapshot{})
	manager.mu.Lock()
	manager.session = session
	manager.status = traversal.Status{TaskID: "task-1", State: traversal.StateStopped}
	manager.mu.Unlock()

	manager.RunTraversalLoop(time.Millisecond)

	select {
	case <-session.Done():
	default:
		t.Fatal("RunTraversalLoop must mark its session done before returning")
	}
}
