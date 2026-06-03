package integration

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"daq-t1603/adapters/config"
	"daq-t1603/adapters/hardware"
	"daq-t1603/adapters/recording"
	"daq-t1603/core"
	"daq-t1603/usecase"
)

func TestSimulatedFullFlow(t *testing.T) {
	dir := t.TempDir()

	cfgStore := config.NewJSONConfigStore(filepath.Join(dir, "profiles.json"))
	dev := hardware.NewSimulatedAdapter()
	rec := recording.NewCSVRecorder()
	duc := usecase.NewDeviceUsecase(dev, cfgStore)
	ruc := usecase.NewRecordingUsecase(rec)

	profile := core.TemperatureProfile{
		ID:      "sim1",
		Name:    "模拟设备",
		Address: "0.0.0.0",
		Port:    0,
		Channels: func() []core.ChannelConfig {
			ch := make([]core.ChannelConfig, 16)
			for i := range ch {
				ch[i] = core.ChannelConfig{Index: i, Name: "CH", Enabled: true, Unit: "°C", Color: "#000"}
			}
			return ch
		}(),
	}

	if err := duc.UpsertProfile(profile); err != nil {
		t.Fatalf("UpsertProfile: %v", err)
	}

	profiles := duc.GetProfiles()
	if len(profiles) != 1 {
		t.Fatalf("expected 1 profile, got %d", len(profiles))
	}

	if err := duc.Connect("sim1"); err != nil {
		t.Fatalf("Connect: %v", err)
	}

	state, ok := duc.GetStatus("sim1")
	if !ok || state.Status != core.StatusConnected {
		t.Fatalf("expected Connected status, got %v", state.Status)
	}

	cfg := core.T1603Config{ThermocoupleTypes: "KKKKKKKKKKKKKKKK", ChannelMask: "FFFF", SamplingRate: 10, AverageCount: 4}
	if err := duc.ApplyConfig("sim1", cfg); err != nil {
		t.Fatalf("ApplyConfig: %v", err)
	}

	ch, err := duc.StartAcquisition("sim1")
	if err != nil {
		t.Fatalf("StartAcquisition: %v", err)
	}

	state, _ = duc.GetStatus("sim1")
	if state.Status != core.StatusAcquiring {
		t.Fatalf("expected Acquiring, got %v", state.Status)
	}

	recDir := filepath.Join(dir, "recordings")
	if err := ruc.Start(recDir, "integration-test"); err != nil {
		t.Fatalf("Recording start: %v", err)
	}

	timeout := time.After(2 * time.Second)
	received := 0
	for received < 3 {
		select {
		case snap, ok := <-ch:
			if !ok {
				t.Fatal("channel closed unexpectedly")
			}
			if len(snap.Values) != 16 {
				t.Fatalf("expected 16 values, got %d", len(snap.Values))
			}
			if err := ruc.Write(snap); err != nil {
				t.Fatalf("Recording write: %v", err)
			}
			received++
		case <-timeout:
			t.Fatalf("timeout waiting for snapshots, got %d", received)
		}
	}

	if err := ruc.Stop(); err != nil {
		t.Fatalf("Recording stop: %v", err)
	}

	recSession := ruc.Status()
	if recSession.SnapshotCount != 3 {
		t.Fatalf("expected 3 recorded snapshots, got %d", recSession.SnapshotCount)
	}

	files, _ := filepath.Glob(filepath.Join(recDir, "integration-test_*.csv"))
	if len(files) == 0 {
		t.Fatal("no CSV file found")
	}
	data, err := os.ReadFile(files[0])
	if err != nil || len(data) == 0 {
		t.Fatal("CSV file is empty or unreadable")
	}

	if err := duc.StopAcquisition("sim1"); err != nil {
		t.Fatalf("StopAcquisition: %v", err)
	}

	if err := duc.Disconnect("sim1"); err != nil {
		t.Fatalf("Disconnect: %v", err)
	}

	state, _ = duc.GetStatus("sim1")
	if state.Status != core.StatusDisconnected {
		t.Fatalf("expected Disconnected, got %v", state.Status)
	}

	if err := duc.DeleteProfile("sim1"); err != nil {
		t.Fatalf("DeleteProfile: %v", err)
	}
	profiles = duc.GetProfiles()
	if len(profiles) != 0 {
		t.Fatalf("expected 0 profiles after delete")
	}
}
