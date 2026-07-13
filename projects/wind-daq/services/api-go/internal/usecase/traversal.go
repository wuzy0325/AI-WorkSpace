// Package usecase 实现风洞 DAQ 的核心业务用例。
//
// 本文件 traversal.go 是 TraversalManager（遍历测试编排器）的主入口：
//   - 状态机：Idle / Running / Paused / Stopped / Error，外加 moving / stabilizing /
//     acquiring / saving 等运行中子状态
//   - 生命周期：Start → RunTraversalLoop（每点调用 RunCurrentPoint）→ Stop / finalizeSink
//   - 资源协调：通过 resourcelock.Default() 与 calibration 等其他工作流互斥
//
// 拆分到同包其他文件：
//   - traversal_acquisition.go：单点采集主流程（移动→稳定→采样→保存）
//   - traversal_checkpoint.go：断点保存/恢复
//   - traversal_config.go：API 入口与配置持久化
//   - traversal_helpers.go：内部工具函数（错误流式构造、任务取消轮询等）
//   - traversal_view.go：状态/历史结果到 API 响应的映射
package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	coreinterp "ai-workspace/shared/algorithms/go/fivehole/interpolation"
	"wind-daq/services/api-go/internal/core/motion"
	"wind-daq/services/api-go/internal/core/realtime"
	"wind-daq/services/api-go/internal/core/resourcelock"
	"wind-daq/services/api-go/internal/core/traversal"
	"wind-daq/services/api-go/internal/ports"
)

// traversalLockResource 工作流级互斥锁的资源名，与 Cursor DAQ 保持一致
const traversalLockResource = "workflow:traversal"

// 运动完成等待超时和轮询间隔
// 使用 time.Duration 类型以避免 time.NewTicker / time.Now().Add 误把裸数当 ns 使用
const (
	motionCompleteTimeout   = 120 * time.Second      // 单点运动到位最大等待
	motionCompletePoll      = 100 * time.Millisecond // 运动到位轮询间隔
	acquisitionBatchTimeout = 2 * time.Second        // 多次采样总体超时
	acquisitionBatchPoll    = 10 * time.Millisecond  // 采样间隔
	cancelCheckPoll         = 100 * time.Millisecond // 任务取消检查间隔
	pausedLoopIdle          = 200 * time.Millisecond // 暂停态主循环空转间隔
	retryWaitInterval       = 200 * time.Millisecond // 数据校验失败后重试等待间隔（让设备产出新帧）
	checkpointInterval      = 10                     // 每完成10个点保存一次断点
)

type TraversalRunSession struct {
	taskID   string
	snapshot traversal.TraversalRunSnapshot
	ctx      context.Context
	cancel   context.CancelFunc
	done     chan struct{}
	doneOnce sync.Once
}

func newTraversalRunSession(parent context.Context, taskID string, snapshot traversal.TraversalRunSnapshot) *TraversalRunSession {
	ctx, cancel := context.WithCancel(parent)
	return &TraversalRunSession{taskID: taskID, snapshot: snapshot, ctx: ctx, cancel: cancel, done: make(chan struct{})}
}

func (s *TraversalRunSession) Context() context.Context { return s.ctx }
func (s *TraversalRunSession) Done() <-chan struct{}    { return s.done }
func (s *TraversalRunSession) Cancel()                  { s.cancel() }
func (s *TraversalRunSession) MarkDone()                { s.doneOnce.Do(func() { close(s.done) }) }

func (s *TraversalRunSession) IsDone() bool {
	select {
	case <-s.done:
		return true
	default:
		return false
	}
}

type TraversalManager struct {
	mu              sync.RWMutex
	session         *TraversalRunSession
	reader          ports.LatestDataReader
	motion          ports.MotionAccess
	sink            ports.TraversalPointSink
	store           ports.TraversalResultStore
	checkpointStore ports.CheckpointStore
	configStore     ports.AppConfigStore // 遍历配置持久化存储
	config          traversal.Config
	status          traversal.Status
	configRaw       json.RawMessage
	interpolator    coreinterp.Interpolator

	// finalizeOnce 确保 finalizeSink 只执行一次实际关闭逻辑。
	// Stop 5s 超时路径与 RunTraversalLoop 的 defer 可能并发调用 finalizeSink，
	// sync.Once 保证端口 Close 不会重入。每次 Start/Resume 时通过 resetFinalizeOnce
	// 重置，让新任务可重新触发关闭流程。
	finalizeOnce sync.Once

	// 实时插值缓存（量化键 + LRU + 容差匹配，对应 Cursor DAQ InterpolationCache）
	interpCache *realtime.InterpolationCache

	// 断点恢复
	lastCheckpointPath string

	// 暂停/停止控制
	isStopped bool
	isPaused  bool

	// 数据验证配置
	validation *traversal.DataValidationConfig

	// 稳定等待配置
	stabilization *traversal.StabilizationConfig

	// 启动时根据持久化配置恢复插值器的最后一次错误（若有），
	// 用于在 CheckPreconditions 中向前端暴露真实失败原因，
	// 避免前端基于配置 JSON 错误推断为"已加载"。
	lastInterpolatorRestoreErr string

	// 插值器加载端口（用于启动恢复时按路径加载 PRB / CSV / 多 PRB），
	// nil 表示未注入加载器，启动恢复将被跳过并写入相应错误消息。
	interpLoader ports.InterpolatorLoader

	// 通道单位提供端口：BuildRawPressure 归一化时按 (deviceID, channelIndex)
	// 查询通道 Unit 才能换算到 Pa。nil 表示未注入，归一化走降级路径
	//（跳过换算并记 warning，保证旧测试不崩）。
	// 由 DeviceManager 实现，装配点通过 SetUnitProvider 注入。
	unitProvider ports.ChannelUnitProvider

	// 设备采集控制端口：用于 CheckPreconditions 真实校验目标设备已连接/正在采集，
	// 并在 ParseAndStartTraversal 启动 loop 之前主动拉起采集，避免"假绿 → no data"。
	// nil 表示未注入，前置检查与主动启动逻辑均走降级路径（保持旧装配向后兼容）。
	// 由 DeviceManager 实现，装配点通过 SetAcquisitionController 注入。
	acquisitionController ports.AcquisitionController

	// v2 可靠存储端口（Task 4-8）：三阶段提交与崩溃恢复所需
	// csvPort      CSV 落盘端口（支持新建/恢复、Sync、截断）
	// resultLogPort JSONL 完整结果日志端口（支持 AppendPrepared、Sync、ValidateTail）
	// checkpointPortFactory 结构化断点端口工厂（按 SavePath 动态创建，支持 Save/Load/Find/Unregister）
	// activeIndex   活动任务索引（taskId → checkpointPath），支持进程重启发现
	csvPort               ports.TraversalCSVPort
	resultLogPort         ports.TraversalResultLogPort
	checkpointPort        ports.TraversalCheckpointPort // 当前活动任务的断点端口（Start 时动态创建）
	checkpointPortFactory ports.TraversalCheckpointPortFactory
	activeIndex           ports.TraversalActiveIndex
}

