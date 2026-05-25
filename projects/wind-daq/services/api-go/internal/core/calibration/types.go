package calibration

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
	TaskID          string         `json:"taskId"`
	DeviceID        string         `json:"deviceId"`
	Type            string         `json:"type"`
	Channels        []int          `json:"channels"`
	PressurePoints  []float64      `json:"pressurePoints"`
	AverageSamples  int            `json:"averageSamples"`
	ProbeChannels   []ProbeChannel `json:"probeChannels,omitempty"`
	Points          []CalPoint     `json:"points,omitempty"`
	SamplesPerPoint int            `json:"samplesPerPoint,omitempty"`
	DwellTimeMs     int            `json:"dwellTimeMs,omitempty"`
	StopOnError     bool           `json:"stopOnError,omitempty"`
}

// ProbeChannel 探针通道配置，将逻辑角色映射到物理通道
type ProbeChannel struct {
	Role         string `json:"role"`
	Name         string `json:"name"`
	DeviceID     string `json:"deviceId"`
	ChannelIndex int    `json:"channelIndex"`
	Enabled      bool   `json:"enabled"`
}

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
	TaskID       string        `json:"taskId"`
	State        State         `json:"state"`
	CurrentPoint int           `json:"currentPoint"`
	TotalPoints  int           `json:"totalPoints"`
	Results      []PointResult `json:"results"`
	LastError    string        `json:"lastError,omitempty"`
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
type ThreeHoleRawData struct {
	P1     float64  `json:"p1"`               // 中心孔压力
	P2     float64  `json:"p2"`               // 侧孔1压力
	P3     float64  `json:"p3"`               // 侧孔2压力
	PAtm   float64  `json:"pAtm"`             // 大气压力
	PTotal *float64 `json:"pTotal,omitempty"` // 风洞总压（可选）
}

// ThreeHoleCoefficients 三孔探针系数
type ThreeHoleCoefficients struct {
	K  float64 `json:"K"`  // 方向系数
	Cv float64 `json:"Cv"` // 速度系数
	Cp float64 `json:"Cp"` // 总压系数
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
type TotalPressureCoefficients struct {
	CPT        float64 `json:"CPT"`        // 总压恢复系数
	Error      float64 `json:"error"`      // 误差(%)
	MachNumber float64 `json:"machNumber"` // 马赫数
}

// TotalPressureDataPoint 总压探针校准数据点
type TotalPressureDataPoint struct {
	PointID      int                       `json:"pointId"`
	Alpha        float64                   `json:"alpha"`
	RawData      TotalPressureRawData      `json:"rawData"`
	Coefficients TotalPressureCoefficients `json:"coefficients"`
	SampleCount  int                       `json:"sampleCount"`
	StartTime    int64                     `json:"startTime"`
	EndTime      int64                     `json:"endTime"`
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
