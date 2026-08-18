package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"os"

	"windlabx4/services/api-go/internal/core/motion"
	"windlabx4/services/api-go/internal/core/traversal"
	"windlabx4/services/api-go/internal/ports"
)

// ---------------------------------------------------------------------------
// registry 测试共享 fakes（Task 3/4；Task 5-7 可扩展）
// ---------------------------------------------------------------------------

// fakeManagedManager 实现 ManagedTraversalManager 的测试桩。
type fakeManagedManager struct {
	mu         sync.Mutex
	parseCalls int
	startCalls int
	stopCalls  int
	runCalls   int
	pauseCalls int
	resumeOps  int
	startErr   error
	startBlock chan struct{}
	startEnter chan struct{}
	onStart    func(ManagedSessionOptions)
	lastConfig traversal.Config
	lastOpts   ManagedSessionOptions
	configRaw  json.RawMessage
	// statusState/statusCSVPath Status() 返回值注入（completion 恢复映射收尾用）。
	statusState   traversal.State
	statusCSVPath string
	// onStop Stop 成功返回后触发的钩子（测试注入完成回调）；stopBlock 非 nil 时
	// Stop 先阻塞至其关闭（生命周期编排测试用）；stopErr 注入 Stop 返回错误。
	onStop    func()
	stopBlock chan struct{}
	stopErr   error
}

func (f *fakeManagedManager) ParseConfig(raw json.RawMessage) (traversal.Config, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.parseCalls++
	var cfg traversal.Config
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return traversal.Config{}, err
	}
	return cfg, nil
}

func (f *fakeManagedManager) StartManaged(cfg traversal.Config, opts ManagedSessionOptions) error {
	f.mu.Lock()
	f.startCalls++
	f.lastConfig = cfg
	f.lastOpts = opts
	err := f.startErr
	block := f.startBlock
	entered := f.startEnter
	hook := f.onStart
	f.mu.Unlock()
	if hook != nil {
		hook(opts)
	}
	if entered != nil {
		close(entered)
	}
	if block != nil {
		<-block
	}
	return err
}

func (f *fakeManagedManager) snapshotStart() (int, traversal.Config, ManagedSessionOptions) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.startCalls, f.lastConfig, f.lastOpts
}

func (f *fakeManagedManager) setStartErr(err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.startErr = err
}

func (f *fakeManagedManager) setStartBlock() (<-chan struct{}, func()) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.startBlock = make(chan struct{})
	f.startEnter = make(chan struct{})
	return f.startEnter, func() { close(f.startBlock) }
}

func (f *fakeManagedManager) setOnStart(hook func(ManagedSessionOptions)) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.onStart = hook
}

func (f *fakeManagedManager) RunCurrentPoint() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.runCalls++
	return nil
}
func (f *fakeManagedManager) Pause() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.pauseCalls++
	return nil
}
func (f *fakeManagedManager) Resume() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.resumeOps++
	return nil
}

func (f *fakeManagedManager) lifecycleCallCounts() (run, pause, resume int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.runCalls, f.pauseCalls, f.resumeOps
}

func (f *fakeManagedManager) Stop() error {
	f.mu.Lock()
	f.stopCalls++
	block := f.stopBlock
	hook := f.onStop
	err := f.stopErr
	f.mu.Unlock()
	if block != nil {
		<-block
	}
	if hook != nil {
		hook()
	}
	return err
}

func (f *fakeManagedManager) stopCallCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.stopCalls
}

// setOnStop 注入 Stop 钩子（如触发完成回调模拟 goroutine exit + finalize）。
func (f *fakeManagedManager) setOnStop(hook func()) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.onStop = hook
}

// setStopErr 注入 Stop 返回错误（错误聚合测试用）。
func (f *fakeManagedManager) setStopErr(err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.stopErr = err
}

