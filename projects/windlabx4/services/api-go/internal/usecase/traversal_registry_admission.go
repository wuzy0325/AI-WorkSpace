package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"slices"
	"sync"

	"windlabx4/services/api-go/internal/core/traversal"
)

// registry 准入事务（Task 4：Start façade + 原子准入 + 回滚）。
//
// 规格：docs/specs/dual-traversal-spec.md I1/I2/I5、FR4；
// docs/specs/tasks-dual-traversal.md Task 4。
//
// 事务顺序（任一步失败按相反顺序回滚）：
//  1. activeCount==0 时获取全局 workflow:traversal lease（固定 holder）；
//  2. 经 ports.ControllerLeasePort.Acquire 原子预占启动快照中的控制器；
//  3. 生成 SessionToken（probeID + 递增 generation）并登记 session、activeCount++；
//  4. 持 admission gate 调用 manager 的 managed Start，返回后才发布给生命周期操作。
//
// 回滚直接撤销临时资源，不走 notifyCompletion（spec I2：防止"未计入却减一"）。

// registryAdmission 一次已提交的准入（步骤 1-3 的产物），供 managed Start 失败时回滚。
type registryAdmission struct {
	session          *registrySession
	releaseCtx       context.Context // 回滚释放用：不受调用方取消影响
	acquiredWorkflow bool
}

type admissionCompletion struct {
	mu        sync.Mutex
	pending   bool
	token     SessionToken
	published bool
	discarded bool
}

func (c *admissionCompletion) callback(r *ManagerRegistry, token SessionToken) {
	c.mu.Lock()
	if c.discarded {
		c.mu.Unlock()
		return
	}
	if !c.published {
		c.pending = true
		c.token = token
		c.mu.Unlock()
		return
	}
	c.mu.Unlock()
	r.notifyCompletion(token)
}

func (c *admissionCompletion) finish(r *ManagerRegistry, publish bool) {
	c.mu.Lock()
	if publish {
		c.published = true
	} else {
		c.discarded = true
	}
	pending := publish && c.pending
	token := c.token
	r.releaseAdmission()
	c.mu.Unlock()
	if pending {
		r.notifyCompletion(token)
	}
}

