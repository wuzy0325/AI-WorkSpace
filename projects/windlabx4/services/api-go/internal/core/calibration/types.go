package calibration

import (
	"windlabx4/services/api-go/internal/core/traversal"
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
	// TypeSevenHole 七孔探针校准类型。
	// 七孔探针含外围 6 孔（P1~P6，按 60° 等分环形分布）+ 中心孔 P7，
	// 通过最大压力孔编号将来流划分为内区（7 区，θ≤30°）与外区（1~6 区，θ≥30°），
	// 内区用 (α,β) 坐标系、外区用 (θ,φ) 坐标系，两套坐标系通过 §3.3 公式相互换算。
	TypeSevenHole CalibrationType = "seven-hole"
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

	// AcquisitionStateProvider 设备采集态查询函数（仅运行时注入，不序列化）。
	// 非 nil 时各算法在 waitForFreshData 超时后调用，区分"用户停采集"（可恢复，继续等待）
	// 与"设备在采集但帧不更新"（真异常，返回超时错误）两类场景。
	// 为 nil 时维持原超时行为（向后兼容旧装配路径与未注入场景）。
	AcquisitionStateProvider AcquisitionStateProvider `json:"-"`

	// ==================== 七孔探针专用注入字段（spec Task 5） ====================
	//
	// 七孔算法的实时回调与滞回状态通过 Config 注入而非算法参数传递——
	// 原因：Algorithm 接口的 AcquireDataWithConfig 签名固定，无法新增 onRealtime 参数；
	// 同时七孔的滞回状态需要跨点持续（由 AutomaticCalibration 持有），
	// 通过 Config 注入可让 SevenHoleAlgorithm 保持空结构体（无状态，spec Task 3 约束）。
	//
	// 非七孔类型忽略这三个字段（保持零值），不影响五孔/三孔/总压的现有行为。

	// RealtimeCallback 七孔实时数据推送回调（仅 TypeSevenHole 使用）。
	// 非 nil 时算法在采样循环中按 100ms 节流推送 (rawData, coeffs, region, sector)。
	// 五孔/三孔/总压走各自的 onRealtime 参数路径，不读此字段。
	RealtimeCallback SevenHoleRealtimeCallback `json:"-"`

	// PrevRegion/PrevSector 七孔流场分区判定的滞回状态（spec §3.2 规则 3）。
	// 由 AutomaticCalibration 在每个点采集前注入：
	//   - 首点：PrevRegion="", PrevSector=0（跳过滞回，仅按规则 1、2 判定）
	//   - 后续点：上一时刻的 region/sector（用于规则 3 滞回判定，避免边界点分区抖动）
	PrevRegion string `json:"-"`
	PrevSector int    `json:"-"`
}

