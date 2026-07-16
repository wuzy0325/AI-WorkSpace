package monitor

import (
	"context"
	"fmt"
	"runtime"
	"sync"
	"testing"
	"time"

	"shared.local/device-sdk/go/motion/core"
)

// waitForGoroutineCount 等待 goroutine 数降至 target+tolerance 或更低，或超时失败。
//
// 设计理由：goroutine 退出到 runtime.NumGoroutine() 反映有延迟（Go runtime 在 schedule 时清理），
// 短轮询（5ms 间隔，含 runtime.Gosched 让出）+ 1s 超时是确定性兜底。
// 容忍 +2 误差：Go runtime 后台 goroutine（GC marker、finalizer 等）+ 测试辅助 goroutine 波动。
//
// 不用 time.Sleep 断言时序——这里是用短轮询等待异步 goroutine 退出，符合"注入 clock/ticker"精神。
func waitForGoroutineCount(t *testing.T, target int, desc string) {
	t.Helper()
	deadline := time.After(1 * time.Second)
	for {
		current := runtime.NumGoroutine()
		if current <= target+2 {
			return
		}
		select {
		case <-deadline:
			t.Fatalf("%s: goroutine count did not drop to <=%d (tolerance +2) within 1s, got %d", desc, target, current)
		case <-time.After(5 * time.Millisecond):
			runtime.Gosched()
		}
	}
}

// TestMonitorNoLeakAfterUnregister 验证 UnregisterController 后 pollLoop goroutine 退出。
//
// spec Interface Contract: Unregister 后该控制器不再采集。
// Task 5 验收: Disconnect/Shutdown 无 goroutine 泄漏。
// UnregisterController 通过 <-cs.done 同步等待 pollLoop goroutine 退出，
// 返回时 goroutine 应已退出；本测试用 runtime.NumGoroutine() 兜底验证。
func TestMonitorNoLeakAfterUnregister(t *testing.T) {
	before := runtime.NumGoroutine()

	clock := NewFakeClock(time.Now())
	m := NewMotionStatusMonitor(DefaultConfig(), clock, DefaultFreshnessPolicy{
		StaleThreshold: 1 * time.Second,
	})
	fc := newFakeController("c1")
	statusCh := make(chan struct{}, 16)
	fc.statusCalled = statusCh

	m.RegisterController("c1", fc)

	// 触发首帧，确保 pollLoop goroutine 已启动
	ticker := clock.LatestTicker()
	ticker.Fire(clock.Now())
	waitForStatusCall(t, statusCh, "initial poll")
	waitForSnapshot(t, m, "c1", 1)

	m.UnregisterController("c1")

	// goroutine 数应回归到 before（容忍 +2 误差）
	waitForGoroutineCount(t, before, "after Unregister")
}

// TestMonitorNoLeakAfterShutdown 验证 Shutdown 后所有 pollLoop goroutine 退出。
//
// spec: 应用关闭时 cancel 根 context，等待所有 monitor goroutine 退出。
// Shutdown 通过 rootCancel + 逐个 <-cs.done 同步等待所有 pollLoop goroutine。
func TestMonitorNoLeakAfterShutdown(t *testing.T) {
	before := runtime.NumGoroutine()

	clock := NewFakeClock(time.Now())
	m := NewMotionStatusMonitor(DefaultConfig(), clock, DefaultFreshnessPolicy{
		StaleThreshold: 1 * time.Second,
	})

	const count = 3
	for i := 0; i < count; i++ {
		id := fmt.Sprintf("c%d", i)
		fc := newFakeController(id)
		m.RegisterController(id, fc)
	}

	m.Shutdown()

	waitForGoroutineCount(t, before, "after Shutdown")
}

