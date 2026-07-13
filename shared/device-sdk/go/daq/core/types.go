package core

import (
	"time"
)

type Type string

const (
	DeviceDAQP1604 Type = "DAQ-P-1604"
	DeviceDaqT1603 Type = "DAQ-T-1603"
	// DeviceDAQP1603 DAQ-P-1603 16 通道通用 AI 采集设备。
	// 与 DAQ-P-1604 并列存在：1604 为压力专用、1603 每通道可接入压力或温度传感器，
	// 因此在 ChannelConfig 上扩展 SensorType 字段以区分通道用途。
	DeviceDAQP1603 Type = "DAQ-P-1603"
)

// ChannelSensorType 通道传感器类型枚举。
// 设计原因：DAQ-P-1603 每通道物理接口既可接压力传感器也可接温度传感器，
// 单位换算与 CSV 表头生成需要按通道类型分支，使用强类型避免裸字符串散落各处。
type ChannelSensorType string

const (
	// SensorPressure 压力通道（Pa/kPa/MPa/mmH2O）
	SensorPressure ChannelSensorType = "pressure"
	// SensorTemperature 温度通道（℃/℉）
	SensorTemperature ChannelSensorType = "temperature"
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
	// SensorType 通道传感器类型（pressure/temperature）。
	// 仅 DAQ-P-1603 使用：每通道物理接口可接入压力或温度传感器。
	// 旧 profile（DAQ-P-1604 / DAQ-T-1603）无此字段，反序列化时由 UnmarshalJSON 默认填充 "pressure"，
	// 保证向后兼容——历史设备本就以压力通道为主，零值空字符串不应进入业务逻辑。
	SensorType ChannelSensorType `json:"sensorType,omitempty"`

	// ---- v2 校零字段（与 wind-daq device.ChannelConfig 同步）----
	CalibrationOffset  float64 `json:"calibrationOffset,omitempty"`
	CalibrationUnit    string  `json:"calibrationUnit,omitempty"`
	CalibrationAt      int64   `json:"calibrationAt,omitempty"`
	// CalibrationEnabled 不使用 omitempty：false 是用户主动设置的合法值
	// （DAQ-P-1603 逐通道可关闭校零应用），加 omitempty 会在序列化时丢字段，
	// 导致 HTTP 回读 / 持久化 JSON 后前端拿到 undefined，UI 复选框跳回默认勾选状态，
	// 造成"配置无法保存"的观感。必须与 wind-daq device.ChannelConfig 保持一致。
	CalibrationEnabled bool `json:"calibrationEnabled"`
}



type Profile struct {
	ID             string                 `json:"id"`
	Name           string                 `json:"name"`
	Type           Type                   `json:"type"`
	Transport      string                 `json:"transport,omitempty"`
	Address        string                 `json:"address,omitempty"`
	Port           int                    `json:"port,omitempty"`
	SamplingRate   int                    `json:"samplingRate"`
	Channels       []ChannelConfig        `json:"channels"`
	DaqT1603Config DaqT1603HardwareConfig `json:"daqT1603Config,omitempty"`
}

type DaqT1603HardwareConfig struct {
	ThermocoupleTypes string `json:"thermocoupleTypes"` // 16 chars, one per channel, e.g. "KKKKKKKKKKKKKKKK"
	ChannelMask       string `json:"channelMask"`        // hex 0000-FFFF
	SamplingRate      int    `json:"samplingRate"`       // Hz
	BinaryFormat      bool   `json:"binaryFormat"`       // true=float32 LE, false=ASCII text
	AverageCount      int    `json:"averageCount"`       // 1-100
	TriggerMode       int    `json:"triggerMode"`        // 0=software, 2=hardware
	TriggerEdge       int    `json:"triggerEdge"`        // 0=rising, 1=falling, 2=change
	TriggerCount      int    `json:"triggerCount"`
	ShowTimestamp     bool   `json:"showTimestamp"`
	ShowSequence      bool   `json:"showSequence"`
	OpenCircuitCheck  string `json:"openCircuitCheck"` // hex mask
}

type Status struct {
	ID         string     `json:"id"`
	Name       string     `json:"name"`
	Type       Type       `json:"type"`
	Connection Connection `json:"connection"`
	Acquiring  bool       `json:"acquiring"`
	LastError  string     `json:"lastError,omitempty"`
}

type DataPayload struct {
	DeviceID         string    `json:"deviceId"`
	Timestamp        int64     `json:"timestamp"`
	HardwareTimestamp float64   `json:"hardwareTimestamp,omitempty"` // 设备硬件时间戳（秒.纳秒），0 表示无时间戳
	Channels         []float64 `json:"channels"`
	ChannelIndices   []int     `json:"channelIndices"`
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
