package usecase

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"
)

// session token / generation 的 exactly-once 完成与 lease 生命周期（Task 5）。
//
// 规格：docs/specs/dual-traversal-spec.md I2/I6；docs/specs/tasks-dual-traversal.md Task 5。
//
// 核心规则：
//   - notifyCompletion 是唯一 completion linearization point；normal/error/Stop/
//     EmergencyStop 各路径最终都收敛到 goroutine exit + finalize 后的同一回调；
//   - token/generation 校验 + session 状态机保证 exactly-once：重复或旧 generation
//     通知只记录诊断，不影响当前 session、controller lease 或全局计数；
//   - 最后一路清理受 workflowTransition gate 保护：先在 mutex 内置位阻止新 admission，
//     锁外释放全局 lease，再在锁内原子提交 activeCount=0 并清除 gate；
//   - Release 失败进入 completion_failed：activeCount 不递减、全局 lease 保留、
//     同 probe Start 禁止；CloseProbe/Shutdown 经 retryCompletionCleanup 幂等重试；
//   - registry mutex 内不执行 Renew/Release 或其它外部 I/O（仅状态转换与不可变快照）。

// ErrRegistryTransitioning 全局 workflow lease 交接中，拒绝新 admission
// （HTTP 层映射 503 registry_transitioning）。
var ErrRegistryTransitioning = errors.New("registry_transitioning")

// registryLeaseRenewInterval lease 续约周期（TTL 的 1/3 安全值）。
const registryLeaseRenewInterval = registryLeaseTTL / 3

// sessionState registry 会话生命周期状态（r.mu 保护读写）。
type sessionState int

const (
	// sessionStateActive 活动会话（lease 已预占，续约器运行中）。
	sessionStateActive sessionState = iota
	// sessionStateCompleting completion 清理进行中（session 保留在 map，阻止同 probe Start）。
	sessionStateCompleting
	// sessionStateCompletionFailed 清理失败：activeCount 未递减、lease 保留，待幂等重试。
	sessionStateCompletionFailed
	// sessionStateCompleted 已提交（计数递减、lease 释放、done 关闭）。
	sessionStateCompleted
)

func (s sessionState) String() string {
	switch s {
	case sessionStateActive:
		return "active"
	case sessionStateCompleting:
		return "completing"
	case sessionStateCompletionFailed:
		return "completion_failed"
	case sessionStateCompleted:
		return "completed"
	default:
		return fmt.Sprintf("unknown(%d)", int(s))
	}
}

// notifyCompletion registry 唯一 completion 入口（spec I2 linearization point）。
//
// 旧 generation 或重复通知只记录诊断：不释放当前 controller lease、不停止当前
// 续约器、不影响当前 session 与全局计数。
func (r *ManagerRegistry) notifyCompletion(token SessionToken) {
	r.mu.Lock()
	session, ok := r.sessions[token.ProbeID]
	if !ok || session.token != token {
		r.mu.Unlock()
		slog.Warn("忽略旧 generation 完成通知", "token", token.String())
		return
	}
	if session.state != sessionStateActive {
		state := session.state
		r.mu.Unlock()
		slog.Warn("忽略重复完成通知", "token", token.String(), "state", state.String())
		return
	}
	session.state = sessionStateCompleting
	r.mu.Unlock()

	// 锁外：先停续约器再做 lease 清理，避免 Renew 与 Release 竞态。
	stopRenewer(session.renewCancel, session.renewDone)
	if err := r.cleanupCompletedSession(session); err != nil {
		slog.Error("session 清理失败（可经 CloseProbe/Shutdown 幂等重试）", "token", token.String(), "error", err)
	}
}

// cleanupCompletedSession completion 清理：controller leases →（最后一路）全局 lease
// → 原子提交。幂等：已成功步骤不重复执行；任何失败把 session 置为 completion_failed
// 并保留计数与 lease，供 retryCompletionCleanup 重试。
// 到达静态（completed/completion_failed）后关闭 session.settled（恰好一次），
// 使 Stop/CloseProbe 能及时唤醒进入重试分支。
func (r *ManagerRegistry) cleanupCompletedSession(session *registrySession) error {
	return r.cleanupCompletedSessionWithGate(session, false)
}

func (r *ManagerRegistry) cleanupCompletedSessionUnderGate(session *registrySession) error {
	return r.cleanupCompletedSessionWithGate(session, true)
}

func (r *ManagerRegistry) cleanupCompletedSessionWithGate(session *registrySession, gateHeld bool) error {
	session.cleanupMu.Lock()
	defer session.cleanupMu.Unlock()
	err := r.runSessionCleanup(session, gateHeld)
	r.mu.Lock()
	quiescent := session.state == sessionStateCompleted || session.state == sessionStateCompletionFailed
	r.mu.Unlock()
	if quiescent {
		session.settledOnce.Do(func() { close(session.settled) })
	}
	return err
}

