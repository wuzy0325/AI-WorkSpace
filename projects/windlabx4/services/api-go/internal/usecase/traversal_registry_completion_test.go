package usecase

import (
	"context"
	"errors"
	"sync"
	"testing"
	"testing/synctest"
	"time"
)

// ---------------------------------------------------------------------------
// Task 5: session token / generation 的 exactly-once 完成与 lease 生命周期
// ---------------------------------------------------------------------------

// completeSession 模拟 manager goroutine exit + finalize 后的完成回调。
func completeSession(fx *registryFixture, probeID ProbeID) {
	fx.registry.mu.Lock()
	session := fx.registry.sessions[probeID]
	fx.registry.mu.Unlock()
	if session == nil {
		return
	}
	fx.registry.notifyCompletion(session.token)
}

func startPairOK(t *testing.T, fx *registryFixture) {
	t.Helper()
	fx.seedPersistedBindings(Probe1, "ctrl-a")
	fx.seedPersistedBindings(Probe2, "ctrl-b")
	ctx := context.Background()
	if _, err := fx.registry.Start(ctx, Probe1, dualConfigJSON("", "ctrl-a")); err != nil {
		t.Fatalf("Start probe1: %v", err)
	}
	if _, err := fx.registry.Start(ctx, Probe2, dualConfigJSON("", "ctrl-b")); err != nil {
		t.Fatalf("Start probe2: %v", err)
	}
}

func TestManagerRegistry_Completion_SingleReleasesLeases(t *testing.T) {
	fx := newRegistryFixture(t)
	raw := startProbe1OK(fx)
	if _, err := fx.registry.Start(context.Background(), Probe1, raw); err != nil {
		t.Fatalf("Start: %v", err)
	}
	session := fx.sessionFor(Probe1)

	completeSession(fx, Probe1)

	if fx.activeCount() != 0 {
		t.Fatalf("正常完成应递减一次, active=%d", fx.activeCount())
	}
	if held, _, _, releases := fx.workflow.state(); held || releases != 1 {
		t.Fatalf("归零时应释放全局 lease: held=%v releases=%d", held, releases)
	}
	if fx.controllers.totalHeld() != 0 {
		t.Fatal("控制器 lease 应全部释放")
	}
	select {
	case <-session.done:
	default:
		t.Fatal("completion done 应已关闭")
	}
	if _, exists := fx.sessionStateOf(Probe1); exists {
		t.Fatal("已提交 session 应移出 map")
	}
	if _, renewerRunning := fx.workflowGate(); renewerRunning {
		t.Fatal("全局续约器应随最后一路提交退出")
	}
}

func TestManagerRegistry_Completion_TwoProbesSequential(t *testing.T) {
	fx := newRegistryFixture(t)
	startPairOK(t, fx)

	completeSession(fx, Probe1)
	if fx.activeCount() != 1 {
		t.Fatalf("第一路完成后 active 应为 1, got %d", fx.activeCount())
	}
	if held, _, _, releases := fx.workflow.state(); !held || releases != 0 {
		t.Fatalf("一路结束期间全局 lease 应仍持有: held=%v releases=%d", held, releases)
	}
	if fx.controllers.heldCount("ctrl-a") != 0 || fx.controllers.heldCount("ctrl-b") != 1 {
		t.Fatal("仅完成路的控制器 lease 应释放")
	}

	completeSession(fx, Probe2)
	if fx.activeCount() != 0 {
		t.Fatalf("最后一路完成后 active 应为 0, got %d", fx.activeCount())
	}
	if held, _, _, releases := fx.workflow.state(); held || releases != 1 {
		t.Fatalf("最后一路清理后才释放全局 lease: held=%v releases=%d", held, releases)
	}
	if fx.controllers.totalHeld() != 0 {
		t.Fatal("全部控制器 lease 应释放")
	}
}

