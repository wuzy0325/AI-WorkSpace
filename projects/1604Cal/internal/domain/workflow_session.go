package domain

import "time"

// WorkflowSession 标定和计量模块的共享工作流会话。
type WorkflowSession struct {
	ID               string            `json:"id"`
	StartTime        time.Time         `json:"startTime"`
	EndTime          *time.Time        `json:"endTime,omitempty"`
	Config           WorkflowConfig    `json:"config"`
	Points           []PressurePoint   `json:"points"`
	MeasureDeviceID  string            `json:"measureDeviceId"`
	MeasureDeviceIDs []string          `json:"measureDeviceIds,omitempty"`
	PressureDeviceID string            `json:"pressureDeviceId"`
	Status           SessionState      `json:"status"`
}
