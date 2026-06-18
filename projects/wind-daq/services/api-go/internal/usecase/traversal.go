package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"

	coreinterp "ai-workspace/shared/algorithms/go/fivehole/interpolation"
	"wind-daq/services/api-go/internal/core/device"
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
	isStopped            bool
	isPaused             bool
	motionPauseCancelled bool

	// 数据验证配置
	validation *traversal.DataValidationConfig

	// 稳定等待配置
	stabilization *traversal.StabilizationConfig
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
	return mgr
}

// loadPersistedConfig 启动时从磁盘加载已保存的遍历配置
// 同时尝试根据上一次的 SavePath 推断 checkpoint 路径并回填 lastCheckpointPath，
// 修复"应用重启后 LoadCheckpoint 永远返回 nil"的问题。
func (m *TraversalManager) loadPersistedConfig() {
	if m.configStore == nil {
		return
	}
	data, err := m.configStore.LoadConfig(traversalConfigKey)
	if err != nil || data == nil {
		return
	}
	m.mu.Lock()
	m.configRaw = json.RawMessage(data)
	m.mu.Unlock()

	// 尝试从已保存的配置中提取 savePath，并探测断点文件是否存在
	var probe struct {
		SavePath string `json:"savePath"`
	}
	if err := json.Unmarshal(data, &probe); err != nil || probe.SavePath == "" {
		return
	}
	if m.checkpointStore == nil {
		return
	}
	candidate := probe.SavePath + ".checkpoint.json"
	exists, err := m.checkpointStore.Stat(candidate)
	if err != nil || !exists {
		return
	}
	m.mu.Lock()
	m.lastCheckpointPath = candidate
	m.mu.Unlock()
}

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
	m.mu.Unlock()
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

