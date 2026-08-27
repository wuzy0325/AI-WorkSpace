package apperrors

import "errors"

var (
	// ErrUnitMismatch 表示设备单位不一致，不能进入采集流程。
	ErrUnitMismatch = errors.New("unit mismatch")
	// ErrInvalidArgument 表示请求参数非法。
	ErrInvalidArgument = errors.New("invalid argument")
	// ErrNotFound 表示请求目标不存在。
	ErrNotFound = errors.New("not found")
	// ErrInvalidStateTransition 表示状态机迁移非法。
	ErrInvalidStateTransition = errors.New("invalid state transition")
	// ErrPrerequisiteNotMet 表示标定启动前置条件不满足（阀门状态、设备连接等）。
	ErrPrerequisiteNotMet = errors.New("prerequisite not met")
	// ErrNoData 表示请求的数据不存在。
	ErrNoData = errors.New("no data")
	// ErrNoActiveSession 表示没有活跃的校准会话，无法导出报告。
	ErrNoActiveSession = errors.New("no active session")
	// ErrReportExport 表示报告导出过程中出错（模板加载、数据填充、保存等）。
	ErrReportExport = errors.New("report export failed")
	// ErrWorkflowConflict 表示已有活跃工作流，拒绝新绑定或操作请求。
	ErrWorkflowConflict = errors.New("active workflow conflict")
	// ErrManualInterventionRequired 表示设备处于需要人工处理状态，拒绝所有非确认/读取操作。
	ErrManualInterventionRequired = errors.New("manual intervention required")
	// ErrStaleWorkflowContext 表示操作携带的上下文已失效（旧流程或延迟请求）。
	ErrStaleWorkflowContext = errors.New("stale workflow context")
	// ErrWorkflowOwnerMismatch 表示操作者与当前活跃工作流所有者不一致。
	ErrWorkflowOwnerMismatch = errors.New("workflow owner mismatch")
)