func TestManagerRegistry_Completion_ConcurrentBoth(t *testing.T) {
	fx := newRegistryFixture(t)
	startPairOK(t, fx)
	s1, s2 := fx.sessionFor(Probe1), fx.sessionFor(Probe2)

	var ready sync.WaitGroup
	ready.Add(2)
	go func() { ready.Done(); fx.registry.notifyCompletion(s1.token) }()
	go func() { ready.Done(); fx.registry.notifyCompletion(s2.token) }()
	ready.Wait()
	// 等待双方提交（done 在 completion 提交点关闭）
	<-s1.done
	<-s2.done

	if fx.activeCount() != 0 {
		t.Fatalf("并发完成后 active 应为 0, got %d", fx.activeCount())
	}
	if _, _, _, releases := fx.workflow.state(); releases != 1 {
		t.Fatalf("全局 lease 应恰好释放一次, got %d", releases)
	}
	if fx.controllers.totalHeld() != 0 {
		t.Fatal("控制器 lease 应全部释放")
	}
}

func TestManagerRegistry_Completion_TwoFinalizersLinearizeThirdStart(t *testing.T) {
	fx := newRegistryFixture(t)
	startPairOK(t, fx)
	s1, s2 := fx.sessionFor(Probe1), fx.sessionFor(Probe2)
	entered, unblock := fx.workflow.setReleaseBlock()
	defer unblock()

	completionDone := make(chan struct{}, 2)
	go func() { fx.registry.notifyCompletion(s1.token); completionDone <- struct{}{} }()
	go func() { fx.registry.notifyCompletion(s2.token); completionDone <- struct{}{} }()
	<-entered

	startDone := make(chan error, 1)
	go func() {
		_, err := fx.registry.Start(context.Background(), Probe1, dualConfigJSON("", "ctrl-a"))
		startDone <- err
	}()
	var startErr error
	select {
	case err := <-startDone:
		startErr = err
		if !errors.Is(err, ErrRegistryTransitioning) && !errors.Is(err, ErrAlreadyRunning) {
			t.Fatalf("final commit 前第三路不得准入, got %v", err)
		}
	case <-time.After(100 * time.Millisecond):
		// The gate may serialize the start until final count commit.
	}

	unblock()
	for i := 0; i < 2; i++ {
		<-completionDone
	}
	if startErr == nil {
		select {
		case startErr = <-startDone:
		case <-time.After(time.Second):
			t.Fatal("third Start did not leave admission gate after final commit")
		}
	}
	if startErr != nil {
		if _, err := fx.registry.Start(context.Background(), Probe1, dualConfigJSON("", "ctrl-a")); err != nil {
			t.Fatalf("final commit 后第三路应完整重新准入: %v", err)
		}
	}
	if fx.activeCount() != 1 {
		t.Fatalf("第三路准入后 activeCount 应为 1, got %d", fx.activeCount())
	}
	if _, _, acquires, releases := fx.workflow.state(); acquires != 2 || releases != 1 {
		t.Fatalf("workflow lease 必须 release 后重新 acquire: acquires=%d releases=%d", acquires, releases)
	}
}

func TestManagerRegistry_Completion_DuplicateNotificationIdempotent(t *testing.T) {
	fx := newRegistryFixture(t)
	raw := startProbe1OK(fx)
	if _, err := fx.registry.Start(context.Background(), Probe1, raw); err != nil {
		t.Fatalf("Start: %v", err)
	}
	session := fx.sessionFor(Probe1)

	// 并发重复通知（同一 token 二次/多次）：仅一次递减、一次释放
	const calls = 8
	var ready sync.WaitGroup
	ready.Add(calls)
	for i := 0; i < calls; i++ {
		go func() { ready.Done(); fx.registry.notifyCompletion(session.token) }()
	}
	ready.Wait()
	<-session.done

	if fx.activeCount() != 0 {
		t.Fatalf("重复完成不得重复递减, active=%d", fx.activeCount())
	}
	if _, _, _, releases := fx.workflow.state(); releases != 1 {
		t.Fatalf("全局 lease 应只释放一次, got %d", releases)
	}
	if fx.controllers.totalHeld() != 0 {
		t.Fatal("控制器 lease 应只释放一轮")
	}
}

