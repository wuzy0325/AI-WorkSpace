package workflow

import (
	"fmt"
	"sync"
	"time"

	"cal1604/internal/domain"
	apperrors "cal1604/internal/errors"
)

// WorkflowOwner 表示业务流程所有者。
type WorkflowOwner string

const (
	// OwnerCalibration 标定工作流。
	OwnerCalibration WorkflowOwner = "calibration"
	// OwnerMeasurement 计量工作流。
	OwnerMeasurement WorkflowOwner = "measurement"
)

// WorkflowCoordinator 管理单活工作流的生命周期、所有者判定和状态迁移。
// 系统同一时刻只允许一个活跃工作流拥有设备操作上下文；新工作流
// 试图绑定时由 coordinator 统一做冲突判定。
type WorkflowCoordinator struct {
	mu      sync.Mutex
	machine *SessionMachine
	owner   WorkflowOwner
	ctxID   string
}

// NewWorkflowCoordinator 创建 WorkflowCoordinator，初始状态 idle。
func NewWorkflowCoordinator() *WorkflowCoordinator {
	return &WorkflowCoordinator{
		machine: NewSessionMachine(),
	}
}

// State 返回当前状态机状态。
func (c *WorkflowCoordinator) State() domain.SessionState {
	return c.machine.State()
}

// Owner 返回当前工作流所有者（若无活跃工作流返回空字符串）。
func (c *WorkflowCoordinator) Owner() WorkflowOwner {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.owner
}

// CtxID 返回当前工作流上下文 ID。
func (c *WorkflowCoordinator) CtxID() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.ctxID
}

// Machine 返回底层状态机，供应用服务执行细粒度状态迁移。
func (c *WorkflowCoordinator) Machine() *SessionMachine {
	return c.machine
}

// HasActiveWorkflow 返回当前是否有活跃工作流。
// 活跃判定包括 ready, paused, pressurizing, stabilizing, collecting,
// await_alarm_resolution, await_manual_collect, point_done, fitting。
func (c *WorkflowCoordinator) HasActiveWorkflow() bool {
	return isActiveState(c.machine.State())
}

// IsManualInterventionRequired 返回当前是否处于需要人工处理状态。
func (c *WorkflowCoordinator) IsManualInterventionRequired() bool {
	return c.machine.State() == domain.SessionStateRequiresManualIntervention
}

// Begin 开始一个工作流：校验冲突、设置所有者、迁移到 ready。
// 同一所有者重复调用为幂等。
func (c *WorkflowCoordinator) Begin(owner WorkflowOwner) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if isActiveState(c.machine.State()) {
		if c.owner == owner {
			return nil
		}
		return fmt.Errorf("%w: %s workflow is running", apperrors.ErrWorkflowConflict, c.owner)
	}

	if c.machine.State() == domain.SessionStateRequiresManualIntervention {
		return apperrors.ErrManualInterventionRequired
	}

	c.owner = owner
	c.ctxID = fmt.Sprintf("%d", time.Now().UnixNano())
	return c.machine.Transition(domain.SessionStateReady)
}

// End 结束当前工作流：停止状态机、清除所有者和上下文。
// 不处理设备 I/O 层面释放（由调用方执行安全释放），不自动断开设备连接。
func (c *WorkflowCoordinator) End() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.machine.ForceStop()
	c.owner = ""
	c.ctxID = ""
}

// Fail 将工作流转入 error 状态，清除所有者和上下文。
// 调用方应先执行安全释放再调用此方法。
func (c *WorkflowCoordinator) Fail() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if err := c.machine.Transition(domain.SessionStateError); err != nil {
		c.machine.ForceStop()
	}
	c.owner = ""
	c.ctxID = ""
	return nil
}

// NotifySafetyReleaseFailed 将工作流转入需要人工处理状态。
// 调用方在安全释放失败后调用此方法，系统不再允许新工作流。
func (c *WorkflowCoordinator) NotifySafetyReleaseFailed() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.machine.Transition(domain.SessionStateRequiresManualIntervention)
}

// ConfirmManualIntervention 确认人工处理完成，回到 idle 并清除上下文。
// 调用方应在此前后重新读取设备状态。
func (c *WorkflowCoordinator) ConfirmManualIntervention() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.machine.State() != domain.SessionStateRequiresManualIntervention {
		return fmt.Errorf("not in manual intervention state, current: %s", c.machine.State())
	}

	c.owner = ""
	c.ctxID = ""
	return c.machine.Transition(domain.SessionStateIdle)
}

// isActiveState 判断给定状态是否为活跃工作流状态。
func isActiveState(s domain.SessionState) bool {
	switch s {
	case domain.SessionStateReady, domain.SessionStatePaused,
		domain.SessionStatePressurizing, domain.SessionStateStabilizing,
		domain.SessionStateCollecting, domain.SessionStateAwaitAlarmResolution,
		domain.SessionStateAwaitManualCollect, domain.SessionStatePointDone,
		domain.SessionStateFitting:
		return true
	}
	return false
}