func (m *TraversalManager) CheckPreconditions() map[string]any {
	hasInterpolator := m.HasLoadedInterpolator()
	hasMotion := m.motion != nil
	hasReader := m.reader != nil
	checks := []map[string]any{
		{"name": "PRB", "passed": hasInterpolator, "message": "Load PRB or calibration CSV before running interpolation"},
		{"name": "Motion", "passed": hasMotion, "message": "Motion manager is available"},
		{"name": "DAQ", "passed": hasReader, "message": "DAQ acquisition hub is available"},
	}
	allPassed := hasInterpolator && hasMotion && hasReader
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
	if config.TaskID == "" {
		return fmt.Errorf("taskID is required")
	}
	if config.DeviceID == "" {
		return fmt.Errorf("deviceID is required")
	}
	if len(config.Channels) == 0 {
		return fmt.Errorf("channels are required")
	}
	if len(config.Path) == 0 {
		return fmt.Errorf("path is required")
	}

	m.mu.Lock()
	if m.status.State == traversal.StateRunning || m.status.State == traversal.StatePaused {
		m.mu.Unlock()
		return fmt.Errorf("a traversal is already %s", m.status.State)
	}
	// 申请工作流级互斥锁（与 calibration 等其他工作流互斥）
	// TTL 给一个保守上限：单次遍历最多跑 24h；过期会被同名 holder 续约或外部接管
	if err := resourcelock.Default().Acquire(traversalLockResource, config.TaskID, 24*time.Hour); err != nil {
		m.mu.Unlock()
		return fmt.Errorf("acquire traversal lock: %w", err)
	}
	m.config = config
	m.isStopped = false
	m.isPaused = false
	m.motionPauseCancelled = false
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
			return err
		}
	}
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
func (m *TraversalManager) RunCurrentPoint() error {
	m.mu.Lock()
	// 允许 running 及其子状态（moving/stabilizing/acquiring/saving）进入；
	// 防止 Resume 与 loop 之间的瞬时竞态把已恢复的子状态误判为"未运行"
	if m.status.State != traversal.StateRunning && !isSubState(m.status.State) {
		errMsg := "traversal is not running"
		m.setErrorLocked(errMsg, traversal.ErrUnknown)
		m.mu.Unlock()
		return errors.New(errMsg)
	}
	if m.reader == nil {
		m.setErrorLocked("latest data reader is required", traversal.ErrAcquisitionFailed)
		m.mu.Unlock()
		return fmt.Errorf("latest data reader is required")
	}
	if m.motion == nil {
		m.setErrorLocked("motion manager is required", traversal.ErrMotionFailed)
		m.mu.Unlock()
		return fmt.Errorf("motion manager is required")
	}
	if m.status.CurrentPoint >= len(m.config.Path) {
		m.mu.Unlock()
		return fmt.Errorf("all traversal points are already complete")
	}
	config := m.config
	pointIndex := m.status.CurrentPoint
	point := config.Path[pointIndex]
	m.mu.Unlock()

	taskID := config.TaskID

	// 阶段1：移动中
	m.updatePhase(taskID, traversal.StateMoving, traversal.PhaseMoving, pointIndex, len(config.Path))
	ctx := context.Background()
	controllerStatuses := m.motion.StatusAll(ctx)
	for _, status := range controllerStatuses {
		if !status.Connected {
			continue
		}
		for axis, position := range availableAxisTargets(status, point) {
			if err := m.motion.MoveTo(ctx, status.ID, axis, position); err != nil {
				return m.failWithCode("move %s axis: %v", traversal.ErrMotionFailed, axis, err)
			}
		}
	}

	motionComplete := m.waitForMotionComplete(ctx, point, taskID)
	if !motionComplete {
		m.stopMotionAxes()
		if m.motionPauseCancelled {
			m.motionPauseCancelled = false
			return nil // 暂停导致的中断，不算错误
		}
		return m.failWithCode("motion did not complete for point %d", traversal.ErrMotionFailed, pointIndex+1)
	}

	// 阶段2：等待稳定
	m.updatePhase(taskID, traversal.StateStabilizing, traversal.PhaseStabilizing, pointIndex, len(config.Path))
	m.waitForStabilization(taskID)

	// 阶段3：采集中（含数据验证 + 可选重试）
	m.updatePhase(taskID, traversal.StateAcquiring, traversal.PhaseAcquiring, pointIndex, len(config.Path))
	samplesPerPoint := config.SamplesPerPoint
	if samplesPerPoint <= 0 {
		samplesPerPoint = 1
	}

	m.mu.RLock()
	validation := m.validation
	var channelLabels map[int]string
	if config.ChannelLabels != nil {
		channelLabels = make(map[int]string, len(config.ChannelLabels))
		for k, v := range config.ChannelLabels {
			channelLabels[k] = v
		}
	}
	m.mu.RUnlock()

	// 计算重试次数：仅在 onInvalid == "retry" 且 retryCount > 0 时启用
	maxAttempts := 1
	if validation != nil && validation.Enabled && validation.OnInvalid == "retry" && validation.RetryCount > 0 {
		maxAttempts = validation.RetryCount + 1
	}

	var resultValues map[int]float64
	var lastWarnings []string
	skipPoint := false
	for attempt := 0; attempt < maxAttempts; attempt++ {
		// 单次或多次采样
		if samplesPerPoint == 1 {
			payload, ok := m.reader.GetLatestData(config.DeviceID)
			if !ok {
				return m.failWithCode("no data available for device %s", traversal.ErrAcquisitionFailed, config.DeviceID)
			}
			resultValues = valuesForChannels(payload, config.Channels)
		} else {
			averaged, err := m.collectAveragedSamples(taskID, config.DeviceID, config.Channels, samplesPerPoint)
			if err != nil {
				return m.failWithCode("averaged sampling failed: %v", traversal.ErrAcquisitionFailed, err)
			}
			resultValues = averaged
		}

		if len(resultValues) != len(config.Channels) {
			return m.failWithCode("latest data does not contain all requested channels", traversal.ErrAcquisitionFailed)
		}

		// 无验证或验证通过即接受本次采集
		if validation == nil || !validation.Enabled {
			break
		}
		valid, warnings := traversal.ValidatePressures(resultValues, validation, channelLabels)
		lastWarnings = warnings
		if valid {
			break
		}

		// 验证失败，根据 onInvalid 决策
		switch validation.OnInvalid {
		case "skip":
			skipPoint = true
		case "retry":
			// 仍在重试上限内则继续；最后一次仍失败则视为接受（避免静默丢点）
			if attempt < maxAttempts-1 {
				continue
			}
		case "continue":
			// 继续接受当前结果
		}
		break
	}

	// 把最近一次校验告警记入 status，供前端展示
	m.mu.Lock()
	m.status.ValidationWarnings = lastWarnings
	m.mu.Unlock()

	if skipPoint {
		// 跳过此点，但 currentPoint 仍需推进
		m.mu.Lock()
		m.status.CurrentPoint++
		m.mu.Unlock()
		return nil
	}

	// 阶段4：保存中
	m.updatePhase(taskID, traversal.StateSaving, traversal.PhaseSaving, pointIndex, len(config.Path))

	dwellTime := config.DwellTimeMs
	// 实时插值（落盘和断点恢复都需要）：失败仅写 warning，不阻塞本点保存
	_, input, hasAll := BuildRawPressure(resultValues, config.ChannelLabels)
	var calculated *traversal.CalculatedResult
	if hasAll {
		interpRes, interpErr := m.CalculateRealtime(input)
		if interpErr == nil && interpRes.IsValid {
			calculated = &traversal.CalculatedResult{
				Valid: true,
				Alpha: interpRes.Alpha,
				Beta:  interpRes.Beta,
				Pt:    interpRes.TotalPressure,
				Ps:    interpRes.StaticPressure,
				Mach:  interpRes.MachNumber,
			}
		}
	}
	result := traversal.PointResult{
		PointIndex:       pointIndex,
		Point:            point,
		Timestamp:        time.Now().UnixMilli(),
		Values:           resultValues,
		SampleCount:      samplesPerPoint,
		DwellTimeElapsed: dwellTime,
		Calculated:       calculated,
	}
	if m.sink != nil {
		if err := m.sink.WriteTraversalPoint(result); err != nil {
			return m.failWithCode("write traversal point: %v", traversal.ErrSaveFailed, err)
		}
	}

	m.mu.Lock()
	m.status.Results = append(m.status.Results, result)
	m.status.CurrentPoint++
	m.status.ValidationWarnings = nil
	allDone := m.status.CurrentPoint >= len(m.config.Path)
	completedCount := m.status.CurrentPoint
	if allDone {
		m.status.State = traversal.StateCompleted
		m.status.CurrentPointPhase = ""
		if m.store != nil {
			status := m.status
			status.Results = append([]traversal.PointResult(nil), m.status.Results...)
			if err := m.store.Save(m.config.TaskID, status); err != nil {
				m.mu.Unlock()
				return fmt.Errorf("save traversal result: %v", err)
			}
		}
	}
	// 用于断点保存的快照（在锁内复制，避免锁外访问竞态）
	checkpointPath := m.config.SavePath
	checkpointPoints := append([]traversal.Point(nil), m.config.Path...)
	m.mu.Unlock()

	// 与 Cursor DAQ 一致：每完成 10 个点或最后一个点时保存断点
	if checkpointPath != "" && len(checkpointPoints) > 0 {
		if (pointIndex+1)%checkpointInterval == 0 || allDone {
			m.saveCheckpoint(checkpointPoints, completedCount, checkpointPath)
		}
	}

	// 测试成功完成后清理断点文件
	if allDone {
		m.ClearCheckpoint()
	}
	return nil
}