// setStopBlock 注入阻塞式 Stop（返回放行函数）。
func (f *fakeManagedManager) setStopBlock() (unblock func()) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.stopBlock = make(chan struct{})
	return func() { close(f.stopBlock) }
}
func (f *fakeManagedManager) Done() <-chan struct{} {
	ch := make(chan struct{})
	close(ch)
	return ch
}
func (f *fakeManagedManager) Status() traversal.Status {
	f.mu.Lock()
	defer f.mu.Unlock()
	return traversal.Status{State: f.statusState, CSVPath: f.statusCSVPath}
}
func (f *fakeManagedManager) GetResult(string) (traversal.Status, bool) {
	return traversal.Status{}, false
}
func (f *fakeManagedManager) SaveConfigRaw(config json.RawMessage) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.configRaw = config
}
func (f *fakeManagedManager) GetConfigRaw() json.RawMessage {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.configRaw
}

// fakeManagerFactory 记录 factory 调用次数与按 probe 创建的 manager。
// create 钩子（barrier 测试用）在 factory 互斥锁外执行，允许阻塞。
type fakeManagerFactory struct {
	mu       sync.Mutex
	calls    int
	perProbe map[ProbeID]*fakeManagedManager
	create   func(probeID ProbeID) (ManagedTraversalManager, error)
}

func (f *fakeManagerFactory) NewManager(probeID ProbeID) (ManagedTraversalManager, error) {
	f.mu.Lock()
	f.calls++
	hook := f.create
	f.mu.Unlock()
	if hook != nil {
		return hook(probeID)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	manager := &fakeManagedManager{}
	f.perProbe[probeID] = manager
	return manager, nil
}

func (f *fakeManagerFactory) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

func (f *fakeManagerFactory) manager(probeID ProbeID) *fakeManagedManager {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.perProbe[probeID]
}

func (f *fakeManagerFactory) setCreateHook(hook func(probeID ProbeID) (ManagedTraversalManager, error)) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.create = hook
}

// fakeTaskIDGenerator 生成含 probe 命名空间的服务端任务 ID。
type fakeTaskIDGenerator struct {
	mu sync.Mutex
	n  int
}

func (g *fakeTaskIDGenerator) NewTaskID(_ context.Context, probeID string) (string, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.n++
	return fmt.Sprintf("%s-task-%d", probeID, g.n), nil
}

// fakeWorkflowLease 进程内固定 holder 工作流 lease。
type fakeWorkflowLease struct {
	mu           sync.Mutex
	held         bool
	holder       string
	acquireCalls int
	releaseCalls int
	renewCalls   int
	renewErr     error
	releaseErr   error
	onAcquire    func()
	// releaseBlock 非 nil 时 Release 在持锁状态下阻塞直至其关闭（completion 窗口编排用）；
	// releaseEntered 在 Release 进入时关闭一次（同步信号）。
	releaseBlock   chan struct{}
	releaseEntered chan struct{}
	enteredOnce    sync.Once
}

func (l *fakeWorkflowLease) Acquire(_ context.Context, holder string, _ time.Duration) error {
	if l.onAcquire != nil {
		l.onAcquire()
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.held && l.holder != holder {
		return fmt.Errorf("workflow lease held by %s", l.holder)
	}
	l.held, l.holder = true, holder
	l.acquireCalls++
	return nil
}

func (l *fakeWorkflowLease) Renew(_ context.Context, holder string, _ time.Duration) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.renewErr != nil {
		return l.renewErr
	}
	if !l.held || l.holder != holder {
		return errors.New("workflow lease not held by holder")
	}
	l.renewCalls++
	return nil
}

