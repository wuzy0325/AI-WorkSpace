package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"wind-daq/services/api-go/internal/ports"
)

// ---------------------------------------------------------------------------
// Task 4: Start façade + 原子准入事务
// ---------------------------------------------------------------------------

// startProbe1OK 常规成功路径：持久化双路绑定 ctrl-a/ctrl-b，rawConfig 带客户端 taskId。
func startProbe1OK(fx *registryFixture) json.RawMessage {
	fx.seedPersistedBindings(Probe1, "ctrl-a")
	fx.seedPersistedBindings(Probe2, "ctrl-b")
	return dualConfigJSON("client-task-id", "ctrl-a")
}

func TestManagerRegistry_Start_ServerSideTaskIDAndOptions(t *testing.T) {
	fx := newRegistryFixture(t)
	raw := startProbe1OK(fx)

	taskID, err := fx.registry.Start(context.Background(), Probe1, raw)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	if taskID != "probe1-task-1" {
		t.Fatalf("应返回服务端生成的 task ID, got %q", taskID)
	}
	startCalls, cfg, opts := fx.factory.manager(Probe1).snapshotStart()
	if startCalls != 1 {
		t.Fatalf("StartManaged 调用次数: %d", startCalls)
	}
	// 客户端 task ID 被覆盖，不得成为权威键
	if cfg.TaskID != taskID {
		t.Fatalf("manager 收到的 config.TaskID 应为服务端 ID %q, got %q", taskID, cfg.TaskID)
	}
	if opts.ProbeID != Probe1 || opts.Token.ProbeID != Probe1 || opts.Token.Generation != 1 {
		t.Fatalf("SessionToken 不符: %+v", opts.Token)
	}
	if opts.TaskID != taskID || opts.ConfigKey != "traversal.probe1" {
		t.Fatalf("ManagedSessionOptions 不符: %+v", opts)
	}
	if opts.CompletionCallback == nil {
		t.Fatal("CompletionCallback 必须注入")
	}
	held, holder, acquires, _ := fx.workflow.state()
	if !held || holder != registryWorkflowHolder || acquires != 1 {
		t.Fatalf("全局 workflow lease 状态: held=%v holder=%q acquires=%d", held, holder, acquires)
	}
	if fx.controllers.heldCount("ctrl-a") != 1 {
		t.Fatal("ctrl-a 应被预占一次")
	}
	if fx.activeCount() != 1 || fx.sessionCount() != 1 {
		t.Fatalf("active=%d sessions=%d", fx.activeCount(), fx.sessionCount())
	}
}

func TestManagerRegistry_Start_EmptyOrSameControllerRejected(t *testing.T) {
	cases := []struct {
		name string
		seed func(fx *registryFixture)
		raw  func() json.RawMessage
	}{
		{
			name: "启动 probe 空绑定",
			seed: func(fx *registryFixture) { fx.seedPersistedBindings(Probe2, "ctrl-b") },
			raw:  func() json.RawMessage { return dualConfigJSON("") },
		},
		{
			name: "另一路未配置",
			seed: func(fx *registryFixture) { fx.seedPersistedBindings(Probe1, "ctrl-a") },
			raw:  func() json.RawMessage { return dualConfigJSON("", "ctrl-a") },
		},
		{
			name: "两路相同控制器",
			seed: func(fx *registryFixture) {
				fx.seedPersistedBindings(Probe1, "ctrl-a")
				fx.seedPersistedBindings(Probe2, "ctrl-a")
			},
			raw: func() json.RawMessage { return dualConfigJSON("", "ctrl-a") },
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fx := newRegistryFixture(t)
			tc.seed(fx)
			_, err := fx.registry.Start(context.Background(), Probe1, tc.raw())
			if !errors.Is(err, ErrResourceConflict) {
				t.Fatalf("应返回 ErrResourceConflict, got %v", err)
			}
			if held, _, acquires, _ := fx.workflow.state(); held || acquires != 0 {
				t.Fatalf("拒绝后不得持有全局 lease: held=%v acquires=%d", held, acquires)
			}
			if fx.controllers.totalHeld() != 0 {
				t.Fatal("拒绝后不得持有控制器 lease")
			}
			if manager := fx.factory.manager(Probe1); manager != nil {
				if calls, _, _ := manager.snapshotStart(); calls != 0 {
					t.Fatal("拒绝后不得调用 StartManaged")
				}
			}
			if fx.activeCount() != 0 || fx.sessionCount() != 0 {
				t.Fatal("拒绝后不得登记 session")
			}
		})
	}
}

