package device

import "time"

type Type string

const (
	DeviceSimulated Type = "SIMULATED"
)

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
}

type Profile struct {
	ID           string          `json:"id"`
	Name         string          `json:"name"`
	Type         Type            `json:"type"`
	SamplingRate int             `json:"samplingRate"`
	Channels     []ChannelConfig `json:"channels"`
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
	if deviceType == DeviceSimulated {
		profile.Channels = defaultSimulatedChannels()
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