func (l *fakeWorkflowLease) Release(_ context.Context, holder string) error {
	l.mu.Lock()
	if l.releaseEntered != nil {
		l.enteredOnce.Do(func() { close(l.releaseEntered) })
	}
	block := l.releaseBlock
	l.mu.Unlock()
	// 阻塞等待在锁外进行，避免持锁阻塞期间 state()/Start 等路径死锁。
	if block != nil {
		<-block
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.releaseErr != nil {
		return l.releaseErr
	}
	if !l.held {
		return nil
	}
	if l.holder != holder {
		return fmt.Errorf("workflow lease held by %s, not %s", l.holder, holder)
	}
	l.held = false
	l.releaseCalls++
	return nil
}

func (l *fakeWorkflowLease) state() (held bool, holder string, acquires, releases int) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.held, l.holder, l.acquireCalls, l.releaseCalls
}

func (l *fakeWorkflowLease) renewCount() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.renewCalls
}

func (l *fakeWorkflowLease) setRenewErr(err error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.renewErr = err
}

func (l *fakeWorkflowLease) setReleaseErr(err error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.releaseErr = err
}

// setReleaseBlock 注入阻塞式 Release（返回通道用于等待 Release 进入）。
func (l *fakeWorkflowLease) setReleaseBlock() (entered <-chan struct{}, unblock func()) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.releaseBlock = make(chan struct{})
	l.releaseEntered = make(chan struct{})
	var once sync.Once
	return l.releaseEntered, func() { once.Do(func() { close(l.releaseBlock) }) }
}

// fakeControllerLease (controllerID, axis)-scoped token lease（互斥锁内原子 check-and-set）。
//
// 资源独占粒度与生产实现一致：同一控制器的不同物理轴可被两个 session 分别 lease；
// 只有同一 (controllerID, axis) 元组才视为冲突资源。failAcquire/failRelease 按
// controllerID 注入失败（覆盖该控制器的所有轴），保持既有测试的控制器级失败语义。
type fakeControllerLease struct {
	mu       sync.Mutex
	held     map[ControllerAxisPair]string // (controllerID, axis) → token
	next     int
	acquires map[ControllerAxisPair]int
	// renewCalls 续约次数；renewErr 注入续约失败；failRelease 按 controllerID 注入释放失败。
	renewCalls  int
	renewErr    error
	failRelease map[string]bool
	failAcquire map[string]bool
	onAcquire   func()
}

func newFakeControllerLease() *fakeControllerLease {
	return &fakeControllerLease{
		held: make(map[ControllerAxisPair]string), acquires: make(map[ControllerAxisPair]int),
		failAcquire: make(map[string]bool), failRelease: make(map[string]bool),
	}
}

func (l *fakeControllerLease) Acquire(_ context.Context, controllerID, axis, _ string, _ time.Duration) (string, error) {
	if l.onAcquire != nil {
		l.onAcquire()
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.failAcquire[controllerID] {
		return "", fmt.Errorf("controller %s acquire failed (injected)", controllerID)
	}
	pair := ControllerAxisPair{ControllerID: controllerID, Axis: axis}
	if _, ok := l.held[pair]; ok {
		return "", fmt.Errorf("controller %s axis %s already leased", controllerID, axis)
	}
	l.next++
	token := fmt.Sprintf("tok-%s-%s-%d", controllerID, axis, l.next)
	l.held[pair] = token
	l.acquires[pair]++
	return token, nil
}

func (l *fakeControllerLease) Renew(_ context.Context, leaseToken string, _ time.Duration) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.renewErr != nil {
		return l.renewErr
	}
	for _, token := range l.held {
		if token == leaseToken {
			l.renewCalls++
			return nil
		}
	}
	return errors.New("unknown controller lease token")
}

func (l *fakeControllerLease) Release(_ context.Context, leaseToken string) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	for pair, token := range l.held {
		if token == leaseToken {
			if l.failRelease[pair.ControllerID] {
				return fmt.Errorf("controller %s release failed (injected)", pair.ControllerID)
			}
			delete(l.held, pair)
			return nil
		}
	}
	return errors.New("unknown controller lease token")
}