// 遍历配置持久化存储的 key
const traversalConfigKey = "traversal"

func NewTraversalManager(reader ports.LatestDataReader, motion ports.MotionAccess, sink ports.TraversalPointSink, store ports.TraversalResultStore, checkpointStore ports.CheckpointStore, configStore ...ports.AppConfigStore) *TraversalManager {
	mgr := &TraversalManager{
		reader:          reader,
		motion:          motion,
		sink:            sink,
		store:           store,
		checkpointStore: checkpointStore,
		status:          traversal.Status{State: traversal.StateIdle},
		// 默认缓存：256 条，容差 1 Pa（与 Cursor DAQ 默认一致）
		interpCache: realtime.NewInterpolationCache(256, 1.0),
	}
	// 可选：注入持久化存储，启动时自动加载已保存的配置
	if len(configStore) > 0 && configStore[0] != nil {
		mgr.configStore = configStore[0]
		mgr.loadPersistedConfig()
	}
	slog.Info("traversal manager initialized",
		"component", "traversal",
		"has_reader", reader != nil,
		"has_motion", motion != nil,
		"has_sink", sink != nil,
		"has_store", store != nil,
		"has_checkpoint_store", checkpointStore != nil,
	)
	return mgr
}

// loadPersistedConfig 启动时从磁盘加载已保存的遍历配置
// 同时尝试根据上一次的 SavePath 推断 checkpoint 路径并回填 lastCheckpointPath，
// 修复"应用重启后 LoadCheckpoint 永远返回 nil"的问题。
func (m *TraversalManager) GenerateGridPath(config traversal.GridConfig) ([]traversal.Point, error) {
	return traversal.GenerateGridPath(config)
}

func (m *TraversalManager) SaveConfigRaw(config json.RawMessage) {
	m.mu.Lock()
	m.configRaw = append(json.RawMessage(nil), config...)
	m.mu.Unlock()
	// 持久化到磁盘，确保重启后配置不丢失
	if m.configStore != nil {
		_ = m.configStore.SaveConfig(traversalConfigKey, []byte(config))
	}
}

func (m *TraversalManager) GetConfigRaw() json.RawMessage {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return append(json.RawMessage(nil), m.configRaw...)
}

// SetInterpolator 注入插值器；切换插值器时清空缓存（避免旧结果污染新算法）
func (m *TraversalManager) SetInterpolator(interpolator coreinterp.Interpolator) {
	m.mu.Lock()
	m.interpolator = interpolator
	if m.interpCache != nil {
		m.interpCache.Clear()
	}
	// 一旦插值器被显式重置（包括用户主动重新加载 PRB），
	// 启动恢复的陈旧错误就不再适用，需要一并清除。
	m.lastInterpolatorRestoreErr = ""
	m.mu.Unlock()
}

// SetInterpolatorLoader 注入插值器加载端口。
// 该方法在装配阶段（apiserver / bootstrap / appcontext）调用一次；
// 不需要在运行期切换，因此不清理任何状态。
func (m *TraversalManager) SetInterpolatorLoader(loader ports.InterpolatorLoader) {
	m.mu.Lock()
	m.interpLoader = loader
	m.mu.Unlock()
}

// SetUnitProvider 注入通道单位提供端口，供 BuildRawPressure 归一化时
// 按 (deviceID, channelIndex) 查询通道 Unit。
//
// 为什么不在构造函数追加可变参数：
//   - Go 不允许两个 variadic 参数（configStore 已占用 variadic 槽位）；
//   - 装配阶段 DeviceManager 创建顺序在 TraversalManager 之后，构造期注入
//     无法表达"先创建 mgr 再创建 manager 再回填"的依赖；
//   - 与 SetInterpolatorLoader 同模式（setter 注入），保持装配风格一致。
//
// 调用时机：装配根创建 DeviceManager 之后立即调用，确保遍历启动时
// unitProvider 字段非 nil，归一化路径生效。
func (m *TraversalManager) SetUnitProvider(provider ports.ChannelUnitProvider) {
	m.mu.Lock()
	m.unitProvider = provider
	m.mu.Unlock()
}

// SetAcquisitionController 注入设备采集控制端口，供遍历启动前
//   - CheckPreconditions 真实校验目标设备已连接 / 正在采集；
//   - ParseAndStartTraversal 在启动 loop 之前主动拉起采集。
//
// 与 SetUnitProvider 同模式（setter 注入）：DeviceManager 创建顺序在
// TraversalManager 之后，构造期无法表达"先创建 mgr 再回填"的依赖。
//
// 调用时机：装配根创建 DeviceManager 之后立即调用，与 SetUnitProvider
// 紧邻；未注入时所有调用方走降级路径（保持旧装配向后兼容）。
func (m *TraversalManager) SetAcquisitionController(controller ports.AcquisitionController) {
	m.mu.Lock()
	m.acquisitionController = controller
	m.mu.Unlock()
}

// SetCsvPort 注入遍历 CSV 端口（v2 可靠存储）。
// 支持新建/恢复模式、行级摘要、尾部截断与 Sync。
// 装配阶段调用；若未注入，遍历仍可通过旧 sink 路径落盘，但无崩溃恢复能力。
func (m *TraversalManager) SetCsvPort(port ports.TraversalCSVPort) {
	m.mu.Lock()
	m.csvPort = port
	m.mu.Unlock()
}

// SetResultLogPort 注入 JSONL 完整结果日志端口（v2 可靠存储）。
// 用于三阶段提交中的结果日志阶段，作为崩溃恢复的权威数据源。
// 装配阶段调用；若未注入，遍历仍运行但无可靠恢复能力。
func (m *TraversalManager) SetResultLogPort(port ports.TraversalResultLogPort) {
	m.mu.Lock()
	m.resultLogPort = port
	m.mu.Unlock()
}

// SetCheckpointPortFactory 注入断点端口工厂（v2 可靠存储）。
// 由于每个任务的 SavePath 不同，断点文件路径需在 Start 时按 SavePath 确定。
// 工厂在 Start 时被调用，为当前任务创建专属的 checkpointPort。
// 装配阶段调用。
func (m *TraversalManager) SetCheckpointPortFactory(factory ports.TraversalCheckpointPortFactory) {
	m.mu.Lock()
	m.checkpointPortFactory = factory
	m.mu.Unlock()
}

