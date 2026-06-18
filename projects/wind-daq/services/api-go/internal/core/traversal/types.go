package traversal

// State 遍历测试状态
type State string

const (
	StateIdle        State = "idle"
	StatePreparing   State = "preparing"
	StateMoving      State = "moving"
	StateStabilizing State = "stabilizing"
	StateAcquiring   State = "acquiring"
	StateSaving      State = "saving"
	StateRunning     State = "running" // 兼容旧状态
	StatePaused      State = "paused"
	StateStopped     State = "stopped"
	StateError       State = "error"
	StateCompleted   State = "completed"
)

// IsTerminal 判断是否为终态
func (s State) IsTerminal() bool {
	return s == StateCompleted || s == StateStopped || s == StateError
}

// PointPhase 当前测试点的处理阶段
type PointPhase string

const (
	PhaseMoving      PointPhase = "moving"
	PhaseStabilizing PointPhase = "stabilizing"
	PhaseAcquiring   PointPhase = "acquiring"
	PhaseSaving      PointPhase = "saving"
)

// Point 遍历测试点坐标
type Point struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
	Z float64 `json:"z"`
}

// SaveOptions CSV 落盘选项（与前端 TraversalSaveOptions 对齐）
type SaveOptions struct {
	SavePointId          bool            `json:"savePointId"`
	SaveTimestamp        bool            `json:"saveTimestamp"`
	SaveRawPressure      bool            `json:"saveRawPressure"`
	SaveCalculatedResult bool            `json:"saveCalculatedResult"`
	CustomFields         map[string]bool `json:"customFields,omitempty"`
}

// Config 遍历测试配置
type Config struct {
	TaskID          string       `json:"taskId"`
	DeviceID        string       `json:"deviceId"`
	Channels        []int        `json:"channels"`
	Path            []Point      `json:"path"`
	DwellTimeMs     int          `json:"dwellTimeMs"`
	SamplesPerPoint int          `json:"samplesPerPoint"`
	SavePath        string       `json:"savePath,omitempty"`     // CSV 输出文件路径（同时用于断点文件命名）
	SaveFileName    string       `json:"saveFileName,omitempty"` // CSV 输出文件名前缀
	SaveOptions     *SaveOptions `json:"saveOptions,omitempty"`  // 落盘选项
	// ChannelLabels 通道索引→标签的显式映射（如 {0:"P1", 16:"Patm", 17:"Tatm"}）
	// 由前端 ProbeChannelConfig.role 推导而来；为空时回退到"按通道索引升序"的旧行为
	ChannelLabels map[int]string `json:"channelLabels,omitempty"`
	// InterpolationMode 多 PRB 插值模式："normal" / "linear" / "nearest"
	// 仅对 MultiPrbInterpolator 生效；为空时使用插值器自身默认（normal）
	InterpolationMode string `json:"interpolationMode,omitempty"`
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

// CalculatedResult 单点的实时插值结果摘要（用于 CSV 落盘列）
// 与 InterpolationResult 解耦：CSV 只需要标量字段
type CalculatedResult struct {
	Valid bool    `json:"valid"`
	Alpha float64 `json:"alpha"`
	Beta  float64 `json:"beta"`
	Pt    float64 `json:"pt"`
	Ps    float64 `json:"ps"`
	Mach  float64 `json:"mach"`
}

// PointResult 单点测试结果
type PointResult struct {
	PointIndex       int               `json:"pointIndex"`
	Point            Point             `json:"point"`
	Timestamp        int64             `json:"timestamp"`
	Values           map[int]float64   `json:"values"`
	SampleCount      int               `json:"sampleCount"`
	DwellTimeElapsed int               `json:"dwellTimeElapsed"`
	Calculated       *CalculatedResult `json:"calculated,omitempty"`
	// CustomValues 用户自定义字段值（key=字段名，value=字符串化值）
	CustomValues map[string]string `json:"customValues,omitempty"`
}

// ErrorCode 遍历测试错误码
type ErrorCode string

const (
	ErrMotionFailed        ErrorCode = "MOTION_FAILED"
	ErrAcquisitionFailed   ErrorCode = "ACQUISITION_FAILED"
	ErrSaveFailed          ErrorCode = "SAVE_FAILED"
	ErrInterpolationFailed ErrorCode = "INTERPOLATION_FAILED"
	ErrUnknown             ErrorCode = "UNKNOWN"
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
	TaskID          string `json:"taskId"`
	Config          []byte `json:"config,omitempty"` // 完整测试配置（用于恢复，原始 JSON 字节）
	CompletedPoints int    `json:"completedPoints"`
	TotalPoints     int    `json:"totalPoints"`
	LastPoint       *Point `json:"lastPoint,omitempty"`
	SavePath        string `json:"savePath"`
	CreatedAt       int64  `json:"createdAt"`
}

// DataValidationConfig 数据验证配置
type DataValidationConfig struct {
	Enabled        bool                      `json:"enabled"`
	PressureRange  map[string]*PressureRange `json:"pressureRange,omitempty"`
	SpikeDetection *SpikeDetectionConfig     `json:"spikeDetection,omitempty"`
	OnInvalid      string                    `json:"onInvalid"` // skip | retry | continue
	RetryCount     int                       `json:"retryCount,omitempty"`
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
	Mode        string                 `json:"mode"` // fixed | adaptive
	FixedTimeMs int                    `json:"fixedTimeMs,omitempty"`
	Adaptive    *AdaptiveStabilization `json:"adaptive,omitempty"`
}

// AdaptiveStabilization 自适应稳定配置
type AdaptiveStabilization struct {
	MaxWaitMs          int     `json:"maxWaitMs"`
	MinWaitMs          int     `json:"minWaitMs"`
	StabilityThreshold float64 `json:"stabilityThreshold"` // 百分比变化阈值
	CheckIntervalMs    int     `json:"checkIntervalMs"`
	ConsecutiveChecks  int     `json:"consecutiveChecks"` // 连续稳定检查次数
}