func TestManagerRegistry_Start_FirstAcquiresWorkflowSecondReuses(t *testing.T) {
	fx := newRegistryFixture(t)
	fx.seedPersistedBindings(Probe1, "ctrl-a")
	fx.seedPersistedBindings(Probe2, "ctrl-b")

	if _, err := fx.registry.Start(context.Background(), Probe1, dualConfigJSON("", "ctrl-a")); err != nil {
		t.Fatalf("Start probe1: %v", err)
	}
	if _, err := fx.registry.Start(context.Background(), Probe2, dualConfigJSON("", "ctrl-b")); err != nil {
		t.Fatalf("Start probe2: %v", err)
	}
	_, _, acquires, _ := fx.workflow.state()
	if acquires != 1 {
		t.Fatalf("第二路启动不得重复 Acquire 全局 lease, acquires=%d", acquires)
	}
	if fx.controllers.heldCount("ctrl-a") != 1 || fx.controllers.heldCount("ctrl-b") != 1 {
		t.Fatal("两路控制器应各自预占")
	}
	if fx.activeCount() != 2 {
		t.Fatalf("activeCount 应为 2, got %d", fx.activeCount())
	}
}

func TestManagerRegistry_Start_ControllerConflictRollback(t *testing.T) {
	fx := newRegistryFixture(t)
	ctx := context.Background()
	// 外部持有 ctrl-a（如 legacy single 或上一任务残留）
	if _, err := fx.controllers.Acquire(ctx, "ctrl-a", "external", time.Minute); err != nil {
		t.Fatalf("预置外部 lease: %v", err)
	}
	fx.seedPersistedBindings(Probe1, "ctrl-a")
	fx.seedPersistedBindings(Probe2, "ctrl-b")

	_, err := fx.registry.Start(ctx, Probe1, dualConfigJSON("", "ctrl-a"))
	if !errors.Is(err, ErrResourceConflict) {
		t.Fatalf("应返回 ErrResourceConflict, got %v", err)
	}
	// 回滚：本次获取的全局 lease 已释放；外部控制器 lease 不受影响
	held, _, acquires, releases := fx.workflow.state()
	if held || acquires != 1 || releases != 1 {
		t.Fatalf("全局 lease 应已回滚: held=%v acquires=%d releases=%d", held, acquires, releases)
	}
	if fx.controllers.heldCount("ctrl-a") != 1 || fx.controllers.totalHeld() != 1 {
		t.Fatal("外部持有的 ctrl-a 不应被回滚释放")
	}
	if fx.activeCount() != 0 || fx.sessionCount() != 0 {
		t.Fatal("回滚后不得有活动 session")
	}
	if calls, _, _ := fx.factory.manager(Probe1).snapshotStart(); calls != 0 {
		t.Fatal("准入失败不得调用 StartManaged")
	}
}

