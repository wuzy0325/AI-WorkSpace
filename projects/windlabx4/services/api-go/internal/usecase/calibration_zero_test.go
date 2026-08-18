package usecase

import (
	"context"
	"math"
	"testing"
	"time"

	"windlabx4/services/api-go/internal/core/device"
)

func TestCalibrationSamplerUsesEveryRawFrame(t *testing.T) {
	stream := NewCalibrationStream()
	sampler := NewCalibrationSampler(stream, device.NewUnitConverter(), 40*time.Millisecond)
	channels := []device.ChannelConfig{{Index: 7, Enabled: true, Unit: "Pa"}}

	done := make(chan []device.CalibrationResult, 1)
	go func() {
		results, err := sampler.Sample(context.Background(), "dev-1", channels, nil, nil)
		if err != nil {
			t.Errorf("Sample returned error: %v", err)
		}
		done <- results
	}()

	time.Sleep(5 * time.Millisecond)
	for _, value := range []float64{2, 4, 6, 8} {
		stream.Publish(device.DataPayload{
			DeviceID:       "dev-1",
			Channels:       []float64{value},
			ChannelIndices: []int{7},
		})
	}

	results := <-done
	if len(results) != 1 {
		t.Fatalf("expected one calibration result, got %+v", results)
	}
	if math.Abs(results[0].Offset-5) > 1e-9 {
		t.Fatalf("expected raw-frame mean 5 Pa, got %v", results[0].Offset)
	}
	if results[0].SampleCount != 4 {
		t.Fatalf("expected all 4 raw frames, got %d", results[0].SampleCount)
	}
}

func TestCalibrationSamplerCanTargetOneChannel(t *testing.T) {
	stream := NewCalibrationStream()
	sampler := NewCalibrationSampler(stream, device.NewUnitConverter(), 30*time.Millisecond)
	target := 3
	channels := []device.ChannelConfig{
		{Index: 1, Enabled: true, Unit: "Pa"},
		{Index: 3, Enabled: false, Unit: "kPa"},
	}

	done := make(chan []device.CalibrationResult, 1)
	go func() {
		results, err := sampler.Sample(context.Background(), "dev-1", channels, &target, nil)
		if err != nil {
			t.Errorf("Sample returned error: %v", err)
		}
		done <- results
	}()

	time.Sleep(5 * time.Millisecond)
	stream.Publish(device.DataPayload{
		DeviceID:       "dev-1",
		Channels:       []float64{10, 2},
		ChannelIndices: []int{1, 3},
	})

	results := <-done
	if len(results) != 1 || results[0].ChannelIndex != target {
		t.Fatalf("expected only target channel %d, got %+v", target, results)
	}
	if results[0].Offset != 2000 {
		t.Fatalf("expected 2 kPa stored as 2000 Pa, got %v", results[0].Offset)
	}
}

func TestCalibrationSamplerCancellationDiscardsPartialSamples(t *testing.T) {
	stream := NewCalibrationStream()
	sampler := NewCalibrationSampler(stream, device.NewUnitConverter(), time.Second)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := sampler.Sample(ctx, "dev-1", []device.ChannelConfig{{Index: 0, Unit: "Pa"}}, nil, nil)
		done <- err
	}()
	time.Sleep(5 * time.Millisecond)
	stream.Publish(device.DataPayload{DeviceID: "dev-1", Channels: []float64{5}, ChannelIndices: []int{0}})
	cancel()
	if err := <-done; err != context.Canceled {
		t.Fatalf("expected context cancellation, got %v", err)
	}
}

func TestDataSinkPublishesRawFrameBeforeApplyingCalibration(t *testing.T) {
	stream := NewCalibrationStream()
	raw, unsubscribe := stream.Subscribe("dev-1", 1)
	defer unsubscribe()

	hub := NewAcquisitionHub(testPublisher{}, 20)
	applier := NewCalibrationApplier(device.NewUnitConverter())
	applier.UpdateOffsets("dev-1", device.CalibrationOffsets{0: 10})
	sink := NewDataSink(hub, nil, stream, applier, func(string) []device.ChannelConfig {
		return []device.ChannelConfig{{Index: 0, Unit: "Pa", CalibrationEnabled: true}}
	})

	sink(device.DataPayload{DeviceID: "dev-1", Channels: []float64{12}, ChannelIndices: []int{0}})

	select {
	case payload := <-raw:
		if payload.Channels[0] != 12 {
			t.Fatalf("expected calibration stream to receive raw value 12, got %v", payload.Channels[0])
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for raw calibration frame")
	}

	latest, ok := hub.GetLatestData("dev-1")
	if !ok || latest.Channels[0] != 2 {
		t.Fatalf("expected downstream hub to receive calibrated value 2, got %+v", latest)
	}
}

func TestCalibrationApplierNeverAppliesTemperatureOffset(t *testing.T) {
	applier := NewCalibrationApplier(device.NewUnitConverter())
	applier.UpdateOffsets("dev-1", device.CalibrationOffsets{0: 25})
	payload := device.DataPayload{DeviceID: "dev-1", Channels: []float64{25}, ChannelIndices: []int{0}}

	applier.Apply(&payload, []device.ChannelConfig{{
		Index: 0, Unit: "degC", SensorType: device.SensorTemperature, CalibrationEnabled: true,
	}})

	if payload.Channels[0] != 25 {
		t.Fatalf("temperature value must not be zero-calibrated, got %v", payload.Channels[0])
	}
}

func TestCalibrationSamplerFiltersTemperatureChannels(t *testing.T) {
	stream := NewCalibrationStream()
	sampler := NewCalibrationSampler(stream, device.NewUnitConverter(), 30*time.Millisecond)
	channels := []device.ChannelConfig{
		{Index: 0, Unit: "Pa", SensorType: device.SensorPressure},
		{Index: 1, Unit: "degC", SensorType: device.SensorTemperature},
	}

	done := make(chan []device.CalibrationResult, 1)
	go func() {
		results, err := sampler.Sample(context.Background(), "dev-1", channels, nil, nil)
		if err != nil {
			t.Errorf("Sample returned error: %v", err)
		}
		done <- results
	}()
	time.Sleep(5 * time.Millisecond)
	stream.Publish(device.DataPayload{DeviceID: "dev-1", Channels: []float64{10, 25}, ChannelIndices: []int{0, 1}})

	results := <-done
	if len(results) != 1 || results[0].ChannelIndex != 0 {
		t.Fatalf("expected pressure channel only, got %+v", results)
	}
}

func TestCalibrationSamplerRejectsTemperatureTarget(t *testing.T) {
	stream := NewCalibrationStream()
	sampler := NewCalibrationSampler(stream, device.NewUnitConverter(), 10*time.Millisecond)
	target := 1
	_, err := sampler.Sample(context.Background(), "dev-1", []device.ChannelConfig{{
		Index: 1, Unit: "degC", SensorType: device.SensorTemperature,
	}}, &target, nil)
	if err == nil || err.Error() != "channel 1 does not support zero calibration" {
		t.Fatalf("expected temperature calibration rejection, got %v", err)
	}
}

type testPublisher struct{}

func (testPublisher) Publish(string, any) {}
