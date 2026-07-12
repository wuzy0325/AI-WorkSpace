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

	// v2 可靠存储端口（Task 4-8）：三阶段提交与崩溃恢复所需
	// csvPort      CSV 落盘端口（支持新建/恢复、Sync、截断）
	// resultLogPort JSONL 完整结果日志端口（支持 AppendPrepared、Sync、ValidateTail）
	// checkpointPortFactory 结构化断点端口工厂（按 SavePath 动态创建，支持 Save/Load/Find/Unregister）
	// activeIndex   活动任务索引（taskId → checkpointPath），支持进程重启发现
	csvPort              ports.TraversalCSVPort
	resultLogPort        ports.TraversalResultLogPort
	checkpointPort       ports.TraversalCheckpointPort // 当前活动任务的断点端口（Start 时动态创建）
	checkpointPortFactory ports.TraversalCheckpointPortFactory
	activeIndex          ports.TraversalActiveIndex
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
	m.mu.RUnlock()

	channelMapPassed := hasPatm && hasTatm
	channelMapMessage := "All required channel labels are mapped"
	if !hasPatm {
		channelMapMessage = "Patm channel label is required for pressure normalization"
	} else if !hasTatm {
		channelMapMessage = "Tatm channel label is required for atmospheric calculation"
	}

	checks := []map[string]any{
		{"name": "PRB", "passed": hasInterpolator, "message": prbMessage},
		{"name": "Motion", "passed": hasMotion, "message": "Motion manager is available"},
		{"name": "DAQ", "passed": hasReader, "message": "DAQ acquisition hub is available"},
		{"name": "ChannelMap", "passed": channelMapPassed, "message": channelMapMessage},
	}
	allPassed := hasInterpolator && hasMotion && hasReader && channelMapPassed
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
		CSVPath:              config.SavePath,
		ResultLogPath:        config.SavePath + ".results.jsonl",
	}
	parentCtx := context.Background()
	session, err := m.beginSession(parentCtx, config.TaskID, snapshot)
	if err != nil {
		slog.Error("traversal start failed", "component", "traversal", "task_id", config.TaskID, "error", err)
		return err
	}

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
	csvPort := m.csvPort
	resultLogPort := m.resultLogPort
	activeIndex := m.activeIndex
	checkpointPortFactory := m.checkpointPortFactory
	sink := m.sink
	m.mu.Unlock()

	// v2：通过工厂按 SavePath 动态创建 checkpointPort（每个任务 SavePath 不同）
	var checkpointPort ports.TraversalCheckpointPort
	if checkpointPortFactory != nil && config.SavePath != "" {
		checkpointPort = checkpointPortFactory.Create(config.SavePath)
		m.mu.Lock()
		m.checkpointPort = checkpointPort
		m.mu.Unlock()
	}

	// v2 存储初始化：CSV 与结果日志
	if csvPort != nil {
		csvSession := ports.TraversalOutputSession{
			TaskID: config.TaskID,
			Mode:   ports.TraversalOutputCreate,
			Path:   snapshot.CSVPath,
		}
		if err := csvPort.Open(session.ctx, csvSession); err != nil {
			m.abortStartLocked(session, config.TaskID, fmt.Sprintf("csv open failed: %v", err), traversal.ErrSaveFailed)
			return err
		}
	}
	if resultLogPort != nil {
		logSession := ports.TraversalOutputSession{
			TaskID: config.TaskID,
			Mode:   ports.TraversalOutputCreate,
			Path:   snapshot.ResultLogPath,
		}
		if err := resultLogPort.Open(session.ctx, logSession); err != nil {
			m.abortStartLocked(session, config.TaskID, fmt.Sprintf("result log open failed: %v", err), traversal.ErrSaveFailed)
			return err
		}
	}
	// 注册活动索引，支持进程重启发现
	if activeIndex != nil && checkpointPort != nil {
		checkpointPath := config.SavePath + ".checkpoint.json"
		if err := activeIndex.Register(session.ctx, config.TaskID, checkpointPath); err != nil {
			slog.Warn("traversal active index register failed",
				"component", "traversal", "task_id", config.TaskID, "error", err)
			// 非阻塞：索引注册失败不影响任务启动，仅影响重启发现
		}
	}

	// 在锁外调用旧 sink.Initialize，避免阻塞其他状态读取（向后兼容）
	if sink != nil {
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

// abortStartLocked 启动失败时的统一回滚：设置错误状态、关闭已打开的 v2 端口、
// 关闭 session、释放工作流锁。
//
// 必须清理已打开的 v2 端口（csvPort/resultLogPort），否则：
//   - csvPort.Open 已创建 CSV 文件并写入表头，未关闭会导致后续 Start 时 Open 失败
//     （"会话已打开"错误），半启动的文件句柄也会泄漏
//   - resultLogPort 同理，已打开的 JSONL 文件句柄不会被 GC 回收
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

// finalizeSink 关闭 sink 并释放工作流级互斥锁
// 注意：Stop() 路径会主动 Finalize，此处再次 Finalize 是幂等操作