// TestMonitorNoLeakAfterSubscribeCancel 验证 Subscribe 清理 goroutine 在 ctx 取消后退出。
//
// spec Subscribe: "context 取消后关闭订阅 channel 并移除订阅者"。
// 清理 goroutine 等待 ctx.Done 后持锁 delete + close；ctx 取消后应退出。
// 本测试验证清理 goroutine 不泄漏。
func TestMonitorNoLeakAfterSubscribeCancel(t *testing.T) {
	before := runtime.NumGoroutine()

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
	waitForStatusCall(t, statusCh, "initial poll")
	waitForSnapshot(t, m, "c1", 1)

	ctx, cancel := context.WithCancel(context.Background())
	ch := m.Subscribe(ctx)
	<-ch // 消费立即投递

	cancel()

	// 等待 subscribers map 清空（清理 goroutine 完成 delete+close）
	deadline := time.After(1 * time.Second)
	for {
		m.mu.RLock()
		remaining := len(m.subscribers)
		m.mu.RUnlock()
		if remaining == 0 {
			break
		}
		select {
		case <-deadline:
			t.Fatalf("subscribers not cleaned up after cancel: %d remaining", remaining)
		case <-time.After(time.Millisecond):
		}
	}

	// goroutine 数应回归（清理 goroutine 已退出）
	waitForGoroutineCount(t, before, "after Subscribe cancel")
}

// TestMonitorConcurrentMixedOperations 验证并发操作不 panic 不 race。
//
// 并发场景（模拟生产环境多消费者）：
//   - 多控制器 Register/Unregister（模拟连接/断开）
//   - 多订阅者 Subscribe/cancel（模拟 UI 挂载/卸载）
//   - RequestRefresh / NotifyCommandExecuted 持续触发（模拟业务命令后刷新）
//   - ticker 持续 Fire（模拟时间推进触发周期轮询）
//
// -race 应无报警，无 panic。opDuration=200ms 足够覆盖 Register→操作→Unregister 全周期。
func TestMonitorConcurrentMixedOperations(t *testing.T) {
	clock := NewFakeClock(time.Now())
	m := NewMotionStatusMonitor(DefaultConfig(), clock, DefaultFreshnessPolicy{
		StaleThreshold: 1 * time.Second,
	})

	const controllerCount = 4
	const opDuration = 200 * time.Millisecond

	var wg sync.WaitGroup

	// 注册者/注销者：每个控制器注册后等待 opDuration 再注销
	wg.Add(controllerCount)
	for i := 0; i < controllerCount; i++ {
		go func(idx int) {
			defer wg.Done()
			id := fmt.Sprintf("c%d", idx)
			fc := newFakeController(id)
			m.RegisterController(id, fc)
			time.Sleep(opDuration)
			m.UnregisterController(id)
		}(i)
	}

	// 订阅者：Subscribe 后消费或超时取消
	const subscriberCount = 8
	for i := 0; i < subscriberCount; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			ch := m.Subscribe(ctx)
			deadline := time.After(opDuration)
			for {
				select {
				case <-ch:
				case <-deadline:
					return
				}
			}
		}()
	}

	// RequestRefresh / NotifyCommandExecuted 持续触发
	for i := 0; i < controllerCount; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			id := fmt.Sprintf("c%d", idx)
			deadline := time.After(opDuration)
			for {
				select {
				case <-deadline:
					return
				default:
				}
				m.RequestRefresh(id)
				m.NotifyCommandExecuted(id, CmdKindMove)
				m.NotifyCommandExecuted(id, CmdKindStop)
				time.Sleep(time.Millisecond)
			}
		}(i)
	}

	// ticker 持续 Fire（触发 pollOnce + 自适应频率切换）
	wg.Add(1)
	go func() {
		defer wg.Done()
		deadline := time.After(opDuration)
		for {
			select {
			case <-deadline:
				return
			default:
			}
			clock.TickAll()
			time.Sleep(time.Millisecond)
		}
	}()

	wg.Wait()
	m.Shutdown()
}

// 编译期断言：core 包被使用（newFakeController 内部用 core.MotionControllerProfile）。
var _ = core.AxisX