// runSessionCleanup cleanup 主体（调用方持有 session.cleanupMu）。
func (r *ManagerRegistry) runSessionCleanup(session *registrySession, gateHeld bool) error {
	alreadyCompleted, rollback := r.prepareSessionCleanup(session)
	if alreadyCompleted {
		return nil
	}
	if !rollback {
		if err := r.completeRecoveryMapping(session); err != nil {
			r.mu.Lock()
			session.recoveryErr = err
			session.state = sessionStateCompletionFailed
			session.completionErr = err
			r.mu.Unlock()
			return err
		}
		r.mu.Lock()
		session.recoveryErr = nil
		r.mu.Unlock()
	}
	if err := r.releasePendingControllers(session); err != nil {
		r.mu.Lock()
		session.state = sessionStateCompletionFailed
		session.completionErr = err
		r.mu.Unlock()
		return err
	}
	// Admission and the final-session decision share one gate through lease release
	// and count commit, so a new session cannot be admitted into that window.
	//
	// 使用 bounded context（I-9）：context.Background() 无法取消，admission gate
	// 若被泄漏（panic/bug）将永久阻塞 cleanup 路径，导致 completion_failed 状态
	// 无法收敛、CloseProbe/Shutdown 重试也卡死。bounded 超时让 cleanup 失败可见，
	// 进入 completion_failed 后由重试机制兜底。
	if !gateHeld {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), registryLeaseTTL)
		defer cancel()
		if err := r.acquireAdmission(cleanupCtx); err != nil {
			return fmt.Errorf("cleanup 获取 admission gate 失败（可能被泄漏）: %w", err)
		}
		defer r.releaseAdmission()
	}
	r.mu.Lock()
	if r.activeCount > 1 {
		r.activeCount--
		r.commitSessionLocked(session)
		r.mu.Unlock()
		return nil
	}
	r.workflowTransition = true
	renewCancel, renewDone := r.workflowRenewCancel, r.workflowRenewDone
	r.workflowRenewCancel, r.workflowRenewDone = nil, nil
	r.mu.Unlock()

	// 锁外：先停全局续约器再释放全局 lease。
	stopRenewer(renewCancel, renewDone)
	releaseErr := r.workflowLease.Release(context.Background(), registryWorkflowHolder)

	r.mu.Lock()
	if releaseErr != nil {
		// activeCount 保持 1、gate 保持置位、session 保留 completion_failed；
		// 重启全局续约器，防止保留期间 lease 过期被接管。
		session.state = sessionStateCompletionFailed
		session.completionErr = fmt.Errorf("释放全局 workflow lease 失败: %w", releaseErr)
		r.startWorkflowRenewerLocked()
		r.mu.Unlock()
		return session.completionErr
	}
	r.workflowTransition = false
	r.activeCount = 0
	r.commitSessionLocked(session)
	r.mu.Unlock()
	return nil
}

func (r *ManagerRegistry) prepareSessionCleanup(session *registrySession) (bool, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	alreadyCompleted := session.state == sessionStateCompleted
	return alreadyCompleted, session.rollback
}

// releasePendingControllers 释放 session 尚未成功释放的控制器轴 lease；
// 成功的条目从 pendingReleases 删除（重试幂等），失败聚合返回。
//
// pendingReleases 的 key 为 ControllerAxisPair（(controllerID, axis) 元组），
// 同一控制器的不同轴 lease 互相独立——释放 X 轴不影响 Y 轴。
func (r *ManagerRegistry) releasePendingControllers(session *registrySession) error {
	if len(session.pendingReleases) == 0 {
		return nil
	}
	released := make([]ControllerAxisPair, 0, len(session.pendingReleases))
	var errs []error
	for pair, leaseToken := range session.pendingReleases {
		if err := r.controllerLease.Release(context.Background(), leaseToken); err != nil {
			errs = append(errs, fmt.Errorf("释放控制器 %s 轴 %s lease: %w", pair.ControllerID, pair.Axis, err))
			continue
		}
		released = append(released, pair)
	}
	for _, pair := range released {
		delete(session.pendingReleases, pair)
	}
	return errors.Join(errs...)
}

// commitSessionLocked 原子提交 completion：状态置 completed、移出 map、关闭 done。
// 调用方持有 r.mu，且负责在调用前完成 activeCount 递减。
func (r *ManagerRegistry) commitSessionLocked(session *registrySession) {
	session.state = sessionStateCompleted
	delete(r.sessions, session.probeID)
	close(session.done)
}