// SetActiveIndex 注入活动任务索引（v2 可靠存储）。
// 用于启动时发现未完成的遍历任务，支持断点续跑。
// 装配阶段调用。
func (m *TraversalManager) SetActiveIndex(index ports.TraversalActiveIndex) {
	m.mu.Lock()
	m.activeIndex = index
	m.mu.Unlock()
}

// InterpolatorRestoreErr 返回最近一次启动恢复 / 显式 RestoreInterpolatorFromPersistedConfig
// 调用所遗留的错误消息，主要供测试与 CheckPreconditions 读取。空字符串表示无错误。
func (m *TraversalManager) InterpolatorRestoreErr() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.lastInterpolatorRestoreErr
}

// Interpolator 返回当前注入的插值器（可能为 nil）；测试代码通过该方法断言
// SetInterpolator 是否被调用，避免直接访问私有字段。
func (m *TraversalManager) Interpolator() coreinterp.Interpolator {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.interpolator
}

// SetValidation 设置数据验证配置
func (m *TraversalManager) SetValidation(config *traversal.DataValidationConfig) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.validation = config
}

// SetStabilization 设置稳定等待配置
func (m *TraversalManager) SetStabilization(config *traversal.StabilizationConfig) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.stabilization = config
}

// CheckPreconditions 校验遍历测试启动前的前置条件。
// 若传入待启动配置（启动确认对话框场景），则基于该配置中的 ChannelLabels 校验 Patm/Tatm；
// 若传入 nil，则回退到当前 manager 持有的 m.config（如 verifyInterpolatorWithBackend 场景）。
//
// 设备采集态校验（acquisitionController 非 nil 时启用）：
//   - DeviceConnected：目标设备是否已连接；
//   - DeviceAcquiring：目标设备是否正在采集。
//
// 这两项替代了旧版"DAQ hub 是否注入"对运行态的失真反映——hub 注入只代表基础设施
// 就绪，不代表目标设备正在持续产帧。新增两项让"全部通过"真实可信赖。
// acquisitionController 为 nil（旧装配）时保持原 4 项检查不变，向后兼容。
//
// 运动控制器连接态校验（m.motion 非 nil 时启用）：
//   - Motion 项 passed 同时要求"端口已注入"且"至少有一个 Connected=true 的控制器"。
//   - 遍历测试运行时不绑定特定 motion ID，按 StatusAll 遍历所有已连接控制器调度，
//     故"目标控制器"语义为"至少一个已连接"。仅查 m.motion != nil 会让"端口注入但
//     实际全部离线"的配置通过前置检查（假绿），到 RunCurrentPoint 才发现无可用控制器。
func (m *TraversalManager) CheckPreconditions(config *traversal.Config) map[string]any {
	hasInterpolator := m.HasLoadedInterpolator()
	hasMotion := m.motion != nil
	hasReader := m.reader != nil

	// PRB 项默认消息；若启动恢复时记录了失败原因，则使用真实原因，
	// 便于前端在 PRB 文件被删除/移动等情况下直接展示根因。
	prbMessage := "Load PRB or calibration CSV before running interpolation"
	// ChannelMap 校验：扫描 ChannelLabels 是否包含 Patm + Tatm 标签。
	// P1-P5 标签存在性由 BuildRawPressure 返回的 ok 标志在运行时承担，
	// 此处只做前置硬性校验——Patm/Tatm 缺失会让归一化与大气计算无法进行。
	hasPatm := false
	hasTatm := false
	m.mu.RLock()
	if !hasInterpolator && m.lastInterpolatorRestoreErr != "" {
		prbMessage = m.lastInterpolatorRestoreErr
	}
	cfg := m.config
	if config != nil {
		cfg = *config
	}
	for _, label := range cfg.ChannelLabels {
		switch label {
		case "Patm":
			hasPatm = true
		case "Tatm":
			hasTatm = true
		}
	}
	// acquisitionController 在锁内取一次：与 SetAcquisitionController 写入互斥，
	// 同时避免锁外单独再读 m.acquisitionController 产生数据竞争。
	acqController := m.acquisitionController
	m.mu.RUnlock()

	channelMapPassed := hasPatm && hasTatm
	channelMapMessage := "All required channel labels are mapped"
	if !hasPatm {
		channelMapMessage = "Patm channel label is required for pressure normalization"
	} else if !hasTatm {
		channelMapMessage = "Tatm channel label is required for atmospheric calculation"
	}

	// 设备采集态校验：仅当端口注入时启用，避免旧装配回归。
	// DeviceAcquiring 项文案区分"未连接"与"未采集"两种失败，
	// 让用户在确认对话框中能直接看到根因而非反推。
	//
	// 为什么锁外调用 IsConnected/IsAcquiring：
	//   acqController 实现为 DeviceManager.GetStatus，内部持 DeviceManager.mu.RLock。
	//   若在 TraversalManager.mu 持锁期间调用，会形成"TraversalManager.mu → DeviceManager.mu"
	//   嵌套锁顺序。虽然当前 DeviceManager 不会回调 TraversalManager，但持锁调用外部接口
	//   是反模式——接口实现若耗时（如未来扩展为网络查询）会阻塞 TraversalManager 的其他操作。
	//   故有意锁外调用，acqController 局部变量已是接口指针的原子快照，无数据竞争。
	deviceConnected := true
	deviceConnectedMsg := "Target device is connected"
	deviceAcquiring := true
	deviceAcquiringMsg := "Target device is acquiring"
	if acqController != nil {
		deviceConnected = acqController.IsConnected(cfg.DeviceID)
		if !deviceConnected {
			deviceConnectedMsg = "Target device is not connected, please connect it first"
		}
		deviceAcquiring = acqController.IsAcquiring(cfg.DeviceID)
		if !deviceAcquiring {
			// 未在采集时给出"将自动启动"的提示，与 ParseAndStartTraversal 主动启动逻辑呼应，
			// 让用户在确认对话框预知行为而非看到隐式自动操作。
			deviceAcquiringMsg = "Target device is not acquiring (will auto-start on confirm)"
		}
	}

	// 运动控制器连接态校验：仅当端口注入时启用，避免旧装配回归。
	// 与 DeviceConnected 同理——仅查端口注入会让"已装配但全部离线"的配置假绿通过。
	// StatusAll 加 2s 超时保护：前置检查在用户点击"开始"后立即触发，UI 响应敏感；
	// 若实现内部 TCP 超时（典型 5~30s）会让确认对话框长时间无响应。
	// ctx.WithTimeout 在 StatusAll 实现支持 ctx 取消时才生效，但即使不支持，
	// 也不会比现状更差——RunCurrentPoint 同样以 ctx.Background 调用 StatusAll。
	motionConnected := true
	motionMessage := "Motion manager is available"
	if hasMotion {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		statuses := m.motion.StatusAll(ctx)
		cancel()
		motionConnected = false
		for _, s := range statuses {
			if s.Connected {
				motionConnected = true
				break
			}
		}
		if !motionConnected {
			motionMessage = "No motion controller is connected, please connect one first"
		}
	}

	checks := []map[string]any{
		{"name": "PRB", "passed": hasInterpolator, "message": prbMessage},
		{"name": "Motion", "passed": hasMotion && motionConnected, "message": motionMessage},
		{"name": "DAQ", "passed": hasReader, "message": "DAQ acquisition hub is available"},
		{"name": "ChannelMap", "passed": channelMapPassed, "message": channelMapMessage},
	}
	if acqController != nil {
		checks = append(checks,
			map[string]any{"name": "DeviceConnected", "passed": deviceConnected, "message": deviceConnectedMsg},
			map[string]any{"name": "DeviceAcquiring", "passed": deviceAcquiring, "message": deviceAcquiringMsg},
		)
	}
	// DeviceAcquiring 不纳入 allPassed：设备未采集时，ParseAndStartTraversal
	// 会在启动 loop 之前主动调用 StartAcquisition 拉起采集。该项仅用于提示用户
	// "将自动启动"，而非阻止开始测试。DeviceConnected 仍必须纳入 allPassed，
	// 因为未连接时无法通过任何自动操作恢复。
	// Motion 项 passed 同时纳入"端口注入"与"已连接"两个条件：未连接的运动控制器
	// 无法通过自动操作恢复（与 DeviceAcquiring 不同），必须阻塞用户开始测试。
	allPassed := hasInterpolator && hasMotion && motionConnected && hasReader && channelMapPassed
	if acqController != nil {
		allPassed = allPassed && deviceConnected
	}
	return map[string]any{"allPassed": allPassed, "checks": checks}
}