func TestManagerRegistry_Start_ControllerAcquireRollbackFailureRetainsOwnership(t *testing.T) {
	fx := newRegistryFixture(t)
	fx.seedPersistedBindings(Probe1, "ctrl-a", "ctrl-c")
	fx.seedPersistedBindings(Probe2, "ctrl-b")
	fx.controllers.setAcquireFail("ctrl-c", true)
	fx.controllers.setReleaseFail("ctrl-a", true)

	_, err := fx.registry.Start(context.Background(), Probe1, dualConfigJSON("", "ctrl-a", "ctrl-c"))
	if !errors.Is(err, ErrResourceConflict) || !strings.Contains(err.Error(), "释放控制器 ctrl-a lease") {
		t.Fatalf("Start 应聚合 acquire 与 rollback release 错误, got %v", err)
	}
	if state, ok := fx.sessionStateOf(Probe1); !ok || state != sessionStateCompletionFailed {
		t.Fatalf("rollback failure 应保留 provisional session, state=%v exists=%v", state, ok)
	}
	if fx.activeCount() != 1 || fx.controllers.heldCount("ctrl-a") != 1 {
		t.Fatal("rollback retry 前必须保留 activeCount 与 controller ownership")
	}
	if held, _, _, _ := fx.workflow.state(); !held {
		t.Fatal("controller rollback failure 时 workflow lease 必须保留")
	}

	fx.controllers.setAcquireFail("ctrl-c", false)
	fx.controllers.setReleaseFail("ctrl-a", false)
	if err := fx.registry.CloseProbe(context.Background(), Probe1); err != nil {
		t.Fatalf("CloseProbe retry: %v", err)
	}
	if fx.activeCount() != 0 || fx.controllers.totalHeld() != 0 {
		t.Fatal("retry 后 provisional ownership 应全部清理")
	}
}

func TestManagerRegistry_Start_WorkflowRollbackFailureRetainsOwnership(t *testing.T) {
	fx := newRegistryFixture(t)
	fx.seedPersistedBindings(Probe1, "ctrl-a")
	fx.seedPersistedBindings(Probe2, "ctrl-b")
	fx.controllers.setAcquireFail("ctrl-a", true)
	releaseErr := errors.New("workflow rollback failed")
	fx.workflow.setReleaseErr(releaseErr)

	_, err := fx.registry.Start(context.Background(), Probe1, dualConfigJSON("", "ctrl-a"))
	if !errors.Is(err, ErrResourceConflict) || !errors.Is(err, releaseErr) {
		t.Fatalf("Start 应聚合 acquire 与 workflow rollback 错误, got %v", err)
	}
	if state, ok := fx.sessionStateOf(Probe1); !ok || state != sessionStateCompletionFailed {
		t.Fatalf("workflow rollback failure 应保留 provisional session, state=%v exists=%v", state, ok)
	}
	if fx.activeCount() != 1 {
		t.Fatal("workflow ownership retry 前 activeCount 必须保留")
	}
	if held, _, _, _ := fx.workflow.state(); !held {
		t.Fatal("失败的 workflow lease 必须保持续约 ownership")
	}

	fx.controllers.setAcquireFail("ctrl-a", false)
	fx.workflow.setReleaseErr(nil)
	if err := fx.registry.CloseProbe(context.Background(), Probe1); err != nil {
		t.Fatalf("CloseProbe retry: %v", err)
	}
	if fx.activeCount() != 0 {
		t.Fatal("retry 后 provisional ownership 应提交清理")
	}
}

func TestManagerRegistry_Start_ManagerStartFailureRollback(t *testing.T) {
	fx := newRegistryFixture(t)
	ctx := context.Background()
	// 预创建 manager 并注入 StartManaged 失败
	if _, err := fx.registry.GetOrCreate(Probe1); err != nil {
		t.Fatalf("GetOrCreate: %v", err)
	}
	startBoom := errors.New("start boom")
	fx.factory.manager(Probe1).setStartErr(startBoom)
	raw := startProbe1OK(fx)

	_, err := fx.registry.Start(ctx, Probe1, raw)
	if !errors.Is(err, startBoom) {
		t.Fatalf("应传播 manager Start 错误, got %v", err)
	}
	// 回滚：session 不登记、activeCount 不增、控制器预占撤销、全局 lease 释放
	if fx.activeCount() != 0 || fx.sessionCount() != 0 {
		t.Fatal("回滚后不得有活动 session")
	}
	if held, _, _, releases := fx.workflow.state(); held || releases != 1 {
		t.Fatalf("全局 lease 应已回滚释放: held=%v releases=%d", held, releases)
	}
	if fx.controllers.totalHeld() != 0 {
		t.Fatal("控制器预占应已撤销")
	}
	// 回滚路径不走 notifyCompletion：直接撤销临时资源（无 panic 且计数归零即证明）。
	// manager 保留在 map 中供重试
	if _, err := fx.registry.GetOrCreate(Probe1); err != nil {
		t.Fatalf("回滚后 manager 应可复用: %v", err)
	}
}

