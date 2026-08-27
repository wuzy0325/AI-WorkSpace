package workflow

import (
	"fmt"
	"sync"

	"cal1604/internal/domain"
)

// SessionMachine 管理会话状态迁移，并对非法迁移进行拦截。
type SessionMachine struct {
	mu          sync.RWMutex
	state       domain.SessionState
	transitions map[domain.SessionState]map[domain.SessionState]struct{}
}

// NewSessionMachine 创建默认状态机，初始状态为 idle。
func NewSessionMachine() *SessionMachine {
	return &SessionMachine{
		state: domain.SessionStateIdle,
		transitions: map[domain.SessionState]map[domain.SessionState]struct{}{
			domain.SessionStateIdle: {
				domain.SessionStateReady:        {},
				domain.SessionStatePressurizing: {},
				domain.SessionStateCollecting:   {},
			},
			domain.SessionStateReady: {
				domain.SessionStatePressurizing: {},
				domain.SessionStateCollecting:   {},
				domain.SessionStatePaused:       {},
				domain.SessionStateIdle:         {},
				domain.SessionStateError:        {},
				domain.SessionStateStopped:      {},
			},
			domain.SessionStatePressurizing: {
				domain.SessionStateStabilizing: {},
				domain.SessionStatePaused:      {},
				domain.SessionStateError:       {},
				domain.SessionStateStopped:     {},
			},
			domain.SessionStateStabilizing: {
				domain.SessionStateCollecting:         {},
				domain.SessionStateAwaitManualCollect: {},
				domain.SessionStatePaused:             {},
				domain.SessionStateError:              {},
				domain.SessionStateStopped:            {},
			},
			domain.SessionStateAwaitManualCollect: {
				domain.SessionStateCollecting: {},
				domain.SessionStatePaused:     {},
				domain.SessionStateStopped:    {},
			},
			domain.SessionStateCollecting: {
				domain.SessionStateReady:                {},
				domain.SessionStateCompleted:            {},
				domain.SessionStatePointDone:            {},
				domain.SessionStateAwaitAlarmResolution: {},
				domain.SessionStatePaused:               {},
				domain.SessionStateError:                {},
				domain.SessionStateStopped:              {},
			},
			domain.SessionStateAwaitAlarmResolution: {
				domain.SessionStateCollecting: {},
				domain.SessionStatePointDone:  {},
				domain.SessionStatePaused:     {},
				domain.SessionStateStopped:    {},
			},
			domain.SessionStatePointDone: {
				domain.SessionStatePressurizing:         {},
				domain.SessionStateAwaitAlarmResolution: {},
				domain.SessionStateFitting:              {},
				domain.SessionStatePaused:               {},
				domain.SessionStateStopped:              {},
			},
			domain.SessionStateFitting: {
				domain.SessionStateCompleted: {},
				domain.SessionStateError:     {},
				domain.SessionStateStopped:   {},
			},
			domain.SessionStateCompleted: {
				domain.SessionStateReady:      {},
				domain.SessionStateIdle:       {},
				domain.SessionStateCollecting: {},
				domain.SessionStateStopped:    {},
			},
			domain.SessionStatePaused: {
				domain.SessionStatePressurizing: {},
				domain.SessionStateCollecting:   {},
				domain.SessionStateIdle:         {},
				domain.SessionStateStopped:      {},
			},
			domain.SessionStateRecovering: {
				domain.SessionStatePressurizing: {},
				domain.SessionStateError:        {},
				domain.SessionStateStopped:      {},
			},
			domain.SessionStateError: {
				domain.SessionStateIdle:                     {},
				domain.SessionStateRecovering:               {},
				domain.SessionStateStopped:                  {},
				domain.SessionStateReady:                    {},
				domain.SessionStateRequiresManualIntervention: {},
			},
			domain.SessionStateRequiresManualIntervention: {
				domain.SessionStateIdle:   {},
				domain.SessionStateError:  {},
				domain.SessionStateStopped: {},
			},
			domain.SessionStateStopped: {
				domain.SessionStateReady: {},
			},
		},
	}
}

// ForceStop 强制迁移到 stopped 状态。
// 若直接迁移失败，尝试经由中间状态逐步抵达。
// 覆盖的所有状态路径：
//   direct:  任意允许直达 stopped 的状态
//   via paused:    pressurizing, stabilizing, collecting, point_done, await_manual_collect, await_alarm_resolution, ready
//   via error:     fitting（fitting 不允许直达 stopped 但可以经 error 中转）
//   via idle:      completed（completed 不允许直达 stopped 但可以经 idle 中转）
func (m *SessionMachine) ForceStop() {
	if m.Transition(domain.SessionStateStopped) == nil {
		return
	}
	// 尝试经由 paused
	if m.Transition(domain.SessionStatePaused) == nil {
		_ = m.Transition(domain.SessionStateStopped)
		return
	}
	// 尝试经由 idle
	if m.Transition(domain.SessionStateIdle) == nil {
		_ = m.Transition(domain.SessionStateStopped)
		return
	}
	// 尝试经由 error
	if m.Transition(domain.SessionStateError) == nil {
		_ = m.Transition(domain.SessionStateStopped)
	}
}

// State 返回当前状态。
func (m *SessionMachine) State() domain.SessionState {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return m.state
}

// Transition 迁移到新状态，若迁移非法则返回错误。
func (m *SessionMachine) Transition(next domain.SessionState) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	allowed, ok := m.transitions[m.state]
	if !ok {
		return fmt.Errorf("state %s has no transitions", m.state)
	}

	if _, exists := allowed[next]; !exists {
		return fmt.Errorf("invalid transition: %s -> %s", m.state, next)
	}

	m.state = next
	return nil
}
