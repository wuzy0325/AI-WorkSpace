package config

import (
	"os"
	"path/filepath"
	"testing"

	"windlabx4/services/api-go/internal/core/device"
)

func TestFileProfileStoreSavesAndLoadsProfiles(t *testing.T) {
	path := filepath.Join(t.TempDir(), "profiles.json")
	store := NewFileProfileStore(path)
	profile := NewDefaultProfile("sim-1", device.DeviceSimulated)
	profile.Name = "Simulator 1"

	if err := store.SaveProfiles([]device.Profile{profile}); err != nil {
		t.Fatalf("SaveProfiles returned error: %v", err)
	}

	reloaded := NewFileProfileStore(path)
	profiles, err := reloaded.LoadProfiles()
	if err != nil {
		t.Fatalf("LoadProfiles returned error: %v", err)
	}
	if len(profiles) != 1 {
		t.Fatalf("expected one profile, got %d", len(profiles))
	}
	if profiles[0].ID != "sim-1" || profiles[0].Name != "Simulator 1" {
		t.Fatalf("unexpected profile loaded: %+v", profiles[0])
	}
}

func TestFileProfileStoreLoadsEmptyListWhenFileMissing(t *testing.T) {
	store := NewFileProfileStore(filepath.Join(t.TempDir(), "missing", "profiles.json"))

	profiles, err := store.LoadProfiles()
	if err != nil {
		t.Fatalf("LoadProfiles returned error: %v", err)
	}
	if len(profiles) != 0 {
		t.Fatalf("expected no profiles, got %d", len(profiles))
	}
}

func TestFileProfileStoreReportsInvalidJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "profiles.json")
	if err := os.WriteFile(path, []byte("not-json"), 0o600); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}
	store := NewFileProfileStore(path)

	if _, err := store.LoadProfiles(); err == nil {
		t.Fatal("expected invalid JSON error")
	}
}

// TestFileProfileStoreRoundTripsDaqT1602Profile 验证 DAQ-T-1602 profile（含
// daqT1602Config.typeCodes）经 JSON 持久化后完整回读。
func TestFileProfileStoreRoundTripsDaqT1602Profile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "profiles.json")
	store := NewFileProfileStore(path)
	profile := NewDefaultProfile("temp-t1602", device.DeviceDaqT1602)
	profile.DaqT1602Config.TypeCodes[0] = 1
	profile.DaqT1602Config.TypeCodes[15] = 3

	if err := store.SaveProfiles([]device.Profile{profile}); err != nil {
		t.Fatalf("SaveProfiles returned error: %v", err)
	}

	reloaded := NewFileProfileStore(path)
	profiles, err := reloaded.LoadProfiles()
	if err != nil {
		t.Fatalf("LoadProfiles returned error: %v", err)
	}
	if len(profiles) != 1 {
		t.Fatalf("expected one profile, got %d", len(profiles))
	}
	got := profiles[0]
	if got.Type != device.DeviceDaqT1602 {
		t.Fatalf("expected type DAQ-T-1602, got %q", got.Type)
	}
	if got.DaqT1602Config != profile.DaqT1602Config {
		t.Fatalf("expected daqT1602Config %+v, got %+v", profile.DaqT1602Config, got.DaqT1602Config)
	}
}

// TestFileProfileStoreLoadsLegacyDaqT1602ProfileWithoutConfig 向后兼容：
// 缺 daqT1602Config 字段的旧 JSON 可正常加载（零值，由 NormalizeProfile 回填默认值）。
func TestFileProfileStoreLoadsLegacyDaqT1602ProfileWithoutConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "profiles.json")
	legacy := `[{"id":"temp-t1602","name":"T1602","type":"DAQ-T-1602","address":"192.168.3.201","port":502,"samplingRate":20,"channels":[]}]`
	if err := os.WriteFile(path, []byte(legacy), 0o600); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}
	store := NewFileProfileStore(path)

	profiles, err := store.LoadProfiles()
	if err != nil {
		t.Fatalf("LoadProfiles returned error: %v", err)
	}
	if len(profiles) != 1 || profiles[0].Type != device.DeviceDaqT1602 {
		t.Fatalf("unexpected profiles loaded: %+v", profiles)
	}
	if profiles[0].DaqT1602Config != (device.DaqT1602HardwareConfig{}) {
		t.Fatalf("expected zero daqT1602Config for legacy profile, got %+v", profiles[0].DaqT1602Config)
	}
}
