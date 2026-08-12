package usecase

import (
	"errors"
	"fmt"

	"wind-daq/services/api-go/internal/core/traversal"
	"wind-daq/services/api-go/internal/ports"
)

// TraversalManager 的 registry-managed 入口（Task 8）。
//
// 规格：docs/specs/dual-traversal-spec.md I2/I5、FR3；
// docs/specs/tasks-dual-traversal.md Task 8。
//
// ownership 机制：
//   - legacy（Start）：manager 自行 Acquire/Release 全局
//     workflow lease、读写 legacy traversal-active-index.json——行为与改造前完全一致；
//   - managed（StartManaged）：lease 由 registry 准入事务持有，
//     manager 不触碰 workflow lease 与 legacy activeIndex；ManagedSessionOptions
//     在启动时一次性注入并冻结为 session 快照字段（session.managedOpts）；
//   - ownership 是 session 快照的一部分，同一 session 不得混用两种模式
//    （beginSession 的"单活动 session"门禁天然阻止跨模式重入）。

// 编译期接口断言：*TraversalManager 必须完整实现 ManagedTraversalManager，
// 方法签名漂移在编译期暴露。
var _ ManagedTraversalManager = (*TraversalManager)(nil)

// StartManaged registry-only 的 managed 启动入口。
//
// 与 legacy Start 共享 startInternal 全部任务执行逻辑，差异仅在 ownership：
//   - 不 Acquire 全局 workflow lease、不登记 legacy activeIndex；
//   - ManagedSessionOptions 冻结进 session 快照；
//   - 成功后自行启动 RunTraversalLoop（legacy 由 ParseAndStartTraversal 启动）；
//   - 完成时（goroutine 退出 + 输出 finalize 之后）恰好一次回调
//     opts.CompletionCallback(opts.Token)；
//   - 返回错误（准入回滚路径）不触发回调。
func (m *TraversalManager) StartManaged(config traversal.Config, opts ManagedSessionOptions) error {
	if err := m.validateManagedOpts(opts); err != nil {
		return err
	}
	if opts.TaskID != "" && opts.TaskID != config.TaskID {
		return fmt.Errorf("managed options taskID %q 与 config taskID %q 不一致", opts.TaskID, config.TaskID)
	}
	m.mu.RLock()
	acqController := m.acquisitionController
	m.mu.RUnlock()
	if dev, abnormal := firstAbnormalAcquisitionDevice(acqController, config); abnormal {
		if dev.state == ports.AcquisitionReconnectRequired {
			return fmt.Errorf("device %s is not connected; reconnect it and start acquisition before traversal", dev.name)
		}
		return fmt.Errorf("device %s is not acquiring; start acquisition before traversal", dev.name)
	}
	if err := m.startInternal(config, &opts); err != nil {
		return err
	}
	go m.RunTraversalLoop()
	return nil
}

// validateManagedOpts 校验 managed 启动选项的完整性（装配一致性防御）。
func (m *TraversalManager) validateManagedOpts(opts ManagedSessionOptions) error {
	if !opts.ProbeID.Valid() {
		return fmt.Errorf("%w: %q", ErrInvalidProbeID, opts.ProbeID)
	}
	if opts.Token.ProbeID != opts.ProbeID {
		return errors.New("session token 与 options probeID 不一致")
	}
	if opts.CompletionCallback == nil {
		return errors.New("managed session 必须注入 CompletionCallback")
	}
	if opts.ConfigKey != "" && opts.ConfigKey != m.currentConfigKey() {
		return fmt.Errorf("managed options configKey %q 与 manager 配置键 %q 不一致", opts.ConfigKey, m.currentConfigKey())
	}
	return nil
}

// Done 返回当前 session 的完成信号通道；无活动 session 时返回已关闭通道。
func (m *TraversalManager) Done() <-chan struct{} {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.session == nil {
		return closedChan()
	}
	return m.session.Done()
}

// notifyManagedCompletion managed 会话的唯一完成回报点。
//
// 由 RunTraversalLoop 的 defer 链在 finalizeSink 之后调用（normal/error/Stop/
// EmergencyStop 各退出路径统一收敛）；legacy 会话（managedOpts == nil）为 no-op。
// registry 侧的 generation 校验与 exactly-once 减计数保证重复/旧通知安全。
func (m *TraversalManager) notifyManagedCompletion(session *TraversalRunSession) {
	if session == nil || session.managedOpts == nil {
		return
	}
	opts := session.managedOpts
	if opts.CompletionCallback == nil {
		return
	}
	opts.CompletionCallback(opts.Token)
}

// notifyManagedCheckpointSaved managed checkpoint 成功落盘后的通知点（Task 11）。
//
// 在 commitPointV2 阶段3 与 saveCheckpoint 成功写入后调用：registry 据此把
// probeId→taskId→checkpointPath 登记到 dual recovery index，保证
// "映射存在 ⟺ checkpoint 文件存在"。legacy 会话为 no-op。
func notifyManagedCheckpointSaved(session *TraversalRunSession, checkpointPath string) {
	if session == nil || session.managedOpts == nil {
		return
	}
	callback := session.managedOpts.CheckpointSavedCallback
	if callback == nil {
		return
	}
	callback(checkpointPath)
}

// SetConfigKey 设置 probe-scoped 配置持久化键（装配期一次性调用，Task 14 factory 用）。
//
// 设置后立即按新键重新加载持久化配置；legacy single 装配不调用本方法，
// 保持默认 "traversal" 键不变。运行期调用属于装配错误（文档约定，不做运行时防御）。
func (m *TraversalManager) SetConfigKey(key string) {
	if key == "" {
		return
	}
	m.mu.Lock()
	m.configKey = key
	m.mu.Unlock()
	if m.configStore != nil {
		m.loadPersistedConfig()
	}
}

// currentConfigKey 返回当前配置持久化键；未设置时回退 legacy "traversal"。
func (m *TraversalManager) currentConfigKey() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.configKey == "" {
		return traversalConfigKey
	}
	return m.configKey
}
