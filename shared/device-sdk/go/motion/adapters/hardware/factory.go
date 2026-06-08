package hardware

import (
	"fmt"

	"shared.local/device-sdk/go/motion/core"
	"shared.local/device-sdk/go/motion/ports"
)

// MotionControllerFactory motion controller factory interface
type MotionControllerFactory interface {
	Create(profile core.MotionControllerProfile) (ports.MotionController, error)
}

// DefaultMotionControllerFactory default factory
type DefaultMotionControllerFactory struct{}

// NewDefaultMotionControllerFactory create default factory
func NewDefaultMotionControllerFactory() *DefaultMotionControllerFactory {
	return &DefaultMotionControllerFactory{}
}

// Create create motion controller by type
func (f *DefaultMotionControllerFactory) Create(profile core.MotionControllerProfile) (ports.MotionController, error) {
	switch profile.Type {
	case core.ControllerTypeSimulated:
		return NewSimulatedMotionController(profile), nil
	case core.ControllerTypeB140:
		return NewB140MotionController(profile), nil
	case core.ControllerTypeWTNMC4A:
		return NewWTNMC4AMotionController(profile), nil
	default:
		return nil, fmt.Errorf("unknown controller type: %s", profile.Type)
	}
}
