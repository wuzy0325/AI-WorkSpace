package hardware

import (
	"context"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"shared.local/device-sdk/go/motion/core"
)

// TestB140StatusConcurrentSharesSingleFlight 验证 B140 Status() 入口存在 single-flight 合并：
// 多个并发 Status() 调用只触发一轮 TD/TS/MG 命令，避免多消费者放大 B140 TCP 命令数。
//
// 这是 spec Decision 2 "一个控制器一个采集 flight" 的最小实现验证。
func TestB140StatusConcurrentSharesSingleFlight(t *testing.T) {
	server := newB140FakeServer(t, map[string]string{
		"SH":      "",
		"MTA=2":   "",
		"CEA=0":   "",
		"TS":      "0,0,0,0",
		"MG _LFA": "1.0000",
		"MG _LRA": "1.0000",
	})
	defer server.close()

	// TD 加 50ms 延迟，扩大 single-flight 命中窗口。
	// 若无 single-flight，3 个并发调用会各自发起一轮 TD。
	var tdCalls atomic.Int32
	server.setDynamic("TD", func() string {
		tdCalls.Add(1)
		time.Sleep(50 * time.Millisecond)
		return "2000,0,0,0"
	})

	ctrl := newTestB140WithServer(t, server)
	if err := ctrl.Connect(context.Background()); err != nil {
		t.Fatalf("Connect: %v", err)
	}

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

	// 验收 1：只发送 1 轮 TD 命令（single-flight 合并）
	if got := tdCalls.Load(); got != 1 {
		t.Fatalf("TD calls = %d, want 1 (single-flight should share one flight)", got)
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
		if results[i].Axes[0].PosLimit != results[0].Axes[0].PosLimit {
			t.Fatalf("Status[%d].PosLimit = %v, want %v (shared result)",
				i, results[i].Axes[0].PosLimit, results[0].Axes[0].PosLimit)
		}
	}
}

// TestB140StatusSingleFlightErrorPropagatedToWaiters 验证 in-flight 失败时，
// 所有等待者收到同一错误而非各自重新发起查询。
func TestB140StatusSingleFlightErrorPropagatedToWaiters(t *testing.T) {
	server := newB140FakeServer(t, map[string]string{
		"SH":      "",
		"MTA=2":   "",
		"CEA=0":   "",
		"TS":      "0,0,0,0",
		"MG _LFA": "1.0000",
		"MG _LRA": "1.0000",
	})
	defer server.close()

	// TD 返回无法被 parseB140Numbers 解析的字符串，触发 Status() 错误路径。
	// 加 50ms 延迟确保多个调用者落在同一 flight。
	var tdCalls atomic.Int32
	server.setDynamic("TD", func() string {
		tdCalls.Add(1)
		time.Sleep(50 * time.Millisecond)
		return "not-a-number"
	})

	ctrl := newTestB140WithServer(t, server)
	if err := ctrl.Connect(context.Background()); err != nil {
		t.Fatalf("Connect: %v", err)
	}

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

	// 验收：只发送 1 轮 TD（single-flight 合并失败查询）
	if got := tdCalls.Load(); got != 1 {
		t.Fatalf("TD calls = %d, want 1 (single-flight should share failed flight)", got)
	}

	// 验收：所有调用者收到错误（而非只有发起者收到错误）
	for i := 0; i < concurrency; i++ {
		if errs[i] == nil {
			t.Fatalf("Status[%d] err = nil, want non-nil (error should propagate to waiters)", i)
		}
		// 错误来自 strconv.ParseFloat，消息形如：
		//   strconv.ParseFloat: parsing "not-a-number": invalid syntax
		// 用 strconv.ParseFloat 单一子串收紧检查，避免 || 引起的语义模糊。
		if !strings.Contains(errs[i].Error(), "strconv.ParseFloat") {
			t.Fatalf("Status[%d] err = %v, want parse error containing strconv.ParseFloat", i, errs[i])
		}
	}

	// 额外断言 1：3 个调用者的错误消息应相同（同一 flight 的快照）。
	// parseB140Numbers 返回 *strconv.NumError 无 Is 方法，errors.Is 退化为指针比较，
	// 这里直接比较错误消息文本即可验证"等待者共享同一错误快照"。
	if errs[0].Error() != errs[1].Error() {
		t.Fatalf("err[0]=%v != err[1]=%v (waiters should share identical error)", errs[0], errs[1])
	}

	// 额外断言 2：所有调用者的 status 也应一致（同一 flight 的失败前缓存快照）。
	// single-flight 契约是"所有等待者收到相同结果"，结果包括 status 和 err。
	for i := 1; i < concurrency; i++ {
		if results[i].LastError != results[0].LastError {
			t.Fatalf("Status[%d].LastError = %q, want %q (waiters should share identical status)",
				i, results[i].LastError, results[0].LastError)
		}
		if len(results[i].Axes) != len(results[0].Axes) {
			t.Fatalf("Status[%d].Axes len = %d, want %d", i, len(results[i].Axes), len(results[0].Axes))
		}
	}
}