// CalculateRealtime 实时插值：先按 Config.InterpolationMode 切换 MultiPRB 模式，
// 再走"缓存 → 计算 → 写回"路径，对应 Cursor DAQ OptimizedRealtimeInterpolator。
func (m *TraversalManager) CalculateRealtime(input coreinterp.InterpolationInput) (coreinterp.InterpolationResult, error) {
	m.mu.RLock()
	interpolator := m.interpolator
	cache := m.interpCache
	mode := m.config.InterpolationMode
	m.mu.RUnlock()
	if interpolator == nil || !interpolator.IsLoaded() {
		return coreinterp.InterpolationResult{}, fmt.Errorf("PRB interpolation data is not loaded")
	}

	// 仅 MultiPrbInterpolator 暴露 SetInterpolationMode；通过类型断言切换
	if mode != "" {
		if multi, ok := interpolator.(interface {
			SetInterpolationMode(coreinterp.MultiPrbInterpolationMode)
		}); ok {
			multi.SetInterpolationMode(coreinterp.MultiPrbInterpolationMode(mode))
		}
	}

	// 缓存命中直接返回
	if cache != nil {
		if cached, hit := cache.Find(input); hit {
			return cached, nil
		}
	}
	// 未命中：计算并写回
	res, err := interpolator.Calculate(input)
	if err == nil && res.IsValid && cache != nil {
		cache.Store(input, res)
	}
	return res, err
}

func (m *TraversalManager) HasLoadedInterpolator() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.interpolator != nil && m.interpolator.IsLoaded()
}

func (m *TraversalManager) beginSession(parent context.Context, taskID string, snapshot traversal.TraversalRunSnapshot) (*TraversalRunSession, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.session != nil && !m.session.IsDone() {
		return nil, fmt.Errorf("traversal session %s is still active", m.session.taskID)
	}
	m.session = newTraversalRunSession(parent, taskID, snapshot)
	return m.session, nil
}

// resetFinalizeOnce 重置 finalizeOnce，让下一次 Start/ResumeFromCheckpoint 可重新触发
// finalizeSinkInternal。sync.Once 一旦执行就永久标记 done，必须在每次新任务开始前
// 通过整体赋值清空，否则新任务的 finalizeSink 会直接返回，端口不会被关闭。
//
// 调用时机：Start / ResumeFromCheckpoint 在 beginSession 成功之后、打开 v2 端口之前
// 调用，保证 finalizeOnce 与 session 生命周期严格对齐。
//
// 并发安全：整体赋值 sync.Once 持有 m.mu，与 finalizeSinkInternal 中的
// m.finalizeOnce.Do(...) 互斥。即使 Stop 5s 超时强制 finalizeSink 后用户立即调 Start，
// 也不会与 RunTraversalLoop 的 defer finalizeSink 并发赋值触发 race。
// 之前的实现不持锁，依赖"Start/Resume 入口互斥"上游不变量；但 Stop 超时路径会
// 在 RunTraversalLoop 仍在跑时强制 finalize，破坏该不变量。
func (m *TraversalManager) resetFinalizeOnce() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.finalizeOnce = sync.Once{}
}

// sinkIsCsvPort 检查 sink 是否与 csvPort 是同一实例。
//
// 装配根（bootstrap.go / context.go）将同一个 TraversalCsvWriter 同时注入为
// sink（ports.TraversalPointSink）和 csvPort（ports.TraversalCSVPort）。此时：
//   - csvPort.Open 已完成文件初始化，sink.InitializeTraversal 必须跳过，
//     否则触发双重初始化防御（P1-I6）返回错误；
//   - sink.WriteTraversalPoint 必须跳过，否则同一行被写两次；
//   - sink.FinalizeTraversal 必须跳过，避免重复关闭（虽然当前实现幂等，但语义混乱）。
//
// 当 csvPort 为 nil（旧装配 / 测试路径）时返回 false，sink 路径正常执行。
// 当 sink 与 csvPort 是不同实例时（理论上未使用的混合装配）返回 false，
// 两条路径并行执行，保持向后兼容。
func sinkIsCsvPort(sink ports.TraversalPointSink, csvPort ports.TraversalCSVPort) bool {
	if sink == nil || csvPort == nil {
		return false
	}
	csvPortAsSink, ok := csvPort.(ports.TraversalPointSink)
	if !ok {
		return false
	}
	return sink == csvPortAsSink
}

