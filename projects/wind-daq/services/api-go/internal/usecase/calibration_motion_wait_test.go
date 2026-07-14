package usecase

import (
	"context"
	"sync"
	"testing"

	"wind-daq/services/api-go/internal/core/calibration"
	"wind-daq/services/api-go/internal/core/motion"
)

type calibrationMotionManager struct {
	mu            sync.Mutex
	statusReads   int
	postMoveReads int
	moveIssued    bool
	movePosition  float64
}

func TestFallbackRuntimeWaitsForCommandedPosition(t *testing.T) {
	mgr := &calibrationMotionManager{}
	runtime := &fallbackRuntime{motion: mgr}
	axis := calibration.MotionAxisConfig{ControllerID: "motion-1", Axis: "X", Name: "α"}

	if err := runtime.MoveToPosition(axis, 10); err != nil {
		t.Fatalf("move to position: %v", err)
	}
	if err := runtime.WaitForMotionComplete(); err != nil {
		t.Fatalf("wait for motion complete: %v", err)
	}
	if mgr.postMoveReads < 2 {
		t.Fatalf("wait returned on the stale post-command status; post-move reads=%d", mgr.postMoveReads)
	}
}

func (m *calibrationMotionManager) LoadProfiles() ([]motion.MotionControllerProfile, error) {
	return m.GetProfiles(), nil
}
func (m *calibrationMotionManager) SaveProfiles([]motion.MotionControllerProfile) error { return nil }
func (m *calibrationMotionManager) GetProfiles() []motion.MotionControllerProfile {
	return []motion.MotionControllerProfile{{ID: "motion-1", Axes: []motion.AxisConfig{{Name: motion.AxisX, Enabled: true}}}}
}
func (m *calibrationMotionManager) UpsertProfile(motion.MotionControllerProfile) error { return nil }
func (m *calibrationMotionManager) DeleteProfile(string) error                         { return nil }
func (m *calibrationMotionManager) Connect(context.Context, string) error              { return nil }
func (m *calibrationMotionManager) Disconnect(context.Context, string) error           { return nil }
func (m *calibrationMotionManager) StatusAll(context.Context) []motion.ControllerStatus {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.statusReads++
	position := 0.0
	if m.moveIssued {
		m.postMoveReads++
	}
	if m.postMoveReads >= 2 {
		position = m.movePosition
	}
	return []motion.ControllerStatus{{
		ID:        "motion-1",
		Connected: true,
		Axes:      []motion.AxisStatus{{Name: motion.AxisX, Position: position, Moving: false}},
	}}
}
func (m *calibrationMotionManager) MoveTo(_ context.Context, _ string, _ motion.AxisName, position float64) error {
	m.mu.Lock()
	m.movePosition = position
	m.moveIssued = true
	m.mu.Unlock()
	return nil
}
func (m *calibrationMotionManager) MoveBy(context.Context, string, motion.AxisName, float64) error {
	return nil
}
func (m *calibrationMotionManager) Jog(context.Context, string, motion.AxisName, float64) error {
	return nil
}
func (m *calibrationMotionManager) Home(context.Context, string, motion.AxisName) error { return nil }
func (m *calibrationMotionManager) Stop(context.Context, string, motion.AxisName) error { return nil }
func (m *calibrationMotionManager) EmergencyStop(context.Context, string) error         { return nil }
func (m *calibrationMotionManager) ResetEmergencyStop(context.Context, string) error    { return nil }
func (m *calibrationMotionManager) DefinePosition(context.Context, string, motion.AxisName, float64) error {
	return nil
}
