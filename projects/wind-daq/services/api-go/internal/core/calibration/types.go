package calibration

import (
	"wind-daq/services/api-go/internal/core/traversal"
)

// State 表示校准任务的状态
type State string

const (
	StateIdle      State = "idle"
	StateRunning   State = "running"
	StatePaused    State = "paused"
	StateStopped   State = "stopped"
	StateError     State = "error"
	StateCompleted State = "completed"
)

// CalibrationType 表示校准类型
type CalibrationType string

const (
	TypeFiveHole         CalibrationType = "five-hole"
	TypeThreeHole        CalibrationType = "three-hole"
	TypeTotalPressure    CalibrationType = "total-pressure"
	TypeTotalTemperature CalibrationType = "total-temperature"
)

// Config 校准任务通用配置
type Config struct {
	TaskID          string             `json:"taskId"`
	DeviceID        string             `json:"deviceId"`
	Type            string             `json:"type"`
	Channels        []int              `json:"channels"`
	PressurePoints  []float64          `json:"pressurePoints"`
	AverageSamples  int                `json:"averageSamples"`
	ProbeChannels   []ProbeChannel     `json:"probeChannels,omitempty"`
	Points          []CalPoint         `json:"points,omitempty"`
	SamplesPerPoint int                `json:"samplesPerPoint,omitempty"`
	DwellTimeMs     int                `json:"dwellTimeMs,omitempty"`
	StopOnError     bool               `json:"stopOnError,omitempty"`
	Name            string             `json:"name"`                 // 校准任务名称
	SavePath        string             `json:"savePath,omitempty"`   // 数据保存路径
	MotionAxes      []MotionAxisConfig `json:"motionAxes,omitempty"` // 运动轴配置
	// MotionSafety 运动安全配置：到位容差、严重偏离阈值、跨样本看门狗等。
	// 为 nil 时下游使用 traversal.DefaultMotionSafety，保证旧配置反序列化兼容。
	// 类型复用 core/traversal.MotionSafetyConfig，与遍历测试共用同一套阈值语义和 Resolve/Merge 方法。
	MotionSafety           *traversal.MotionSafetyConfig `json:"motionSafety,omitempty"`           // 运动安全配置
	SphereTankGate         *SphereTankGateConfig         `json:"sphereTankGate,omitempty"`         // 球罐闸门配置
	AcquisitionSampling    *AcquisitionSamplingConfig    `json:"acquisitionSampling,omitempty"`    // 采集采样配置
	TotalTemperatureConfig *TotalTemperatureConfig       `json:"totalTemperatureConfig,omitempty"` // 总温校准专用配置
	// TimestampReader 设备时间戳读取函数（仅运行时注入，不序列化）。
	// 非 nil 时各算法在多次采样间等待设备推送新数据帧后才计入有效采样，避免重复读缓存旧数据。
	TimestampReader TimestampReader `json:"-"`
}

// ProbeChannel 探针通道配置，将逻辑角色映射到物理通道
type ProbeChannel struct {
	Role         string `json:"role"`
	Name         string `json:"name"`
	DeviceID     string `json:"deviceId"`
	ChannelIndex int    `json:"channelIndex"`
	Enabled      bool   `json:"enabled"`
}

// 注意：ProbeChannel 不再自带 UnmarshalJSON。
// 前端发送的嵌套 channel 格式（{ channel: { deviceId, channelIndex } }）由
// adapters/config 层的 CalibrationConfigDTO 负责解码（见 DecodeCalibrationConfig）。
// core 层禁止做字节级 I/O（CLAUDE.md 零容忍约束），struct tag 仅描述序列化字段名，不是 I/O。

// CalPoint 校准测点定义
type CalPoint struct {
	ID          int                `json:"id"`
	Coordinates map[string]float64 `json:"coordinates"`
}

// PointResult 通用校准点位结果
type PointResult struct {
	PointIndex     int             `json:"pointIndex"`
	TargetPressure float64         `json:"targetPressure"`
	Timestamp      int64           `json:"timestamp"`
	Values         map[int]float64 `json:"values"`
}