// Start registry 的 probe-scoped 启动 façade（HTTP 必须经此，不得直接调 manager Start）。
//
// 顺序：准入前检查与准备（锁外）→ per-probe gate + admission gate 双重获取 →
// 原子准入与 managed Start。
//
// gate 获取顺序（I-7）：probeGate 先于 admission。probeGate 串行化同 probe 的
// lifecycle 操作（publication barrier + generation stability），admission 串行化
// 全局 workflow lease 交接。两 gate 独立，release 顺序与 acquire 相反。
//
// admission 释放采用 panic-safe 模式：completion.finish 内部会调用 r.releaseAdmission()，
// 但错误路径或 panic 发生在 finish 之前时由 defer 兜底，杜绝 admission gate 泄漏。
// probeGate 始终由 defer 释放（不与 admission 复用释放路径），简化生命周期管理。
func (r *ManagerRegistry) Start(ctx context.Context, probeID ProbeID, rawConfig json.RawMessage) (string, error) {
	prep, err := r.prepareStart(ctx, probeID, rawConfig)
	if err != nil {
		return "", err
	}
	r.mu.Lock()
	transitioning := r.workflowTransition
	r.mu.Unlock()
	if transitioning {
		return "", ErrRegistryTransitioning
	}
	// probeGate 先于 admission：保证同 probe 的 Start/Stop/Pause/Resume/RunPoint 串行化，
	// 且不阻塞其他 probe 的并发操作（I-7）。
	if err := r.acquireProbeGate(ctx, probeID); err != nil {
		return "", err
	}
	defer r.releaseProbeGate(probeID)
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
	admission, err := r.admitLockedUnderGate(ctx, probeID, prep.taskID, prep.manager, prep.startAxisPairs, prep.otherAxisPairs)
	if err != nil {
		return "", err
	}
	completion := &admissionCompletion{}
	opts := ManagedSessionOptions{
		ProbeID:                 probeID,
		ConfigKey:               probeConfigKey(probeID),
		Token:                   admission.session.token,
		TaskID:                  prep.taskID,
		CompletionCallback:      func(token SessionToken) { completion.callback(r, token) },
		CheckpointSavedCallback: r.checkpointSavedCallbackFor(admission.session),
	}
	if err := prep.manager.StartManaged(prep.config, opts); err != nil {
		rollbackErr := r.rollbackAdmissionUnderGate(admission)
		completion.finish(r, false)
		admissionReleased = true
		return "", errors.Join(fmt.Errorf("启动遍历任务失败: %w", err), rollbackErr)
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
	return prep.taskID, nil
}

// startPreparation Start 准入前的锁外准备产物。
type startPreparation struct {
	manager        ManagedTraversalManager
	config         traversal.Config
	taskID         string
	startAxisPairs []traversal.MotionAxisBinding
	otherAxisPairs []traversal.MotionAxisBinding
}

// prepareStart 创建或加载 manager、解析配置、生成权威 task ID，并准备绑定校验输入。
// 已停止的任务不会阻止新任务启动。
func (r *ManagerRegistry) prepareStart(ctx context.Context, probeID ProbeID, rawConfig json.RawMessage) (*startPreparation, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if !probeID.Valid() {
		return nil, fmt.Errorf("%w: %q", ErrInvalidProbeID, probeID)
	}
	manager, err := r.GetOrCreate(probeID)
	if err != nil {
		return nil, err
	}
	cfg, err := manager.ParseConfig(rawConfig)
	if err != nil {
		return nil, fmt.Errorf("解析遍历配置失败: %w", err)
	}
	// 服务端权威 dual task ID：忽略并覆盖客户端提交值，客户端 ID 不得成为
	// 结果/index/checkpoint 的权威键（spec I5）。
	taskID, err := r.taskIDs.NewTaskID(ctx, string(probeID))
	if err != nil {
		return nil, fmt.Errorf("生成服务端任务 ID 失败: %w", err)
	}
	cfg.TaskID = taskID
	// 持久化配置读取为文件 I/O，不得持有 registry mutex。启动 probe 的绑定以实际
	// 运行的 rawConfig 为准；rawConfig 未绑定轴时回退到其持久化配置（前端先保存
	// 后启动的常规流程二者一致）。
	//
	// 资源独占粒度为 (controllerID, axis) 元组：同一控制器的不同物理轴可被两个 probe
	// 分别 lease（风洞实验中两探针共用同一运动控制器的不同轴是常见配置）。
	//
	// I/O 错误必须向上传播：吞错会让用户看到 resource_conflict 而非真实的存储故障，
	// 误导排查方向（I-6）。
	startAxisPairs := boundControllerAxisPairs(cfg)
	if len(startAxisPairs) == 0 {
		persisted, err := r.loadProbeBindings(probeID)
		if err != nil {
			return nil, fmt.Errorf("读取 probe %s 持久化配置失败: %w", probeID, err)
		}
		startAxisPairs = persisted
	}
	otherAxisPairs, err := r.loadProbeBindings(otherProbe(probeID))
	if err != nil {
		return nil, fmt.Errorf("读取另一路 probe 持久化配置失败: %w", err)
	}
	return &startPreparation{
		manager:        manager,
		config:         cfg,
		taskID:         taskID,
		startAxisPairs: startAxisPairs,
		otherAxisPairs: otherAxisPairs,
	}, nil
}

// admitLockedUnderGate executes admission while the caller holds admissionGate.
func (r *ManagerRegistry) admitLockedUnderGate(
	ctx context.Context,
	probeID ProbeID,
	taskID string,
	manager ManagedTraversalManager,
	startAxisPairs, otherPersisted []traversal.MotionAxisBinding,
) (*registryAdmission, error) {
	acquireWorkflow, err := r.checkAdmissionState(probeID, startAxisPairs, otherPersisted)
	if err != nil {
		return nil, err
	}
	admission := &registryAdmission{releaseCtx: context.WithoutCancel(ctx)}
	if acquireWorkflow {
		if err := r.workflowLease.Acquire(ctx, registryWorkflowHolder, registryLeaseTTL); err != nil {
			return nil, conflictOrCtx(ctx, err, "获取全局 workflow lease 失败")
		}
		admission.acquiredWorkflow = true
	}
	tokens, acquireErr, rollbackErr := r.acquireControllers(ctx, taskID, startAxisPairs)
	if acquireErr != nil {
		retainWorkflow := admission.acquiredWorkflow && len(tokens) > 0
		if admission.acquiredWorkflow && !retainWorkflow {
			if releaseErr := r.workflowLease.Release(admission.releaseCtx, registryWorkflowHolder); releaseErr != nil {
				retainWorkflow = true
				rollbackErr = errors.Join(rollbackErr, fmt.Errorf("释放全局 workflow lease: %w", releaseErr))
			}
		}
		if retainWorkflow || len(tokens) > 0 {
			admission.acquiredWorkflow = retainWorkflow
			r.mu.Lock()
			r.registerSessionLocked(probeID, taskID, manager, startAxisPairs, tokens, admission)
			admission.session.rollback = true
			admission.session.state = sessionStateCompletionFailed
			admission.session.completionErr = rollbackErr
			admission.session.settledOnce.Do(func() { close(admission.session.settled) })
			r.mu.Unlock()
		}
		return nil, errors.Join(acquireErr, rollbackErr)
	}
	r.mu.Lock()
	r.registerSessionLocked(probeID, taskID, manager, startAxisPairs, tokens, admission)
	r.mu.Unlock()
	return admission, nil
}

func (r *ManagerRegistry) checkAdmissionState(probeID ProbeID, startAxisPairs, otherPersisted []traversal.MotionAxisBinding) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closing {
		return false, ErrRegistryClosing
	}
	if _, active := r.sessions[probeID]; active {
		return false, fmt.Errorf("%w: probe %s", ErrAlreadyRunning, probeID)
	}
	if r.workflowTransition {
		// 全局 lease 交接中：不得按旧 activeCount 跳过 workflow Acquire，
		// 返回稳定 registry_transitioning（调用方可稍后重试）。
		return false, ErrRegistryTransitioning
	}
	// 另一路已运行时以其启动快照绑定为准（冻结值），否则用其持久化配置。
	otherBindings := otherPersisted
	if other, active := r.sessions[otherProbe(probeID)]; active {
		otherBindings = other.boundAxisPairs
	}
	if err := validateDualBindings(startAxisPairs, otherBindings); err != nil {
		return false, err
	}
	return r.activeCount == 0, nil
}

