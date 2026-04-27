package hardware

import (
	"sync"

	"wind-daq/services/api-go/internal/core/motion"
	"wind-daq/services/api-go/internal/ports"
)

// SimulatedMotionController is a simulated motion controller for testing
type SimulatedMotionController struct {
	mu      sync.RWMutex
	profile motion.MotionControllerProfile
	axes    map[motion.AxisName]simulatedAxisState
}

type simulatedAxisState struct {
	position float64
	moving   bool
	homed    bool
}

// NewSimulatedMotionController creates a simulated motion controller
func NewSimulatedMotionController(profile motion.MotionControllerProfile) *SimulatedMotionController {
	mc := &SimulatedMotionController{
		profile: profile,
		axes:    make(map[motion.AxisName]simulatedAxisState),
	}
	for _, ax := range profile.Axes {
		mc.axes[ax.Name] = simulatedAxisState{}
	}
	return mc
}

func (s *SimulatedMotionController) Connect() error                          { return nil }
func (s *SimulatedMotionController) Disconnect() error                       { return nil }
func (s *SimulatedMotionController) IsConnected() bool                       { return true }
func (s *SimulatedMotionController) Profile() motion.MotionControllerProfile { return s.profile }
func (s *SimulatedMotionController) EmergencyStop() error                    { return nil }

func (s *SimulatedMotionController) UpdateProfile(profile motion.MotionControllerProfile) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.profile = profile
}

func (s *SimulatedMotionController) MoveTo(axis motion.AxisName, position float64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	state := s.axes[axis]
	state.position = position
	state.moving = false
	s.axes[axis] = state
	return nil
}

func (s *SimulatedMotionController) MoveBy(axis motion.AxisName, delta float64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	state := s.axes[axis]
	state.position += delta
	s.axes[axis] = state
	return nil
}

func (s *SimulatedMotionController) Jog(axis motion.AxisName, direction string, speed float64) error {
	return nil
}

func (s *SimulatedMotionController) Home(axis motion.AxisName) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	state := s.axes[axis]
	state.position = 0
	state.homed = true
	s.axes[axis] = state
	return nil
}

func (s *SimulatedMotionController) Stop(axis motion.AxisName) error {
	return nil
}

func (s *SimulatedMotionController) DefinePosition(axis motion.AxisName, position float64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	state := s.axes[axis]
	state.position = position
	s.axes[axis] = state
	return nil
}

func (s *SimulatedMotionController) GetAxisStatus(axis motion.AxisName) (motion.AxisStatus, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	state := s.axes[axis]
	return motion.AxisStatus{
		Name:     axis,
		Position: state.position,
		Moving:   state.moving,
		Homed:    state.homed,
	}, nil
}

func (s *SimulatedMotionController) GetAllAxisStatus() ([]motion.AxisStatus, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var result []motion.AxisStatus
	for _, ax := range s.profile.Axes {
		state := s.axes[ax.Name]
		result = append(result, motion.AxisStatus{
			Name:     ax.Name,
			Position: state.position,
			Moving:   state.moving,
			Homed:    state.homed,
		})
	}
	return result, nil
}

// Verify interface compliance
var _ ports.MotionController = (*SimulatedMotionController)(nil)
