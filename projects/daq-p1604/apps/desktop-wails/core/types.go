package core

import "time"

// DeviceStatus 设备连接状态
type DeviceStatus int

const (
	StatusDisconnected DeviceStatus = iota
	StatusConnected
	StatusAcquiring
	StatusError
)

func (s DeviceStatus) String() string {
	switch s {
	case StatusDisconnected:
		return "Disconnected"
	case StatusConnected:
		return "Connected"
	case StatusAcquiring:
		return "Acquiring"
	case StatusError:
		return "Error"
	default:
		return "Unknown"
	}
}

// P1604Config DAQ-P-1604 设备硬件配置
type P1604Config struct {
	SamplingRate int    `json:"samplingRate"` // 采样周期（毫秒），最小 10ms，由采样频率换算得出
	Unit         string `json:"unit"`         // 压力单位：psi, Pa, kPa, MPa, kgf/cm²
	AutoConnect  bool   `json:"autoConnect"`  // 启动时自动连接
	Precision    int    `json:"precision"`    // 全局默认显示精度（小数位数 0-6），单通道精度未设置时回退到此值
}

// ChannelConfig 通道配置
type ChannelConfig struct {
	Index     int     `json:"index"`
	Name      string  `json:"name"`
	Enabled   bool    `json:"enabled"`
	Unit      string  `json:"unit"`
	Color     string  `json:"color"`
	Precision int     `json:"precision"`
	RangeMin  float64 `json:"rangeMin,omitempty"`
	RangeMax  float64 `json:"rangeMax,omitempty"`
}

// PressureProfile 压力采集设备配置档案
type PressureProfile struct {
	ID           string          `json:"id"`
	Name         string          `json:"name"`
	Address      string          `json:"address"`
	Port         int             `json:"port"`
	SamplingRate int             `json:"samplingRate"`
	Channels     []ChannelConfig `json:"channels"`
	P1604Cfg     P1604Config     `json:"p1604Config"`
	CreatedAt    int64           `json:"createdAt,omitempty"`
}

// PressureSnapshot 压力采集数据快照（18 通道）
type PressureSnapshot struct {
	DeviceID          string    `json:"deviceId"`
	Timestamp         int64     `json:"timestamp"`
	HardwareTimestamp float64   `json:"hardwareTimestamp,omitempty"`
	Values            []float64 `json:"values"` // CH1-CH16 压力 + CH17 大气压力 + CH18 大气温度
	Unit              string    `json:"unit"`
}

// DeviceState 设备运行状态
type DeviceState struct {
	Profile      PressureProfile `json:"profile"`
	Status       DeviceStatus    `json:"status"`
	StatusText   string          `json:"statusText"`
	Error        string          `json:"error"`
	ConnectedAt  int64           `json:"connectedAt"`
	AcquiringAt  int64           `json:"acquiringAt"`
	SamplingRate float64         `json:"samplingRate"`
}

// ScanResult 设备扫描结果
type ScanResult struct {
	ID              string `json:"id"`
	Name            string `json:"name"`
	Address         string `json:"address"`
	Port            int    `json:"port"`
	MacAddress      string `json:"macAddress,omitempty"`
	SerialNumber    string `json:"serialNumber,omitempty"`
	FirmwareVersion string `json:"firmwareVersion,omitempty"`
}

func (s *DeviceState) SetStatus(status DeviceStatus) {
	s.Status = status
	s.StatusText = status.String()
}

// TimestampMs 返回当前时间的毫秒时间戳
func TimestampMs() int64 {
	return time.Now().UnixMilli()
}