// heldCount 返回该控制器当前被持有的轴 lease 数量（同一控制器的多个轴各算一份）。
// 既有测试在每控制器只配一个轴的场景下期待返回 1；新场景下若两 probe 分别 lease
// 同一控制器的 X/Y 轴，则返回 2。
func (l *fakeControllerLease) heldCount(controllerID string) int {
	l.mu.Lock()
	defer l.mu.Unlock()
	count := 0
	for pair := range l.held {
		if pair.ControllerID == controllerID {
			count++
		}
	}
	return count
}

// heldAxisCount 返回 (controllerID, axis) 元组的 lease 数量（0 或 1）。
// 用于精确断言同控制器不同轴的 lease 状态。
func (l *fakeControllerLease) heldAxisCount(controllerID, axis string) int {
	l.mu.Lock()
	defer l.mu.Unlock()
	if _, ok := l.held[ControllerAxisPair{ControllerID: controllerID, Axis: axis}]; ok {
		return 1
	}
	return 0
}

func (l *fakeControllerLease) totalHeld() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.held)
}

func (l *fakeControllerLease) renewCount() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.renewCalls
}

func (l *fakeControllerLease) setRenewErr(err error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.renewErr = err
}

func (l *fakeControllerLease) setReleaseFail(controllerID string, fail bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.failRelease[controllerID] = fail
}

func (l *fakeControllerLease) setAcquireFail(controllerID string, fail bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.failAcquire[controllerID] = fail
}

// fakeRecoveryIndex 内存版 DualTraversalRecoveryIndex。
type fakeRecoveryIndex struct {
	mu            sync.Mutex
	candidates    map[string]ports.TraversalCheckpointRef
	registerErr   error
	unregisterErr error
}

func (i *fakeRecoveryIndex) Register(_ context.Context, probeID, taskID, checkpointPath string) error {
	i.mu.Lock()
	defer i.mu.Unlock()
	if i.registerErr != nil {
		return i.registerErr
	}
	if i.candidates == nil {
		i.candidates = make(map[string]ports.TraversalCheckpointRef)
	}
	// 与真实 adapter 语义一致：同 probe 同 taskID 幂等更新，不同 taskID 拒绝。
	if existing, exists := i.candidates[probeID]; exists && existing.TaskID != taskID {
		return ports.ErrRecoverableTaskExists
	}
	i.candidates[probeID] = ports.TraversalCheckpointRef{TaskID: taskID, Path: checkpointPath}
	return nil
}

func (i *fakeRecoveryIndex) Find(_ context.Context, probeID string) (ports.TraversalCheckpointRef, bool, error) {
	i.mu.Lock()
	defer i.mu.Unlock()
	ref, found := i.candidates[probeID]
	return ref, found, nil
}

func (i *fakeRecoveryIndex) Unregister(_ context.Context, probeID, taskID string) error {
	i.mu.Lock()
	defer i.mu.Unlock()
	if i.unregisterErr != nil {
		return i.unregisterErr
	}
	// 与真实 adapter 语义一致：无候选幂等成功；候选存在但 taskID 不符返回错误
	// （防止实现误传文件 taskID 而非索引 taskID 时测试静默通过）。
	if existing, found := i.candidates[probeID]; found && existing.TaskID != taskID {
		return fmt.Errorf("注销任务与登记候选不一致: probe=%s task=%s", probeID, taskID)
	}
	delete(i.candidates, probeID)
	return nil
}

func (i *fakeRecoveryIndex) setErrors(registerErr, unregisterErr error) {
	i.mu.Lock()
	defer i.mu.Unlock()
	i.registerErr = registerErr
	i.unregisterErr = unregisterErr
}

func (i *fakeRecoveryIndex) ListProbeTaskIDs(_ context.Context, probeID string) ([]string, error) {
	i.mu.Lock()
	defer i.mu.Unlock()
	ref, found := i.candidates[probeID]
	if !found {
		return []string{}, nil
	}
	return []string{ref.TaskID}, nil
}

