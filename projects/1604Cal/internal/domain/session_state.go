package domain

// SessionState 表示采集会话状态机当前状态。
type SessionState string

const (
	SessionStateIdle                 SessionState = "idle"
	SessionStateReady                SessionState = "ready"
	SessionStatePressurizing         SessionState = "pressurizing"
	SessionStateStabilizing          SessionState = "stabilizing"
	SessionStateCollecting           SessionState = "collecting"
	SessionStatePointDone            SessionState = "point_done"
	SessionStateFitting              SessionState = "fitting"
	SessionStateCompleted            SessionState = "completed"
	SessionStatePaused               SessionState = "paused"
	SessionStateStopped              SessionState = "stopped"
	SessionStateAwaitManualCollect   SessionState = "await_manual_collect"
	SessionStateAwaitAlarmResolution SessionState = "await_alarm_resolution"
	SessionStateRecovering           SessionState = "recovering"
	SessionStateError                         SessionState = "error"
	SessionStateRequiresManualIntervention    SessionState = "requires_manual_intervention"
)
