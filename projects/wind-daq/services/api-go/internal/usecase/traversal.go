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

type TraversalManager struct {
	mu              sync.RWMutex
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

	m.mu.Lock()
	if m.status.State == traversal.StateRunning || m.status.State == traversal.StatePaused {
		m.mu.Unlock()
		err := fmt.Errorf("a traversal is already %s", m.status.State)
		slog.Error("traversal start failed", "component", "traversal", "task_id", config.TaskID, "error", err)
		return err
	}
	// 申请工作流级互斥锁（与 calibration 等其他工作流互斥）
	// TTL 给一个保守上限：单次遍历最多跑 24h；过期会被同名 holder 续约或外部接管
	if err := resourcelock.Default().Acquire(traversalLockResource, config.TaskID, 24*time.Hour); err != nil {
		m.mu.Unlock()
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
	sink := m.sink
	m.mu.Unlock()

	// 在锁外调用 sink.Initialize，避免阻塞其他状态读取
	if sink != nil {
		if err := sink.InitializeTraversal(config); err != nil {
			// 初始化失败：回滚状态并释放锁，避免半启动
			m.mu.Lock()
			m.status.State = traversal.StateError
			m.status.LastError = fmt.Sprintf("sink init failed: %v", err)
			m.status.LastErrorCode = traversal.ErrSaveFailed
			m.mu.Unlock()
			_ = resourcelock.Default().Release(traversalLockResource, config.TaskID)
			slog.Error("traversal start failed",
				"component", "traversal", "task_id", config.TaskID,
				"error", fmt.Sprintf("sink init failed: %v", err),
			)
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

func (m *TraversalManager) Stop() error {
	// 先设置停止标志，让运行中的循环能尽快感知
	m.mu.Lock()
	m.isStopped = true
	m.isPaused = false
	m.status.State = traversal.StateStopped
	sink := m.sink
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
	m.mu.Unlock()

	if savePending {
		if err := m.store.Save(saveTaskID, saveStatus); err != nil {
			slog.Error("traversal stop save failed",
				"component", "traversal",
				"task_id", taskID,
				"error", err,
			)
			return fmt.Errorf("save traversal result: %v", err)
		}
		slog.Info("traversal result saved on stop",
			"component", "traversal",
			"task_id", taskID,
		)
	}

	// 在锁外关闭 sink，确保 CSV 缓冲被刷盘
	if sink != nil {
		if err := sink.FinalizeTraversal(); err != nil && stopErr == nil {
			stopErr = err
		}
	}

	// 释放工作流级互斥锁；幂等
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
	defer m.finalizeSink() // 所有退出路径统一关闭 sink

	initStatus := m.Status()
	slog.Info("traversal loop started",
		"component", "traversal",
		"task_id", initStatus.TaskID,
		"total_points", initStatus.TotalPoints,
		"start_point", initStatus.CurrentPoint,
	)

	for {
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
