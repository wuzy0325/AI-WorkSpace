package domain

// ControlMode 控制模式枚举（auto=自动打压采集, manual=手动打压采集）。
type ControlMode string

const (
	ControlModeAuto   ControlMode = "auto"
	ControlModeManual ControlMode = "manual"
)

// PressureMode 压力模式枚举（single=单程, roundTrip=回程）。
type PressureMode string

const (
	PressureModeSingle    PressureMode = "single"
	PressureModeRoundTrip PressureMode = "roundTrip"
)

// WorkflowConfig 标定和计量模块的共享工作流配置。
type WorkflowConfig struct {
	Channels          []int        `json:"channels"`
	MinPressure       float64      `json:"minPressure"`
	MaxPressure       float64      `json:"maxPressure"`
	PointCount        int          `json:"pointCount"`
	Precision         int          `json:"precision"`
	AverageCount      int          `json:"averageCount"`
	PrecisionLevel    float64      `json:"precisionLevel"`
	StableWaitMs      int          `json:"stableWaitMs"`
	StabilityTimeoutMs int         `json:"stabilityTimeoutMs"`
	ControlMode       ControlMode  `json:"controlMode"`
	PressureMode      PressureMode `json:"pressureMode"`
	CustomPoints      []float64    `json:"customPoints"`
	DeviceNumber      string       `json:"deviceNumber"`
}
