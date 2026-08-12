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
	"math"
	"sort"
	"time"

	"shared.local/device-sdk/go/pkg/slog"
	"wind-daq/services/api-go/internal/core/motion"
	"wind-daq/services/api-go/internal/core/traversal"
	"wind-daq/services/api-go/internal/ports"
)

// stabilityNearZeroEpsilon 稳定性判断中"近零值"的判定阈值
// 当 |prevVal| 小于该值时，切换为绝对差阈值比较，避免极小分母放大波动。
// 取值理由：传感器分辨率典型量级（< 1e-3 物理单位）远大于该值，不会误判正常读数。
const stabilityNearZeroEpsilon = 1e-9

// 注：motionInterruptReason 类型和常量已提升到 core/traversal 包为导出类型
// traversal.MotionInterruptReason，校准模块跨包引用（spec AD-1）。

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
	acqController := m.acquisitionController
	pointIndex := m.status.CurrentPoint
	point := config.Path[pointIndex]
	// per-point DwellMs 优先于全局 DwellTimeMs：仅 custom 布局点位会携带 DwellMs。
	// effectiveDwellMs 供 Phase 2 等待时长与 PointResult.DwellTimeElapsed 统一使用；
	// waitForStabilization 内部也通过 resolveEffectiveDwellMs 读取同一语义
	effectiveDwellMs := resolveEffectiveDwellMs(point, config.DwellTimeMs)
	m.mu.Unlock()
	taskID := config.TaskID
	// 多设备采集态校验：通道可能跨设备绑定（如五孔在 A、大气压/温度在 B），
	// 任一设备非 Acquiring 时进入"类暂停"无限期等待恢复（spec-traversal-acquisition-stop）。
	// 等待前显式置 StateMoving + waiting_acquisition（首点/后续点公开状态一致，
	// 避免上一点残留 StateSaving），设备恢复后置 PhaseMoving 继续；恢复前不下发运动命令。
	// 点位开始等待时采样尚未开始，恢复后无需重建 freshness 基线（lastTimestamps 在采样入口建立）。
	if acqController != nil {
		if len(abnormalAcquisitionDevices(acqController, config)) > 0 {
			m.updatePhase(taskID, traversal.StateMoving, traversal.PhaseWaitingForAcquisition, pointIndex, len(config.Path))
		}
		classify := func() []acquisitionDeviceState {
			return abnormalAcquisitionDevices(acqController, config)
		}
		if _, err := m.waitForAcquisitionResume(taskID, classify); err != nil {
			return err
		}
	}
	// 采样通道按设备分组：每组内部键 ↔ 硬件索引一一对应，
	// 采集/稳定等待共用，避免每次调用重复解析 ChannelRefs。
	channelGroups := groupChannelsByDevice(config.Channels, config.ResolvedChannelRefs())

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
	m.motionCommandMu.Lock()
	m.mu.RLock()
	paused := m.isPaused
	stopped := m.isStopped
	m.mu.RUnlock()
	if paused || stopped {
		m.motionCommandMu.Unlock()
		return nil
	}
	for _, status := range controllerStatuses {
		if !status.Connected {
			continue
		}
		for axis, position := range availableAxisTargets(status, point, motionAxes) {
			m.mu.RLock()
			paused = m.isPaused
			stopped = m.isStopped
			m.mu.RUnlock()
			if paused || stopped {
				m.motionCommandMu.Unlock()
				return nil
			}
			if err := m.motion.MoveTo(ctx, status.ID, axis, position); err != nil {
				m.motionCommandMu.Unlock()
				slog.Error("traversal move failed",
					"component", "traversal",
					"task_id", taskID,
					"controller", status.ID,
					"axis", axis,
					"error", err,
				)
				stopErr := m.stopMotionWithEmergencyFallback()
				if stopErr != nil {
					return m.failWithCode("move %s axis: %v; stopping traversal axes also failed: %v", traversal.ErrMotionFailed, axis, err, stopErr)
				}
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
	m.motionCommandMu.Unlock()

	motionComplete, reason, failure := m.waitForMotionComplete(ctx, point, taskID, pointIndex)
	slog.Info("traversal motion phase ended",
		"component", "traversal",
		"task_id", taskID,
		"point_index", pointIndex+1,
		"motion_complete", motionComplete,
		"reason", reason,
		"has_failure", failure != nil,
	)
	// 运动安全故障：立即调用 handleMotionSafetyFailure 分发 Stop/EmergencyStop
	// 故障现场已由 waitForMotionComplete 快照到 failure 中，本路径不再读硬件
	if failure != nil {
		return m.handleMotionSafetyFailure(failure)
	}
	if !motionComplete {
		// 非故障中断：按 waitForMotionComplete 返回的不可变 reason 分支处理。
		// 原实现事后读 m.isPaused 推断中断类型，Resume 在等待函数返回与读取之间
		// 清零标志会导致暂停被误判为超时；改为直接用 reason 判断避免竞态。
		switch reason {
		case traversal.MotionInterruptPaused:
			slog.Info("traversal motion interrupted by pause",
				"component", "traversal",
				"task_id", taskID,
				"point_index", pointIndex+1,
			)
			return nil // 暂停导致的中断，不算错误
		case traversal.MotionInterruptStopped, traversal.MotionInterruptCancelled:
			// 用户停止 / ctx 取消——停止运动轴，由上层 RunTraversalLoop 通过 session.IsDone() 退出
			m.stopMotionAxes()
			slog.Info("traversal motion interrupted by stop/cancel",
				"component", "traversal",
				"task_id", taskID,
				"point_index", pointIndex+1,
				"reason", reason,
			)
			return nil
		case traversal.MotionInterruptTimeout:
			// 120s 兜底超时——停止运动轴并返回 ErrMotionTimeout
			m.stopMotionAxes()
			slog.Error("traversal motion timeout",
				"component", "traversal",
				"task_id", taskID,
				"point_index", pointIndex+1,
			)
			return m.failWithCode("motion did not complete for point %d (timeout)", traversal.ErrMotionTimeout, pointIndex+1)
		default:
			// traversal.MotionInterruptNone 不应到达（completed=false 时 reason 必非 None）
			m.stopMotionAxes()
			return m.failWithCode("motion did not complete for point %d (unknown reason)", traversal.ErrUnknown, pointIndex+1)
		}
	}

	// per-point Test=false 跳过分支：走到位置后直接进入下一步，不采集不保存
	// 结果 CSV 仍占一行（数据列空字符串由 buildRow 天然支持 nil Values），
	// 用 PointStatusNotTested 区别于 PointStatusSkipped（数据验证 OnInvalid=skip 跳过）
	// 与 Skipped 同样算 Committed，崩溃恢复时不重走该点
	if point.Test != nil && !*point.Test {
		slog.Info("traversal point skipped by config (test=false)",
			"component", "traversal",
			"task_id", taskID,
			"point_index", pointIndex+1,
			"coordinates", fmt.Sprintf("(X=%s, Y=%s, Z=%s, U=%s)",
				formatCoord(point.X), formatCoord(point.Y), formatCoord(point.Z), formatCoord(point.U)),
		)

		// 分配单调递增 commitSeq（与 normal/skip 分支一致）
		m.mu.Lock()
		commitSeq := m.session.snapshot.CommitSeq + 1
		m.mu.Unlock()

		now := time.Now().UnixMilli()
		notTestedResult := traversal.PointResult{
			TaskID:           taskID,
			CommitSeq:        commitSeq,
			PointStatus:      traversal.PointStatusNotTested,
			PointIndex:       pointIndex,
			Point:            point,
			Timestamp:        now,
			StartedAt:        now,
			CompletedAt:      now,
			Values:           nil, // 不采集，CSV 数据列由 buildRow 写空字符串
			SampleCount:      0,
			DwellTimeElapsed: 0, // 未进入稳定等待阶段
		}
		// notTested 分支无 ValidationWarnings，clearValidationWarnings=false 不必清空
		return m.commitAndAdvance(taskID, &notTestedResult, false)
	}

	// 阶段2：等待稳定
	m.updatePhase(taskID, traversal.StateStabilizing, traversal.PhaseStabilizing, pointIndex, len(config.Path))
	stabFailure := m.waitForStabilization(taskID, point, pointIndex, channelGroups)
	// 稳定阶段检测到运动安全故障：立即停止并返回错误
	// 与 Moving 阶段共用 handleMotionSafetyFailure，保持故障处理一致性
	if stabFailure != nil {
		return m.handleMotionSafetyFailure(stabFailure)
	}

	// v2：检查会话是否被取消（Stop 调用后快速退出）
	m.mu.RLock()
	session := m.session
	m.mu.RUnlock()
	if session != nil && session.ctx.Err() != nil {
		return nil
	}

	// 阶段3：采集中（含数据验证 + 可选重试）
	m.updatePhase(taskID, traversal.StateAcquiring, traversal.PhaseAcquiring, pointIndex, len(config.Path))
	// per-point 优先：point.Samples 非 nil 时覆盖全局 config.SamplesPerPoint
	// 仅 custom 布局点位会携带 Samples；line/rectangle/sector 生成的 Point 字段为 nil 走全局
	samplesPerPoint := config.SamplesPerPoint
	if point.Samples != nil {
		samplesPerPoint = *point.Samples
	}
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
	// StartedAt/CompletedAt 语义（与 CSV writer 表头契约对齐）：
	//   - StartedAt：本点首次采样尝试开始时间（进入采集阶段后的第一次 collectAveragedSamples 调用前）
	//   - CompletedAt：本点最终被接受的采样尝试结束时间（最后一次 collectAveragedSamples 返回后）
	// 二者差值即"单点总耗时"，包含验证失败重试间的 retryWaitInterval 等待。
	// 不含稳定等待 dwell 时间——dwell 由 DwellTimeElapsed 单独记录。
	//
	// 历史问题：原先 StartedAt 在每次成功 attempt 内被覆盖，导致重试等待被排除，
	// 与 CSV 契约"单点总耗时"声明不一致。修复后首次尝试开始时间被锁定保留。
	firstAttemptStartMs := time.Now().UnixMilli()
	var samplingEndMs int64
	for attempt := 0; attempt < maxAttempts; attempt++ {
		averaged, err := m.collectAveragedSamples(taskID, channelGroups, samplesPerPoint)
		callEnd := time.Now()
		if err != nil {
			if m.isTaskCancelled(taskID) {
				return nil
			}
			slog.Error("traversal sampling failed",
				"component", "traversal",
				"task_id", taskID,
				"point_index", pointIndex+1,
				"error", err,
			)
			return m.failWithCode("sampling failed: %v", traversal.ErrAcquisitionFailed, err)
		}
		// CompletedAt 始终对齐最后一次成功采样结束时间；firstAttemptStartMs 锁定不变，
		// 保证 StartedAt↔首次尝试、CompletedAt↔最终采样窗口右端，二者差值 = 单点总耗时。
		samplingEndMs = callEnd.UnixMilli()
		resultValues = averaged

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
		m.mu.Unlock()

		now := time.Now().UnixMilli()
		skipResult := traversal.PointResult{
			TaskID:      taskID,
			CommitSeq:   commitSeq,
			PointStatus: traversal.PointStatusSkipped,
			PointIndex:  pointIndex,
			Point:       point,
			Timestamp:   now,
			StartedAt:   firstAttemptStartMs,
			CompletedAt: samplingEndMs,
			Values:      resultValues,
			SampleCount: samplesPerPoint,
			// 使用 effectiveDwellMs 反映实际等待时长（per-point 优先于全局）
			DwellTimeElapsed:   effectiveDwellMs,
			ValidationWarnings: lastWarnings,
		}
		// skip 分支采集了数据但被验证规则跳过，clearValidationWarnings=true 清空告警
		return m.commitAndAdvance(taskID, &skipResult, true)
	}

	// 阶段4：保存中
	m.updatePhase(taskID, traversal.StateSaving, traversal.PhaseSaving, pointIndex, len(config.Path))

	// 使用 effectiveDwellMs 反映实际等待时长（per-point 优先于全局）
	dwellTime := effectiveDwellMs
	// 实时插值（落盘和断点恢复都需要）：失败仅写 warning，不阻塞本点保存。
	// buildRawPressureForProbe 按当前探针类型的策略标签集装配并归一化到 Pa+表压，
	// unitProvider 为 nil 时走降级路径（保持原值），保证离线/旧测试不崩。
	//
	// 三态 Status 填充逻辑抽取到 classifyCalculatedResult 中,便于单元测试覆盖;
	// 与 UI 实时插值卡片三态(绿色/橙色/红色)一一对应,便于 CSV 排障。
	strategy, strategyOK := probeStrategyFor(config.ProbeType)
	var (
		probeIn   probeCalcInput
		hasAll    bool
		interpRes probeCalcResult
		interpErr error
	)
	if strategyOK {
		_, probeIn, hasAll = buildRawPressureForProbe(resultValues, config.ChannelLabels, config.ResolvedChannelRefs(), unitProvider, config.PProbePressureType, strategy)
		if hasAll {
			interpRes, interpErr = m.CalculateRealtimeByProbe(config.ProbeType, probeIn)
		}
	}
	calculated := classifyCalculatedResult(
		strategyOK,
		hasAll,
		interpRes,
		interpErr,
		m.HasLoadedInterpolatorFor(config.ProbeType),
	)
	// v2：生成单调递增提交序号（仅在 commitPointV2 成功后才推进 snapshot.CommitSeq）
	m.mu.Lock()
	commitSeq := m.session.snapshot.CommitSeq + 1
	m.mu.Unlock()

	now := time.Now().UnixMilli()
	result := traversal.PointResult{
		TaskID:             taskID,
		CommitSeq:          commitSeq,
		PointStatus:        traversal.PointStatusCompleted,
		PointIndex:         pointIndex,
		Point:              point,
		Timestamp:          now,
		StartedAt:          firstAttemptStartMs,
		CompletedAt:        samplingEndMs,
		Values:             resultValues,
		SampleCount:        samplesPerPoint,
		DwellTimeElapsed:   dwellTime,
		Calculated:         calculated,
		ValidationWarnings: lastWarnings,
	}

	// normal 分支采集了数据，clearValidationWarnings=true 清空告警
	// commitAndAdvance 内部完成：commitPointV2 + 旧 sink 写 + snapshot 推进 +
	// allDone 处理 + v1 兼容 saveCheckpoint（每 10 个点）
	return m.commitAndAdvance(taskID, &result, true)
}

// errReturnToOriginAborted 标记回零被用户停止/取消（区别于回零运动失败/超时）。
// 完成阶段调用点据此区分：中止运行维持原语义返回错误；运动失败仅提示并照常完成。
var errReturnToOriginAborted = errors.New("return to origin cancelled")

// returnToOrigin moves every configured traversal axis back to coordinate zero
// and waits for the same motion-safety checks used at regular traversal points.
func (m *TraversalManager) returnToOrigin(taskID string, pointIndex int) error {
	m.mu.RLock()
	motionAccess := m.motion
	motionAxes := append([]traversal.MotionAxisBinding(nil), m.config.MotionAxes...)
	path := append([]traversal.Point(nil), m.config.Path...)
	session := m.session
	m.mu.RUnlock()
	if motionAccess == nil {
		return m.failWithCode("return to origin: motion manager is required", traversal.ErrMotionFailed)
	}

	ctx := context.Background()
	if session != nil {
		ctx = session.Context()
	}
	origin := originForPath(path)
	m.updatePhase(taskID, traversal.StateMoving, traversal.PhaseMoving, pointIndex, pointIndex)
	statuses := motionAccess.StatusAll(ctx)
	motionAxes = resolveMotionAxes(motionAxes, statuses)

	for _, status := range statuses {
		if !status.Connected {
			continue
		}
		for axis := range availableAxisTargets(status, origin, motionAxes) {
			if err := motionAccess.MoveTo(ctx, status.ID, axis, 0); err != nil {
				return m.failWithCode("return %s axis to origin: %v", traversal.ErrMotionFailed, axis, err)
			}
		}
	}

	for {
		completed, reason, failure := m.waitForMotionComplete(ctx, origin, taskID, pointIndex)
		if failure != nil {
			return m.handleMotionSafetyFailure(failure)
		}
		if completed {
			m.mu.RLock()
			paused := m.isPaused
			m.mu.RUnlock()
			if paused {
				if err := m.waitUntilResumed(ctx); err != nil {
					return err
				}
			}
			return nil
		}
		switch reason {
		case traversal.MotionInterruptStopped, traversal.MotionInterruptCancelled:
			return errReturnToOriginAborted
		case traversal.MotionInterruptPaused:
			if err := m.waitUntilResumed(ctx); err != nil {
				return err
			}
		default:
			return m.failWithCode("return to origin timed out", traversal.ErrMotionTimeout)
		}
	}
}

// completeAfterReturnToOrigin 处理完成阶段的回零结果。
// 数据此时已全部采完，回零失败不判测试失败：
//   - 用户停止/取消（errReturnToOriginAborted）：维持中止语义，原样返回错误；
//   - 运动失败/超时等：降级为 Status.Warning 提示，清除 returnToOrigin 内部
//     failWithCode 写入的错误态与故障快照，返回 nil 让流程继续置 Completed。
func (m *TraversalManager) completeAfterReturnToOrigin(taskID string, err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, errReturnToOriginAborted) {
		return err
	}
	m.mu.Lock()
	m.status.Warning = err.Error()
	m.status.LastError = ""
	m.status.LastErrorCode = ""
	m.status.MotionSafetyFailure = nil
	m.mu.Unlock()
	slog.Warn("traversal return to origin failed; run still completes",
		"component", "traversal",
		"task_id", taskID,
		"error", err,
	)
	return nil
}

func originForPath(path []traversal.Point) traversal.Point {
	origin := traversal.Point{X: math.NaN(), Y: math.NaN(), Z: math.NaN(), U: math.NaN()}
	for _, point := range path {
		if !math.IsNaN(point.X) {
			origin.X = 0
		}
		if !math.IsNaN(point.Y) {
			origin.Y = 0
		}
		if !math.IsNaN(point.Z) {
			origin.Z = 0
		}
		if !math.IsNaN(point.U) {
			origin.U = 0
		}
	}
	return origin
}

func (m *TraversalManager) waitUntilResumed(ctx context.Context) error {
	ticker := time.NewTicker(pausedLoopIdle)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return errReturnToOriginAborted
		case <-ticker.C:
			m.mu.RLock()
			paused := m.isPaused
			stopped := m.isStopped
			m.mu.RUnlock()
			if stopped {
				return errReturnToOriginAborted
			}
			if !paused {
				return nil
			}
		}
	}
}

