package ports

import "wind-daq/services/api-go/internal/core/motion"

type MotionFactory interface {
	Create(profile motion.MotionControllerProfile) MotionController
}
