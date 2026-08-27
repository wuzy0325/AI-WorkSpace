package workflow

import "cal1604/internal/domain"

// MeasurementStateMachine 计量模块的状态机窄接口，隐藏标定侧不需要的状态（point_done, fitting 等）。
type MeasurementStateMachine struct {
	inner *SessionMachine
}

func NewMeasurementStateMachine(inner *SessionMachine) *MeasurementStateMachine {
	return &MeasurementStateMachine{inner: inner}
}

func (m *MeasurementStateMachine) State() domain.SessionState { return m.inner.State() }

// Ready 从 idle 迁移到 ready（手动模式工作流入口）。
func (m *MeasurementStateMachine) Ready() error { return m.inner.Transition(domain.SessionStateReady) }

// Collect 从 idle/completed/error/ready/paused 迁移到 collecting。
func (m *MeasurementStateMachine) Collect() error { return m.inner.Transition(domain.SessionStateCollecting) }

// Pressurize 从 ready 迁移到 pressurizing。
func (m *MeasurementStateMachine) Pressurize() error { return m.inner.Transition(domain.SessionStatePressurizing) }

// Stabilize 从 pressurizing 迁移到 stabilizing。
func (m *MeasurementStateMachine) Stabilize() error { return m.inner.Transition(domain.SessionStateStabilizing) }

// Stop 停止到 completed 再到 idle，或从当前状态 ForceStop。
func (m *MeasurementStateMachine) Stop() { m.inner.ForceStop() }

// Pause 从 collecting/ready/pressurizing/stabilizing 迁移到 paused。
func (m *MeasurementStateMachine) Pause() error { return m.inner.Transition(domain.SessionStatePaused) }

// Idle 从 stopped/completed/paused/error 迁移到 idle。
func (m *MeasurementStateMachine) Idle() error { return m.inner.Transition(domain.SessionStateIdle) }

// Complete 从 collecting 迁移到 completed。
func (m *MeasurementStateMachine) Complete() error { return m.inner.Transition(domain.SessionStateCompleted) }

// AwaitAlarm 从 collecting 迁移到 await_alarm_resolution。
func (m *MeasurementStateMachine) AwaitAlarm() error { return m.inner.Transition(domain.SessionStateAwaitAlarmResolution) }

// Error 从当前状态迁移到 error。
func (m *MeasurementStateMachine) Error() error { return m.inner.Transition(domain.SessionStateError) }
