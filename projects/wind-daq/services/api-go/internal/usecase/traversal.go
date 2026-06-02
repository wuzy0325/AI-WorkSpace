package usecase

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"sync"
	"time"

	coreinterp "ai-workspace/shared/algorithms/go/fivehole/interpolation"
	"shared.local/device-sdk/go/motion/core"
	"wind-daq/services/api-go/internal/core/traversal"
	"wind-daq/services/api-go/internal/ports"
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

func (m *TraversalManager) RunCurrentPoint() error {
	m.mu.Lock()
	if m.status.State != traversal.StateRunning {
		m.mu.Unlock()
		return fmt.Errorf("traversal is not running")
	}
	if m.reader == nil {
		m.setErrorLocked("latest data reader is required")
		m.mu.Unlock()
		return fmt.Errorf("latest data reader is required")
	}
	if m.motion == nil {
		m.setErrorLocked("motion manager is required")
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

	ctx := context.Background()
	controllerStatuses := m.motion.StatusAll(ctx)
	for _, status := range controllerStatuses {
		if !status.Connected {
			continue
		}
		for axis, position := range availableAxisTargets(status, point) {
			if err := m.motion.MoveTo(ctx, status.ID, axis, position); err != nil {
				return m.fail("move %s axis: %v", axis, err)
			}
		}
	}

	deadline := time.Now().Add(2500 * time.Millisecond)
	motionStatuses := m.motion.StatusAll(ctx)
	for time.Now().Before(deadline) {
		allReached := true
		for _, motionStatus := range motionStatuses {
			for _, axis := range motionStatus.Axes {
				target, hasTarget := availableAxisTargets(motionStatus, point)[axis.Name]
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
			break
		}
		time.Sleep(50 * time.Millisecond)
		motionStatuses = m.motion.StatusAll(ctx)
	}

	payload, ok := m.reader.GetLatestData(config.DeviceID)
	if !ok {
		return m.fail("no data available for device %s", config.DeviceID)
	}
	result := traversal.PointResult{
		PointIndex: pointIndex,
		Point:      point,
		Timestamp:  payload.Timestamp,
		Values:     valuesForChannels(payload, config.Channels),
	}
	if len(result.Values) != len(config.Channels) {
		return m.fail("latest data does not contain all requested channels")
	}
	if m.sink != nil {
		if err := m.sink.WriteTraversalPoint(result); err != nil {
			return m.fail("write traversal point: %v", err)
		}
	}

	m.mu.Lock()
	m.status.Results = append(m.status.Results, result)
	m.status.CurrentPoint++
	allDone := m.status.CurrentPoint >= len(m.config.Path)
	if allDone {
		m.status.State = traversal.StateIdle
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

func (m *TraversalManager) Pause() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.status.State != traversal.StateRunning {
		return fmt.Errorf("traversal is not running")
	}
	m.status.State = traversal.StatePaused
	return nil
}

func (m *TraversalManager) Resume() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.status.State != traversal.StatePaused {
		return fmt.Errorf("traversal is not paused")
	}
	m.status.State = traversal.StateRunning
	return nil
}

func (m *TraversalManager) Stop() error {
	if m.motion != nil {
		ctx := context.Background()
		for _, status := range m.motion.StatusAll(ctx) {
			for _, axis := range status.Axes {
				if axis.Moving {
					if err := m.motion.Stop(ctx, status.ID, axis.Name); err != nil {
						return err
					}
				}
			}
		}
	}
	m.mu.Lock()
	m.status.State = traversal.StateStopped
	if m.store != nil {
		status := m.status
		status.Results = append([]traversal.PointResult(nil), m.status.Results...)
		if err := m.store.Save(m.config.TaskID, status); err != nil {
			m.mu.Unlock()
			return fmt.Errorf("save traversal result: %v", err)
		}
	}
	m.mu.Unlock()
	return nil
}

func (m *TraversalManager) GetResult(taskID string) (traversal.Status, bool) {
	if m.store == nil {
		return traversal.Status{}, false
	}
	return m.store.Get(taskID)
}

func (m *TraversalManager) fail(format string, args ...any) error {
	message := fmt.Sprintf(format, args...)
	m.mu.Lock()
	m.setErrorLocked(message)
	m.mu.Unlock()
	return fmt.Errorf("%s", message)
}

func (m *TraversalManager) setErrorLocked(message string) {
	m.status.State = traversal.StateError
	m.status.LastError = message
}

func abs(f float64) float64 {
	if f < 0 {
		return -f
	}
	return f
}

func availableAxisTargets(status core.ControllerStatus, point traversal.Point) map[core.AxisName]float64 {
	targets := make(map[core.AxisName]float64, len(status.Axes))
	for _, axis := range status.Axes {
		switch axis.Name {
		case "X":
			targets[axis.Name] = point.X
		case "Y":
			targets[axis.Name] = point.Y
		case "Z":
			targets[axis.Name] = point.Z
		}
	}
	return targets
}

// RunTraversalLoop 执行遍历任务循环
func (m *TraversalManager) RunTraversalLoop(dwell time.Duration) {
	if dwell <= 0 {
		dwell = 100 * time.Millisecond
	}
	for {
		status := m.Status()
		switch status.State {
		case traversal.StateRunning:
			if status.TotalPoints > 0 && status.CurrentPoint >= status.TotalPoints {
				return
			}
			if err := m.RunCurrentPoint(); err != nil {
				return
			}
			time.Sleep(dwell)
		case traversal.StatePaused:
			time.Sleep(200 * time.Millisecond)
		default:
			return
		}
	}
}

// BuildStatusResponse 构建遍历状态响应
func (m *TraversalManager) BuildStatusResponse() map[string]any {
	status := m.Status()
	state := string(status.State)
	if status.State == traversal.StateIdle && status.TotalPoints > 0 && status.CurrentPoint >= status.TotalPoints {
		state = "completed"
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
		"status":                  state,
		"currentPoint":            status.CurrentPoint,
		"currentPointCoordinates": currentPoint,
		"completedPoints":         status.CurrentPoint,
		"totalPoints":             status.TotalPoints,
		"progress":                progress,
		"startTime":               status.StartedAt,
		"lastError":               status.LastError,
		"results":                 status.Results,
		"dataPoints":              dataPoints,
		"latestData":              latestData,
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
			"sampleCount":         1,
			"timestamp":           result.Timestamp,
			"dwellTimeElapsed":    0,
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
