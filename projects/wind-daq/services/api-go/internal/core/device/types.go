package device

import (
	"fmt"
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

type DataSink func(payload DataPayload)

func NewDefaultProfile(id string, deviceType Type) Profile {
	profile := Profile{
		ID:           id,
		Name:         id,
		Type:         deviceType,
		Transport:    "tcp",
		SamplingRate: 20,
		AutoConnect:  true,
		BaudRate:     115200,
	}
	switch deviceType {
	case DeviceSimulated:
		profile.Channels = defaultSimulatedChannels()
	case DeviceDAQP1604:
		profile.Address = "192.168.3.101"
		profile.Port = 9000
		profile.Channels = defaultDAQP1604Channels()
	case DeviceDaqT1603:
		profile.Address = "192.168.3.101"
		profile.Port = 9000
		profile.Channels = defaultDaqT1603Channels()
		profile.DaqT1603Config = DaqT1603HardwareConfig{
			ThermocoupleType: "K",
			ColdJunction:     "internal",
			FilterHz:         50,
		}
	case DeviceDAQP1064Pre:
		profile.Address = "192.168.1.100"
		profile.Port = 5000
		profile.Channels = defaultDAQP1064PreChannels()
	case DeviceWTNPXI:
		profile.Address = "192.168.3.101"
		profile.Port = 9000
		profile.Channels = defaultWTNPXIChannels()
	case DeviceDSA3217:
		profile.Address = "192.168.1.254"
		profile.Port = 5000
		profile.Channels = defaultDSA3217Channels()
	}
	return profile
}

func NowMs() int64 {
	return time.Now().UnixMilli()
}

func defaultSimulatedChannels() []ChannelConfig {
	channels := make([]ChannelConfig, 4)
	for i := range channels {
		channels[i] = ChannelConfig{
			Index:     i,
			Name:      "CH" + string(rune('1'+i)),
			Enabled:   true,
			Unit:      "V",
			Precision: 3,
			RangeMin:  -10,
			RangeMax:  10,
		}
	}
	return channels
}

func defaultDaqT1603Channels() []ChannelConfig {
	channels := make([]ChannelConfig, 16)
	for i := range channels {
		channels[i] = ChannelConfig{
			Index:     i,
			Name:      fmt.Sprintf("TC%d", i+1),
			Enabled:   true,
			Unit:      "degC",
			Precision: 2,
		}
	}
	return channels
}

func defaultDAQP1604Channels() []ChannelConfig {
	channels := make([]ChannelConfig, 18)
	for i := 0; i < 16; i++ {
		channels[i] = ChannelConfig{
			Index:     i,
			Name:      fmt.Sprintf("CH%d", i+1),
			Enabled:   true,
			Unit:      "Pa",
			Precision: 2,
			RangeMin:  -5000,
			RangeMax:  5000,
		}
	}
	channels[16] = ChannelConfig{Index: 16, Name: "大气压", Enabled: false, Unit: "Pa", Precision: 2}
	channels[17] = ChannelConfig{Index: 17, Name: "大气温度", Enabled: false, Unit: "degC", Precision: 2}
	return channels
}

func defaultDAQP1064PreChannels() []ChannelConfig {
	channels := make([]ChannelConfig, 16)
	for i := range channels {
		channels[i] = ChannelConfig{
			Index:     i,
			Name:      fmt.Sprintf("CH%d", i+1),
			Enabled:   true,
			Unit:      "Pa",
			Precision: 2,
			RangeMin:  -5000,
			RangeMax:  5000,
		}
	}
	return channels
}

func defaultWTNPXIChannels() []ChannelConfig {
	names := []string{"球罐压力", "球罐总压", "球罐静压", "球罐温度1", "球罐温度2", "球罐温度3", "球罐温度4", "球罐温度5"}
	units := []string{"Pa", "Pa", "Pa", "degC", "degC", "degC", "degC", "degC"}
	channels := make([]ChannelConfig, 8)
	for i := range channels {
		channels[i] = ChannelConfig{
			Index:     i,
			Name:      names[i],
			Enabled:   true,
			Unit:      units[i],
			Precision: 2,
		}
	}
	return channels
}

func defaultDSA3217Channels() []ChannelConfig {
	channels := make([]ChannelConfig, 16)
	for i := range channels {
		channels[i] = ChannelConfig{
			Index:     i,
			Name:      fmt.Sprintf("CH%d", i+1),
			Enabled:   true,
			Unit:      "Pa",
			Precision: 2,
			RangeMin:  -5000,
			RangeMax:  5000,
		}
	}
	return channels
}
