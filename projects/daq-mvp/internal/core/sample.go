package core

// SampleBatch is a fixed-size batch of multi-channel floating-point samples.
// Values are row-major: [ch0_s0, ch1_s0, ..., ch0_s1, ch1_s1, ...]
type SampleBatch struct {
	DeviceID        string    `json:"deviceId"`
	SequenceStart   uint64    `json:"sequenceStart"`
	SampleRateHz    float64   `json:"sampleRateHz"`
	ChannelCount    int       `json:"channelCount"`
	SampleCount     int       `json:"sampleCount"`
	Values          []float32 `json:"values"`
	HostTimestampMs int64     `json:"hostTimestampMs"`
}

// UiSampleFrame is the lightweight DTO pushed to the frontend via Wails Events.
type UiSampleFrame struct {
	DeviceID          string    `json:"deviceId"`
	SequenceStart     uint64    `json:"sequenceStart"`
	SampleCount       int       `json:"sampleCount"`
	ChannelIDs        []int     `json:"channels"`
	LatestValues      []float32 `json:"latestValues"`
	SamplesPerChannel int       `json:"samplesPerChannel"`
	HostTimestampMs   int64     `json:"hostTimestampMs"`
}
