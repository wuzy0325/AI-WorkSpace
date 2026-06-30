package appcontext

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCopyDefaultProfilesCreatesValidDefaults(t *testing.T) {
	dir := t.TempDir()
	devicePath := filepath.Join(dir, "device-profiles.json")
	motionPath := filepath.Join(dir, "motion-profiles.json")

	if err := copyDefaultProfilesIfNeeded(devicePath, motionPath); err != nil {
		t.Fatalf("copyDefaultProfilesIfNeeded returned error: %v", err)
	}

	assertValidJSONArray(t, devicePath)
	assertValidJSONArray(t, motionPath)
}

func TestCopyDefaultProfilesRepairsInvalidMotionConfig(t *testing.T) {
	dir := t.TempDir()
	devicePath := filepath.Join(dir, "device-profiles.json")
	motionPath := filepath.Join(dir, "motion-profiles.json")
	invalid := `[{"id":"sim-motion-1","name":"broken}]`
	if err := os.WriteFile(motionPath, []byte(invalid), 0o600); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	if err := copyDefaultProfilesIfNeeded(devicePath, motionPath); err != nil {
		t.Fatalf("copyDefaultProfilesIfNeeded returned error: %v", err)
	}

	assertValidJSONArray(t, motionPath)
	if _, err := os.Stat(motionPath + ".invalid"); err != nil {
		t.Fatalf("expected invalid config backup: %v", err)
	}
}

func TestAppContextSimulatedAcquisitionPreservesPayloadSlices(t *testing.T) {
	ctx, err := NewAppContext(t.TempDir())
	if err != nil {
		t.Fatalf("NewAppContext returned error: %v", err)
	}
	if err := ctx.DeviceManager.Connect("sim-1"); err != nil {
		t.Fatalf("Connect returned error: %v", err)
	}
	if err := ctx.DeviceManager.StartAcquisition("sim-1"); err != nil {
		t.Fatalf("StartAcquisition returned error: %v", err)
	}
	defer ctx.DeviceManager.StopAcquisition("sim-1")

	deadline := time.After(700 * time.Millisecond)
	for {
		payload, ok := ctx.AcquisitionHub.GetLatestData("sim-1")
		if ok && len(payload.Channels) == 18 && len(payload.ChannelIndices) == 18 {
			if payload.ChannelIndices[1] != 1 || payload.ChannelIndices[17] != 17 {
				t.Fatalf("expected preserved channel indices, got %#v", payload.ChannelIndices)
			}
			return
		}

		select {
		case <-deadline:
			t.Fatal("timed out waiting for simulated acquisition data")
		case <-time.After(20 * time.Millisecond):
		}
	}
}

func assertValidJSONArray(t *testing.T, path string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s) returned error: %v", path, err)
	}
	var rows []map[string]any
	if err := json.Unmarshal(data, &rows); err != nil {
		t.Fatalf("expected valid JSON array in %s: %v", path, err)
	}
	if len(rows) == 0 {
		t.Fatalf("expected at least one profile in %s", path)
	}
}
