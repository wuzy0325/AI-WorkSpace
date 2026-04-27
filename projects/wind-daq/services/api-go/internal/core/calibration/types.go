package calibration

import "time"

// ==================== 风洞校准数据类型定义 ====================
// 用于五孔探针、三孔探针、总压探针、总温探针的校准数据

// ChannelData 通道数据快照
// 某时刻多个通道的采集值
type ChannelData struct {
	DeviceID string          `json:"deviceId"` // 设备ID
	Channels map[int]float64 `json:"channels"` // channelIndex -> 值映射
}

// ==================== 校准类型定义 ====================

// CalibrationType 校准类型
type CalibrationType string

const (
	CalFiveHole         CalibrationType = "five-hole"         // 五孔探针校准
	CalThreeHole        CalibrationType = "three-hole"        // 三孔探针校准
	CalTotalPressure    CalibrationType = "total-pressure"    // 总压探针校准
	CalTotalTemperature CalibrationType = "total-temperature" // 总温探针校准
)

// CalibrationStatus 校准任务状态
type CalibrationStatus string

const (
	CalIdle        CalibrationStatus = "idle"        // 空闲
	CalConfiguring CalibrationStatus = "configuring" // 配置中
	CalRunning     CalibrationStatus = "running"     // 校准中
	CalPaused      CalibrationStatus = "paused"      // 暂停
	CalCompleted   CalibrationStatus = "completed"   // 完成
	CalError       CalibrationStatus = "error"       // 错误
)

// ==================== 校准配置结构 ====================

// ProbeChannelRole 测点通道角色
// 探针上各通道的功能角色
type ProbeChannelRole string

// ProbeChannelConfig 测点通道配置
// 定义探针上每个通道的关联设备和角色
type ProbeChannelConfig struct {
	DeviceID string           `json:"deviceId"`          // 设备ID
	Channel  int              `json:"channel"`           // 通道号
	Role     ProbeChannelRole `json:"role"`              // 通道角色
	Name     string           `json:"name"`              // 通道名称
	Enabled  bool             `json:"enabled,omitempty"` // 是否启用
}

// MotionAxisConfig 运动轴配置
// 用于自动校准时的点位控制
type MotionAxisConfig struct {
	ControllerID string `json:"controllerId"` // 运动控制器ID
	Axis         string `json:"axis"`         // 轴名称(X/Y/Z/U)
}

// CalibrationPoint 校准点位
// 自动校准时需要到达的位置点
type CalibrationPoint struct {
	Index    int     `json:"index"`           // 点位索引(从0开始)
	X        float64 `json:"x,omitempty"`     // X坐标
	Y        float64 `json:"y,omitempty"`     // Y坐标
	Z        float64 `json:"z,omitempty"`     // Z坐标
	Alpha    float64 `json:"alpha,omitempty"` // 攻角(°)
	Beta     float64 `json:"beta,omitempty"`  // 侧滑角(°)
	Acquired bool    `json:"acquired"`        // 是否已采集
}

// SphereTankGateConfig 球罐判定门控配置
// 用于球罐标定时判断数据稳定性的配置
type SphereTankGateConfig struct {
	Enabled            bool    `json:"enabled"`            // 是否启用
	WaitTimeSec        float64 `json:"waitTimeSec"`        // 等待稳定时间(秒)
	StableTimeDeviceID string  `json:"stableTimeDeviceId"` // 稳定检测设备ID
	StableTimeChannel  int     `json:"stableTimeChannel"`  // 稳定检测通道号
}

// CalibrationConfig 校准任务配置
// 完整的自动校准配置参数
type CalibrationConfig struct {
	Type            CalibrationType       `json:"type"`                     // 校准类型
	Name            string                `json:"name"`                     // 校准名称
	ProbeChannels   []ProbeChannelConfig  `json:"probeChannels"`            // 测点通道配置
	MotionAxes      []MotionAxisConfig    `json:"motionAxes"`               // 运动轴配置
	Points          []CalibrationPoint    `json:"points"`                   // 校准点位列表
	DwellTimeMs     int                   `json:"dwellTimeMs"`              // 停留时间(毫秒)
	SamplesPerPoint int                   `json:"samplesPerPoint"`          // 每点采样次数
	SavePath        string                `json:"savePath"`                 // 结果保存路径
	StopOnError     bool                  `json:"stopOnError,omitempty"`    // 错误时是否停止
	SphereTankGate  *SphereTankGateConfig `json:"sphereTankGate,omitempty"` // 球罐门控配置
}

// ==================== 校准任务状态结构 ====================

// CalibrationTaskStatus 校准任务状态
type CalibrationTaskStatus struct {
	TaskID          string            `json:"taskId"`              // 任务ID
	Status          CalibrationStatus `json:"status"`              // 状态
	CurrentPoint    int               `json:"currentPoint"`        // 当前点位索引
	CompletedPoints int               `json:"completedPoints"`     // 已完成点数
	TotalPoints     int               `json:"totalPoints"`         // 总点数
	StartTime       time.Time         `json:"startTime,omitempty"` // 开始时间
	LastError       string            `json:"lastError,omitempty"` // 最后错误
}

// CalibrationProgressEvent 进度事件
// 校准过程中推送的进度更新
type CalibrationProgressEvent struct {
	TaskID          string      `json:"taskId"`               // 任务ID
	CurrentPoint    int         `json:"currentPoint"`         // 当前点位
	CompletedPoints int         `json:"completedPoints"`      // 已完成点数
	TotalPoints     int         `json:"totalPoints"`          // 总点数
	LatestData      interface{} `json:"latestData,omitempty"` // 最新采集数据
	Timestamp       time.Time   `json:"timestamp"`            // 时间戳
}

