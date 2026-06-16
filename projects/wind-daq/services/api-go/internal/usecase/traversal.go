package usecase

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"sync"
	"time"

	coreinterp "ai-workspace/shared/algorithms/go/fivehole/interpolation"
	"wind-daq/services/api-go/internal/core/device"
	"wind-daq/services/api-go/internal/core/motion"
	"wind-daq/services/api-go/internal/core/traversal"
	"wind-daq/services/api-go/internal/ports"
)

// 运动完成等待超时和轮询间隔
const (
	motionCompleteTimeoutMs   = 120000
	motionCompletePollMs      = 100
	acquisitionBatchTimeoutMs = 2000
	acquisitionBatchPollMs    = 10
	checkpointInterval        = 10 // 每完成10个点保存一次断点
)

type TraversalManager struct {
	mu           sync.RWMutex
	reader       ports.LatestDataReader
	motion       ports.MotionAccess
	sink         ports.TraversalPointSink
	store        ports.TraversalResultStore
	config       traversal.Config
	status       traversal.Status
	configRaw    json.RawMessage
	interpolator coreinterp.Interpolator

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

func NewTraversalManager(reader ports.LatestDataReader, motion ports.MotionAccess, sink ports.TraversalPointSink, store ports.TraversalResultStore) *TraversalManager {
	return &TraversalManager{
		reader: reader,
		motion: motion,
		sink:   sink,
		store:  store,
		status: traversal.Status{State: traversal.StateIdle},
	}
}

func (m *TraversalManager) GenerateGridPath(config traversal.GridConfig) ([]traversal.Point, error) {
	return traversal.GenerateGridPath(config)
}

func (m *TraversalManager) SaveConfigRaw(config json.RawMessage) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.configRaw = append(json.RawMessage(nil), config...)
}

func (m *TraversalManager) GetConfigRaw() json.RawMessage {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return append(json.RawMessage(nil), m.configRaw...)
}

func (m *TraversalManager) SetInterpolator(interpolator coreinterp.Interpolator) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.interpolator = interpolator
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

