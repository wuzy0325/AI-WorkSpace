package usecase

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"time"
)

// registry Stop / CloseProbe / Shutdown 生命周期（Task 6 / Task 7）。
//
// 规格：docs/specs/dual-traversal-spec.md I6/FR9；docs/specs/tasks-dual-traversal.md
// Task 6/Task 7。
//
// 核心约定：
//   - 唯一 completion 收敛点在 Task 5 的 notifyCompletion；本文件不自行 finalize、
//     不自行 Release、不重复通知；
//   - manager shutdown I/O（Stop / EmergencyStop）一律在 registry mutex 外执行；
//   - 请求停止与等待错误用 errors.Join 聚合，不因第一步失败跳过有界等待；
//   - Shutdown 失败返回非 nil error：调用方禁止继续关闭共享服务（Task 14 接线约定）。

// Stop 请求指定 probe 停止并有界等待 completion 提交（spec I6）。
//
// 成功仅在 goroutine 退出、输出 finalize、checkpoint 保留和 completion lease 释放
// 完成后返回；cleanup 失败（completion_failed）返回可诊断错误（重试入口在
// CloseProbe/Shutdown）。
func (r *ManagerRegistry) Stop(ctx context.Context, probeID ProbeID) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if !probeID.Valid() {
		return fmt.Errorf("%w: %q", ErrInvalidProbeID, probeID)
	}
	if err := r.acquireAdmission(ctx); err != nil {
		return err
	}
	r.mu.Lock()
	session := r.sessions[probeID]
	r.mu.Unlock()
	r.releaseAdmission()
	if session == nil {
		return fmt.Errorf("probe %s 无活动遍历任务", probeID)
	}
	// 锁外请求停止；即使失败仍执行有界等待（errors.Join 聚合）。
	stopErr := session.manager.Stop()
	waitErr := waitSettled(ctx, session)
	r.mu.Lock()
	state := session.state
	completionErr := session.completionErr
	r.mu.Unlock()
	if waitErr != nil {
		return errors.Join(stopErr, fmt.Errorf("等待 probe %s 停止超时: %w", probeID, waitErr))
	}
	if state == sessionStateCompletionFailed {
		return errors.Join(stopErr, fmt.Errorf("probe %s 停止后清理失败: %w", probeID, completionErr))
	}
	return stopErr
}

// CloseProbe 关闭指定 probe：请求停止 → 有界等待 completion 提交 →（必要时幂等重试
// 失败的 Release）→ 仅在 completion 已提交后删除终态 manager。
//
// ctx 超时返回 ErrCloseProbeTimeout：map 条目与 closing 标记保留（GetOrCreate 返回
// probe_closing，不创建新 manager）；幂等：无 manager 或保留期间重复调用均可重入。
func (r *ManagerRegistry) CloseProbe(ctx context.Context, probeID ProbeID) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if !probeID.Valid() {
		return fmt.Errorf("%w: %q", ErrInvalidProbeID, probeID)
	}
	if err := r.acquireAdmission(ctx); err != nil {
		return err
	}
	r.mu.Lock()
	if r.closing {
		r.mu.Unlock()
		r.releaseAdmission()
		return ErrRegistryClosing
	}
	manager, exists := r.managers[probeID]
	if !exists {
		r.mu.Unlock()
		r.releaseAdmission()
		return nil // 幂等：无 manager 视为已关闭
	}
	session := r.sessions[probeID]
	if session != nil && session.state == sessionStateActive {
		r.mu.Unlock()
		r.releaseAdmission()
		return fmt.Errorf("%w: probe %s 任务运行中，停止后才能关闭", ErrAlreadyRunning, probeID)
	}
	// closing 标记在 shutdown I/O 前置位；失败/超时路径保留（供重试并阻止 GetOrCreate）。
	r.closingProbes[probeID] = true
	r.mu.Unlock()
	r.releaseAdmission()

	// 锁外执行 manager shutdown I/O：并发 CloseProbe 双 probe 互不阻塞。
	if err := r.closeProbeSession(ctx, probeID, session, manager); err != nil {
		return err
	}
	// completion 已提交：删除终态 manager 并解除 closing 标记（此后可重建）。
	r.mu.Lock()
	delete(r.managers, probeID)
	delete(r.closingProbes, probeID)
	r.mu.Unlock()
	return nil
}