// SevenHoleRealtimeCallback 七孔实时数据推送回调类型
//
// 参数语义：
//   - raw：当前样本的 11 通道原始数据
//   - coeffs：基于当前样本实时计算的系数（内区或外区，按当前分区判定结果选择公式）
//   - region：当前分区判定结果（"inner"/"outer"）
//   - sector：当前扇区编号（内区 7，外区 1..6）
//
// 节流策略：调用方（SevenHoleAlgorithm）按 100ms 节流间隔调用，
// 最后一个样本必发——确保前端拿到最终系数用于显示。
type SevenHoleRealtimeCallback func(raw SevenHoleRawData, coeffs SevenHoleCoefficients, region string, sector int)

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
//
// 双坐标模型（七孔探针校准引入，spec §3.4）：
//   - Coordinates（逻辑坐标）：业务语义角度，用于配置生成、CSV 落盘、系数计算、图表绘制。
//     内区表示为 (α,β)，外区表示为 (θ,φ)。
//   - MotionCoordinates（运动坐标）：运动控制器实际下发的目标角度，统一为 (α,β) 双轴。
//     外区点由 (θ,φ) 按 spec §3.3 正向公式换算（α=-arctan(tanθ×sinφ)，负号必须保留）。
//
// 向后兼容：五孔/三孔/总压/总温等已有模块不填 MotionCoordinates/Region/Sector 时，
// moveToPoint 默认走 Coordinates 路径（已有行为），新字段 omitempty 不影响序列化结果。
// 仅 TypeSevenHole 在点位生成阶段（GenerateSevenHolePoints）显式填充双坐标。
type CalPoint struct {
	ID          int                `json:"id"`
	Coordinates map[string]float64 `json:"coordinates"`
	// MotionCoordinates 运动坐标（运动控制器下发用，统一为 α-β 双轴）。
	// 为 nil 时 moveToPoint 回退到 Coordinates（向后兼容）。
	// 消费方（moveToPoint 七孔分支）在 Task 10 落地。
	MotionCoordinates map[string]float64 `json:"motionCoordinates,omitempty"`
	// Region 流场分区："inner"（内区，7 区）或 "outer"（外区，1~6 区）。
	// 仅七孔校准填充，其他类型留空。
	Region string `json:"region,omitempty"`
	// Sector 外区扇区编号 1~6；内区固定 7；其他类型留空（零值）。
	Sector int `json:"sector,omitempty"`
}

// PointResult 通用校准点位结果
type PointResult struct {
	PointIndex     int             `json:"pointIndex"`
	TargetPressure float64         `json:"targetPressure"`
	Timestamp      int64           `json:"timestamp"`
	Values         map[int]float64 `json:"values"`
}

