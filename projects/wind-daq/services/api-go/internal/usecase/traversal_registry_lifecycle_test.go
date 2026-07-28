package usecase

import (
	"context"
	"errors"
	"strings"
	"testing"
	"testing/synctest"
	"time"
)

// ---------------------------------------------------------------------------
// Task 6: Stop / CloseProbe 生命周期
// ---------------------------------------------------------------------------

// hookCompletionOnStop 让 fake manager 在 Stop 时触发完成回调（模拟 goroutine exit + finalize）。
func hookCompletionOnStop(fx *registryFixture, probeID ProbeID) {
	_, _, opts := fx.factory.manager(probeID).snapshotStart()
	fx.factory.manager(probeID).setOnStop(func() {
		opts.CompletionCallback(opts.Token)
	})
}

func TestManagerRegistry_Stop_ActiveSessionWaitsCompletion(t *testing.T) {
	fx := newRegistryFixture(t)
	raw := startProbe1OK(fx)
	if _, err := fx.registry.Start(context.Background(), Probe1, raw); err != nil {
		t.Fatalf("Start: %v", err)
	}
	hookCompletionOnStop(fx, Probe1)

	if err := fx.registry.Stop(context.Background(), Probe1); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	// Stop 成功意味着 goroutine 退出 + finalize + completion lease 释放均已完成
	if fx.activeCount() != 0 || fx.controllers.totalHeld() != 0 {
		t.Fatal("Stop 返回时 completion 应已提交并释放 lease")
	}
	if held, _, _, releases := fx.workflow.state(); held || releases != 1 {
		t.Fatalf("全局 lease 应已释放: held=%v releases=%d", held, releases)
	}
	if calls := fx.factory.manager(Probe1).stopCallCount(); calls != 1 {
		t.Fatalf("manager.Stop 应调用一次, got %d", calls)
	}
}

func TestManagerRegistry_Stop_NoActiveSession(t *testing.T) {
	fx := newRegistryFixture(t)
	if err := fx.registry.Stop(context.Background(), Probe1); err == nil {
		t.Fatal("无活动任务时 Stop 应返回错误")
	}
	if err := fx.registry.Stop(context.Background(), "probe9"); !errors.Is(err, ErrInvalidProbeID) {
		t.Fatalf("未知 probe 应返回 ErrInvalidProbeID, got %v", err)
	}
}

func TestManagerRegistry_Stop_WaitsForResumeManagedPublication(t *testing.T) {
	fx := newRegistryFixture(t)
	seedDualCheckpoint(t, fx, Probe1, "probe1-task-9", []string{"ctrl-a"})
	fx.seedPersistedBindings(Probe2, "ctrl-b")
	if _, err := fx.registry.GetOrCreate(Probe1); err != nil {
		t.Fatalf("GetOrCreate: %v", err)
	}
	manager := fx.factory.manager(Probe1)
	resumeEntered, unblockResume := manager.setResumeBlock()
	manager.setOnStop(func() {
		manager.mu.Lock()
		opts := manager.lastOpts
		manager.mu.Unlock()
		opts.CompletionCallback(opts.Token)
	})

	resumeDone := make(chan error, 1)
	go func() {
		_, err := fx.registry.ResumeFromCheckpoint(context.Background(), Probe1, "probe1-task-9")
		resumeDone <- err
	}()
	<-resumeEntered
	if fx.registry.tryAcquireAdmissionForTest() {
		fx.registry.releaseAdmission()
		t.Fatal("ResumeManaged 阻塞期间 admission gate 必须保持持有")
	}

	stopDone := make(chan error, 1)
	stopAttempted := make(chan struct{})
	go func() {
		close(stopAttempted)
		stopDone <- fx.registry.Stop(context.Background(), Probe1)
	}()
	<-stopAttempted
	select {
	case err := <-stopDone:
		t.Fatalf("ResumeManaged 返回前 Stop 不得返回: %v", err)
	default:
	}
	if calls := manager.stopCallCount(); calls != 0 {
		t.Fatalf("ResumeManaged 返回前 Stop 不得到达 manager, calls=%d", calls)
	}

	unblockResume()
	if err := <-resumeDone; err != nil {
		t.Fatalf("ResumeFromCheckpoint: %v", err)
	}
	if err := <-stopDone; err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if calls := manager.stopCallCount(); calls != 1 {
		t.Fatalf("恢复发布后 Stop 必须送达一次, calls=%d", calls)
	}
}