// registerSessionLocked 登记 session、递增计数并启动续约器（调用方持有 r.mu）。
//
// boundControllerIDs（控制器级，去重）用于 EmergencyStop 等控制器级操作；
// boundAxisPairs（(controllerID, axis) 元组）为资源独占的真实粒度，作为 lease token
// map 的 key 源。两者都从 startAxisPairs 派生，保持单一真相源。
func (r *ManagerRegistry) registerSessionLocked(
	probeID ProbeID,
	taskID string,
	manager ManagedTraversalManager,
	startAxisPairs []traversal.MotionAxisBinding,
	tokens map[ControllerAxisPair]string,
	admission *registryAdmission,
) {
	r.generations[probeID]++
	admission.session = &registrySession{
		probeID:            probeID,
		taskID:             taskID,
		token:              SessionToken{ProbeID: probeID, Generation: r.generations[probeID]},
		manager:            manager,
		boundControllerIDs: boundControllerIDsFromPairs(startAxisPairs),
		boundAxisPairs:     slices.Clone(startAxisPairs),
		controllerTokens:   tokens,
		workflowAcquired:   admission.acquiredWorkflow,
		state:              sessionStateActive,
		done:               make(chan struct{}),
		settled:            make(chan struct{}),
		pendingReleases:    maps.Clone(tokens),
	}
	r.sessions[probeID] = admission.session
	r.activeCount++
	// 第一路准入同时启动全局 workflow lease 续约器（固定 holder 周期续约）。
	if admission.acquiredWorkflow {
		r.startWorkflowRenewerLocked()
	}
	r.startSessionRenewerLocked(admission.session)
}

