package usecase

import motionmanager "shared.local/motion-control/go/manager"

type MotionControllerFactory = motionmanager.MotionControllerFactory
type MotionManager = motionmanager.MotionManager

var NewMotionManager = motionmanager.NewMotionManager