func TestManagerRegistry_Completion_StaleGenerationIgnored(t *testing.T) {
	fx := newRegistryFixture(t)
	raw := startProbe1OK(fx)
	ctx := context.Background()

	if _, err := fx.registry.Start(ctx, Probe1, raw); err != nil {
		t.Fatalf("Start gen1: %v", err)
	}
	stale := fx.sessionFor(Probe1).token
	completeSession(fx, Probe1)
	if fx.activeCount() != 0 {
		t.Fatal("gen1 应已完成")
	}
	if _, err := fx.registry.Start(ctx, Probe1, raw); err != nil {
		t.Fatalf("Start gen2: %v", err)
	}
	current := fx.sessionFor(Probe1)

	// 旧 generation 完成通知：不影响当前 session、controller lease、续约器与计数
	fx.registry.notifyCompletion(stale)
	if fx.activeCount() != 1 {
		t.Fatal("旧 generation 通知不得递减计数")
	}
	if fx.controllers.heldCount("ctrl-a") != 1 {
		t.Fatal("旧 generation 通知不得释放当前控制器 lease")
	}
	select {
	case <-current.renewDone:
		t.Fatal("旧 generation 通知不得停止当前续约器")
	default:
	}
	if state, _ := fx.sessionStateOf(Probe1); state != sessionStateActive {
		t.Fatalf("当前 session 应保持 active, got %v", state)
	}
	completeSession(fx, Probe1)
	if fx.activeCount() != 0 {
		t.Fatal("当前 generation 完成应正常清理")
	}
}

func TestManagerRegistry_Completion_ControllerReleaseFailureRetryable(t *testing.T) {
	fx := newRegistryFixture(t)
	raw := startProbe1OK(fx)
	if _, err := fx.registry.Start(context.Background(), Probe1, raw); err != nil {
		t.Fatalf("Start: %v", err)
	}
	session := fx.sessionFor(Probe1)
	fx.controllers.setReleaseFail("ctrl-a", true)

	completeSession(fx, Probe1)

	// completion_failed：activeCount 不递减、全局 lease 保留、同 probe Start 禁止、done 未关闭
	if fx.activeCount() != 1 {
		t.Fatalf("controller Release 失败时 activeCount 应保持 1, got %d", fx.activeCount())
	}
	if state, exists := fx.sessionStateOf(Probe1); !exists || state != sessionStateCompletionFailed {
		t.Fatalf("session 应为 completion_failed, state=%v exists=%v", state, exists)
	}
	if held, _, _, releases := fx.workflow.state(); !held || releases != 0 {
		t.Fatalf("全局 lease 应保留: held=%v releases=%d", held, releases)
	}
	if fx.controllers.heldCount("ctrl-a") != 1 {
		t.Fatal("失败控制器 lease 应保留待重试")
	}
	select {
	case <-session.done:
		t.Fatal("失败时不得关闭 completion done")
	default:
	}
	if _, err := fx.registry.Start(context.Background(), Probe1, raw); !errors.Is(err, ErrAlreadyRunning) {
		t.Fatalf("completion_failed 时同 probe Start 应禁止, got %v", err)
	}
	if session.completionErr == nil {
		t.Fatal("completionErr 应记录可诊断错误")
	}

	// 幂等重试：修复后成功提交计数与 done
	fx.controllers.setReleaseFail("ctrl-a", false)
	if err := fx.registry.retryCompletionCleanup(session); err != nil {
		t.Fatalf("重试应成功: %v", err)
	}
	if fx.activeCount() != 0 {
		t.Fatal("重试成功后应提交计数")
	}
	select {
	case <-session.done:
	default:
		t.Fatal("重试成功后应关闭 done")
	}
	if fx.controllers.totalHeld() != 0 {
		t.Fatal("重试成功后控制器 lease 应释放")
	}
	if held, _, _, releases := fx.workflow.state(); held || releases != 1 {
		t.Fatalf("最后一路重试成功应释放全局 lease: held=%v releases=%d", held, releases)
	}
	// 二次重试幂等
	if err := fx.registry.retryCompletionCleanup(session); err != nil {
		t.Fatalf("已完成 session 的重试应幂等返回 nil: %v", err)
	}
}