// updatePhase 更新当前阶段状态
func (m *TraversalManager) updatePhase(taskID string, state traversal.State, phase traversal.PointPhase, pointIndex, totalPoints int) {
	m.mu.Lock()
	if m.status.TaskID == taskID {
		m.status.State = state
		m.status.CurrentPointPhase = phase
	}
	m.mu.Unlock()
}

// waitForStabilization 等待数据稳定
func (m *TraversalManager) waitForStabilization(taskID string) {
	m.mu.RLock()
	stab := m.stabilization
	dwellMs := m.config.DwellTimeMs
	deviceID := m.config.DeviceID
	channels := m.config.Channels
	m.mu.RUnlock()

	if stab == nil || stab.Mode == "fixed" {
		// 固定等待模式
		waitMs := dwellMs
		if stab != nil && stab.FixedTimeMs > 0 {
			waitMs = stab.FixedTimeMs
		}
		if waitMs <= 0 {
			waitMs = 1000
		}
		m.sleepWithTaskCheck(taskID, time.Duration(waitMs)*time.Millisecond)
		return
	}

	// 自适应稳定模式
	if stab.Adaptive == nil {
		m.sleepWithTaskCheck(taskID, time.Duration(dwellMs)*time.Millisecond)
		return
	}

	adaptive := stab.Adaptive
	minWait := time.Duration(adaptive.MinWaitMs) * time.Millisecond
	maxWait := time.Duration(adaptive.MaxWaitMs) * time.Millisecond
	checkInterval := time.Duration(adaptive.CheckIntervalMs) * time.Millisecond
	threshold := adaptive.StabilityThreshold

	// 至少等待最小时间
	m.sleepWithTaskCheck(taskID, minWait)

	// 读取初始参考值
	prevValues := m.readCurrentValues(deviceID, channels)
	start := time.Now()
	stableCount := 0

	for time.Since(start) < maxWait && stableCount < adaptive.ConsecutiveChecks {
		if m.isTaskCancelled(taskID) {
			return
		}
		m.sleepWithTaskCheck(taskID, checkInterval)

		// 读取当前值并判断稳定性
		curValues := m.readCurrentValues(deviceID, channels)
		if m.isStable(prevValues, curValues, threshold) {
			stableCount++
		} else {
			stableCount = 0 // 不稳定则重置计数
		}
		prevValues = curValues
	}
}

