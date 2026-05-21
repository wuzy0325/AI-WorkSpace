package usecase

import (
	"fmt"

	"wind-daq/services/api-go/internal/core/motion"
	"wind-daq/services/api-go/internal/ports"
)

type MotionManager struct {
	controller ports.MotionController
}

func NewMotionManager(controller ports.MotionController) *MotionManager {
	return &MotionManager{controller: controller}
}

func (m *MotionManager) Connect() error {
	if m.controller == nil {
		return fmt.Errorf("motion controller is required")
	}
	return m.controller.Connect()
}

func (m *MotionManager) Disconnect() error {
	if m.controller == nil {
		return nil
	}
	return m.controller.Disconnect()
}

func (m *MotionManager) Status() motion.ControllerStatus {
	if m.controller == nil {
		return motion.ControllerStatus{}
	}
	return m.controller.Status()
}

func (m *MotionManager) MoveTo(axis motion.AxisName, position float64) error {
	if m.controller == nil {
		return fmt.Errorf("motion controller is required")
	}
	return m.controller.MoveTo(axis, position)
}

func (m *MotionManager) MoveBy(axis motion.AxisName, delta float64) error {
	if m.controller == nil {
		return fmt.Errorf("motion controller is required")
	}
	return m.controller.MoveBy(axis, delta)
}

func (m *MotionManager) Jog(axis motion.AxisName, velocity float64) error {
	if m.controller == nil {
		return fmt.Errorf("motion controller is required")
	}
	return m.controller.Jog(axis, velocity)
}

func (m *MotionManager) Home(axis motion.AxisName) error {
	if m.controller == nil {
		return fmt.Errorf("motion controller is required")
	}
	return m.controller.Home(axis)
}

func (m *MotionManager) Stop(axis motion.AxisName) error {
	if m.controller == nil {
		return nil
	}
	return m.controller.Stop(axis)
}

func (m *MotionManager) EmergencyStop() error {
	if m.controller == nil {
		return nil
	}
	return m.controller.EmergencyStop()
}
