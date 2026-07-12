// Package usecase — traversal 采集执行（从 traversal.go 拆分）
//
// 包含 RunCurrentPoint 单点采集主流程，以及稳定等待、平均采样、运动到位
// 等子流程。RunTraversalLoop 仍在 traversal.go，本文件聚焦 "如何采到一个点"。
package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"time"

	"wind-daq/services/api-go/internal/core/traversal"
	"wind-daq/services/api-go/internal/ports"
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

	// 每个遍历点都会触发一次，属于高频日志，降级为 Debug 避免刷屏。
	// 进度信息已经通过 status API 推送到 UI，LOG 画面里只在调试时才需要看到。
	// 4 轴全输出：系统已支持 X/Y/Z/U 四轴运动控制，调试时需看到完整坐标
	// 才能排查 4 轴定位/插值问题，仅输出 X/Y 会丢失 Z/U 上下文。
	slog.Debug("traversal running point",
		"component", "traversal",
		"task_id", taskID,
		"point_index", pointIndex+1,
		"total_points", len(config.Path),
		"coordinates", fmt.Sprintf("(X=%.2f, Y=%.2f, Z=%.2f, U=%.2f)", point.X, point.Y, point.Z, point.U),
	)

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
				slog.Error("traversal move failed",
					"component", "traversal",
					"task_id", taskID,
					"controller", status.ID,
					"axis", axis,
					"error", err,
				)
				return m.failWithCode("move %s axis: %v", traversal.ErrMotionFailed, axis, err)
			}
		}
	}

	motionComplete := m.waitForMotionComplete(ctx, point, taskID)
	if !motionComplete {
		m.stopMotionAxes()
		// 在锁内读取并清零 motionPauseCancelled，避免与 waitForMotionComplete /
		// ResumeFromCheckpoint 的写并发产生数据竞争。
		m.mu.Lock()
		paused := m.motionPauseCancelled
		if paused {
			m.motionPauseCancelled = false
		}
		m.mu.Unlock()
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

	// v2：检查会话是否被取消（Stop 调用后快速退出）
	m.mu.RLock()
	session := m.session
	m.mu.RUnlock()
	if session != nil && session.ctx.Err() != nil {
		return m.failWithCode("traversal cancelled during stabilization", traversal.ErrUnknown)
	}

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
		// skip 点也需走 commitPointV2 持久化（PointStatusSkipped），
		// 否则崩溃恢复后 skip 点从 CompletedPoints 中"消失"会重新采点。
		// 与正常分支一致：分配单调递增 commitSeq，提交成功后才推进 snapshot.CommitSeq。
		m.mu.Lock()
		commitSeq := m.session.snapshot.CommitSeq + 1
		session := m.session
		m.mu.Unlock()

		now := time.Now().UnixMilli()
		skipResult := traversal.PointResult{
			TaskID:             taskID,
			CommitSeq:          commitSeq,
			PointStatus:        traversal.PointStatusSkipped,
			PointIndex:         pointIndex,
			Point:              point,
			Timestamp:          now,
			CompletedAt:        now,
			Values:             resultValues,
			SampleCount:        samplesPerPoint,
			DwellTimeElapsed:   config.DwellTimeMs,
			ValidationWarnings: lastWarnings,
		}
		if err := m.commitPointV2(taskID, &skipResult); err != nil {
			return m.failWithCode("commit skip point %d failed: %v", traversal.ErrSaveFailed, pointIndex+1, err)
		}
		// 提交成功后才推进 snapshot.CommitSeq（线性化点 = Checkpoint 持久化成功）
		m.mu.Lock()
		session.snapshot.CommitSeq = commitSeq
		m.status.Results = append(m.status.Results, skipResult)
		m.status.CurrentPoint++
		m.status.CommittedPoints = int(commitSeq)
		m.status.ValidationWarnings = nil
		allDone := m.status.CurrentPoint >= len(m.config.Path)
		m.mu.Unlock()
		if allDone {
			m.mu.Lock()
			m.status.State = traversal.StateCompleted
			m.status.CurrentPointPhase = ""
			m.mu.Unlock()
		}
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
	// v2：生成单调递增提交序号（仅在 commitPointV2 成功后才推进 snapshot.CommitSeq）
	m.mu.Lock()
	commitSeq := m.session.snapshot.CommitSeq + 1
	runSession := m.session
	m.mu.Unlock()

	now := time.Now().UnixMilli()
	result := traversal.PointResult{
		TaskID:             taskID,
		CommitSeq:          commitSeq,
		PointStatus:        traversal.PointStatusCompleted,
		PointIndex:         pointIndex,
		Point:              point,
		Timestamp:          now,
		StartedAt:          now - int64(dwellTime) - int64(samplesPerPoint)*int64(acquisitionBatchPoll/time.Millisecond),
		CompletedAt:        now,
		Values:             resultValues,
		SampleCount:        samplesPerPoint,
		DwellTimeElapsed:   dwellTime,
		Calculated:         calculated,
		ValidationWarnings: lastWarnings,
	}

	// 三阶段提交协议（v2 可靠存储）：
	//   阶段1：CSV Append + Sync（可读视图，先拿 RowHash）
	//   阶段2：结果日志 AppendPrepared + Sync（权威数据源，含 CSVRowHash）
	//   阶段3：Checkpoint 原子替换（恢复锚点）
	// 提交成功后才推进 snapshot.CommitSeq（线性化点 = Checkpoint 持久化成功）
	if err := m.commitPointV2(taskID, &result); err != nil {
		return m.failWithCode("commit point %d failed: %v", traversal.ErrSaveFailed, pointIndex+1, err)
	}

	// v2：提交成功后才推进 snapshot.CommitSeq
	m.mu.Lock()
	runSession.snapshot.CommitSeq = commitSeq
	m.mu.Unlock()

	// 向后兼容：旧 sink 路径仍执行，但不参与事务回滚
	if m.sink != nil {
		if err := m.sink.WriteTraversalPoint(result); err != nil {
			slog.Error("traversal old sink save failed",
				"component", "traversal",
				"task_id", taskID,
				"point_index", pointIndex+1,
				"error", err,
			)
			// 旧 sink 失败不触发 v2 回滚，避免双写冲突导致数据丢失
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
	m.status.CommittedPoints = int(commitSeq)
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

	// v2 回退：未注入 v2 checkpoint 端口时，沿用旧 saveCheckpoint 逻辑（每10个点）
	m.mu.RLock()
	checkpointPortV2 := m.checkpointPort
	m.mu.RUnlock()
	if checkpointPortV2 == nil {
		// 与 Cursor DAQ 一致：每完成 10 个点或最后一个点时保存断点
		if checkpointPath != "" && len(checkpointPoints) > 0 {
			if (pointIndex+1)%checkpointInterval == 0 || allDone {
				m.saveCheckpoint(checkpointPoints, completedCount, checkpointPath)
			}
		}
	}

	// 测试成功完成后清理断点文件
	if allDone {
		m.ClearCheckpoint()
	}
	return nil
}

// commitPointV2 三阶段提交：CSV → 结果日志 → Checkpoint。
//
// 阶段顺序与回滚设计：
//   - 阶段1：CSV Append + Sync（先拿 RowHash，写回 result.CSVRowHash）
//   - 阶段2：结果日志 AppendPrepared + Sync（权威数据源，携带 CSVRowHash 供一致性校验）
//   - 阶段3：Checkpoint 原子替换（恢复锚点）
//
// 线性化点：阶段3 Checkpoint 持久化成功。只有 commitPointV2 返回 nil 后调用方才推进
// snapshot.CommitSeq，保证崩溃恢复时 checkpoint 中的 CommitSeq 严格反映已确认提交。
//
// 回滚策略：任何阶段失败都把 CSV/结果日志回滚到 commitSeq-1（上一个已确认点）。
// commitSeq-1=0 表示清空到初始状态（仅表头/空文件）。回滚错误用 errors.Join 聚合，
// 避免静默丢失回滚失败信息导致数据不一致。
//
// result 通过指针传入：阶段1 写入的 CSVRowHash 会被阶段2 读到，并最终反映到调用方
// 持有的 result 中（用于 status.Results 等后续路径）。
func (m *TraversalManager) commitPointV2(taskID string, result *traversal.PointResult) error {
	m.mu.RLock()
	csvPort := m.csvPort
	resultLogPort := m.resultLogPort
	checkpointPort := m.checkpointPort
	checkpointStore := m.checkpointStore
	session := m.session
	m.mu.RUnlock()

	// 若 v2 端口/存储均未注入，直接返回成功（向后兼容旧装配路径，仅走 sink）
	if csvPort == nil && resultLogPort == nil && checkpointPort == nil && checkpointStore == nil {
		return nil
	}

	// 使用 session ctx：Stop 调用 session.Cancel() 后能立即中断 I/O，
	// 避免 commitPointV2 在已停止的任务上继续刷盘。
	// session 为 nil 时（理论上不应发生，Start 必创建 session）回退到 Background。
	ctx := context.Background()
	if session != nil {
		ctx = session.ctx
	}
	commitSeq := result.CommitSeq

	// 阶段1：CSV Append + Sync（可读视图，先拿 RowHash 写回 result）
	if csvPort != nil {
		summary, err := csvPort.Append(ctx, *result)
		if err != nil {
			slog.Error("traversal csv append failed",
				"component", "traversal", "task_id", taskID,
				"commit_seq", commitSeq, "error", err)
			return fmt.Errorf("csv append: %w", err)
		}
		result.CSVRowHash = summary.RowHash
		if err := csvPort.Sync(ctx); err != nil {
			slog.Error("traversal csv sync failed",
				"component", "traversal", "task_id", taskID,
				"commit_seq", commitSeq, "error", err)
			// CSV Sync 失败：截断刚写入但未持久化的 CSV 行
			rbErr := rollbackCSV(ctx, csvPort, commitSeq-1)
			return errors.Join(fmt.Errorf("csv sync: %w", err), rbErr)
		}
	}

	// 阶段2：结果日志 AppendPrepared + Sync（权威数据源，含 CSVRowHash）
	if resultLogPort != nil {
		if err := resultLogPort.AppendPrepared(ctx, *result); err != nil {
			slog.Error("traversal result log append failed",
				"component", "traversal", "task_id", taskID,
				"commit_seq", commitSeq, "error", err)
			// 结果日志 Append 失败：回滚 CSV 到 commitSeq-1
			rbErr := rollbackCSV(ctx, csvPort, commitSeq-1)
			return errors.Join(fmt.Errorf("result log append: %w", err), rbErr)
		}
		if err := resultLogPort.Sync(ctx); err != nil {
			slog.Error("traversal result log sync failed",
				"component", "traversal", "task_id", taskID,
				"commit_seq", commitSeq, "error", err)
			// 结果日志 Sync 失败：截断结果日志 + 回滚 CSV
			var rbErrs []error
			if rbErr := resultLogPort.TruncateAfter(ctx, commitSeq-1); rbErr != nil {
				rbErrs = append(rbErrs, fmt.Errorf("result log rollback: %w", rbErr))
			}
			if rbErr := rollbackCSV(ctx, csvPort, commitSeq-1); rbErr != nil {
				rbErrs = append(rbErrs, rbErr)
			}
			return errors.Join(append([]error{fmt.Errorf("result log sync: %w", err)}, rbErrs...)...)
		}
	}

	// 阶段3：Checkpoint 原子替换（恢复锚点）
	// cp.Snapshot 显式设置 CommitSeq = commitSeq，使 checkpoint 内部一致
	// （反映"本次提交完成后的状态"），ResumeFromCheckpoint 读取时无需再推断。
	m.mu.RLock()
	var snapshot traversal.TraversalRunSnapshot
	if session != nil {
		snapshot = session.snapshot
	}
	m.mu.RUnlock()
	snapshot.CommitSeq = commitSeq
	snapshot.CommittedPoints = int(commitSeq)
	configJSON, _ := json.Marshal(snapshot.Config)
	cp := traversal.Checkpoint{
		Version:         traversal.CheckpointVersion,
		TaskID:          taskID,
		State:           traversal.StateRunning,
		Snapshot:        snapshot,
		Config:          configJSON,
		CompletedPoints: int(commitSeq),
		TotalPoints:     snapshot.TotalPoints,
		SavePath:        snapshot.CSVPath,
		CreatedAt:       time.Now().UnixMilli(),
	}

	var checkpointErr error
	if checkpointPort != nil {
		checkpointErr = checkpointPort.Save(ctx, cp)
	} else if checkpointStore != nil {
		// 回退：直接使用底层 CheckpointStore 原子写入（与 saveCheckpoint 同机制）
		data, marshalErr := json.MarshalIndent(cp, "", "  ")
		if marshalErr != nil {
			checkpointErr = fmt.Errorf("marshal checkpoint: %w", marshalErr)
		} else if err := checkpointStore.Write(snapshot.CSVPath+".checkpoint.json", data); err != nil {
			checkpointErr = fmt.Errorf("checkpoint write: %w", err)
		}
	}
	if checkpointErr != nil {
		slog.Error("traversal checkpoint save failed",
			"component", "traversal", "task_id", taskID,
			"commit_seq", commitSeq, "error", checkpointErr)
		// Checkpoint 失败：回滚 CSV + 结果日志到 commitSeq-1
		var rbErrs []error
		if rbErr := rollbackCSV(ctx, csvPort, commitSeq-1); rbErr != nil {
			rbErrs = append(rbErrs, rbErr)
		}
		if resultLogPort != nil {
			if rbErr := resultLogPort.TruncateAfter(ctx, commitSeq-1); rbErr != nil {
				rbErrs = append(rbErrs, fmt.Errorf("result log rollback: %w", rbErr))
			}
		}
		return errors.Join(append([]error{fmt.Errorf("checkpoint save: %w", checkpointErr)}, rbErrs...)...)
	}

	return nil
}

// rollbackCSV 安全回滚 CSV 到指定 commitSeq。
// csvPort 为 nil 时直接跳过（v2 端口未注入的兼容路径）。
// commitSeq=0 表示截断到初始状态（仅保留表头）。
func rollbackCSV(ctx context.Context, csvPort ports.TraversalCSVPort, commitSeq uint64) error {
	if csvPort == nil {
		return nil
	}
	if err := csvPort.TruncateAfter(ctx, commitSeq); err != nil {
		return fmt.Errorf("csv rollback to seq %d: %w", commitSeq, err)
	}
	return nil
}

// updatePhase 更新当前阶段状态
func (m *TraversalManager) updatePhase(taskID string, state traversal.State, phase traversal.PointPhase, pointIndex, totalPoints int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.status.TaskID != taskID || m.session == nil || m.session.taskID != taskID {
		return
	}
	if m.status.State == traversal.StatePaused || m.status.State == traversal.StateStopped || m.status.State == traversal.StateError {
		return
	}
	m.status.State = state
	m.status.CurrentPointPhase = phase
	m.status.CurrentPointIndex = pointIndex
	m.status.TotalPoints = totalPoints
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
			// 检查是否暂停或停止；若已暂停顺手在同一把写锁内置位 motionPauseCancelled，
			// 让 RunCurrentPoint 在锁内读到的状态保持一致，避免与 Resume 的清零并发竞态。
			m.mu.Lock()
			isPaused := m.isPaused
			stopped := m.isStopped || isPaused
			if stopped && isPaused {
				m.motionPauseCancelled = true
			}
			m.mu.Unlock()
			if stopped {
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
					if axis.Moving || math.Abs(axis.Position-target) > 0.01 {
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