// closeProbeSession CloseProbe 的 session 清理段（锁外）；返回 nil 表示 completion 已提交。
func (r *ManagerRegistry) closeProbeSession(ctx context.Context, probeID ProbeID, session *registrySession, manager ManagedTraversalManager) error {
	var stopErr error
	if session == nil {
		return nil
	}
	r.mu.Lock()
	state := session.state
	r.mu.Unlock()
	if state == sessionStateActive {
		// completing/completion_failed 状态的 manager 已自行停止，不重复请求。
		stopErr = manager.Stop()
	}
	if err := waitSettled(ctx, session); err != nil {
		return errors.Join(stopErr, fmt.Errorf("%w: probe %s", ErrCloseProbeTimeout, probeID))
	}
	r.mu.Lock()
	state = session.state
	r.mu.Unlock()
	if state == sessionStateCompletionFailed {
		// 同 token 幂等重试失败的 Release；成功提交后才算完成。
		if err := r.retryCompletionCleanup(session); err != nil {
			return errors.Join(stopErr, fmt.Errorf("probe %s 清理重试失败: %w", probeID, err))
		}
	}
	return stopErr
}

// Shutdown 原子拒绝新任务并并行关闭全部 session（spec FR9 双 deadline）。
//
// 流程：closing=true → 每个 manager 并行 Stop + graceful deadline 内等待 →
// graceful 到期后对仍活动 controller 并发 EmergencyStop（各自从剩余 hard deadline
// 派生 context，单 adapter 卡住不延长总 deadline、不阻止其它）→ hard deadline 到期
// 返回包含 probe/task ID 的 ErrShutdownTimeout。超时任务保留 registry 条目与最后一个
// 有效 checkpoint（不删除，供诊断与重试）。
func (r *ManagerRegistry) Shutdown(ctx context.Context) error {
	started := time.Now()
	hardDeadline := started.Add(r.hardTimeout)
	r.markClosing()
	hardCtx, cancel := context.WithDeadline(ctx, hardDeadline)
	defer cancel()
	sessions, err := r.beginShutdown(hardCtx)
	if err != nil {
		unfinished := r.retainedSessionIDs()
		diagnostic := "等待 managed startup publication barrier"
		if len(unfinished) > 0 {
			diagnostic += ": " + strings.Join(unfinished, ", ")
		}
		if ctxErr := ctx.Err(); ctxErr != nil {
			return fmt.Errorf("%s: %w", diagnostic, ctxErr)
		}
		return fmt.Errorf("%w: %s: %v", ErrShutdownTimeout, diagnostic, err)
	}
	if len(sessions) == 0 {
		return nil // 幂等：已 closing 或无活动 session
	}
	r.gracefulStopSessions(sessions, started.Add(r.gracefulTimeout))
	if pending := r.pendingSessions(sessions); len(pending) > 0 {
		r.emergencyStopSessions(ctx, pending, hardDeadline)
		r.waitSessionsUntil(pending, hardDeadline)
	}
	if unfinished := r.unfinishedSessionIDs(sessions); len(unfinished) > 0 {
		return fmt.Errorf("%w: 未退出的遍历任务: %s", ErrShutdownTimeout, strings.Join(unfinished, ", "))
	}
	return nil
}

// beginShutdown 原子置 closing 并快照全部 retained session。重复调用仍重新快照，
// 使 completion_failed/超时 session 可继续重试。
func (r *ManagerRegistry) beginShutdown(ctx context.Context) ([]*registrySession, error) {
	if err := r.acquireAdmission(ctx); err != nil {
		return nil, err
	}
	defer r.releaseAdmission()
	r.mu.Lock()
	defer r.mu.Unlock()
	sessions := make([]*registrySession, 0, len(r.sessions))
	for _, session := range r.sessions {
		sessions = append(sessions, session)
	}
	return sessions, nil
}

func (r *ManagerRegistry) markClosing() {
	r.mu.Lock()
	r.closing = true
	r.mu.Unlock()
}

