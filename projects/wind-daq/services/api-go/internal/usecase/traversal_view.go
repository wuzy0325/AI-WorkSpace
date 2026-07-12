// Package usecase — traversal 视图响应构造（从 traversal.go 拆分）
//
// 这些方法不修改 TraversalManager 状态，仅把当前状态/历史结果转成 API 层
// 需要的 map[string]any。finalizeSink 归到本文件，它在 RunTraversalLoop
// 完成时把结果交给 sink，是结果输出路径的一部分。
package usecase

import (
	"fmt"
	"log/slog"
	"math"
	"sort"

	coreinterp "ai-workspace/shared/algorithms/go/fivehole/interpolation"
	"wind-daq/services/api-go/internal/core/device"
	"wind-daq/services/api-go/internal/core/pressure"
	"wind-daq/services/api-go/internal/core/resourcelock"
	"wind-daq/services/api-go/internal/core/traversal"
	"wind-daq/services/api-go/internal/ports"
)

// nullIfNaN 在 v 为 NaN 时返回 nil（JSON 序列化为 null），否则返回原值。
// 用于 API 响应中的坐标字段：line/rectangle/sector 模式未配置的轴标记为 NaN，
// encoding/json 不支持 NaN/Inf 的序列化，必须转为 nil 避免整个响应崩溃。
func nullIfNaN(v float64) any {
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
		"x": nullIfNaN(point.X),
		"y": nullIfNaN(point.Y),
		"z": nullIfNaN(point.Z),
		"u": nullIfNaN(point.U),
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
			sanitizedValues[k] = nullIfNaN(v)
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
				"alpha": nullIfNaN(result.Calculated.Alpha),
				"beta":  nullIfNaN(result.Calculated.Beta),
				"pt":    nullIfNaN(result.Calculated.Pt),
				"ps":    nullIfNaN(result.Calculated.Ps),
				"mach":  nullIfNaN(result.Calculated.Mach),
			}
		}
		if len(result.CustomValues) > 0 {
			item["customValues"] = result.CustomValues
		}
		out = append(out, item)
	}
	return out
}

