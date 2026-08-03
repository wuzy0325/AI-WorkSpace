package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"wind-daq/services/api-go/internal/core/traversal"
	"wind-daq/services/api-go/internal/ports"
)

// 双探针并行遍历 registry 契约（Phase 1 Foundation，仅类型与窄接口）。
//
// 规格：docs/specs/dual-traversal-spec.md（I2/I3/I5、FR2/FR3）。
// 本文件只定义后续 ManagerRegistry 所需的最小完整契约；registry 的
// GetOrCreate / admission / completion / Shutdown 实现在后续 Task 追加。
// Task 8 才让真实 TraversalManager 实现 ManagedTraversalManager，
// 在此之前本接口处于"已定义、未绑定"状态（编译不受影响的允许状态）。

// ProbeID 双探针标识（spec FR2：固定支持 probe1/probe2）。
type ProbeID string

const (
	Probe1 ProbeID = "probe1"
	Probe2 ProbeID = "probe2"
)

// ErrInvalidProbeID 未知 probe 标识（HTTP 层映射为 400 invalid_probe_id）。
var ErrInvalidProbeID = errors.New("invalid_probe_id")

// ParseProbeID 解析并校验 probe 标识字符串；非法值返回 ErrInvalidProbeID。
func ParseProbeID(s string) (ProbeID, error) {
	switch ProbeID(s) {
	case Probe1:
		return Probe1, nil
	case Probe2:
		return Probe2, nil
	default:
		return "", fmt.Errorf("%w: %q", ErrInvalidProbeID, s)
	}
}

// Valid 报告该 probe 标识是否为受支持的固定值。
func (p ProbeID) Valid() bool {
	return p == Probe1 || p == Probe2
}

// SessionToken registry 准入事务提交时生成的不可复用会话令牌（spec I2）。
//
// 包含 probe ID 与单调递增 generation；manager 的完成通知必须携带该 token，
// registry 仅在 token 仍是该 probe 当前 session 且尚未完成时原子减计数一次。
// 旧 generation 通知只记录诊断，不影响当前 session、控制器 lease 或全局计数。
type SessionToken struct {
	ProbeID    ProbeID
	Generation uint64
}

// String 返回诊断用文本形式（如 "probe1#3"），不用于持久化键。
func (t SessionToken) String() string {
	return fmt.Sprintf("%s#%d", t.ProbeID, t.Generation)
}

// ManagedSessionOptions managed 会话的不可变启动选项（Task 8 由 manager 冻结快照）。
//
// 在 managed Start/Resume 时一次性注入，禁止通过多个 setter 形成半配置状态；
// legacy single 路径不注入本结构，两种 lease ownership 不得在同一 session 混用。
type ManagedSessionOptions struct {
	// ProbeID 该 session 所属 probe。
	ProbeID ProbeID
	// ConfigKey probe-scoped 配置持久化键（如 "traversal.probe1"）。
	ConfigKey string
	// Token registry 颁发的会话令牌，完成回调时原样带回。
	Token SessionToken
	// TaskID 服务端权威任务 ID（registry 经 ports.TaskIDGenerator 生成）；
	// 客户端提交的 task ID 不得成为该值（spec I5）。
	TaskID string
	// CompletionCallback manager goroutine 退出且输出端口 finalize 后的唯一
	// 完成回报入口；registry 侧做 generation 校验与 exactly-once 减计数。
	CompletionCallback func(SessionToken)
	// CheckpointSavedCallback managed checkpoint 首次及后续每次成功保存后的通知
	// （Task 11：registry 据此把 probeId→taskId→checkpointPath 登记到 dual
	// recovery index，保证"映射存在 ⟺ checkpoint 文件存在"）。
	// 回调携带实际 checkpoint 文件路径（撞名 -2/-3 后的派生路径）。
	CheckpointSavedCallback func(checkpointPath string)
}

