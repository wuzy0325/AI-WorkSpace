package device

import (
	"time"
)

type Type string

const (
	DeviceSimulated   Type = "SIMULATED"
	DeviceDAQP1604    Type = "DAQ-P-1604"
	DeviceDaqT1603    Type = "DAQ-T-1603"
	DeviceDAQP1064Pre Type = "DAQ-P-1064Pre"
	DeviceWTNPXI      Type = "WTN_PXI"
	DeviceDSA3217     Type = "DSA3217"
)

type Connection string

const (
	ConnectionDisconnected Connection = "Disconnected"
	ConnectionConnected    Connection = "Connected"
	ConnectionAcquiring    Connection = "Acquiring"
	ConnectionError        Connection = "Error"
)

type ChannelConfig struct {
	Index      int     `json:"index"`
	Name       string  `json:"name"`
	Enabled    bool    `json:"enabled"`
	Unit       string  `json:"unit"`
	Precision  int     `json:"precision"`
	RangeMin   float64 `json:"rangeMin,omitempty"`
	RangeMax   float64 `json:"rangeMax,omitempty"`
	TareOffset float64 `json:"tareOffset,omitempty"`
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
