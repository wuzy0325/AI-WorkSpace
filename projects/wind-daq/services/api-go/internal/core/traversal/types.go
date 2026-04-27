package traversal

// TraversalStatus 位移测试状态
type TraversalStatus string

const (
	TraversalIdle    TraversalStatus = "idle"
	TraversalRunning TraversalStatus = "running"
	TraversalPaused  TraversalStatus = "paused"
	TraversalDone    TraversalStatus = "completed"
	TraversalError   TraversalStatus = "error"
)

// TraversalConfig 位移测试配置
type TraversalConfig struct {
	Name         string   `json:"name"`
	ControllerID string   `json:"controllerId"`
	Axis         string   `json:"axis"`
	StartPos     float64  `json:"startPos"`
	EndPos       float64  `json:"endPos"`
	StepSize     float64  `json:"stepSize"`
	DwellTimeMs  int      `json:"dwellTimeMs"`
	SamplesPerPt int      `json:"samplesPerPoint"`
	DeviceIDs    []string `json:"deviceIds"`
}

// TraversalProgress 位移测试进度
type TraversalProgress struct {
	CurrentPos  float64         `json:"currentPos"`
	CurrentStep int             `json:"currentStep"`
	TotalSteps  int             `json:"totalSteps"`
	Status      TraversalStatus `json:"status"`
}