func TestManagerRegistry_Stop_AggregatesStopErrorAndWaits(t *testing.T) {
	fx := newRegistryFixture(t)
	raw := startProbe1OK(fx)
	if _, err := fx.registry.Start(context.Background(), Probe1, raw); err != nil {
		t.Fatalf("Start: %v", err)
	}
	// manager.Stop 返回错误，但 completion 仍随后发生：
	// Stop 必须聚合错误（errors.Join）且不跳过有界等待。
	stopBoom := errors.New("stop boom")
	hookCompletionOnStop(fx, Probe1)
	fx.factory.manager(Probe1).setStopErr(stopBoom)

	err := fx.registry.Stop(context.Background(), Probe1)
	if !errors.Is(err, stopBoom) {
		t.Fatalf("应聚合 manager.Stop 错误, got %v", err)
	}
	if fx.activeCount() != 0 {
		t.Fatal("第一步失败不得跳过有界等待：completion 应已提交")
	}
}

func TestManagerRegistry_Stop_Timeout(t *testing.T) {
	fx := newRegistryFixture(t)
	raw := startProbe1OK(fx)
	if _, err := fx.registry.Start(context.Background(), Probe1, raw); err != nil {
		t.Fatalf("Start: %v", err)
	}
	// fake manager 不触发完成：Stop 有界等待超时
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	err := fx.registry.Stop(ctx, Probe1)
	if err == nil {
		t.Fatal("completion 未完成时 Stop 应返回超时错误")
	}
	if fx.activeCount() != 1 {
		t.Fatal("超时后 session 应保留（不得误递减）")
	}
	// 清理：手动完成
	completeSession(fx, Probe1)
}

func TestManagerRegistry_CloseProbe_ActiveSession(t *testing.T) {
	fx := newRegistryFixture(t)
	raw := startProbe1OK(fx)
	if _, err := fx.registry.Start(context.Background(), Probe1, raw); err != nil {
		t.Fatalf("Start: %v", err)
	}

	if err := fx.registry.CloseProbe(context.Background(), Probe1); !errors.Is(err, ErrAlreadyRunning) {
		t.Fatalf("活动 session 应返回 ErrAlreadyRunning: %v", err)
	}
	if calls := fx.factory.manager(Probe1).stopCallCount(); calls != 0 {
		t.Fatalf("活动 session 不得由 CloseProbe 停止, calls=%d", calls)
	}
	if fx.activeCount() != 1 || fx.sessionCount() != 1 {
		t.Fatal("活动 session 必须保持运行并保留 registry ownership")
	}
	completeSession(fx, Probe1)
}

func TestManagerRegistry_CloseProbe_CannotCloseSessionPublishedByConcurrentStart(t *testing.T) {
	fx := newRegistryFixture(t)
	raw := startProbe1OK(fx)
	if _, err := fx.registry.GetOrCreate(Probe1); err != nil {
		t.Fatalf("GetOrCreate: %v", err)
	}
	manager := fx.factory.manager(Probe1)
	startEntered, unblockStart := manager.setStartBlock()
	startDone := make(chan error, 1)
	go func() {
		_, err := fx.registry.Start(context.Background(), Probe1, raw)
		startDone <- err
	}()
	<-startEntered

	closeDone := make(chan error, 1)
	go func() { closeDone <- fx.registry.CloseProbe(context.Background(), Probe1) }()
	select {
	case err := <-closeDone:
		t.Fatalf("Start publication gate 释放前 CloseProbe 不得完成: %v", err)
	default:
	}

	unblockStart()
	if err := <-startDone; err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := <-closeDone; !errors.Is(err, ErrAlreadyRunning) {
		t.Fatalf("CloseProbe 必须看到新发布的 session 并拒绝关闭: %v", err)
	}
	if manager.stopCallCount() != 0 || fx.activeCount() != 1 {
		t.Fatal("并发 CloseProbe 不得停止或删除新准入 session")
	}
	completeSession(fx, Probe1)
}

func TestManagerRegistry_CloseProbe_TerminalManager(t *testing.T) {
	fx := newRegistryFixture(t)
	// 仅创建 manager（终态、无 session）
	if _, err := fx.registry.GetOrCreate(Probe1); err != nil {
		t.Fatalf("GetOrCreate: %v", err)
	}
	manager := fx.factory.manager(Probe1)

	if err := fx.registry.CloseProbe(context.Background(), Probe1); err != nil {
		t.Fatalf("CloseProbe: %v", err)
	}
	if manager.stopCallCount() != 0 {
		t.Fatal("无活动 session 时不得调用 manager.Stop")
	}
	// 删除后可重建
	if _, err := fx.registry.GetOrCreate(Probe1); err != nil {
		t.Fatalf("重建: %v", err)
	}
	if fx.factory.manager(Probe1) == manager {
		t.Fatal("终态 manager 应已删除并重建")
	}
}

