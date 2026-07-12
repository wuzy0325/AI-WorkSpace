// Package usecase — traversal 采集执行（从 traversal.go 拆分）
//
// 包含 RunCurrentPoint 单点采集主流程，以及稳定等待、平均采样、运动到位
// 等子流程。RunTraversalLoop 仍在 traversal.go，本文件聚焦 "如何采到一个点"。
package usecase

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"time"

	"wind-daq/services/api-go/internal/core/traversal"
)

// stabilityNearZeroEpsilon 稳定性判断中"近零值"的判定阈值
// 当 |prevVal| 小于该值时，切换为绝对差阈值比较，避免极小分母放大波动。
// 取值理由：传感器分辨率典型量级（< 1e-3 物理单位）远大于该值，不会误判正常读数。
const stabilityNearZeroEpsilon = 1e-9

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
	// unitProvider 与 config 同步取出，供 BuildRawPressure 归一化使用。
	// 在锁内取避免与 SetUnitProvider 写入并发竞争。
	unitProvider := m.unitProvider
	pointIndex := m.status.CurrentPoint
	point := config.Path[pointIndex]
	m.mu.Unlock()

	taskID := config.TaskID
	// motionAxes 决定哪些轴参与遍历运动；为空时（旧配置）保持原行为对所有轴发 MoveTo
	motionAxes := config.MotionAxes

	// 每个遍历点都会触发一次，属于高频日志，降级为 Debug 避免刷屏。
	// 进度信息已经通过 status API 推送到 UI，LOG 画面里只在调试时才需要看到。
	// 4 轴全输出：系统已支持 X/Y/Z/U 四轴运动控制，调试时需看到完整坐标
	// 才能排查 4 轴定位/插值问题，仅输出 X/Y 会丢失 Z/U 上下文。
	// NaN 显示为 "N/A"：line/rectangle/sector 模式未配置的轴标记为 NaN，避免日志出现 "NaN" 干扰排查。
	slog.Debug("traversal running point",
		"component", "traversal",
		"task_id", taskID,
		"point_index", pointIndex+1,
		"total_points", len(config.Path),
		"coordinates", fmt.Sprintf("(X=%s, Y=%s, Z=%s, U=%s)",
			formatCoord(point.X), formatCoord(point.Y), formatCoord(point.Z), formatCoord(point.U)),
	)

	// 阶段1：移动中
	m.updatePhase(taskID, traversal.StateMoving, traversal.PhaseMoving, pointIndex, len(config.Path))
	ctx := context.Background()
	controllerStatuses := m.motion.StatusAll(ctx)
	// 容错预处理：controllerId 全部不匹配已连接控制器时（如前端用别名 "sim-motion-1"
	// 而后端 profile.ID 是 UUID），回退到按轴名匹配，避免遍历空转卡在「稳定中」阶段。
	motionAxes = resolveMotionAxes(motionAxes, controllerStatuses)
	// 诊断日志：记录运动阶段开始时的关键信息，便于排查「卡在稳定中」类问题。
	// 属于硬件通信日志范畴，默认随硬件通信开关显示。
	connectedIDs := make([]string, 0)
	for _, s := range controllerStatuses {
		if s.Connected {
			connectedIDs = append(connectedIDs, s.ID)
		}
	}
	slog.Info("traversal motion phase started",
		"component", "traversal",
		"task_id", taskID,
		"point_index", pointIndex+1,
		"connected_controllers", connectedIDs,
		"motion_axes", motionAxes,
		"point", fmt.Sprintf("(X=%s, Y=%s, Z=%s, U=%s)",
			formatCoord(point.X), formatCoord(point.Y), formatCoord(point.Z), formatCoord(point.U)),
	)
	for _, status := range controllerStatuses {
		if !status.Connected {
			continue
		}
		for axis, position := range availableAxisTargets(status, point, motionAxes) {
			if err := m.motion.MoveTo(ctx, status.ID, axis, position); err != nil {
				slog.Error("traversal move failed",
					"component", "traversal",
					"task_id", taskID,
					"controller", status.ID,
					"axis", axis,
					"error", err,
				)
				return m.failWithCode("move %s axis: %v", traversal.ErrMotionFailed, axis, err)
			}
			slog.Info("traversal move sent",
				"component", "traversal",
				"task_id", taskID,
				"controller", status.ID,
				"axis", string(axis),
				"target", position,
			)
		}
	}

	motionComplete, paused := m.waitForMotionComplete(ctx, point, taskID)
	slog.Info("traversal motion phase ended",
		"component", "traversal",
		"task_id", taskID,
		"point_index", pointIndex+1,
		"motion_complete", motionComplete,
		"paused", paused,
	)
	if !motionComplete {
		m.stopMotionAxes()
		if paused {
			slog.Info("traversal motion interrupted by pause",
				"component", "traversal",
				"task_id", taskID,
				"point_index", pointIndex+1,
			)
			return nil // 暂停导致的中断，不算错误
		}
		slog.Error("traversal motion timeout",
			"component", "traversal",
			"task_id", taskID,
			"point_index", pointIndex+1,
		)
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
				slog.Error("traversal acquisition failed",
					"component", "traversal",
					"task_id", taskID,
					"point_index", pointIndex+1,
					"device_id", config.DeviceID,
					"error", "no data available",
				)
				return m.failWithCode("no data available for device %s", traversal.ErrAcquisitionFailed, config.DeviceID)
			}
			resultValues = valuesForChannels(payload, config.Channels)
		} else {
			averaged, err := m.collectAveragedSamples(taskID, config.DeviceID, config.Channels, samplesPerPoint)
			if err != nil {
				slog.Error("traversal averaged sampling failed",
					"component", "traversal",
					"task_id", taskID,
					"point_index", pointIndex+1,
					"error", err,
				)
				return m.failWithCode("averaged sampling failed: %v", traversal.ErrAcquisitionFailed, err)
			}
			resultValues = averaged
		}

		if len(resultValues) != len(config.Channels) {
			slog.Error("traversal channel mismatch",
				"component", "traversal",
				"task_id", taskID,
				"point_index", pointIndex+1,
				"expected", len(config.Channels),
				"got", len(resultValues),
			)
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
			// 仍在重试上限内则继续；最后一次仍失败则视为接受（避免静默丢点）。
			// 重试前等待 retryWaitInterval，让设备产出新一帧，否则 GetLatestData
			// 会立即返回同一份缓存数据，重试毫无意义。
			if attempt < maxAttempts-1 {
				m.sleepWithTaskCheck(taskID, retryWaitInterval)
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
	// 实时插值（落盘和断点恢复都需要）：失败仅写 warning，不阻塞本点保存。
	// BuildRawPressure 内部按 unitProvider 查通道 Unit 并归一化到 Pa+表压，
	// unitProvider 为 nil 时走降级路径（保持原值），保证离线/旧测试不崩。
	_, input, hasAll := BuildRawPressure(resultValues, config.ChannelLabels, config.DeviceID, unitProvider, config.PProbePressureType)
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
			slog.Error("traversal save failed",
				"component", "traversal",
				"task_id", taskID,
				"point_index", pointIndex+1,
				"error", err,
			)
			return m.failWithCode("write traversal point: %v", traversal.ErrSaveFailed, err)
		}
	}
	checkpointSavePath := config.SavePath
	if pathSink, ok := m.sink.(interface{ OutputPath() string }); ok {
		if outputPath := pathSink.OutputPath(); outputPath != "" {
			checkpointSavePath = outputPath
		}
	}

	m.mu.Lock()
	m.status.Results = append(m.status.Results, result)
	m.status.CurrentPoint++
	m.status.ValidationWarnings = nil
	allDone := m.status.CurrentPoint >= len(m.config.Path)
	completedCount := m.status.CurrentPoint
	// 收集 store.Save 所需快照；store.Save 必须在锁外执行，
	// 避免磁盘 I/O 卡住整个 m.mu，导致 Status()/前端轮询全部阻塞。
	var pendingSave bool
	var saveTaskID string
	var saveStatus traversal.Status
	if allDone {
		m.status.State = traversal.StateCompleted
		m.status.CurrentPointPhase = ""
		if m.store != nil {
			pendingSave = true
			saveTaskID = m.config.TaskID
			saveStatus = m.status
			saveStatus.Results = append([]traversal.PointResult(nil), m.status.Results...)
		}
	}
	// 用于断点保存的快照（在锁内复制，避免锁外访问竞态）
	checkpointPath := checkpointSavePath
	checkpointPoints := append([]traversal.Point(nil), m.config.Path...)
	m.mu.Unlock()

	if pendingSave {
		if err := m.store.Save(saveTaskID, saveStatus); err != nil {
			slog.Error("traversal final save failed",
				"component", "traversal",
				"task_id", saveTaskID,
				"error", err,
			)
			return fmt.Errorf("save traversal result: %v", err)
		}
		slog.Info("traversal result saved on completion",
			"component", "traversal",
			"task_id", saveTaskID,
			"completed_points", completedCount,
		)
	}

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
//
// 近零值处理：当 |prevVal| 小于 stabilityNearZeroEpsilon 时，
// 百分比变化在数值上虽然可计算，但语义无意义（极小分母会把任何微小波动放大）；
// 改用绝对阈值比较（|curVal-prevVal| ≤ threshold）。
func (m *TraversalManager) isStable(prev, cur map[int]float64, threshold float64) bool {
	if prev == nil || cur == nil {
		return false
	}
	for k, prevVal := range prev {
		curVal, ok := cur[k]
		if !ok {
			return false
		}
		// 近零值（含精确为 0）：使用绝对差阈值，避免分母被放大
		if math.Abs(prevVal) < stabilityNearZeroEpsilon {
			if math.Abs(curVal-prevVal) > threshold {
				return false
			}
			continue
		}
		// 计算百分比变化
		change := math.Abs((curVal-prevVal)/prevVal) * 100
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
			slog.Warn("traversal averaged sampling cancelled",
				"component", "traversal",
				"task_id", taskID,
				"valid_samples", validSamples,
				"target_samples", samplesPerPoint,
			)
			return nil, fmt.Errorf("acquisition cancelled")
		}
		if time.Now().After(deadline) {
			slog.Warn("traversal averaged sampling timeout",
				"component", "traversal",
				"task_id", taskID,
				"valid_samples", validSamples,
				"target_samples", samplesPerPoint,
			)
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
		slog.Error("traversal no valid samples",
			"component", "traversal",
			"task_id", taskID,
		)
		return nil, fmt.Errorf("no valid samples collected within timeout")
	}

	averaged := make(map[int]float64, len(totals))
	for k, total := range totals {
		averaged[k] = total / float64(validSamples)
	}
	return averaged, nil
}

