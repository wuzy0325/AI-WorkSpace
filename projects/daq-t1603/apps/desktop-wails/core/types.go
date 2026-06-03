package core

import "time"

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

type T1603Config struct {
	ThermocoupleTypes string `json:"thermocoupleTypes"` // 16 chars, one per channel
	ChannelMask       string `json:"channelMask"`        // hex 0000-FFFF
	SamplingRate      int    `json:"samplingRate"`       // Hz
	AverageCount      int    `json:"averageCount"`       // 1-100
	ShowTimestamp     bool   `json:"showTimestamp"`
	ShowSequence      bool   `json:"showSequence"`
}

type ChannelConfig struct {
	Index            int     `json:"index"`
	Name             string  `json:"name"`
	Enabled          bool    `json:"enabled"`
	Unit             string  `json:"unit"`
	Color            string  `json:"color"`
	Precision        int     `json:"precision"`
	RangeMin         float64 `json:"rangeMin,omitempty"`
	RangeMax         float64 `json:"rangeMax,omitempty"`
	ThermocoupleType string  `json:"thermocoupleType,omitempty"`
}

type TemperatureProfile struct {
	ID           string          `json:"id"`
	Name         string          `json:"name"`
	Address      string          `json:"address"`
	Port         int             `json:"port"`
	SamplingRate int             `json:"samplingRate"`
	Channels     []ChannelConfig `json:"channels"`
	T1603Cfg     T1603Config     `json:"t1603Config"`
	CreatedAt    int64           `json:"createdAt,omitempty"`
}

type TemperatureSnapshot struct {
	DeviceID  string    `json:"deviceId"`
	Timestamp int64     `json:"timestamp"`
	Values    []float64 `json:"values"`
	Unit      string    `json:"unit"`
}

type DeviceState struct {
	Profile      TemperatureProfile `json:"profile"`
	Status       DeviceStatus       `json:"status"`
	Error        string             `json:"error"`
	ConnectedAt  int64              `json:"connectedAt"`
	AcquiringAt  int64              `json:"acquiringAt"`
	SamplingRate float64            `json:"samplingRate"`
}

type ScanResult struct {
	ID              string `json:"id"`
	Name            string `json:"name"`
	Address         string `json:"address"`
	Port            int    `json:"port"`
	MacAddress      string `json:"macAddress,omitempty"`
	SerialNumber    string `json:"serialNumber,omitempty"`
	FirmwareVersion string `json:"firmwareVersion,omitempty"`
}

func TimestampMs() int64 {
	return time.Now().UnixMilli()
}