func (i *fakeRecoveryIndex) seed(probeID, taskID, path string) {
	i.mu.Lock()
	defer i.mu.Unlock()
	if i.candidates == nil {
		i.candidates = make(map[string]ports.TraversalCheckpointRef)
	}
	i.candidates[probeID] = ports.TraversalCheckpointRef{TaskID: taskID, Path: path}
}

// fakeConfigStore 内存版 AppConfigStore。
type fakeConfigStore struct {
	mu   sync.Mutex
	data map[string][]byte
}

func (s *fakeConfigStore) LoadConfig(key string) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	data, ok := s.data[key]
	if !ok {
		// 与生产 FileAppConfigStore 一致：key 不存在返回 (nil, nil)，
		// 让调用方能区分"未配置"与真实 I/O 错误（I-6 修复后 loadProbeBindings 依赖该语义）。
		return nil, nil
	}
	return data, nil
}

func (s *fakeConfigStore) SaveConfig(key string, data []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data[key] = data
	return nil
}

func (s *fakeConfigStore) seed(key string, data []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data[key] = data
}

// registryFixture Task 3/4 测试装配。
type registryFixture struct {
	registry    *ManagerRegistry
	factory     *fakeManagerFactory
	taskIDs     *fakeTaskIDGenerator
	workflow    *fakeWorkflowLease
	controllers *fakeControllerLease
	index       *fakeRecoveryIndex
	configs     *fakeConfigStore
	motion      *fakeMotionAccess
	cpStore     *fakeCheckpointStore
}

// completeSession 模拟 manager goroutine exit + finalize 后的完成回调。
// 补回 lts/win7 误删的 traversal_registry_completion_test.go 中的通用 helper，
// 供 admission/lifecycle/recovery 等 test 复用（master 版本依赖 testing/synctest，
// lts/win7 go.mod 声明 go 1.20 不支持，故仅提取此函数）。
func completeSession(fx *registryFixture, probeID ProbeID) {
	fx.registry.mu.Lock()
	session := fx.registry.sessions[probeID]
	fx.registry.mu.Unlock()
	if session == nil {
		return
	}
	fx.registry.notifyCompletion(session.token)
}

// startPairOK 启动 Probe1/Probe2 双探针的成功路径快捷封装。
// 同样补回自 traversal_registry_completion_test.go（master 版本依赖 testing/synctest，
// lts/win7 go 1.20 不支持，故仅提取此函数）。
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

func newRegistryFixture(t *testing.T) *registryFixture {
	t.Helper()
	fx := &registryFixture{
		factory:     &fakeManagerFactory{perProbe: make(map[ProbeID]*fakeManagedManager)},
		taskIDs:     &fakeTaskIDGenerator{},
		workflow:    &fakeWorkflowLease{},
		controllers: newFakeControllerLease(),
		index:       &fakeRecoveryIndex{},
		configs:     &fakeConfigStore{data: make(map[string][]byte)},
		motion:      newFakeMotionAccess(),
		cpStore:     newFakeCheckpointStore(),
	}
	registry, err := NewManagerRegistry(ManagerRegistryDeps{
		Factory:         fx.factory,
		TaskIDGenerator: fx.taskIDs,
		WorkflowLease:   fx.workflow,
		ControllerLease: fx.controllers,
		RecoveryIndex:   fx.index,
		ConfigStore:     fx.configs,
		MotionAccess:    fx.motion,
		CheckpointStore: fx.cpStore,
	})
	if err != nil {
		t.Fatalf("NewManagerRegistry: %v", err)
	}
	fx.registry = registry
	return fx
}

func (f *registryFixture) activeCount() int {
	f.registry.mu.Lock()
	defer f.registry.mu.Unlock()
	return f.registry.activeCount
}

