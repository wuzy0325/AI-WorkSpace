// Package usecase — traversal 视图响应构造（从 traversal.go 拆分）
//
// 这些方法不修改 TraversalManager 状态，仅把当前状态/历史结果转成 API 层
// 需要的 map[string]any。finalizeSink 归到本文件，它在 RunTraversalLoop
// 完成时把结果交给 sink，是结果输出路径的一部分。
package usecase

import (
	"context"
	"fmt"
	"shared.local/device-sdk/go/pkg/slog"
	"math"
	"sort"

	coreinterp "ai-workspace/shared/algorithms/go/fivehole/interpolation"
	"wind-daq/services/api-go/internal/core/device"
	"wind-daq/services/api-go/internal/core/pressure"
	"wind-daq/services/api-go/internal/core/traversal"
	"wind-daq/services/api-go/internal/ports"
)

// nullIfNonFinite 在 v 为 NaN 时返回 nil（JSON 序列化为 null），否则返回原值。
// 用于 API 响应中的坐标字段：line/rectangle/sector 模式未配置的轴标记为 NaN，
// encoding/json 不支持 NaN/Inf 的序列化，必须转为 nil 避免整个响应崩溃。
func nullIfNonFinite(v float64) any {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return nil
	}
	return v
}

// sanitizePointForJSON 清洗点坐标中的 NaN/Inf，避免 status.results 直接序列化崩溃。
// line/rectangle/sector 路径会通过 markAxesNaN 将未配置轴标记为 NaN；
// 若 results 原样塞进 JSON，encoding/json 会返回 "unsupported value: NaN"，
// 导致 /api/traversal/status 写响应失败，前端轮询停在最后一次成功状态（常见为稳定中）。
func sanitizePointForJSON(point traversal.Point) map[string]any {
	return map[string]any{
		"x": nullIfNonFinite(point.X),
		"y": nullIfNonFinite(point.Y),
		"z": nullIfNonFinite(point.Z),
		"u": nullIfNonFinite(point.U),
	}
}

// sanitizeResultsForJSON 将 PointResult 列表转为 JSON 安全结构。
// 前端主路径消费 dataPoints；results 仅作兼容字段，但仍必须可序列化。
// values（传感器读数）和 calculated（插值结果）中的 NaN/Inf 也必须清洗——
// 通信异常或 Ps=0 等边界可导致 NaN/Inf，原样序列化会崩整个 status API。
func sanitizeResultsForJSON(results []traversal.PointResult) []map[string]any {
	out := make([]map[string]any, 0, len(results))
	for _, result := range results {
		// values: map[int]float64 → map[int]any，NaN/Inf → nil
		sanitizedValues := make(map[int]any, len(result.Values))
		for k, v := range result.Values {
			sanitizedValues[k] = nullIfNonFinite(v)
		}
		item := map[string]any{
			"pointIndex":       result.PointIndex,
			"point":            sanitizePointForJSON(result.Point),
			"timestamp":        result.Timestamp,
			"values":           sanitizedValues,
			"sampleCount":      result.SampleCount,
			"dwellTimeElapsed": result.DwellTimeElapsed,
		}
		if result.Calculated != nil {
			// calculated: 逐字段清洗 NaN/Inf
			item["calculated"] = map[string]any{
				"valid": result.Calculated.Valid,
				"alpha": nullIfNonFinite(result.Calculated.Alpha),
				"beta":  nullIfNonFinite(result.Calculated.Beta),
				"pt":    nullIfNonFinite(result.Calculated.Pt),
				"ps":    nullIfNonFinite(result.Calculated.Ps),
				"mach":  nullIfNonFinite(result.Calculated.Mach),
			}
		}
		if len(result.CustomValues) > 0 {
			item["customValues"] = result.CustomValues
		}
		out = append(out, item)
	}
	return out
}

