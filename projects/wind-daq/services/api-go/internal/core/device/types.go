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
	ID             string                 `json:"id"`
	Name           string                 `json:"name"`
	Type           Type                   `json:"type"`
	Transport      string                 `json:"transport,omitempty"`
	Address        string                 `json:"address,omitempty"`
	Port           int                    `json:"port,omitempty"`
	SerialPort     string                 `json:"serialPort,omitempty"`
	BaudRate       int                    `json:"baudRate,omitempty"`
	AutoConnect    bool                   `json:"autoConnect,omitempty"`
	MacAddress     string                 `json:"macAddress,omitempty"`
	SamplingRate   int                    `json:"samplingRate"`
	Channels       []ChannelConfig        `json:"channels"`
	DaqT1603Config DaqT1603HardwareConfig `json:"daqT1603Config,omitempty"`
}

type DaqT1603HardwareConfig struct {
	ThermocoupleType string `json:"thermocoupleType"`
	ColdJunction     string `json:"coldJunction"`
	FilterHz         int    `json:"filterHz"`
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
}

type DataPayload struct {
	DeviceID       string    `json:"deviceId"`
	Timestamp      int64     `json:"timestamp"`
	Channels       []float64 `json:"channels"`
	ChannelIndices []int     `json:"channelIndices"`
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