// waitForMotionComplete 等待运动完成（带超时和取消检查）。
// 返回值：(completed, paused)
//   - completed=true：所有参与运动的轴已到位
//   - completed=false, paused=true：因暂停中断（非错误，RunCurrentPoint 应静默退出）
//   - completed=false, paused=false：超时或停止中断（错误路径）
//
// 暂停信息通过返回值传递而非全局标志位，避免与 Resume 并发清零产生竞态。
func (m *TraversalManager) waitForMotionComplete(ctx context.Context, point traversal.Point, taskID string) (completed bool, paused bool) {
	ticker := time.NewTicker(motionCompletePoll)
	defer ticker.Stop()
	deadline := time.Now().Add(motionCompleteTimeout)

	// 读取 motionAxes 配置，仅检查参与遍历运动的轴是否到位
	m.mu.RLock()
	motionAxes := m.config.MotionAxes
	m.mu.RUnlock()
	// 容错预处理：与 RunCurrentPoint 保持一致，否则 controllerId 全部不匹配时
	// targets 为 nil → allReached 恒为 true → 跳过运动直接进入「稳定中」阶段
	motionAxes = resolveMotionAxes(motionAxes, m.motion.StatusAll(ctx))

	for {
		select {
		case <-ctx.Done():
			return false, false
		case <-ticker.C:
			// 优先检查到位：运动在 deadline 边界附近完成时，先判到位避免假超时
			if motionTargetsReached(m.motion.StatusAll(ctx), point, motionAxes) {
				return true, false
			}
			// 再检查暂停/停止
			m.mu.Lock()
			isPaused := m.isPaused
			stopped := m.isStopped
			m.mu.Unlock()
			if stopped {
				return false, false
			}
			if isPaused {
				return false, true
			}
			if time.Now().After(deadline) {
				return false, false
			}
		}
	}
}