// finalizeSink 关闭 sink 并释放工作流级互斥锁
//
// 并发关闭防御（v2 关键）：
// Stop 5s 超时路径与 RunTraversalLoop 的 defer 都会调用 finalizeSink，
// 两者可能并发执行。finalizeSink 在锁内快照端口后锁外调用 Close，
// 两次 Close 不互斥。若 adapter 的 Close 不幂等（如已关闭文件句柄再 Close 报错），
// 会触发 panic 或返回错误。用 sync.Once 包裹实际关闭逻辑，确保只执行一次。
//
// 注意：Stop() 路径会主动 Finalize，此处再次 Finalize 是幂等操作
func (m *TraversalManager) finalizeSink() {
	// finalizeOnce 在第一次 finalizeSink 调用时执行实际关闭，
	// 后续调用（Stop 超时 + RunTraversalLoop defer 并发）直接返回。
	m.finalizeOnce.Do(func() {
		m.finalizeSinkInternal()
	})
}

// finalizeSinkInternal 执行实际的端口关闭与锁释放，仅由 finalizeSink 通过
// sync.Once 调用一次。提取为独立方法是为了让 sync.Once 闭包保持简洁。
func (m *TraversalManager) finalizeSinkInternal() {
	m.mu.Lock()
	sink := m.sink
	csvPort := m.csvPort
	resultLogPort := m.resultLogPort
	checkpointPort := m.checkpointPort
	taskID := m.config.TaskID
	session := m.session
	// 清理当前任务的 checkpointPort 引用：与 abortStartLocked 一致，
	// 避免下一次 Start 复用已关闭的实例。csvPort/resultLogPort 是跨任务共享实例
	// （appcontext 装配一次），不能置 nil，需保证 Open 可在 Close 后再次调用。
	m.checkpointPort = nil
	m.mu.Unlock()

	slog.Info("traversal finalizing sink",
		"component", "traversal",
		"task_id", taskID,
	)

	// v2：先关闭结果日志，再关闭 CSV，最后关闭 checkpoint（与打开顺序相反）
	// 顺序一致确保刷盘顺序正确，避免 CSV 未刷盘就关闭结果日志导致数据不一致。
	ctx := context.Background()
	if resultLogPort != nil {
		if err := resultLogPort.Close(ctx); err != nil {
			slog.Error("traversal finalize result log failed",
				"component", "traversal", "task_id", taskID, "error", err)
		}
	}
	if csvPort != nil {
		if err := csvPort.Close(ctx); err != nil {
			slog.Error("traversal finalize csv failed",
				"component", "traversal", "task_id", taskID, "error", err)
		}
	}
	// checkpointPort 是按任务动态创建的（factory.Create），任务结束必须 Close
	// 释放底层资源（文件句柄/锁），高频启停场景下避免泄漏
	if checkpointPort != nil {
		if err := checkpointPort.Close(ctx); err != nil {
			slog.Warn("traversal finalize checkpoint port failed",
				"component", "traversal", "task_id", taskID, "error", err)
		}
	}
	// 若 sink 与 csvPort 为同一实例（v2 装配下常见），csvPort.Close 已通过
	// TraversalCsvWriter.Close → FinalizeTraversal 关闭文件，再调用 sink.FinalizeTraversal
	// 会触发 P1-I6 的双重初始化防御（"遍历 CSV 会话已打开，拒绝重复初始化"反向场景：
	// 文件已关闭但状态机仍记为已初始化）。sinkIsCsvPort 通过指针比较跳过重复 Finalize。
	if sink != nil && !sinkIsCsvPort(sink, csvPort) {
		// FinalizeTraversal 自身需保证幂等（多次调用安全）
		if err := sink.FinalizeTraversal(); err != nil {
			m.mu.Lock()
			if m.status.TaskID == taskID {
				m.setErrorLocked(fmt.Sprintf("finalize traversal sink: %v", err), traversal.ErrSaveFailed)
			}
			m.mu.Unlock()
			slog.Error("traversal finalize sink failed",
				"component", "traversal",
				"task_id", taskID,
				"error", err,
			)
		} else {
			slog.Info("traversal sink finalized",
				"component", "traversal",
				"task_id", taskID,
			)
		}
	}
	// 释放工作流级互斥锁；幂等。
	// 仅 legacy ownership：managed 会话不持有 workflow lease（registry 负责释放）。
	// spec Task 21 Path 4（void 路径）：Release 失败仅记录 Warn，不影响 void 签名；
	// 成功时才记录 Info "traversal lock released"，确保"失败后不记录成功 info"契约。
	// 不强制释放他人锁——resourcelock.Service.Release 自身有 holder 校验。
	managed := session != nil && session.managedOpts != nil
	if !managed && taskID != "" {
		if releaseErr := m.lockService.Release(traversalLockResource, taskID); releaseErr != nil {
			slog.Warn("traversal finalize release lock failed",
				"component", "traversal", "task_id", taskID, "error", releaseErr)
		} else {
			slog.Info("traversal lock released",
				"component", "traversal",
				"task_id", taskID,
			)
		}
	}
}