func (f *registryFixture) sessionCount() int {
	f.registry.mu.Lock()
	defer f.registry.mu.Unlock()
	return len(f.registry.sessions)
}

func (f *registryFixture) setClosing() {
	f.registry.mu.Lock()
	defer f.registry.mu.Unlock()
	f.registry.closing = true
}

// sessionFor 白盒读取 session（可能为 nil）。
func (f *registryFixture) sessionFor(probeID ProbeID) *registrySession {
	f.registry.mu.Lock()
	defer f.registry.mu.Unlock()
	return f.registry.sessions[probeID]
}

// workflowGate 白盒读取 transition gate 与全局续约器状态。
func (f *registryFixture) workflowGate() (transition bool, renewerRunning bool) {
	f.registry.mu.Lock()
	defer f.registry.mu.Unlock()
	return f.registry.workflowTransition, f.registry.workflowRenewCancel != nil
}

// sessionStateOf 白盒读取 session 状态（不存在时返回 completed 语义外的指示）。
func (f *registryFixture) sessionStateOf(probeID ProbeID) (sessionState, bool) {
	f.registry.mu.Lock()
	defer f.registry.mu.Unlock()
	session, ok := f.registry.sessions[probeID]
	if !ok {
		return sessionStateCompleted, false
	}
	return session.state, true
}

// dualConfigJSON 构造含控制器绑定的配置 JSON（每控制器一个轴）。
func dualConfigJSON(clientTaskID string, controllerIDs ...string) json.RawMessage {
	axes := ""
	axisNames := []string{"X", "Y", "Z", "U"}
	for i, id := range controllerIDs {
		if i > 0 {
			axes += ","
		}
		axes += fmt.Sprintf(`{"controllerId":%q,"axis":%q}`, id, axisNames[i%len(axisNames)])
	}
	return json.RawMessage(fmt.Sprintf(`{"taskId":%q,"deviceId":"dev-1","channels":[0],"motionAxes":[%s]}`, clientTaskID, axes))
}

// dualConfigWithAxisJSON 构造只含单个 (controllerID, axis) 元组的配置 JSON。
// 用于同控制器不同轴场景：两 probe 共用同一控制器但绑定不同物理轴时，
// 需要精确指定每个 probe 的 axis（dualConfigJSON 按 X/Y/Z/U 顺序自动派生无法表达此场景）。
func dualConfigWithAxisJSON(clientTaskID, controllerID, axis string) json.RawMessage {
	return json.RawMessage(fmt.Sprintf(
		`{"taskId":%q,"deviceId":"dev-1","channels":[0],"motionAxes":[{"controllerId":%q,"axis":%q}]}`,
		clientTaskID, controllerID, axis,
	))
}

// seedPersistedBindings 写入 probe-scoped 持久化配置。
func (f *registryFixture) seedPersistedBindings(probeID ProbeID, controllerIDs ...string) {
	f.configs.seed(probeConfigKey(probeID), dualConfigJSON("", controllerIDs...))
}

// seedPersistedAxisBinding 写入 probe-scoped 持久化配置（单个 (controllerID, axis) 元组）。
// 用于同控制器不同轴场景的持久化绑定种子。
func (f *registryFixture) seedPersistedAxisBinding(probeID ProbeID, controllerID, axis string) {
	f.configs.seed(probeConfigKey(probeID), dualConfigWithAxisJSON("", controllerID, axis))
}

// ---------------------------------------------------------------------------
// Task 3: GetOrCreate
// ---------------------------------------------------------------------------

func TestManagerRegistry_GetOrCreate_InvalidProbeID(t *testing.T) {
	fx := newRegistryFixture(t)
	for _, id := range []ProbeID{"", "probe3", "PROBE1"} {
		if _, err := fx.registry.GetOrCreate(id); !errors.Is(err, ErrInvalidProbeID) {
			t.Fatalf("probeID %q 应返回 ErrInvalidProbeID, got %v", id, err)
		}
	}
	if fx.factory.callCount() != 0 {
		t.Fatal("非法 probeID 不得触发 factory")
	}
}

