package hardware

import (
	"testing"
	"time"

	"wind-daq/services/api-go/internal/core/device"
	windaqconfig "wind-daq/services/api-go/internal/adapters/config"
	"wind-daq/services/api-go/internal/ports"
)

func TestSimulatedDeviceImplementsDevicePort(t *testing.T) {
	var _ ports.Device = NewSimulatedDevice(windaqconfig.NewDefaultProfile("sim-1", device.DeviceSimulated))
}

func TestSimulatedDeviceEmitsDataWhenAcquiring(t *testing.T) {
	dev := NewSimulatedDevice(windaqconfig.NewDefaultProfile("sim-1", device.DeviceSimulated))
	payloads := make(chan device.DataPayload, 1)
	dev.SetDataSink(func(payload device.DataPayload) {
		payloads <- payload
	})

	if err := dev.Connect(); err != nil {
		t.Fatalf("Connect returned error: %v", err)
	}
	if err := dev.StartAcquisition(); err != nil {
		t.Fatalf("StartAcquisition returned error: %v", err)
	}
	defer dev.StopAcquisition()

	select {
	case payload := <-payloads:
		if payload.DeviceID != "sim-1" {
			t.Fatalf("expected device id sim-1, got %q", payload.DeviceID)
		}
		if len(payload.Channels) != 18 {
			t.Fatalf("expected 18 channels, got %d", len(payload.Channels))
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timed out waiting for simulated data")
	}
}
