package config

import (
	"os"
	"path/filepath"
	"testing"

	"wind-daq/services/api-go/internal/core/motion"
)

func TestFileMotionProfileStoreLoadsProfilesFromFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "motion-profiles.json")
	content := `[{"id":"motion-1","name":"Motion 1","type":"SIMULATED","address":"127.0.0.1","port":9000}]`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	store := NewFileMotionProfileStore(path)
	profiles, err := store.LoadProfiles()
	if err != nil {
		t.Fatalf("LoadProfiles returned error: %v", err)
	}
	if len(profiles) != 1 || profiles[0].ID != "motion-1" {
		t.Fatalf("unexpected profiles loaded: %+v", profiles)
	}
}

func TestFileMotionProfileStoreLoadsDefaultsWhenFileMissing(t *testing.T) {
	store := NewFileMotionProfileStore(filepath.Join(t.TempDir(), "missing", "motion-profiles.json"))

	profiles, err := store.LoadProfiles()
	if err != nil {
		t.Fatalf("LoadProfiles returned error: %v", err)
	}
	if len(profiles) == 0 {
		t.Fatal("expected default motion profiles")
	}
}

func TestFileMotionProfileStoreSavesAndLoadsProfiles(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "motion-profiles.json")
	store := NewFileMotionProfileStore(path)
	profile := motion.MotionControllerProfile{
		ID:      "motion-1",
		Name:    "Motion 1",
		Type:    motion.ControllerTypeSimulated,
		Address: "127.0.0.1",
		Port:    9000,
	}

	if err := store.SaveProfiles([]motion.MotionControllerProfile{profile}); err != nil {
		t.Fatalf("SaveProfiles returned error: %v", err)
	}

	reloaded := NewFileMotionProfileStore(path)
	profiles, err := reloaded.LoadProfiles()
	if err != nil {
		t.Fatalf("LoadProfiles returned error: %v", err)
	}
	if len(profiles) != 1 || profiles[0].ID != "motion-1" {
		t.Fatalf("unexpected profiles loaded: %+v", profiles)
	}
}

func TestFileMotionProfileStoreReportsInvalidJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "motion-profiles.json")
	if err := os.WriteFile(path, []byte("not-json"), 0o600); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	store := NewFileMotionProfileStore(path)
	if _, err := store.LoadProfiles(); err == nil {
		t.Fatal("expected invalid JSON error")
	}
}
