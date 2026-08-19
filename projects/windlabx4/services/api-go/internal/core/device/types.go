package device

import (
	"time"
)

type Type string

const (
	DeviceSimulated   Type = "SIMULATED"
	DeviceDAQP1604    Type = "DAQ-P-1604"
	DeviceDaqT1603    Type = "DAQ-T-1603"
	DeviceDAQP1604Pre Type = "DAQ-P-1604Pre" // 原 DAQ-P-1064Pre，统一为 1604Pre
	DeviceWTNPXI      Type = "WTN_PXI"
	DeviceDSA3217     Type = "DSA3217"
	// DeviceDAQP1603 DAQ-P-1603 16 通道通用 AI 采集设备。
	// 与 shared SDK 的 core.DeviceDAQP1603 字面量保持一致，
	// 驱动 bootstrap 工厂 switch 与 profile JSON 反序列化时的类型路由。
	DeviceDAQP1603 Type = "DAQ-P-1603"
	// DeviceDaqT1602 DAQ-T-1602 温度扫描阀：Modbus TCP（端口 502），
	// 机内 2 张独立采集卡（Unit ID 1/2），每卡 8 通道，合并对外 16 通道。
	// 与 DAQ-T-1603 协议、传输、数据语义完全不同，身份严格隔离（spec-daq-t1602）。
	DeviceDaqT1602 Type = "DAQ-T-1602"
	// DevicePACE1000 PACE1000 单通道大气压力串口采集设备。
	DevicePACE1000 Type = "PACE1000"
)

// ChannelSensorType 通道传感器类型枚举（仅 DAQ-P-1603 使用）。
// 字面量与 shared/device-sdk/go/daq/core.ChannelSensorType 保持一致，
// 保证 adapter 层做类型翻译时无需额外转换。
type ChannelSensorType string

const (
	// SensorPressure 压力通道（Pa/kPa/MPa/mmH2O）
	SensorPressure ChannelSensorType = "pressure"
	// SensorTemperature 温度通道（℃/℉）
	SensorTemperature ChannelSensorType = "temperature"
)

// DAQ-P-1604Pre 通道布局常量
// 数据帧 payload 共 72 字节，按以下布局解析：
//
//	[0..3]  大气压     → P1604PreAtmChannelIndex (16)
//	[4..7]  大气温度   → P1604PreAtmTempChannelIndex (17)
//	[8..71] 16 路压力  → Index 0..15
//
// 这些常量在 adapter 数据解析、profile 默认值、normalize 升级三处共用，
// 避免硬编码 16/17 导致修改时遗漏。提取到 core/device 包是为了让 adapter
// 与 usecase 都能引用，同时不引入硬件协议细节（仅是通道索引约定）。
const (
	// P1604PreAtmChannelIndex 大气压通道在 profile.Channels 中的索引
	P1604PreAtmChannelIndex = 16
	// P1604PreAtmTempChannelIndex 大气温度通道在 profile.Channels 中的索引
	P1604PreAtmTempChannelIndex = 17
	// P1604PrePressureChannelCount 1604Pre 压力通道数量
	P1604PrePressureChannelCount = 16
)

// IsAtmosphericChannel 判断指定设备类型的通道是否为大气压/大气温度辅助通道。
//
// DAQ-P-1604 / DAQ-P-1604Pre 的 Index 16/17 为大气辅助通道（环境量），典型值
// 大气压 ~101325 Pa、大气温度 ~25 ℃，与常规测量通道（±5000 Pa 量级）物理含义不同。
// 这类通道不得参与校零——若被校零，采样均值 ~101325 Pa 会被写入 CalibrationOffset，
// 后续采集时 CalibrationApplier 减去该偏移，导致大气压读数恒为 ~0，完全失真。
//
// 与前端 shouldDisableTare（DeviceDetailPanel.vue）逻辑对齐，避免前端禁用了单通道
// 校零按钮、后端却在"设备级校零/全部校零"（targetChannel==nil）路径上误校零。
//
// 其他设备类型（DAQ-P-1603 / DAQ-T-1603 / DSA3217 / WTN_PXI / SIMULATED）
// 无大气辅助通道，统一返回 false。
// PACE1000 唯一通道（Index 0）即大气压力，同样禁止校零（spec-pace1000-integration）。
func IsAtmosphericChannel(profileType Type, channelIndex int) bool {
	switch profileType {
	case DeviceDAQP1604, DeviceDAQP1604Pre:
		return channelIndex == P1604PreAtmChannelIndex || channelIndex == P1604PreAtmTempChannelIndex
	case DevicePACE1000:
		return channelIndex == 0
	default:
		return false
	}
}

