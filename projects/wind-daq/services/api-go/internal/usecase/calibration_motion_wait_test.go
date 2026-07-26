package usecase

import (
	"context"
	"sync"
	"testing"

	"wind-daq/services/api-go/internal/core/calibration"
	"wind-daq/services/api-go/internal/core/motion"
	"wind-daq/services/api-go/internal/core/traversal"
)

// calibrationMotionManager 模拟运动控制器，复现"MoveTo 后轴 Moving=true，
// 经若干轮询后到达 target 并 Moving=false"的真实运动过程。
//
// 行为时间线：
//  1. MoveTo 调用前：position=0, Moving=false（初始态）
//  2. MoveTo 调用后第 1~2 次轮询：position=0, Moving=true（运动中）
//  3. 第 3 次轮询起：position=movePosition, Moving=false（到位）
//
// 这样 EvaluateMotionSafety 在运动中会返回 MotionSafetyOK（不判偏差），
// 到位时返回 MotionSafetyArrived，避免被误判为 CriticalDeviation。
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
	completed, reason, failure := runtime.WaitForMotionComplete()
	if !completed || reason != traversal.MotionInterruptNone || failure != nil {
		t.Fatalf("wait for motion complete: completed=%v reason=%v failure=%v", completed, reason, failure)
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
	moving := false
	if m.moveIssued {
		m.postMoveReads++
	}
	// MoveTo 后前 2 次轮询：运动中（position 未更新但 Moving=true）
	// 第 3 次起：到位（position=movePosition, Moving=false）
	if m.moveIssued && m.postMoveReads >= 3 {
		position = m.movePosition
		moving = false
	} else if m.moveIssued {
		moving = true
	}
	return []motion.ControllerStatus{{
		ID:        "motion-1",
		Connected: true,
		Axes:      []motion.AxisStatus{{Name: motion.AxisX, Position: position, Moving: moving}},
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