func (m *TraversalManager) BuildStatusResponse() map[string]any {
	status := m.Status()
	state := string(status.State)

	slog.Info("building status response",
		"component", "traversal",
		"task_id", status.TaskID,
		"state", state,
		"current_point", status.CurrentPoint,
		"total_points", status.TotalPoints,
	)
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
	// currentPoint 用 map[string]any 而非 map[string]float64：
	// line/rectangle/sector 模式通过 markAxesNaN 将未配置轴标记为 NaN，
	// encoding/json 遇到 NaN 会返回 "unsupported value" 错误导致整个 status API 崩溃。
	// NaN 时输出 nil（JSON null），前端用 optional chaining 处理。
	// alpha/beta 为历史兼容字段名（语义是逻辑目标 X/Y），z/u 为后加的逻辑目标 Z/U
	// （仅 custom 模式有实际值；line/rectangle/sector 经 markAxesNaN → null）。
	var currentPoint map[string]any
	if status.CurrentPointCoordinates != nil {
		point := *status.CurrentPointCoordinates
		currentPoint = map[string]any{
			"alpha": nullIfNonFinite(point.X),
			"beta":  nullIfNonFinite(point.Y),
			"z":     nullIfNonFinite(point.Z),
			"u":     nullIfNonFinite(point.U),
		}
	}
	dataPoints := m.BuildDataPoints(status.Results)
	var latestData any
	if len(dataPoints) > 0 {
		latestData = dataPoints[len(dataPoints)-1]
	}
	return map[string]any{
		"taskId":                  status.TaskID,
		// state 必须使用本地变量 state（已根据 currentPoint/totalPoints 修正为 "completed"），
		// 而非原始 status.State——否则完成后前端会读到 state="idle"+status="completed" 的矛盾组合。
		"state":                   state,
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
		// 非致命警告（当前唯一来源：回零失败）。State 为 Completed 时仍可能存在，
		// 前端据此提示"回零未完成"而不影响完成态判定。
		"warning": status.Warning,
		"csvPath": status.CSVPath,
		// results 必须清洗 NaN：line 模式首点保存后 Y/Z/U=NaN，原样序列化会让 status API 崩溃。
		"results":            sanitizeResultsForJSON(status.Results),
		"dataPoints":         dataPoints,
		"latestData":         latestData,
		"validationWarnings": status.ValidationWarnings,
		// 运动安全故障现场快照：仅 handleMotionSafetyFailure 路径写入，前端据此展示
		// "故障发生在哪个控制器/轴/第几个点，目标 vs 实际" 等关键诊断信息。
		// nil 时 JSON 序列化为 null，前端用 optional chaining 处理。
		"motionSafetyFailure": status.MotionSafetyFailure,
		// 等待设备恢复采集（spec-traversal-acquisition-stop）：BuildStatusResponse 手工
		// 构造 map，traversal.Status 新增字段不会自动出现，需显式输出到 legacy /status
		// 与 dual probe /status。
		"waitingForAcquisition":        status.WaitingForAcquisition,
		"waitingDevices":               status.WaitingDevices,
		"waitingForAcquisitionSinceMs": status.WaitingForAcquisitionSinceMs,
	}
}

