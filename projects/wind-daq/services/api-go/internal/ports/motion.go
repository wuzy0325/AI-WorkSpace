package ports

import "wind-daq/services/api-go/internal/core/motion"

type MotionController interface {
	Connect() error
	Disconnect() error
	Status() motion.ControllerStatus
	MoveTo(axis motion.AxisName, position float64) error
	MoveBy(axis motion.AxisName, delta float64) error
	Jog(axis motion.AxisName, velocity float64) error
	Home(axis motion.AxisName) error
	Stop(axis motion.AxisName) error
	EmergencyStop() error
	ResetEmergencyStop() error
	DefinePosition(axis motion.AxisName, position float64) error
	GetProfile() motion.MotionControllerProfile
}

// MotionProfileStore 运动控制器配置存储接口
type MotionProfileStore interface {
	LoadProfiles() ([]motion.MotionControllerProfile, error)
	SaveProfiles(profiles []motion.MotionControllerProfile) error
}