func TestManagerRegistry_Completion_WorkflowReleaseFailureRetryable(t *testing.T) {
	fx := newRegistryFixture(t)
	raw := startProbe1OK(fx)
	ctx := context.Background()
	if _, err := fx.registry.Start(ctx, Probe1, raw); err != nil {
		t.Fatalf("Start: %v", err)
	}
	session := fx.sessionFor(Probe1)
	releaseBoom := errors.New("workflow release boom")
	fx.workflow.setReleaseErr(releaseBoom)

	completeSession(fx, Probe1)

	// 最后一路 global Release 失败：activeCount 保持 1、session completion_failed、
	// gate 保持置位（不得报告完成或允许新 Start）、全局续约器重启防止 lease 过期
	if fx.activeCount() != 1 {
		t.Fatalf("global Release 失败时 activeCount 应保持 1, got %d", fx.activeCount())
	}
	if state, exists := fx.sessionStateOf(Probe1); !exists || state != sessionStateCompletionFailed {
		t.Fatalf("session 应保留 completion_failed, state=%v exists=%v", state, exists)
	}
	transition, renewerRunning := fx.workflowGate()
	if !transition {
		t.Fatal("失败时 transition gate 应保持置位")
	}
	if !renewerRunning {
		t.Fatal("保留期间应重启全局续约器")
	}
	select {
	case <-session.done:
		t.Fatal("失败时不得关闭 completion done")
	default:
	}
	if _, err := fx.registry.Start(ctx, Probe2, dualConfigJSON("", "ctrl-b")); !errors.Is(err, ErrRegistryTransitioning) {
		t.Fatalf("gate 置位期间新 Start 应返回 registry_transitioning, got %v", err)
	}

	// 幂等重试成功后才提交计数、清除 gate、关闭 done
	fx.workflow.setReleaseErr(nil)
	if err := fx.registry.retryCompletionCleanup(session); err != nil {
		t.Fatalf("重试应成功: %v", err)
	}
	if fx.activeCount() != 0 {
		t.Fatal("重试成功后 activeCount 应归零")
	}
	transition, renewerRunning = fx.workflowGate()
	if transition || renewerRunning {
		t.Fatalf("提交后 gate 应清除、续约器应退出: transition=%v renewer=%v", transition, renewerRunning)
	}
	select {
	case <-session.done:
	default:
		t.Fatal("重试成功后应关闭 done")
	}
	if _, err := fx.registry.Start(ctx, Probe2, dualConfigJSON("", "ctrl-b")); err != nil {
		t.Fatalf("提交后新 Start 应成功: %v", err)
	}
}