func TestManagerRegistry_CloseProbe_TimeoutRetainsEntries(t *testing.T) {
	fx := newRegistryFixture(t)
	raw := startProbe1OK(fx)
	if _, err := fx.registry.Start(context.Background(), Probe1, raw); err != nil {
		t.Fatalf("Start: %v", err)
	}
	// 模拟 manager 已退出并进入 completion，但 cleanup 尚未 settle。
	fx.registry.mu.Lock()
	fx.registry.sessions[Probe1].state = sessionStateCompleting
	fx.registry.mu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	err := fx.registry.CloseProbe(ctx, Probe1)
	if !errors.Is(err, ErrCloseProbeTimeout) {
		t.Fatalf("应返回 close_probe_timeout, got %v", err)
	}
	// map 条目保留、manager 标记 closing：GetOrCreate 返回 probe_closing 且不新建
	callsBefore := fx.factory.callCount()
	if _, err := fx.registry.GetOrCreate(Probe1); !errors.Is(err, ErrProbeClosing) {
		t.Fatalf("保留期间 GetOrCreate 应返回 probe_closing, got %v", err)
	}
	if fx.factory.callCount() != callsBefore {
		t.Fatal("保留期间不得创建新 manager")
	}
	if fx.sessionCount() != 1 {
		t.Fatal("超时应保留 session 条目")
	}
	// 重入 CloseProbe（幂等重试）：恢复完成通知入口后手动完成并成功删除。
	fx.registry.mu.Lock()
	fx.registry.sessions[Probe1].state = sessionStateActive
	fx.registry.mu.Unlock()
	completeSession(fx, Probe1)
	if err := fx.registry.CloseProbe(context.Background(), Probe1); err != nil {
		t.Fatalf("重入 CloseProbe 应成功: %v", err)
	}
	if _, err := fx.registry.GetOrCreate(Probe1); err != nil {
		t.Fatalf("删除后应可重建: %v", err)
	}
}

func TestManagerRegistry_CloseProbe_ConcurrentProbesNonBlocking(t *testing.T) {
	fx := newRegistryFixture(t)
	for _, probeID := range []ProbeID{Probe1, Probe2} {
		if _, err := fx.registry.GetOrCreate(probeID); err != nil {
			t.Fatalf("GetOrCreate(%s): %v", probeID, err)
		}
	}
	done := make(chan error, 2)
	go func() { done <- fx.registry.CloseProbe(context.Background(), Probe1) }()
	go func() { done <- fx.registry.CloseProbe(context.Background(), Probe2) }()
	for range 2 {
		if err := <-done; err != nil {
			t.Fatalf("terminal manager 并发 CloseProbe: %v", err)
		}
	}
	if fx.registry.tryAcquireAdmissionForTest() {
		fx.registry.releaseAdmission()
	} else {
		t.Fatal("并发 CloseProbe 返回后 admission gate 必须释放")
	}
}

func TestManagerRegistry_CloseProbe_CompletionFailedRetry(t *testing.T) {
	fx := newRegistryFixture(t)
	raw := startProbe1OK(fx)
	if _, err := fx.registry.Start(context.Background(), Probe1, raw); err != nil {
		t.Fatalf("Start: %v", err)
	}
	fx.controllers.setReleaseFail("ctrl-a", true)
	completeSession(fx, Probe1)
	if state, exists := fx.sessionStateOf(Probe1); !exists || state != sessionStateCompletionFailed {
		t.Fatalf("预置 completion_failed 失败: state=%v exists=%v", state, exists)
	}
	// 重试仍失败：保留条目
	if err := fx.registry.CloseProbe(context.Background(), Probe1); err == nil {
		t.Fatal("重试失败时 CloseProbe 应返回错误")
	}
	if fx.sessionCount() != 1 {
		t.Fatal("重试失败应保留 session 条目")
	}
	// 修复后重入：幂等重试成功，删除终态 manager
	fx.controllers.setReleaseFail("ctrl-a", false)
	if err := fx.registry.CloseProbe(context.Background(), Probe1); err != nil {
		t.Fatalf("重试成功应返回 nil: %v", err)
	}
	if fx.activeCount() != 0 || fx.sessionCount() != 0 {
		t.Fatal("重试成功后应提交计数并删除 session")
	}
	if fx.controllers.totalHeld() != 0 {
		t.Fatal("重试成功后控制器 lease 应释放")
	}
}

