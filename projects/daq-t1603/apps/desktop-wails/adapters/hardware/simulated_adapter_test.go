package hardware

import (
	"testing"
	"time"

	"daq-t1603/core"
)

func TestSimulatedConnect(t *testing.T) {
	ad := NewSimulatedAdapter()
	profile := core.TemperatureProfile{ID: "sim1", Name: "Sim", Address: "0.0.0.0", Port: 0}

	if err := ad.Connect(profile); err != nil {
		t.Fatalf("Connect: %v", err)
	}

	state, ok := ad.Status("sim1")
	if !ok {
		t.Fatal("expected status for sim1")
	}
	if state.Status != core.StatusConnected {
		t.Fatalf("expected Connected, got %v", state.Status)
	}
}

func TestSimulatedDoubleConnect(t *testing.T) {
	ad := NewSimulatedAdapter()
	profile := core.TemperatureProfile{ID: "sim1"}
	if err := ad.Connect(profile); err != nil {
		t.Fatal(err)
	}
	if err := ad.Connect(profile); err == nil {
		t.Fatal("expected error on double connect")
	}
}

func TestSimulatedStartStopAcquisition(t *testing.T) {
	ad := NewSimulatedAdapter()
	_ = ad.Connect(core.TemperatureProfile{ID: "sim1"})

	ch, err := ad.StartAcquisition("sim1")
	if err != nil {
		t.Fatalf("StartAcquisition: %v", err)
	}

	select {
	case snap := <-ch:
		if len(snap.Values) != 16 {
			t.Fatalf("expected 16 values, got %d", len(snap.Values))
		}
	case <-time.After(1 * time.Second):
		t.Fatal("timeout waiting for first snapshot")
	}

	if err := ad.StopAcquisition("sim1"); err != nil {
		t.Fatalf("StopAcquisition: %v", err)
	}
}

func TestSimulatedAcquisitionClosedOnStop(t *testing.T) {
	ad := NewSimulatedAdapter()
	_ = ad.Connect(core.TemperatureProfile{ID: "sim1"})
	ch, _ := ad.StartAcquisition("sim1")
	_ = ad.StopAcquisition("sim1")

	_, open := <-ch
	if open {
		t.Fatal("expected channel to be closed after stop")
	}
}

func TestSimulatedStatusNotConnected(t *testing.T) {
	ad := NewSimulatedAdapter()
	_, ok := ad.Status("nonexistent")
	if ok {
		t.Fatal("expected false for nonexistent device")
	}
}

func TestSimulatedDisconnect(t *testing.T) {
	ad := NewSimulatedAdapter()
	_ = ad.Connect(core.TemperatureProfile{ID: "sim1"})
	if err := ad.Disconnect("sim1"); err != nil {
		t.Fatalf("Disconnect: %v", err)
	}
	state, ok := ad.Status("sim1")
	if !ok {
		t.Fatal("expected status after disconnect")
	}
	if state.Status != core.StatusDisconnected {
		t.Fatalf("expected Disconnected, got %v", state.Status)
	}
}

func TestSimulatedApplyConfig(t *testing.T) {
	ad := NewSimulatedAdapter()
	_ = ad.Connect(core.TemperatureProfile{ID: "sim1"})

	cfg := core.T1603Config{ThermocoupleType: "K", ChannelMask: "FFFF", SamplingRate: 10, AverageCount: 4}
	if err := ad.ApplyConfig("sim1", cfg); err != nil {
		t.Fatalf("ApplyConfig: %v", err)
	}
}

func TestSimulatedAcquisitionFromNotConnected(t *testing.T) {
	ad := NewSimulatedAdapter()
	_, err := ad.StartAcquisition("nonexistent")
	if err == nil {
		t.Fatal("expected error starting acquisition on not connected device")
	}
}