// Status 校准任务状态
type Status struct {
	TaskID string `json:"taskId"`
	State  State  `json:"state"`
	// CurrentPoint 当前正在处理的点索引（autoEngine.currentPointIdx，processPoint 循环顶部推进，早于 moveToPoint）。
	// 非"已完成点数"——后者见 CompletedPoints。前端 progressInfo 据此索引查 config.points 得到"目标点"，
	// 让目标角度先于实际角度变化。autoEngine 为 nil（未启动/总温手动模式）时为 0。
	CurrentPoint int    `json:"currentPoint"`
	TotalPoints  int    `json:"totalPoints"`
	LastError    string `json:"lastError,omitempty"`
	// LastErrorCode 结构化错误码（新增，运动安全故障时写入对应的 traversal.ErrorCode）。
	// 前端根据此字段展示对应级别的告警（急停类红色 / 普通停止类橙色 / 超时类黄色）。
	// 非运动安全错误（采集失败/保存失败等）写入对应业务错误码或空串。
	LastErrorCode string `json:"lastErrorCode,omitempty"`
	// MotionSafetyFailure 运动安全故障现场快照。
	// 仅在运动安全故障路径写入，其他错误路径（采集失败/保存失败等）保持 nil。
	// 前端轮询拿到后用于展示故障现场（控制器/轴/verdict/目标/实际/点号），
	// 避免 lastError 字符串正则解析的不稳定。
	MotionSafetyFailure *traversal.MotionSafetyFailure `json:"motionSafetyFailure,omitempty"`
	Type                string                         `json:"type"`
	CompletedPoints     int                            `json:"completedPoints"`
	Progress            float64                        `json:"progress"` // 百分比 0-100
	StartTime           int64                          `json:"startTime,omitempty"`
	// PausedDurationMs 是截至本次状态快照时累计的暂停时长，包含当前尚未结束的暂停段。
	PausedDurationMs int64       `json:"pausedDurationMs"`
	DataPoints       []DataPoint `json:"dataPoints,omitempty"`
	// 当前点采样进度：CurrentSample=当前点已采样本数（1..SamplesPerPoint），0 表示未开始/已完成
	// SamplesPerPoint=当前点总采样数。前端据此显示"当前点采样 3/10"子进度。
	CurrentSample   int `json:"currentSample,omitempty"`
	SamplesPerPoint int `json:"samplesPerPoint,omitempty"`
}

// ==================== 五孔探针类型 ====================

// FiveHoleRawData 五孔探针原始数据
type FiveHoleRawData struct {
	P1      float64  `json:"p1"`                // 下孔压力
	P2      float64  `json:"p2"`                // 中孔（中心孔）压力
	P3      float64  `json:"p3"`                // 上孔压力
	P4      float64  `json:"p4"`                // 左孔压力
	P5      float64  `json:"p5"`                // 右孔压力
	PAtm    float64  `json:"pAtm"`              // 大气压力
	TAtm    float64  `json:"tAtm"`              // 大气温度
	PTotal  *float64 `json:"pTotal,omitempty"`  // 风洞总压（可选）
	PStatic *float64 `json:"pStatic,omitempty"` // 风洞静压（可选）
}

// FiveHoleCoefficients 五孔探针系数
type FiveHoleCoefficients struct {
	Kalpha     float64  `json:"Kalpha"`               // 攻角系数
	Kbeta      float64  `json:"Kbeta"`                // 侧滑角系数
	CPT        float64  `json:"CPT"`                  // 总压恢复系数
	CPS        float64  `json:"CPS"`                  // 静压恢复系数
	MachNumber *float64 `json:"machNumber,omitempty"` // 马赫数（可选）
}

// FiveHoleDataPoint 五孔探针校准数据点
type FiveHoleDataPoint struct {
	PointID           int                  `json:"pointId"`
	Coordinates       map[string]float64   `json:"coordinates"`
	RawData           FiveHoleRawData      `json:"rawData"`
	Coefficients      FiveHoleCoefficients `json:"coefficients"`
	SampleCount       int                  `json:"sampleCount"`
	StdDev            float64              `json:"stdDev"`
	StartTime         int64                `json:"startTime"`
	EndTime           int64                `json:"endTime"`
	RawDeviceChannels map[string][]float64 `json:"rawDeviceChannels,omitempty"`
}

