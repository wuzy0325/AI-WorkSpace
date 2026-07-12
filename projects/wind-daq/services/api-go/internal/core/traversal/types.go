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

// Point 遍历测试点坐标（4 轴：X/Y/Z/U，对应位移机构全轴能力）
// U 字段零值为 0，旧配置文件无 u 字段时 Go JSON 反序列化自动填 0，向后兼容
type Point struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
	Z float64 `json:"z"`
	U float64 `json:"u"`
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
	// PProbePressureType 五孔探针 P1-P5 通道的压力传感器类型："gauge"（表压，默认）/ "absolute"（绝压）。
	// 约束：P1-P5 必须为同一类型（5 孔传感器物理上同型号），由全局开关统一表达。
	// Patm 通道始终视为绝压，不消费此字段。
	// 空串在 ParseAndStartTraversal 中兜底为 "gauge"，保证旧配置反序列化兼容。
	PProbePressureType string `json:"pProbePressureType,omitempty"`

	// MotionAxes 参与遍历运动的轴绑定列表，来自前端 motionAxes 配置。
	// 仅对这些「控制器+轴」发送 MoveTo 并等待到位，避免：
	// 1) 对未配置/未接硬件的轴（如 Z/U）强制归零；
	// 2) 对未绑定的真实控制器（即使 autoConnect 已连接）发指令并卡死等待。
	// 为空时（旧配置兼容）保持原行为：对所有已连接控制器的所有轴生成目标。
	MotionAxes []MotionAxisBinding `json:"motionAxes,omitempty"`
}

// MotionAxisBinding 遍历运动轴绑定：指定由哪台控制器的哪个轴执行运动。
// ControllerID 为空时表示不限制控制器（仅按轴名过滤，兼容旧数据）。
type MotionAxisBinding struct {
	ControllerID string `json:"controllerId,omitempty"`
	Axis         string `json:"axis"`
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

type PointStatus string

const (
	PointStatusCompleted PointStatus = "completed"
	PointStatusSkipped   PointStatus = "skipped"
	PointStatusFailed    PointStatus = "failed"
)

func (s PointStatus) IsCommitted() bool {
	return s == PointStatusCompleted || s == PointStatusSkipped
}

// PointResult 单点测试结果
type PointResult struct {
	TaskID             string            `json:"taskId,omitempty"`
	CommitSeq          uint64            `json:"commitSeq,omitempty"`
	PointIndex         int               `json:"pointIndex"`
	PointStatus        PointStatus       `json:"pointStatus,omitempty"`
	Point              Point             `json:"point"`
	Timestamp          int64             `json:"timestamp"`
	StartedAt          int64             `json:"startedAt,omitempty"`
	CompletedAt        int64             `json:"completedAt,omitempty"`
	Values             map[int]float64   `json:"values"`
	SampleCount        int               `json:"sampleCount"`
	DwellTimeElapsed   int               `json:"dwellTimeElapsed"`
	Calculated         *CalculatedResult `json:"calculated,omitempty"`
	ValidationWarnings []string          `json:"validationWarnings,omitempty"`
	CSVRowHash         string            `json:"csvRowHash,omitempty"`
	CustomValues       map[string]string `json:"customValues,omitempty"`
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
	CommittedPoints         int           `json:"committedPoints"`
	CurrentPointIndex       int           `json:"currentPointIndex"`
	CurrentPointCoordinates *Point        `json:"currentPointCoordinates,omitempty"`
	CurrentPointPhase       PointPhase    `json:"currentPointPhase,omitempty"`
	TotalPoints             int           `json:"totalPoints"`
	Results                 []PointResult `json:"results"`
	LastError               string        `json:"lastError,omitempty"`
	LastErrorCode           ErrorCode     `json:"lastErrorCode,omitempty"`
	StartedAt               int64         `json:"startedAt,omitempty"`
	ValidationWarnings      []string      `json:"validationWarnings,omitempty"`
}

const CheckpointVersion = 2

type TraversalRunSnapshot struct {
	Config               Config                `json:"config"`
	Validation           *DataValidationConfig `json:"validation,omitempty"`
	Stabilization        *StabilizationConfig  `json:"stabilization,omitempty"`
	InterpolatorIdentity string                `json:"interpolatorIdentity,omitempty"`
	SaveOptions          *SaveOptions          `json:"saveOptions,omitempty"`
	TotalPoints          int                   `json:"totalPoints"`
	CommittedPoints      int                   `json:"committedPoints"`
	CommitSeq            uint64                `json:"commitSeq"`
	CSVPath              string                `json:"csvPath"`
	ResultLogPath        string                `json:"resultLogPath"`
	CSVHeaderHash        string                `json:"csvHeaderHash,omitempty"`
	LastCommitHash       string                `json:"lastCommitHash,omitempty"`
}

// Checkpoint 断点恢复信息
// Config 字段同时写入 Snapshot.Config，读取侧优先读 Config，
// 为空时回退到 Snapshot.Config（兼容 v2 新路径只写 Snapshot 的场景）。
type Checkpoint struct {
	Version         int                  `json:"version"`
	TaskID          string               `json:"taskId"`
	State           State                `json:"state"`
	Snapshot        TraversalRunSnapshot `json:"snapshot"`
	// Config 保存遍历配置的原始 JSON 字节，由 usecase 层通过 json.Marshal 生成。
	// 使用 []byte 而非 json.RawMessage：core/ 禁止导入 encoding/json（六边形架构硬约束）。
	// JSON 序列化时 []byte 会编码为 base64 字符串，反序列化时自动解码，仍可正常往返。
	Config          []byte               `json:"config,omitempty"`
	CompletedPoints int                  `json:"completedPoints"`
	TotalPoints     int                  `json:"totalPoints"`
	LastPoint       *Point               `json:"lastPoint,omitempty"`
	SavePath        string               `json:"savePath"`
	CreatedAt       int64                `json:"createdAt"`
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
