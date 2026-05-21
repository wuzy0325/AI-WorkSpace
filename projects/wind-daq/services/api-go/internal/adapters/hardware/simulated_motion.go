package hardware

import (
	"fmt"
	"sync"

	"wind-daq/services/api-go/internal/core/motion"
)

type SimulatedMotionController struct {
	mu     sync.RWMutex
	status motion.ControllerStatus
}

func NewSimulatedMotionController(id string, axes []motion.AxisName) *SimulatedMotionController {
	status := motion.ControllerStatus{ID: id, Axes: make([]motion.AxisStatus, len(axes))}
	for i, axis := range axes {
		status.Axes[i] = motion.AxisStatus{Name: axis}
	}
	return &SimulatedMotionController{status: status}
}

func (c *SimulatedMotionController) Connect() error {
	c.mu.Lock()
	c.status.Connected = true
	c.status.EmergencyStopped = false
	c.mu.Unlock()
	return nil
}

func (c *SimulatedMotionController) Disconnect() error {
	c.mu.Lock()
	c.status.Connected = false
	for i := range c.status.Axes {
		c.status.Axes[i].Moving = false
		c.status.Axes[i].Velocity = 0
	}
	c.mu.Unlock()
	return nil
}

func (c *SimulatedMotionController) Status() motion.ControllerStatus {
	c.mu.RLock()
	defer c.mu.RUnlock()
	status := c.status
	status.Axes = append([]motion.AxisStatus(nil), c.status.Axes...)
	return status
}

func (c *SimulatedMotionController) MoveTo(axis motion.AxisName, position float64) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	axisIndex, err := c.axisIndexLocked(axis)
	if err != nil {
		return err
	}
	c.status.Axes[axisIndex].Position = position
	c.status.Axes[axisIndex].Moving = false
	c.status.Axes[axisIndex].Velocity = 0
	return nil
}

func (c *SimulatedMotionController) MoveBy(axis motion.AxisName, delta float64) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	axisIndex, err := c.axisIndexLocked(axis)
	if err != nil {
		return err
	}
	c.status.Axes[axisIndex].Position += delta
	c.status.Axes[axisIndex].Moving = false
	c.status.Axes[axisIndex].Velocity = 0
	return nil
}

func (c *SimulatedMotionController) Jog(axis motion.AxisName, velocity float64) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	axisIndex, err := c.axisIndexLocked(axis)
	if err != nil {
		return err
	}
	c.status.Axes[axisIndex].Moving = true
	c.status.Axes[axisIndex].Velocity = velocity
	return nil
}

func (c *SimulatedMotionController) Home(axis motion.AxisName) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	axisIndex, err := c.axisIndexLocked(axis)
	if err != nil {
		return err
	}
	c.status.Axes[axisIndex].Position = 0
	c.status.Axes[axisIndex].Velocity = 0
	c.status.Axes[axisIndex].Moving = false
	c.status.Axes[axisIndex].Homed = true
	return nil
}

func (c *SimulatedMotionController) Stop(axis motion.AxisName) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	axisIndex, err := c.axisIndexLocked(axis)
	if err != nil {
		return err
	}
	c.status.Axes[axisIndex].Moving = false
	c.status.Axes[axisIndex].Velocity = 0
	return nil
}

func (c *SimulatedMotionController) EmergencyStop() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.status.EmergencyStopped = true
	for i := range c.status.Axes {
		c.status.Axes[i].Moving = false
		c.status.Axes[i].Velocity = 0
	}
	return nil
}

func (c *SimulatedMotionController) axisIndexLocked(axis motion.AxisName) (int, error) {
	for i := range c.status.Axes {
		if c.status.Axes[i].Name == axis {
			return i, nil
		}
	}
	return 0, fmt.Errorf("unknown motion axis: %s", axis)
}
