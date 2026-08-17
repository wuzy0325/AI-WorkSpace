package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
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

func TestManagerRegistry_LoadProbeBindingsFromFrontendConfig(t *testing.T) {
	fx := newRegistryFixture(t)
	fx.configs.seed(probeConfigKey(Probe2), json.RawMessage(`{
		"channels":{"motionAxes":[
			{"name":"X","controllerId":" ctrl-b ","axis":"X"},
			{"name":"Y","controllerId":"ctrl-b","axis":"Y"}
		]}
	}`))

	bindings, err := fx.registry.loadProbeBindings(Probe2)
	if err != nil {
		t.Fatalf("loadProbeBindings 应在配置存在时返回 nil 错误, got %v", err)
	}
	// 资源独占粒度为 (controllerID, axis) 元组：同一控制器的两个不同轴各自算一份。
	// ControllerID 经 TrimSpace 规整，两个 (ctrl-b, X) / (ctrl-b, Y) 元组都应被提取。
	if len(bindings) != 2 {
		t.Fatalf("应从前端 channels.motionAxes 提取两路 axis pairs, got %d", len(bindings))
	}
	for _, b := range bindings {
		if b.ControllerID != "ctrl-b" {
			t.Fatalf("ControllerID 应规整为 ctrl-b, got %q", b.ControllerID)
		}
		if b.Axis != "X" && b.Axis != "Y" {
			t.Fatalf("Axis 应为 X 或 Y, got %q", b.Axis)
		}
	}
}

// TestManagerRegistry_LoadProbeBindings_IOErrorPropagated 验证 I-6 修复：
// 真实 I/O 错误（非"未配置"）必须向上传播，而非被吞掉返回 nil。
// 这样用户能看到真实存储故障而非 resource_conflict 误导。
func TestManagerRegistry_LoadProbeBindings_IOErrorPropagated(t *testing.T) {
	fx := newRegistryFixture(t)
	// 注入会返回真实 I/O 错误的 configStore（不是 not-exist 的 nil, nil）。
	fx.registry.configStore = &errorConfigStore{err: errors.New("disk read error")}

	bindings, err := fx.registry.loadProbeBindings(Probe1)
	if err == nil {
		t.Fatalf("I/O 错误应向上传播, got nil err with bindings=%v", bindings)
	}
	if !strings.Contains(err.Error(), "disk read error") {
		t.Fatalf("错误应包含原始 I/O 错误信息, got %v", err)
	}
	if bindings != nil {
		t.Fatalf("I/O 错误下 bindings 应为 nil, got %v", bindings)
	}
}

// errorConfigStore 注入式 AppConfigStore，对所有 LoadConfig 返回固定错误。
// 用于验证 I/O 错误向上传播（区别于 fakeConfigStore 的"未配置"语义）。
type errorConfigStore struct {
	err error
}