// ManagedTraversalManager registry 视角所需的窄 manager 接口（spec FR3）。
//
// 覆盖 managed Start/Resume、RunPoint、Pause、Resume、Stop、Done 与
// status/result/config/checkpoint 操作。managed Start/Resume 不 Acquire/Release
// 全局 workflow lease 或 controller lease（由 registry 准入事务负责），
// 完成时 finalize 后调用 ManagedSessionOptions.CompletionCallback。
//
// 注意：Task 8 才让 *TraversalManager 实现本接口；此处故意不做编译期断言，
// 避免在 manager 改造完成前产生编译错误。
type ManagedTraversalManager interface {
	// ParseConfig 解析并校验原始配置 JSON（registry Start façade 调用）。
	ParseConfig(raw json.RawMessage) (traversal.Config, error)
	// StartManaged 以 managed ownership 启动新任务；admission 已持有全部 lease。
	StartManaged(config traversal.Config, opts ManagedSessionOptions) error
	// ResumeManaged 以 managed ownership 从 checkpoint 恢复；返回权威 taskID。
	ResumeManaged(cp traversal.Checkpoint, opts ManagedSessionOptions) (string, error)
	// RunCurrentPoint 执行当前点位（供 runPoint 单步操作）。
	RunCurrentPoint() error
	// Pause 暂停运行中的任务。
	Pause() error
	// Resume 恢复已暂停的任务。
	Resume() error
	// Stop 请求停止；goroutine 退出与 finalize 由 manager 自身完成。
	Stop() error
	// Done 返回当前 session 的完成信号通道；无活动 session 时返回已关闭通道。
	Done() <-chan struct{}
	// Status 返回当前任务状态快照。
	Status() traversal.Status
	// GetResult 按 taskID 查询最终结果。
	GetResult(taskID string) (traversal.Status, bool)
	// SaveConfigRaw 持久化原始配置 JSON（probe-scoped key 由 manager 自身决定）。
	SaveConfigRaw(config json.RawMessage)
	// GetConfigRaw 读取持久化的原始配置 JSON。
	GetConfigRaw() json.RawMessage
	// LoadCheckpoint 读取该 manager 的可恢复 checkpoint（无候选返回 nil）。
	LoadCheckpoint() (*traversal.Checkpoint, error)
	// ClearCheckpoint 显式放弃当前可恢复 checkpoint。
	ClearCheckpoint()
}

// TraversalManagerFactory 按 probe 创建完整装配的 managed manager（spec FR3）。
//
// 由 composition root 实现：共享依赖（AcquisitionHub / MotionAccess / 查询端口 /
// 配置存储）通过闭包传入；每 manager 新建 TraversalCsvWriter / TraversalResultLog /
// checkpoint port 等有状态实例。registry 只依赖本接口，不导入 storage/hardware adapter。
type TraversalManagerFactory interface {
	// NewManager 创建指定 probe 的完整装配 manager；失败返回原始错误。
	NewManager(probeID ProbeID) (ManagedTraversalManager, error)
}

// ---------------------------------------------------------------------------
// ManagerRegistry（Task 3：核心结构 + GetOrCreate；Task 4：准入事务见
// traversal_registry_admission.go；Task 5-7：completion/Stop/Shutdown 生命周期）
// ---------------------------------------------------------------------------

// registry 业务错误码哨兵（HTTP 层按 errors.Is 映射状态码，spec FR4）。
var (
	// ErrRegistryClosing registry 正在关闭，拒绝新任务（HTTP 503 registry_closing）。
	ErrRegistryClosing = errors.New("registry_closing")
	// ErrResourceConflict 资源冲突：控制器空绑定/双路相同/已被占用（HTTP 409 resource_conflict）。
	ErrResourceConflict = errors.New("resource_conflict")
	// ErrAlreadyRunning 同一 probe 已有活动 session（HTTP 409 already_running）。
	ErrAlreadyRunning = errors.New("already_running")
	// ErrProbeClosing probe 的 manager 处于 CloseProbe 保留期间（HTTP 409 probe_closing）。
	ErrProbeClosing = errors.New("probe_closing")
	// ErrCloseProbeTimeout CloseProbe 等待 completion 提交超时（HTTP 504/409 close_probe_timeout）。
	ErrCloseProbeTimeout = errors.New("close_probe_timeout")
	// ErrShutdownTimeout shutdown hard deadline 到期仍有未退出任务（含 probe/task ID）。
	ErrShutdownTimeout = errors.New("shutdown_timeout")
	// ErrTaskIDMismatch 请求 taskID 与 dual recovery index 权威候选不一致（HTTP 400）。
	ErrTaskIDMismatch = errors.New("task_id_mismatch")
	// ErrProbeIDMismatch checkpoint 中 ProbeID 与请求 probeID 不一致（HTTP 400）。
	ErrProbeIDMismatch = errors.New("probe_id_mismatch")
)