func TestManagerRegistry_Start_SynchronousCompletionCallbackDoesNotDeadlock(t *testing.T) {
	fx := newRegistryFixture(t)
	if _, err := fx.registry.GetOrCreate(Probe1); err != nil {
		t.Fatalf("GetOrCreate: %v", err)
	}
	fx.factory.manager(Probe1).setOnStart(func(opts ManagedSessionOptions) {
		opts.CompletionCallback(opts.Token)
	})

	done := make(chan error, 1)
	go func() {
		_, err := fx.registry.Start(context.Background(), Probe1, startProbe1OK(fx))
		done <- err
	}()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Start: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("synchronous completion callback deadlocked StartManaged")
	}
	if fx.activeCount() != 0 || fx.sessionCount() != 0 {
		t.Fatal("synchronous completion must settle after managed start returns")
	}
}

func TestManagerRegistry_Start_LeaseAcquireRunsOutsideRegistryMutex(t *testing.T) {
	fx := newRegistryFixture(t)
	raw := startProbe1OK(fx)
	assertUnlocked := func() {
		if !fx.registry.mu.TryLock() {
			t.Fatal("external lease Acquire ran while registry mutex was held")
		}
		fx.registry.mu.Unlock()
	}
	fx.workflow.onAcquire = assertUnlocked
	fx.controllers.onAcquire = assertUnlocked
	if _, err := fx.registry.Start(context.Background(), Probe1, raw); err != nil {
		t.Fatalf("Start: %v", err)
	}
	completeSession(fx, Probe1)
}

func TestManagerRegistry_Start_RollbackReleaseFailureRetainsSessionForRetry(t *testing.T) {
	fx := newRegistryFixture(t)
	if _, err := fx.registry.GetOrCreate(Probe1); err != nil {
		t.Fatalf("GetOrCreate: %v", err)
	}
	startErr := errors.New("start failed")
	fx.factory.manager(Probe1).setStartErr(startErr)
	fx.controllers.setReleaseFail("ctrl-a", true)

	_, err := fx.registry.Start(context.Background(), Probe1, startProbe1OK(fx))
	if !errors.Is(err, startErr) || !strings.Contains(err.Error(), "回滚准入资源失败") {
		t.Fatalf("Start 应聚合启动与回滚错误, got %v", err)
	}
	if state, ok := fx.sessionStateOf(Probe1); !ok || state != sessionStateCompletionFailed {
		t.Fatalf("释放失败应保留 completion_failed session, state=%v exists=%v", state, ok)
	}
	if fx.activeCount() != 1 || fx.controllers.heldCount("ctrl-a") != 1 {
		t.Fatal("释放成功前必须保留 activeCount 与 lease ownership")
	}

	fx.controllers.setReleaseFail("ctrl-a", false)
	if err := fx.registry.CloseProbe(context.Background(), Probe1); err != nil {
		t.Fatalf("CloseProbe retry: %v", err)
	}
	if fx.activeCount() != 0 || fx.sessionCount() != 0 || fx.controllers.totalHeld() != 0 {
		t.Fatal("重试成功后应提交回滚清理")
	}
}