// sanitizePointResultNaN 原地清洗 PointResult 中设备层异常可能引入的 NaN→0。
//
// 仅清洗 Calculated 与 Values：
//   - Calculated：插值器极端输入可能产出 NaN，且有 IsValid 守卫，清洗为 0 不影响
//     "是否有效"的判定（消费方先看 Valid 再读数值）。
//   - Values：设备层异常可能返回 NaN 采集值，前端无对应 null 语义契约，清洗为 0
//     保留通道存在性（不删 key），让前端仍能看到该通道有数据。
//
// 不清洗 Point.X/Y/Z/U：
//   - traversal.Point 已实现 MarshalJSON（point_json.go），NaN 会序列化为 null，
//     与前端 TraversalCoordValue = number | null 契约对齐；
//   - markAxesNaN 把"未配置轴"标记为 NaN 是运动恢复语义依据，清洗为 0 会让
//     availableAxisTargets 把这些轴当成"目标位置 0"发 MoveTo，破坏运动正确性。
//
// 此函数仅做设备层异常的防御性清洗，Point 的 NaN 由 Point.MarshalJSON 处理。
func sanitizePointResultNaN(result *traversal.PointResult) {
	if result == nil {
		return
	}
	if result.Calculated != nil {
		c := result.Calculated
		if math.IsNaN(c.Alpha) {
			c.Alpha = 0
		}
		if math.IsNaN(c.Beta) {
			c.Beta = 0
		}
		if math.IsNaN(c.Pt) {
			c.Pt = 0
		}
		if math.IsNaN(c.Ps) {
			c.Ps = 0
		}
		if math.IsNaN(c.Mach) {
			c.Mach = 0
		}
	}
	// Values 是 map[int]float64，设备层异常可能返回 NaN。遍历清洗避免序列化失败。
	// 不删除 key（保留通道存在性），仅把 NaN 值改 0，前端仍能看到该通道有数据。
	for ch, v := range result.Values {
		if math.IsNaN(v) {
			result.Values[ch] = 0
		}
	}
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

	// 使用 session ctx：Stop 调用 session.Cancel() 后能立即中断正常路径 I/O，
	// 避免 commitPointV2 在已停止的任务上继续刷盘。
	// session 为 nil 时（理论上不应发生，Start 必创建 session）回退到 Background。
	//
	// 回滚专用 ctx：阶段1/2/3 任何一步因 ctx 取消而失败时，回滚操作必须用独立 ctx。
	// 若沿用 session.ctx，port 实现会立即返回 ctx.Err()，导致刚 Append 的行无法截断，
	// 崩溃恢复时结果日志包含未确认的"半提交"记录，破坏权威数据源一致性。
	ctx := context.Background()
	rollbackCtx := context.Background()
	if session != nil {
		ctx = session.ctx
	}
	commitSeq := result.CommitSeq

	// NaN 清洗：仅对 Calculated 与 Values 做防御性清洗（设备层/插值器极端异常）。
	//
	// Point.X/Y/Z/U 不清洗：traversal.Point 已实现 MarshalJSON（point_json.go），
	// NaN 会序列化为 null，与前端 TraversalCoordValue = number | null 契约对齐。
	// markAxesNaN 标记的"未配置轴"NaN 在 result log / checkpoint 序列化时输出 null，
	// 反序列化时还原为 NaN，运动恢复（availableAxisTargets）仍能正确跳过这些轴。
	//
	// Calculated 有 IsValid 守卫但极端输入可能产出 NaN 数值，Values 来自设备层
	// 异常可能含 NaN，前端无 null 语义契约，清洗为 0 避免序列化失败阻塞整个提交。
	sanitizePointResultNaN(result)

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
			// 回滚用独立 ctx，避免 session 已取消时截断也失败
			rbErr := rollbackCSV(rollbackCtx, csvPort, commitSeq-1)
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
			rbErr := rollbackCSV(rollbackCtx, csvPort, commitSeq-1)
			return errors.Join(fmt.Errorf("result log append: %w", err), rbErr)
		}
		if err := resultLogPort.Sync(ctx); err != nil {
			slog.Error("traversal result log sync failed",
				"component", "traversal", "task_id", taskID,
				"commit_seq", commitSeq, "error", err)
			// 结果日志 Sync 失败：截断结果日志 + 回滚 CSV
			// 回滚用独立 ctx，避免 session 已取消时截断也失败
			var rbErrs []error
			if rbErr := resultLogPort.TruncateAfter(rollbackCtx, commitSeq-1); rbErr != nil {
				rbErrs = append(rbErrs, fmt.Errorf("result log rollback: %w", rbErr))
			}
			if rbErr := rollbackCSV(rollbackCtx, csvPort, commitSeq-1); rbErr != nil {
				rbErrs = append(rbErrs, rbErr)
			}
			return errors.Join(append([]error{fmt.Errorf("result log sync: %w", err)}, rbErrs...)...)
		}
	}

	// 阶段3：Checkpoint 原子替换（恢复锚点）
	// 通过 buildCheckpoint 统一构造 DTO（Important-5），与 saveCheckpoint 共享逻辑，
	// 保证字段语义一致。CommitSeq = commitSeq 是本次提交的权威水位，
	// helper 内部会强制 cp.Snapshot.CommitSeq/CommittedPoints/CompletedPoints 三者对齐。
	// managed 会话写入 v3（ProbeID 来自冻结的 managedOpts，BoundControllerIDs 在快照中）。
	m.mu.RLock()
	var snapshot traversal.TraversalRunSnapshot
	var probeID ProbeID
	if session != nil {
		snapshot = session.snapshot
		if session.managedOpts != nil {
			probeID = session.managedOpts.ProbeID
		}
	}
	m.mu.RUnlock()
	cp := buildCheckpoint(taskID, snapshot, commitSeq, snapshot.CSVPath, nil, traversal.StateRunning, probeID)

	var checkpointErr error
	if checkpointPort != nil {
		checkpointErr = checkpointPort.Save(ctx, cp)
	} else if checkpointStore != nil {
		// 回退：直接使用底层 CheckpointStore 原子写入（与 saveCheckpoint 同机制）
		data, marshalErr := json.MarshalIndent(cp, "", "  ")
		if marshalErr != nil {
			checkpointErr = fmt.Errorf("marshal checkpoint: %w", marshalErr)
		} else {
			// 路径派生收敛到 ResolveCheckpointPathFromCSV 单一真相源，
			// 与 saveCheckpoint / FileCheckpointPort.path() / activeIndex.Register 保持一致。
			cpPath := traversal.ResolveCheckpointPathFromCSV(snapshot.CSVPath)
			if err := checkpointStore.Write(cpPath, data); err != nil {
				checkpointErr = fmt.Errorf("checkpoint write: %w", err)
			}
		}
	}
	if checkpointErr != nil {
		slog.Error("traversal checkpoint save failed",
			"component", "traversal", "task_id", taskID,
			"commit_seq", commitSeq, "error", checkpointErr)
		// Checkpoint 失败：回滚 CSV + 结果日志到 commitSeq-1
		// 回滚用独立 ctx，避免 session 已取消时截断也失败
		var rbErrs []error
		if rbErr := rollbackCSV(rollbackCtx, csvPort, commitSeq-1); rbErr != nil {
			rbErrs = append(rbErrs, rbErr)
		}
		if resultLogPort != nil {
			if rbErr := resultLogPort.TruncateAfter(rollbackCtx, commitSeq-1); rbErr != nil {
				rbErrs = append(rbErrs, fmt.Errorf("result log rollback: %w", rbErr))
			}
		}
		return errors.Join(append([]error{fmt.Errorf("checkpoint save: %w", checkpointErr)}, rbErrs...)...)
	}

	// managed 会话：通知 registry checkpoint 已落盘（dual recovery index 登记）。
	// 路径与 checkpointPort/FileCheckpointPort.path() 派生一致（单一真相源）。
	notifyManagedCheckpointSaved(session, traversal.ResolveCheckpointPathFromCSV(snapshot.CSVPath))

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