const (
	// registryWorkflowHolder registry 持有全局 workflow:traversal lease 的固定身份
	// （spec I2：与 legacy single 的 taskID holder 经同一资源互斥）。
	registryWorkflowHolder = "traversal-registry"
	// registryLeaseTTL 工作流/控制器 lease TTL。Task 5 的续约器在 session 运行期间
	// 定期续约；进程崩溃时 lease 在 TTL 后自动过期可被接管（优于 legacy 24h）。
	registryLeaseTTL = 30 * time.Second
)

// ManagerRegistryDeps registry 装配依赖（全部注入端口/工厂，registry 不导入 adapter）。
type ManagerRegistryDeps struct {
	Factory         TraversalManagerFactory
	TaskIDGenerator ports.TaskIDGenerator
	WorkflowLease   ports.WorkflowLeasePort
	ControllerLease ports.ControllerLeasePort
	RecoveryIndex   ports.DualTraversalRecoveryIndex
	ConfigStore     ports.AppConfigStore
	// MotionAccess Shutdown hard deadline 路径的 EmergencyStop 端口（可选；
	// 未注入时 ES 阶段仅记录诊断错误）。生产装配见 Task 14。
	MotionAccess ports.MotionAccess
	// CheckpointStore dual v3 checkpoint 的读取/删除端口（可选；Task 11 的
	// registry probe-scoped resume/clear 用；未注入时恢复 façade 返回装配错误）。
	CheckpointStore ports.CheckpointStore
}

// probeCreationGate per-probe 创建闸门：保证同一 probe 仅一个 in-flight factory 调用。
// done 关闭后 result/err 可读（close 与字段写入在同一临界区，happen-before 由 channel 保证）。
type probeCreationGate struct {
	done   chan struct{}
	result ManagedTraversalManager
	err    error
}

// registrySession 一次准入事务登记的活动会话（spec I2/I3）。
//
// 生命周期（state 字段，r.mu 保护）：active → completing → completed /
// completion_failed。completion_failed 的 session 保留在 map 中（activeCount 不递减），
// 由 Task 6 CloseProbe/Shutdown 经 retryCompletionCleanup 幂等重试。
type registrySession struct {
	probeID ProbeID
	taskID  string // 服务端权威任务 ID
	token   SessionToken
	manager ManagedTraversalManager
	// boundControllerIDs 启动快照冻结的控制器 ID 去重列表（Task 9 运动安全隔离的输入）。
	// 急停按控制器级别执行（EmergencyStop 是控制器级操作，无法只停单轴），故仍保留
	// 去重后的 controllerID 列表；同控制器的不同轴 lease 在 boundAxisPairs 中维护。
	boundControllerIDs []string
	// boundAxisPairs 启动快照冻结的 (controllerID, axis) 元组列表（资源独占的真实粒度）。
	// 用于冲突检测快照与 lease token 映射的 key 源。
	boundAxisPairs []traversal.MotionAxisBinding
	// controllerTokens (controllerID, axis) → leaseToken（Task 1 ControllerLeasePort 签发）。
	// admission 后不可变，续约器读取无需持锁。key 为 ControllerAxisPair 结构体，
	// 同一控制器的不同轴有独立 token，互不影响。
	controllerTokens map[ControllerAxisPair]string
	// workflowAcquired 本次准入是否触发了全局 workflow lease 获取（仅第一路 true），
	// 供准入回滚决定是否释放。
	workflowAcquired bool
	// state 会话状态（r.mu 保护）。
	state sessionState
	// done 在 completion 提交后关闭，供 Stop/CloseProbe 有界等待。
	done chan struct{}
	// settled 在 cleanup 到达静态（completed 或 completion_failed）后关闭；
	// 与 done 的区别：completion_failed 也会关闭 settled，使 Stop/CloseProbe
	// 能及时进入重试分支而非干等超时。
	settled     chan struct{}
	settledOnce sync.Once
	// completionErr completion 阶段的聚合错误（r.mu 保护），供诊断与重试决策。
	completionErr error
	// recoveryErr/recoveryPath 保留 checkpoint recovery index 的失败状态与重试输入。
	recoveryErr  error
	recoveryPath string
	// rollback 标记 manager 启动失败的准入回滚，不执行 recovery index 收尾。
	rollback bool
	// renewErr 续约失败诊断（r.mu 保护）；写入后即请求该 probe 停止。
	renewErr error
	// renewCancel/renewDone session 续约器控制；completion 先停续约器再清理 lease，
	// 避免 Renew 与 Release 竞态。
	renewCancel context.CancelFunc
	renewDone   chan struct{}
	// cleanupMu 串行化 completion 清理与 CloseProbe/Shutdown 的幂等重试。
	cleanupMu sync.Mutex
	// recoveryStopOnce bounds asynchronous stop requests after recovery index failures.
	recoveryStopOnce sync.Once
	// pendingReleases 尚未成功释放的控制器轴 lease（cleanupMu 保护）；
	// 初始为 controllerTokens 的拷贝，释放成功即删除对应条目。
	pendingReleases map[ControllerAxisPair]string
}