func TestManagerRegistry_CloseProbe_IdempotentNoManager(t *testing.T) {
	fx := newRegistryFixture(t)
	if err := fx.registry.CloseProbe(context.Background(), Probe1); err != nil {
		t.Fatalf("无 manager 时 CloseProbe 应幂等返回 nil: %v", err)
	}
	if err := fx.registry.CloseProbe(context.Background(), "probe9"); !errors.Is(err, ErrInvalidProbeID) {
		t.Fatalf("未知 probe 应返回 ErrInvalidProbeID, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// Task 7: Shutdown 双 deadline + 并发 EmergencyStop + 聚合错误
// ---------------------------------------------------------------------------

func TestManagerRegistry_Shutdown_GracefulExit(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		fx := newRegistryFixture(t)
		startPairOK(t, fx)
		hookCompletionOnStop(fx, Probe1)
		hookCompletionOnStop(fx, Probe2)

		if err := fx.registry.Shutdown(context.Background()); err != nil {
			t.Fatalf("graceful 全部退出应返回 nil: %v", err)
		}
		if fx.activeCount() != 0 || fx.sessionCount() != 0 {
			t.Fatal("全部 session 应已完成清理")
		}
		for _, id := range []string{"ctrl-a", "ctrl-b"} {
			if n := fx.motion.esCallCount(id); n != 0 {
				t.Fatalf("graceful 成功不得触发 EmergencyStop, %s got %d", id, n)
			}
		}
		// closing：拒绝一切新任务
		if _, err := fx.registry.GetOrCreate(Probe1); !errors.Is(err, ErrRegistryClosing) {
			t.Fatalf("Shutdown 后 GetOrCreate 应拒绝, got %v", err)
		}
		if _, err := fx.registry.Start(context.Background(), Probe1, dualConfigJSON("", "ctrl-a")); !errors.Is(err, ErrRegistryClosing) {
			t.Fatalf("Shutdown 后 Start 应拒绝, got %v", err)
		}
		// 幂等
		if err := fx.registry.Shutdown(context.Background()); err != nil {
			t.Fatalf("重复 Shutdown 应幂等返回 nil: %v", err)
		}
	})
}

func TestManagerRegistry_Shutdown_WaitsForStartManagedPublication(t *testing.T) {
	fx := newRegistryFixture(t)
	raw := startProbe1OK(fx)
	if _, err := fx.registry.GetOrCreate(Probe1); err != nil {
		t.Fatalf("GetOrCreate: %v", err)
	}
	manager := fx.factory.manager(Probe1)
	startEntered, unblockStart := manager.setStartBlock()
	manager.setOnStop(func() {
		_, _, opts := manager.snapshotStart()
		opts.CompletionCallback(opts.Token)
	})

	startDone := make(chan error, 1)
	go func() {
		_, err := fx.registry.Start(context.Background(), Probe1, raw)
		startDone <- err
	}()
	<-startEntered
	if fx.registry.tryAcquireAdmissionForTest() {
		fx.registry.releaseAdmission()
		t.Fatal("StartManaged 阻塞期间 admission gate 必须保持持有")
	}

	shutdownDone := make(chan error, 1)
	shutdownAttempted := make(chan struct{})
	go func() {
		close(shutdownAttempted)
		shutdownDone <- fx.registry.Shutdown(context.Background())
	}()
	<-shutdownAttempted
	select {
	case err := <-shutdownDone:
		t.Fatalf("StartManaged 返回前 Shutdown 不得返回: %v", err)
	default:
	}
	if calls := manager.stopCallCount(); calls != 0 {
		t.Fatalf("StartManaged 返回前 Shutdown 不得到达 manager, calls=%d", calls)
	}

	unblockStart()
	if err := <-startDone; !errors.Is(err, ErrRegistryClosing) {
		t.Fatalf("Shutdown 已进入 closing 时 Start 应返回 registry_closing: %v", err)
	}
	if err := <-shutdownDone; err != nil {
		t.Fatalf("Shutdown: %v", err)
	}
	if calls := manager.stopCallCount(); calls == 0 {
		t.Fatal("启动发布后 Shutdown 必须送达停止请求")
	}
}

func TestManagerRegistry_Shutdown_EscalatesEmergencyStopAndReports(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		fx := newRegistryFixture(t)
		startPairOK(t, fx)
		// fake manager 永不完成；ES 尊重 ctx（hard deadline 触发返回）
		fx.motion.mu.Lock()
		fx.motion.esRespectCtx = true
		fx.motion.mu.Unlock()
		unblockA := fx.motion.setESBlock("ctrl-a")
		defer unblockA()
		unblockB := fx.motion.setESBlock("ctrl-b")
		defer unblockB()

		err := fx.registry.Shutdown(context.Background())
		if !errors.Is(err, ErrShutdownTimeout) {
			t.Fatalf("应返回 shutdown_timeout, got %v", err)
		}
		// 错误包含未退出 probe/task ID，便于诊断
		for _, want := range []string{"probe1/probe1-task-1", "probe2/probe2-task-2"} {
			if !strings.Contains(err.Error(), want) {
				t.Fatalf("shutdown 错误应包含 %q: %v", want, err)
			}
		}
		// 两台控制器均尝试过 EmergencyStop
		for _, id := range []string{"ctrl-a", "ctrl-b"} {
			if n := fx.motion.esCallCount(id); n != 1 {
				t.Fatalf("%s 应 ES 一次, got %d", id, n)
			}
		}
		// 超时任务保留 registry 条目（不删除，供诊断）
		if fx.sessionCount() != 2 {
			t.Fatalf("超时任务应保留条目, sessions=%d", fx.sessionCount())
		}
		// 清理 bubble：手动完成两个 session
		completeSession(fx, Probe1)
		completeSession(fx, Probe2)
	})
}

