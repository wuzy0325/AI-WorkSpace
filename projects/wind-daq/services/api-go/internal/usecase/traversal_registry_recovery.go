package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"

	"wind-daq/services/api-go/internal/core/traversal"
	"wind-daq/services/api-go/internal/ports"
)

// registry probe-scoped 恢复 façade（Task 11）。
//
// 规格：docs/specs/dual-traversal-spec.md FR4/FR8；docs/specs/tasks-dual-traversal.md Task 11。
//
// 权威映射：dual recovery index（probeId → taskId → checkpointPath，Task 2 adapter）。
// 注册/注销时机（"映射存在 ⟺ checkpoint 文件存在"语义）：
//   - 注册：managed checkpoint 每次成功保存后（CheckpointSavedCallback，
//     commitPointV2 阶段3 / saveCheckpoint 触发），同 taskID 幂等更新；
//   - 注销：正常完成（completed）的 completion 提交后 Unregister；
//     stopped/error 终态保留映射（spec I6：可恢复 checkpoint 必须可发现）；
//   - 显式放弃：ClearCheckpoint 校验 taskID 后原子 Unregister 并删除文件；
//   - dual 路径不读写 legacy traversal-active-index.json（manager ownership 分支保证）。

// LoadCheckpoint 返回该 probe 的唯一可恢复 checkpoint（不扫描目录猜测）。
// registry 仍持有 session 时，索引中的 checkpoint 属于运行期容灾快照，不可恢复，
// 返回 nil 避免前端同时展示“正在运行”和“继续/放弃”。
// 无候选时返回 (nil, nil)；候选文件损坏或版本不符返回错误。
func (r *ManagerRegistry) LoadCheckpoint(ctx context.Context, probeID ProbeID) (*traversal.Checkpoint, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if !probeID.Valid() {
		return nil, fmt.Errorf("%w: %q", ErrInvalidProbeID, probeID)
	}
	r.mu.Lock()
	_, active := r.sessions[probeID]
	r.mu.Unlock()
	if active {
		return nil, nil
	}
	ref, found, err := r.recoveryIndex.Find(ctx, string(probeID))
	if err != nil {
		return nil, fmt.Errorf("查询双探针恢复索引失败: %w", err)
	}
	if !found {
		return nil, nil
	}
	cp, err := r.loadDualCheckpoint(ref.Path)
	if err != nil {
		return nil, err
	}
	if cp.ProbeID != string(probeID) {
		return nil, fmt.Errorf("%w: checkpoint 属于 %s，请求为 %s", ErrProbeIDMismatch, cp.ProbeID, probeID)
	}
	return cp, nil
}