func (s *errorConfigStore) LoadConfig(string) ([]byte, error) { return nil, s.err }
func (s *errorConfigStore) SaveConfig(string, []byte) error   { return nil }

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
	// 外部持有 ctrl-a 的 X 轴（dualConfigJSON("", "ctrl-a") 默认配 X 轴），如 legacy single 或上一任务残留
	if _, err := fx.controllers.Acquire(ctx, "ctrl-a", "X", "external", time.Minute); err != nil {
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
	// dualConfigJSON 为 ctrl-a 派生 X 轴、ctrl-c 派生 Y 轴（按顺序 X/Y/Z/U）；
	// 错误聚合应同时包含 acquire 失败（ctrl-c 轴 Y）与 rollback release 失败（ctrl-a 轴 X）。
	if !errors.Is(err, ErrResourceConflict) || !strings.Contains(err.Error(), "释放控制器 ctrl-a 轴 X lease") {
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

func TestManagerRegistry_Start_IgnoresLegacyRecoveryEntry(t *testing.T) {
	fx := newRegistryFixture(t)
	fx.index.seed("probe1", "probe1-task-9", "/tmp/probe1.checkpoint.json")
	raw := startProbe1OK(fx)

	if _, err := fx.registry.Start(context.Background(), Probe1, raw); err != nil {
		t.Fatalf("legacy recovery entry must not block a new start: %v", err)
	}
	completeSession(fx, Probe1)
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

// TestManagerRegistry_Start_SameControllerDifferentAxesAllowed 验证资源独占粒度为
// (controllerID, axis) 元组：两 probe 共用同一运动控制器的不同物理轴时应允许双探针并行启动
// （风洞实验常见配置：两探针分别由同一控制器的 X/Y 轴驱动）。
func TestManagerRegistry_Start_SameControllerDifferentAxesAllowed(t *testing.T) {
	fx := newRegistryFixture(t)
	ctx := context.Background()
	// 两 probe 各配 ctrl-a 的一个轴（probe1→X，probe2→Y），共用控制器但物理轴不同
	fx.seedPersistedAxisBinding(Probe1, "ctrl-a", "X")
	fx.seedPersistedAxisBinding(Probe2, "ctrl-a", "Y")

	if _, err := fx.registry.Start(ctx, Probe1, dualConfigWithAxisJSON("", "ctrl-a", "X")); err != nil {
		t.Fatalf("Start probe1 (ctrl-a, X): %v", err)
	}
	if _, err := fx.registry.Start(ctx, Probe2, dualConfigWithAxisJSON("", "ctrl-a", "Y")); err != nil {
		t.Fatalf("Start probe2 (ctrl-a, Y): %v", err)
	}
	// 全局 workflow lease 只获取一次（两 probe 共享）
	if _, _, acquires, _ := fx.workflow.state(); acquires != 1 {
		t.Fatalf("全局 lease 应只获取一次, got %d", acquires)
	}
	// 同控制器的两个轴 lease 互相独立——heldCount 返回 2，heldAxisCount 各为 1
	if fx.controllers.heldCount("ctrl-a") != 2 {
		t.Fatalf("ctrl-a 应持有两个轴 lease, got %d", fx.controllers.heldCount("ctrl-a"))
	}
	if fx.controllers.heldAxisCount("ctrl-a", "X") != 1 || fx.controllers.heldAxisCount("ctrl-a", "Y") != 1 {
		t.Fatal("ctrl-a 的 X/Y 轴 lease 应各自独立持有")
	}
	if fx.activeCount() != 2 {
		t.Fatalf("activeCount 应为 2, got %d", fx.activeCount())
	}
}

// TestManagerRegistry_Start_SameControllerSameAxisRejected 验证冲突检测粒度：
// 两 probe 绑定到同一控制器的同一物理轴时必须被拒绝（资源冲突）。
// validateDualBindings 用另一路 probe 的持久化绑定做静态校验——
// 即使另一路未运行，启动第一路时就会立即拒绝（spec I1：双模式两路绑定不得重叠）。
func TestManagerRegistry_Start_SameControllerSameAxisRejected(t *testing.T) {
	fx := newRegistryFixture(t)
	ctx := context.Background()
	// 两 probe 都配 ctrl-a 的 X 轴（同一 (controllerID, axis) 元组）
	fx.seedPersistedAxisBinding(Probe1, "ctrl-a", "X")
	fx.seedPersistedAxisBinding(Probe2, "ctrl-a", "X")

	_, err := fx.registry.Start(ctx, Probe1, dualConfigWithAxisJSON("", "ctrl-a", "X"))
	if !errors.Is(err, ErrResourceConflict) {
		t.Fatalf("同控制器同轴应返回 ErrResourceConflict, got %v", err)
	}
	if !strings.Contains(err.Error(), "轴 X") {
		t.Fatalf("错误信息应指明冲突的轴, got %v", err)
	}
	// 冲突后不得登记任何 session 或预占 lease
	if fx.activeCount() != 0 || fx.sessionCount() != 0 {
		t.Fatalf("冲突后不得登记 session, active=%d sessions=%d", fx.activeCount(), fx.sessionCount())
	}
	if fx.controllers.totalHeld() != 0 {
		t.Fatal("冲突后不得持有任何 lease")
	}
}

// TestManagerRegistry_Start_SameControllerDifferentAxesIndependentCompletion 验证
// 同控制器不同轴 lease 的释放互相独立：一路完成时另一路的轴 lease 不受影响。
func TestManagerRegistry_Start_SameControllerDifferentAxesIndependentCompletion(t *testing.T) {
	fx := newRegistryFixture(t)
	ctx := context.Background()
	fx.seedPersistedAxisBinding(Probe1, "ctrl-a", "X")
	fx.seedPersistedAxisBinding(Probe2, "ctrl-a", "Y")

	if _, err := fx.registry.Start(ctx, Probe1, dualConfigWithAxisJSON("", "ctrl-a", "X")); err != nil {
		t.Fatalf("Start probe1: %v", err)
	}
	if _, err := fx.registry.Start(ctx, Probe2, dualConfigWithAxisJSON("", "ctrl-a", "Y")); err != nil {
		t.Fatalf("Start probe2: %v", err)
	}

	// probe1 完成：释放 (ctrl-a, X) lease，但 (ctrl-a, Y) 必须仍被 probe2 持有
	completeSession(fx, Probe1)
	if fx.controllers.heldAxisCount("ctrl-a", "X") != 0 {
		t.Fatal("probe1 完成后 (ctrl-a, X) lease 应释放")
	}
	if fx.controllers.heldAxisCount("ctrl-a", "Y") != 1 {
		t.Fatal("probe1 完成不得影响 probe2 的 (ctrl-a, Y) lease")
	}
	if fx.activeCount() != 1 {
		t.Fatalf("probe1 完成后 activeCount 应为 1, got %d", fx.activeCount())
	}
	// 全局 workflow lease 在仍有一路活动时不得释放
	if held, _, _, _ := fx.workflow.state(); !held {
		t.Fatal("probe2 仍活动时全局 workflow lease 应保留")
	}

	// probe2 完成：最后一路释放全部 lease 与全局 workflow lease
	completeSession(fx, Probe2)
	if fx.controllers.totalHeld() != 0 {
		t.Fatal("全部完成后控制器 lease 应释放")
	}
	if held, _, _, releases := fx.workflow.state(); held || releases != 1 {
		t.Fatalf("最后一路完成应释放全局 workflow lease: held=%v releases=%d", held, releases)
	}
}