// acquireControllers 依次预占启动快照中的控制器轴；任一失败按相反顺序撤销已取得的预占。
//
// 资源独占粒度为 (controllerID, axis) 元组：同一控制器的不同物理轴可被两个 probe
// 分别 lease；只有同一物理轴才视为冲突资源。
func (r *ManagerRegistry) acquireControllers(ctx context.Context, taskID string, bindings []traversal.MotionAxisBinding) (map[ControllerAxisPair]string, error, error) {
	tokens := make(map[ControllerAxisPair]string, len(bindings))
	for _, binding := range bindings {
		pair := ControllerAxisPair{ControllerID: binding.ControllerID, Axis: binding.Axis}
		token, err := r.controllerLease.Acquire(ctx, binding.ControllerID, binding.Axis, taskID, registryLeaseTTL)
		if err != nil {
			releaseCtx := context.WithoutCancel(ctx)
			var rollbackErrs []error
			for _, acquired := range slices.Backward(bindings) {
				acqPair := ControllerAxisPair{ControllerID: acquired.ControllerID, Axis: acquired.Axis}
				if leaseToken, ok := tokens[acqPair]; ok {
					if releaseErr := r.controllerLease.Release(releaseCtx, leaseToken); releaseErr != nil {
						rollbackErrs = append(rollbackErrs, fmt.Errorf("释放控制器 %s 轴 %s lease: %w", acquired.ControllerID, acquired.Axis, releaseErr))
					} else {
						delete(tokens, acqPair)
					}
				}
			}
			return tokens,
				conflictOrCtx(ctx, err, fmt.Sprintf("预占控制器 %s 轴 %s 失败", binding.ControllerID, binding.Axis)),
				errors.Join(rollbackErrs...)
		}
		tokens[pair] = token
	}
	return tokens, nil, nil
}

// rollbackAdmissionUnderGate managed Start 失败的准入回滚。调用方持有 admissionGate；
// 资源全部释放成功前保留 session、
// activeCount 与 ownership；失败进入 completion_failed，供 CloseProbe/Shutdown 重试。
func (r *ManagerRegistry) rollbackAdmissionUnderGate(admission *registryAdmission) error {
	session := admission.session
	r.mu.Lock()
	session.rollback = true
	session.state = sessionStateCompleting
	r.mu.Unlock()
	stopRenewer(session.renewCancel, session.renewDone)
	if err := r.cleanupCompletedSessionUnderGate(session); err != nil {
		return fmt.Errorf("回滚准入资源失败: %w", err)
	}
	return nil
}

// validateDualBindings 双模式启动前原子校验：两路 (controllerID, axis) 元组均非空且互不重叠（spec I1）。
//
// 资源独占粒度为 (controllerID, axis) 元组：同一控制器的不同物理轴可被两个 probe 分别
// 绑定（风洞实验常见配置）；仅当两路绑定到同一控制器的同一物理轴时才视为冲突。
func validateDualBindings(startAxisPairs, otherAxisPairs []traversal.MotionAxisBinding) error {
	if len(startAxisPairs) == 0 || len(otherAxisPairs) == 0 {
		return fmt.Errorf("%w: 双模式要求两路均配置非空运动控制器轴", ErrResourceConflict)
	}
	otherSet := make(map[ControllerAxisPair]bool, len(otherAxisPairs))
	for _, b := range otherAxisPairs {
		otherSet[ControllerAxisPair{ControllerID: b.ControllerID, Axis: b.Axis}] = true
	}
	for _, b := range startAxisPairs {
		pair := ControllerAxisPair{ControllerID: b.ControllerID, Axis: b.Axis}
		if otherSet[pair] {
			return fmt.Errorf("%w: 控制器 %s 轴 %s 已被另一路绑定", ErrResourceConflict, b.ControllerID, b.Axis)
		}
	}
	return nil
}

