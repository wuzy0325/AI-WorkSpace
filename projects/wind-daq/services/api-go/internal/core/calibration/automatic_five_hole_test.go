package calibration

import (
	"fmt"
	"testing"
)

type fakeCalibrationRuntime struct {
	values map[string]float64
}

func (f fakeCalibrationRuntime) GetChannelValue(deviceID string, channelIndex int) (float64, bool) {
	v, ok := f.values[fmt.Sprintf("%s:%d", deviceID, channelIndex)]
	return v, ok
}

func (f fakeCalibrationRuntime) MoveToPosition(_ MotionAxisConfig, _ float64) error { return nil }

func (f fakeCalibrationRuntime) WaitForMotionComplete() error { return nil }

// 注意：原 TestProbeChannelUnmarshalNestedFrontendShape 已迁移到
// adapters/config/calibration_config_decoder_test.go（TestDecodeCalibrationConfig_NestedFrontendShape），
// 因为 core 层不再做字节级 I/O，解码逻辑由 adapters/config 层的 DecodeCalibrationConfig 负责。

func TestAutomaticFiveHoleCalibrationUsesConfiguredProbeChannels(t *testing.T) {
	config := Config{
		TaskID:          "cal-1",
		Type:            string(TypeFiveHole),
		SamplesPerPoint: 1,
		ProbeChannels: []ProbeChannel{
			{Role: "fiveHole.p1", Name: "P1", DeviceID: "dev-1", ChannelIndex: 1, Enabled: true},
			{Role: "fiveHole.p2", Name: "P2", DeviceID: "dev-1", ChannelIndex: 2, Enabled: true},
			{Role: "fiveHole.p3", Name: "P3", DeviceID: "dev-1", ChannelIndex: 3, Enabled: true},
			{Role: "fiveHole.p4", Name: "P4", DeviceID: "dev-1", ChannelIndex: 4, Enabled: true},
			{Role: "fiveHole.p5", Name: "P5", DeviceID: "dev-1", ChannelIndex: 5, Enabled: true},
			{Role: "fiveHole.pAtm", Name: "Patm", DeviceID: "dev-1", ChannelIndex: 16, Enabled: true},
			{Role: "fiveHole.tAtm", Name: "Tatm", DeviceID: "dev-1", ChannelIndex: 17, Enabled: true},
		},
		Points: []CalPoint{{ID: 1, Coordinates: map[string]float64{"α": 0, "β": 0}}},
	}
	runtime := fakeCalibrationRuntime{values: map[string]float64{
		"dev-1:1":  10,
		"dev-1:2":  20,
		"dev-1:3":  30,
		"dev-1:4":  40,
		"dev-1:5":  50,
		"dev-1:16": 101325,
		"dev-1:17": 25,
	}}

	engine := NewAutomaticCalibration(config, nil, runtime, nil)
	if err := engine.Start(NewFiveHoleAlgorithm()); err != nil {
		t.Fatalf("start calibration: %v", err)
	}

	points := engine.GetDataPoints()
	if len(points) != 1 {
		t.Fatalf("expected 1 point, got %d", len(points))
	}
	fiveHolePoint, ok := points[0].(*FiveHoleDataPoint)
	if !ok {
		t.Fatalf("expected FiveHoleDataPoint, got %T", points[0])
	}
	if fiveHolePoint.RawData.P1 != 10 || fiveHolePoint.RawData.P5 != 50 || fiveHolePoint.RawData.PAtm != 101325 {
		t.Fatalf("unexpected raw data: %+v", fiveHolePoint.RawData)
	}
}

func TestAutomaticCalibrationInvokesOnDataPointForEachPoint(t *testing.T) {
	config := Config{
		TaskID:          "cal-realtime",
		Type:            string(TypeFiveHole),
		SamplesPerPoint: 1,
		ProbeChannels: []ProbeChannel{
			{Role: "fiveHole.p1", DeviceID: "dev-1", ChannelIndex: 1, Enabled: true},
			{Role: "fiveHole.p2", DeviceID: "dev-1", ChannelIndex: 2, Enabled: true},
			{Role: "fiveHole.p3", DeviceID: "dev-1", ChannelIndex: 3, Enabled: true},
			{Role: "fiveHole.p4", DeviceID: "dev-1", ChannelIndex: 4, Enabled: true},
			{Role: "fiveHole.p5", DeviceID: "dev-1", ChannelIndex: 5, Enabled: true},
			{Role: "fiveHole.pAtm", DeviceID: "dev-1", ChannelIndex: 16, Enabled: true},
			{Role: "fiveHole.tAtm", DeviceID: "dev-1", ChannelIndex: 17, Enabled: true},
		},
		Points: []CalPoint{
			{ID: 1, Coordinates: map[string]float64{"α": 0, "β": 0}},
			{ID: 2, Coordinates: map[string]float64{"α": 5, "β": 0}},
		},
	}
	runtime := fakeCalibrationRuntime{values: map[string]float64{
		"dev-1:1": 10, "dev-1:2": 20, "dev-1:3": 30, "dev-1:4": 40, "dev-1:5": 50,
		"dev-1:16": 101325, "dev-1:17": 25,
	}}

	var received []DataPoint
	sink := func(dp DataPoint) { received = append(received, dp) }
	engine := NewAutomaticCalibration(config, nil, runtime, sink)
	if err := engine.Start(NewFiveHoleAlgorithm()); err != nil {
		t.Fatalf("start calibration: %v", err)
	}
	if len(received) != 2 {
		t.Fatalf("expected onDataPoint called 2 times, got %d", len(received))
	}
}