func (m *TraversalManager) finalizeSink() {
	m.mu.Lock()
	sink := m.sink
	taskID := m.config.TaskID
	m.mu.Unlock()

	slog.Info("traversal finalizing sink",
		"component", "traversal",
		"task_id", taskID,
	)

	if sink != nil {
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
	// 释放工作流级互斥锁；幂等
	if taskID != "" {
		_ = resourcelock.Default().Release(traversalLockResource, taskID)
		slog.Info("traversal lock released",
			"component", "traversal",
			"task_id", taskID,
		)
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
	var currentPoint map[string]any
	if status.CurrentPointCoordinates != nil {
		point := *status.CurrentPointCoordinates
		currentPoint = map[string]any{"alpha": nullIfNaN(point.X), "beta": nullIfNaN(point.Y)}
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
		// results 必须清洗 NaN：line 模式首点保存后 Y/Z/U=NaN，原样序列化会让 status API 崩溃。
		"results":            sanitizeResultsForJSON(status.Results),
		"dataPoints":         dataPoints,
		"latestData":         latestData,
		"validationWarnings": status.ValidationWarnings,
	}
}

// BuildDataPoints 从遍历结果构建数据点
func (m *TraversalManager) BuildDataPoints(results []traversal.PointResult) []map[string]any {
	dataPoints := make([]map[string]any, 0, len(results))
	// 一次性读取归一化所需配置：channelLabels（channelIndex→label）、
	// DeviceID（unitProvider 查询）、PProbePressureType（绝压/表压）、unitProvider。
	// 多字段一次 RLock 避免循环内重复加锁。
	m.mu.RLock()
	channelLabels := m.config.ChannelLabels
	deviceID := m.config.DeviceID
	pressureType := m.config.PProbePressureType
	unitProvider := m.unitProvider
	m.mu.RUnlock()
	for _, result := range results {
		rawPressure, input, ok := BuildRawPressure(result.Values, channelLabels, deviceID, unitProvider, pressureType)
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
			"pointId": result.PointIndex + 1,
			// coordinates 用 map[string]any：NaN 轴输出 null 避免 JSON 序列化失败（见 currentPoint 注释）
			"coordinates":         map[string]any{"alpha": nullIfNaN(result.Point.X), "beta": nullIfNaN(result.Point.Y)},
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
//   - values: 通道索引→原始读数
//   - labels: 通道索引→标签（P1-P5/Patm/Tatm）
//   - deviceID: 设备 ID，用于 unitProvider 查询通道 Unit
//   - unitProvider: 通道单位提供端口，nil 时走降级路径
//   - pressureType: "gauge"|"absolute"，空串按 "gauge" 兜底（由 pressure.NormalizePressureToGaugePa 处理）
func BuildRawPressure(
	values map[int]float64,
	labels map[int]string,
	deviceID string,
	unitProvider ports.ChannelUnitProvider,
	pressureType string,
) (map[string]float64, coreinterp.InterpolationInput, bool) {
	raw := make(map[string]float64, 7)
	// labelToChannel 反查表：label→channelIndex，供 unitProvider.ChannelUnit 查询。
	// labels 是 channelIndex→label，遍历一次建反查避免后续重复扫描。
	labelToChannel := make(map[string]int, len(labels))
	if len(labels) > 0 {
		for chIdx, value := range values {
			if label, ok := labels[chIdx]; ok && label != "" {
				raw[label] = value
				labelToChannel[label] = chIdx
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
			labelToChannel[label] = orderedKeys[i]
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
			"device_id", deviceID,
		)
	} else if len(labels) == 0 {
		// legacy 路径：按索引猜标签不可靠（设备通道顺序可能与 P1..Tatm 不一致），
		// 跳过归一化避免基于错误标签换算产生静默错误。
		slog.Warn("traversal BuildRawPressure: labels empty (legacy mode), skip normalization",
			"component", "traversal",
			"device_id", deviceID,
		)
	} else {
		normalized = normalizeRawPressure(raw, labelToChannel, deviceID, unitProvider, pressureType)
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
	hasAllLabels := hasP1 && hasP2 && hasP3 && hasP4 && hasP5 && hasPatm && hasTatm
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
//  2. 再用 NormalizePressureToGaugePa 归一化 P1-P5（绝压类型时减去 Patm）；
//  3. Tatm 不参与。
//
// 返回值 normalized：所有压力通道（Patm + P1-P5）是否全部成功归一化到 Pa+表压。
// false 时调用方应跳过插值，避免混合单位输入或假表压。
//
// 失败处理：
//   - 单位查询/换算失败：跳过该通道（保留原值），记 warning，normalized=false
//   - 绝压 + Patm 失败：跳过 P1-P5 归一化（避免 patmPa=0 导致假表压），normalized=false
//   - 表压 + Patm 失败：P1-P5 仍可归一化（不减 Patm），但 Patm 字段保留原值非 Pa，normalized=false
func normalizeRawPressure(
	raw map[string]float64,
	labelToChannel map[string]int,
	deviceID string,
	unitProvider ports.ChannelUnitProvider,
	pressureType string,
) bool {
	// Step 1: 归一化 Patm（仅单位换算，不减）。
	// patmValid 跟踪 Patm 是否成功归一化：绝压场景需要 patmPa 做减法，
	// 表压场景虽不减但插值器仍需 Patm 为 Pa 单位，故任一场景 Patm 失败都判 normalized=false。
	patmValid := false
	patmPa := 0.0
	if chIdx, ok := labelToChannel["Patm"]; ok {
		if unit, err := unitProvider.ChannelUnit(deviceID, chIdx); err == nil {
			if pa, err := pressure.ConvertToPa(raw["Patm"], unit, sharedUnitConverter); err == nil {
				patmPa = pa
				raw["Patm"] = pa
				patmValid = true
			} else {
				slog.Warn("traversal BuildRawPressure: convert Patm unit failed, keep raw value",
					"component", "traversal",
					"device_id", deviceID,
					"unit", unit,
					"error", err,
				)
			}
		} else {
			slog.Warn("traversal BuildRawPressure: query Patm channel unit failed, keep raw value",
				"component", "traversal",
				"device_id", deviceID,
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
			"device_id", deviceID,
		)
		return false
	}

	// Step 2: 归一化 P1-P5（绝压类型减 patmPa）。
	// 单位查询/换算失败时跳过该通道，保留原值，allProbeNormalized=false。
	probeLabels := []string{"P1", "P2", "P3", "P4", "P5"}
	allProbeNormalized := true
	for _, label := range probeLabels {
		chIdx, ok := labelToChannel[label]
		if !ok {
			// 标签缺失由 hasAllLabels 兜底，此处不重复记 warning
			allProbeNormalized = false
			continue
		}
		unit, err := unitProvider.ChannelUnit(deviceID, chIdx)
		if err != nil {
			slog.Warn("traversal BuildRawPressure: query channel unit failed, skip normalization",
				"component", "traversal",
				"label", label,
				"device_id", deviceID,
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
				"device_id", deviceID,
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