func (r *ManagerRegistry) retainedSessionIDs() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	ids := make([]string, 0, len(r.sessions))
	for _, session := range r.sessions {
		ids = append(ids, fmt.Sprintf("%s/%s", session.probeID, session.taskID))
	}
	sort.Strings(ids)
	return ids
}

func (r *ManagerRegistry) stopSessionAfterClosing(session *registrySession) {
	if err := session.manager.Stop(); err != nil {
		slog.Warn("registry closing 后请求停止新发布 session 失败", "probe", session.probeID, "task", session.taskID, "error", err)
	}
}

// waitSessionsUntil 在 EmergencyStop 返回后继续等待 completion，直到全部提交或共享
// hard deadline 到达。adapter 忽略 context 时 ES 自身的等待仍由同一 deadline 截断。
func (r *ManagerRegistry) waitSessionsUntil(sessions []*registrySession, deadline time.Time) {
	for _, session := range sessions {
		if r.sessionCompleted(session) {
			continue
		}
		waitCompletedUntil(session, deadline)
	}
}

func waitCompletedUntil(session *registrySession, deadline time.Time) bool {
	timer := time.NewTimer(time.Until(deadline))
	defer timer.Stop()
	select {
	case <-session.done:
		return true
	case <-timer.C:
		return false
	}
}

// gracefulStopSessions 并行请求停止全部 session，每个在统一 graceful deadline 前有界等待。
func (r *ManagerRegistry) gracefulStopSessions(sessions []*registrySession, deadline time.Time) {
	for _, session := range sessions {
		go r.gracefulStopSession(session, deadline)
	}
	for _, session := range sessions {
		waitSettledUntil(session, deadline)
	}
}

// gracefulStopSession 单 session 的 graceful 关闭（锁外 I/O）。
func (r *ManagerRegistry) gracefulStopSession(session *registrySession, deadline time.Time) {
	r.mu.Lock()
	state := session.state
	r.mu.Unlock()
	if state == sessionStateCompletionFailed {
		// manager 已退出，仅清理失败：同 token 幂等重试失败的 Release。
		if err := r.retryCompletionCleanup(session); err != nil {
			slog.Error("shutdown 重试 session 清理失败", "probe", session.probeID, "task", session.taskID, "error", err)
		}
		return
	}
	if state == sessionStateActive {
		if err := session.manager.Stop(); err != nil {
			slog.Warn("shutdown 请求停止失败", "probe", session.probeID, "task", session.taskID, "error", err)
		}
	}
}

// emergencyStopSessions 对仍未退出 session 的绑定控制器并发 EmergencyStop（spec FR9）。
//
// 每个控制器独立 goroutine + 独立从剩余 hard deadline 派生的 context；等待本身也被
// hard deadline 截断：单 adapter 卡住不延长总 deadline、不阻止尝试其它控制器，
// 也不等待越过 deadline 的 adapter 调用。
func (r *ManagerRegistry) emergencyStopSessions(ctx context.Context, sessions []*registrySession, hardDeadline time.Time) {
	actions := len(sessions)
	for _, session := range sessions {
		actions += len(session.boundControllerIDs)
	}
	done := make(chan struct{}, actions)
	for _, session := range sessions {
		for _, controllerID := range session.boundControllerIDs {
			go func(id string) {
				r.emergencyStopController(ctx, id, hardDeadline)
				done <- struct{}{}
			}(controllerID)
		}
		// 再次请求停止（FR9：ES 与再次 cancel 并行）。
		go func(s *registrySession) {
			if err := s.manager.Stop(); err != nil {
				slog.Warn("shutdown 再次请求停止失败", "probe", s.probeID, "task", s.taskID, "error", err)
			}
			done <- struct{}{}
		}(session)
	}
	timer := time.NewTimer(time.Until(hardDeadline))
	defer timer.Stop()
	for completed := 0; completed < actions; completed++ {
		select {
		case <-done:
		case <-timer.C:
			slog.Error("shutdown hard deadline 到期，EmergencyStop/Stop 未全部返回")
			return
		}
	}
}