// ResumeFromCheckpoint 恢复该 probe 的唯一可恢复任务。
//
// 顺序（spec FR8）：校验 taskID == index[probeID].taskID → 加载并校验 v3
// （ProbeID 一致）→ 与新 Start 完全相同的准入事务（admitLocked：workflow lease +
// controller lease + session token，全部在任何 append 文件/运动 I/O 之前）→
// managed Resume。准入或 Resume 失败按相反顺序回滚，checkpoint 与输出文件保持不变。
//
// admission 释放采用 panic-safe 模式（与 Start 一致，I-8）：completion.finish 内部
// 释放 admission，错误路径或 panic 由 defer 兜底，杜绝 gate 泄漏导致 registry 冻结。
func (r *ManagerRegistry) ResumeFromCheckpoint(ctx context.Context, probeID ProbeID, taskID string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if !probeID.Valid() {
		return "", fmt.Errorf("%w: %q", ErrInvalidProbeID, probeID)
	}
	if err := r.acquireAdmission(ctx); err != nil {
		return "", err
	}
	// admissionReleased 标记 admission 是否已通过 completion.finish 释放；
	// defer 仅在未释放时兜底（错误路径或 panic），避免 gate 永久泄漏。
	admissionReleased := false
	defer func() {
		if !admissionReleased {
			r.releaseAdmission()
		}
	}()
	cp, err := r.authoritativeCheckpoint(ctx, probeID, taskID)
	if err != nil {
		return "", err
	}
	// 准入前不得打开 append 文件或执行任何运动 I/O：先完成准入事务。
	admission, err := r.admitResumeLockedUnderGate(ctx, probeID, cp)
	if err != nil {
		return "", err
	}
	completion := &admissionCompletion{}
	opts := ManagedSessionOptions{
		ProbeID:                 probeID,
		ConfigKey:               probeConfigKey(probeID),
		Token:                   admission.session.token,
		TaskID:                  cp.TaskID,
		CompletionCallback:      func(token SessionToken) { completion.callback(r, token) },
		CheckpointSavedCallback: r.checkpointSavedCallbackFor(admission.session),
	}
	resumedTaskID, err := admission.session.manager.ResumeManaged(*cp, opts)
	if err != nil {
		rollbackErr := r.rollbackAdmissionUnderGate(admission)
		completion.finish(r, false)
		admissionReleased = true
		return "", errors.Join(fmt.Errorf("恢复遍历任务失败: %w", err), rollbackErr)
	}
	r.mu.Lock()
	closing := r.closing
	r.mu.Unlock()
	completion.finish(r, true)
	admissionReleased = true
	if closing {
		go r.stopSessionAfterClosing(admission.session)
		return "", ErrRegistryClosing
	}
	return resumedTaskID, nil
}

// ClearCheckpoint 显式放弃该 probe 的可恢复任务：校验 taskID 后原子注销映射并删除文件。
// 无候选时幂等返回 nil；taskID 不符返回 ErrTaskIDMismatch（映射与文件均不动）。
func (r *ManagerRegistry) ClearCheckpoint(ctx context.Context, probeID ProbeID, taskID string) error {
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
	r.mu.Lock()
	_, active := r.sessions[probeID]
	r.mu.Unlock()
	if active {
		return fmt.Errorf("%w: probe %s 任务运行中，不得清除恢复映射", ErrAlreadyRunning, probeID)
	}
	ref, found, err := r.recoveryIndex.Find(ctx, string(probeID))
	if err != nil {
		return fmt.Errorf("查询双探针恢复索引失败: %w", err)
	}
	if !found {
		return nil
	}
	if ref.TaskID != taskID {
		return fmt.Errorf("%w: probe %s 权威候选为 %s，请求为 %s", ErrTaskIDMismatch, probeID, ref.TaskID, taskID)
	}
	if err := r.recoveryIndex.Unregister(ctx, string(probeID), taskID); err != nil {
		return fmt.Errorf("注销双探针恢复映射失败: %w", err)
	}
	// 映射已注销（权威移除）；checkpoint 文件 best-effort 删除，残留仅浪费磁盘。
	if r.checkpointStore != nil {
		if err := r.checkpointStore.Remove(ref.Path); err != nil {
			slog.Warn("删除 dual checkpoint 文件失败（映射已注销）", "path", ref.Path, "error", err)
		}
	}
	return nil
}

// authoritativeCheckpoint 校验请求 taskID 并加载权威 v3 checkpoint。
func (r *ManagerRegistry) authoritativeCheckpoint(ctx context.Context, probeID ProbeID, taskID string) (*traversal.Checkpoint, error) {
	ref, found, err := r.recoveryIndex.Find(ctx, string(probeID))
	if err != nil {
		return nil, fmt.Errorf("查询双探针恢复索引失败: %w", err)
	}
	if !found {
		return nil, fmt.Errorf("probe %s 无可恢复任务", probeID)
	}
	if ref.TaskID != taskID {
		return nil, fmt.Errorf("%w: probe %s 权威候选为 %s，请求为 %s", ErrTaskIDMismatch, probeID, ref.TaskID, taskID)
	}
	cp, err := r.loadDualCheckpoint(ref.Path)
	if err != nil {
		return nil, err
	}
	if cp.ProbeID != string(probeID) {
		return nil, fmt.Errorf("%w: checkpoint 属于 %s，请求为 %s", ErrProbeIDMismatch, cp.ProbeID, probeID)
	}
	if cp.TaskID != ref.TaskID {
		return nil, fmt.Errorf("%w: checkpoint taskID 为 %s，索引为 %s", ErrTaskIDMismatch, cp.TaskID, ref.TaskID)
	}
	return cp, nil
}