type Connection string

const (
	ConnectionDisconnected Connection = "Disconnected"
	ConnectionConnected    Connection = "Connected"
	ConnectionAcquiring    Connection = "Acquiring"
	ConnectionError        Connection = "Error"
)

type ChannelConfig struct {
	Index     int     `json:"index"`
	Name      string  `json:"name"`
	Enabled   bool    `json:"enabled"`
	Unit      string  `json:"unit"`
	Precision int     `json:"precision"`
	RangeMin  float64 `json:"rangeMin,omitempty"`
	RangeMax  float64 `json:"rangeMax,omitempty"`
	// 【废弃】TareOffset 瞬时归零偏移，v2 迁移后由 CalibrationOffset 接管。
	// 迁移逻辑见 adapters/config/migration.go。保留字段以确保向后兼容旧 JSON 反序列化。
	TareOffset float64 `json:"tareOffset,omitempty"`
	// SensorType 通道传感器类型（pressure/temperature），仅 DAQ-P-1603 使用。
	// 旧 profile（含 DAQ-P-1604 / DAQ-T-1603 / 历史 SIMULATED）无此字段，
	// 反序列化时由 UnmarshalJSON 兜底为 "pressure"，
	// 保证读路径拿到的 ChannelConfig 永远有合法 SensorType 值，避免业务层到处判空。
	SensorType ChannelSensorType `json:"sensorType,omitempty"`

	// ---- v2 校零字段 ----
	// CalibrationOffset 校零偏移值（已转为基单位，如 Pa/℃）。
	// 零值表示未校零。CalibrationApplier 将此值从 DataPayload.Channels 中减去。
	CalibrationOffset float64 `json:"calibrationOffset,omitempty"`
	// CalibrationUnit 校零时的原始单位（如 "kPa"、"℉"），用于迁移校验与审计。
	// 注意：存储值已转为基单位，此字段仅作元数据记录，不参与实时计算。
	CalibrationUnit string `json:"calibrationUnit,omitempty"`
	// CalibrationAt 校零时间戳（unix ms），用于 UI 展示"上次校零于 xxx"。
	CalibrationAt int64 `json:"calibrationAt,omitempty"`
	// CalibrationEnabled 校零使能开关（仅 DAQ-P-1603 在 UI 暴露逐通道配置）。
	// 关闭时 CalibrationApplier 跳过该通道偏移。其他设备默认 true。
	// 注意：不使用 omitempty——false 是用户主动设置的合法值（"该通道不应用校零"），
	// 若加 omitempty，序列化（持久化 / HTTP 回读 / Wails binding）会丢弃该字段，
	// 导致前端 draft 被回读值覆盖后 UI 复选框跳回勾选状态，给用户造成"无法保存"的假象。
	// 与 Enabled 字段保持一致：布尔关必须显式输出，区分"未设置"与"显式 false"。
	CalibrationEnabled bool `json:"calibrationEnabled"`
}