func (m *TraversalManager) Start(config traversal.Config) error {
	slog.Info("traversal starting",
		"component", "traversal",
		"task_id", config.TaskID,
		"device_id", config.DeviceID,
		"total_points", len(config.Path),
		"channels", config.Channels,
	)
	if config.TaskID == "" {
		err := fmt.Errorf("taskID is required")
		slog.Error("traversal start failed", "component", "traversal", "error", err)
		return err
	}
	if config.DeviceID == "" {
		err := fmt.Errorf("deviceID is required")
		slog.Error("traversal start failed", "component", "traversal", "task_id", config.TaskID, "error", err)
		return err
	}
	if len(config.Channels) == 0 {
		err := fmt.Errorf("channels are required")
		slog.Error("traversal start failed", "component", "traversal", "task_id", config.TaskID, "error", err)
		return err
	}
	if len(config.Path) == 0 {
		err := fmt.Errorf("path is required")
		slog.Error("traversal start failed", "component", "traversal", "task_id", config.TaskID, "error", err)
		return err
	}

	// v2：使用 beginSession 活动会话门禁，防止旧 taskId 污染新任务
	snapshot := traversal.TraversalRunSnapshot{
		Config:               config,
		Validation:           m.validation,
		Stabilization:        m.stabilization,
		InterpolatorIdentity: m.interpolatorIdentity(),
		SaveOptions:          config.SaveOptions,
		TotalPoints:          len(config.Path),
		CommittedPoints:      0,
		CommitSeq:            0,
		CSVPath:              traversal.ResolveOutputPath(config),
		ResultLogPath:        traversal.ResolveResultLogPath(config),
	}
	parentCtx := context.Background()
	session, err := m.beginSession(parentCtx, config.TaskID, snapshot)
	if err != nil {
		slog.Error("traversal start failed", "component", "traversal", "task_id", config.TaskID, "error", err)
		return err
	}
	// 重置 finalizeOnce：上一次任务结束（Stop/Complete）已消费 once，
	// 新任务必须重新武装 finalizeSink 才能在结束时正确关闭端口。
	m.resetFinalizeOnce()

	m.mu.Lock()
	if m.status.State == traversal.StateRunning || m.status.State == traversal.StatePaused {
		m.mu.Unlock()
		session.Cancel()
		session.MarkDone()
		err := fmt.Errorf("a traversal is already %s", m.status.State)
		slog.Error("traversal start failed", "component", "traversal", "task_id", config.TaskID, "error", err)
		return err
	}
	// 申请工作流级互斥锁（与 calibration 等其他工作流互斥）
	// TTL 给一个保守上限：单次遍历最多跑 24h；过期会被同名 holder 续约或外部接管
	if err := resourcelock.Default().Acquire(traversalLockResource, config.TaskID, 24*time.Hour); err != nil {
		m.mu.Unlock()
		session.Cancel()
		session.MarkDone()
		slog.Error("traversal start failed", "component", "traversal", "task_id", config.TaskID, "error", err)
		return fmt.Errorf("acquire traversal lock: %w", err)
	}
	m.config = config
	m.isStopped = false
	m.isPaused = false
	m.status = traversal.Status{
		TaskID:      config.TaskID,
		State:       traversal.StateRunning,
		TotalPoints: len(config.Path),
		StartedAt:   time.Now().UnixMilli(),
	}
	// 快照 v2 端口（可能为 nil，表示未注入 v2 组件）
	// csvPort 用于 sinkIsCsvPort 同实例检测；resultLogPort 由 openReliabilityPorts 内部读取，
	// 不在此处快照，避免未使用变量。
	csvPort := m.csvPort
	activeIndex := m.activeIndex
	checkpointPortFactory := m.checkpointPortFactory
	sink := m.sink
	m.mu.Unlock()

	// v2：通过工厂按解析后的 CSVPath 动态创建 checkpointPort。
	// 必须用 snapshot.CSVPath（= ResolveOutputPath(config)）而非 config.SavePath：
	//   - SavePath 可能是目录（如 "D:/data"），factory.Create 基于它派生 checkpoint 路径
	//     会落在 ".traversal/" 同目录下，但 SavePath 是目录时 Ext 为空，派生结果错乱。
	//   - ResumeFromCheckpoint 用 snapshot.CSVPath 创建 checkpointPort（见 traversal_checkpoint.go），
	//     Start 必须用同一 basePath 才能保证崩溃恢复链路一致。
	//   - activeIndex.Register 与 saveCheckpoint/loadCheckpoint 全部基于 snapshot.CSVPath 派生，
	//     形成单一真相源。
	var checkpointPort ports.TraversalCheckpointPort
	if checkpointPortFactory != nil && snapshot.CSVPath != "" {
		var cpErr error
		checkpointPort, cpErr = checkpointPortFactory.Create(session.ctx, snapshot.CSVPath)
		if cpErr != nil {
			// 工厂创建失败：回滚已 Begin 的 session 并释放工作流锁
			m.mu.Lock()
			m.checkpointPort = nil
			m.mu.Unlock()
			session.Cancel()
			session.MarkDone()
			_ = resourcelock.Default().Release(traversalLockResource, config.TaskID)
			slog.Error("traversal start failed: create checkpoint port",
				"component", "traversal", "task_id", config.TaskID, "error", cpErr)
			return fmt.Errorf("create checkpoint port: %w", cpErr)
		}
		m.mu.Lock()
		m.checkpointPort = checkpointPort
		m.mu.Unlock()
	}

	// v2 存储初始化：CSV 与结果日志（Create 模式，不截断）
	// 列配置通过 helper 内部从 config 复制到 session，确保 v2 表头与旧 sink 一致。
	// openReliabilityPorts 内部会在撞名 -2/-3 时回写 session.snapshot.CSVPath / ResultLogPath，
	// 调用方在函数返回后用 session.snapshot.CSVPath 即可拿到实际落盘路径。
	if err := m.openReliabilityPorts(session, ports.TraversalOutputCreate, config); err != nil {
		m.abortStartLocked(session, config.TaskID, err.Error(), traversal.ErrSaveFailed)
		return err
	}
	// 注册活动索引，支持进程重启发现。
	// checkpointPath 派生规则收敛到 ResolveCheckpointPathFromCSV 单一真相源，
	// 与 FileCheckpointPort.path() / saveCheckpoint / commitPointV2 fallback 保持一致。
	// 用 session.snapshot.CSVPath（撞名回写后的实际路径）派生，避免与实际 CSV stem 错位。
	if activeIndex != nil && checkpointPort != nil {
		checkpointPath := traversal.ResolveCheckpointPathFromCSV(session.snapshot.CSVPath)
		if err := activeIndex.Register(session.ctx, config.TaskID, checkpointPath); err != nil {
			slog.Warn("traversal active index register failed",
				"component", "traversal", "task_id", config.TaskID, "error", err)
			// 非阻塞：索引注册失败不影响任务启动，仅影响重启发现
		}
	}

	// 在锁外调用旧 sink.Initialize（向后兼容）。
	// sink 与 csvPort 是同一实例时跳过：csvPort.Open 已完成文件初始化，
	// 再次 InitializeTraversal 会触发双重初始化防御（P1-I6）返回错误。
	if sink != nil && !sinkIsCsvPort(sink, csvPort) {
		if err := sink.InitializeTraversal(config); err != nil {
			// 初始化失败：回滚状态并释放锁，避免半启动
			m.abortStartLocked(session, config.TaskID, fmt.Sprintf("sink init failed: %v", err), traversal.ErrSaveFailed)
			return err
		}
	}
	slog.Info("traversal started successfully",
		"component", "traversal",
		"task_id", config.TaskID,
		"device_id", config.DeviceID,
		"total_points", len(config.Path),
	)
	return nil
}

