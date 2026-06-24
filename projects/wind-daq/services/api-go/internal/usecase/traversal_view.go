// Package usecase — traversal 视图响应构造（从 traversal.go 拆分）
//
// 这些方法不修改 TraversalManager 状态，仅把当前状态/历史结果转成 API 层
// 需要的 map[string]any。finalizeSink 归到本文件，它在 RunTraversalLoop
// 完成时把结果交给 sink，是结果输出路径的一部分。
package usecase

import (
	"fmt"
	"sort"

	coreinterp "ai-workspace/shared/algorithms/go/fivehole/interpolation"
	"wind-daq/services/api-go/internal/core/resourcelock"
	"wind-daq/services/api-go/internal/core/traversal"
)

func (m *TraversalManager) finalizeSink() {
	m.mu.Lock()
	sink := m.sink
	taskID := m.config.TaskID
	m.mu.Unlock()
	if sink != nil {
		// FinalizeTraversal 自身需保证幂等（多次调用安全）
		if err := sink.FinalizeTraversal(); err != nil {
			m.mu.Lock()
			if m.status.TaskID == taskID {
				m.setErrorLocked(fmt.Sprintf("finalize traversal sink: %v", err), traversal.ErrSaveFailed)
			}
			m.mu.Unlock()
		}
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