// ManagerRegistry 双探针 manager 注册表（spec FR3）。
//
// 并发约定：
//   - mu 保护 closing/managers/creating/sessions/activeCount/generations；
//   - factory 调用、持久化配置读取、manager 生命周期 I/O 均在 mu 外执行；
//   - 资源检查与预占（绑定比较 + lease Acquire）在同一临界区完成，杜绝 TOCTOU。
type ManagerRegistry struct {
	mu sync.Mutex
	// admissionGate 串行化 admission、回滚和最后一路 workflow lease 交接。
	// Channel ownership makes waiting context-bounded; external lease I/O never holds r.mu.
	admissionGate chan struct{}
	// probeGates 每 probe 独立生命周期 gate（I-7）：序列化同 probe 的 Start/Stop/Pause/
	// Resume/RunPoint/CloseProbe，避免单 probe 长耗时操作（如 RunPoint 运动）阻塞其他
	// probe 的并发操作（违反 user_rules 第 9 条）。全局 admissionGate 仅用于 workflow
	// lease 交接等全局事务；per-probe 操作经 probeGates 互不阻塞。
	probeGates   map[ProbeID]chan struct{}
	probeGatesMu sync.Mutex
	closing      bool
	factory      TraversalManagerFactory
	managers     map[ProbeID]ManagedTraversalManager
	creating     map[ProbeID]*probeCreationGate
	sessions     map[ProbeID]*registrySession
	activeCount  int
	generations  map[ProbeID]uint64
	// workflowTransition 全局 lease 交接 gate：最后一路清理时置位，阻止新 admission
	// 复用旧 lease 状态；全局 lease 释放提交后清除。
	workflowTransition bool
	// workflowRenewCancel/workflowRenewDone 全局 workflow lease 续约器控制；
	// 第一路准入启动，最后一路提交前停止。
	workflowRenewCancel context.CancelFunc
	workflowRenewDone   chan struct{}
	// closingProbes CloseProbe 保留期间的 probe 集合；GetOrCreate 对其返回 probe_closing。
	closingProbes map[ProbeID]bool
	// gracefulTimeout/hardTimeout shutdown 双 deadline（可经 option 覆盖）。
	gracefulTimeout time.Duration
	hardTimeout     time.Duration
	// motion Shutdown hard deadline 路径的 EmergencyStop 端口（可选注入；Task 14 装配）。
	motion ports.MotionAccess
	// checkpointStore dual v3 checkpoint 读取/删除端口（可选注入；Task 11 恢复 façade 用）。
	checkpointStore ports.CheckpointStore

	taskIDs         ports.TaskIDGenerator
	workflowLease   ports.WorkflowLeasePort
	controllerLease ports.ControllerLeasePort
	recoveryIndex   ports.DualTraversalRecoveryIndex
	configStore     ports.AppConfigStore
}

// 默认 shutdown 双 deadline（spec FR9：graceful 5s / hard 10s，hard > graceful）。
const (
	defaultGracefulShutdownTimeout = 5 * time.Second
	defaultHardShutdownTimeout     = 10 * time.Second
)

// ManagerRegistryOption composition root 可配置项（Task 14 装配）。
type ManagerRegistryOption func(*ManagerRegistry) error

// WithShutdownTimeouts 覆盖 shutdown graceful/hard deadline；必须为有限正值且 hard > graceful。
func WithShutdownTimeouts(graceful, hard time.Duration) ManagerRegistryOption {
	return func(r *ManagerRegistry) error {
		if graceful <= 0 || hard <= 0 || hard <= graceful {
			return fmt.Errorf("shutdown 超时配置无效: graceful=%s hard=%s（必须为有限正值且 hard > graceful）", graceful, hard)
		}
		r.gracefulTimeout, r.hardTimeout = graceful, hard
		return nil
	}
}