// admitResume resume 的准入事务：与 Start 完全相同的临界区（绑定校验 +
// workflow/controller lease + session token 登记）。任何 lease/文件/运动 I/O
// 之前完成：冲突时 checkpoint 与输出文件保持不变。
//
// 资源独占粒度为 (controllerID, axis) 元组：从 checkpoint 快照的 Config.MotionAxes
// 提取 axis pairs（v3 快照中 BoundControllerIDs 仅用于 EmergencyStop 等控制器级操作，
// 不携带轴信息；Config 才持有完整 (controllerID, axis) 绑定）。
func (r *ManagerRegistry) admitResumeLockedUnderGate(ctx context.Context, probeID ProbeID, cp *traversal.Checkpoint) (*registryAdmission, error) {
	manager, err := r.GetOrCreate(probeID)
	if err != nil {
		return nil, err
	}
	// 恢复任务的轴绑定以 checkpoint 快照 Config 为准（v3 快照含完整 MotionAxes）。
	// 若 Config 无有效 axis pairs（旧 v3 快照可能只存了 BoundControllerIDs 而无轴信息），
	// boundControllerAxisPairs 返回 nil，由 validateDualBindings 拒绝启动。
	axisPairs := boundControllerAxisPairs(cp.Snapshot.Config)
	otherAxisPairs, err := r.loadProbeBindings(otherProbe(probeID))
	if err != nil {
		return nil, fmt.Errorf("读取另一路 probe 持久化配置失败: %w", err)
	}
	admission, err := r.admitLockedUnderGate(ctx, probeID, cp.TaskID, manager, axisPairs, otherAxisPairs)
	if err != nil {
		return nil, err
	}
	return admission, nil
}

// loadDualCheckpoint 读取并按 v3 校验 checkpoint 文件（不自动迁移 v1/v2）。
func (r *ManagerRegistry) loadDualCheckpoint(path string) (*traversal.Checkpoint, error) {
	if r.checkpointStore == nil {
		return nil, errors.New("checkpoint store 未注入，无法加载 dual checkpoint")
	}
	data, err := r.checkpointStore.Read(path)
	if err != nil {
		return nil, fmt.Errorf("读取 dual checkpoint 失败: %w", err)
	}
	var header struct {
		Version int `json:"version"`
	}
	if err := json.Unmarshal(data, &header); err != nil {
		return nil, fmt.Errorf("探测 dual checkpoint 版本失败: %w", err)
	}
	if header.Version != traversal.DualCheckpointVersion {
		return nil, fmt.Errorf("%w: dual 恢复路径仅接受 v3（got v%d）", ports.ErrCheckpointVersionMismatch, header.Version)
	}
	var cp traversal.Checkpoint
	if err := json.Unmarshal(data, &cp); err != nil {
		return nil, fmt.Errorf("解析 dual checkpoint 失败: %w", err)
	}
	return &cp, nil
}