// openReliabilityPorts 打开 v2 可靠存储端口（CSV + 结果日志）。
//
// Create 模式：仅 Open，不截断。
// Resume 模式：Open 后对结果日志执行 ValidateTail + TruncateAfter，
//
//	丢弃崩溃前未 Sync 的半写入记录，水位严格对齐 CommittedSeq。
//	CSV 截断由 csvPort.Open 内部根据 CommittedSeq 完成（openResumeLocked）。
//
// 列配置（SaveOptions/Channels/ChannelLabels）从 config 复制到 session，
// 让 csvPort.Open 构建与 InitializeTraversal 一致的表头/labels，避免 v2 主路径
// 表头缺少通道列（Critical-3）。
//
// 撞名回写（Critical：v2 撞名 stem 错位修复）：
//   - csvPort.Open(Create) 在文件已存在时会自动加 -2/-3 后缀（openCreateUnique），
//     实际落盘路径可能与 session.snapshot.CSVPath 不同；
//   - Open 后必须用 csvPort.OutputPath() 拿真实路径回写 session.snapshot.CSVPath，
//     并调 checkpointPort.SetBasePath(actualCSVPath) 让后续 Save 派生路径与实际 CSV 一致；
//   - resultLogPort.Open 同理回写 session.snapshot.ResultLogPath。
//
// 调用方负责：beginSession + resetFinalizeOnce + Acquire(traversalLockResource)；
// 失败时调用 abortStartLocked 回滚已打开的端口。
func (m *TraversalManager) openReliabilityPorts(
	session *TraversalRunSession,
	mode ports.TraversalOutputMode,
	config traversal.Config,
) error {
	m.mu.RLock()
	csvPort := m.csvPort
	resultLogPort := m.resultLogPort
	checkpointPort := m.checkpointPort
	m.mu.RUnlock()

	// 从 session.snapshot 读取路径：openReliabilityPorts 内部回写后，
	// 调用方在函数返回后用 session.snapshot.CSVPath 即可拿到实际落盘路径。
	snapshot := session.snapshot

	if csvPort != nil {
		csvSession := ports.TraversalOutputSession{
			TaskID:        config.TaskID,
			Mode:          mode,
			Path:          snapshot.CSVPath,
			CommittedSeq:  snapshot.CommitSeq,
			SaveOptions:   config.SaveOptions,
			Channels:      config.Channels,
			ChannelLabels: config.ChannelLabels,
		}
		if err := csvPort.Open(session.ctx, csvSession); err != nil {
			return fmt.Errorf("csv open: %w", err)
		}
		// 撞名回写：csvPort.Open 实际落盘路径可能与传入的 snapshot.CSVPath 不同
		// （Create 模式撞名 -2/-3）。回写 session.snapshot.CSVPath 让后续
		// activeIndex.Register / saveCheckpoint / commitPointV2 fallback 派生的
		// checkpoint 路径与实际 CSV stem 一致，避免 Resume 打开错误 CSV 污染旧数据。
		actualCSVPath := csvPort.OutputPath()
		if actualCSVPath != "" && actualCSVPath != snapshot.CSVPath {
			session.snapshot.CSVPath = actualCSVPath
			// 同步更新 checkpointPort.basePath，让 Save/Load/Find/Unregister
			// 派生路径与新 CSV stem 一致。Resume 模式下撞名不会发生（Open 不创建文件），
			// 但 SetBasePath 仍是无副作用的幂等操作。
			if checkpointPort != nil {
				checkpointPort.SetBasePath(actualCSVPath)
			}
		}
	}
	if resultLogPort != nil {
		logSession := ports.TraversalOutputSession{
			TaskID:       config.TaskID,
			Mode:         mode,
			Path:         snapshot.ResultLogPath,
			CommittedSeq: snapshot.CommitSeq,
			// 结果日志是 JSONL（每行一个完整 PointResult），不消费列配置字段；
			// 仍填入是为了字段对称，便于未来扩展（如 header hash 校验）。
			SaveOptions:   config.SaveOptions,
			Channels:      config.Channels,
			ChannelLabels: config.ChannelLabels,
		}
		if err := resultLogPort.Open(session.ctx, logSession); err != nil {
			return fmt.Errorf("result log open: %w", err)
		}
		// 撞名回写：与 csvPort 同理，结果日志 Create 模式撞名时实际路径不同，
		// 回写 session.snapshot.ResultLogPath 保证后续恢复/状态展示一致。
		actualLogPath := resultLogPort.OutputPath()
		if actualLogPath != "" && actualLogPath != snapshot.ResultLogPath {
			session.snapshot.ResultLogPath = actualLogPath
		}
		// Resume 模式：Open 仅追加打开，不做水位对齐。必须显式 ValidateTail +
		// TruncateAfter，丢弃崩溃前未 Sync 的半写入记录，保证权威日志与
		// CommittedSeq 严格一致（Critical-2）。CSV 已在 Open 内部按 CommittedSeq 截断。
		if mode == ports.TraversalOutputResume {
			if err := resultLogPort.ValidateTail(session.ctx, snapshot.CommitSeq); err != nil {
				return fmt.Errorf("result log validate tail: %w", err)
			}
			if err := resultLogPort.TruncateAfter(session.ctx, snapshot.CommitSeq); err != nil {
				return fmt.Errorf("result log truncate: %w", err)
			}
		}
	}
	return nil
}

