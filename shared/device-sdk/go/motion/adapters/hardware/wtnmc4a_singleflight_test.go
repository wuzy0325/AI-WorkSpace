//go:build windows

package hardware

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"shared.local/device-sdk/go/motion/core"
)

// TestWTNMC4AStatusConcurrentSharesSingleFlight 验证 WTNMC4A Status() 入口存在 single-flight 合并：
// 多个并发 Status() 调用只触发一轮 RR0 查询，避免多消费者在 ioMu 上排队后重复访问 DLL。
//
// 这是 B140 同名测试的 WTNMC4A 对应版本，对应 spec Decision 2 "一个控制器一个采集 flight"
// 在 WTNMC4A 上的最小实现验证。
//
// 测试前置：注入 readRR0 加 50ms 延迟并计数；readRR1/readLP 返回合法值
// 测试步骤：3 个并发 Status() 调用同时发起
// 期待结果：readRR0 只被调用 1 次；3 个调用者收到相同结果
func TestWTNMC4AStatusConcurrentSharesSingleFlight(t *testing.T) {
	ctrl := NewWTNMC4AMotionController(wtnmc4aTestProfile())
	ctrl.handle = 1
	ctrl.status.Connected = true

	// readRR0 加 50ms 延迟，扩大 single-flight 命中窗口。
	// 若无 single-flight，3 个并发调用会各自发起一轮 RR0。
	var rr0Calls atomic.Int32
	ctrl.readRR0 = func(uintptr) (rr0Status, error) {
		rr0Calls.Add(1)
		time.Sleep(50 * time.Millisecond)
		return rr0Status{}, nil // 所有 DRV=0，轴停止
	}
	ctrl.readRR1 = func(uintptr, int) (rr1Status, error) {
		return rr1Status{}, nil
	}
	ctrl.readLP = func(uintptr, int) int32 { return 1000 }

	const concurrency = 3
	results := make([]core.ControllerStatus, concurrency)
	errs := make([]error, concurrency)
	start := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(concurrency)
	for i := 0; i < concurrency; i++ {
		go func(idx int) {
			defer wg.Done()
			<-start
			results[idx], errs[idx] = ctrl.Status(context.Background())
		}(i)
	}
	close(start)
	wg.Wait()

	// 验收 1：只发送 1 轮 RR0 查询（single-flight 合并）
	if got := rr0Calls.Load(); got != 1 {
		t.Fatalf("RR0 calls = %d, want 1 (single-flight should share one flight)", got)
	}

	// 验收 2：所有调用者收到相同结果
	for i := 0; i < concurrency; i++ {
		if errs[i] != nil {
			t.Fatalf("Status[%d] err = %v, want nil", i, errs[i])
		}
		if results[i].Axes[0].Position != results[0].Axes[0].Position {
			t.Fatalf("Status[%d].Position = %v, want %v (shared result)",
				i, results[i].Axes[0].Position, results[0].Axes[0].Position)
		}
	}
}

// TestWTNMC4AStatusSingleFlightErrorPropagatedToWaiters 验证 in-flight 失败时，
// 所有等待者收到同一错误而非各自重新发起查询。
//
// 测试前置：readRR0 返回错误并加 50ms 延迟确保多个调用者落在同一 flight
// 测试步骤：3 个并发 Status() 调用同时发起
// 期待结果：readRR0 只被调用 1 次；3 个调用者都收到错误且消息一致
func TestWTNMC4AStatusSingleFlightErrorPropagatedToWaiters(t *testing.T) {
	ctrl := NewWTNMC4AMotionController(wtnmc4aTestProfile())
	ctrl.handle = 1
	ctrl.status.Connected = true

	sentinelErr := errors.New("WTNMC4A RR0 读取失败（模拟）")
	var rr0Calls atomic.Int32
	ctrl.readRR0 = func(uintptr) (rr0Status, error) {
		rr0Calls.Add(1)
		time.Sleep(50 * time.Millisecond)
		return rr0Status{}, sentinelErr
	}
	ctrl.readRR1 = func(uintptr, int) (rr1Status, error) {
		return rr1Status{}, nil
	}
	ctrl.readLP = func(uintptr, int) int32 { return 1000 }

	const concurrency = 3
	errs := make([]error, concurrency)
	start := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(concurrency)
	for i := 0; i < concurrency; i++ {
		go func(idx int) {
			defer wg.Done()
			<-start
			_, errs[idx] = ctrl.Status(context.Background())
		}(i)
	}
	close(start)
	wg.Wait()

	// 验收 1：只发送 1 轮 RR0（single-flight 合并失败查询）
	if got := rr0Calls.Load(); got != 1 {
		t.Fatalf("RR0 calls = %d, want 1 (single-flight should share failed flight)", got)
	}

	// 验收 2：所有调用者收到错误（而非只有发起者收到错误）
	for i := 0; i < concurrency; i++ {
		if errs[i] == nil {
			t.Fatalf("Status[%d] err = nil, want non-nil (error should propagate to waiters)", i)
		}
		// 错误消息应一致（来自同一 flight 的快照）
		if !errors.Is(errs[i], sentinelErr) {
			t.Fatalf("Status[%d] err = %v, want sentinel error (shared identical error)", i, errs[i])
		}
	}
}