// waitForStabilization 等待数据稳定，期间持续进行运动安全复检。
//
// 与 Moving 阶段的安全判定差异：
//   - 轴应已停止——EvaluateMotionSafety 期望 Arrived，任何其他故障 verdict 都表示异常
//   - 跨样本看门狗仍调用，但轴停止时 Observe 内部返回 nil（仅防御性观察意外运动）
//   - validateMotionStatuses 检查控制器掉线/急停（连续 3 快照）
//
// 返回 *MotionSafetyFailure 表示检测到故障，调用方应走 handleMotionSafetyFailure；
// 返回 nil 表示稳定等待正常结束（含暂停/停止中断，由调用方按非故障路径处理）。
//
// 复检间隔：fixed 与 adaptive 模式均使用 motionCompletePoll（100ms），
// 与 Moving 阶段对齐，保证故障检测延迟一致。

// resolveEffectiveDwellMs 解析单点有效稳定等待时长：per-point DwellMs 非 nil 时覆盖全局，
// 否则回退 globalMs。仅 custom 布局点位会携带 DwellMs；line/rectangle/sector 生成的
// Point 字段恒为 nil 走全局。语义在 RunCurrentPoint 入口与 waitForStabilization 内
// 保持一致，避免两处独立维护 fallback 逻辑漂移。
func resolveEffectiveDwellMs(point traversal.Point, globalMs int) int {
	if point.DwellMs != nil {
		return *point.DwellMs
	}
	return globalMs
}