// TestB140StatusSingleFlightWaiterDoesNotReissueTD 验证 single-flight 等待者不重新
// 发起 TD 查询：等待者通过 <-flight.done 复用发起者结果，不进入 sendCommand 路径，
// 因此即便有 MoveTo 并发下 connMu 竞争，TD 调用数仍为 1。
//
// 注意：本测试不验证"MoveTo 不被等待者阻塞"的时序——那需要测量 MoveTo 完成时间
// < Status 完成时间。当前测试只验证命令计数不变量，对应 spec Decision 2 的
// "一个控制器一个采集 flight"。时序验证留给 Task 2 Priority Coordinator。
func TestB140StatusSingleFlightWaiterDoesNotReissueTD(t *testing.T) {
	server := newB140FakeServer(t, map[string]string{
		"SH":       "",
		"MTA=2":    "",
		"CEA=0":    "",
		"TS":       "0,0,0,0",
		"MG _LFA":  "1.0000",
		"MG _LRA":  "1.0000",
		"SPA=4000": "",
		"PAA=2000": "",
		"BGA":      "",
	})
	defer server.close()

	// TD 加 80ms 延迟，确保 Status() 进行中时 MoveTo 进入并发场景。
	var tdCalls atomic.Int32
	server.setDynamic("TD", func() string {
		tdCalls.Add(1)
		time.Sleep(80 * time.Millisecond)
		return "2000,0,0,0"
	})

	ctrl := newTestB140WithServer(t, server)
	if err := ctrl.Connect(context.Background()); err != nil {
		t.Fatalf("Connect: %v", err)
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

	// 等待发起者进入 fake server 的 TD 处理（即 single-flight 已激活）
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if tdCalls.Load() >= 1 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if tdCalls.Load() == 0 {
		t.Fatal("TD calls = 0, want >=1 (Status initiator should have entered TD)")
	}

	// 在 Status() 进行中调用 MoveTo：MoveTo 会等 connMu，
	// 但 Status 等待者不应在 connMu 上重复排队，所以最终 TD 只被调用 1 次。
	moveErr := make(chan error, 1)
	go func() {
		moveErr <- ctrl.MoveTo(context.Background(), core.AxisX, 10)
	}()

	// 等待所有 Status 完成 + MoveTo 完成
	statusWg.Wait()
	if err := <-moveErr; err != nil {
		t.Fatalf("MoveTo err = %v, want nil", err)
	}

	// 验收：TD 只被调用 1 次（发起者）。
	// 若等待者持锁并重复发起 Status，TD 会变成 2。
	if got := tdCalls.Load(); got != 1 {
		t.Fatalf("TD calls = %d, want 1 (single-flight waiter should not re-issue TD)", got)
	}
}