func TestManagerRegistry_Completion_TransitionGateNoLeaseStateReuse(t *testing.T) {
	fx := newRegistryFixture(t)
	raw := startProbe1OK(fx)
	ctx := context.Background()
	if _, err := fx.registry.Start(ctx, Probe1, raw); err != nil {
		t.Fatalf("Start: %v", err)
	}
	session := fx.sessionFor(Probe1)
	entered, unblock := fx.workflow.setReleaseBlock()

	// completion 阻塞在全局 lease 释放窗口（gate 已置位）
	done := make(chan struct{})
	go func() {
		fx.registry.notifyCompletion(session.token)
		close(done)
	}()
	<-entered

	// 窗口期间新 Start：返回稳定 registry_transitioning，不得复用旧 lease 状态
	if _, err := fx.registry.Start(ctx, Probe2, dualConfigJSON("", "ctrl-b")); !errors.Is(err, ErrRegistryTransitioning) {
		t.Fatalf("gate 期间应返回 registry_transitioning, got %v", err)
	}
	if _, _, acquires, _ := fx.workflow.state(); acquires != 1 {
		t.Fatalf("窗口期间不得跳过/重复 Acquire, acquires=%d", acquires)
	}
	unblock()
	<-done
	<-session.done

	// 提交后新 Start 走完整 Acquire（不按旧 activeCount 跳过）
	fx.seedPersistedBindings(Probe2, "ctrl-b")
	if _, err := fx.registry.Start(ctx, Probe2, dualConfigJSON("", "ctrl-b")); err != nil {
		t.Fatalf("提交后 Start 应成功: %v", err)
	}
	if _, _, acquires, _ := fx.workflow.state(); acquires != 2 {
		t.Fatalf("提交后应重新 Acquire 全局 lease, acquires=%d", acquires)
	}
}

func TestManagerRegistry_Completion_ConcurrentWithNewStart(t *testing.T) {
	fx := newRegistryFixture(t)
	raw := startProbe1OK(fx)
	ctx := context.Background()
	if _, err := fx.registry.Start(ctx, Probe1, raw); err != nil {
		t.Fatalf("Start: %v", err)
	}
	session := fx.sessionFor(Probe1)
	entered, unblock := fx.workflow.setReleaseBlock()

	done := make(chan struct{})
	go func() {
		fx.registry.notifyCompletion(session.token)
		close(done)
	}()
	<-entered

	// 并发 NotifyCompletion 与同 probe 新 Start：完成提交前不得复用 probe ID
	if _, err := fx.registry.Start(ctx, Probe1, raw); err == nil {
		t.Fatal("旧 session 完成提交前同 probe Start 应被拒绝")
	}
	unblock()
	<-done
	<-session.done

	if _, err := fx.registry.Start(ctx, Probe1, raw); err != nil {
		t.Fatalf("完成提交后同 probe Start 应成功: %v", err)
	}
	if token := fx.sessionFor(Probe1).token; token.Generation != 2 {
		t.Fatalf("新 session generation 应为 2, got %d", token.Generation)
	}
}

func TestManagerRegistry_Completion_RenewalLifecycleAndExit(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		fx := newRegistryFixture(t)
		startPairOK(t, fx)

		// 假时钟推进 35s：10s 周期下控制器与全局 lease 各续约 3 次
		time.Sleep(35 * time.Second)
		if n := fx.controllers.renewCount(); n != 6 {
			t.Fatalf("两个 session 应各续约 3 次（共 6 次）, got %d", n)
		}
		if n := fx.workflow.renewCount(); n != 3 {
			t.Fatalf("全局 lease 应续约 3 次, got %d", n)
		}

		// 第一路完成：其续约器退出；全局续约器保留（lease 仍持有）
		completeSession(fx, Probe1)
		time.Sleep(35 * time.Second)
		if n := fx.workflow.renewCount(); n <= 3 {
			t.Fatal("一路完成期间全局续约器应继续运行")
		}
		if fx.controllers.heldCount("ctrl-b") != 1 {
			t.Fatal("probe2 控制器 lease 应仍持有")
		}

		// 最后一路完成：全部续约器退出，此后时间推进不再产生续约。
		// 快照在 complete 之后取（续约器已同步退出，睡眠边界的滞留 tick 不再干扰）。
		completeSession(fx, Probe2)
		controllerRenews := fx.controllers.renewCount()
		workflowRenews := fx.workflow.renewCount()
		time.Sleep(35 * time.Second)
		if fx.controllers.renewCount() != controllerRenews {
			t.Fatal("completion 后控制器续约器必须退出")
		}
		if fx.workflow.renewCount() != workflowRenews {
			t.Fatal("最后一路提交后全局续约器必须退出")
		}
	})
}