// BuildDataPoints 从遍历结果构建数据点
func (m *TraversalManager) BuildDataPoints(results []traversal.PointResult) []map[string]any {
	dataPoints := make([]map[string]any, 0, len(results))
	// 一次性读取归一化所需配置：channelLabels（内部键→label）、
	// ChannelRefs（内部键→设备+硬件通道，unitProvider 查询）、
	// PProbePressureType（绝压/表压）、unitProvider。
	// 多字段一次 RLock 避免循环内重复加锁。
	m.mu.RLock()
	channelLabels := m.config.ChannelLabels
	channelRefs := m.config.ResolvedChannelRefs()
	pressureType := m.config.PProbePressureType
	probeType := m.config.ProbeType
	unitProvider := m.unitProvider
	m.mu.RUnlock()
	strategy, strategyOK := probeStrategyFor(probeType)
	for _, result := range results {
		var rawPressure map[string]float64
		var interpolationResult any
		if !strategyOK {
			// 未知探针类型在配置边界被拒绝；防御路径返回零结果而非静默降级。
			rawPressure = map[string]float64{}
			interpolationResult = coreinterp.InterpolationResult{IsValid: false, Warning: fmt.Sprintf("未知探针类型: %q", probeType)}
		} else {
			var probeIn probeCalcInput
			var assembled bool
			rawPressure, probeIn, assembled = buildRawPressureForProbe(result.Values, channelLabels, channelRefs, unitProvider, pressureType, strategy)
			interpolationResult = strategy.viewCalculate(m, probeIn, assembled)
		}
		dataPoints = append(dataPoints, map[string]any{
			"pointId": result.PointIndex + 1,
			// coordinates 用 map[string]any：NaN 轴输出 null 避免 JSON 序列化失败（见 currentPoint 注释）
			"coordinates":         map[string]any{"alpha": nullIfNonFinite(result.Point.X), "beta": nullIfNonFinite(result.Point.Y)},
			"rawPressure":         rawPressure,
			"interpolationResult": interpolationResult,
			"sampleCount":         result.SampleCount,
			"timestamp":           result.Timestamp,
			"dwellTimeElapsed":    result.DwellTimeElapsed,
		})
	}
	return dataPoints
}

// BuildRawPressure 从通道值构建原始压力数据和插值输入。
//
// 归一化策略（spec §A4）：
//   - P1-P5：按 Unit 换算到 Pa；若 pressureType=="absolute" 再减去已归一化的 Patm
//   - Patm：仅按 Unit 换算到 Pa（绝压值，不减）
//   - Tatm：不参与归一化（温度通道，保持原值）
//
// 降级策略（spec §A4）：
//   - unitProvider == nil：记 warning，跳过换算保持原值（兼容旧测试与离线场景）
//   - 查询单位失败：跳过该通道换算记 warning，其他通道正常归一化
//
// 参数：
//   - values: 内部通道键→原始读数
//   - labels: 内部通道键→标签（P1-P5/Patm/Tatm）
//   - refs: 内部通道键→物理通道（设备+硬件通道索引），用于 unitProvider 按
//     真实设备查询通道 Unit；跨设备绑定（五孔在 A、大气压/温度在 B）时必须正确传入
//   - unitProvider: 通道单位提供端口，nil 时走降级路径
//   - pressureType: "gauge"|"absolute"，空串按 "gauge" 兜底（由 pressure.NormalizePressureToGaugePa 处理）
func BuildRawPressure(
	values map[int]float64,
	labels map[int]string,
	refs map[int]traversal.ChannelRef,
	unitProvider ports.ChannelUnitProvider,
	pressureType string,
) (map[string]float64, coreinterp.InterpolationInput, bool) {
	// BuildRawPressure 是五孔专用稳定入口：签名固定返回 coreinterp.InterpolationInput，
	// 内部调用 buildRawPressureForProbe 仅消费 raw/ok，丢弃 probeIn（七孔走
	// probeStrategies[ProbeTypeSevenHole] 独立路径，不经此函数）。
	raw, _, ok := buildRawPressureForProbe(values, labels, refs, unitProvider, pressureType, probeStrategies[traversal.ProbeTypeFiveHole])
	input := coreinterp.InterpolationInput{
		P1:   raw["P1"],
		P2:   raw["P2"],
		P3:   raw["P3"],
		P4:   raw["P4"],
		P5:   raw["P5"],
		PAtm: raw["Patm"],
		TAtm: raw["Tatm"],
	}
	return raw, input, ok
}

