package usecase

import (
	"testing"

	"wind-daq/services/api-go/internal/core/device"
	"wind-daq/services/api-go/internal/core/traversal"
)

type delayedLatestDataReader struct {
	calls int
}

func (r *delayedLatestDataReader) GetLatestData(deviceID string) (device.DataPayload, bool) {
	r.calls++
	if r.calls <= 2 {
		return device.DataPayload{}, false
	}
	return device.DataPayload{
		DeviceID:       deviceID,
		Channels:       []float64{12.5},
		ChannelIndices: []int{0},
	}, true
}

func (r *delayedLatestDataReader) GetLatestTimestamp(_ string) (int64, bool) { return 0, false }

func TestCollectAveragedSamplesWaitsForDelayedFirstData(t *testing.T) {
	reader := &delayedLatestDataReader{}
	manager := NewTraversalManager(reader, nil, nil, nil, nil)
	manager.status = traversal.Status{TaskID: "trav-1"}

	values, err := manager.collectAveragedSamples("trav-1", "dev-1", []int{0}, 2)
	if err != nil {
		t.Fatalf("collectAveragedSamples returned error: %v", err)
	}
	if got := values[0]; got != 12.5 {
		t.Fatalf("averaged channel 0 = %v, want 12.5", got)
	}
	if reader.calls != 4 {
		t.Fatalf("GetLatestData calls = %d, want 4", reader.calls)
	}
}