func TestManagerRegistry_Shutdown_ConcurrentEmergencyStops(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		fx := newRegistryFixture(t)
		startPairOK(t, fx)
		// 两台控制器 ES 均阻塞（不尊重 ctx）：验证单 adapter 卡住不阻止其它
		unblockA := fx.motion.setESBlock("ctrl-a")
		unblockB := fx.motion.setESBlock("ctrl-b")
		entered := make(chan string, 2)
		fx.motion.mu.Lock()
		fx.motion.esEntered = entered
		fx.motion.mu.Unlock()

		shutdownDone := make(chan error, 1)
		go func() { shutdownDone <- fx.registry.Shutdown(context.Background()) }()
		// 两个 ES 都必须进入（第一个未返回时第二个已启动）
		got := map[string]bool{}
		for i := 0; i < 2; i++ {
			got[<-entered] = true
		}
		if !got["ctrl-a"] || !got["ctrl-b"] {
			t.Fatalf("两台控制器应并发进入 ES: %v", got)
		}
		unblockA()
		unblockB()
		err := <-shutdownDone
		if !errors.Is(err, ErrShutdownTimeout) {
			t.Fatalf("manager 未退出时应返回 shutdown_timeout, got %v", err)
		}
		completeSession(fx, Probe1)
		completeSession(fx, Probe2)
	})
}

func TestManagerRegistry_Shutdown_StuckAdapterHardDeadline(t *testing.T) {
	fx := newRegistryFixture(t)
	// 覆盖 deadline：graceful 50ms / hard 200ms（有限正值且 hard > graceful）
	registry, err := NewManagerRegistry(ManagerRegistryDeps{
		Factory:         fx.factory,
		TaskIDGenerator: fx.taskIDs,
		WorkflowLease:   fx.workflow,
		ControllerLease: fx.controllers,
		RecoveryIndex:   fx.index,
		ConfigStore:     fx.configs,
		MotionAccess:    fx.motion,
	}, WithShutdownTimeouts(50*time.Millisecond, 200*time.Millisecond))
	if err != nil {
		t.Fatalf("NewManagerRegistry: %v", err)
	}
	fx.registry = registry
	startPairOK(t, fx)
	// ctrl-a 的 ES 永久阻塞（不尊重 ctx 的 stuck adapter）；ctrl-b 立即返回
	unblockA := fx.motion.setESBlock("ctrl-a")
	defer unblockA() // 测试结束放行，避免 goroutine 泄漏

	start := time.Now()
	err = fx.registry.Shutdown(context.Background())
	elapsed := time.Since(start)
	if !errors.Is(err, ErrShutdownTimeout) {
		t.Fatalf("应返回 shutdown_timeout, got %v", err)
	}
	// 单 adapter 卡住不延长总 deadline（hard 200ms + 充足余量）
	if elapsed > 2*time.Second {
		t.Fatalf("Shutdown 应在 hard deadline 附近返回, elapsed=%s", elapsed)
	}
	if !strings.Contains(err.Error(), "probe1/") {
		t.Fatalf("错误应包含未退出 probe/task ID: %v", err)
	}
	if n := fx.motion.esCallCount("ctrl-b"); n != 1 {
		t.Fatalf("卡住的 ctrl-a 不得阻止 ctrl-b 的 ES 尝试, got %d", n)
	}
	completeSession(fx, Probe1)
	completeSession(fx, Probe2)
}