// buildRawPressureForProbe 探针感知的原始压力装配：标签集与齐全性校验由
// strategy.pressureLabels 驱动（五孔 P1..P5 / 七孔 P1..P7）。legacy 无标签
// 回退（按 CH 顺序映射 P1..P5）仅服务五孔旧配置（spec 附录假设 6）。
func buildRawPressureForProbe(
	values map[int]float64,
	labels map[int]string,
	refs map[int]traversal.ChannelRef,
	unitProvider ports.ChannelUnitProvider,
	pressureType string,
	strategy probeStrategy,
) (map[string]float64, probeCalcInput, bool) {
	raw := make(map[string]float64, 9)
	// labelToRef 反查表：label→物理通道，供 unitProvider.ChannelUnit 按真实设备查询。
	// labels 是 内部键→label，遍历一次建反查避免后续重复扫描。
	labelToRef := make(map[string]traversal.ChannelRef, len(labels))
	isFiveHole := len(strategy.pressureLabels) == 5
	if len(labels) > 0 {
		for chIdx, value := range values {
			if label, ok := labels[chIdx]; ok && label != "" {
				raw[label] = value
				labelToRef[label] = refs[chIdx]
			}
		}
	} else if isFiveHole {
		// 兼容旧行为：通道索引升序对应 P1..Tatm（仅五孔 legacy 配置）
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
			labelToRef[label] = refs[orderedKeys[i]]
		}
	}

	// normalized 跟踪"全部压力通道是否成功归一化到 Pa+表压"。
	// false 时调用方应跳过插值（ok=false），避免混合单位输入或假表压。
	// 三种 false 场景：
	//   1. unitProvider=nil：降级路径，保留原值（兼容旧测试/离线，但插值不可信）
	//   2. labels=nil/空：legacy 路径，按索引猜标签不可靠，跳过归一化
	//   3. normalizeRawPressure 返回 false：Patm 失败或 P1-P5 部分失败
	normalized := false
	if unitProvider == nil {
		slog.Warn("traversal BuildRawPressure: unitProvider is nil, skip normalization",
			"component", "traversal",
		)
	} else if len(labels) == 0 {
		// legacy 路径：按索引猜标签不可靠（设备通道顺序可能与 P1..Tatm 不一致），
		// 跳过归一化避免基于错误标签换算产生静默错误。
		slog.Warn("traversal BuildRawPressure: labels empty (legacy mode), skip normalization",
			"component", "traversal",
		)
	} else {
		normalized = normalizeRawPressure(raw, labelToRef, unitProvider, pressureType, strategy.pressureLabels)
	}

	input := probeCalcInput{PAtm: raw["Patm"], TAtm: raw["Tatm"]}
	for i, label := range strategy.pressureLabels {
		input.P[i] = raw[label]
	}
	hasAllLabels := true
	for _, label := range strategy.pressureLabels {
		if _, ok := raw[label]; !ok {
			hasAllLabels = false
			break
		}
	}
	if _, ok := raw["Patm"]; !ok {
		hasAllLabels = false
	}
	if _, ok := raw["Tatm"]; !ok {
		hasAllLabels = false
	}
	// ok 语义：标签齐全 && 全部归一化成功。
	// 调用方在 ok=true 时才调用插值器，避免混合单位或假表压输入。
	return raw, input, hasAllLabels && normalized
}

// sharedUnitConverter 包级 UnitConverter 单例。
//
// 为什么用包级单例：
//   - device.UnitConverter 无状态（families 是固定单位族），线程安全；
//   - 避免每次 BuildRawPressure 调用都 new 一个，浪费 GC；
//   - 不引入新字段/setter，保持 BuildRawPressure 签名与 spec §A4 一致。
var sharedUnitConverter = device.NewUnitConverter()