// checkpointSavedCallbackFor 生成 session 的 CheckpointSavedCallback：
// managed checkpoint 每次成功保存后把 probeId→taskId→checkpointPath 幂等登记到
// dual recovery index（同 taskID 重复登记为更新，见 adapter Register 语义）。
func (r *ManagerRegistry) checkpointSavedCallbackFor(session *registrySession) func(string) {
	return func(checkpointPath string) {
		ctx := context.Background()
		r.mu.Lock()
		session.recoveryPath = checkpointPath
		r.mu.Unlock()
		if err := r.recoveryIndex.Register(ctx, string(session.probeID), session.taskID, checkpointPath); err != nil {
			r.mu.Lock()
			session.recoveryErr = fmt.Errorf("登记 dual recovery 映射失败: %w", err)
			r.mu.Unlock()
			slog.Error("登记 dual recovery 映射失败，请求停止 session", "probe", session.probeID, "task", session.taskID, "error", err)
			session.recoveryStopOnce.Do(func() {
				go func() {
					if stopErr := session.manager.Stop(); stopErr != nil {
						slog.Error("恢复映射失败后请求停止 session 失败", "probe", session.probeID, "task", session.taskID, "error", stopErr)
					}
				}()
			})
			return
		}
		r.mu.Lock()
		session.recoveryErr = nil
		r.mu.Unlock()
	}
}

// completeRecoveryMapping completion 提交后的恢复映射收尾（runSessionCleanup 调用）：
//   - 正常完成：原子注销映射（任务数据已权威落盘，无可恢复需求）；
//   - stopped/error：保留并确认映射（checkpoint 文件存在时可恢复，spec I6）；
//   - completion_failed 不调用本函数（session 保留，映射不动）。
//
// 不变量（I-11）："映射存在 ⟺ checkpoint 文件存在"。stopped/error 状态下若
// checkpoint 文件不存在（被外部删除或 Stat 失败），必须注销映射，避免 stale
// mapping 让 LoadCheckpoint 在下次启动时返回不存在的路径。
func (r *ManagerRegistry) completeRecoveryMapping(session *registrySession) error {
	ctx := context.Background()
	r.mu.Lock()
	recoveryPath := session.recoveryPath
	r.mu.Unlock()
	if recoveryPath != "" {
		if err := r.recoveryIndex.Register(ctx, string(session.probeID), session.taskID, recoveryPath); err != nil {
			return fmt.Errorf("登记 dual recovery 映射失败: %w", err)
		}
	}
	status := session.manager.Status()
	recoverable := status.State == traversal.StateStopped || status.State == traversal.StateError
	if !recoverable {
		if err := r.recoveryIndex.Unregister(ctx, string(session.probeID), session.taskID); err != nil {
			return fmt.Errorf("注销 dual recovery 映射失败: %w", err)
		}
		return nil
	}
	// stopped/error：确认映射（运行期回调已登记的为幂等更新；未登记且文件存在时补登）。
	if status.CSVPath == "" || r.checkpointStore == nil {
		return nil
	}
	checkpointPath := traversal.ResolveCheckpointPathFromCSV(status.CSVPath)
	exists, err := r.checkpointStore.Stat(checkpointPath)
	if err != nil {
		// Stat 失败：文件状态未知，按 stale 处理注销映射（保留映射会让 LoadCheckpoint
		// 指向不可读路径，下次启动恢复失败）。注销失败才向上传播。
		slog.Warn("检查 checkpoint 文件失败，注销可能 stale 的恢复映射",
			"probe", session.probeID, "task", session.taskID, "path", checkpointPath, "error", err)
		if unregErr := r.recoveryIndex.Unregister(ctx, string(session.probeID), session.taskID); unregErr != nil {
			return fmt.Errorf("注销 stale dual recovery 映射失败（Stat 错误: %v）: %w", err, unregErr)
		}
		return nil
	}
	if !exists {
		// checkpoint 文件不存在：映射必须注销以维持"映射存在 ⟺ 文件存在"不变量。
		slog.Warn("checkpoint 文件不存在，注销 stale 恢复映射",
			"probe", session.probeID, "task", session.taskID, "path", checkpointPath)
		if err := r.recoveryIndex.Unregister(ctx, string(session.probeID), session.taskID); err != nil {
			return fmt.Errorf("注销 stale dual recovery 映射失败: %w", err)
		}
		return nil
	}
	if err := r.recoveryIndex.Register(ctx, string(session.probeID), session.taskID, checkpointPath); err != nil {
		return fmt.Errorf("确认 dual recovery 映射失败: %w", err)
	}
	return nil
}