// ==================== 三孔探针类型 ====================

// ThreeHoleRawData 三孔探针原始数据
//
// 孔序约定（与 shared/algorithms/go/threehole/interpolation 对齐）：
//
//	P1 = 侧孔1（左孔）
//	P2 = 中心孔（ΔP 中心参考）
//	P3 = 侧孔2（右孔）
//
// 插值器：ΔP = 2·P2 - P1 - P3，Kb = (P3 - P1) / ΔP。
type ThreeHoleRawData struct {
	P1      float64  `json:"p1"`                // 侧孔1压力（左孔）
	P2      float64  `json:"p2"`                // 中心孔压力
	P3      float64  `json:"p3"`                // 侧孔2压力（右孔）
	PAtm    float64  `json:"pAtm"`              // 大气压力
	TAtm    float64  `json:"tAtm"`              // 大气温度
	PTotal  *float64 `json:"pTotal,omitempty"`  // 风洞总压（必需，用于 K0/Kv）
	PStatic *float64 `json:"pStatic,omitempty"` // 风洞静压（必需，用于 Kv）
}

// ThreeHoleCoefficients 三孔探针校准系数
//
// 工程命名：Kb(Kβ) / K0 / Kv，与插值器 PRB 文件列对齐
// （shared/algorithms/go/threehole/interpolation/three_hole.go）：
//
//	ΔP = 2·P2 - P1 - P3           中心孔与侧孔差压
//	Kb = (P3 - P1) / ΔP           角度系数 Kβ（仅孔压，始终可算）
//	K0 = (Pt - P2) / ΔP           总压系数 K0（需 PTotal；缺失置 0，不发误导值）
//	Kv = (Pt - Ps) / ΔP           速度系数 Kv（需 PTotal + PStatic；缺失置 0）
//
// 插值时反演：Pt = P2 + K0·ΔP, Ps = Pt - Kv·ΔP
//
// MachNumber/Velocity 为实时气动参数（可选）：需 PTotal + PStatic + PAtm + TAtm 齐全时计算，
// 缺失任一通道时为 nil，CSV 写空字符串、UI 显示 "--"。
type ThreeHoleCoefficients struct {
	Kb         float64  `json:"Kb"`                   // 角度系数 Kβ
	K0         float64  `json:"K0"`                   // 总压系数 K0
	Kv         float64  `json:"Kv"`                   // 速度系数 Kv
	MachNumber *float64 `json:"machNumber,omitempty"` // 马赫数（可选，需风洞总压/静压/大气压/温度齐全）
	Velocity   *float64 `json:"velocity,omitempty"`   // 速度 m/s（可选，需风洞总压/静压/大气压/温度齐全）
}

// ThreeHoleDataPoint 三孔探针校准数据点
type ThreeHoleDataPoint struct {
	PointID      int                   `json:"pointId"`
	Coordinates  map[string]float64    `json:"coordinates"`
	RawData      ThreeHoleRawData      `json:"rawData"`
	Coefficients ThreeHoleCoefficients `json:"coefficients"`
	SampleCount  int                   `json:"sampleCount"`
	StdDev       float64               `json:"stdDev"`
	StartTime    int64                 `json:"startTime"`
	EndTime      int64                 `json:"endTime"`
}

// ==================== 总压探针类型 ====================

// TotalPressureRawData 总压探针原始数据
type TotalPressureRawData struct {
	PAtm          float64 `json:"pAtm"`          // 大气压力
	TAtm          float64 `json:"tAtm"`          // 大气温度
	PTunnelTotal  float64 `json:"pTunnelTotal"`  // 风洞总压
	PTunnelStatic float64 `json:"pTunnelStatic"` // 风洞静压
	TTunnel       float64 `json:"tTunnel"`       // 风洞温度
	PProbeTotal   float64 `json:"pProbeTotal"`   // 探针总压
}