// loadProbeBindings 从 probe-scoped 持久化配置读取控制器轴绑定（(controllerID, axis) 元组）。
//
// 语义（I-6 修复）：
//   - 未配置（key 不存在）：返回 (nil, nil)，调用方按"未配置"处理（validateDualBindings 拒绝启动）；
//   - 真实 I/O 错误（LoadConfig 返回非 nil error）：返回 (nil, err) 向上传播，让用户看到
//     真实存储故障而非 resource_conflict 误导；
//   - 顶层 JSON 损坏（无法解析为 envelope）：返回 (nil, err) 向上传播（真实配置损坏）；
//   - channels 子字段解析失败：lenient 返回 nil（channels 可能是对象 {motionAxes:[...]}、
//     数组 [...] 或其他历史形态，子字段形态不符不是真实错误，跳过即可）。
//
// AppConfigStore.LoadConfig 的契约：key 不存在时返回 (nil, nil)，与 os.IsNotExist 等价；
// 其他错误（权限/磁盘等）包装后返回。本函数严格遵循该契约区分"未配置"与"I/O 错误"。
func (r *ManagerRegistry) loadProbeBindings(probeID ProbeID) ([]traversal.MotionAxisBinding, error) {
	key := probeConfigKey(probeID)
	data, err := r.configStore.LoadConfig(key)
	if err != nil {
		// 真实 I/O 错误必须向上传播（I-6）：吞错会让用户看到 resource_conflict 而非存储故障。
		return nil, fmt.Errorf("读取 probe %s 持久化配置 key=%s 失败: %w", probeID, key, err)
	}
	if len(data) == 0 {
		// key 不存在（未配置）：合法状态，返回 nil 让 validateDualBindings 处理。
		return nil, nil
	}
	var envelope struct {
		MotionAxes []traversal.MotionAxisBinding `json:"motionAxes"`
		Channels   json.RawMessage               `json:"channels"`
	}
	if err := json.Unmarshal(data, &envelope); err != nil {
		// 顶层 JSON 损坏属真实配置错误，向上传播。
		return nil, fmt.Errorf("解析 probe %s 持久化配置 key=%s 失败: %w", probeID, key, err)
	}
	var channels struct {
		MotionAxes []traversal.MotionAxisBinding `json:"motionAxes"`
	}
	if len(envelope.Channels) > 0 {
		// channels 子字段可能是对象 {motionAxes:[...]} 或历史数组形态；
		// 解析失败时 lenient 跳过（仅用 envelope.MotionAxes），不视为真实错误。
		if err := json.Unmarshal(envelope.Channels, &channels); err != nil {
			slog.Debug("probe 持久化配置 channels 子字段非 {motionAxes:[...]} 形态，跳过",
				"probe", string(probeID), "key", key, "error", err)
		}
	}
	axes := append(envelope.MotionAxes, channels.MotionAxes...)
	return boundControllerAxisPairs(traversal.Config{MotionAxes: axes}), nil
}

// conflictOrCtx 区分上下文取消与资源冲突：取消按原样返回，其余包装为 resource_conflict。
func conflictOrCtx(ctx context.Context, cause error, message string) error {
	if ctxErr := ctx.Err(); ctxErr != nil {
		return ctxErr
	}
	return fmt.Errorf("%w: %s: %v", ErrResourceConflict, message, cause)
}