// LivePhysics 实时物理量快照（Task 13）。
//
// 设计动机：前端 5Hz 轮询 status 时需展示当前马赫数/速度，但既有 currentStatus 只持久化
// 校准业务字段（点号/进度/数据点），物理量需基于实时通道数据即时计算。直接写入 currentStatus
// 会导致：(1) 持久化污染（writer/CSV 误把快照写盘）；(2) stale 残留（设备离线后旧值不消失）。
//
// 解决方案：LivePhysics 仅由 Status() 调用时即时计算，绝不写入 currentStatus。
// 通过 *float64 指针语义区分三种状态：
//   - nil：缺失（必需通道未配置/读取失败/物理非法如 Pt < Ps），UI 显示 "--"
//   - &0：有效零（Pt == Ps 即零流量，Task 12），UI 显示格式化的 0
//   - &ma/&v：正常计算值
//
// 整体 *LivePhysics 为 nil 表示类型不支持实时物理量（总温）或未启动校准（currentConfig 为空）。
type LivePhysics struct {
	MachNumber *float64 `json:"machNumber,omitempty"` // 马赫数（缺失 nil / 有效零 &0 / 正常 &ma）
	Velocity   *float64 `json:"velocity,omitempty"`   // 真空速 m/s（缺失 nil / 有效零 &0 / 正常 &v）
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
	// LivePhysics 实时物理量快照（Task 13）：每次 Status() 调用在 m.mu 解锁后
	// 从 m.reader 即时计算，不持久化到 currentStatus（避免 stale 残留与 writer 污染）。
	// 类型不支持（总温）或未启动校准时为 nil；通道齐全但读取失败时为 &LivePhysics{nil, nil}。
	LivePhysics *LivePhysics `json:"livePhysics,omitempty"`
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
	// CurrentRegion/CurrentSector 七孔流场分区当前状态（spec Task 11）。
	// 仅 TypeSevenHole 校准期间有值，供前端 5Hz 轮询 status 时展示"当前区域 inner / 扇区 3"。
	// 其他类型保持零值（omitempty 在 Region="" 时省略字段，Sector=0 时省略字段）。
	CurrentRegion string `json:"currentRegion,omitempty"`
	CurrentSector int    `json:"currentSector,omitempty"`
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
	// TTunnel 风洞温度（可选）。TAT 选取优先级：TTunnel > TAtm（spec §4.4，与七孔/总压一致）。
	// 前端 FiveHoleSettings 把 fiveHole.tTunnel 列为必填通道，但后端 raw data 历史上无此字段，
	// 导致 UI 显示的 TTunnel 永远不参与速度计算（review P1 缺陷修复）。
	TTunnel *float64 `json:"tTunnel,omitempty"`
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

// ==================== 七孔探针类型 ====================
//
// 压力基准三分离声明（spec §2.1）：
//   - A 基准（通道原始值）：P1~P7、p_t、p_s 全部为表压（相对环境大气压，可正可负）。
//   - B 基准（系数计算值）：与 A 同基准——系数是压差比，分子分母同基准时表压与绝压等价，
//     因此系数公式（4.1/4.2 节）直接使用通道原始值，不做转换。
//   - C 基准（大气计算值）：仅马赫数/速度公式（4.4 节）入口处将 p_t、p_s 转绝压，
//     公式 p_abs = p_gauge + 大气压力。禁止在系数计算阶段提前转绝压。
//
// 上述基准分离是 spec §2.1 解决"绝压/表压混用导致大气压重复叠加"问题的硬约束。

// SevenHoleRawData 七孔探针原始数据
//
// 字段语义：
//   - P1~P6：外围 6 孔压力（表压，A 基准），按 60° 等分环形分布。
//   - P7：中心孔压力（表压，A 基准），朝向来流。
//   - PAtm/TAtm：大气压力（绝压）/ 大气温度，用于 A→C 边界转换与静温/真空速计算。
//   - PTotal/PStatic/TTunnel：风洞参考总压/静压/温度（表压），指针类型，
//     缺失时为 nil——此时马赫数/速度无法计算（CSV 写空字符串、UI 显示 "--"）。
type SevenHoleRawData struct {
	P1      float64  `json:"p1"`                // 外围孔 1（方位角 0°，+Y 方向）压力
	P2      float64  `json:"p2"`                // 外围孔 2（方位角 60°）压力
	P3      float64  `json:"p3"`                // 外围孔 3（方位角 120°）压力
	P4      float64  `json:"p4"`                // 外围孔 4（方位角 180°，-Y 方向）压力
	P5      float64  `json:"p5"`                // 外围孔 5（方位角 240°）压力
	P6      float64  `json:"p6"`                // 外围孔 6（方位角 300°）压力
	P7      float64  `json:"p7"`                // 中心孔压力
	PAtm    float64  `json:"pAtm"`              // 大气压力（绝压）
	TAtm    float64  `json:"tAtm"`              // 大气温度（°C）
	PTotal  *float64 `json:"pTotal,omitempty"`  // 风洞参考总压（表压，必需用于 K0/Ks/Ma）
	PStatic *float64 `json:"pStatic,omitempty"` // 风洞参考静压（表压，必需用于 Ks/Ma）
	TTunnel *float64 `json:"tTunnel,omitempty"` // 风洞温度（°C，可选，优先于 TAtm 用于静温计算）
}

// SevenHoleCoefficients 七孔探针校准系数
//
// 内区系数（P7 为最大压力孔时，spec §4.1 公式 1-8）：
//   - Kalpha/Kbeta：α/β 角度系数
//   - K0：内区总压系数 (P7-p_t)/(p_t-p_s)
//   - Ks：内区静压系数 (p_s-P̄)/(p_t-p_s)，P̄=(P1+...+P6)/6
//
// 外区系数（Pn 为最大压力孔时，n∈{1..6}，spec §4.2 公式 9-12）：
//   - Ktheta/Kphi：θ/φ 角度系数（环形取模：n=1 时 n-1=6，n=6 时 n+1=1）
//   - K0Outer/KsOuter：外区总/静压系数
//   - 扇区编号 n 不在系数结构内单独表示，由 SevenHoleDataPoint.Sector 字段携带
//
// 实时气动参数（指针，缺失时为 nil）：
//   - MachNumber：马赫数（spec §4.4，需 PTotal+PStatic+PAtm 齐全，仅在 Ma 计算入口转绝压）
//   - Velocity：速度 m/s（需 MachNumber + 温度齐全）
type SevenHoleCoefficients struct {
	// 内区系数（P7 最大时填充，外区点这些字段保持零值）
	Kalpha float64 `json:"Kalpha"` // 内区 α 角度系数
	Kbeta  float64 `json:"Kbeta"`  // 内区 β 角度系数
	K0     float64 `json:"K0"`     // 内区总压系数
	Ks     float64 `json:"Ks"`     // 内区静压系数
	// 外区系数（Pn 最大时填充，内区点这些字段保持零值）
	// 扇区编号 n 由 SevenHoleDataPoint.Sector 字段携带，CSV 表头按 Kθ[n]/Kφ[n]/K0[n]/Ks[n] 标注
	Ktheta  float64 `json:"Ktheta"`  // 外区 θ 角度系数
	Kphi    float64 `json:"Kphi"`    // 外区 φ 角度系数（边界符号反转不取绝对值，spec §4.3）
	K0Outer float64 `json:"K0Outer"` // 外区总压系数
	KsOuter float64 `json:"KsOuter"` // 外区静压系数
	// 实时气动参数（可选指针，缺失时为 nil）
	MachNumber *float64 `json:"machNumber,omitempty"` // 马赫数（需 PTotal/PStatic/PAtm 齐全）
	Velocity   *float64 `json:"velocity,omitempty"`   // 速度 m/s（需 Ma + 温度齐全）
}

// SevenHoleDataPoint 七孔探针校准数据点
//
// 双坐标模型（spec §3.4）：
//   - Coordinates：逻辑坐标（业务语义，CSV 落盘用）——内区 (α,β)，外区 (θ,φ)
//   - MotionCoordinates：运动坐标（运动控制器下发用，统一 (α,β)）——外区由 (θ,φ) 换算得来
//
// 分区信息：
//   - Region："inner"（7 区）或 "outer"（1~6 区）
//   - Sector：外区扇区编号 1~6；内区固定 7
//   - BoundaryFlag：边界点标记（spec §3.2），非边界点为空串；标记格式如 "P7-P1"、"P1-P2"
//
// 不确定度字段（指针，缺失时为 nil）由 Task 4 的 UncertaintyCalculator 填充。
type SevenHoleDataPoint struct {
	PointID           int                   `json:"pointId"`
	Coordinates       map[string]float64    `json:"coordinates"` // 逻辑坐标
	MotionCoordinates map[string]float64    `json:"motionCoordinates,omitempty"`
	Region            string                `json:"region"` // "inner" / "outer"
	Sector            int                   `json:"sector"` // 内区 7；外区 1~6
	BoundaryFlag      string                `json:"boundaryFlag,omitempty"`
	RawData           SevenHoleRawData      `json:"rawData"`
	Coefficients      SevenHoleCoefficients `json:"coefficients"`
	SampleCount       int                   `json:"sampleCount"`
	StdDev            float64               `json:"stdDev"`
	StartTime         int64                 `json:"startTime"`
	EndTime           int64                 `json:"endTime"`
	RawDeviceChannels map[string][]float64  `json:"rawDeviceChannels,omitempty"`
	// 不确定度字段（spec §5，Task 4 填充）
	UncertaintyKalpha *float64 `json:"uncertaintyKalpha,omitempty"`
	UncertaintyKbeta  *float64 `json:"uncertaintyKbeta,omitempty"`
	UncertaintyK0     *float64 `json:"uncertaintyK0,omitempty"`
	UncertaintyKs     *float64 `json:"uncertaintyKs,omitempty"`
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
	// PressureChannel 球罐压力通道引用，仅用于前端实时显示当前球罐压力值，不参与闸门判定逻辑
	// 当配置了有效 DeviceID 时，采集协调器会自动订阅该设备，以便前端能收到实时快照
	PressureChannel ChannelRef `json:"pressureChannel,omitempty"` // 球罐压力通道引用（仅显示，不参与判定）
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
//
// 各类型实时数据字段（FiveHoleRaw/ThreeHoleRaw/SevenHoleRaw 等）均为指针 + omitempty，
// 仅对应类型的校准任务填充——五孔场景序列化结果不会出现 sevenHoleRaw key（向后兼容）。
type RealtimeEvent struct {
	TaskID                string                 `json:"taskId"`
	WindowTag             string                 `json:"windowTag"`
	Type                  CalibrationType        `json:"type"`
	Timestamp             int64                  `json:"timestamp"`
	FiveHoleRaw           *FiveHoleRawData       `json:"fiveHoleRaw,omitempty"`
	FiveHoleCoefficients  *FiveHoleCoefficients  `json:"fiveHoleCoefficients,omitempty"`
	ThreeHoleRaw          *ThreeHoleRawData      `json:"threeHoleRaw,omitempty"`
	ThreeHoleCoefficients *ThreeHoleCoefficients `json:"threeHoleCoefficients,omitempty"`
	// 七孔实时数据（仅 TypeSevenHole 填充，其他类型留空确保向后兼容）
	SevenHoleRaw          *SevenHoleRawData      `json:"sevenHoleRaw,omitempty"`
	SevenHoleCoefficients *SevenHoleCoefficients `json:"sevenHoleCoefficients,omitempty"`
	Point                 *CalPoint              `json:"point,omitempty"`
}

// RegionChangedEvent 七孔流场分区变更事件（spec Task 11 + §3.2）
//
// 仅 TypeSevenHole 推送；五孔/三孔/总压/总温不触发，EventPublisher 实现可空实现。
//
// 推送时机（processPoint 流程内）：
//   - 首点：必推送一次，PrevRegion/PrevSector=nil（JSON 序列化为 null）
//   - 后续点：当 region 或 sector 与上一时刻不同时立即推送，PrevRegion/PrevSector 指向上一时刻值
//   - 不变时不推送（避免噪声）
//
// PrevRegion/PrevSector 为指针类型——首点时 nil（JSON null）与合法零值（"inner"/0）明确区分，
// 此设计与 spec §9.4 的 JSON null 契约一致，前端可通过 ===null 判断"无前序"。
type RegionChangedEvent struct {
	TaskID       string  `json:"taskId"`
	WindowTag    string  `json:"windowTag"`
	Region       string  `json:"region"`             // 当前区域："inner" 或 "outer"
	Sector       int     `json:"sector"`              // 当前扇区：1~6（外区）或 7（内区）
	PrevRegion   *string `json:"prevRegion"`          // 上一时刻区域，首点 nil（JSON null）
	PrevSector   *int    `json:"prevSector"`          // 上一时刻扇区，首点 nil（JSON null）
	BoundaryFlag string  `json:"boundaryFlag"`        // 边界标志："first" / "inner-outer" / "sector-switch" / "same-region"
	PointIndex   int     `json:"pointIndex"`          // 当前点索引（0-based）
	TotalPoints  int     `json:"totalPoints"`         // 总点数
	Timestamp    int64   `json:"timestamp"`           // 事件时间戳（Unix 毫秒）
}

// EventPublisher 校准事件发布接口
type EventPublisher interface {
	OnProgress(event ProgressEvent)
	OnComplete(event CompleteEvent)
	OnRealtime(event RealtimeEvent)
	// OnRegionChanged 七孔分区变更事件（必需方法，非可选）。
	// 七孔校准首点及分区切换时调用；其他类型不触发，实现可空实现（no-op）。
	OnRegionChanged(event RegionChangedEvent)
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