// TestWTNMC4AStatusSingleFlightDoesNotBlockMoveTo 验证 single-flight 等待者不重复
// 触发 Status 查询：MoveTo 命令能在 Status() 完成后立即下发，不会因等待者在
// ioMu 上重复排队而放大 RR0 调用次数。
//
// 对应 B140 同名测试，验证 spec Decision 16 "不替换驱动串行锁" + "防饥饿"
// 在 WTNMC4A 上的前置基础。
//
// 测试前置：readRR0 加 80ms 延迟并计数；readLP/startMove 注入合法值让 MoveTo 可执行
// 测试步骤：启动 2 个并发 Status()，等发起者进入 RR0 后调用 MoveTo
// 期待结果：RR0 只被调用 1 次（发起者）；MoveTo 成功完成
func TestWTNMC4AStatusSingleFlightDoesNotBlockMoveTo(t *testing.T) {
	profile := wtnmc4aTestProfile()
	profile.Axes[0].MinLimit = core.PtrFloat64(-100)
	profile.Axes[0].MaxLimit = core.PtrFloat64(100)
	ctrl := NewWTNMC4AMotionController(profile)
	ctrl.handle = 1
	ctrl.status.Connected = true
	ctrl.speedParams[0] = &axisSpeedParams{
		DriveSpeed: 100, StartSpeed: 10,
		Acceleration: 500, Deceleration: 500,
		AccIncRate: 1000, DecIncRate: 1000, Multiple: 1,
	}
	ctrl.trustedPositions[0] = trustedPositionSample{pulse: 0, at: time.Now()}

	// readRR0 加 80ms 延迟，确保 Status() 进行中时 MoveTo 进入并发场景。
	var rr0Calls atomic.Int32
	ctrl.readRR0 = func(uintptr) (rr0Status, error) {
		rr0Calls.Add(1)
		time.Sleep(80 * time.Millisecond)
		return rr0Status{}, nil
	}
	ctrl.readRR1 = func(uintptr, int) (rr1Status, error) {
		return rr1Status{}, nil
	}
	ctrl.readLP = func(uintptr, int) int32 { return 0 }

	var starts atomic.Int32
	ctrl.startMove = func(int, int32) error {
		starts.Add(1)
		return nil
	}

	// 启动 2 个并发 Status() 调用（A 是发起者，B 是等待者）
	statusStart := make(chan struct{})
	var statusWg sync.WaitGroup
	statusWg.Add(2)
	go func() {
		defer statusWg.Done()
		<-statusStart
		_, _ = ctrl.Status(context.Background())
	}()
	go func() {
		defer statusWg.Done()
		<-statusStart
		_, _ = ctrl.Status(context.Background())
	}()
	close(statusStart)

	// 等待发起者进入 readRR0（即 single-flight 已激活）
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if rr0Calls.Load() >= 1 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if rr0Calls.Load() == 0 {
		t.Fatal("RR0 calls = 0, want >=1 (Status initiator should have entered RR0)")
	}

	// 在 Status() 进行中调用 MoveTo：MoveTo 会等 ioMu，
	// 但 Status 等待者不应在 ioMu 上重复发起 Status，所以最终 RR0 只被调用 1 次。
	moveErr := make(chan error, 1)
	go func() {
		moveErr <- ctrl.MoveTo(context.Background(), core.AxisX, 5)
	}()

	// 等待所有 Status 完成 + MoveTo 完成
	statusWg.Wait()
	if err := <-moveErr; err != nil {
		t.Fatalf("MoveTo err = %v, want nil", err)
	}

	// 验收：RR0 只被调用 1 次（发起者）。
	// 若等待者持锁并重复发起 Status，RR0 会变成 2。
	if got := rr0Calls.Load(); got != 1 {
		t.Fatalf("RR0 calls = %d, want 1 (single-flight waiter should not re-issue RR0)", got)
	}
	if got := starts.Load(); got != 1 {
		t.Fatalf("startMove calls = %d, want 1 (MoveTo should execute once)", got)
	}
}