func (m *TraversalManager) CalculateRealtime(input coreinterp.InterpolationInput) (coreinterp.InterpolationResult, error) {
	m.mu.RLock()
	interpolator := m.interpolator
	m.mu.RUnlock()
	if interpolator == nil || !interpolator.IsLoaded() {
		return coreinterp.InterpolationResult{}, fmt.Errorf("PRB interpolation data is not loaded")
	}
	return interpolator.Calculate(input)
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
	defer m.mu.Unlock()
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
	if m.status.State != traversal.StateRunning {
		errMsg := "traversal is not running"
		m.setErrorLocked(errMsg, traversal.ErrUnknown)
		m.mu.Unlock()
		return fmt.Errorf(errMsg)
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

	// 阶段3：采集中
	m.updatePhase(taskID, traversal.StateAcquiring, traversal.PhaseAcquiring, pointIndex, len(config.Path))
	samplesPerPoint := config.SamplesPerPoint
	if samplesPerPoint <= 0 {
		samplesPerPoint = 1
	}

	var resultValues map[int]float64
	var ok bool
	if samplesPerPoint == 1 {
		// 单次采样（兼容旧逻辑）
		var payload device.DataPayload
		payload, ok = m.reader.GetLatestData(config.DeviceID)
		if !ok {
			return m.failWithCode("no data available for device %s", traversal.ErrAcquisitionFailed, config.DeviceID)
		}
		resultValues = valuesForChannels(payload, config.Channels)
	} else {
		// 多次采样取平均
		averaged, err := m.collectAveragedSamples(config.DeviceID, config.Channels, samplesPerPoint)
		if err != nil {
			return m.failWithCode("averaged sampling failed: %v", traversal.ErrAcquisitionFailed, err)
		}
		resultValues = averaged
	}

	if len(resultValues) != len(config.Channels) {
		return m.failWithCode("latest data does not contain all requested channels", traversal.ErrAcquisitionFailed)
	}

	// 数据验证
	m.mu.RLock()
	validation := m.validation
	m.mu.RUnlock()
	if validation != nil && validation.Enabled {
		valid, warnings := traversal.ValidatePressures(resultValues, validation)
		m.mu.Lock()
		m.status.ValidationWarnings = warnings
		m.mu.Unlock()
		if !valid && validation.OnInvalid == "skip" {
			// 跳过此点，继续下一个
			m.mu.Lock()
			m.status.CurrentPoint++
			m.mu.Unlock()
			return nil
		}
	}

	// 阶段4：保存中
	m.updatePhase(taskID, traversal.StateSaving, traversal.PhaseSaving, pointIndex, len(config.Path))

	dwellTime := config.DwellTimeMs
	result := traversal.PointResult{
		PointIndex:       pointIndex,
		Point:            point,
		Timestamp:        time.Now().UnixMilli(),
		Values:           resultValues,
		SampleCount:      samplesPerPoint,
		DwellTimeElapsed: dwellTime,
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
	m.mu.Unlock()
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

// collectAveragedSamples 多次采样取平均（带总体超时保护）
func (m *TraversalManager) collectAveragedSamples(deviceID string, channels []int, samplesPerPoint int) (map[int]float64, error) {
	totals := make(map[int]float64)
	validSamples := 0
	deadline := time.Now().Add(time.Duration(acquisitionBatchTimeoutMs) * time.Millisecond)

	for i := 0; i < samplesPerPoint; i++ {
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
		time.Sleep(time.Duration(acquisitionBatchPollMs) * time.Millisecond)
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
	ticker := time.NewTicker(motionCompletePollMs)
	defer ticker.Stop()
	deadline := time.Now().Add(motionCompleteTimeoutMs)

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

func (m *TraversalManager) GetResult(taskID string) (traversal.Status, bool) {
	if m.store == nil {
		return traversal.Status{}, false
	}
	return m.store.Get(taskID)
}

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
			Channel struct {
				DeviceID     string `json:"deviceId"`
				ChannelIndex int    `json:"channelIndex"`
			} `json:"channel"`
			Enabled bool `json:"enabled"`
		} `json:"probeChannels"`
	} `json:"channels"`
	DwellTimeMs     int `json:"dwellTimeMs"`
	SamplesPerPoint int `json:"samplesPerPoint"`
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
	deviceID := ""
	for _, probe := range cfg.Channels.ProbeChannels {
		if !probe.Enabled || probe.Channel.ChannelIndex < 0 {
			continue
		}
		if deviceID == "" {
			deviceID = probe.Channel.DeviceID
		}
		channels = append(channels, probe.Channel.ChannelIndex)
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
		TaskID:          fmt.Sprintf("trav-%d", time.Now().UnixMilli()),
		DeviceID:        deviceID,
		Channels:        channels,
		Path:            points,
		DwellTimeMs:     cfg.DwellTimeMs,
		SamplesPerPoint: samplesPerPoint,
	}
	if err := m.Start(config); err != nil {
		return "", err
	}
	m.SaveConfigRaw(raw)
	if dwell > 0 {
		go m.RunTraversalLoop(dwell)
	}
	return config.TaskID, nil
}

func (m *TraversalManager) fail(format string, args ...any) error {
	message := fmt.Sprintf(format, args...)
	m.mu.Lock()
	m.setErrorLocked(message, traversal.ErrUnknown)
	m.mu.Unlock()
	return fmt.Errorf("%s", message)
}

// failWithCode 带错误码的失败
func (m *TraversalManager) failWithCode(format string, code traversal.ErrorCode, args ...any) error {
	message := fmt.Sprintf(format, args...)
	m.mu.Lock()
	m.setErrorLocked(message, code)
	m.mu.Unlock()
	return fmt.Errorf("%s", message)
}

func (m *TraversalManager) setErrorLocked(message string, code traversal.ErrorCode) {
	m.status.State = traversal.StateError
	m.status.LastError = message
	m.status.LastErrorCode = code
}

// isTaskCancelled 检查任务是否已取消
func (m *TraversalManager) isTaskCancelled(taskID string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.status.TaskID != taskID || m.isStopped
}

// sleepWithTaskCheck 带任务取消检查的睡眠
func (m *TraversalManager) sleepWithTaskCheck(taskID string, d time.Duration) {
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		if m.isTaskCancelled(taskID) {
			return
		}
		time.Sleep(time.Duration(motionCompletePollMs) * time.Millisecond)
	}
}

func abs(f float64) float64 {
	if f < 0 {
		return -f
	}
	return f
}

// valuesForChannels 从数据载荷中提取指定通道索引的值
func valuesForChannels(payload device.DataPayload, channels []int) map[int]float64 {
	valuesByIndex := make(map[int]float64, len(payload.Channels))
	for i, value := range payload.Channels {
		chIdx := i
		if i < len(payload.ChannelIndices) {
			chIdx = payload.ChannelIndices[i]
		}
		valuesByIndex[chIdx] = value
	}

	values := make(map[int]float64, len(channels))
	for _, channel := range channels {
		value, ok := valuesByIndex[channel]
		if ok {
			values[channel] = value
		}
	}
	return values
}

func availableAxisTargets(status motion.ControllerStatus, point traversal.Point) map[motion.AxisName]float64 {
	targets := make(map[motion.AxisName]float64, len(status.Axes))
	for _, axis := range status.Axes {
		switch axis.Name {
		case motion.AxisX:
			targets[axis.Name] = point.X
		case motion.AxisY:
			targets[axis.Name] = point.Y
		case motion.AxisZ:
			targets[axis.Name] = point.Z
		}
	}
	return targets
}

func (m *TraversalManager) RunTraversalLoop(dwell time.Duration) {
	if dwell <= 0 {
		dwell = 100 * time.Millisecond
	}
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
			time.Sleep(200 * time.Millisecond)
		default:
			return
		}
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
	for _, result := range results {
		rawPressure, input, ok := BuildRawPressure(result.Values)
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
func BuildRawPressure(values map[int]float64) (map[string]float64, coreinterp.InterpolationInput, bool) {
	orderedKeys := make([]int, 0, len(values))
	for key := range values {
		orderedKeys = append(orderedKeys, key)
	}
	sort.Ints(orderedKeys)
	raw := make(map[string]float64, 7)
	labels := []string{"P1", "P2", "P3", "P4", "P5", "Patm", "Tatm"}
	for i, label := range labels {
		if i >= len(orderedKeys) {
			continue
		}
		raw[label] = values[orderedKeys[i]]
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