// readCurrentValues 读取当前设备数据值
func (m *TraversalManager) readCurrentValues(deviceID string, channels []int) map[int]float64 {
	if m.reader == nil {
		return nil
	}
	payload, ok := m.reader.GetLatestData(deviceID)
	if !ok {
		return nil
	}
	return valuesForChannels(payload, channels)
}

// isStable 判断两组数据是否在稳定性阈值内
func (m *TraversalManager) isStable(prev, cur map[int]float64, threshold float64) bool {
	if prev == nil || cur == nil {
		return false
	}
	for k, prevVal := range prev {
		curVal, ok := cur[k]
		if !ok {
			return false
		}
		if prevVal == 0 {
			if abs(curVal) > threshold {
				return false
			}
			continue
		}
		// 计算百分比变化
		change := abs((curVal-prevVal)/prevVal) * 100
		if change > threshold {
			return false
		}
	}
	return true
}

// collectAveragedSamples 多次采样取平均（带总体超时保护、暂停/停止响应）
func (m *TraversalManager) collectAveragedSamples(taskID, deviceID string, channels []int, samplesPerPoint int) (map[int]float64, error) {
	totals := make(map[int]float64)
	validSamples := 0
	deadline := time.Now().Add(acquisitionBatchTimeout)

	for i := 0; i < samplesPerPoint; i++ {
		// 暂停或停止时立即中断采集，避免出现"测试已停止仍在累加"的情况
		if m.isTaskCancelled(taskID) {
			return nil, fmt.Errorf("acquisition cancelled")
		}
		if time.Now().After(deadline) {
			break // 超时保护：不再继续采样
		}
		payload, ok := m.reader.GetLatestData(deviceID)
		if !ok {
			continue
		}
		values := valuesForChannels(payload, channels)
		if len(values) == len(channels) {
			for k, v := range values {
				totals[k] += v
			}
			validSamples++
		}
		time.Sleep(acquisitionBatchPoll)
	}

	if validSamples == 0 {
		return nil, fmt.Errorf("no valid samples collected within timeout")
	}

	averaged := make(map[int]float64, len(totals))
	for k, total := range totals {
		averaged[k] = total / float64(validSamples)
	}
	return averaged, nil
}

// waitForMotionComplete 等待运动完成（带超时和取消检查）
func (m *TraversalManager) waitForMotionComplete(ctx context.Context, point traversal.Point, taskID string) bool {
	ticker := time.NewTicker(motionCompletePoll)
	defer ticker.Stop()
	deadline := time.Now().Add(motionCompleteTimeout)

	for {
		select {
		case <-ctx.Done():
			return false
		case <-ticker.C:
			// 检查是否暂停或停止（在锁内读取 isPaused 避免竞态）
			m.mu.RLock()
			isPaused := m.isPaused
			stopped := m.isStopped || isPaused
			m.mu.RUnlock()
			if stopped {
				if isPaused {
					m.motionPauseCancelled = true
				}
				return false
			}

			if time.Now().After(deadline) {
				return false
			}
			allReached := true
			for _, status := range m.motion.StatusAll(ctx) {
				for _, axis := range status.Axes {
					target, hasTarget := availableAxisTargets(status, point)[axis.Name]
					if !hasTarget {
						continue
					}
					if axis.Moving || abs(axis.Position-target) > 0.01 {
						allReached = false
						break
					}
				}
				if !allReached {
					break
				}
			}
			if allReached {
				return true
			}
		}
	}
}