// NewManagerRegistry 装配 registry；除 MotionAccess（可选）外任一依赖为 nil 时返回错误。
func NewManagerRegistry(deps ManagerRegistryDeps, opts ...ManagerRegistryOption) (*ManagerRegistry, error) {
	if deps.Factory == nil || deps.TaskIDGenerator == nil || deps.WorkflowLease == nil ||
		deps.ControllerLease == nil || deps.RecoveryIndex == nil || deps.ConfigStore == nil {
		return nil, errors.New("manager registry deps 不完整")
	}
	registry := &ManagerRegistry{
		factory:         deps.Factory,
		admissionGate:   make(chan struct{}, 1),
		probeGates:      make(map[ProbeID]chan struct{}),
		managers:        make(map[ProbeID]ManagedTraversalManager),
		creating:        make(map[ProbeID]*probeCreationGate),
		sessions:        make(map[ProbeID]*registrySession),
		generations:     make(map[ProbeID]uint64),
		closingProbes:   make(map[ProbeID]bool),
		gracefulTimeout: defaultGracefulShutdownTimeout,
		hardTimeout:     defaultHardShutdownTimeout,
		motion:          deps.MotionAccess,
		checkpointStore: deps.CheckpointStore,
		taskIDs:         deps.TaskIDGenerator,
		workflowLease:   deps.WorkflowLease,
		controllerLease: deps.ControllerLease,
		recoveryIndex:   deps.RecoveryIndex,
		configStore:     deps.ConfigStore,
	}
	registry.admissionGate <- struct{}{}
	for _, opt := range opts {
		if err := opt(registry); err != nil {
			return nil, err
		}
	}
	return registry, nil
}