// emergencyStopController 单控制器 ES；context 从剩余 hard deadline 派生（有界等待）。
func (r *ManagerRegistry) emergencyStopController(ctx context.Context, controllerID string, hardDeadline time.Time) {
	if r.motion == nil {
		slog.Error("shutdown 需要 EmergencyStop 但未注入 MotionAccess", "controller", controllerID)
		return
	}
	esCtx, cancel := context.WithTimeout(ctx, time.Until(hardDeadline))
	defer cancel()
	if err := r.motion.EmergencyStop(esCtx, controllerID); err != nil {
		slog.Error("shutdown EmergencyStop 失败", "controller", controllerID, "error", err)
	}
}

// pendingSessions 过滤尚未提交完成的 session。
func (r *ManagerRegistry) pendingSessions(sessions []*registrySession) []*registrySession {
	var pending []*registrySession
	for _, session := range sessions {
		if !r.sessionCompleted(session) {
			pending = append(pending, session)
		}
	}
	return pending
}

// unfinishedSessionIDs 返回未退出任务的 "probe/task" 诊断列表（字典序）。
func (r *ManagerRegistry) unfinishedSessionIDs(sessions []*registrySession) []string {
	var ids []string
	for _, session := range sessions {
		if !r.sessionCompleted(session) {
			ids = append(ids, fmt.Sprintf("%s/%s", session.probeID, session.taskID))
		}
	}
	sort.Strings(ids)
	return ids
}

// sessionCompleted 报告 session 是否已提交完成（r.mu 保护读）。
func (r *ManagerRegistry) sessionCompleted(session *registrySession) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return session.state == sessionStateCompleted
}

// waitSettled 等待 session cleanup 到达静态（completed 或 completion_failed），
// 或 ctx 到期（调用方界定有界等待）。
func waitSettled(ctx context.Context, session *registrySession) error {
	select {
	case <-session.settled:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// waitSettledUntil 在 deadline 前等待 session cleanup 到达静态；超时返回 false。
func waitSettledUntil(session *registrySession, deadline time.Time) bool {
	timer := time.NewTimer(time.Until(deadline))
	defer timer.Stop()
	select {
	case <-session.settled:
		return true
	case <-timer.C:
		return false
	}
}

// RunPoint registry 的 runPoint façade（Task 12：HTTP 生命周期操作只经 registry）。
func (r *ManagerRegistry) RunPoint(ctx context.Context, probeID ProbeID) error {
	return r.callProbeManager(ctx, probeID, true, func(m ManagedTraversalManager) error {
		return m.RunCurrentPoint()
	})
}

// Pause registry 的 pause façade。
func (r *ManagerRegistry) Pause(ctx context.Context, probeID ProbeID) error {
	return r.callProbeManager(ctx, probeID, false, func(m ManagedTraversalManager) error {
		return m.Pause()
	})
}

// Resume registry 的 resume（暂停恢复）façade。
func (r *ManagerRegistry) Resume(ctx context.Context, probeID ProbeID) error {
	return r.callProbeManager(ctx, probeID, false, func(m ManagedTraversalManager) error {
		return m.Resume()
	})
}

// callProbeManager 选择 probe 的 manager 并调用生命周期方法（registry 统一入口，
// HTTP handler 不得绕过）。manager 创建失败/未知 probe/closing 的错误原样传播。
func (r *ManagerRegistry) callProbeManager(ctx context.Context, probeID ProbeID, rejectManagedAutoRun bool, call func(ManagedTraversalManager) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if !probeID.Valid() {
		return fmt.Errorf("%w: %q", ErrInvalidProbeID, probeID)
	}
	if err := r.acquireAdmission(ctx); err != nil {
		return err
	}
	defer r.releaseAdmission()
	manager, err := r.GetOrCreate(probeID)
	if err != nil {
		return err
	}
	r.mu.Lock()
	_, active := r.sessions[probeID]
	r.mu.Unlock()
	if rejectManagedAutoRun && active {
		return fmt.Errorf("%w: probe %s managed traversal loop 已自动运行", ErrAlreadyRunning, probeID)
	}
	return call(manager)
}