// Profile 设备配置档案
// 注意：硬件特定的默认值生成已迁移到 adapters/config 包，
// core 层只保留类型定义，不包含基础设施知识
//
// Version 字段说明（v2）：
//
//	1 = 旧格式（TareOffset 瞬时归零）
//	2 = 新格式（CalibrationOffset 校零）。LoadProfiles 时自动迁移 1→2。
type Profile struct {
	// Version profile 格式版本，默认 1（旧格式），迁移后为 2。
	Version                    int                    `json:"version"`
	ID                         string                 `json:"id"`
	Name                       string                 `json:"name"`
	Type                       Type                   `json:"type"`
	Transport                  string                 `json:"transport,omitempty"`
	Address                    string                 `json:"address,omitempty"`
	LocalAddress               string                 `json:"localAddress,omitempty"`
	Port                       int                    `json:"port,omitempty"`
	SerialPort                 string                 `json:"serialPort,omitempty"`
	BaudRate                   int                    `json:"baudRate,omitempty"`
	AutoConnect                bool                   `json:"autoConnect,omitempty"`
	MacAddress                 string                 `json:"macAddress,omitempty"`
	SamplingRate               int                    `json:"samplingRate"`
	Channels                   []ChannelConfig        `json:"channels"`
	DaqP1604UseDeviceTimestamp *bool                  `json:"daqP1604UseDeviceTimestamp,omitempty"`
	DaqT1603Config             DaqT1603HardwareConfig `json:"daqT1603Config,omitempty"`
	DaqT1602Config             DaqT1602HardwareConfig `json:"daqT1602Config,omitempty"`
}

// UseDeviceTimestampEnabled 返回 DAQ-P-1604 是否启用硬件时间戳。
// 三态语义（与 daq-p1604 项目对齐）：
//   - nil（老 profile 缺字段）：默认开启，保证升级后行为与"默认开启"决策一致
//   - 显式 true：用户开启
//   - 显式 false：用户关闭，回退到系统时间戳
func (p Profile) UseDeviceTimestampEnabled() bool {
	if p.DaqP1604UseDeviceTimestamp == nil {
		return true
	}
	return *p.DaqP1604UseDeviceTimestamp
}

type DaqT1603HardwareConfig struct {
	ThermocoupleTypes string `json:"thermocoupleTypes"` // 16 chars, one per channel
	ChannelMask       string `json:"channelMask"`       // hex 0000-FFFF
	SamplingRate      int    `json:"samplingRate"`      // Hz
	BinaryFormat      bool   `json:"binaryFormat"`      // true=float32 LE, false=ASCII text
	AverageCount      int    `json:"averageCount"`      // 1-100
	TriggerMode       int    `json:"triggerMode"`       // 0=software, 2=hardware
	TriggerEdge       int    `json:"triggerEdge"`       // 0=rising, 1=falling, 2=change
	TriggerCount      int    `json:"triggerCount"`
	ShowTimestamp     bool   `json:"showTimestamp"`
	ShowSequence      bool   `json:"showSequence"`
	OpenCircuitCheck  string `json:"openCircuitCheck"` // hex mask
}

// DaqT1602HardwareConfig DAQ-T-1602 硬件配置（镜像 shared SDK daq/core.DaqT1602HardwareConfig）。
// T1602 固件固定 ~100ms 采集周期，无采样率/通道掩码/触发概念（spec-daq-t1602 Q5），
// 可配置项是 16 通道热电偶类型码 + 采集/保存频率（软件轮询间隔控制）。
type DaqT1602HardwareConfig struct {
	// TypeCodes 16 通道热电偶类型码（0=J 1=K 2=T 3=E 4=R 5=S 6=B 7=N）；
	// 索引 0~7 为卡1（Unit ID 1，Holding 200~207），索引 8~15 为卡2（Unit ID 2）。
	TypeCodes [16]uint8 `json:"typeCodes"`
	// SampleRateHz 采集/保存频率（Hz），范围 1~5；<=0 表示全速
	// （驱动按设备单帧上限 ~4.9Hz）。仅控制驱动轮询节奏，不写设备寄存器。
	SampleRateHz float64 `json:"sampleRateHz,omitempty"`
}

// DSA3217ScanConfig DSA3217 扫描配置（从 LIST S 读取）
type DSA3217ScanConfig struct {
	Avg    int    `json:"avg"`
	Period int    `json:"period"`
	Fps    string `json:"fps"`
	Unit   string `json:"unit"`
}

