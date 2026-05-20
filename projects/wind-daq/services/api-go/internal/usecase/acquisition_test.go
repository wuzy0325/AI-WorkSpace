package usecase

import (
	"testing"

	"wind-daq/services/api-go/internal/core/device"
)

type capturePublisher struct {
	channel string
	data    any
}

func (p *capturePublisher) Publish(channel string, data any) {
	p.channel = channel
	p.data = data
}

func TestAcquisitionHubStoresLatestPayloadByDevice(t *testing.T) {
	publisher := &capturePublisher{}
	hub := NewAcquisitionHub(publisher, 20)
	hub.OnData(device.DataPayload{DeviceID: "dev-1", Channels: []float64{1}})
	hub.OnData(device.DataPayload{DeviceID: "dev-1", Channels: []float64{2}})

	latest, ok := hub.GetLatestData("dev-1")
	if !ok {
		t.Fatal("expected latest data")
	}
	if latest.Channels[0] != 2 {
		t.Fatalf("expected latest channel value 2, got %v", latest.Channels[0])
	}
}

func TestAcquisitionHubRejectsInvalidPublishRate(t *testing.T) {
	publisher := &capturePublisher{}
	hub := NewAcquisitionHub(publisher, 20)

	if err := hub.SetPublishRate(0); err == nil {
		t.Fatal("expected invalid publish rate error")
	}
	if err := hub.SetPublishRate(101); err == nil {
		t.Fatal("expected invalid publish rate error")
	}
	if err := hub.SetPublishRate(50); err != nil {
		t.Fatalf("expected 50 Hz to be valid: %v", err)
	}
}