// CalibrationCompleteEvent 完成事件
// 校准完成时推送的结果事件
type CalibrationCompleteEvent struct {
	TaskID        string    `json:"taskId"`                  // 任务ID
	Success       bool      `json:"success"`                 // 是否成功
	FilePath      string    `json:"filePath,omitempty"`      // 结果文件路径
	Error         string    `json:"error,omitempty"`         // 错误信息
	Duration      float64   `json:"duration"`                // 耗时(秒)
	TotalPoints   int       `json:"totalPoints"`             // 总点数
	SuccessPoints int       `json:"successPoints,omitempty"` // 成功点数
	Timestamp     time.Time `json:"timestamp"`               // 时间戳
}

// CalibrationRealtimeEvent 实时事件
// 实时数据推送事件
type CalibrationRealtimeEvent struct {
	TaskID    string            `json:"taskId"`          // 任务ID
	Type      string            `json:"type"`            // 事件类型:"data"/"moving"/"waiting"
	Point     *CalibrationPoint `json:"point,omitempty"` // 当前点位
	Data      interface{}       `json:"data,omitempty"`  // 数据(如移动中坐标)
	Timestamp time.Time         `json:"timestamp"`       // 时间戳
}

// ==================== 五孔探针数据 ====================
// 五孔探针校准数据结构
// P1:中心孔, P2/P4:上下孔, P3/P5:左右孔

type FiveHoleRawData struct {
	P1     float64 `json:"p1"`               // 中心孔压力
	P2     float64 `json:"p2"`               // 上孔压力
	P3     float64 `json:"p3"`               // 左孔压力
	P4     float64 `json:"p4"`               // 右孔压力
	P5     float64 `json:"p5"`               // 下孔压力
	PAtm   float64 `json:"pAtm"`             // 大气压
	TAtm   float64 `json:"tAtm"`             // 环境温度
	PTotal float64 `json:"pTotal,omitempty"` // 总压(可选)
}

type FiveHoleCoefficients struct {
	Kalpha float64 `json:"Kalpha"` // 攻角系数: (P2-P3)/(P1-Pavg)
	Kbeta  float64 `json:"Kbeta"`  // 侧滑角系数: (P4-P5)/(P1-Pavg)
	CPT    float64 `json:"CPT"`    // 总压系数: (P1-P∞)/(Pt-P∞)
	CPS    float64 `json:"CPS"`    // 静压系数: (Pavg-P∞)/(P1-P∞)
}

// ==================== 三孔探针数据 ====================
// 三孔探针校准数据结构

type ThreeHoleRawData struct {
	P1     float64 `json:"p1"`               // 中心孔压力
	P2     float64 `json:"p2"`               // 上孔压力
	P3     float64 `json:"p3"`               // 下孔压力
	PAtm   float64 `json:"pAtm"`             // 大气压
	PTotal float64 `json:"pTotal,omitempty"` // 总压(可选)
}

type ThreeHoleCoefficients struct {
	K  float64 `json:"K"`  // 方向系数: (P2-P3)/(P1-P∞)
	Cv float64 `json:"Cv"` // 速度系数: (P1-P∞)/(Pt-P∞)
	Cp float64 `json:"Cp"` // 总压系数
}

// ==================== 总压探针数据 ====================
// 总压探针校准数据结构

type TotalPressureRawData struct {
	PAtm          float64 `json:"pAtm"`          // 大气压
	TAtm          float64 `json:"tAtm"`          // 环境温度
	PTunnelTotal  float64 `json:"pTunnelTotal"`  // 风洞总压
	PTunnelStatic float64 `json:"pTunnelStatic"` // 风洞静压
	TTunnel       float64 `json:"tTunnel"`       // 风洞温度
	PProbeTotal   float64 `json:"pProbeTotal"`   // 探针测量总压
}

type TotalPressureCoefficients struct {
	CPT        float64 `json:"CPT"`        // 总压恢复系数: Pt_probe/Pt_tunnel
	Error      float64 `json:"error"`      // 测量误差(%)
	MachNumber float64 `json:"machNumber"` // 马赫数
}

// ==================== 总温探针数据 ====================
// 总温探针校准数据结构

type TotalTemperatureCalibrationPoint struct {
	ID                     int       `json:"id"`                     // 校准点ID
	TargetMachNumber       float64   `json:"targetMachNumber"`       // 目标马赫数
	ActualMachNumber       float64   `json:"actualMachNumber"`       // 实际马赫数
	TestProbeTemp          float64   `json:"testProbeTemp"`          // 被检探针温度
	StandardProbeTemp      float64   `json:"standardProbeTemp"`      // 标准探针温度
	RecoveryCoefficient    float64   `json:"recoveryCoefficient"`    // 恢复系数
	TotalPressure          float64   `json:"totalPressure"`          // 总压
	StaticPressure         float64   `json:"staticPressure"`         // 静压
	AtmosphericPressure    float64   `json:"atmosphericPressure"`    // 大气压
	AtmosphericTemperature float64   `json:"atmosphericTemperature"` // 环境温度
	StdDev                 float64   `json:"stdDev"`                 // 标准差
	Timestamp              time.Time `json:"timestamp"`              // 时间戳
}