func TestManagerRegistry_Shutdown_BlockingStopHonorsHardDeadline(t *testing.T) {
	fx := newRegistryFixture(t)
	registry, err := NewManagerRegistry(ManagerRegistryDeps{
		Factory: fx.factory, TaskIDGenerator: fx.taskIDs, WorkflowLease: fx.workflow,
		ControllerLease: fx.controllers, RecoveryIndex: fx.index, ConfigStore: fx.configs,
		MotionAccess: fx.motion,
	}, WithShutdownTimeouts(30*time.Millisecond, 180*time.Millisecond))
	if err != nil {
		t.Fatalf("NewManagerRegistry: %v", err)
	}
	fx.registry = registry
	startPairOK(t, fx)
	unblock1 := fx.factory.manager(Probe1).setStopBlock()
	unblock2 := fx.factory.manager(Probe2).setStopBlock()
	defer unblock1()
	defer unblock2()

	started := time.Now()
	err = fx.registry.Shutdown(context.Background())
	elapsed := time.Since(started)
	if !errors.Is(err, ErrShutdownTimeout) {
		t.Fatalf("blocking Stop 应返回 shutdown_timeout, got %v", err)
	}
	if elapsed > time.Second {
		t.Fatalf("blocking Stop 不得越过 hard deadline, elapsed=%s", elapsed)
	}
	for _, want := range []string{"probe1/probe1-task-1", "probe2/probe2-task-2"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("shutdown diagnostic 缺少 %q: %v", want, err)
		}
	}
	for _, probeID := range []ProbeID{Probe1, Probe2} {
		if calls := fx.factory.manager(probeID).stopCallCount(); calls != 2 {
			t.Fatalf("%s graceful/emergency Stop 应独立尝试两次, got %d", probeID, calls)
		}
	}
}

func TestManagerRegistry_Shutdown_ConfigValidation(t *testing.T) {
	newDeps := func() ManagerRegistryDeps {
		fx := newRegistryFixture(t)
		return ManagerRegistryDeps{
			Factory:         fx.factory,
			TaskIDGenerator: fx.taskIDs,
			WorkflowLease:   fx.workflow,
			ControllerLease: fx.controllers,
			RecoveryIndex:   fx.index,
			ConfigStore:     fx.configs,
		}
	}
	invalid := [][2]time.Duration{{0, time.Second}, {-time.Second, time.Second}, {time.Second, time.Second}, {2 * time.Second, time.Second}}
	for _, pair := range invalid {
		if _, err := NewManagerRegistry(newDeps(), WithShutdownTimeouts(pair[0], pair[1])); err == nil {
			t.Fatalf("graceful=%s hard=%s 应拒绝（有限正值且 hard > graceful）", pair[0], pair[1])
		}
	}
	registry, err := NewManagerRegistry(newDeps(), WithShutdownTimeouts(time.Second, 2*time.Second))
	if err != nil {
		t.Fatalf("合法覆盖应接受: %v", err)
	}
	if registry.gracefulTimeout != time.Second || registry.hardTimeout != 2*time.Second {
		t.Fatal("覆盖值未生效")
	}
	// 默认 5s/10s
	registry, err = NewManagerRegistry(newDeps())
	if err != nil {
		t.Fatalf("默认装配: %v", err)
	}
	if registry.gracefulTimeout != 5*time.Second || registry.hardTimeout != 10*time.Second {
		t.Fatal("默认 graceful 5s / hard 10s")
	}
}

func TestManagerRegistry_Shutdown_CompletionFailedRetry(t *testing.T) {
	fx := newRegistryFixture(t)
	raw := startProbe1OK(fx)
	if _, err := fx.registry.Start(context.Background(), Probe1, raw); err != nil {
		t.Fatalf("Start: %v", err)
	}
	fx.controllers.setReleaseFail("ctrl-a", true)
	completeSession(fx, Probe1)
	if state, _ := fx.sessionStateOf(Probe1); state != sessionStateCompletionFailed {
		t.Fatal("预置 completion_failed 失败")
	}
	// Shutdown 幂等重试失败的 Release：修复后成功提交
	fx.controllers.setReleaseFail("ctrl-a", false)
	if err := fx.registry.Shutdown(context.Background()); err != nil {
		t.Fatalf("completion_failed 重试成功后 Shutdown 应返回 nil: %v", err)
	}
	if fx.activeCount() != 0 || fx.sessionCount() != 0 {
		t.Fatal("重试成功后应提交计数并清理 session")
	}
}

