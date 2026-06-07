package backend

import (
	"context"
	"testing"

	"daq-t1603/adapters/config"
	"daq-t1603/adapters/hardware"
	"daq-t1603/adapters/logging"
	"daq-t1603/adapters/recording"
	"daq-t1603/core"
	"daq-t1603/usecase"
)

func newTestApp(t *testing.T) *App {
	t.Helper()
	dir := t.TempDir()
	cfg := config.NewJSONConfigStore(dir + "/profiles.json")
	dev := hardware.NewSimulatedAdapter()
	rec := recording.NewCSVRecorder()
	logWriter := logging.NewLogFileWriter()
	duc := usecase.NewDeviceUsecase(dev, cfg, hardware.NewSimulatedScanner())
	ruc := usecase.NewRecordingUsecase(rec)
	luc := usecase.NewLogUsecase(logWriter)
	app := NewApp(duc, ruc, luc, "")
	return app
}

func TestGetProfiles_Empty(t *testing.T) {
	app := newTestApp(t)
	profiles := app.GetProfiles()
	if len(profiles) != 0 {
		t.Fatalf("expected 0 profiles, got %d", len(profiles))
	}
}

func TestUpsertAndGetProfiles(t *testing.T) {
	app := newTestApp(t)

	p := core.TemperatureProfile{ID: "dev1", Name: "Test", Address: "x", Port: 1}
	if err := app.UpsertProfile(p); err != nil {
		t.Fatalf("UpsertProfile: %v", err)
	}

	profiles := app.GetProfiles()
	if len(profiles) != 1 || profiles[0].ID != "dev1" {
		t.Fatalf("expected 1 profile (dev1), got %v", profiles)
	}
}

func TestDeleteProfile(t *testing.T) {
	app := newTestApp(t)
	_ = app.UpsertProfile(core.TemperatureProfile{ID: "dev1", Name: "t1", Address: "x", Port: 1})
	if err := app.DeleteProfile("dev1"); err != nil {
		t.Fatalf("DeleteProfile: %v", err)
	}
	profiles := app.GetProfiles()
	if len(profiles) != 0 {
		t.Fatalf("expected 0 profiles after delete")
	}
}

func TestConnectDisconnect(t *testing.T) {
	app := newTestApp(t)
	_ = app.UpsertProfile(core.TemperatureProfile{ID: "dev1", Name: "t1", Address: "x", Port: 1})

	if err := app.Connect("dev1"); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	state, ok := app.GetStatus("dev1")
	if !ok {
		t.Fatal("expected status for dev1")
	}
	if state.Status != core.StatusConnected {
		t.Fatalf("expected Connected, got %v", state.Status)
	}

	if err := app.Disconnect("dev1"); err != nil {
		t.Fatalf("Disconnect: %v", err)
	}
}

func TestConnectNonexistent(t *testing.T) {
	app := newTestApp(t)
	if err := app.Connect("nonexistent"); err == nil {
		t.Fatal("expected error connecting nonexistent profile")
	}
}

func TestApplyConfigNotConnected(t *testing.T) {
	app := newTestApp(t)
	_ = app.UpsertProfile(core.TemperatureProfile{ID: "dev1"})
	err := app.ApplyConfig("dev1", core.T1603Config{
		ThermocoupleTypes: "KKKKKKKKKKKKKKKK", ChannelMask: "FFFF", SamplingRate: 10, AverageCount: 4,
	})
	if err == nil {
		t.Fatal("expected error applying config to disconnected device")
	}
}

func TestApplyConfig(t *testing.T) {
	app := newTestApp(t)
	_ = app.UpsertProfile(core.TemperatureProfile{ID: "dev1", Name: "t1", Address: "x", Port: 1})
	_ = app.Connect("dev1")

	err := app.ApplyConfig("dev1", core.T1603Config{
		ThermocoupleTypes: "KKKKKKKKKKKKKKKK", ChannelMask: "FFFF", SamplingRate: 10, AverageCount: 4,
	})
	if err != nil {
		t.Fatalf("ApplyConfig: %v", err)
	}
}

