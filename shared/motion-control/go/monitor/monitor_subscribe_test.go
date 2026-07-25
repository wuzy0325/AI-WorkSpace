package monitor

import (
	"context"
	"sync"
	"testing"
	"time"

	"shared.local/device-sdk/go/motion/core"
)

// TestMonitorSubscribeReceivesInitialSnapshot 验证首帧已发布后 Subscribe 立即投递当前快照。
//
// spec Subscribe: "首次订阅立即投递当前快照（若存在）"。
// 测试先 publish 一帧，再 Subscribe，应立即从返回 channel 收到该快照，无需等待新 publish。
func TestMonitorSubscribeReceivesInitialSnapshot(t *testing.T) {
	clock := NewFakeClock(time.Now())
	m := NewMotionStatusMonitor(DefaultConfig(), clock, DefaultFreshnessPolicy{
		StaleThreshold: 1 * time.Second,
	})
	fc := newFakeController("c1")
	fc.setStatus(core.ControllerStatus{
		ID:        "c1",
		Connected: true,
		Axes:      []core.AxisStatus{{Name: core.AxisX, Position: 10.0, Moving: false}},
	})
	statusCh := make(chan struct{}, 16)
	fc.statusCalled = statusCh

	m.RegisterController("c1", fc)
	defer m.UnregisterController("c1")

	// 触发首帧发布
	ticker := clock.LatestTicker()
	ticker.Fire(clock.Now())
	waitForStatusCall(t, statusCh, "first poll")
	waitForSnapshot(t, m, "c1", 1)

	// Subscribe 后应立即收到当前快照
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ch := m.Subscribe(ctx)

	select {
	case snap := <-ch:
		if len(snap.Controllers) != 1 {
			t.Fatalf("len(Controllers) = %d, want 1", len(snap.Controllers))
		}
		if snap.Controllers[0].ControllerID != "c1" {
			t.Fatalf("ControllerID = %q, want c1", snap.Controllers[0].ControllerID)
		}
		if snap.Controllers[0].Sequence != 1 {
			t.Fatalf("Sequence = %d, want 1", snap.Controllers[0].Sequence)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("Subscribe did not receive initial snapshot within 500ms")
	}
}

// TestMonitorSubscribeReceivesUpdates 验证订阅者在新快照发布后收到更新。
//
// spec Subscribe: "新快照到达而旧快照未消费时覆盖/丢弃旧快照"。
// 测试 Subscribe 立即投递后，再触发一次 publish，订阅者应收到第二个快照。
func TestMonitorSubscribeReceivesUpdates(t *testing.T) {
	clock := NewFakeClock(time.Now())
	m := NewMotionStatusMonitor(DefaultConfig(), clock, DefaultFreshnessPolicy{
		StaleThreshold: 1 * time.Second,
	})
	fc := newFakeController("c1")
	fc.setStatus(core.ControllerStatus{
		ID:        "c1",
		Connected: true,
		Axes:      []core.AxisStatus{{Name: core.AxisX, Position: 10.0, Moving: false}},
	})
	statusCh := make(chan struct{}, 16)
	fc.statusCalled = statusCh

	m.RegisterController("c1", fc)
	defer m.UnregisterController("c1")

	ticker := clock.LatestTicker()
	ticker.Fire(clock.Now())
	waitForStatusCall(t, statusCh, "first poll")
	waitForSnapshot(t, m, "c1", 1)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ch := m.Subscribe(ctx)

	// 消费首帧立即投递
	<-ch

	// 推进时钟，更新位置后触发第二次 publish
	clock.Advance(10 * time.Millisecond)
	fc.setStatus(core.ControllerStatus{
		ID:        "c1",
		Connected: true,
		Axes:      []core.AxisStatus{{Name: core.AxisX, Position: 20.0, Moving: true}},
	})
	ticker.Fire(clock.Now())
	waitForStatusCall(t, statusCh, "second poll")
	waitForSnapshot(t, m, "c1", 2)

	// 应收到第二个快照
	select {
	case snap := <-ch:
		if snap.Controllers[0].Sequence != 2 {
			t.Fatalf("Sequence = %d, want 2", snap.Controllers[0].Sequence)
		}
		if snap.Controllers[0].Status.Axes[0].Position != 20.0 {
			t.Fatalf("Position = %v, want 20.0", snap.Controllers[0].Status.Axes[0].Position)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("Subscribe did not receive update within 500ms")
	}
}

// TestMonitorSubscribeLatestOnly 验证慢订阅者只看到最新快照（latest-only 语义）。
//
// spec Subscribe: "新快照到达而旧快照未消费时覆盖/丢弃旧快照，不阻塞采集循环"。
// 测试 Subscribe 后不消费 channel，连续触发多帧 publish，最终订阅者应只看到最新一帧
//（而非中间所有帧），且 channel 不应因积压阻塞采集循环。
func TestMonitorSubscribeLatestOnly(t *testing.T) {
	clock := NewFakeClock(time.Now())
	m := NewMotionStatusMonitor(DefaultConfig(), clock, DefaultFreshnessPolicy{
		StaleThreshold: 1 * time.Second,
	})
	fc := newFakeController("c1")
	statusCh := make(chan struct{}, 16)
	fc.statusCalled = statusCh

	m.RegisterController("c1", fc)
	defer m.UnregisterController("c1")

	ticker := clock.LatestTicker()

	// 首帧建立 lastSnap
	fc.setStatus(core.ControllerStatus{
		ID:        "c1",
		Connected: true,
		Axes:      []core.AxisStatus{{Name: core.AxisX, Position: 0.0, Moving: false}},
	})
	ticker.Fire(clock.Now())
	waitForStatusCall(t, statusCh, "first poll")
	waitForSnapshot(t, m, "c1", 1)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ch := m.Subscribe(ctx)
	<-ch // 消费首帧立即投递

	// 连续触发 3 帧 publish，不消费 channel
	for i := 1; i <= 3; i++ {
		clock.Advance(10 * time.Millisecond)
		fc.setStatus(core.ControllerStatus{
			ID:        "c1",
			Connected: true,
			Axes:      []core.AxisStatus{{Name: core.AxisX, Position: float64(i) * 10.0, Moving: true}},
		})
		ticker.Fire(clock.Now())
		waitForStatusCall(t, statusCh, "poll iteration")
		// 等待 publish 完成（按 sequence 推进）
		waitForSnapshot(t, m, "c1", uint64(1+i))
	}

	// 订阅者最终应看到最新一帧（Position=30.0），中间帧被覆盖
	// latest-only 语义下，channel 容量 1，最多收到 1 个额外快照
	select {
	case snap := <-ch:
		if snap.Controllers[0].Status.Axes[0].Position != 30.0 {
			t.Fatalf("Position = %v, want 30.0 (latest)", snap.Controllers[0].Status.Axes[0].Position)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("Subscribe did not receive latest snapshot within 500ms")
	}
}

// TestMonitorSubscribeContextCancelClosesChannel 验证 ctx 取消后 channel 关闭且订阅者被移除。
//
// spec Subscribe: "context 取消后关闭订阅 channel 并移除订阅者"。
// 测试 Subscribe 后取消 ctx，channel 应关闭（recv 返回 zero, false）。
func TestMonitorSubscribeContextCancelClosesChannel(t *testing.T) {
	clock := NewFakeClock(time.Now())
	m := NewMotionStatusMonitor(DefaultConfig(), clock, DefaultFreshnessPolicy{
		StaleThreshold: 1 * time.Second,
	})
	fc := newFakeController("c1")
	statusCh := make(chan struct{}, 16)
	fc.statusCalled = statusCh

	m.RegisterController("c1", fc)
	defer m.UnregisterController("c1")

	ticker := clock.LatestTicker()
	ticker.Fire(clock.Now())
	waitForStatusCall(t, statusCh, "first poll")
	waitForSnapshot(t, m, "c1", 1)

	ctx, cancel := context.WithCancel(context.Background())
	ch := m.Subscribe(ctx)
	<-ch // 消费立即投递

	cancel()

	// channel 应关闭：recv 返回 zero, false
	select {
	case _, ok := <-ch:
		if ok {
			t.Fatal("channel should be closed after ctx cancel")
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("channel close not observed within 500ms after cancel")
	}

	// 内部状态检查：subscribers map 应为空
	m.mu.RLock()
	remaining := len(m.subscribers)
	m.mu.RUnlock()
	if remaining != 0 {
		t.Fatalf("subscribers map should be empty after cancel, got %d", remaining)
	}
}

// TestMonitorSubscribeDoesNotBlockPolling 验证慢订阅者不阻塞采集循环。
//
// spec Subscribe: "不阻塞采集循环"。
// 测试：订阅者从不消费 channel，连续触发多帧 publish，每帧都应在合理时间内完成
//（通过 waitForSnapshot 验证 lastSnap sequence 推进）。若 publish 被订阅者 channel 阻塞，
// waitForSnapshot 会超时。
func TestMonitorSubscribeDoesNotBlockPolling(t *testing.T) {
	clock := NewFakeClock(time.Now())
	m := NewMotionStatusMonitor(DefaultConfig(), clock, DefaultFreshnessPolicy{
		StaleThreshold: 1 * time.Second,
	})
	fc := newFakeController("c1")
	statusCh := make(chan struct{}, 16)
	fc.statusCalled = statusCh

	m.RegisterController("c1", fc)
	defer m.UnregisterController("c1")

	ticker := clock.LatestTicker()
	ticker.Fire(clock.Now())
	waitForStatusCall(t, statusCh, "first poll")
	waitForSnapshot(t, m, "c1", 1)

	// 订阅后从不消费 channel（慢订阅者）
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	_ = m.Subscribe(ctx)

	// 连续触发 5 帧 publish，每帧都应推进 lastSnap
	for i := 1; i <= 5; i++ {
		clock.Advance(10 * time.Millisecond)
		ticker.Fire(clock.Now())
		waitForStatusCall(t, statusCh, "poll iteration")
		waitForSnapshot(t, m, "c1", uint64(1+i))
	}
}

// TestMonitorSubscribeConcurrentNoSendOnClosed 验证发布与订阅取消并发时不 send-on-closed 或 data race。
//
// spec Subscribe: "channel 只允许 monitor 所有者关闭；发布、注销和关闭必须经同一锁/事件循环串行化，
// 禁止 send-on-closed"。
// 测试：多个订阅者并发 Subscribe/取消，同时触发 publish，-race 不应报警，不应 panic。
func TestMonitorSubscribeConcurrentNoSendOnClosed(t *testing.T) {
	clock := NewFakeClock(time.Now())
	m := NewMotionStatusMonitor(DefaultConfig(), clock, DefaultFreshnessPolicy{
		StaleThreshold: 1 * time.Second,
	})
	fc := newFakeController("c1")
	statusCh := make(chan struct{}, 16)
	fc.statusCalled = statusCh

	m.RegisterController("c1", fc)
	defer m.UnregisterController("c1")

	ticker := clock.LatestTicker()
	ticker.Fire(clock.Now())
	waitForStatusCall(t, statusCh, "first poll")
	waitForSnapshot(t, m, "c1", 1)

	// 启动 N 个订阅者，各自在随机时刻取消
	const subscriberCount = 8
	var wg sync.WaitGroup
	ctxs := make([]context.Context, subscriberCount)
	cancels := make([]context.CancelFunc, subscriberCount)
	chs := make([]<-chan StatusSnapshot, subscriberCount)

	for i := 0; i < subscriberCount; i++ {
		ctx, cancel := context.WithCancel(context.Background())
		ctxs[i] = ctx
		cancels[i] = cancel
		chs[i] = m.Subscribe(ctx)
	}
	defer func() {
		for _, c := range cancels {
			c()
		}
	}()

	// 并发：每个订阅者在不同时间点取消，同时一个 goroutine 持续触发 publish
	wg.Add(subscriberCount + 1)

	// 发布者：触发多帧 publish
	go func() {
		defer wg.Done()
		for i := 0; i < 20; i++ {
			clock.Advance(5 * time.Millisecond)
			ticker.Fire(clock.Now())
			// 不等待 publish 完成——目的是制造并发压力
			time.Sleep(time.Millisecond)
		}
	}()

	// 订阅者：各自消费或取消
	for i := 0; i < subscriberCount; i++ {
		go func(idx int) {
			defer wg.Done()
			// 部分订阅者消费几帧后取消，部分立即取消
			deadline := time.After(time.Duration(idx) * time.Millisecond)
			for {
				select {
				case <-chs[idx]:
					// 消费一帧
				case <-deadline:
					cancels[idx]()
					return
				case <-ctxs[idx].Done():
					return
				}
			}
		}(i)
	}

	wg.Wait()

	// 兜底：所有订阅者已取消，subscribers map 应清空
	// 给清理 goroutine 一个宽限期
	deadline := time.After(500 * time.Millisecond)
	for {
		m.mu.RLock()
		remaining := len(m.subscribers)
		m.mu.RUnlock()
		if remaining == 0 {
			return
		}
		select {
		case <-deadline:
			t.Fatalf("subscribers map not empty after concurrent test: %d remaining", remaining)
		case <-time.After(5 * time.Millisecond):
		}
	}
}
