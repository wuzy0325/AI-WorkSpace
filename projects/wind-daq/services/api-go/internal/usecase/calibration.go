package usecase

import (
	"context"
	"fmt"
	"sync"
	"time"

	"wind-daq/services/api-go/internal/core/calibration"
	"wind-daq/services/api-go/internal/core/device"
	"wind-daq/services/api-go/internal/ports"
)

const defaultSampleInterval = 50 * time.Millisecond

type CalibrationManager struct {
	mu     sync.RWMutex
	reader ports.LatestDataReader
	motion *MotionManager
	sink   ports.CalibrationPointSink
	store  ports.CalibrationResultStore
	config calibration.Config
	status calibration.Status
}

func NewCalibrationManager(reader ports.LatestDataReader, motion *MotionManager, sink ports.CalibrationPointSink, store ports.CalibrationResultStore) *CalibrationManager {
	return &CalibrationManager{
		reader: reader,
		motion: motion,
		sink:   sink,
		store:  store,
		status: calibration.Status{State: calibration.StateIdle},
	}
}

func (m *CalibrationManager) Start(config calibration.Config) error {
	if config.TaskID == "" {
		return fmt.Errorf("taskID is required")
	}
	if config.DeviceID == "" {
		return fmt.Errorf("deviceID is required")
	}
	if len(config.Channels) == 0 {
		return fmt.Errorf("channels are required")
	}
	if len(config.PressurePoints) < 2 {
		return fmt.Errorf("at least two pressure points are required")
	}
	if config.AverageSamples < 1 {
		return fmt.Errorf("averageSamples must be greater than zero")
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	m.config = config
	m.status = calibration.Status{
		TaskID:      config.TaskID,
		State:       calibration.StateRunning,
		TotalPoints: len(config.PressurePoints),
	}
	return nil
}

func (m *CalibrationManager) Status() calibration.Status {
	m.mu.RLock()
	defer m.mu.RUnlock()
	status := m.status
	status.Results = append([]calibration.PointResult(nil), m.status.Results...)
	return status
}

func (m *CalibrationManager) CollectCurrentPoint() error {
	m.mu.Lock()
	if m.status.State != calibration.StateRunning {
		m.mu.Unlock()
		return fmt.Errorf("calibration is not running")
	}
	if m.reader == nil {
		m.setErrorLocked("latest data reader is required")
		m.mu.Unlock()
		return fmt.Errorf("latest data reader is required")
	}
	if m.status.CurrentPoint >= len(m.config.PressurePoints) {
		m.mu.Unlock()
		return fmt.Errorf("all pressure points are already collected")
	}
	config := m.config
	pointIndex := m.status.CurrentPoint
	m.mu.Unlock()

	values := m.collectAverageValues(config.DeviceID, config.Channels, config.AverageSamples)

	if len(values) == 0 {
		return m.fail("no data available for device %s", config.DeviceID)
	}

	result := calibration.PointResult{
		PointIndex:     pointIndex,
		TargetPressure: config.PressurePoints[pointIndex],
		Timestamp:      time.Now().UnixMilli(),
		Values:         values,
	}

	if m.sink != nil {
		if err := m.sink.WriteCalibrationPoint(result); err != nil {
			return m.fail("write calibration point: %v", err)
		}
	}

	m.mu.Lock()
	m.status.Results = append(m.status.Results, result)
	m.status.CurrentPoint++
	if m.status.CurrentPoint >= len(m.config.PressurePoints) {
		m.status.State = calibration.StateIdle
		if m.store != nil {
			status := m.status
			status.Results = append([]calibration.PointResult(nil), m.status.Results...)
			if err := m.store.Save(m.config.TaskID, status); err != nil {
				m.mu.Unlock()
				return fmt.Errorf("save calibration result: %v", err)
			}
		}
	}
	m.mu.Unlock()
	return nil
}

func (m *CalibrationManager) collectAverageValues(deviceID string, channels []int, samples int) map[int]float64 {
	accum := make(map[int]float64, len(channels))
	count := make(map[int]int, len(channels))
	for _, ch := range channels {
		accum[ch] = 0
	}

	for i := 0; i < samples; i++ {
		if i > 0 {
			time.Sleep(defaultSampleInterval)
		}
		payload, ok := m.reader.GetLatestData(deviceID)
		if !ok {
			continue
		}
		vals := valuesForChannels(payload, channels)
		for ch, v := range vals {
			accum[ch] += v
			count[ch]++
		}
	}

	values := make(map[int]float64, len(channels))
	for _, ch := range channels {
		if count[ch] > 0 {
			values[ch] = accum[ch] / float64(count[ch])
		}
	}
	return values
}

func (m *CalibrationManager) Pause() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.status.State != calibration.StateRunning {
		return fmt.Errorf("calibration is not running")
	}
	m.status.State = calibration.StatePaused
	return nil
}

func (m *CalibrationManager) Resume() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.status.State != calibration.StatePaused {
		return fmt.Errorf("calibration is not paused")
	}
	m.status.State = calibration.StateRunning
	return nil
}

func (m *CalibrationManager) Stop() error {
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
	m.status.State = calibration.StateStopped
	if m.store != nil {
		status := m.status
		status.Results = append([]calibration.PointResult(nil), m.status.Results...)
		if err := m.store.Save(m.config.TaskID, status); err != nil {
			m.mu.Unlock()
			return fmt.Errorf("save calibration result: %v", err)
		}
	}
	m.mu.Unlock()
	return nil
}

func (m *CalibrationManager) GetResult(taskID string) (calibration.Status, bool) {
	if m.store == nil {
		return calibration.Status{}, false
	}
	return m.store.Get(taskID)
}

func (m *CalibrationManager) fail(format string, args ...any) error {
	message := fmt.Sprintf(format, args...)
	m.mu.Lock()
	m.setErrorLocked(message)
	m.mu.Unlock()
	return fmt.Errorf("%s", message)
}

func (m *CalibrationManager) setErrorLocked(message string) {
	m.status.State = calibration.StateError
	m.status.LastError = message
}

func valuesForChannels(payload device.DataPayload, channels []int) map[int]float64 {
	valuesByIndex := make(map[int]float64, len(payload.Channels))
	for i, value := range payload.Channels {
		chIdx := i
		if i < len(payload.ChannelIndices) {
			chIdx = payload.ChannelIndices[i]
		}
		valuesByIndex[chIdx] = value
	}

	values := make(map[int]float64, len(channels))
	for _, channel := range channels {
		value, ok := valuesByIndex[channel]
		if ok {
			values[channel] = value
		}
	}
	return values
}