func TestManagerRegistry_Completion_RenewFailureStopsProbe(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		fx := newRegistryFixture(t)
		raw := startProbe1OK(fx)
		if _, err := fx.registry.Start(context.Background(), Probe1, raw); err != nil {
			t.Fatalf("Start: %v", err)
		}
		session := fx.sessionFor(Probe1)
		renewBoom := errors.New("renew boom")
		fx.controllers.setRenewErr(renewBoom)

		// 首个续约周期失败：写入可诊断错误并请求该 probe 停止，不静默继续
		time.Sleep(15 * time.Second)
		if calls := fx.factory.manager(Probe1).stopCallCount(); calls != 1 {
			t.Fatalf("续约失败应请求停止该 probe 一次, got %d", calls)
		}
		fx.registry.mu.Lock()
		renewErr := session.renewErr
		fx.registry.mu.Unlock()
		if !errors.Is(renewErr, renewBoom) {
			t.Fatalf("session 应记录续约失败诊断, got %v", renewErr)
		}
		// 续约器失败后退出（不再反复 Stop）
		select {
		case <-session.renewDone:
		default:
			t.Fatal("续约失败后续约器应退出")
		}
		time.Sleep(35 * time.Second)
		if calls := fx.factory.manager(Probe1).stopCallCount(); calls != 1 {
			t.Fatalf("续约器退出后不得重复 Stop, got %d", calls)
		}

		// fake manager 不自动完成：测试手动注入完成回调（模拟 goroutine exit + finalize）
		fx.controllers.setRenewErr(nil)
		completeSession(fx, Probe1)
		if fx.activeCount() != 0 {
			t.Fatal("完成后应正常清理")
		}
	})
}

func TestManagerRegistry_Completion_WorkflowRenewFailureStopsAll(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		fx := newRegistryFixture(t)
		startPairOK(t, fx)
		fx.workflow.setRenewErr(errors.New("workflow renew boom"))

		time.Sleep(15 * time.Second)
		for _, probeID := range []ProbeID{Probe1, Probe2} {
			if calls := fx.factory.manager(probeID).stopCallCount(); calls != 1 {
				t.Fatalf("workflow 续约失败应请求全部活动 probe 停止, %s got %d", probeID, calls)
			}
		}
		// 两路分别完成（fake manager 不自动完成，手动注入回调）
		fx.workflow.setRenewErr(nil)
		completeSession(fx, Probe1)
		completeSession(fx, Probe2)
		if fx.activeCount() != 0 {
			t.Fatal("全部完成后 activeCount 应归零")
		}
	})
}

func TestManagerRegistry_Completion_RollbackDoesNotNotifyCompletion(t *testing.T) {
	// 回归确认（Task 4 AC）：准入回滚路径直接撤销临时资源，不走 NotifyCompletion。
	fx := newRegistryFixture(t)
	ctx := context.Background()
	if _, err := fx.registry.GetOrCreate(Probe1); err != nil {
		t.Fatalf("GetOrCreate: %v", err)
	}
	fx.factory.manager(Probe1).setStartErr(errors.New("start boom"))
	raw := startProbe1OK(fx)
	if _, err := fx.registry.Start(ctx, Probe1, raw); err == nil {
		t.Fatal("Start 应失败")
	}
	if fx.activeCount() != 0 || fx.sessionCount() != 0 {
		t.Fatal("回滚后不得有活动 session（未计入则不得减一）")
	}
	// 回滚后新一轮准入可用：证明回滚未污染 lease 状态（无"未计入却减一"窗口）
	fx.factory.manager(Probe1).setStartErr(nil)
	if _, err := fx.registry.Start(ctx, Probe1, raw); err != nil {
		t.Fatalf("回滚后重试应成功: %v", err)
	}
	if fx.activeCount() != 1 {
		t.Fatalf("重试后 activeCount 应为 1, got %d", fx.activeCount())
	}
}