func TestManagerRegistry_Start_RecoverableTaskRejected(t *testing.T) {
	fx := newRegistryFixture(t)
	fx.index.seed("probe1", "probe1-task-9", "/tmp/probe1.checkpoint.json")
	raw := startProbe1OK(fx)

	_, err := fx.registry.Start(context.Background(), Probe1, raw)
	if !errors.Is(err, ports.ErrRecoverableTaskExists) {
		t.Fatalf("应返回 ErrRecoverableTaskExists, got %v", err)
	}
	// 拒绝发生在 factory 创建与任何文件/运动 I/O 之前
	if fx.factory.callCount() != 0 {
		t.Fatal("recoverable 拒绝不得触发 factory 创建")
	}
	if _, _, acquires, _ := fx.workflow.state(); acquires != 0 {
		t.Fatal("recoverable 拒绝不得触碰全局 lease")
	}
	if fx.controllers.totalHeld() != 0 || fx.sessionCount() != 0 {
		t.Fatal("recoverable 拒绝不得预占控制器或登记 session")
	}
}

func TestManagerRegistry_Start_AlreadyRunning(t *testing.T) {
	fx := newRegistryFixture(t)
	raw := startProbe1OK(fx)
	ctx := context.Background()

	if _, err := fx.registry.Start(ctx, Probe1, raw); err != nil {
		t.Fatalf("首次 Start: %v", err)
	}
	_, err := fx.registry.Start(ctx, Probe1, raw)
	if !errors.Is(err, ErrAlreadyRunning) {
		t.Fatalf("同 probe 重复启动应返回 ErrAlreadyRunning, got %v", err)
	}
	if calls, _, _ := fx.factory.manager(Probe1).snapshotStart(); calls != 1 {
		t.Fatalf("StartManaged 应只调用一次, got %d", calls)
	}
	if fx.activeCount() != 1 {
		t.Fatalf("activeCount 应保持 1, got %d", fx.activeCount())
	}
}

func TestManagerRegistry_Start_ConcurrentDistinctProbes(t *testing.T) {
	fx := newRegistryFixture(t)
	fx.seedPersistedBindings(Probe1, "ctrl-a")
	fx.seedPersistedBindings(Probe2, "ctrl-b")
	ctx := context.Background()

	var ready sync.WaitGroup
	ready.Add(2)
	errs := make(chan error, 2)
	go func() {
		ready.Done()
		_, err := fx.registry.Start(ctx, Probe1, dualConfigJSON("", "ctrl-a"))
		errs <- err
	}()
	go func() {
		ready.Done()
		_, err := fx.registry.Start(ctx, Probe2, dualConfigJSON("", "ctrl-b"))
		errs <- err
	}()
	ready.Wait()
	for i := 0; i < 2; i++ {
		if err := <-errs; err != nil {
			t.Fatalf("并发启动不同 probe 应双双成功: %v", err)
		}
	}
	if _, _, acquires, _ := fx.workflow.state(); acquires != 1 {
		t.Fatalf("全局 lease 应只获取一次, got %d", acquires)
	}
	if fx.activeCount() != 2 {
		t.Fatalf("activeCount 应为 2, got %d", fx.activeCount())
	}
	if fx.controllers.heldCount("ctrl-a") != 1 || fx.controllers.heldCount("ctrl-b") != 1 {
		t.Fatal("两控制器应各被预占一次")
	}
}

func TestManagerRegistry_Start_ConcurrentOverlappingSingleOccupancy(t *testing.T) {
	fx := newRegistryFixture(t)
	// probe2 持久化绑定 ctrl-b，但本次启动 rawConfig 改用 ctrl-a（与 probe1 冲突）：
	// 无论哪路先进入临界区，都必须恰好一路成功，且 ctrl-a 只被预占一次。
	fx.seedPersistedBindings(Probe1, "ctrl-a")
	fx.seedPersistedBindings(Probe2, "ctrl-b")
	ctx := context.Background()

	var ready sync.WaitGroup
	ready.Add(2)
	results := make(chan error, 2)
	go func() {
		ready.Done()
		_, err := fx.registry.Start(ctx, Probe1, dualConfigJSON("", "ctrl-a"))
		results <- err
	}()
	go func() {
		ready.Done()
		_, err := fx.registry.Start(ctx, Probe2, dualConfigJSON("", "ctrl-a"))
		results <- err
	}()
	ready.Wait()
	var successes, conflicts int
	for i := 0; i < 2; i++ {
		err := <-results
		switch {
		case err == nil:
			successes++
		case errors.Is(err, ErrResourceConflict):
			conflicts++
		default:
			t.Fatalf("非预期错误: %v", err)
		}
	}
	if successes != 1 || conflicts != 1 {
		t.Fatalf("重叠绑定时应恰好一路成功一路冲突: success=%d conflict=%d", successes, conflicts)
	}
	if fx.controllers.heldCount("ctrl-a") != 1 || fx.controllers.totalHeld() != 1 {
		t.Fatal("ctrl-a 不得被双重占用")
	}
	if fx.activeCount() != 1 {
		t.Fatalf("activeCount 应为 1, got %d", fx.activeCount())
	}
}

