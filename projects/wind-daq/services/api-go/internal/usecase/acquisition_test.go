package usecase

import (
	"sync"
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
	// maxPublishHz 已从 100 提到 500，覆盖 1kHz 设备的 1/2 采样率直送场景
	if err := hub.SetPublishRate(501); err == nil {
		t.Fatal("expected invalid publish rate error")
	}
	if err := hub.SetPublishRate(50); err != nil {
		t.Fatalf("expected 50 Hz to be valid: %v", err)
	}
	if err := hub.SetPublishRate(500); err != nil {
		t.Fatalf("expected 500 Hz to be valid: %v", err)
	}
}

func TestAcquisitionHubStoresRecentPayloadHistoryUpToCapacity(t *testing.T) {
	hub := NewAcquisitionHubWithHistoryCapacity(&capturePublisher{}, 20, 3)
	for i := 1; i <= 5; i++ {
		hub.OnData(device.DataPayload{DeviceID: "dev-1", Channels: []float64{float64(i)}})
	}

	history := hub.GetRecentData("dev-1", 10)
	if len(history) != 3 {
		t.Fatalf("expected 3 recent payloads, got %d", len(history))
	}
	want := []float64{3, 4, 5}
	for i, payload := range history {
		if payload.Channels[0] != want[i] {
			t.Fatalf("expected history[%d] channel value %.0f, got %.0f", i, want[i], payload.Channels[0])
		}
	}
}

func TestAcquisitionHubLimitsRecentPayloadHistoryResult(t *testing.T) {
	hub := NewAcquisitionHubWithHistoryCapacity(&capturePublisher{}, 20, 5)
	for i := 1; i <= 4; i++ {
		hub.OnData(device.DataPayload{DeviceID: "dev-1", Channels: []float64{float64(i)}})
	}

	history := hub.GetRecentData("dev-1", 2)
	if len(history) != 2 {
		t.Fatalf("expected 2 recent payloads, got %d", len(history))
	}
	if history[0].Channels[0] != 3 || history[1].Channels[0] != 4 {
		t.Fatalf("expected most recent values 3 and 4, got %+v", history)
	}
}

func TestAcquisitionHubAllowsConcurrentHistoryReads(t *testing.T) {
	hub := NewAcquisitionHubWithHistoryCapacity(&capturePublisher{}, 20, 20)
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(2)
		go func(i int) {
			defer wg.Done()
			hub.OnData(device.DataPayload{DeviceID: "dev-1", Channels: []float64{float64(i)}})
		}(i)
		go func() {
			defer wg.Done()
			_ = hub.GetRecentData("dev-1", 10)
		}()
	}
	wg.Wait()

	history := hub.GetRecentData("dev-1", 100)
	if len(history) > 20 {
		t.Fatalf("expected history length to stay within capacity, got %d", len(history))
	}
}

func TestAcquisitionHubPublishesPayloadToSubscribers(t *testing.T) {
	hub := NewAcquisitionHubWithHistoryCapacity(&capturePublisher{}, 20, 3)
	subscription, unsubscribe := hub.Subscribe("dev-1", 1)
	defer unsubscribe()

	hub.OnData(device.DataPayload{DeviceID: "dev-1", Channels: []float64{42}})

	select {
	case payload := <-subscription:
		if payload.Channels[0] != 42 {
			t.Fatalf("expected subscriber payload value 42, got %.0f", payload.Channels[0])
		}
	default:
		t.Fatal("expected subscriber to receive payload")
	}
}

func TestAcquisitionHubDoesNotPublishOtherDevicePayloadsToSubscriber(t *testing.T) {
	hub := NewAcquisitionHubWithHistoryCapacity(&capturePublisher{}, 20, 3)
	subscription, unsubscribe := hub.Subscribe("dev-1", 1)
	defer unsubscribe()

	hub.OnData(device.DataPayload{DeviceID: "dev-2", Channels: []float64{42}})

	select {
	case payload := <-subscription:
		t.Fatalf("expected no payload, got %+v", payload)
	default:
	}
}
