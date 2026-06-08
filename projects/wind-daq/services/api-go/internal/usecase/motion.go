package usecase

import (
	motionadapter "wind-daq/services/api-go/internal/adapters/motion"
	motionmanager "shared.local/motion-control/go/manager"
	sharedcore "shared.local/device-sdk/go/motion/core"
	sharedports "shared.local/device-sdk/go/motion/ports"

	"wind-daq/services/api-go/internal/ports"
)

func NewMotionManager(
	profileStore sharedports.MotionProfileStore,
	factory func(profile sharedcore.MotionControllerProfile) (sharedports.MotionController, error),
) ports.MotionManager {
	return motionadapter.WrapMotionManager(motionmanager.NewMotionManager(profileStore, factory))
}

func WrapMotionManager(raw *motionmanager.MotionManager) ports.MotionManager {
	return motionadapter.WrapMotionManager(raw)
}
