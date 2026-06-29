package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadProfilesNormalizesAxesEnabled(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "motion-profiles.json")
	data := []byte(`[
  {
    "id": "legacy",
    "name": "Legacy Controller",
    "type": "SIMULATED",
    "axes": [
      { "name": "X", "enabled": false, "kind": "LINEAR", "maxSpeed": 10 },
      { "name": "U", "enabled": false, "kind": "ROTARY", "maxSpeed": 10 }
    ]
  }
]`)
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	store := NewFileMotionProfileStore(path)
	profiles, err := store.LoadProfiles()
	if err != nil {
		t.Fatalf("LoadProfiles returned error: %v", err)
	}
	if len(profiles) != 1 {
		t.Fatalf("expected 1 profile, got %d", len(profiles))
	}

	for _, axis := range profiles[0].Axes {
		if !axis.Enabled {
			t.Fatalf("expected axis %s to be normalized enabled", axis.Name)
		}
	}
}