func TestManagerRegistry_GetOrCreate_ClosingRejected(t *testing.T) {
	fx := newRegistryFixture(t)
	fx.setClosing()
	if _, err := fx.registry.GetOrCreate(Probe1); !errors.Is(err, ErrRegistryClosing) {
		t.Fatalf("closing 时应返回 ErrRegistryClosing, got %v", err)
	}
	if fx.factory.callCount() != 0 {
		t.Fatal("closing 时不得触发 factory")
	}
}

func TestManagerRegistry_GetOrCreate_ConcurrentSingleFactoryCall(t *testing.T) {
	fx := newRegistryFixture(t)
	entered := make(chan struct{})
	release := make(chan struct{})
	var once sync.Once
	shared := &fakeManagedManager{}
	fx.factory.setCreateHook(func(ProbeID) (ManagedTraversalManager, error) {
		once.Do(func() { close(entered) })
		<-release
		return shared, nil
	})

	const goroutines = 8
	var ready sync.WaitGroup
	ready.Add(goroutines)
	type outcome struct {
		manager ManagedTraversalManager
		err     error
	}
	results := make(chan outcome, goroutines)
	for g := 0; g < goroutines; g++ {
		go func() {
			ready.Done()
			manager, err := fx.registry.GetOrCreate(Probe1)
			results <- outcome{manager, err}
		}()
	}
	ready.Wait()
	<-entered // leader 已进入 factory（gate 已建立，其余调用成为等待者）
	close(release)
	for g := 0; g < goroutines; g++ {
		result := <-results
		if result.err != nil {
			t.Fatalf("GetOrCreate: %v", result.err)
		}
		if result.manager != shared {
			t.Fatal("并发 GetOrCreate 应共享同一 manager 实例")
		}
	}
	if calls := fx.factory.callCount(); calls != 1 {
		t.Fatalf("factory 应只调用一次, got %d", calls)
	}
	// 后续调用命中 map，不再触发 factory
	if _, err := fx.registry.GetOrCreate(Probe1); err != nil {
		t.Fatalf("命中缓存: %v", err)
	}
	if calls := fx.factory.callCount(); calls != 1 {
		t.Fatalf("命中缓存后 factory 不应再调用, got %d", calls)
	}
}

func TestManagerRegistry_GetOrCreate_FactoryFailureWakesWaitersAndRetry(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		fx := newRegistryFixture(t)
		boom := errors.New("factory boom")
		release := make(chan struct{})
		var fail atomic.Bool
		fail.Store(true)
		fx.factory.setCreateHook(func(ProbeID) (ManagedTraversalManager, error) {
			<-release
			if fail.Load() {
				return nil, boom
			}
			return &fakeManagedManager{}, nil
		})

		const goroutines = 6
		errs := make(chan error, goroutines)
		for g := 0; g < goroutines; g++ {
			go func() {
				_, err := fx.registry.GetOrCreate(Probe2)
				errs <- err
			}()
		}
		// 等待 bubble 内全部 goroutine 阻塞：leader 卡在 factory，其余成为 gate 等待者。
		// 此后再放行 factory，保证恰好一次 in-flight factory 调用（不依赖 time.Sleep）。
		synctest.Wait()
		close(release)
		for g := 0; g < goroutines; g++ {
			if err := <-errs; !errors.Is(err, boom) {
				t.Fatalf("所有等待者应收到同一 factory 错误, got %v", err)
			}
		}
		if calls := fx.factory.callCount(); calls != 1 {
			t.Fatalf("失败时 factory 应只调用一次, got %d", calls)
		}
		// 失败不污染 map，修复后重试成功
		fail.Store(false)
		if _, err := fx.registry.GetOrCreate(Probe2); err != nil {
			t.Fatalf("重试应成功: %v", err)
		}
		if calls := fx.factory.callCount(); calls != 2 {
			t.Fatalf("重试应再次调用 factory, got %d", calls)
		}
	})
}