func (m *TraversalManager) Pause() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.status.State != traversal.StateRunning && !isSubState(m.status.State) {
		return fmt.Errorf("traversal is not running")
	}
	m.isPaused = true
	m.status.State = traversal.StatePaused
	return nil
}

func (m *TraversalManager) Resume() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.status.State != traversal.StatePaused {
		return fmt.Errorf("traversal is not paused")
	}
	m.isPaused = false
	m.motionPauseCancelled = false
	m.status.State = traversal.StateRunning
	return nil
}

func (m *TraversalManager) Stop() error {
	// 先设置停止标志，让运行中的循环能尽快感知
	m.mu.Lock()
	m.isStopped = true
	m.isPaused = false
	m.status.State = traversal.StateStopped
	sink := m.sink
	m.mu.Unlock()

	// 停止所有运动轴（在锁外执行，避免持锁调用外部接口）
	stopErr := m.stopMotionAxes()

	m.mu.Lock()
	if m.store != nil {
		status := m.status
		status.Results = append([]traversal.PointResult(nil), m.status.Results...)
		if err := m.store.Save(m.config.TaskID, status); err != nil {
			m.mu.Unlock()
			return fmt.Errorf("save traversal result: %v", err)
		}
	}
	m.mu.Unlock()

	// 在锁外关闭 sink，确保 CSV 缓冲被刷盘
	if sink != nil {
		if err := sink.FinalizeTraversal(); err != nil && stopErr == nil {
			stopErr = err
		}
	}

	// 释放工作流级互斥锁；幂等
	if taskID := m.config.TaskID; taskID != "" {
		_ = resourcelock.Default().Release(traversalLockResource, taskID)
	}

	return stopErr
}

