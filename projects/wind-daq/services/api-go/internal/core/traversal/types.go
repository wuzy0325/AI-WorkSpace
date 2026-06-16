package traversal

// State 遍历测试状态
type State string

const (
	StateIdle       State = "idle"
	StatePreparing  State = "preparing"
	StateMoving     State = "moving"
	StateStabilizing State = "stabilizing"
	StateAcquiring  State = "acquiring"
	StateSaving     State = "saving"
	StateRunning    State = "running" // 兼容旧状态
	StatePaused     State = "paused"
	StateStopped    State = "stopped"
	StateError      State = "error"
	StateCompleted  State = "completed"
)

// IsTerminal 判断是否为终态
func (s State) IsTerminal() bool {
	return s == StateCompleted || s == StateStopped || s == StateError
}

// PointPhase 当前测试点的处理阶段
type PointPhase string

const (
	PhaseMoving     PointPhase = "moving"
	PhaseStabilizing PointPhase = "stabilizing"
	PhaseAcquiring  PointPhase = "acquiring"
	PhaseSaving     PointPhase = "saving"
)

// Point 遍历测试点坐标
type Point struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
	Z float64 `json:"z"`
}

// Config 遍历测试配置
type Config struct {
	TaskID        string  `json:"taskId"`
	DeviceID      string  `json:"deviceId"`
	Channels      []int   `json:"channels"`
	Path          []Point `json:"path"`
	DwellTimeMs   int     `json:"dwellTimeMs"`
	SamplesPerPoint int   `json:"samplesPerPoint"`
}

// GridConfig 网格配置（旧接口兼容）
type GridConfig struct {
	XStart float64 `json:"xStart"`
	XEnd   float64 `json:"xEnd"`
	XStep  float64 `json:"xStep"`
	YStart float64 `json:"yStart"`
	YEnd   float64 `json:"yEnd"`
	YStep  float64 `json:"yStep"`
	ZStart float64 `json:"zStart"`
}

// PointResult 单点测试结果
type PointResult struct {
	PointIndex      int             `json:"pointIndex"`
	Point           Point           `json:"point"`
	Timestamp       int64           `json:"timestamp"`
	Values          map[int]float64 `json:"values"`
	SampleCount     int             `json:"sampleCount"`
	DwellTimeElapsed int            `json:"dwellTimeElapsed"`
}

// ErrorCode 遍历测试错误码
type ErrorCode string

const (
	ErrMotionFailed       ErrorCode = "MOTION_FAILED"
	ErrAcquisitionFailed  ErrorCode = "ACQUISITION_FAILED"
	ErrSaveFailed         ErrorCode = "SAVE_FAILED"
	ErrInterpolationFailed ErrorCode = "INTERPOLATION_FAILED"
	ErrUnknown            ErrorCode = "UNKNOWN"
)

// Status 遍历测试状态
type Status struct {
	TaskID                  string        `json:"taskId"`
	State                   State         `json:"state"`
	CurrentPoint            int           `json:"currentPoint"`
	CurrentPointCoordinates *Point        `json:"currentPointCoordinates,omitempty"`
	CurrentPointPhase       PointPhase    `json:"currentPointPhase,omitempty"`
	TotalPoints             int           `json:"totalPoints"`
	Results                 []PointResult `json:"results"`
	LastError               string        `json:"lastError,omitempty"`
	LastErrorCode           ErrorCode     `json:"lastErrorCode,omitempty"`
	StartedAt               int64         `json:"startedAt,omitempty"`
	ValidationWarnings      []string      `json:"validationWarnings,omitempty"`
}

// Checkpoint 断点恢复信息
type Checkpoint struct {
	TaskID          string  `json:"taskId"`
	CompletedPoints int     `json:"completedPoints"`
	TotalPoints     int     `json:"totalPoints"`
	LastPoint       *Point  `json:"lastPoint,omitempty"`
	SavePath        string  `json:"savePath"`
	CreatedAt       int64   `json:"createdAt"`
}

// DataValidationConfig 数据验证配置
type DataValidationConfig struct {
	Enabled       bool                       `json:"enabled"`
	PressureRange map[string]*PressureRange  `json:"pressureRange,omitempty"`
	SpikeDetection *SpikeDetectionConfig     `json:"spikeDetection,omitempty"`
	OnInvalid     string                     `json:"onInvalid"` // skip | retry | continue
	RetryCount    int                        `json:"retryCount,omitempty"`
}

// PressureRange 压力范围限制
type PressureRange struct {
	Min float64 `json:"min"`
	Max float64 `json:"max"`
}

// SpikeDetectionConfig 尖峰检测配置
type SpikeDetectionConfig struct {
	Enabled   bool    `json:"enabled"`
	Threshold float64 `json:"threshold"` // 百分比阈值
}

// StabilizationConfig 稳定等待配置
type StabilizationConfig struct {
	Mode     string              `json:"mode"` // fixed | adaptive
	FixedTimeMs int              `json:"fixedTimeMs,omitempty"`
	Adaptive *AdaptiveStabilization `json:"adaptive,omitempty"`
}

// AdaptiveStabilization 自适应稳定配置
type AdaptiveStabilization struct {
	MaxWaitMs          int     `json:"maxWaitMs"`
	MinWaitMs          int     `json:"minWaitMs"`
	StabilityThreshold float64 `json:"stabilityThreshold"` // 百分比变化阈值
	CheckIntervalMs    int     `json:"checkIntervalMs"`
	ConsecutiveChecks  int     `json:"consecutiveChecks"` // 连续稳定检查次数
}
