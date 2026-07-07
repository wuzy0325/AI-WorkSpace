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

type Connection string

const (
	ConnectionDisconnected Connection = "Disconnected"
	ConnectionConnected    Connection = "Connected"
	ConnectionAcquiring    Connection = "Acquiring"
	ConnectionError        Connection = "Error"
)

type ChannelConfig struct {
	Index      int                `json:"index"`
	Name       string             `json:"name"`
	Enabled    bool               `json:"enabled"`
	Unit       string             `json:"unit"`
	Precision  int                `json:"precision"`
	RangeMin   float64            `json:"rangeMin,omitempty"`
	RangeMax   float64            `json:"rangeMax,omitempty"`
	TareOffset float64            `json:"tareOffset,omitempty"`
	// SensorType 通道传感器类型（pressure/temperature），仅 DAQ-P-1603 使用。
	// 旧 profile（含 DAQ-P-1604 / DAQ-T-1603 / 历史 SIMULATED）无此字段，
	// 反序列化时由 UnmarshalJSON 兜底为 "pressure"，
	// 保证读路径拿到的 ChannelConfig 永远有合法 SensorType 值，避免业务层到处判空。
	SensorType ChannelSensorType `json:"sensorType,omitempty"`
}



// Profile 设备配置档案
// 注意：硬件特定的默认值生成已迁移到 adapters/config 包，
// core 层只保留类型定义，不包含基础设施知识
type Profile struct {
	ID                         string                 `json:"id"`
	Name                       string                 `json:"name"`
	Type                       Type                   `json:"type"`
	Transport                  string                 `json:"transport,omitempty"`
	Address                    string                 `json:"address,omitempty"`
	Port                       int                    `json:"port,omitempty"`
	SerialPort                 string                 `json:"serialPort,omitempty"`
	BaudRate                   int                    `json:"baudRate,omitempty"`
	AutoConnect                bool                   `json:"autoConnect,omitempty"`
	MacAddress                 string                 `json:"macAddress,omitempty"`
	SamplingRate               int                    `json:"samplingRate"`
	Channels                   []ChannelConfig        `json:"channels"`
	DaqP1604UseDeviceTimestamp *bool                  `json:"daqP1604UseDeviceTimestamp,omitempty"`
	DaqT1603Config             DaqT1603HardwareConfig `json:"daqT1603Config,omitempty"`
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
