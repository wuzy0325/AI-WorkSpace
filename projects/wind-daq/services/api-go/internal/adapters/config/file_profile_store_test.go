package config

import (
	"os"
	"path/filepath"
	"testing"

	"wind-daq/services/api-go/internal/core/device"
)

func TestFileProfileStoreSavesAndLoadsProfiles(t *testing.T) {
	path := filepath.Join(t.TempDir(), "profiles.json")
	store := NewFileProfileStore(path)
	profile := device.NewDefaultProfile("sim-1", device.DeviceSimulated)
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