// waitWhilePaused blocks without consuming stabilization time while traversal is paused.
// It returns the paused duration so callers can extend their active-time deadline.
func (m *TraversalManager) waitWhilePaused(taskID string) (time.Duration, bool) {
	start := time.Time{}
	for {
		if m.isTaskCancelled(taskID) {
			if start.IsZero() {
				return 0, true
			}
			return time.Since(start), true
		}
		m.mu.RLock()
		paused := m.isPaused && m.status.State == traversal.StatePaused
		m.mu.RUnlock()
		if !paused {
			if start.IsZero() {
				return 0, false
			}
			return time.Since(start), false
		}
		if start.IsZero() {
			start = time.Now()
		}
		time.Sleep(pausedLoopIdle)
	}
}

// sleepStabilizationInterval waits for active stabilization time and suspends
// the interval while traversal is paused.
func (m *TraversalManager) sleepStabilizationInterval(taskID string, d time.Duration) (time.Duration, bool) {
	deadline := time.Now().Add(d)
	var pausedTotal time.Duration
	for time.Now().Before(deadline) {
		if m.isTaskCancelled(taskID) {
			return pausedTotal, true
		}
		m.mu.RLock()
		paused := m.isPaused && m.status.State == traversal.StatePaused
		m.mu.RUnlock()
		if paused {
			pausedFor, cancelled := m.waitWhilePaused(taskID)
			pausedTotal += pausedFor
			if cancelled {
				return pausedTotal, true
			}
			deadline = deadline.Add(pausedFor)
			continue
		}
		remaining := time.Until(deadline)
		if remaining > cancelCheckPoll {
			remaining = cancelCheckPoll
		}
		time.Sleep(remaining)
	}
	return pausedTotal, false
}

func (m *TraversalManager) waitForStabilization(taskID string, point traversal.Point, pointIndex int, channelGroups []deviceChannelGroup) *traversal.MotionSafetyFailure {
	m.mu.RLock()
	stab := m.stabilization
	// per-point DwellMs fallback 与 RunCurrentPoint 入口共用 resolveEffectiveDwellMs，
	// 保证 fixed 模式下等待时长语义在两处调用点一致
	dwellMs := resolveEffectiveDwellMs(point, m.config.DwellTimeMs)
	motionAxes := m.config.MotionAxes
	safetyCfg := m.config.MotionSafety
	m.mu.RUnlock()

	// 稳定阶段独立看门狗——Moving 阶段的看门狗已随 waitForMotionComplete 退出而销毁
	watchdog := newMotionWatchdog()
	statusMissCounter := make(map[string]int)

	if stab == nil || stab.Mode == "fixed" {
		// 固定等待模式：将原 sleepWithTaskCheck 替换为周期性复检循环
		waitMs := dwellMs
		if stab != nil && stab.FixedTimeMs > 0 {
			waitMs = stab.FixedTimeMs
		}
		if waitMs <= 0 {
			waitMs = 1000
		}
		deadline := time.Now().Add(time.Duration(waitMs) * time.Millisecond)
		ticker := time.NewTicker(motionCompletePoll)
		defer ticker.Stop()
		for time.Now().Before(deadline) {
			if pausedFor, cancelled := m.waitWhilePaused(taskID); cancelled {
				return nil
			} else {
				deadline = deadline.Add(pausedFor)
			}
			if m.isTaskCancelled(taskID) {
				return nil
			}
			// 安全复检：任一故障立即返回
			if f := m.recheckMotionSafety(motionAxes, safetyCfg, point, pointIndex, watchdog, statusMissCounter); f != nil {
				return f
			}
			// 等待下一个复检周期
			<-ticker.C
		}
		return nil
	}

	// 自适应稳定模式
	if stab.Adaptive == nil {
		// 退化为固定等待
		deadline := time.Now().Add(time.Duration(dwellMs) * time.Millisecond)
		ticker := time.NewTicker(motionCompletePoll)
		defer ticker.Stop()
		for time.Now().Before(deadline) {
			if pausedFor, cancelled := m.waitWhilePaused(taskID); cancelled {
				return nil
			} else {
				deadline = deadline.Add(pausedFor)
			}
			if m.isTaskCancelled(taskID) {
				return nil
			}
			if f := m.recheckMotionSafety(motionAxes, safetyCfg, point, pointIndex, watchdog, statusMissCounter); f != nil {
				return f
			}
			<-ticker.C
		}
		return nil
	}

	adaptive := stab.Adaptive
	// per-point 优先：point.DwellMs 非 nil 时覆盖 adaptive.MaxWaitMs，保持 per-point 语义一致
	// fixed 模式下 DwellMs 直接作为等待时长生效；adaptive 模式此前静默忽略 DwellMs，
	// 用户设置后无效果无告警。此处统一行为：DwellMs 在 adaptive 模式下作为新的 MaxWaitMs 上限。
	// 同时同步压缩 MinWaitMs，避免 minWait > maxWait 导致最小等待阶段反而长于最大允许等待。
	maxWaitMs := adaptive.MaxWaitMs
	if point.DwellMs != nil && *point.DwellMs > 0 {
		maxWaitMs = *point.DwellMs
	}
	minWaitMs := adaptive.MinWaitMs
	if minWaitMs > maxWaitMs {
		minWaitMs = maxWaitMs
	}
	minWait := time.Duration(minWaitMs) * time.Millisecond
	maxWait := time.Duration(maxWaitMs) * time.Millisecond
	checkInterval := time.Duration(adaptive.CheckIntervalMs) * time.Millisecond
	threshold := adaptive.StabilityThreshold

	// 至少等待最小时间——期间同样进行安全复检
	minDeadline := time.Now().Add(minWait)
	minTicker := time.NewTicker(motionCompletePoll)
	defer minTicker.Stop()
	for time.Now().Before(minDeadline) {
		if pausedFor, cancelled := m.waitWhilePaused(taskID); cancelled {
			return nil
		} else {
			minDeadline = minDeadline.Add(pausedFor)
		}
		if m.isTaskCancelled(taskID) {
			return nil
		}
		if f := m.recheckMotionSafety(motionAxes, safetyCfg, point, pointIndex, watchdog, statusMissCounter); f != nil {
			return f
		}
		<-minTicker.C
	}

	// 读取初始参考值
	prevValues := m.readCurrentValues(channelGroups)
	start := time.Now()
	stableCount := 0

	for time.Since(start) < maxWait && stableCount < adaptive.ConsecutiveChecks {
		if pausedFor, cancelled := m.waitWhilePaused(taskID); cancelled {
			return nil
		} else {
			start = start.Add(pausedFor)
		}
		if m.isTaskCancelled(taskID) {
			return nil
		}
		// 自适应采样前先做安全复检——任何故障立即中断
		if f := m.recheckMotionSafety(motionAxes, safetyCfg, point, pointIndex, watchdog, statusMissCounter); f != nil {
			return f
		}
		if pausedFor, cancelled := m.sleepStabilizationInterval(taskID, checkInterval); cancelled {
			return nil
		} else {
			start = start.Add(pausedFor)
		}

		// 读取当前值并判断稳定性
		curValues := m.readCurrentValues(channelGroups)
		if m.isStable(prevValues, curValues, threshold) {
			stableCount++
		} else {
			stableCount = 0 // 不稳定则重置计数
		}
		prevValues = curValues
	}
	return nil
}

// recheckMotionSafety 稳定阶段运动安全复检。
//
// 与 waitForMotionComplete 中的判定逻辑一致，但使用独立的看门狗与计数器，
// 因为稳定阶段是新的判定周期（Moving 阶段的状态已销毁）。
//
// 检查项：
//  1. validateMotionStatuses——控制器掉线/急停/目标轴连续缺失
//  2. 每轴 EvaluateMotionSafety——撞限位/到位/超差/严重偏离
//  3. 跨样本看门狗 Observe——无进展/越过目标（防御性，轴应已停）
//
// 任一故障立即返回 *MotionSafetyFailure；nil 表示状态正常。
func (m *TraversalManager) recheckMotionSafety(
	motionAxes []traversal.MotionAxisBinding,
	safetyCfg *traversal.MotionSafetyConfig,
	point traversal.Point,
	pointIndex int,
	watchdog *motionWatchdog,
	statusMissCounter map[string]int,
) *traversal.MotionSafetyFailure {
	if m.motion == nil {
		return nil
	}
	ctx := context.Background()
	statuses := m.motion.StatusAll(ctx)

	// 1. 状态可用性校验
	if f := m.validateMotionStatuses(statuses, motionAxes, statusMissCounter, pointIndex); f != nil {
		return f
	}

	// 2. 每轴 EvaluateMotionSafety + 看门狗 Observe
	for _, status := range statuses {
		if !status.Connected {
			continue
		}
		targets := availableAxisTargets(status, point, motionAxes)
		for _, axis := range status.Axes {
			target, hasTarget := targets[axis.Name]
			if !hasTarget {
				continue
			}
			resolved := safetyCfg.Resolve(string(axis.Name))
			verdict := EvaluateMotionSafety(axis, target, resolved)
			if verdict.IsFailure() {
				return &traversal.MotionSafetyFailure{
					ControllerID:   status.ID,
					ControllerName: status.Name,
					Axis:           string(axis.Name),
					Verdict:        verdict,
					Target:         target,
					Actual:         axis.Position,
					PointIndex:     pointIndex,
				}
			}
			if f := watchdog.Observe(status.ID, axis, target, resolved, pointIndex); f != nil {
				f.ControllerName = status.Name
				return f
			}
		}
	}
	return nil
}