func TestManagerRegistry_GetOrCreate_FactoryRunsOutsideMutex(t *testing.T) {
	fx := newRegistryFixture(t)
	fx.factory.setCreateHook(func(ProbeID) (ManagedTraversalManager, error) {
		// factory 执行期间 registry mutex 必须已释放，否则 TryLock 失败
		if !fx.registry.mu.TryLock() {
			return nil, errors.New("factory 在 registry mutex 持锁状态下被调用")
		}
		fx.registry.mu.Unlock()
		return &fakeManagedManager{}, nil
	})
	if _, err := fx.registry.GetOrCreate(Probe1); err != nil {
		t.Fatalf("GetOrCreate: %v", err)
	}
}

// fakeMotionAccess 实现 ports.MotionAccess 的测试桩（Shutdown EmergencyStop 用）。
type fakeMotionAccess struct {
	mu      sync.Mutex
	esCalls map[string]int
	// esBlock 按 controllerID 注入阻塞：值为 nil channel 时永久阻塞（stuck adapter），
	// 为 channel 时阻塞至关闭；esRespectCtx=true 时阻塞同时响应 ctx.Done。
	esBlock      map[string]chan struct{}
	esRespectCtx bool
	// esEntered ES 进入通知（并发性断言用，缓冲通道按 controllerID 投递）。
	esEntered chan<- string
}

func newFakeMotionAccess() *fakeMotionAccess {
	return &fakeMotionAccess{esCalls: make(map[string]int), esBlock: make(map[string]chan struct{})}
}

func (m *fakeMotionAccess) StatusAll(context.Context) []motion.ControllerStatus { return nil }

func (m *fakeMotionAccess) MoveTo(context.Context, string, motion.AxisName, float64) error {
	return nil
}

func (m *fakeMotionAccess) Stop(context.Context, string, motion.AxisName) error { return nil }

func (m *fakeMotionAccess) EmergencyStop(ctx context.Context, id string) error {
	m.mu.Lock()
	m.esCalls[id]++
	block := m.esBlock[id]
	respectCtx := m.esRespectCtx
	entered := m.esEntered
	m.mu.Unlock()
	if entered != nil {
		entered <- id
	}
	if block == nil {
		return nil
	}
	if respectCtx {
		select {
		case <-block:
		case <-ctx.Done():
			return ctx.Err()
		}
		return nil
	}
	<-block
	return nil
}

func (m *fakeMotionAccess) esCallCount(id string) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.esCalls[id]
}

// setESBlock 注入按控制器的阻塞式 EmergencyStop（返回放行函数）。
func (m *fakeMotionAccess) setESBlock(controllerID string) (unblock func()) {
	m.mu.Lock()
	defer m.mu.Unlock()
	ch := make(chan struct{})
	m.esBlock[controllerID] = ch
	return func() { close(ch) }
}

// fakeCheckpointStore 内存版 ports.CheckpointStore（Task 11 恢复 façade 用）。
type fakeCheckpointStore struct {
	mu   sync.Mutex
	data map[string][]byte
}

func newFakeCheckpointStore() *fakeCheckpointStore {
	return &fakeCheckpointStore{data: make(map[string][]byte)}
}

func (s *fakeCheckpointStore) Stat(path string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.data[path]
	return ok, nil
}

func (s *fakeCheckpointStore) Read(path string) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	data, ok := s.data[path]
	if !ok {
		return nil, os.ErrNotExist
	}
	return append([]byte(nil), data...), nil
}

func (s *fakeCheckpointStore) Write(path string, data []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data[path] = append([]byte(nil), data...)
	return nil
}

func (s *fakeCheckpointStore) Remove(path string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.data, path)
	return nil
}
