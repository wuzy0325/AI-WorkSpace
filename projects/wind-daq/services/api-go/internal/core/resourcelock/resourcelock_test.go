package resourcelock

import (
	"errors"
	"testing"
	"time"
)

func TestAcquire_FirstHolderSucceeds(t *testing.T) {
	s := New()
	if err := s.Acquire("workflow:traversal", "task-1", 0); err != nil {
		t.Fatalf("Acquire failed: %v", err)
	}
	held, holder := s.IsHeld("workflow:traversal")
	if !held || holder != "task-1" {
		t.Errorf("expected held by task-1, got held=%v holder=%q", held, holder)
	}
}

func TestAcquire_RejectsConcurrentDifferentHolder(t *testing.T) {
	s := New()
	if err := s.Acquire("workflow:traversal", "task-1", 0); err != nil {
		t.Fatalf("first Acquire failed: %v", err)
	}
	err := s.Acquire("workflow:traversal", "task-2", 0)
	if err == nil {
		t.Fatal("expected ErrLockHeld for second holder")
	}
	if !errors.Is(err, ErrLockHeld) {
		t.Errorf("expected ErrLockHeld, got %v", err)
	}
}

func TestAcquire_SameHolderRenews(t *testing.T) {
	s := New()
	if err := s.Acquire("workflow:traversal", "task-1", time.Second); err != nil {
		t.Fatalf("Acquire failed: %v", err)
	}
	// 同一 holder 再次 Acquire 视为续约，不应报错
	if err := s.Acquire("workflow:traversal", "task-1", 5*time.Second); err != nil {
		t.Fatalf("renew failed: %v", err)
	}
}

func TestAcquire_TakesOverExpiredLock(t *testing.T) {
	s := New()
	if err := s.Acquire("workflow:traversal", "task-1", 1*time.Millisecond); err != nil {
		t.Fatalf("first Acquire failed: %v", err)
	}
	time.Sleep(5 * time.Millisecond) // 等待过期
	if err := s.Acquire("workflow:traversal", "task-2", 0); err != nil {
		t.Errorf("expected takeover after expiry, got %v", err)
	}
	_, holder := s.IsHeld("workflow:traversal")
	if holder != "task-2" {
		t.Errorf("expected new holder task-2, got %q", holder)
	}
}

func TestRelease_HolderMismatchRejected(t *testing.T) {
	s := New()
	_ = s.Acquire("workflow:traversal", "task-1", 0)
	if err := s.Release("workflow:traversal", "task-2"); err == nil {
		t.Error("expected release-denied when holder mismatches")
	}
	// 锁仍在
	if held, _ := s.IsHeld("workflow:traversal"); !held {
		t.Error("lock should still be held after rejected release")
	}
}

func TestRelease_CorrectHolderClears(t *testing.T) {
	s := New()
	_ = s.Acquire("workflow:traversal", "task-1", 0)
	if err := s.Release("workflow:traversal", "task-1"); err != nil {
		t.Errorf("Release failed: %v", err)
	}
	if held, _ := s.IsHeld("workflow:traversal"); held {
		t.Error("lock should be released")
	}
}

func TestRelease_MissingLockIsNoop(t *testing.T) {
	s := New()
	if err := s.Release("nonexistent", "anyone"); err != nil {
		t.Errorf("Release on missing lock should be no-op, got %v", err)
	}
}

func TestRelease_ExpiredLockClearedSilently(t *testing.T) {
	s := New()
	_ = s.Acquire("workflow:traversal", "task-1", 1*time.Millisecond)
	time.Sleep(5 * time.Millisecond)
	// holder 不匹配但已过期 → 应当作幂等成功
	if err := s.Release("workflow:traversal", "task-2"); err != nil {
		t.Errorf("Release on expired lock should be no-op, got %v", err)
	}
}

func TestAcquire_ValidatesInputs(t *testing.T) {
	s := New()
	if err := s.Acquire("", "task-1", 0); err == nil {
		t.Error("expected error on empty resource")
	}
	if err := s.Acquire("workflow:traversal", "", 0); err == nil {
		t.Error("expected error on empty holder")
	}
}

func TestDefault_ReturnsSingleton(t *testing.T) {
	a := Default()
	b := Default()
	if a != b {
		t.Error("Default should return the same singleton")
	}
}
