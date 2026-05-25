package appcontext

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
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