func (r *ManagerRegistry) acquireAdmission(ctx context.Context) error {
	select {
	case <-r.admissionGate:
		if err := ctx.Err(); err != nil {
			r.releaseAdmission()
			return err
		}
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (r *ManagerRegistry) releaseAdmission() {
	r.admissionGate <- struct{}{}
}

// probeGate 返回指定 probe 的 lifecycle gate（不存在时懒创建并填充初始 token）。
// gate 为 capacity-1 channel：acquire = 读取 token，release = 写回 token。
// 不同 probe 的 gate 互相独立，杜绝跨 probe 阻塞（I-7）。
func (r *ManagerRegistry) probeGate(probeID ProbeID) chan struct{} {
	r.probeGatesMu.Lock()
	defer r.probeGatesMu.Unlock()
	gate, ok := r.probeGates[probeID]
	if !ok {
		gate = make(chan struct{}, 1)
		gate <- struct{}{}
		r.probeGates[probeID] = gate
	}
	return gate
}

// acquireProbeGate 获取指定 probe 的 lifecycle gate（context-aware）。
// 用于 Start/Stop/Pause/Resume/RunPoint/CloseProbe 串行化同 probe 操作。
func (r *ManagerRegistry) acquireProbeGate(ctx context.Context, probeID ProbeID) error {
	select {
	case <-r.probeGate(probeID):
		if err := ctx.Err(); err != nil {
			r.releaseProbeGate(probeID)
			return err
		}
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// releaseProbeGate 释放指定 probe 的 lifecycle gate。
func (r *ManagerRegistry) releaseProbeGate(probeID ProbeID) {
	r.probeGate(probeID) <- struct{}{}
}

// GetOrCreate 返回指定 probe 的 manager；不存在时经 per-probe creation gate 创建。
//
//   - 未知 probeID → ErrInvalidProbeID；closing → ErrRegistryClosing；
//   - 同一 probe 并发调用只触发一次 factory（等待者共享同一结果，含失败）；
//   - factory 在 registry mutex 外运行；失败不污染 map，后续调用可重试。
func (r *ManagerRegistry) GetOrCreate(probeID ProbeID) (ManagedTraversalManager, error) {
	if !probeID.Valid() {
		return nil, fmt.Errorf("%w: %q", ErrInvalidProbeID, probeID)
	}
	gate, leader := r.creationGateFor(probeID)
	if !leader {
		<-gate.done
		return gate.result, gate.err
	}
	manager, err := r.factory.NewManager(probeID)
	r.mu.Lock()
	if err == nil && r.closing {
		// 创建期间进入 closing：不安装、不启动，等待者同样收到 closing 错误。
		err = ErrRegistryClosing
	}
	if err == nil {
		r.managers[probeID] = manager
		gate.result = manager
	}
	delete(r.creating, probeID)
	gate.err = err
	close(gate.done)
	r.mu.Unlock()
	return gate.result, gate.err
}

// creationGateFor 返回该 probe 的创建闸门；leader=true 表示调用者负责执行 factory。
func (r *ManagerRegistry) creationGateFor(probeID ProbeID) (*probeCreationGate, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closing {
		return &probeCreationGate{done: closedChan(), err: ErrRegistryClosing}, false
	}
	if r.closingProbes[probeID] {
		// CloseProbe 保留期间：不返回 closing manager，也不创建新 manager。
		return &probeCreationGate{done: closedChan(), err: fmt.Errorf("%w: %s", ErrProbeClosing, probeID)}, false
	}
	if manager, ok := r.managers[probeID]; ok {
		return &probeCreationGate{done: closedChan(), result: manager}, false
	}
	if gate, ok := r.creating[probeID]; ok {
		return gate, false
	}
	gate := &probeCreationGate{done: make(chan struct{})}
	r.creating[probeID] = gate
	return gate, true
}

// closedChan 返回已关闭的通道（gate 即时完成语义）。
func closedChan() chan struct{} {
	ch := make(chan struct{})
	close(ch)
	return ch
}

// otherProbe 返回另一路 probe 标识。
func otherProbe(probeID ProbeID) ProbeID {
	if probeID == Probe1 {
		return Probe2
	}
	return Probe1
}

// probeConfigKey probe-scoped 配置持久化键（spec FR2：traversal.probe1/probe2）。
func probeConfigKey(probeID ProbeID) string {
	return "traversal." + string(probeID)
}

// boundControllerIDs 提取配置中唯一非空控制器 ID 列表（字典序，便于比较与诊断）。
// 仅用于急停快照（EmergencyStop 按控制器级执行，去重后逐台急停）。
// 资源独占检测与 lease 请用 boundControllerAxisPairs。
func boundControllerIDs(cfg traversal.Config) []string {
	seen := make(map[string]bool)
	ids := make([]string, 0, len(cfg.MotionAxes))
	for _, axis := range cfg.MotionAxes {
		id := strings.TrimSpace(axis.ControllerID)
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

// boundControllerIDsFromPairs 从 (controllerID, axis) 元组列表派生去重后的控制器 ID 列表。
// 用于 EmergencyStop 等控制器级操作：同一控制器的多个轴 lease 只产生一次 ES 调用。
// 接受 MotionAxisBinding 列表（与 boundControllerAxisPairs 输出类型一致），便于
// 在 registerSessionLocked 中直接从启动快照的 axis pairs 派生 controller 列表。
func boundControllerIDsFromPairs(pairs []traversal.MotionAxisBinding) []string {
	seen := make(map[string]bool)
	ids := make([]string, 0, len(pairs))
	for _, p := range pairs {
		id := strings.TrimSpace(p.ControllerID)
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

// ControllerAxisPair 控制器轴资源标识 (controllerID, axis) 元组。
// 作为 lease token map 的 key 与冲突检测的输入——同一控制器的不同物理轴
// 可被两个 probe 分别独占（风洞实验中两探针共用同一运动控制器的不同轴）。
type ControllerAxisPair struct {
	ControllerID string
	Axis         string
}

// boundControllerAxisPairs 提取配置中唯一 (controllerID, axis) 元组列表。
// 资源独占的真实粒度：同一控制器的不同轴不视为冲突，可分别 lease。
// controllerID 或 axis 为空的绑定被跳过（未完成配置由 ParseConfig 拦截）。
func boundControllerAxisPairs(cfg traversal.Config) []traversal.MotionAxisBinding {
	seen := make(map[ControllerAxisPair]bool)
	pairs := make([]traversal.MotionAxisBinding, 0, len(cfg.MotionAxes))
	for _, axis := range cfg.MotionAxes {
		id := strings.TrimSpace(axis.ControllerID)
		ax := strings.TrimSpace(axis.Axis)
		if id == "" || ax == "" {
			continue
		}
		key := ControllerAxisPair{ControllerID: id, Axis: ax}
		if seen[key] {
			continue
		}
		seen[key] = true
		pairs = append(pairs, traversal.MotionAxisBinding{
			Name:         axis.Name,
			ControllerID: id,
			Axis:         ax,
		})
	}
	return pairs
}
