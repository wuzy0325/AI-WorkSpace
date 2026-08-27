package measurement

import (
	"context"
	"testing"
	"time"
)

func newPendingTestService() *Service {
	return &Service{
		stabilityTimeoutCh: make(chan string, 1),
		publish:            func(string, any) {},
	}
}

// TestStabilityTimeoutPendingQueryable 验证稳定超时挂起状态可查询：
// 后端阻塞等待用户决策期间，页面刷新/崩溃恢复后前端可通过
// GET /measurement/stability-timeout/pending 重新弹窗，避免流程卡死。
func TestStabilityTimeoutPendingQueryable(t *testing.T) {
	svc := newPendingTestService()

	if pending, idx := svc.GetStabilityTimeoutPending(); pending || idx != 0 {
		t.Fatalf("expected no pending initially, got pending=%v pointIndex=%d", pending, idx)
	}

	svc.mu.Lock()
	svc.stabilityTimeoutPending = true
	svc.stabilityTimeoutPointIndex = 3
	svc.mu.Unlock()

	pending, idx := svc.GetStabilityTimeoutPending()
	if !pending || idx != 3 {
		t.Fatalf("expected pending pointIndex=3, got pending=%v pointIndex=%d", pending, idx)
	}
}

// TestResolveStabilityTimeoutGatedWhenNotPending 验证无挂起时决策投递被忽略，
// 防止决策滞留通道后被下一次超时误消费（跳过点未弹窗）。
func TestResolveStabilityTimeoutGatedWhenNotPending(t *testing.T) {
	svc := newPendingTestService()

	svc.ResolveStabilityTimeout("skip")

	select {
	case d := <-svc.stabilityTimeoutCh:
		t.Fatalf("expected no queued decision when not pending, got %q", d)
	default:
	}
}

// TestAwaitStabilityTimeoutDecisionLifecycle 验证阻塞等待期间挂起状态置位、
// 收到决策或 context 取消后正确清理。
func TestAwaitStabilityTimeoutDecisionLifecycle(t *testing.T) {
	t.Run("resolve clears pending and delivers continue", func(t *testing.T) {
		svc := newPendingTestService()
		done := make(chan error, 1)
		go func() { done <- svc.awaitStabilityTimeoutDecision(context.Background(), 2) }()

		waitUntil(t, time.Second, func() bool {
			pending, idx := svc.GetStabilityTimeoutPending()
			return pending && idx == 2
		})

		svc.ResolveStabilityTimeout("continue")

		select {
		case err := <-done:
			if err != nil {
				t.Fatalf("awaitStabilityTimeoutDecision: %v", err)
			}
		case <-time.After(time.Second):
			t.Fatal("decision was not delivered to blocked waiter")
		}

		if pending, _ := svc.GetStabilityTimeoutPending(); pending {
			t.Fatal("expected pending cleared after decision")
		}
	})

	t.Run("skip returns ErrPointSkipped", func(t *testing.T) {
		svc := newPendingTestService()
		done := make(chan error, 1)
		go func() { done <- svc.awaitStabilityTimeoutDecision(context.Background(), 1) }()

		waitUntil(t, time.Second, func() bool {
			pending, _ := svc.GetStabilityTimeoutPending()
			return pending
		})

		svc.ResolveStabilityTimeout("skip")

		select {
		case err := <-done:
			if err != ErrPointSkipped {
				t.Fatalf("expected ErrPointSkipped, got %v", err)
			}
		case <-time.After(time.Second):
			t.Fatal("decision was not delivered to blocked waiter")
		}
	})

	t.Run("context cancel clears pending", func(t *testing.T) {
		svc := newPendingTestService()
		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan error, 1)
		go func() { done <- svc.awaitStabilityTimeoutDecision(ctx, 1) }()

		waitUntil(t, time.Second, func() bool {
			pending, _ := svc.GetStabilityTimeoutPending()
			return pending
		})

		cancel()

		select {
		case <-done:
		case <-time.After(time.Second):
			t.Fatal("awaitStabilityTimeoutDecision did not return after cancel")
		}

		if pending, _ := svc.GetStabilityTimeoutPending(); pending {
			t.Fatal("expected pending cleared after cancel")
		}
	})
}

// TestGetCurrentAlarmSnapshot 验证报警详情查询返回隔离快照，
// 页面刷新后前端可据此恢复报警弹窗与自动放行判断。
func TestGetCurrentAlarmSnapshot(t *testing.T) {
	svc := newPendingTestService()

	if alarm := svc.GetCurrentAlarm(); alarm != nil {
		t.Fatalf("expected nil alarm when nothing pending, got %+v", alarm)
	}

	svc.mu.Lock()
	svc.alarmPending = true
	svc.currentAlarm = &Alarm{
		PointID:           "p1",
		DeviceID:          "d1",
		OverLimitChannels: []int{2, 4},
	}
	svc.mu.Unlock()

	got := svc.GetCurrentAlarm()
	if got == nil {
		t.Fatal("expected pending alarm snapshot")
	}
	if got.PointID != "p1" || got.DeviceID != "d1" {
		t.Fatalf("unexpected alarm snapshot: %+v", got)
	}

	// 快照必须与内部状态隔离，调用方修改不影响后续查询。
	got.OverLimitChannels[0] = 99
	if svc.currentAlarm.OverLimitChannels[0] != 2 {
		t.Fatal("GetCurrentAlarm returned an aliased slice instead of a snapshot")
	}

	svc.mu.Lock()
	svc.alarmPending = false
	svc.currentAlarm = nil
	svc.mu.Unlock()
	if alarm := svc.GetCurrentAlarm(); alarm != nil {
		t.Fatalf("expected nil alarm after resolve, got %+v", alarm)
	}
}

func waitUntil(t *testing.T, timeout time.Duration, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("condition not met before timeout")
}
