package driver

import (
	"context"
	"fmt"
	"math"
	"sync"
	"time"
)

// SimulatedPressureDriver 模拟打压设备驱动，用于无真实设备时的开发和测试。
type SimulatedPressureDriver struct {
	mu              sync.Mutex
	connected       bool
	targetPressure  float64
	currentPressure float64
	unit            string
	stableStart     time.Time
	stableDuration  time.Duration
	stopSim         chan struct{}
}

// NewSimulatedPressureDriver 创建打压模拟驱动。
func NewSimulatedPressureDriver() *SimulatedPressureDriver {
	return &SimulatedPressureDriver{
		unit:           "MPa",
		stableDuration: 2000 * time.Millisecond,
	}
}

func (d *SimulatedPressureDriver) Connect(_ context.Context) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.connected = true
	d.currentPressure = 0
	return nil
}

func (d *SimulatedPressureDriver) Disconnect(_ context.Context) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.connected = false
	if d.stopSim != nil {
		close(d.stopSim)
		d.stopSim = nil
	}
	return nil
}

func (d *SimulatedPressureDriver) SetTargetPressure(_ context.Context, target float64) error {
	d.mu.Lock()
	d.targetPressure = target
	d.stableStart = time.Now()
	if d.stopSim != nil {
		close(d.stopSim)
	}
	d.stopSim = make(chan struct{})
	startPres := d.currentPressure
	stopCh := d.stopSim
	d.mu.Unlock()

	go d.simulateStabilization(target, startPres, stopCh)
	return nil
}

func (d *SimulatedPressureDriver) Stop(_ context.Context) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.targetPressure = 0
	if d.stopSim != nil {
		close(d.stopSim)
		d.stopSim = nil
	}
	return nil
}

func (d *SimulatedPressureDriver) Exhaust(_ context.Context) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.targetPressure = 0
	d.currentPressure = 0
	if d.stopSim != nil {
		close(d.stopSim)
		d.stopSim = nil
	}
	return nil
}

func (d *SimulatedPressureDriver) ReadCurrentPressure(_ context.Context) (float64, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if !d.connected {
		return 0, fmt.Errorf("simulated pressure device not connected")
	}
	return d.currentPressure, nil
}

func (d *SimulatedPressureDriver) ReadUnit(_ context.Context) (string, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.unit, nil
}

func (d *SimulatedPressureDriver) SetUnit(_ context.Context, unit string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.unit = NormalizePressureUnit(unit)
	return nil
}

func (d *SimulatedPressureDriver) ReadStability(_ context.Context) (bool, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.targetPressure == 0 && d.currentPressure < 0.001 {
		return true, nil
	}

	elapsed := time.Since(d.stableStart)
	if elapsed < d.stableDuration {
		return false, nil
	}

	deviation := math.Abs(d.currentPressure - d.targetPressure)
	return deviation <= 0.001, nil
}

func (d *SimulatedPressureDriver) simulateStabilization(target float64, startPres float64, stopCh chan struct{}) {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	startTime := time.Now()

	for {
		select {
		case <-stopCh:
			return
		case <-ticker.C:
		}

		elapsed := time.Since(startTime)
		progress := float64(elapsed) / float64(d.stableDuration)
		if progress > 1 {
			progress = 1
		}

		eased := easeInOutCubic(progress)
		diff := target - startPres

		d.mu.Lock()
		d.currentPressure = startPres + diff*eased
		done := progress >= 1
		d.mu.Unlock()

		if done {
			return
		}
	}
}

// easeInOutCubic S 曲线缓动函数。
func easeInOutCubic(t float64) float64 {
	if t < 0.5 {
		return 4 * t * t * t
	}
	return 1 - math.Pow(-2*t+2, 3)/2
}
