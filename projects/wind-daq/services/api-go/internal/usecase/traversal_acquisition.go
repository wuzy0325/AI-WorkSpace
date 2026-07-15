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

	// 阶段2：等待稳定
	m.updatePhase(taskID, traversal.StateStabilizing, traversal.PhaseStabilizing, pointIndex, len(config.Path))
	stabFailure := m.waitForStabilization(taskID, point, pointIndex)
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
		// 提交成功后才推进 snapshot.CommitSeq 和 CommittedPoints（线性化点 = Checkpoint 持久化成功）
		// CommittedPoints 同步更新，与正常分支保持一致，避免 session.snapshot 字段陈旧
		m.mu.Lock()
		session.snapshot.CommitSeq = commitSeq
		session.snapshot.CommittedPoints = int(commitSeq)
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

	// v2：提交成功后才推进 snapshot.CommitSeq 和 CommittedPoints
	// CommittedPoints 必须同步更新：session.snapshot 作为"运行态权威快照"，
	// CommittedPoints 恒为 0 会让任何读取该字段的代码（调试端点/未来扩展）拿到错误值。
	m.mu.Lock()
	runSession.snapshot.CommitSeq = commitSeq
	runSession.snapshot.CommittedPoints = int(commitSeq)
	m.mu.Unlock()

	// 向后兼容：旧 sink 路径仍执行，但不参与事务回滚
	// 若 sink 与 csvPort 为同一实例（v2 装配下常见），CSV 已通过 csvPort.Append 写入，
	// 这里再调用 sink.WriteTraversalPoint 会重复落盘同一行，破坏 CSV 行号 ↔ commitSeq 一致性。
	// sinkIsCsvPort 通过类型断言 + 指针比较检测同实例，跳过重复写入。
	if m.sink != nil && !sinkIsCsvPort(m.sink, m.csvPort) {
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
	// 旧 saveCheckpoint 路径基址：必须用 ResolveOutputPath 解析出完整 CSV 文件路径，
	// 不能直接用 config.SavePath（可能是目录）。
	// 否则 saveCheckpoint 派生 .checkpoint.json 时会拼成 "目录.checkpoint.json"，
	// 落在父目录而非数据目录内；旧格式 checkpoint 的 SavePath 为目录还会污染 Resume 回退路径
	// （ResumeFromCheckpoint 已统一改用 ResolveOutputPath 重算，这里保持一致源头）。
	// 若 sink 实现了 OutputPath()（如 TraversalCsvWriter 在 InitializeTraversal 后可提供），
	// 优先用 sink 实际打开的文件路径——v2 装配下 sink/csvPort 同实例，
	// sink.OutputPath() 即 csvPort.Open 实际创建的文件路径（含 -2/-3 撞名后缀）。
	checkpointSavePath := traversal.ResolveOutputPath(config)
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
			return fmt.Errorf("save traversal result: %w", err)
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
	m.mu.RLock()
	var snapshot traversal.TraversalRunSnapshot
	if session != nil {
		snapshot = session.snapshot
	}
	m.mu.RUnlock()
	cp := buildCheckpoint(taskID, snapshot, commitSeq, snapshot.CSVPath, nil, traversal.StateRunning)

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
func (m *TraversalManager) waitForStabilization(taskID string, point traversal.Point, pointIndex int) *traversal.MotionSafetyFailure {
	m.mu.RLock()
	stab := m.stabilization
	dwellMs := m.config.DwellTimeMs
	deviceID := m.config.DeviceID
	channels := m.config.Channels
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
	minWait := time.Duration(adaptive.MinWaitMs) * time.Millisecond
	maxWait := time.Duration(adaptive.MaxWaitMs) * time.Millisecond
	checkInterval := time.Duration(adaptive.CheckIntervalMs) * time.Millisecond
	threshold := adaptive.StabilityThreshold

	// 至少等待最小时间——期间同样进行安全复检
	minDeadline := time.Now().Add(minWait)
	minTicker := time.NewTicker(motionCompletePoll)
	defer minTicker.Stop()
	for time.Now().Before(minDeadline) {
		if m.isTaskCancelled(taskID) {
			return nil
		}
		if f := m.recheckMotionSafety(motionAxes, safetyCfg, point, pointIndex, watchdog, statusMissCounter); f != nil {
			return f
		}
		<-minTicker.C
	}

	// 读取初始参考值
	prevValues := m.readCurrentValues(deviceID, channels)
	start := time.Now()
	stableCount := 0

	for time.Since(start) < maxWait && stableCount < adaptive.ConsecutiveChecks {
		if m.isTaskCancelled(taskID) {
			return nil
		}
		// 自适应采样前先做安全复检——任何故障立即中断
		if f := m.recheckMotionSafety(motionAxes, safetyCfg, point, pointIndex, watchdog, statusMissCounter); f != nil {
			return f
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

	// 自诊断：无有效样本时区分"设备未在采集"与"通道索引对不上"两类根因。
	// everOk=false → GetLatestData 始终返回 ok=false（设备未采集或 deviceID 不匹配）；
	// everOk=true 但 validSamples==0 → 设备有数据，但 payload 通道集合不包含请求的通道。
	var everOk bool
	var lastIndices []int
	var lastChannelCount int
	// 采样失败计数：区分"设备无数据"与"通道不匹配"两类失败
	var noDataCount int
	var channelMismatchCount int
	// 时间戳去重：跳过同一帧的重复读取。
	// 当设备刷新周期 > 轮询间隔（10ms）时，多次 GetLatestData 可能返回同一帧，
	// 直接累加会导致假平均（同一帧的值反复相加）。此处用时间戳确保每帧只采一次。
	var lastTimestamp int64 = -1

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
		if time.Now().After(deadline) {
			slog.Warn("traversal averaged sampling timeout",
				"component", "traversal",
				"task_id", taskID,
				"valid_samples", validSamples,
				"target_samples", samplesPerPoint,
				"no_data_count", noDataCount,
				"channel_mismatch_count", channelMismatchCount,
				"device_ever_answered", everOk,
			)
			break // 超时保护：不再继续采样
		}
		payload, ok := m.reader.GetLatestData(deviceID)
		if !ok {
			noDataCount++
			time.Sleep(acquisitionBatchPoll)
			continue
		}
		everOk = true
		if payload.Timestamp <= lastTimestamp {
			noDataCount++
			time.Sleep(acquisitionBatchPoll)
			continue
		}
		lastTimestamp = payload.Timestamp
		lastChannelCount = len(payload.Channels)
		lastIndices = append(lastIndices[:0], payload.ChannelIndices...)
		values := valuesForChannels(payload, channels)
		if len(values) == len(channels) {
			for k, v := range values {
				totals[k] += v
			}
			validSamples++
		} else {
			channelMismatchCount++
		}
		time.Sleep(acquisitionBatchPoll)
	}

	if validSamples == 0 {
		slog.Error("traversal no valid samples",
			"component", "traversal",
			"task_id", taskID,
			"device_id", deviceID,
			"requested_channels", channels,
			"device_ever_answered", everOk,
			"last_payload_channel_count", lastChannelCount,
			"last_payload_channel_indices", lastIndices,
		)
		return nil, fmt.Errorf("no valid samples collected within timeout")
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
//  7. 120s 兜底超时——返回 (false, timeout, nil)，调用方按 ErrMotionTimeout 处理
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

			// 5. 120s 兜底超时——返回 (false, timeout, nil)，调用方按 ErrMotionTimeout 处理
			if time.Now().After(deadline) {
				return false, traversal.MotionInterruptTimeout, nil
			}
		}
	}
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
