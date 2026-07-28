package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"slices"
	"sync"

	"wind-daq/services/api-go/internal/core/traversal"
	"wind-daq/services/api-go/internal/ports"
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
// 顺序：准入前检查与准备（锁外）→ 原子准入与 managed Start（同一 admission gate）。
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
	if err := r.acquireAdmission(ctx); err != nil {
		return "", err
	}
	admission, err := r.admitLockedUnderGate(ctx, probeID, prep.taskID, prep.manager, prep.startBindings, prep.otherBindings)
	if err != nil {
		r.releaseAdmission()
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
		return "", errors.Join(fmt.Errorf("启动遍历任务失败: %w", err), rollbackErr)
	}
	r.mu.Lock()
	closing := r.closing
	r.mu.Unlock()
	completion.finish(r, true)
	if closing {
		go r.stopSessionAfterClosing(admission.session)
		return "", ErrRegistryClosing
	}
	return prep.taskID, nil
}

// startPreparation Start 准入前的锁外准备产物。
type startPreparation struct {
	manager       ManagedTraversalManager
	config        traversal.Config
	taskID        string
	startBindings []string
	otherBindings []string
}

// prepareStart 可恢复候选检查（任何输出文件/运动 I/O 之前）→ GetOrCreate → 解析配置 →
// 服务端生成权威 task ID（覆盖客户端 config.TaskID）→ 锁外准备绑定校验输入。
func (r *ManagerRegistry) prepareStart(ctx context.Context, probeID ProbeID, rawConfig json.RawMessage) (*startPreparation, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if !probeID.Valid() {
		return nil, fmt.Errorf("%w: %q", ErrInvalidProbeID, probeID)
	}
	// 可恢复候选拒绝：发生在 factory 创建、配置解析和任何文件/运动 I/O 之前（spec FR4）。
	ref, found, err := r.recoveryIndex.Find(ctx, string(probeID))
	if err != nil {
		return nil, fmt.Errorf("查询双探针恢复索引失败: %w", err)
	}
	if found {
		return nil, fmt.Errorf("%w: probe %s 存在可恢复任务 %s", ports.ErrRecoverableTaskExists, probeID, ref.TaskID)
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
	startBindings := boundControllerIDs(cfg)
	if len(startBindings) == 0 {
		startBindings = r.loadProbeBindings(probeID)
	}
	return &startPreparation{
		manager:       manager,
		config:        cfg,
		taskID:        taskID,
		startBindings: startBindings,
		otherBindings: r.loadProbeBindings(otherProbe(probeID)),
	}, nil
}

// admit 通过 admission gate 串行化跨 probe 状态与 lease 交接。外部 lease I/O
// 在 r.mu 外执行，最终登记前重新校验 closing 状态。
func (r *ManagerRegistry) admit(
	ctx context.Context,
	probeID ProbeID,
	taskID string,
	manager ManagedTraversalManager,
	startBindings, otherPersisted []string,
) (*registryAdmission, error) {
	r.mu.Lock()
	transitioning := r.workflowTransition
	r.mu.Unlock()
	if transitioning {
		return nil, ErrRegistryTransitioning
	}
	if err := r.acquireAdmission(ctx); err != nil {
		return nil, err
	}
	defer r.releaseAdmission()
	return r.admitLockedUnderGate(ctx, probeID, taskID, manager, startBindings, otherPersisted)
}

// admitLockedUnderGate executes admission while the caller holds admissionGate.
func (r *ManagerRegistry) admitLockedUnderGate(
	ctx context.Context,
	probeID ProbeID,
	taskID string,
	manager ManagedTraversalManager,
	startBindings, otherPersisted []string,
) (*registryAdmission, error) {
	acquireWorkflow, err := r.checkAdmissionState(probeID, startBindings, otherPersisted)
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
	tokens, acquireErr, rollbackErr := r.acquireControllers(ctx, taskID, startBindings)
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
			r.registerSessionLocked(probeID, taskID, manager, startBindings, tokens, admission)
			admission.session.rollback = true
			admission.session.state = sessionStateCompletionFailed
			admission.session.completionErr = rollbackErr
			admission.session.settledOnce.Do(func() { close(admission.session.settled) })
			r.mu.Unlock()
		}
		return nil, errors.Join(acquireErr, rollbackErr)
	}
	r.mu.Lock()
	r.registerSessionLocked(probeID, taskID, manager, startBindings, tokens, admission)
	r.mu.Unlock()
	return admission, nil
}