// TotalPressureCoefficients 总压探针系数
//
// MachNumber/Velocity 为实时气动参数（可选指针）：
// 需风洞总压/静压/大气压/温度齐全且物理合法时才计算，
// 风洞未建立有效压差或通道缺失时为 nil，CSV 写空字符串、UI 显示 "--"。
// 与 ThreeHoleCoefficients 保持一致的 nil 语义，避免 0 值误导操作员。
type TotalPressureCoefficients struct {
	CPT        float64  `json:"CPT"`                  // 总压恢复系数
	Error      float64  `json:"error"`                // 误差(%)
	MachNumber *float64 `json:"machNumber,omitempty"` // 马赫数（可选，需风洞总压/静压/大气压/温度齐全）
	Velocity   *float64 `json:"velocity,omitempty"`   // 速度 m/s（可选，需风洞总压/静压/大气压/温度齐全）
}

// TotalPressureDataPoint 总压探针校准数据点
type TotalPressureDataPoint struct {
	PointID      int                       `json:"pointId"`
	Alpha        float64                   `json:"alpha"`
	RawData      TotalPressureRawData      `json:"rawData"`
	Coefficients TotalPressureCoefficients `json:"coefficients"`
	SampleCount  int                       `json:"sampleCount"`
	// StdDev 多次采样探针总压的样本标准差（Pa），用于判断采样稳定性。
	// 与 FiveHole/ThreeHole/TotalTemperature 保持一致的字段命名，便于 CSV/UI 统一展示。
	StdDev    float64 `json:"stdDev"`
	StartTime int64   `json:"startTime"`
	EndTime   int64   `json:"endTime"`
}

// ==================== 总温探针类型 ====================

// TotalTemperatureConfig 总温校准专用配置
type TotalTemperatureConfig struct {
	ProbeChannels     map[string]ChannelRef      `json:"probeChannels"`
	TargetMachNumbers []float64                  `json:"targetMachNumbers"`
	MachTolerance     float64                    `json:"machTolerance"`
	StabilityCriteria TemperatureStabilityConfig `json:"stabilityCriteria"`
	SamplesPerPoint   int                        `json:"samplesPerPoint"`
	SampleInterval    int                        `json:"sampleInterval"` // 毫秒
	EnableFitting     bool                       `json:"enableFitting"`
}

// ChannelRef 通道引用
type ChannelRef struct {
	DeviceID     string `json:"deviceId"`
	ChannelIndex int    `json:"channelIndex"`
}

// TemperatureStabilityConfig 温度稳定性检测配置
type TemperatureStabilityConfig struct {
	SampleCount    int     `json:"sampleCount"`
	SampleInterval int     `json:"sampleInterval"` // 毫秒
	MaxStdDev      float64 `json:"maxStdDev"`
}

// TotalTemperatureState 总温校准专用状态
type TotalTemperatureState struct {
	Status            string  `json:"status"`
	CurrentIndex      int     `json:"currentIndex"`
	CurrentMachNumber float64 `json:"currentMachNumber"`
	TargetMachNumber  float64 `json:"targetMachNumber"`
	IsNearTarget      bool    `json:"isNearTarget"`
	TemperatureStable bool    `json:"temperatureStable"`
	StartTime         int64   `json:"startTime"`
}

// TotalTemperatureDataPoint 总温探针校准数据点
type TotalTemperatureDataPoint struct {
	ID                     int     `json:"id"`
	TargetMachNumber       float64 `json:"targetMachNumber"`
	ActualMachNumber       float64 `json:"actualMachNumber"`
	TestProbeTemp          float64 `json:"testProbeTemp"`
	StandardProbeTemp      float64 `json:"standardProbeTemp"`
	RecoveryCoefficient    float64 `json:"recoveryCoefficient"`
	TotalPressure          float64 `json:"totalPressure"`
	StaticPressure         float64 `json:"staticPressure"`
	AtmosphericPressure    float64 `json:"atmosphericPressure"`
	AtmosphericTemperature float64 `json:"atmosphericTemperature"`
	StdDev                 float64 `json:"stdDev"`
	Timestamp              int64   `json:"timestamp"`
}

