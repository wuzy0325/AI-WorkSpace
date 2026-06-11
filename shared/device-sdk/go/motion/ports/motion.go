package ports

import (
	"context"

	"shared.local/device-sdk/go/motion/core"
)

// MotionController motion controller interface
type MotionController interface {
	Connect(ctx context.Context) error
	Disconnect(ctx context.Context) error
	Status(ctx context.Context) (core.ControllerStatus, error)
	MoveTo(ctx context.Context, axis core.AxisName, position float64) error
	MoveBy(ctx context.Context, axis core.AxisName, delta float64) error
	Jog(ctx context.Context, axis core.AxisName, velocity float64) error
	Home(ctx context.Context, axis core.AxisName) error
	Stop(ctx context.Context, axis core.AxisName) error
	EmergencyStop(ctx context.Context) error
	ResetEmergencyStop(ctx context.Context) error
	DefinePosition(ctx context.Context, axis core.AxisName, position float64) error
	GetProfile() core.MotionControllerProfile
	ApplyConfig(ctx context.Context, profile core.MotionControllerProfile) error
}

// MotionProfileStore motion controller profile store interface
type MotionProfileStore interface {
	LoadProfiles() ([]core.MotionControllerProfile, error)
	SaveProfiles(profiles []core.MotionControllerProfile) error
}
