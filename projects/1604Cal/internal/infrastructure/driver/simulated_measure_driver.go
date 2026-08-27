package driver

import (
	"context"
	"fmt"
	"math"
	"math/rand"
	"sync"
	"time"
)

// SimulatedMeasureDriver 模拟计量设备驱动，用于无真实设备时的开发和测试。
type SimulatedMeasureDriver struct {
	mu           sync.Mutex
	connected    bool
	valveStatus  string
	unit         string
	channelCount int
}

// NewSimulatedMeasureDriver 创建计量模拟驱动。
func NewSimulatedMeasureDriver() *SimulatedMeasureDriver {
	return &SimulatedMeasureDriver{
		valveStatus:  "calibration",
		unit:         "MPa",
		channelCount: 16,
	}
}

func (d *SimulatedMeasureDriver) Connect(_ context.Context) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.connected = true
	return nil
}

func (d *SimulatedMeasureDriver) Disconnect(_ context.Context) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.connected = false
	return nil
}

func (d *SimulatedMeasureDriver) ReadValveStatus(_ context.Context) (string, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if !d.connected {
		return "", fmt.Errorf("simulated measure device not connected")
	}
	return d.valveStatus, nil
}

func (d *SimulatedMeasureDriver) SetValveStatus(_ context.Context, status string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if !d.connected {
		return fmt.Errorf("simulated measure device not connected")
	}
	if status != "calibration" && status != "measurement" {
		return fmt.Errorf("invalid valve status: %s", status)
	}
	d.valveStatus = status
	return nil
}

func (d *SimulatedMeasureDriver) ReadUnit(_ context.Context) (string, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.unit, nil
}

func (d *SimulatedMeasureDriver) SetUnit(_ context.Context, unit string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.unit = unit
	return nil
}

func (d *SimulatedMeasureDriver) CollectData(_ context.Context, channels []int) ([]float64, error) {
	d.mu.Lock()
	connected := d.connected
	valveStatus := d.valveStatus
	d.mu.Unlock()

	if !connected {
		return nil, fmt.Errorf("simulated measure device not connected")
	}

	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	maxCh := 16

	result := make([]float64, 0, len(channels))
	for _, ch := range channels {
		idx := ch - 1
		if idx < 0 || idx >= maxCh {
			continue
		}

		var value float64
		if valveStatus == "calibration" {
			value = 0.0
		} else {
			value = 10.0 + float64(idx)*0.5 + (rng.Float64()-0.5)*0.1 + math.Sin(float64(idx)*0.3)*0.2
		}
		result = append(result, math.Round(value*1e4)/1e4)
	}

	return result, nil
}

func (d *SimulatedMeasureDriver) CalibrateZero(_ context.Context, channels []int) ([]float64, error) {
	result := make([]float64, len(channels))
	for i := range result {
		result[i] = 0
	}
	return result, nil
}

func (d *SimulatedMeasureDriver) CalibrateFullScale(_ context.Context, channels []int, fullScaleValue float64) ([]float64, error) {
	result := make([]float64, len(channels))
	for i := range result {
		result[i] = fullScaleValue
	}
	return result, nil
}

func (d *SimulatedMeasureDriver) ReadDeviceInfo(_ context.Context) (map[string]string, error) {
	return map[string]string{
		"model":   "SIMULATED_16CH",
		"version": "1.0.0-sim",
	}, nil
}

func (d *SimulatedMeasureDriver) Reset(_ context.Context) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.valveStatus = "calibration"
	return nil
}
