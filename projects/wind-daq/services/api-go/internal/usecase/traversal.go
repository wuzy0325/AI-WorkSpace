package usecase

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"sync"
	"time"

	coreinterp "ai-workspace/shared/algorithms/go/fivehole/interpolation"
	"wind-daq/services/api-go/internal/core/motion"
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

	m.waitForMotionComplete(ctx, point)

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

func (m *TraversalManager) waitForMotionComplete(ctx context.Context, point traversal.Point) {
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	deadline := time.Now().Add(2500 * time.Millisecond)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if time.Now().After(deadline) {
				return
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
				return
			}
		}
	}
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

type traversalAPIConfig struct {
	Name   string `json:"name"`
	Layout struct {
		Pattern string `json:"pattern"`
		Line    *struct {
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
	DwellTimeMs int `json:"dwellTimeMs"`
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
		Pattern: cfg.Layout.Pattern,
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
	config := traversal.Config{
		TaskID: fmt.Sprintf("trav-%d", time.Now().UnixMilli()),
		DeviceID: deviceID, Channels: channels, Path: points,
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
