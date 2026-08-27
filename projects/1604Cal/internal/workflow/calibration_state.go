package workflow

import "cal1604/internal/domain"

// CalibrationStateMachine 标定模块的状态机窄接口，隐藏计量侧不需要的状态。
type CalibrationStateMachine struct {
	inner *SessionMachine
}

// NewCalibrationStateMachine 包装共享 SessionMachine。
func NewCalibrationStateMachine(inner *SessionMachine) *CalibrationStateMachine {
	return &CalibrationStateMachine{inner: inner}
}

// State 返回当前状态。
func (m *CalibrationStateMachine) State() domain.SessionState { return m.inner.State() }

// Ready 从 idle 迁移到 ready。
func (m *CalibrationStateMachine) Ready() error { return m.inner.Transition(domain.SessionStateReady) }

// Pressurize 从 ready/point_done 迁移到 pressurizing。
func (m *CalibrationStateMachine) Pressurize() error { return m.inner.Transition(domain.SessionStatePressurizing) }

// Stabilize 从 pressurizing 迁移到 stabilizing。
func (m *CalibrationStateMachine) Stabilize() error { return m.inner.Transition(domain.SessionStateStabilizing) }

// Collect 从 stabilizing/await_manual_collect 迁移到 collecting。
func (m *CalibrationStateMachine) Collect() error { return m.inner.Transition(domain.SessionStateCollecting) }

// PointDone 从 collecting 迁移到 point_done。
func (m *CalibrationStateMachine) PointDone() error { return m.inner.Transition(domain.SessionStatePointDone) }

// Fit 从 collecting/point_done/completed 迁移到 fitting。
func (m *CalibrationStateMachine) Fit() error { return m.inner.Transition(domain.SessionStateFitting) }

// Complete 从 fitting 迁移到 completed。
func (m *CalibrationStateMachine) Complete() error { return m.inner.Transition(domain.SessionStateCompleted) }

// Pause 从当前状态迁移到 paused（允许从 pressurizing/stabilizing/collecting/point_done 暂停）。
func (m *CalibrationStateMachine) Pause() error { return m.inner.Transition(domain.SessionStatePaused) }

// Resume 从 paused 迁移到 collecting。
func (m *CalibrationStateMachine) Resume() error { return m.inner.Transition(domain.SessionStateCollecting) }

// WaitManualCollect 从 stabilizing 迁移到 await_manual_collect。
func (m *CalibrationStateMachine) WaitManualCollect() error { return m.inner.Transition(domain.SessionStateAwaitManualCollect) }

// Recover 从 error 迁移到 recovering。
func (m *CalibrationStateMachine) Recover() error { return m.inner.Transition(domain.SessionStateRecovering) }

// Error 将状态设为 error（从 ready/collecting/point_done/fitting/stabilizing/pressurizing 等）。
func (m *CalibrationStateMachine) Error() error { return m.inner.Transition(domain.SessionStateError) }

// Stop 强制停止到 stopped（try allowed transition first, fallback via ForceStop）。
func (m *CalibrationStateMachine) Stop() { m.inner.ForceStop() }

// Idle 从 stopped/completed/paused 迁移到 idle。
func (m *CalibrationStateMachine) Idle() error { return m.inner.Transition(domain.SessionStateIdle) }
