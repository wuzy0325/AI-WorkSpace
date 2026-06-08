package ports

import (
	"context"

	"wind-daq/services/api-go/internal/core/motion"
)

type MotionManager interface {
	LoadProfiles() ([]motion.MotionControllerProfile, error)
	SaveProfiles(profiles []motion.MotionControllerProfile) error
	GetProfiles() []motion.MotionControllerProfile
	UpsertProfile(profile motion.MotionControllerProfile) error
	DeleteProfile(id string) error

	Connect(ctx context.Context, id string) error
	Disconnect(ctx context.Context, id string) error
	StatusAll(ctx context.Context) []motion.ControllerStatus
	MoveTo(ctx context.Context, id string, axis motion.AxisName, position float64) error
	MoveBy(ctx context.Context, id string, axis motion.AxisName, delta float64) error
	Jog(ctx context.Context, id string, axis motion.AxisName, velocity float64) error
	Home(ctx context.Context, id string, axis motion.AxisName) error
	Stop(ctx context.Context, id string, axis motion.AxisName) error
	EmergencyStop(ctx context.Context, id string) error
	ResetEmergencyStop(ctx context.Context, id string) error
	DefinePosition(ctx context.Context, id string, axis motion.AxisName, position float64) error
}