// stopMotionAxes 停止所有运动轴，返回第一个遇到的错误
func (m *TraversalManager) stopMotionAxes() error {
	if m.motion == nil {
		return nil
	}
	var firstErr error
	ctx := context.Background()
	for _, status := range m.motion.StatusAll(ctx) {
		for _, axis := range status.Axes {
			if axis.Moving {
				if err := m.motion.Stop(ctx, status.ID, axis.Name); err != nil && firstErr == nil {
					firstErr = err
				}
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

// LoadCheckpoint 从最近一次保存的断点文件加载恢复信息
// 与 Cursor DAQ 行为一致：若 lastCheckpointPath 为空或文件不存在，返回 nil 且无错误
// isSubState 判断是否为运行中的子状态
func isSubState(s traversal.State) bool {
	return s == traversal.StateMoving || s == traversal.StateStabilizing ||
		s == traversal.StateAcquiring || s == traversal.StateSaving || s == traversal.StatePreparing
}

type traversalAPIConfig struct {
	Name   string `json:"name"`
	Layout struct {
		Pattern    string `json:"pattern"`
		SnakeOrder bool   `json:"snakeOrder"`
		Line       *struct {
			StartX        float64                 `json:"startX"`
			StartY        float64                 `json:"startY"`
			EndX          float64                 `json:"endX"`
			EndY          float64                 `json:"endY"`
			XStepSegments []traversal.StepSegment `json:"xStepSegments"`
			YStepSegments []traversal.StepSegment `json:"yStepSegments"`
		} `json:"line"`
		Rectangle *struct {
			XMin          float64                 `json:"xMin"`
			XMax          float64                 `json:"xMax"`
			XStepSegments []traversal.StepSegment `json:"xStepSegments"`
			YMin          float64                 `json:"yMin"`
			YMax          float64                 `json:"yMax"`
			YStepSegments []traversal.StepSegment `json:"yStepSegments"`
		} `json:"rectangle"`
		Sector *struct {
			CenterX             float64                 `json:"centerX"`
			CenterY             float64                 `json:"centerY"`
			RadiusMin           float64                 `json:"radiusMin"`
			RadiusMax           float64                 `json:"radiusMax"`
			RadialStepSegments  []traversal.StepSegment `json:"radialStepSegments"`
			AngleStart          float64                 `json:"angleStart"`
			AngleEnd            float64                 `json:"angleEnd"`
			AngularStepSegments []traversal.StepSegment `json:"angularStepSegments"`
		} `json:"sector"`
		Custom *struct {
			Points []struct {
				X float64 `json:"x"`
				Y float64 `json:"y"`
			} `json:"points"`
		} `json:"custom"`
	} `json:"layout"`
	Channels struct {
		ProbeChannels []struct {
			Name    string `json:"name"`
			Role    string `json:"role"`
			Channel struct {
				DeviceID     string `json:"deviceId"`
				ChannelIndex int    `json:"channelIndex"`
			} `json:"channel"`
			Enabled bool `json:"enabled"`
		} `json:"probeChannels"`
	} `json:"channels"`
	DwellTimeMs       int                             `json:"dwellTimeMs"`
	SamplesPerPoint   int                             `json:"samplesPerPoint"`
	SavePath          string                          `json:"savePath"`
	SaveFileName      string                          `json:"saveFileName"`
	SaveOptions       *traversal.SaveOptions          `json:"saveOptions,omitempty"`
	Validation        *traversal.DataValidationConfig `json:"validation,omitempty"`
	Stabilization     *traversal.StabilizationConfig  `json:"stabilization,omitempty"`
	InterpolationMode string                          `json:"interpolationMode,omitempty"`
}

// roleToLabel 将前端 ProbeChannelConfig.role 转为压力标签
// 例如 "fiveHole.p1" → "P1"，"fiveHole.pAtm" → "Patm"
func roleToLabel(role, name string) string {
	switch role {
	case "fiveHole.p1":
		return "P1"
	case "fiveHole.p2":
		return "P2"
	case "fiveHole.p3":
		return "P3"
	case "fiveHole.p4":
		return "P4"
	case "fiveHole.p5":
		return "P5"
	case "fiveHole.pAtm":
		return "Patm"
	case "fiveHole.tAtm":
		return "Tatm"
	}
	// 回退使用 name 字段作为标签
	return name
}

func (m *TraversalManager) ParseAndStartTraversal(raw json.RawMessage) (string, error) {
	var legacy struct {
		TaskID   string            `json:"taskId"`
		DeviceID string            `json:"deviceId"`
		Channels []int             `json:"channels"`
		Path     []traversal.Point `json:"path"`
	}
	if err := json.Unmarshal(raw, &legacy); err == nil && legacy.TaskID != "" {
		config := traversal.Config{
			TaskID: legacy.TaskID, DeviceID: legacy.DeviceID,
			Channels: legacy.Channels, Path: legacy.Path,
		}
		if err := m.Start(config); err != nil {
			return "", err
		}
		m.SaveConfigRaw(raw)
		return config.TaskID, nil
	}

	var cfg traversalAPIConfig
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return "", fmt.Errorf("invalid request: %w", err)
	}

	points := traversal.PointsFromLayout(traversal.LayoutConfig{
		Pattern:    cfg.Layout.Pattern,
		SnakeOrder: cfg.Layout.SnakeOrder,
		Line: func() *traversal.LineLayout {
			if cfg.Layout.Line == nil {
				return nil
			}
			return &traversal.LineLayout{
				StartX: cfg.Layout.Line.StartX, StartY: cfg.Layout.Line.StartY,
				EndX: cfg.Layout.Line.EndX, EndY: cfg.Layout.Line.EndY,
				XStepSegments: cfg.Layout.Line.XStepSegments,
				YStepSegments: cfg.Layout.Line.YStepSegments,
			}
		}(),
		Rectangle: func() *traversal.RectangleLayout {
			if cfg.Layout.Rectangle == nil {
				return nil
			}
			return &traversal.RectangleLayout{
				XMin: cfg.Layout.Rectangle.XMin, XMax: cfg.Layout.Rectangle.XMax,
				XStepSegments: cfg.Layout.Rectangle.XStepSegments,
				YStepSegments: cfg.Layout.Rectangle.YStepSegments,
			}
		}(),
		Sector: func() *traversal.SectorLayout {
			if cfg.Layout.Sector == nil {
				return nil
			}
			return &traversal.SectorLayout{
				CenterX: cfg.Layout.Sector.CenterX, CenterY: cfg.Layout.Sector.CenterY,
				RadiusMin: cfg.Layout.Sector.RadiusMin, RadiusMax: cfg.Layout.Sector.RadiusMax,
				RadialStepSegments:  cfg.Layout.Sector.RadialStepSegments,
				AngleStart:          cfg.Layout.Sector.AngleStart,
				AngleEnd:            cfg.Layout.Sector.AngleEnd,
				AngularStepSegments: cfg.Layout.Sector.AngularStepSegments,
			}
		}(),
		Custom: func() *traversal.CustomLayout {
			if cfg.Layout.Custom == nil {
				return nil
			}
			cl := &traversal.CustomLayout{}
			for _, p := range cfg.Layout.Custom.Points {
				cl.Points = append(cl.Points, struct {
					X float64 `json:"x"`
					Y float64 `json:"y"`
				}{X: p.X, Y: p.Y})
			}
			return cl
		}(),
	})

	channels := make([]int, 0, len(cfg.Channels.ProbeChannels))
	channelLabels := make(map[int]string)
	deviceID := ""
	for _, probe := range cfg.Channels.ProbeChannels {
		if !probe.Enabled || probe.Channel.ChannelIndex < 0 {
			continue
		}
		if deviceID == "" {
			deviceID = probe.Channel.DeviceID
		}
		channels = append(channels, probe.Channel.ChannelIndex)
		// 通过 role/name 显式建立 channelIndex→label 映射，避免依赖通道索引顺序
		if label := roleToLabel(probe.Role, probe.Name); label != "" {
			channelLabels[probe.Channel.ChannelIndex] = label
		}
	}
	if deviceID == "" {
		return "", fmt.Errorf("deviceId is required")
	}
	if len(channels) == 0 {
		return "", fmt.Errorf("channels are required")
	}
	if len(points) == 0 {
		return "", fmt.Errorf("path is required")
	}

	dwell := time.Duration(cfg.DwellTimeMs) * time.Millisecond
	if dwell < 0 {
		dwell = 0
	}
	samplesPerPoint := cfg.SamplesPerPoint
	if samplesPerPoint <= 0 {
		samplesPerPoint = 1
	}
	config := traversal.Config{
		TaskID:            fmt.Sprintf("trav-%d", time.Now().UnixMilli()),
		DeviceID:          deviceID,
		Channels:          channels,
		Path:              points,
		DwellTimeMs:       cfg.DwellTimeMs,
		SamplesPerPoint:   samplesPerPoint,
		SavePath:          cfg.SavePath,
		SaveFileName:      cfg.SaveFileName,
		SaveOptions:       cfg.SaveOptions,
		ChannelLabels:     channelLabels,
		InterpolationMode: cfg.InterpolationMode,
	}
	// 注入数据验证与稳定等待配置（前端可选传入）
	m.SetValidation(cfg.Validation)
	m.SetStabilization(cfg.Stabilization)

	if err := m.Start(config); err != nil {
		return "", err
	}
	m.SaveConfigRaw(raw)
	if dwell > 0 {
		go m.RunTraversalLoop(dwell)
	}
	return config.TaskID, nil
}

// 任何退出路径都会调用 sink.FinalizeTraversal 关闭文件，保证落盘
func (m *TraversalManager) RunTraversalLoop(dwell time.Duration) {
	if dwell <= 0 {
		dwell = 100 * time.Millisecond
	}
	defer m.finalizeSink() // 所有退出路径统一关闭 sink
	for {
		status := m.Status()
		switch {
		case status.State == traversal.StateRunning || isSubState(status.State):
			if status.TotalPoints > 0 && status.CurrentPoint >= status.TotalPoints {
				return
			}
			if err := m.RunCurrentPoint(); err != nil {
				return
			}
			time.Sleep(dwell)
		case status.State == traversal.StatePaused:
			time.Sleep(pausedLoopIdle)
		default:
			return
		}
	}
}

// finalizeSink 关闭 sink 并释放工作流级互斥锁
// 注意：Stop() 路径会主动 Finalize，此处再次 Finalize 是幂等操作
func (m *TraversalManager) finalizeSink() {
	m.mu.Lock()
	sink := m.sink
	taskID := m.config.TaskID
	m.mu.Unlock()
	if sink != nil {
		// FinalizeTraversal 自身需保证幂等（多次调用安全）
		_ = sink.FinalizeTraversal()
	}
	// 释放工作流级互斥锁；幂等
	if taskID != "" {
		_ = resourcelock.Default().Release(traversalLockResource, taskID)
	}
}

func (m *TraversalManager) BuildStatusResponse() map[string]any {
	status := m.Status()
	state := string(status.State)
	if status.State == traversal.StateIdle && status.TotalPoints > 0 && status.CurrentPoint >= status.TotalPoints {
		state = "completed"
	}
	// 兼容：子状态也映射为 running
	displayState := state
	if isSubState(status.State) {
		displayState = "running"
	}
	progress := 0.0
	if status.TotalPoints > 0 {
		progress = float64(status.CurrentPoint) / float64(status.TotalPoints) * 100
	}
	var currentPoint map[string]float64
	if status.CurrentPointCoordinates != nil {
		point := *status.CurrentPointCoordinates
		currentPoint = map[string]float64{"alpha": point.X, "beta": point.Y}
	}
	dataPoints := m.BuildDataPoints(status.Results)
	var latestData any
	if len(dataPoints) > 0 {
		latestData = dataPoints[len(dataPoints)-1]
	}
	return map[string]any{
		"taskId":                  status.TaskID,
		"state":                   string(status.State),
		"status":                  displayState,
		"currentPoint":            status.CurrentPoint,
		"currentPointCoordinates": currentPoint,
		"currentPointPhase":       string(status.CurrentPointPhase),
		"completedPoints":         status.CurrentPoint,
		"totalPoints":             status.TotalPoints,
		"progress":                progress,
		"startTime":               status.StartedAt,
		"lastError":               status.LastError,
		"lastErrorCode":           string(status.LastErrorCode),
		"results":                 status.Results,
		"dataPoints":              dataPoints,
		"latestData":              latestData,
		"validationWarnings":      status.ValidationWarnings,
	}
}

// BuildDataPoints 从遍历结果构建数据点
func (m *TraversalManager) BuildDataPoints(results []traversal.PointResult) []map[string]any {
	dataPoints := make([]map[string]any, 0, len(results))
	// 优先使用 Config.ChannelLabels 进行 channelIndex→label 映射
	m.mu.RLock()
	channelLabels := m.config.ChannelLabels
	m.mu.RUnlock()
	for _, result := range results {
		rawPressure, input, ok := BuildRawPressure(result.Values, channelLabels)
		interpolationResult := coreinterp.InterpolationResult{IsValid: false}
		if ok {
			calculated, err := m.CalculateRealtime(input)
			if err == nil {
				interpolationResult = calculated
			} else {
				interpolationResult.Warning = err.Error()
			}
		}
		dataPoints = append(dataPoints, map[string]any{
			"pointId":             result.PointIndex + 1,
			"coordinates":         map[string]float64{"alpha": result.Point.X, "beta": result.Point.Y},
			"rawPressure":         rawPressure,
			"interpolationResult": interpolationResult,
			"sampleCount":         result.SampleCount,
			"timestamp":           result.Timestamp,
			"dwellTimeElapsed":    result.DwellTimeElapsed,
		})
	}
	return dataPoints
}

// BuildRawPressure 从通道值构建原始压力数据和插值输入
// 通道映射策略：若 labels 提供则按显式映射；否则按通道索引升序回退到旧行为
func BuildRawPressure(values map[int]float64, labels map[int]string) (map[string]float64, coreinterp.InterpolationInput, bool) {
	raw := make(map[string]float64, 7)
	if len(labels) > 0 {
		for chIdx, value := range values {
			if label, ok := labels[chIdx]; ok && label != "" {
				raw[label] = value
			}
		}
	} else {
		// 兼容旧行为：通道索引升序对应 P1..Tatm
		orderedKeys := make([]int, 0, len(values))
		for key := range values {
			orderedKeys = append(orderedKeys, key)
		}
		sort.Ints(orderedKeys)
		legacyLabels := []string{"P1", "P2", "P3", "P4", "P5", "Patm", "Tatm"}
		for i, label := range legacyLabels {
			if i >= len(orderedKeys) {
				continue
			}
			raw[label] = values[orderedKeys[i]]
		}
	}
	input := coreinterp.InterpolationInput{
		P1:   raw["P1"],
		P2:   raw["P2"],
		P3:   raw["P3"],
		P4:   raw["P4"],
		P5:   raw["P5"],
		PAtm: raw["Patm"],
		TAtm: raw["Tatm"],
	}
	_, hasP1 := raw["P1"]
	_, hasP2 := raw["P2"]
	_, hasP3 := raw["P3"]
	_, hasP4 := raw["P4"]
	_, hasP5 := raw["P5"]
	_, hasPatm := raw["Patm"]
	_, hasTatm := raw["Tatm"]
	return raw, input, hasP1 && hasP2 && hasP3 && hasP4 && hasP5 && hasPatm && hasTatm
}