// ==================== 运动轴配置 ====================

// MotionAxisConfig 校准运动轴配置，将逻辑轴名映射到物理运动控制器
type MotionAxisConfig struct {
	ControllerID string `json:"controllerId"` // 运动控制器ID
	Axis         string `json:"axis"`         // 轴名称（如 "x", "y", "z"）
	Name         string `json:"name"`         // 逻辑轴名（如 "α", "β", "θ"）
}

// ==================== 球罐闸门配置 ====================

// SphereTankGateConfig 球罐闸门判定配置
type SphereTankGateConfig struct {
	Enabled           bool       `json:"enabled"`              // 是否启用球罐判定
	WaitTimeSec       float64    `json:"waitTimeSec"`          // 等待稳定时间（秒）
	TimeoutSec        int        `json:"timeoutSec,omitempty"` // 球罐判定总超时（秒），<=0 时使用默认 300 秒
	StableTimeChannel ChannelRef `json:"stableTimeChannel"`    // 稳定时间通道引用
}

// ==================== 采集采样配置 ====================

// AcquisitionSamplingConfig 采集采样参数配置
type AcquisitionSamplingConfig struct {
	BatchTimeoutMs      int `json:"batchTimeoutMs,omitempty"`      // 批量读取超时（毫秒）
	BatchPollIntervalMs int `json:"batchPollIntervalMs,omitempty"` // 批量读取轮询间隔（毫秒）
	BatchMaxAgeMs       int `json:"batchMaxAgeMs,omitempty"`       // 数据最大年龄（毫秒）
}

// ==================== 校准事件类型 ====================

// ProgressEvent 校准进度事件
type ProgressEvent struct {
	TaskID          string    `json:"taskId"`
	WindowTag       string    `json:"windowTag"`
	CurrentPoint    CalPoint  `json:"currentPoint"`
	CompletedPoints int       `json:"completedPoints"`
	TotalPoints     int       `json:"totalPoints"`
	LatestData      DataPoint `json:"latestData,omitempty"`
	Timestamp       int64     `json:"timestamp"`
}

// CompleteEvent 校准完成事件
type CompleteEvent struct {
	TaskID        string `json:"taskId"`
	WindowTag     string `json:"windowTag"`
	Success       bool   `json:"success"`
	Error         string `json:"error,omitempty"`
	Duration      int64  `json:"duration"` // 毫秒
	TotalPoints   int    `json:"totalPoints"`
	SuccessPoints int    `json:"successPoints"`
	FilePath      string `json:"filePath,omitempty"`
}

// RealtimeEvent 校准实时数据事件
type RealtimeEvent struct {
	TaskID                string                 `json:"taskId"`
	WindowTag             string                 `json:"windowTag"`
	Type                  CalibrationType        `json:"type"`
	Timestamp             int64                  `json:"timestamp"`
	FiveHoleRaw           *FiveHoleRawData       `json:"fiveHoleRaw,omitempty"`
	FiveHoleCoefficients  *FiveHoleCoefficients  `json:"fiveHoleCoefficients,omitempty"`
	ThreeHoleRaw          *ThreeHoleRawData      `json:"threeHoleRaw,omitempty"`
	ThreeHoleCoefficients *ThreeHoleCoefficients `json:"threeHoleCoefficients,omitempty"`
	Point                 *CalPoint              `json:"point,omitempty"`
}

// EventPublisher 校准事件发布接口
type EventPublisher interface {
	OnProgress(event ProgressEvent)
	OnComplete(event CompleteEvent)
	OnRealtime(event RealtimeEvent)
}

// ==================== 校准导出载荷 ====================

// ExportPayload 校准导出数据
type ExportPayload struct {
	Type       CalibrationType `json:"type"`
	Config     Config          `json:"config"`
	DataPoints []DataPoint     `json:"dataPoints"`
}

// ModuleResult 校准模块结果
type ModuleResult struct {
	TaskID     string          `json:"taskId"`
	Type       CalibrationType `json:"type"`
	Config     Config          `json:"config"`
	DataPoints []DataPoint     `json:"dataPoints"`
}
