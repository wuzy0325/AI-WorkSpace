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
	SamplingRate   int                    `json:"samplingRate"`
	Channels       []ChannelConfig        `json:"channels"`
	Address        string                 `json:"address,omitempty"`
	DaqT1603Config DaqT1603HardwareConfig `json:"daqT1603Config,omitempty"`
}

type DaqT1603HardwareConfig struct {
	ThermocoupleType string `json:"thermocoupleType"`
	ColdJunction     string `json:"coldJunction"`
	FilterHz         int    `json:"filterHz"`
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
		SamplingRate: 20,
	}
	switch deviceType {
	case DeviceSimulated:
		profile.Channels = defaultSimulatedChannels()
	case DeviceDaqT1603:
		profile.Channels = defaultDaqT1603Channels()
		profile.DaqT1603Config = DaqT1603HardwareConfig{
			ThermocoupleType: "K",
			ColdJunction:     "internal",
			FilterHz:         50,
		}
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