// readCurrentValues 读取各设备最新数据并按内部通道键合并。
// 供稳定等待做前后值对比，无新鲜度要求（同一帧多次读取不影响稳定性判定）。
// 任一设备无数据即返回 nil——isStable 对 nil 判不稳定，与旧单设备行为等价。
func (m *TraversalManager) readCurrentValues(groups []deviceChannelGroup) map[int]float64 {
	if m.reader == nil {
		return nil
	}
	merged := make(map[int]float64)
	for _, g := range groups {
		payload, ok := m.reader.GetLatestData(g.deviceID)
		if !ok {
			return nil
		}
		values := valuesForChannels(payload, g.hwIndices)
		for i, key := range g.keys {
			if v, ok := values[g.hwIndices[i]]; ok {
				merged[key] = v
			}
		}
	}
	if len(merged) == 0 {
		return nil
	}
	return merged
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

// commitAndAdvance 提交单点结果并推进遍历进度。
//
// 设计动机（code-review P2 修复）：normal/skip/notTested 三分支原本各自实现
// commitPointV2 → 旧 sink 写 → snapshot/status 推进 → allDone 处理流程，
// 约 93 行重复代码。抽取后未来新增"提交后副作用"（如事件推送、CSV header hash
// 更新）只需改一处，避免遗漏某分支造成数据不一致。
//
// 流程（与原 normal 分支保持完全一致）：
//  1. commitPointV2 三阶段提交（CSV Append + ResultLog AppendPrepared + Checkpoint 原子替换）
//  2. 向后兼容旧 sink 写入（不参与 v2 回滚；失败提升为 status.Warning 让前端可见）
//  3. 推进 snapshot.CommitSeq/CommittedPoints + status.Results/CurrentPoint
//  4. allDone 时 completeAfterReturnToOrigin + store.Save + ClearCheckpoint
//  5. v1 兼容：未注入 v2 checkpoint 端口时沿用旧 saveCheckpoint（每 10 个点）
//
// 参数：
//   - taskID: 任务 ID
//   - result: 待提交的 PointResult（调用方负责设置 PointStatus/Values/SampleCount 等；
//     CommitSeq 由本函数内部从 session.snapshot 推进分配；commitPointV2 会写回 CSVRowHash）
//   - clearValidationWarnings: 是否清空 status.ValidationWarnings
//     （normal/skip 分支清空；notTested 未采集无 warnings 不需要）
//
// 返回：失败时返回 error（已通过 failWithCode 设置状态）
func (m *TraversalManager) commitAndAdvance(taskID string, result *traversal.PointResult, clearValidationWarnings bool) error {
	pointIndex := result.PointIndex

	if err := m.commitPointV2(taskID, result); err != nil {
		return m.failWithCode("commit point %d failed: %v", traversal.ErrSaveFailed, pointIndex+1, err)
	}

	// 推进 snapshot.CommitSeq（线性化点 = Checkpoint 持久化成功）
	// CommittedPoints 同步更新，避免 session.snapshot 字段陈旧
	m.mu.Lock()
	session := m.session
	session.snapshot.CommitSeq = result.CommitSeq
	session.snapshot.CommittedPoints = int(result.CommitSeq)
	m.status.Results = append(m.status.Results, *result)
	m.status.CurrentPoint++
	m.status.CommittedPoints = int(result.CommitSeq)
	if clearValidationWarnings {
		m.status.ValidationWarnings = nil
	}
	allDone := m.status.CurrentPoint >= len(m.config.Path)
	completedCount := m.status.CurrentPoint
	// store.Save 所需快照在置 Completed 之后的锁内捕获（见下），
	// 在置 Completed 之前捕获会把过期状态快照持久化到 result store。
	var pendingSave bool
	var saveTaskID string
	var saveStatus traversal.Status
	// 用于断点保存的快照（在锁内复制，避免锁外访问竞态）
	checkpointPath := traversal.ResolveOutputPath(m.config)
	if pathSink, ok := m.sink.(interface{ OutputPath() string }); ok {
		if outputPath := pathSink.OutputPath(); outputPath != "" {
			checkpointPath = outputPath
		}
	}
	checkpointPoints := append([]traversal.Point(nil), m.config.Path...)
	m.mu.Unlock()

	// 向后兼容：旧 sink 路径仍执行，但不参与事务回滚
	// 若 sink 与 csvPort 为同一实例（v2 装配下常见），CSV 已通过 csvPort.Append 写入，
	// 这里再调用 sink.WriteTraversalPoint 会重复落盘同一行，破坏 CSV 行号 ↔ commitSeq 一致性。
	// sinkIsCsvPort 通过类型断言 + 指针比较检测同实例，跳过重复写入。
	//
	// P2 修复（code-review Important-3）：旧 sink 失败仅日志记录会让用户看不到失败信号，
	// 改为同步写入 status.Warning，前端轮询可见提示。失败不触发 v2 回滚的语义保持不变
	// （双写冲突会导致数据丢失，权衡是接受旧 sink 数据缺失但保 v2 一致性）。
	if m.sink != nil && !sinkIsCsvPort(m.sink, m.csvPort) {
		if err := m.sink.WriteTraversalPoint(*result); err != nil {
			slog.Error("traversal old sink save failed",
				"component", "traversal",
				"task_id", taskID,
				"point_index", pointIndex+1,
				"error", err,
			)
			// 失败写入 status.Warning：与"回零失败"同处理路径，前端可见
			m.mu.Lock()
			warnMsg := fmt.Sprintf("point %d: sink write failed: %v", pointIndex+1, err)
			if m.status.Warning != "" {
				m.status.Warning = m.status.Warning + "; " + warnMsg
			} else {
				m.status.Warning = warnMsg
			}
			m.mu.Unlock()
		}
	}

	if allDone {
		if err := m.completeAfterReturnToOrigin(taskID, m.returnToOrigin(taskID, len(m.config.Path))); err != nil {
			return err
		}
		m.mu.Lock()
		m.status.State = traversal.StateCompleted
		m.status.CurrentPointPhase = ""
		if m.store != nil {
			pendingSave = true
			saveTaskID = m.config.TaskID
			saveStatus = m.status
			saveStatus.Results = append([]traversal.PointResult(nil), m.status.Results...)
		}
		m.mu.Unlock()
		if pendingSave {
			if err := m.store.Save(saveTaskID, saveStatus); err != nil {
				slog.Error("traversal final save failed",
					"component", "traversal",
					"task_id", saveTaskID,
					"error", err,
				)
				return fmt.Errorf("save traversal result: %w", err)
			}
			slog.Info("traversal result saved on completion",
				"component", "traversal",
				"task_id", saveTaskID,
				"completed_points", completedCount,
			)
		}
		m.ClearCheckpoint()
	}

	// v2 回退：未注入 v2 checkpoint 端口时，沿用旧 saveCheckpoint 逻辑（每 10 个点）
	// 与原 normal 分支保持一致：v1 装配下需要断点保护，v2 已通过 commitPointV2 持久化
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

	return nil
}

// deviceChannelGroup 单台设备上需要采样的通道集合。
// keys 为内部通道键（与 Config.Channels / PointResult.Values / ChannelLabels 一致），
// hwIndices 为该设备 profile 内的硬件通道索引，二者按下标一一对应。
type deviceChannelGroup struct {
	deviceID  string
	keys      []int
	hwIndices []int
}

// groupChannelsByDevice 把内部通道键按物理设备分组。
// 设备顺序按通道首次出现顺序确定，保证采样与日志输出确定性。
// refs 必须覆盖 channels 的每个键（Config.ResolvedChannelRefs 语义保证）；
// 缺失的键会被跳过并记日志，最终由采样长度校验暴露为通道不匹配。
func groupChannelsByDevice(channels []int, refs map[int]traversal.ChannelRef) []deviceChannelGroup {
	groups := make([]deviceChannelGroup, 0, 2)
	groupIndex := make(map[string]int)
	for _, key := range channels {
		ref, ok := refs[key]
		if !ok {
			slog.Error("traversal channel missing physical ref",
				"component", "traversal",
				"channel_key", key,
			)
			continue
		}
		gi, ok := groupIndex[ref.DeviceID]
		if !ok {
			gi = len(groups)
			groupIndex[ref.DeviceID] = gi
			groups = append(groups, deviceChannelGroup{deviceID: ref.DeviceID})
		}
		groups[gi].keys = append(groups[gi].keys, key)
		groups[gi].hwIndices = append(groups[gi].hwIndices, ref.Index)
	}
	return groups
}

// acquisitionRecovery 单台设备等待结束时的恢复信息。
// wasReconnect 表示本次等待期间该设备出现过 ReconnectRequired（最严重状态），
// 只有它需要重建时间戳基线——重连后设备时间戳可能归零/回绕，旧 lastTimestamps
// 会让新帧永远"不够新"而误判"在采但无新帧"（10s 停滞）。
// staleTimestamp/staleKnown 记录首次观察到 ReconnectRequired 时 hub 缓存帧的
// 时间戳：设备断开时 hub 不清 latest，恢复后状态已 Acquiring、但新连接首帧未到
// 的窗口内 GetLatestData 仍可能返回这帧旧数据，必须忽略该精确旧帧，否则其
// 时间戳会重新锁死时间戳归零/回绕的新帧。
type acquisitionRecovery struct {
	id             string
	wasReconnect   bool
	staleTimestamp int64
	staleKnown     bool
}

// waitForAcquisitionResume 等待全部引用设备恢复采集（spec-traversal-acquisition-stop §等待恢复阶段）。
//
// 语义：
//   - **无限期等待，无时间预算**。退出路径：全部设备恢复 Acquiring / Stop（返回错误）/ Pause（语义等同暂停）。
//   - 每个 tick 通过 classify 重新分类（返回当前异常设备列表）。
//   - 期间维护 Status.WaitingForAcquisition / WaitingDevices / WaitingForAcquisitionSinceMs /
//     CurrentPointPhase=waiting_acquisition：进入等待置位，SinceMs 记录会话起点；
//     Pause 保留字段（横幅被暂停 UI 取代）；Resume 后仍异常 → 新会话（SinceMs 重新计时）。
//   - **不持有 stallDeadline**（采样局部变量，helper 访问不到），由采样调用方返回后重置；
//     不管理顶层 State（调用方负责，点位开始进入前须显式置 StateMoving）。
//
// 返回：本次等待期间观察到异常的每台设备的恢复信息（空 = 无需等待即全部 Acquiring）。
// 调用方据此按设备重建 freshness 基线（Stopped 清 fresh/pending；wasReconnect
// 重置 lastTimestamps=-1、忽略 staleTimestamp 旧帧并丢弃恢复后首帧）。
//
// 注意：每台设备记录的是**本次等待期间观察到的最严重状态**（ReconnectRequired >
// Stopped），而不是最后一次观察——设备经历 reconnect_required → stopped → acquiring
// 的典型恢复路径时，最后一次分类可能是 stopped，但"曾重连"语义（时间戳基线重建）
// 必须保留，否则重连后归零的时间戳会继续用旧基线比较。
func (m *TraversalManager) waitForAcquisitionResume(taskID string, classify func() []acquisitionDeviceState) ([]acquisitionRecovery, error) {
	m.mu.RLock()
	savedPhase := m.status.CurrentPointPhase
	m.mu.RUnlock()
	worst := make(map[string]ports.AcquisitionState)
	staleTs := make(map[string]int64)
	// 进入等待前设备已异常（如点位开始）：同属一次等待会话，纳入最严重状态统计。
	for _, s := range classify() {
		m.trackWorstAcquisitionState(worst, staleTs, s)
	}
	if len(worst) == 0 {
		// 全部设备已在采集，无需等待。
		m.clearWaitingForAcquisition()
		return nil, nil
	}
	var sessionStart time.Time
	wasPaused := false
	for {
		if m.isTaskCancelled(taskID) {
			m.clearWaitingForAcquisition()
			return nil, fmt.Errorf("acquisition cancelled")
		}
		m.mu.RLock()
		paused := m.isPaused && m.status.State == traversal.StatePaused
		m.mu.RUnlock()
		if paused {
			wasPaused = true
			time.Sleep(pausedLoopIdle)
			continue
		}
		abnormal := classify()
		if len(abnormal) == 0 {
			m.clearWaitingForAcquisition()
			m.restoreCurrentPointPhase(taskID, savedPhase)
			return buildAcquisitionRecovery(worst, staleTs), nil
		}
		for _, s := range abnormal {
			m.trackWorstAcquisitionState(worst, staleTs, s)
		}
		// 首次进入等待，或从暂停恢复后仍异常：开启新等待会话。
		if sessionStart.IsZero() || wasPaused {
			sessionStart = time.Now()
			wasPaused = false
		}
		m.setWaitingForAcquisition(taskID, abnormal, sessionStart)
		time.Sleep(acquisitionBatchPoll)
	}
}

// trackWorstAcquisitionState 累计设备在本次等待期间观察到的最严重采集态
// （ReconnectRequired > Stopped），并在首次观察到 ReconnectRequired 时记录 hub
// 缓存帧时间戳——设备断开时 hub 不清 latest，恢复后 GetLatestData 仍可能返回
// 这帧旧数据，靠该时间戳识别并忽略。
func (m *TraversalManager) trackWorstAcquisitionState(worst map[string]ports.AcquisitionState, staleTs map[string]int64, s acquisitionDeviceState) {
	prev, seen := worst[s.id]
	if s.state == ports.AcquisitionReconnectRequired {
		if !seen || prev != ports.AcquisitionReconnectRequired {
			if m.reader != nil {
				if ts, ok := m.reader.GetLatestTimestamp(s.id); ok {
					staleTs[s.id] = ts
				}
			}
		}
		worst[s.id] = ports.AcquisitionReconnectRequired
		return
	}
	if !seen {
		worst[s.id] = s.state
	}
	// 已记录 ReconnectRequired 的严重状态不因后续 Stopped 观察而降级。
}

// buildAcquisitionRecovery 把最严重状态统计转换为稳定的恢复信息列表（按 deviceID 字典序）。
func buildAcquisitionRecovery(worst map[string]ports.AcquisitionState, staleTs map[string]int64) []acquisitionRecovery {
	ids := make([]string, 0, len(worst))
	for id := range worst {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	out := make([]acquisitionRecovery, 0, len(ids))
	for _, id := range ids {
		r := acquisitionRecovery{id: id, wasReconnect: worst[id] == ports.AcquisitionReconnectRequired}
		if ts, ok := staleTs[id]; ok {
			r.staleTimestamp = ts
			r.staleKnown = true
		}
		out = append(out, r)
	}
	return out
}

// setWaitingForAcquisition 写入等待状态字段并置 waiting_acquisition 阶段。
// 设备级 SinceMs 不维护：单台设备的"进入该状态时间"跨 tick 需要额外状态表，而
// UI 只需总等待时长（WaitingForAcquisitionSinceMs）——设备级时间戳语义不满足
// 且无消费者（见 code-review Important-5），故不在 AcquisitionDeviceStatus 暴露。
func (m *TraversalManager) setWaitingForAcquisition(taskID string, abnormal []acquisitionDeviceState, since time.Time) {
	devices := make([]traversal.AcquisitionDeviceStatus, 0, len(abnormal))
	for _, s := range abnormal {
		devices = append(devices, traversal.AcquisitionDeviceStatus{
			Name:  s.name,
			State: acquisitionStateString(s.state),
		})
	}
	m.mu.Lock()
	if m.status.TaskID != taskID {
		m.mu.Unlock()
		return
	}
	m.status.WaitingForAcquisition = true
	m.status.WaitingDevices = devices
	m.status.WaitingForAcquisitionSinceMs = since.UnixMilli()
	m.status.CurrentPointPhase = traversal.PhaseWaitingForAcquisition
	m.mu.Unlock()
}

// clearWaitingForAcquisition 清空等待状态字段（不动 CurrentPointPhase，由调用方/restore 负责）。
func (m *TraversalManager) clearWaitingForAcquisition() {
	m.mu.Lock()
	m.status.WaitingForAcquisition = false
	m.status.WaitingDevices = nil
	m.status.WaitingForAcquisitionSinceMs = 0
	m.mu.Unlock()
}

// restoreCurrentPointPhase 还原等待前的 CurrentPointPhase（taskID 守卫）。
func (m *TraversalManager) restoreCurrentPointPhase(taskID string, phase traversal.PointPhase) {
	m.mu.Lock()
	if m.status.TaskID != taskID {
		m.mu.Unlock()
		return
	}
	m.status.CurrentPointPhase = phase
	m.mu.Unlock()
}

// acquisitionStateString 三态 → API 字符串（仅异常设备出现在 WaitingDevices）。
func acquisitionStateString(s ports.AcquisitionState) string {
	switch s {
	case ports.AcquisitionStopped:
		return "stopped"
	case ports.AcquisitionReconnectRequired:
		return "reconnect_required"
	default:
		return "acquiring"
	}
}

// groupDeviceIDs 提取分组中的去重设备 ID（顺序无关，classify 内部已去重语义）。
func groupDeviceIDs(groups []deviceChannelGroup) []string {
	ids := make([]string, 0, len(groups))
	seen := make(map[string]bool, len(groups))
	for _, g := range groups {
		if !seen[g.deviceID] {
			seen[g.deviceID] = true
			ids = append(ids, g.deviceID)
		}
	}
	return ids
}

// collectAveragedSamples 多设备分组采样取平均（带总体超时保护、暂停/停止响应）。
//
// 有效样本语义（多设备）：
// 所有参与设备都产出至少一帧「新数据」（时间戳晚于本样本起始边界）才计 1 个
// 有效样本——保证同一样本内各设备的值都是最新的，凑满 samplesPerPoint 个才返回。
// 设备刷新周期不同（如 20Hz 与 5Hz 混采）时采样节奏以慢设备为准。
//
// 时间戳去重：设备刷新周期 > 轮询间隔（10ms）时，多次 GetLatestData 返回同一帧，
// 直接累加会导致假平均（同一帧的值反复相加），故每台设备独立记录已消费时间戳。
func (m *TraversalManager) collectAveragedSamples(taskID string, groups []deviceChannelGroup, samplesPerPoint int) (map[int]float64, error) {
	totals := make(map[int]float64)
	validSamples := 0
	// 停滞超时：每凑满一个有效样本即重置，只惩罚"长时间凑不齐新样本"
	// （设备停采/断链/通道配置错误），不惩罚正常低频设备的多样本采集
	// （帧去重要求每个样本都是新帧，2Hz 设备采 10 个样本需约 5s，固定总超时会必然失败）。
	// 注意按"完成样本"而非"任意新帧"重置：多设备混采时若某台设备死透，
	// 其余设备的新帧不会掩盖停滞，凑不齐样本仍会超时。
	stallDeadline := time.Now().Add(acquisitionStallTimeout)

	// 自诊断：无有效样本时区分"设备未在采集"与"通道索引对不上"两类根因。
	// everOk=false → 所有设备 GetLatestData 始终 ok=false（设备未采集或 deviceID 不匹配）；
	// everOk=true 但 validSamples==0 → 设备有数据，但 payload 通道集合不包含请求的通道。
	var everOk bool
	var lastIndices []int
	var lastChannelCount int
	// 采样失败计数：区分"设备无数据"与"通道不匹配"两类失败
	var noDataCount int
	var channelMismatchCount int

	// 每台设备独立的时间戳去重与"本样本已就绪"标记。
	// 起始时间戳取采样开始前的最新帧，确保只累计采样开始后新产出的帧（fresh 语义）。
	lastTimestamps := make(map[string]int64, len(groups))
	fresh := make(map[string]bool, len(groups))
	pending := make(map[string]map[int]float64, len(groups)) // deviceID → hwIndex → value
	// rebaseDropFirst 重连恢复后需丢弃首帧的设备（仅 wasReconnect 时置位）：
	// 首帧只作为时间戳基线消费、不计入样本，防 hub 残留旧连接缓存帧当样本。
	rebaseDropFirst := make(map[string]bool, len(groups))
	// rebaseStaleTimestamp 重连恢复后需要忽略的旧连接缓存帧时间戳（按设备）：
	// 设备断开时 hub 不清 latest，恢复后状态已 Acquiring 但新连接首帧未到的窗口内，
	// GetLatestData 仍返回这帧旧数据；进入 ReconnectRequired 时已捕获其时间戳，
	// 该精确时间戳的帧不作为新基线，只允许真正的新帧建立基线。
	rebaseStaleTimestamp := make(map[string]int64, len(groups))
	for _, g := range groups {
		lastTs := int64(-1)
		if ts, ok := m.reader.GetLatestTimestamp(g.deviceID); ok {
			lastTs = ts
		}
		lastTimestamps[g.deviceID] = lastTs
	}

	// 采集态控制器在测试运行期间不会变更（仅 SetAcquisitionController 装配阶段会写），
	// 循环外读一次即可，避免每个 polling tick 重复拿 m.mu.RLock 放大锁竞争。
	m.mu.RLock()
	acqController := m.acquisitionController
	m.mu.RUnlock()

	for validSamples < samplesPerPoint {
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
		// 遍历暂停是用户主动控制，允许无限期等待；暂停期间旁路一切采集态判定。
		m.mu.RLock()
		paused := m.isPaused && m.status.State == traversal.StatePaused
		m.mu.RUnlock()
		if paused {
			stallDeadline = time.Now().Add(acquisitionStallTimeout)
			// 丢弃暂停前正在攒的当前样本（fresh/pending 中已就绪的旧帧）。
			// 恢复后必须从新帧重新起算，否则第一个样本会把暂停前旧值并入均值。
			fresh = make(map[string]bool, len(groups))
			pending = make(map[string]map[int]float64, len(groups))
			time.Sleep(pausedLoopIdle)
			continue
		}
		// 设备采集态检查：任一设备非 Acquiring 时进入"类暂停"无限期等待恢复
		// （spec-traversal-acquisition-stop）。不自动判失败，退出路径 =
		// 设备全部恢复 / Stop / Pause。
		if acqController != nil {
			classify := func() []acquisitionDeviceState {
				return abnormalAcquisitionDevicesForIDs(acqController, groupDeviceIDs(groups))
			}
			recovered, err := m.waitForAcquisitionResume(taskID, classify)
			if err != nil {
				return nil, err
			}
			if len(recovered) > 0 {
				// 设备恢复采集：重建 freshness 基线。
				// 等待期间采样循环被阻塞未读数据，所有设备的 pending 都可能已过期——
				// 统一清 fresh/pending（与暂停分支一致），避免把等待前旧帧并入恢复后
				// 首个样本（不止异常设备，正常设备等待前已就绪的 pending 同样过期）。
				fresh = make(map[string]bool, len(groups))
				pending = make(map[string]map[int]float64, len(groups))
				// wasReconnect 设备额外重置时间戳基线：重连后时间戳可能归零/回绕，
				// 旧 lastTimestamps 会让新帧永不够新而被误判"在采但无新帧"（10s 停滞）。
				// 同时记录进入 ReconnectRequired 时捕获的 staleTimestamp：恢复后忽略
				// 该精确旧帧——重连状态已恢复但新连接首帧未到前，GetLatestData 仍
				// 可能返回 hub 中残留的旧连接缓存帧，其时间戳会再次锁死归零/回绕的新帧。
				for _, r := range recovered {
					if !r.wasReconnect {
						continue
					}
					lastTimestamps[r.id] = -1
					rebaseDropFirst[r.id] = true
					if r.staleKnown {
						rebaseStaleTimestamp[r.id] = r.staleTimestamp
					}
				}
				// 等待期间不推进停滞计时：返回后重置，避免立刻触发旧的 10s 超时。
				stallDeadline = time.Now().Add(acquisitionStallTimeout)
			}
		}
		if time.Now().After(stallDeadline) {
			slog.Warn("traversal averaged sampling stalled",
				"component", "traversal",
				"task_id", taskID,
				"valid_samples", validSamples,
				"target_samples", samplesPerPoint,
				"no_data_count", noDataCount,
				"channel_mismatch_count", channelMismatchCount,
				"device_ever_answered", everOk,
				"stall_timeout", acquisitionStallTimeout.String(),
			)
			break // 停滞超时保护：不再继续采样
		}

		for _, g := range groups {
			if fresh[g.deviceID] {
				continue // 本样本内该设备已就绪，等其余设备
			}
			payload, ok := m.reader.GetLatestData(g.deviceID)
			if !ok {
				noDataCount++
				continue
			}
			everOk = true
			if payload.Timestamp <= lastTimestamps[g.deviceID] {
				noDataCount++
				continue // 同一帧重复读取，跳过
			}
			// 重连恢复后丢弃首帧：仅作为时间戳基线消费、不计入样本，
			// 防止把 hub 中残留的旧连接缓存帧当样本。
			if rebaseDropFirst[g.deviceID] {
				// 忽略进入 ReconnectRequired 时捕获的旧连接缓存帧（时间戳一致）——
				// 设备断开时 hub 不清 latest，恢复后该帧可能被 GetLatestData 继续返回；
				// 只有时间戳不同的帧才是新连接的真正首帧，作为新基线。
				if stale, ok := rebaseStaleTimestamp[g.deviceID]; ok && payload.Timestamp == stale {
					noDataCount++
					continue
				}
				lastTimestamps[g.deviceID] = payload.Timestamp
				rebaseDropFirst[g.deviceID] = false
				delete(rebaseStaleTimestamp, g.deviceID)
				continue
			}
			lastTimestamps[g.deviceID] = payload.Timestamp
			lastChannelCount = len(payload.Channels)
			lastIndices = append(lastIndices[:0], payload.ChannelIndices...)
			values := valuesForChannels(payload, g.hwIndices)
			if len(values) != len(g.hwIndices) {
				// 新帧但不含全部请求通道：不计入本样本，等下一帧；
				// 若配置错误（通道在别的设备上）会持续不匹配直至超时，由日志暴露。
				channelMismatchCount++
				continue
			}
			pending[g.deviceID] = values
			fresh[g.deviceID] = true
		}

		// 所有设备都出了新帧 → 合并为 1 个有效样本
		allFresh := len(groups) > 0
		for _, g := range groups {
			if !fresh[g.deviceID] {
				allFresh = false
				break
			}
		}
		if allFresh {
			for _, g := range groups {
				vals := pending[g.deviceID]
				for i, key := range g.keys {
					totals[key] += vals[g.hwIndices[i]]
				}
				fresh[g.deviceID] = false
			}
			validSamples++
			// 完成一个样本即重置停滞超时：样本节奏由最慢设备决定，帧间隔不计入停滞
			stallDeadline = time.Now().Add(acquisitionStallTimeout)
		}
		time.Sleep(acquisitionBatchPoll)
	}

	if validSamples != samplesPerPoint {
		slog.Error("traversal no valid samples",
			"component", "traversal",
			"task_id", taskID,
			"device_groups", groups,
			"device_ever_answered", everOk,
			"last_payload_channel_count", lastChannelCount,
			"last_payload_channel_indices", lastIndices,
		)
		return nil, fmt.Errorf("collected %d/%d fresh samples (no new complete sample for %s)", validSamples, samplesPerPoint, acquisitionStallTimeout)
	}

	averaged := make(map[int]float64, len(totals))
	for k, total := range totals {
		averaged[k] = total / float64(validSamples)
	}
	return averaged, nil
}

// waitForMotionComplete 等待运动完成，集成运动安全判定与跨样本看门狗。
//
// 返回值：
//   - completed=true, reason=none, failure=nil：所有参与运动的轴已到位
//   - completed=false, failure!=nil：检测到运动安全故障（调用方应调用 handleMotionSafetyFailure）
//   - completed=false, failure=nil, reason≠none：因暂停/停止/取消/超时中断
//     （调用方按 reason 分支处理，不再读 m.isPaused/m.isStopped 推断，避免竞态）
//
// 故障判定优先级（spec §waitForMotionComplete）：
//  1. ctx 取消（虽然当前传 Background 不会触发，保留语义以备未来改造）
//  2. 到位检查（motionTargetsReachedWithTolerance）——deadline 边界附近先判到位避免假超时
//  3. 暂停/停止标志——用户主动中断，不算故障
//  4. validateMotionStatuses——已绑定控制器掉线/已急停/目标轴连续 3 快照缺失
//  5. 每轴 EvaluateMotionSafety 单次快照判定（撞限位/到位/超差/严重偏离）
//  6. 跨样本看门狗 Observe（无进展/越过目标）
//  7. 120s 停止未到位兜底超时——仍在正常运动时继续等待，由无进展看门狗识别卡死
//
// 故障现场快照原则：检测到故障时立即构造 MotionSafetyFailure，错误处理阶段不再读硬件。
func (m *TraversalManager) waitForMotionComplete(ctx context.Context, point traversal.Point, taskID string, pointIndex int) (completed bool, reason traversal.MotionInterruptReason, failure *traversal.MotionSafetyFailure) {
	ticker := time.NewTicker(motionCompletePoll)
	defer ticker.Stop()
	deadline := time.Now().Add(motionCompleteTimeout)

	// 读取 motionAxes 与运动安全配置；motionAxes 为空时（旧配置兼容）对所有已连接控制器判定
	m.mu.RLock()
	motionAxes := m.config.MotionAxes
	safetyCfg := m.config.MotionSafety
	m.mu.RUnlock()
	statusValidationAxes := motionAxes

	// 容错预处理：与 RunCurrentPoint 保持一致，否则 controllerId 全部不匹配时
	// targets 为 nil → allReached 恒为 true → 跳过运动直接进入「稳定中」阶段
	motionAxes = resolveMotionAxes(motionAxes, m.motion.StatusAll(ctx))

	// 跨样本看门狗：每点位独立，新点位开始时构造新实例
	watchdog := newMotionWatchdog()

	// 状态缺失抖动计数器：(controllerID|axis) → 连续缺失次数
	// 连续 3 快照缺失才升级为 StatusUnavailable，避免单帧抖动误报
	statusMissCounter := make(map[string]int)

	for {
		select {
		case <-ctx.Done():
			// ctx 取消优先于故障判定——外部 Stop 调用后立即返回，不再触发故障处理
			return false, traversal.MotionInterruptCancelled, nil
		case <-ticker.C:
			// 1. 优先检查到位：运动在 deadline 边界附近完成时，先判到位避免假超时
			statuses := m.motion.StatusAll(ctx)
			allReached, _ := motionTargetsReachedWithTolerance(statuses, point, motionAxes, safetyCfg)
			if allReached {
				return true, traversal.MotionInterruptNone, nil
			}

			// 2. 暂停/停止检查（非故障中断）——拆分判断以返回不可变原因
			// 原实现合并 stopped||isPaused 返回 (false, nil) 再事后读 m.isPaused，
			// Resume 在两者之间清零标志会导致暂停被误判为超时
			m.mu.Lock()
			isPaused := m.isPaused
			stopped := m.isStopped
			m.mu.Unlock()
			if stopped {
				return false, traversal.MotionInterruptStopped, nil
			}
			if isPaused {
				return false, traversal.MotionInterruptPaused, nil
			}

			// 3. 状态可用性校验：控制器掉线/已急停/目标轴连续缺失
			if f := m.validateMotionStatuses(statuses, statusValidationAxes, statusMissCounter, pointIndex); f != nil {
				return false, traversal.MotionInterruptNone, f
			}

			// 4. 每轴 EvaluateMotionSafety + 看门狗 Observe
			for _, status := range statuses {
				if !status.Connected {
					continue
				}
				targets := availableAxisTargets(status, point, motionAxes)
				for _, axis := range status.Axes {
					target, hasTarget := targets[axis.Name]
					if !hasTarget {
						continue
					}

					// 按轴解析有效配置（合并默认值 + 全局 + 按轴覆盖）
					resolved := safetyCfg.Resolve(string(axis.Name))

					// 单次快照判定：撞限位/到位/超差/严重偏离
					verdict := EvaluateMotionSafety(axis, target, resolved)
					if verdict.IsFailure() {
						return false, traversal.MotionInterruptNone, &traversal.MotionSafetyFailure{
							ControllerID:   status.ID,
							ControllerName: status.Name,
							Axis:           string(axis.Name),
							Verdict:        verdict,
							Target:         target,
							Actual:         axis.Position,
							PointIndex:     pointIndex,
						}
					}

					// 跨样本看门狗：仅运动中观察，识别无进展/越过目标
					// 轴已停时 Observe 内部会清空历史并返回 nil
					if f := watchdog.Observe(status.ID, axis, target, resolved, pointIndex); f != nil {
						f.ControllerName = status.Name
						return false, traversal.MotionInterruptNone, f
					}
				}
			}

			// 5. 固定时限只兜底处理已停止但未到位；长距离低速运动不得误报超时。
			if motionWaitDeadlineExceeded(deadline, statuses, point, motionAxes) {
				return false, traversal.MotionInterruptTimeout, nil
			}
		}
	}
}

func motionWaitDeadlineExceeded(deadline time.Time, statuses []motion.ControllerStatus, point traversal.Point, motionAxes []traversal.MotionAxisBinding) bool {
	if !time.Now().After(deadline) {
		return false
	}
	for _, status := range statuses {
		if !status.Connected {
			continue
		}
		targets := availableAxisTargets(status, point, motionAxes)
		for _, axis := range status.Axes {
			if _, targeted := targets[axis.Name]; targeted && axis.Moving {
				return false
			}
		}
	}
	return true
}

// validateMotionStatuses 校验运动状态可用性，检测三类异常：
//  1. 已绑定控制器掉线（Connected=false 或不在 statuses 中）
//  2. 已绑定控制器已急停（EmergencyStopped=true）
//  3. 目标轴连续 3 快照缺失（status.Axes 中找不到绑定轴名）
//
// 连续 3 快照缺失才升级为故障，避免单帧抖动（如设备临时未响应）误报。
// statusMissCounter 由调用方持有，跨快照累积计数；找到轴时清零对应计数。
//
// 返回 *MotionSafetyFailure 表示需立即停止；nil 表示状态正常或仅瞬时抖动。
//
// 注意：本函数不读硬件——statuses 由调用方传入，已是当前快照。
func (m *TraversalManager) validateMotionStatuses(
	statuses []motion.ControllerStatus,
	motionAxes []traversal.MotionAxisBinding,
	statusMissCounter map[string]int,
	pointIndex int,
) *traversal.MotionSafetyFailure {
	// 无绑定配置时跳过校验（旧行为兼容：对所有已连接控制器判定）
	if len(motionAxes) == 0 {
		return nil
	}

	// 构建当前快照的控制器索引（ID → ControllerStatus）便于按 ID 查找
	statusByController := make(map[string]motion.ControllerStatus, len(statuses))
	for _, s := range statuses {
		statusByController[s.ID] = s
	}

	// 遍历绑定，逐项检查控制器与轴可用性
	for _, binding := range motionAxes {
		if binding.Axis == "" {
			continue
		}

		// 已绑定 controllerId 时严格按 ID 查找；为空时按轴名匹配（已在 resolveMotionAxes 回退）
		// 这里仅检查 controllerId 非空的绑定，空 controllerId 的绑定由 availableAxisTargets 处理
		if binding.ControllerID == "" {
			continue
		}

		status, exists := statusByController[binding.ControllerID]
		missKey := binding.ControllerID + "|" + binding.Axis

		// 1. 控制器掉线（不在 statuses 或 Connected=false）
		if !exists || !status.Connected {
			statusMissCounter[missKey]++
			if statusMissCounter[missKey] >= 3 {
				return &traversal.MotionSafetyFailure{
					ControllerID: binding.ControllerID,
					Axis:         binding.Axis,
					Verdict:      traversal.MotionSafetyStatusUnavailable,
					Target:       0,
					Actual:       0,
					PointIndex:   pointIndex,
				}
			}
			continue
		}

		// 2. 控制器已急停——硬件层已触发急停，遍历层应立即停止避免继续下发指令
		if status.EmergencyStopped {
			return &traversal.MotionSafetyFailure{
				ControllerID: binding.ControllerID,
				Axis:         binding.Axis,
				Verdict:      traversal.MotionSafetyStatusUnavailable,
				Target:       0,
				Actual:       0,
				PointIndex:   pointIndex,
			}
		}

		// 3. 目标轴连续 3 快照缺失
		axisFound := false
		for _, axis := range status.Axes {
			if string(axis.Name) == binding.Axis {
				axisFound = true
				break
			}
		}
		if !axisFound {
			statusMissCounter[missKey]++
			if statusMissCounter[missKey] >= 3 {
				return &traversal.MotionSafetyFailure{
					ControllerID: binding.ControllerID,
					Axis:         binding.Axis,
					Verdict:      traversal.MotionSafetyStatusUnavailable,
					Target:       0,
					Actual:       0,
					PointIndex:   pointIndex,
				}
			}
			continue
		}

		// 轴存在：清零该绑定的缺失计数
		delete(statusMissCounter, missKey)
	}

	return nil
}
