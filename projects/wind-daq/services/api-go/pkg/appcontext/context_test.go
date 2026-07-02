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

	if productionBuild {
		// 生产构建不预置 sim-1 默认 profile，避免向客户暴露开发样例。
		assertJSONArrayLen(t, devicePath, 0)
	} else {
		// 开发构建必须写入 sim-1 默认 profile，否则前端首次启动会丢失模拟器设备。
		assertJSONArrayLen(t, devicePath, 1)
	}
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
	if productionBuild {
		t.Skip("production builds do not seed a simulated DAQ profile")
	}

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
}

func assertJSONArrayLen(t *testing.T, path string, want int) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s) returned error: %v", path, err)
	}
	var rows []map[string]any
	if err := json.Unmarshal(data, &rows); err != nil {
		t.Fatalf("expected valid JSON array in %s: %v", path, err)
	}
	if len(rows) != want {
		t.Fatalf("expected %d profiles in %s, got %d", want, path, len(rows))
	}
}