// abortStartLocked 启动失败时的统一回滚：设置错误状态、关闭已打开的 v2 端口、
// 关闭 session、释放工作流锁。
//
// 必须清理已打开的 v2 端口（csvPort/resultLogPort/checkpointPort），否则：
//   - csvPort.Open 已创建 CSV 文件并写入表头，未关闭会导致后续 Start 时 Open 失败
//     （"会话已打开"错误），半启动的文件句柄也会泄漏
//   - resultLogPort 同理，已打开的 JSONL 文件句柄不会被 GC 回收
//   - checkpointPort 由 factory.Create 动态创建，必须 Close 释放底层资源（与
//     finalizeSinkInternal 对称，避免换实现后句柄泄漏）
//
// 端口关闭顺序与 finalizeSink 一致：先结果日志后 CSV，确保刷盘顺序正确。
// 关闭失败仅记录日志，不影响错误状态传播（启动失败的根因优先暴露给调用方）。
func (m *TraversalManager) abortStartLocked(session *TraversalRunSession, taskID, message string, code traversal.ErrorCode) {
	m.mu.Lock()
	m.status.State = traversal.StateError
	m.status.LastError = message
	m.status.LastErrorCode = code
	csvPort := m.csvPort
	resultLogPort := m.resultLogPort
	checkpointPort := m.checkpointPort
	activeIndex := m.activeIndex
	m.checkpointPort = nil // 清理当前任务的断点端口引用，避免下一次 Start 复用错误实例
	m.mu.Unlock()
	session.Cancel()
	session.MarkDone()

	// 在锁外执行可能阻塞的端口关闭操作
	ctx := context.Background()
	if resultLogPort != nil {
		if err := resultLogPort.Close(ctx); err != nil {
			slog.Warn("traversal abort close result log failed",
				"component", "traversal", "task_id", taskID, "error", err)
		}
	}
	if csvPort != nil {
		if err := csvPort.Close(ctx); err != nil {
			slog.Warn("traversal abort close csv failed",
				"component", "traversal", "task_id", taskID, "error", err)
		}
	}
	// checkpointPort 由 factory.Create 动态创建，必须 Close 释放底层资源。
	// 与 finalizeSinkInternal 对称，避免未来切换到带句柄的实现后泄漏。
	if checkpointPort != nil {
		if err := checkpointPort.Close(ctx); err != nil {
			slog.Warn("traversal abort close checkpoint port failed",
				"component", "traversal", "task_id", taskID, "error", err)
		}
	}
	// 清理活动索引注册（启动时已 Register，失败必须 Unregister 否则下次启动会发现"幽灵任务"）
	if activeIndex != nil && checkpointPort != nil && taskID != "" {
		if err := activeIndex.Unregister(ctx, taskID); err != nil {
			slog.Warn("traversal abort unregister active index failed",
				"component", "traversal", "task_id", taskID, "error", err)
		}
	}
	_ = resourcelock.Default().Release(traversalLockResource, taskID)
	slog.Error("traversal start failed",
		"component", "traversal", "task_id", taskID,
		"error", message,
	)
}

// interpolatorIdentity 返回当前插值器的身份标识（用于快照）
func (m *TraversalManager) interpolatorIdentity() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.interpolator == nil {
		return ""
	}
	// 优先使用已实现 Identity() 的插值器
	if id, ok := m.interpolator.(interface{ Identity() string }); ok {
		return id.Identity()
	}
	return ""
}

func (m *TraversalManager) Status() traversal.Status {
	m.mu.RLock()
	defer m.mu.RUnlock()
	status := m.status
	status.Results = append([]traversal.PointResult(nil), m.status.Results...)
	if status.CurrentPoint >= 0 && status.CurrentPoint < len(m.config.Path) {
		point := m.config.Path[status.CurrentPoint]
		status.CurrentPointCoordinates = &point
	}
	return status
}

// RunCurrentPoint 执行当前测试点的完整流程（移动→稳定→采集→保存）
func (m *TraversalManager) Pause() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.status.State != traversal.StateRunning && !isSubState(m.status.State) {
		err := fmt.Errorf("traversal is not running")
		slog.Warn("traversal pause rejected",
			"component", "traversal",
			"task_id", m.config.TaskID,
			"state", m.status.State,
			"error", err,
		)
		return err
	}
	m.isPaused = true
	m.status.State = traversal.StatePaused
	slog.Info("traversal paused",
		"component", "traversal",
		"task_id", m.config.TaskID,
		"completed_points", m.status.CurrentPoint,
	)
	return nil
}

func (m *TraversalManager) Resume() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.status.State != traversal.StatePaused {
		err := fmt.Errorf("traversal is not paused")
		slog.Warn("traversal resume rejected",
			"component", "traversal",
			"task_id", m.config.TaskID,
			"state", m.status.State,
			"error", err,
		)
		return err
	}
	m.isPaused = false
	m.status.State = traversal.StateRunning
	slog.Info("traversal resumed",
		"component", "traversal",
		"task_id", m.config.TaskID,
		"completed_points", m.status.CurrentPoint,
	)
	return nil
}