func (r *ManagerRegistry) checkAdmissionState(probeID ProbeID, startBindings, otherPersisted []string) (bool, error) {
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
		otherBindings = other.boundControllerIDs
	}
	if err := validateDualBindings(startBindings, otherBindings); err != nil {
		return false, err
	}
	return r.activeCount == 0, nil
}

// registerSessionLocked 登记 session、递增计数并启动续约器（调用方持有 r.mu）。
func (r *ManagerRegistry) registerSessionLocked(
	probeID ProbeID,
	taskID string,
	manager ManagedTraversalManager,
	startBindings []string,
	tokens map[string]string,
	admission *registryAdmission,
) {
	r.generations[probeID]++
	admission.session = &registrySession{
		probeID:            probeID,
		taskID:             taskID,
		token:              SessionToken{ProbeID: probeID, Generation: r.generations[probeID]},
		manager:            manager,
		boundControllerIDs: slices.Clone(startBindings),
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

// acquireControllers 依次预占启动快照中的控制器；任一失败按相反顺序撤销已取得的预占。
func (r *ManagerRegistry) acquireControllers(ctx context.Context, taskID string, bindings []string) (map[string]string, error, error) {
	tokens := make(map[string]string, len(bindings))
	for _, controllerID := range bindings {
		token, err := r.controllerLease.Acquire(ctx, controllerID, taskID, registryLeaseTTL)
		if err != nil {
			releaseCtx := context.WithoutCancel(ctx)
			var rollbackErrs []error
			for _, acquired := range slices.Backward(bindings) {
				if leaseToken, ok := tokens[acquired]; ok {
					if releaseErr := r.controllerLease.Release(releaseCtx, leaseToken); releaseErr != nil {
						rollbackErrs = append(rollbackErrs, fmt.Errorf("释放控制器 %s lease: %w", acquired, releaseErr))
					} else {
						delete(tokens, acquired)
					}
				}
			}
			return tokens,
				conflictOrCtx(ctx, err, fmt.Sprintf("预占控制器 %s 失败", controllerID)),
				errors.Join(rollbackErrs...)
		}
		tokens[controllerID] = token
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

// validateDualBindings 双模式启动前原子校验：两路控制器绑定均非空且互不重叠（spec I1）。
func validateDualBindings(startBindings, otherBindings []string) error {
	if len(startBindings) == 0 || len(otherBindings) == 0 {
		return fmt.Errorf("%w: 双模式要求两路均配置非空运动控制器", ErrResourceConflict)
	}
	for _, id := range startBindings {
		if slices.Contains(otherBindings, id) {
			return fmt.Errorf("%w: 控制器 %s 已被另一路绑定", ErrResourceConflict, id)
		}
	}
	return nil
}

// loadProbeBindings 从 probe-scoped 持久化配置读取控制器绑定。
// 未配置或配置损坏时返回 nil（调用方按"未配置"处理，拒绝启动）。
func (r *ManagerRegistry) loadProbeBindings(probeID ProbeID) []string {
	data, err := r.configStore.LoadConfig(probeConfigKey(probeID))
	if err != nil || len(data) == 0 {
		return nil
	}
	var cfg traversal.Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil
	}
	return boundControllerIDs(cfg)
}

// conflictOrCtx 区分上下文取消与资源冲突：取消按原样返回，其余包装为 resource_conflict。
func conflictOrCtx(ctx context.Context, cause error, message string) error {
	if ctxErr := ctx.Err(); ctxErr != nil {
		return ctxErr
	}
	return fmt.Errorf("%w: %s: %v", ErrResourceConflict, message, cause)
}