// retryCompletionCleanup 供 Task 6 CloseProbe/Shutdown 以同 token 幂等重试失败的
// Release；成功后才提交计数并关闭 completion done。已完成时返回 nil（幂等）。
func (r *ManagerRegistry) retryCompletionCleanup(session *registrySession) error {
	stopRenewer(session.renewCancel, session.renewDone)
	return r.cleanupCompletedSession(session)
}

// startSessionRenewerLocked 启动 session 控制器 lease 续约器（调用方持有 r.mu；
// goroutine 启动非外部 I/O，允许在临界区内执行）。
func (r *ManagerRegistry) startSessionRenewerLocked(session *registrySession) {
	ctx, cancel := context.WithCancel(context.Background())
	session.renewCancel = cancel
	session.renewDone = make(chan struct{})
	go r.runSessionRenewer(session, ctx)
}

// runSessionRenewer 每 session 控制器 lease 续约器：按 TTL/3 周期续约；
// 续约失败写入可诊断错误并请求该 probe 停止（不允许静默继续到 lease 过期）；
// 由 session context 管理，completion 前被取消后退出。
func (r *ManagerRegistry) runSessionRenewer(session *registrySession, ctx context.Context) {
	defer close(session.renewDone)
	ticker := time.NewTicker(registryLeaseRenewInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
		// ticker 通道缓冲的滞留 tick 可能与 cancel 同时就绪；
		// 续约前二次确认，避免 session 拆除期间误续约（及真实 adapter 的 ctx 取消错误）。
		select {
		case <-ctx.Done():
			return
		default:
		}
		if err := r.renewSessionControllers(ctx, session); err != nil {
			slog.Error("控制器 lease 续约失败，请求停止该 probe",
				"probe", session.probeID, "task", session.taskID, "error", err)
			r.mu.Lock()
			session.renewErr = err
			r.mu.Unlock()
			if stopErr := session.manager.Stop(); stopErr != nil {
				slog.Error("续约失败后请求停止 probe 失败", "probe", session.probeID, "error", stopErr)
			}
			return
		}
	}
}

// renewSessionControllers 续约 session 全部控制器轴 lease。
// controllerTokens 自 admission 后不可变，读取无需持锁。
// key 为 ControllerAxisPair，同一控制器的不同轴 lease 互相独立续约。
func (r *ManagerRegistry) renewSessionControllers(ctx context.Context, session *registrySession) error {
	for pair, leaseToken := range session.controllerTokens {
		if err := r.controllerLease.Renew(ctx, leaseToken, registryLeaseTTL); err != nil {
			return fmt.Errorf("续约控制器 %s 轴 %s lease: %w", pair.ControllerID, pair.Axis, err)
		}
	}
	return nil
}

// startWorkflowRenewerLocked 启动全局 workflow lease 续约器（调用方持有 r.mu）。
func (r *ManagerRegistry) startWorkflowRenewerLocked() {
	if r.workflowRenewCancel != nil {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	r.workflowRenewCancel = cancel
	r.workflowRenewDone = make(chan struct{})
	go r.runWorkflowRenewer(ctx, r.workflowRenewDone)
}

// runWorkflowRenewer 全局 workflow lease 续约器（固定 holder）；
// 续约失败请求全部活动 probe 停止（互斥语义可能已被破坏，不允许静默继续）。
func (r *ManagerRegistry) runWorkflowRenewer(ctx context.Context, done chan<- struct{}) {
	defer close(done)
	ticker := time.NewTicker(registryLeaseRenewInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
		// 同 runSessionRenewer：滞留 tick 与 cancel 同时就绪时不得误续约。
		select {
		case <-ctx.Done():
			return
		default:
		}
		if err := r.workflowLease.Renew(ctx, registryWorkflowHolder, registryLeaseTTL); err != nil {
			slog.Error("全局 workflow lease 续约失败，请求停止全部活动 probe", "error", err)
			r.requestStopAllSessions(err)
			return
		}
	}
}

// requestStopAllSessions 请求全部活动 session 停止（续约失败的止损路径）。
func (r *ManagerRegistry) requestStopAllSessions(cause error) {
	r.mu.Lock()
	sessions := make([]*registrySession, 0, len(r.sessions))
	for _, session := range r.sessions {
		if session.state == sessionStateActive {
			session.renewErr = cause
			sessions = append(sessions, session)
		}
	}
	r.mu.Unlock()
	for _, session := range sessions {
		if err := session.manager.Stop(); err != nil {
			slog.Error("workflow 续约失败后请求停止 probe 失败", "probe", session.probeID, "error", err)
		}
	}
}

// stopRenewer 停止续约器并等待其退出（在 registry mutex 外调用）。
func stopRenewer(cancel context.CancelFunc, done chan struct{}) {
	if cancel != nil {
		cancel()
	}
	if done != nil {
		<-done
	}
}