// Stop 停止遍历测试。
//
// 端口关闭竞态修复（Fix 6）：
//   - 旧实现直接关闭 csvPort/resultLogPort/sink，与 RunCurrentPoint.commitPointV2
//     中的并发写入产生竞态（Close 期间 commitPointV2 可能写入已关闭的文件句柄）
//   - 新实现：Stop 只设置停止标志 + Cancel session，端口关闭集中到 finalizeSink
//     （由 RunTraversalLoop 的 defer 调用）
//   - Stop 等待 session.done 确保 RunTraversalLoop 已退出（commitPointV2 已完成），
//     超时 5s 保护避免 RunTraversalLoop 卡死导致 Stop 永久阻塞
//
// 错误聚合（Fix 7）：
//   - 旧实现 store.Save 失败时直接 return，导致后续清理（活动索引/锁）被跳过
//   - 新实现用 errors.Join 聚合 stopErr + store.Save 错误，所有清理都执行
func (m *TraversalManager) Stop() error {
	// 先设置停止标志，让运行中的循环能尽快感知
	m.mu.Lock()
	if m.isStopped {
		// 已停止，幂等返回（用户连续点 Stop 时避免重复清理）
		m.mu.Unlock()
		return nil
	}
	m.isStopped = true
	m.isPaused = false
	m.status.State = traversal.StateStopped
	// v2：取消活动会话，让 RunCurrentPoint 中的 ctx 检查立即失败
	session := m.session
	if session != nil {
		session.Cancel()
	}
	// 在持锁时快照 TaskID，避免 Resume/Start 并发改写 m.config 导致的数据竞争
	taskID := m.config.TaskID
	completedPoints := m.status.CurrentPoint
	m.mu.Unlock()

	slog.Info("traversal stopping",
		"component", "traversal",
		"task_id", taskID,
		"completed_points", completedPoints,
	)

	// 停止所有运动轴（在锁外执行，避免持锁调用外部接口）
	stopErr := m.stopMotionAxes()

	// 等待 RunTraversalLoop 退出：其 defer finalizeSink 会关闭所有端口，
	// 保证 commitPointV2 已完成，避免 Stop 与 commitPointV2 竞态。
	// 超时 5s 保护：RunTraversalLoop 可能卡在 motion.MoveTo 等不响应 ctx 的调用上。
	if session != nil && !session.IsDone() {
		select {
		case <-session.Done():
			// RunTraversalLoop 已退出，finalizeSink 已执行
		case <-time.After(5 * time.Second):
			slog.Warn("traversal stop timeout waiting for loop exit, force finalizing sink",
				"component", "traversal",
				"task_id", taskID,
			)
			// 超时强制关闭端口（finalizeSink 幂等，重复调用安全）
			m.finalizeSink()
		}
	}

	// 先在锁内收集 Save 所需快照，再到锁外执行可能耗时的 store.Save
	m.mu.Lock()
	var savePending bool
	var saveStatus traversal.Status
	var saveTaskID string
	if m.store != nil && taskID != "" {
		savePending = true
		saveTaskID = taskID
		saveStatus = m.status
		saveStatus.Results = append([]traversal.PointResult(nil), m.status.Results...)
	}
	activeIndex := m.activeIndex
	m.mu.Unlock()

	if savePending {
		if err := m.store.Save(saveTaskID, saveStatus); err != nil {
			slog.Error("traversal stop save failed",
				"component", "traversal",
				"task_id", taskID,
				"error", err,
			)
			// 聚合错误而非提前返回，确保后续清理（活动索引/锁）仍执行
			stopErr = errors.Join(stopErr, fmt.Errorf("save traversal result: %w", err))
		} else {
			slog.Info("traversal result saved on stop",
				"component", "traversal",
				"task_id", taskID,
			)
		}
	}

	// 清理活动索引
	if activeIndex != nil && taskID != "" {
		if err := activeIndex.Unregister(context.Background(), taskID); err != nil {
			slog.Warn("traversal active index unregister failed",
				"component", "traversal", "task_id", taskID, "error", err)
		}
	}

	// 释放工作流级互斥锁；幂等（finalizeSink 也会释放，重复调用安全）
	if taskID != "" {
		_ = resourcelock.Default().Release(traversalLockResource, taskID)
	}

	if stopErr != nil {
		slog.Error("traversal stop with error",
			"component", "traversal",
			"task_id", taskID,
			"error", stopErr,
		)
	} else {
		slog.Info("traversal stopped successfully",
			"component", "traversal",
			"task_id", taskID,
		)
	}
	return stopErr
}

// stopMotionAxes 停止遍历相关运动轴，返回第一个遇到的错误。
// 仅停止当前配置绑定的控制器/轴，避免误停其它已连接但未参与遍历的控制器。
func (m *TraversalManager) stopMotionAxes() error {
	if m.motion == nil {
		return nil
	}
	m.mu.RLock()
	motionAxes := m.config.MotionAxes
	m.mu.RUnlock()

	var firstErr error
	ctx := context.Background()
	// 容错预处理：与 RunCurrentPoint 保持一致，避免停止时也因 controllerId 不匹配
	// 而无法停止运动中的轴
	statuses := m.motion.StatusAll(ctx)
	motionAxes = resolveMotionAxes(motionAxes, statuses)
	for _, status := range statuses {
		// 无绑定配置时保持旧行为：停止所有运动中的轴
		if len(motionAxes) == 0 {
			for _, axis := range status.Axes {
				if axis.Moving {
					if err := m.motion.Stop(ctx, status.ID, axis.Name); err != nil && firstErr == nil {
						firstErr = err
					}
				}
			}
			continue
		}
		// 仅停止绑定到该控制器的轴（与当前位置无关，直接按绑定表过滤）
		for _, binding := range motionAxes {
			if binding.Axis == "" {
				continue
			}
			if binding.ControllerID != "" && binding.ControllerID != status.ID {
				continue
			}
			if err := m.motion.Stop(ctx, status.ID, motion.AxisName(binding.Axis)); err != nil && firstErr == nil {
				firstErr = err
			}
		}
	}
	return firstErr
}

// GetResult 获取测试结果
func (m *TraversalManager) GetResult(taskID string) (traversal.Status, bool) {
	if m.store == nil {
		return traversal.Status{}, false
	}
	return m.store.Get(taskID)
}

// RunTraversalLoop 主循环：按点驱动 RunCurrentPoint，直至完成/停止/错误。
// 稳定等待由 RunCurrentPoint → waitForStabilization 内部执行，
// 点间不再额外 sleep——旧实现点间再睡一次会使状态长时间停留在 saving，
// UI 观感像“卡在不动”，且每个点实际等待时间翻倍。
func (m *TraversalManager) RunTraversalLoop() {
	m.mu.RLock()
	session := m.session
	m.mu.RUnlock()
	if session == nil {
		return
	}
	defer session.MarkDone()
	defer m.finalizeSink() // 所有退出路径统一关闭 sink

	initStatus := m.Status()
	slog.Info("traversal loop started",
		"component", "traversal",
		"task_id", initStatus.TaskID,
		"total_points", initStatus.TotalPoints,
		"start_point", initStatus.CurrentPoint,
	)

	for {
		// v2：检查活动会话是否被取消（Stop 调用 session.Cancel() 后）
		if session.IsDone() {
			slog.Info("traversal loop exiting: session cancelled",
				"component", "traversal",
				"task_id", session.taskID,
			)
			return
		}
		status := m.Status()
		switch {
		case status.State == traversal.StateRunning || isSubState(status.State):
			if status.TotalPoints > 0 && status.CurrentPoint >= status.TotalPoints {
				slog.Info("traversal loop completed",
					"component", "traversal",
					"task_id", status.TaskID,
					"total_points", status.TotalPoints,
				)
				return
			}
			if err := m.RunCurrentPoint(); err != nil {
				slog.Error("traversal loop aborted",
					"component", "traversal",
					"task_id", status.TaskID,
					"point", status.CurrentPoint+1,
					"error", err,
				)
				return
			}
			// 获取最新状态以记录进度
			curStatus := m.Status()
			completed := curStatus.CurrentPoint
			total := curStatus.TotalPoints
			if completed > 0 && (completed%checkpointInterval == 0 || completed >= total) {
				// 进度信息已通过 status API 推送到 UI 状态栏，LOG 画面里属于刷屏噪音，降级为 Debug。
				slog.Debug("traversal progress",
					"component", "traversal",
					"task_id", curStatus.TaskID,
					"completed_points", completed,
					"total_points", total,
					"progress_pct", fmt.Sprintf("%.1f", float64(completed)/float64(total)*100),
				)
			}
		case status.State == traversal.StatePaused:
			time.Sleep(pausedLoopIdle)
		default:
			slog.Warn("traversal loop exiting on state",
				"component", "traversal",
				"task_id", status.TaskID,
				"state", status.State,
			)
			return
		}
	}
}