func TestManagerRegistry_Shutdown_ResnapshotsRetainedSessionsWhenClosing(t *testing.T) {
	fx := newRegistryFixture(t)
	if _, err := fx.registry.Start(context.Background(), Probe1, startProbe1OK(fx)); err != nil {
		t.Fatalf("Start: %v", err)
	}
	fx.controllers.setReleaseFail("ctrl-a", true)
	completeSession(fx, Probe1)
	if err := fx.registry.Shutdown(context.Background()); !errors.Is(err, ErrShutdownTimeout) {
		t.Fatalf("首次 Shutdown 应报告 retained session, got %v", err)
	}

	fx.controllers.setReleaseFail("ctrl-a", false)
	if err := fx.registry.Shutdown(context.Background()); err != nil {
		t.Fatalf("closing=true 时重复 Shutdown 应重新快照并重试: %v", err)
	}
	if fx.activeCount() != 0 || fx.sessionCount() != 0 {
		t.Fatal("重复 Shutdown 重试成功后应清理 retained session")
	}
}

func TestManagerRegistry_Shutdown_WaitsForCompletionAfterEmergencyStop(t *testing.T) {
	fx := newRegistryFixture(t)
	registry, err := NewManagerRegistry(ManagerRegistryDeps{
		Factory:         fx.factory,
		TaskIDGenerator: fx.taskIDs,
		WorkflowLease:   fx.workflow,
		ControllerLease: fx.controllers,
		RecoveryIndex:   fx.index,
		ConfigStore:     fx.configs,
		MotionAccess:    fx.motion,
		CheckpointStore: fx.cpStore,
	}, WithShutdownTimeouts(20*time.Millisecond, 300*time.Millisecond))
	if err != nil {
		t.Fatalf("NewManagerRegistry: %v", err)
	}
	fx.registry = registry
	if _, err := fx.registry.Start(context.Background(), Probe1, startProbe1OK(fx)); err != nil {
		t.Fatalf("Start: %v", err)
	}
	manager := fx.factory.manager(Probe1)
	_, _, opts := manager.snapshotStart()
	manager.setOnStop(func() {
		if manager.stopCallCount() != 2 {
			return
		}
		go func() {
			time.Sleep(40 * time.Millisecond)
			opts.CompletionCallback(opts.Token)
		}()
	})

	started := time.Now()
	if err := fx.registry.Shutdown(context.Background()); err != nil {
		t.Fatalf("ES 后 hard deadline 内完成应成功: %v", err)
	}
	if elapsed := time.Since(started); elapsed < 35*time.Millisecond {
		t.Fatalf("Shutdown 必须在 ES 后继续等待 completion, elapsed=%s", elapsed)
	}
}

func TestManagerRegistry_Shutdown_StartupBarrierHonorsHardDeadline(t *testing.T) {
	fx := newRegistryFixture(t)
	registry, err := NewManagerRegistry(ManagerRegistryDeps{
		Factory: fx.factory, TaskIDGenerator: fx.taskIDs, WorkflowLease: fx.workflow,
		ControllerLease: fx.controllers, RecoveryIndex: fx.index, ConfigStore: fx.configs,
		MotionAccess: fx.motion,
	}, WithShutdownTimeouts(20*time.Millisecond, 120*time.Millisecond))
	if err != nil {
		t.Fatalf("NewManagerRegistry: %v", err)
	}
	fx.registry = registry
	raw := startProbe1OK(fx)
	if _, err := fx.registry.GetOrCreate(Probe1); err != nil {
		t.Fatalf("GetOrCreate: %v", err)
	}
	manager := fx.factory.manager(Probe1)
	startEntered, unblockStart := manager.setStartBlock()
	manager.setOnStop(func() {
		_, _, opts := manager.snapshotStart()
		opts.CompletionCallback(opts.Token)
	})
	startDone := make(chan error, 1)
	go func() {
		_, err := fx.registry.Start(context.Background(), Probe1, raw)
		startDone <- err
	}()
	<-startEntered

	started := time.Now()
	err = fx.registry.Shutdown(context.Background())
	if elapsed := time.Since(started); elapsed > 750*time.Millisecond {
		t.Fatalf("Shutdown 等待 startup barrier 必须受 hard deadline 限制, elapsed=%s", elapsed)
	}
	if !errors.Is(err, ErrShutdownTimeout) || !strings.Contains(err.Error(), "startup publication barrier") {
		t.Fatalf("应返回带 startup barrier 诊断的 shutdown_timeout, got %v", err)
	}
	if _, err := fx.registry.Start(context.Background(), Probe2, dualConfigJSON("", "ctrl-b")); !errors.Is(err, ErrRegistryClosing) {
		t.Fatalf("Shutdown 入口必须立即拒绝新准入, got %v", err)
	}

	unblockStart()
	if err := <-startDone; !errors.Is(err, ErrRegistryClosing) {
		t.Fatalf("deadline 后发布的 startup 必须观察 closing, got %v", err)
	}
	deadline := time.After(time.Second)
	for manager.stopCallCount() == 0 {
		select {
		case <-deadline:
			t.Fatal("deadline 后发布的 session 必须请求停止")
		default:
			time.Sleep(time.Millisecond)
		}
	}
}