// normalizeRawPressure 原地归一化 raw map 中的压力通道。
//
// 归一化顺序：
//  1. 先用 ConvertToPa 归一化 Patm（得到 Pa 单位的绝压值）；
//  2. 再用 NormalizePressureToGaugePa 归一化探针孔道（绝压类型时减去 Patm）；
//  3. Tatm 不参与。
//
// 返回值 normalized：所有压力通道（Patm + 探针孔道）是否全部成功归一化到 Pa+表压。
// false 时调用方应跳过插值，避免混合单位输入或假表压。
//
// 失败处理：
//   - 单位查询/换算失败：跳过该通道（保留原值），记 warning，normalized=false
//   - 绝压 + Patm 失败：跳过探针孔道归一化（避免 patmPa=0 导致假表压），normalized=false
//   - 表压 + Patm 失败：探针孔道仍可归一化（不减 Patm），但 Patm 字段保留原值非 Pa，normalized=false
//
// probeLabels 探针孔道标签集（五孔 P1..P5 / 七孔 P1..P7），由策略表传入。
func normalizeRawPressure(
	raw map[string]float64,
	labelToRef map[string]traversal.ChannelRef,
	unitProvider ports.ChannelUnitProvider,
	pressureType string,
	probeLabels []string,
) bool {
	// Step 1: 归一化 Patm（仅单位换算，不减）。
	// patmValid 跟踪 Patm 是否成功归一化：绝压场景需要 patmPa 做减法，
	// 表压场景虽不减但插值器仍需 Patm 为 Pa 单位，故任一场景 Patm 失败都判 normalized=false。
	patmValid := false
	patmPa := 0.0
	if ref, ok := labelToRef["Patm"]; ok {
		if unit, err := unitProvider.ChannelUnit(ref.DeviceID, ref.Index); err == nil {
			if pa, err := pressure.ConvertToPa(raw["Patm"], unit, sharedUnitConverter); err == nil {
				patmPa = pa
				raw["Patm"] = pa
				patmValid = true
			} else {
				slog.Warn("traversal BuildRawPressure: convert Patm unit failed, keep raw value",
					"component", "traversal",
					"device_id", ref.DeviceID,
					"unit", unit,
					"error", err,
				)
			}
		} else {
			slog.Warn("traversal BuildRawPressure: query Patm channel unit failed, keep raw value",
				"component", "traversal",
				"device_id", ref.DeviceID,
				"error", err,
			)
		}
	}

	// 绝压场景必须 Patm 有效：否则 P1-P5 减 patmPa=0 会得到"假表压"
	// （绝压值被误认为表压值，造成约 1 atm 系统偏差），让 CSV/UI/插值器产生错误数据。
	// 跳过 P1-P5 归一化（保留原值），normalized=false 让调用方跳过插值。
	if pressure.PressureType(pressureType) == pressure.PressureTypeAbsolute && !patmValid {
		slog.Warn("traversal BuildRawPressure: absolute pressure type requires valid Patm, skip P1-P5 normalization",
			"component", "traversal",
		)
		return false
	}

	// Step 2: 归一化探针孔道（绝压类型减 patmPa）。
	// 单位查询/换算失败时跳过该通道，保留原值，allProbeNormalized=false。
	allProbeNormalized := true
	for _, label := range probeLabels {
		ref, ok := labelToRef[label]
		if !ok {
			// 标签缺失由 hasAllLabels 兜底，此处不重复记 warning
			allProbeNormalized = false
			continue
		}
		unit, err := unitProvider.ChannelUnit(ref.DeviceID, ref.Index)
		if err != nil {
			slog.Warn("traversal BuildRawPressure: query channel unit failed, skip normalization",
				"component", "traversal",
				"label", label,
				"device_id", ref.DeviceID,
				"error", err,
			)
			allProbeNormalized = false
			continue
		}
		normalized, err := pressure.NormalizePressureToGaugePa(raw[label], unit, pressureType, patmPa, sharedUnitConverter)
		if err != nil {
			slog.Warn("traversal BuildRawPressure: convert unit failed, skip normalization",
				"component", "traversal",
				"label", label,
				"device_id", ref.DeviceID,
				"unit", unit,
				"error", err,
			)
			allProbeNormalized = false
			continue
		}
		raw[label] = normalized
	}
	// Tatm 不参与归一化，保持原值
	// 表压场景 Patm 失败时 P1-P5 可能全部成功，但 Patm 字段非 Pa 仍判 normalized=false
	return allProbeNormalized && patmValid
}