func TestScanDevices(t *testing.T) {
	app := newTestApp(t)
	results, err := app.ScanDevices()
	if err != nil {
		t.Fatalf("ScanDevices: %v", err)
	}
	if len(results) < 2 {
		t.Fatalf("expected at least 2 devices from simulated scanner, got %d", len(results))
	}
}

func TestRecordingLifecycle(t *testing.T) {
	app := newTestApp(t)
	dir := t.TempDir()

	session := app.GetRecordingStatus()
	if session.Status != core.RecordingIdle {
		t.Fatal("expected idle recording initially")
	}

	if err := app.StartRecording(dir, "test"); err != nil {
		t.Fatalf("StartRecording: %v", err)
	}

	session = app.GetRecordingStatus()
	if session.Status != core.RecordingActive {
		t.Fatal("expected active recording after start")
	}

	if err := app.StopRecording(); err != nil {
		t.Fatalf("StopRecording: %v", err)
	}

	session = app.GetRecordingStatus()
	if session.Status != core.RecordingIdle {
		t.Fatal("expected idle after stop")
	}
}

func TestRelayStreamRecordsEverySnapshot(t *testing.T) {
	app := newTestApp(t)
	dir := t.TempDir()

	if err := app.StartRecording(dir, "relay"); err != nil {
		t.Fatalf("StartRecording: %v", err)
	}

	ch := make(chan core.TemperatureSnapshot)
	done := make(chan struct{})
	go func() {
		defer close(done)
		app.relayStream(context.Background(), "dev1", ch)
	}()

	const samples = 1000
	for i := 0; i < samples; i++ {
		values := make([]float64, 16)
		values[0] = float64(i)
		ch <- core.TemperatureSnapshot{
			DeviceID:  "dev1",
			Timestamp: int64(i + 1),
			Values:    values,
			Unit:      "°C",
		}
	}
	close(ch)
	<-done

	if err := app.StopRecording(); err != nil {
		t.Fatalf("StopRecording: %v", err)
	}
	session := app.GetRecordingStatus()
	if session.SnapshotCount != samples {
		t.Fatalf("expected %d recorded snapshots, got %d", samples, session.SnapshotCount)
	}
}

func TestAcquisitionFlow(t *testing.T) {
	app := newTestApp(t)
	_ = app.UpsertProfile(core.TemperatureProfile{ID: "dev1", Name: "t1", Address: "x", Port: 1})
	_ = app.Connect("dev1")

	if err := app.StartAcquisition("dev1"); err != nil {
		t.Fatalf("StartAcquisition: %v", err)
	}

	state, ok := app.GetStatus("dev1")
	if !ok {
		t.Fatal("expected status")
	}
	if state.Status != core.StatusAcquiring {
		t.Fatalf("expected Acquiring after start, got %v", state.Status)
	}

	if err := app.StopAcquisition("dev1"); err != nil {
		t.Fatalf("StopAcquisition: %v", err)
	}
}

func TestDoubleAcquisition(t *testing.T) {
	app := newTestApp(t)
	_ = app.UpsertProfile(core.TemperatureProfile{ID: "dev1", Name: "t1", Address: "x", Port: 1})
	_ = app.Connect("dev1")
	_ = app.StartAcquisition("dev1")
	err := app.StartAcquisition("dev1")
	if err == nil {
		t.Fatal("expected error on double acquisition")
	}
	app.StopAcquisition("dev1")
}

func TestStopAcquisitionIdle(t *testing.T) {
	app := newTestApp(t)
	_ = app.UpsertProfile(core.TemperatureProfile{ID: "dev1", Name: "t1", Address: "x", Port: 1})
	err := app.StopAcquisition("dev1")
	if err != nil {
		t.Fatalf("StopAcquisition on idle device should be noop: %v", err)
	}
}