func TestManagerRegistry_LifecycleWaitsForPublicationAndHonorsContext(t *testing.T) {
	fx := newRegistryFixture(t)
	raw := startProbe1OK(fx)
	if _, err := fx.registry.GetOrCreate(Probe1); err != nil {
		t.Fatalf("GetOrCreate: %v", err)
	}
	manager := fx.factory.manager(Probe1)
	startEntered, unblockStart := manager.setStartBlock()
	startDone := make(chan error, 1)
	go func() {
		_, err := fx.registry.Start(context.Background(), Probe1, raw)
		startDone <- err
	}()
	<-startEntered

	for _, call := range []struct {
		name string
		fn   func(context.Context) error
	}{
		{name: "runPoint", fn: func(ctx context.Context) error { return fx.registry.RunPoint(ctx, Probe1) }},
		{name: "pause", fn: func(ctx context.Context) error { return fx.registry.Pause(ctx, Probe1) }},
		{name: "resume", fn: func(ctx context.Context) error { return fx.registry.Resume(ctx, Probe1) }},
	} {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
		err := call.fn(ctx)
		cancel()
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("%s 等待 publication barrier 应 honor ctx, got %v", call.name, err)
		}
	}
	if run, pause, resume := manager.lifecycleCallCounts(); run != 0 || pause != 0 || resume != 0 {
		t.Fatalf("publication 前不得调用 manager: run=%d pause=%d resume=%d", run, pause, resume)
	}
	unblockStart()
	if err := <-startDone; err != nil {
		t.Fatalf("Start: %v", err)
	}
}

func TestManagerRegistry_LifecycleCallRetainsAdmissionUntilSelectedManagerReturns(t *testing.T) {
	fx := newRegistryFixture(t)
	if _, err := fx.registry.GetOrCreate(Probe1); err != nil {
		t.Fatalf("GetOrCreate: %v", err)
	}
	manager := fx.factory.manager(Probe1)
	manager.mu.Lock()
	pauseDone := make(chan error, 1)
	go func() { pauseDone <- fx.registry.Pause(context.Background(), Probe1) }()

	deadline := time.After(time.Second)
	for fx.registry.tryAcquireAdmissionForTest() {
		fx.registry.releaseAdmission()
		select {
		case <-deadline:
			manager.mu.Unlock()
			t.Fatal("Pause 未进入 manager 调用")
		default:
			time.Sleep(time.Millisecond)
		}
	}
	closeDone := make(chan error, 1)
	go func() { closeDone <- fx.registry.CloseProbe(context.Background(), Probe1) }()
	select {
	case err := <-closeDone:
		manager.mu.Unlock()
		t.Fatalf("manager action 返回前 CloseProbe 不得替换其 generation: %v", err)
	default:
	}

	manager.mu.Unlock()
	if err := <-pauseDone; err != nil {
		t.Fatalf("Pause: %v", err)
	}
	if err := <-closeDone; err != nil {
		t.Fatalf("CloseProbe: %v", err)
	}
	if _, err := fx.registry.GetOrCreate(Probe1); err != nil {
		t.Fatalf("replacement GetOrCreate: %v", err)
	}
	if replacement := fx.factory.manager(Probe1); replacement == manager {
		t.Fatal("CloseProbe 后应创建 replacement generation")
	} else if _, pauses, _ := replacement.lifecycleCallCounts(); pauses != 0 {
		t.Fatalf("旧 action 不得命中 replacement generation, pauses=%d", pauses)
	}
}

func TestManagerRegistry_RunPointRejectsManagedAutoSession(t *testing.T) {
	fx := newRegistryFixture(t)
	if _, err := fx.registry.Start(context.Background(), Probe1, startProbe1OK(fx)); err != nil {
		t.Fatalf("Start: %v", err)
	}
	manager := fx.factory.manager(Probe1)
	if err := fx.registry.RunPoint(context.Background(), Probe1); !errors.Is(err, ErrAlreadyRunning) {
		t.Fatalf("managed auto session 的 RunPoint 应返回 already_running, got %v", err)
	}
	if run, _, _ := manager.lifecycleCallCounts(); run != 0 {
		t.Fatalf("registry 不得在 active auto session 调用 RunCurrentPoint, calls=%d", run)
	}
	completeSession(fx, Probe1)
}

func (r *ManagerRegistry) tryAcquireAdmissionForTest() bool {
	select {
	case <-r.admissionGate:
		return true
	default:
		return false
	}
}