func TestManagerRegistry_Start_CompletionIdempotentAndRestart(t *testing.T) {
	fx := newRegistryFixture(t)
	raw := startProbe1OK(fx)
	ctx := context.Background()

	if _, err := fx.registry.Start(ctx, Probe1, raw); err != nil {
		t.Fatalf("Start: %v", err)
	}
	_, _, opts := fx.factory.manager(Probe1).snapshotStart()
	// manager 完成回调：计数递减、控制器与全局 lease 释放
	opts.CompletionCallback(opts.Token)
	if fx.activeCount() != 0 || fx.sessionCount() != 0 {
		t.Fatal("完成后 activeCount/session 应归零")
	}
	if held, _, _, releases := fx.workflow.state(); held || releases != 1 {
		t.Fatalf("最后一路完成应释放全局 lease: held=%v releases=%d", held, releases)
	}
	if fx.controllers.totalHeld() != 0 {
		t.Fatal("完成后控制器 lease 应释放")
	}
	// 重复完成：幂等，不再递减、不再释放
	opts.CompletionCallback(opts.Token)
	if fx.activeCount() != 0 {
		t.Fatal("重复完成不得再次递减")
	}
	if _, _, _, releases := fx.workflow.state(); releases != 1 {
		t.Fatal("重复完成不得再次释放全局 lease")
	}
	// 重新启动：新 generation token
	if _, err := fx.registry.Start(ctx, Probe1, raw); err != nil {
		t.Fatalf("完成后应可重新启动: %v", err)
	}
	_, _, opts2 := fx.factory.manager(Probe1).snapshotStart()
	if opts2.Token.Generation != 2 {
		t.Fatalf("重启 generation 应递增, got %d", opts2.Token.Generation)
	}
	// 旧 generation 通知：不影响当前 session/lease
	opts.CompletionCallback(opts.Token)
	if fx.activeCount() != 1 || fx.controllers.heldCount("ctrl-a") != 1 {
		t.Fatal("旧 generation 通知不得影响当前 session")
	}
	opts2.CompletionCallback(opts2.Token)
	if fx.activeCount() != 0 || fx.controllers.totalHeld() != 0 {
		t.Fatal("当前 generation 完成应正常清理")
	}
}

func TestManagerRegistry_Start_ClosingRejected(t *testing.T) {
	fx := newRegistryFixture(t)
	raw := startProbe1OK(fx)
	fx.setClosing()
	if _, err := fx.registry.Start(context.Background(), Probe1, raw); !errors.Is(err, ErrRegistryClosing) {
		t.Fatalf("closing 时应返回 ErrRegistryClosing, got %v", err)
	}
	if fx.sessionCount() != 0 || fx.controllers.totalHeld() != 0 {
		t.Fatal("closing 拒绝不得登记 session 或预占资源")
	}
}

func TestManagerRegistry_Start_InvalidProbeID(t *testing.T) {
	fx := newRegistryFixture(t)
	if _, err := fx.registry.Start(context.Background(), "probe9", dualConfigJSON("", "ctrl-a")); !errors.Is(err, ErrInvalidProbeID) {
		t.Fatalf("未知 probe 应返回 ErrInvalidProbeID, got %v", err)
	}
}
