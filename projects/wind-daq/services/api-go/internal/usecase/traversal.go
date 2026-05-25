package usecase

import (
	"fmt"
	"sync"
	"time"

	"wind-daq/services/api-go/internal/core/motion"
	"wind-daq/services/api-go/internal/core/traversal"
	"wind-daq/services/api-go/internal/ports"
)

type TraversalManager struct {
	mu     sync.RWMutex
	reader ports.LatestDataReader
	motion *MotionManager
	sink   ports.TraversalPointSink
	store  ports.TraversalResultStore
	config traversal.Config
	status traversal.Status
}

func NewTraversalManager(reader ports.LatestDataReader, motion *MotionManager, sink ports.TraversalPointSink, store ports.TraversalResultStore) *TraversalManager {
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
	}
	return nil
}

func (m *TraversalManager) Status() traversal.Status {
	m.mu.RLock()
	defer m.mu.RUnlock()
	status := m.status
	status.Results = append([]traversal.PointResult(nil), m.status.Results...)
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

	controllerStatuses := m.motion.StatusAll()
	for _, status := range controllerStatuses {
		if !status.Connected {
			continue
		}
		for axis, position := range availableAxisTargets(status, point) {
			if err := m.motion.MoveTo(status.ID, axis, position); err != nil {
				return m.fail("move %s axis: %v", axis, err)
			}
		}
	}

	deadline := time.Now().Add(2500 * time.Millisecond)
	motionStatuses := m.motion.StatusAll()
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
		motionStatuses = m.motion.StatusAll()
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
		for _, status := range m.motion.StatusAll() {
			for _, axis := range status.Axes {
				if axis.Moving {
					if err := m.motion.Stop(status.ID, axis.Name); err != nil {
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

func availableAxisTargets(status motion.ControllerStatus, point traversal.Point) map[motion.AxisName]float64 {
	targets := make(map[motion.AxisName]float64, len(status.Axes))
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