type Status struct {
	ID         string     `json:"id"`
	Name       string     `json:"name"`
	Type       Type       `json:"type"`
	Connection Connection `json:"connection"`
	Acquiring  bool       `json:"acquiring"`
	LastError  string     `json:"lastError,omitempty"`
}

type ScanResult struct {
	ID              string `json:"id"`
	Name            string `json:"name"`
	Type            Type   `json:"type"`
	Available       bool   `json:"available"`
	Address         string `json:"address,omitempty"`
	Port            int    `json:"port,omitempty"`
	MacAddress      string `json:"macAddress,omitempty"`
	SerialNumber    string `json:"serialNumber,omitempty"`
	FirmwareVersion string `json:"firmwareVersion,omitempty"`
	Model           string `json:"model,omitempty"`
	SubnetMask      string `json:"subnetMask,omitempty"`
	Gateway         string `json:"gateway,omitempty"`
	IpMode          string `json:"ipMode,omitempty"`
	TcpConnected    bool   `json:"tcpConnected,omitempty"`
	IpAssigned      bool   `json:"ipAssigned,omitempty"`
}

type DataPayload struct {
	DeviceID        string    `json:"deviceId"`
	DeviceType      Type      `json:"deviceType,omitempty"` // 设备类型，用于 sink 路由（如 CSV 按设备类型分派宽/长格式）
	DeviceName      string    `json:"deviceName,omitempty"` // 设备名（profile.Name），用于生成人类可读的文件名（比 UUID 友好）
	Timestamp       int64     `json:"timestamp"`
	DeviceTimestamp int64     `json:"deviceTimestamp,omitempty"` // 设备帧内时间戳（毫秒），仅 DAQ-P-1604 开启设备时间戳时有效
	Channels        []float64 `json:"channels"`
	ChannelIndices  []int     `json:"channelIndices"`
}

func (p *DataPayload) EnsureNonNilSlices() {
	if p.Channels == nil {
		p.Channels = make([]float64, 0)
	}
	if p.ChannelIndices == nil {
		p.ChannelIndices = make([]int, 0)
	}
}

type DataSink func(payload DataPayload)

func NowMs() int64 {
	return time.Now().UnixMilli()
}

// ---- v2 校零相关类型 ----

// CalibrationResult 单通道校零结果，由 CalibrationSampler 计算后返回。
type CalibrationResult struct {
	ChannelIndex int     `json:"channelIndex"`
	Offset       float64 `json:"offset"` // 已转为基单位的校零偏移
	Unit         string  `json:"unit"`   // 校零时的原始单位
	At           int64   `json:"at"`     // unix ms 时间戳
	SampleCount  int     `json:"sampleCount"`
}

type CalibrationRecord struct {
	ChannelIndex int     `json:"channelIndex"`
	Offset       float64 `json:"offset"`
	Unit         string  `json:"unit"`
	At           int64   `json:"at"`
	Enabled      bool    `json:"enabled"`
}

type CalibrationProgress struct {
	Running      bool  `json:"running"`
	ChannelIndex *int  `json:"channelIndex,omitempty"`
	ElapsedMs    int64 `json:"elapsedMs"`
	SampleCount  int   `json:"sampleCount"`
}

// CalibrationOffsets 设备级校零偏移快照，供 CalibrationApplier 高频读取。
// key = channelIndex, value = CalibrationOffset。
// 所有值已转为基单位（Pa/℃），CalibrationApplier 需按当前单位换算后再减偏移。
type CalibrationOffsets map[int]float64

// CalibrationDurationSec 校零采样时长（秒）。
// 前后端共享此常量定义，避免前端硬编码 5 处 "/5s" 与后端默认时长脱节。
// 前端通过 GET /api/v1/calibrationConfig 获取此值。
const CalibrationDurationSec = 5

// CurrentProfileVersion 当前 profile 格式版本号。
const CurrentProfileVersion = 2
