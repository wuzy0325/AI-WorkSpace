package config

import (
	"os"
	"path/filepath"
	"testing"

	"daq-t1603/core"
)

func newTempStore(t *testing.T) (*JSONConfigStore, string) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "profiles.json")
	return NewJSONConfigStore(path), dir
}

func TestLoadProfiles_FileNotExist(t *testing.T) {
	store, _ := newTempStore(t)
	profiles, err := store.LoadProfiles()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(profiles) != 0 {
		t.Fatalf("expected 0 profiles, got %d", len(profiles))
	}
}

func TestSaveAndLoad(t *testing.T) {
	store, _ := newTempStore(t)

	p := core.TemperatureProfile{
		ID:      "dev1",
		Name:    "Test Device",
		Address: "192.168.1.100",
		Port:    9000,
		Channels: []core.ChannelConfig{
			{Index: 0, Name: "CH1", Enabled: true, Unit: "°C", Color: "#ff0000"},
		},
		T1603Cfg: core.T1603Config{
			ThermocoupleType: "K",
			ChannelMask:      "FFFF",
			SamplingRate:     10,
			AverageCount:     4,
		},
	}

	if err := store.SaveProfile(p); err != nil {
		t.Fatalf("SaveProfile: %v", err)
	}

	profiles, err := store.LoadProfiles()
	if err != nil {
		t.Fatalf("LoadProfiles: %v", err)
	}
	if len(profiles) != 1 {
		t.Fatalf("expected 1 profile, got %d", len(profiles))
	}
	if profiles[0].ID != "dev1" {
		t.Fatalf("expected ID dev1, got %s", profiles[0].ID)
	}
}

func TestUpdateProfile(t *testing.T) {
	store, _ := newTempStore(t)

	p1 := core.TemperatureProfile{ID: "dev1", Name: "Old Name", Address: "x", Port: 1}
	if err := store.SaveProfile(p1); err != nil {
		t.Fatal(err)
	}

	p2 := core.TemperatureProfile{ID: "dev1", Name: "New Name", Address: "y", Port: 2}
	if err := store.SaveProfile(p2); err != nil {
		t.Fatal(err)
	}

	profiles, _ := store.LoadProfiles()
	if len(profiles) != 1 {
		t.Fatalf("expected 1 profile after update, got %d", len(profiles))
	}
	if profiles[0].Name != "New Name" {
		t.Fatalf("expected 'New Name', got '%s'", profiles[0].Name)
	}
}

func TestDeleteProfile(t *testing.T) {
	store, _ := newTempStore(t)
	_ = store.SaveProfile(core.TemperatureProfile{ID: "dev1", Name: "A", Address: "x", Port: 1})
	_ = store.SaveProfile(core.TemperatureProfile{ID: "dev2", Name: "B", Address: "y", Port: 2})

	if err := store.DeleteProfile("dev1"); err != nil {
		t.Fatalf("DeleteProfile: %v", err)
	}

	profiles, _ := store.LoadProfiles()
	if len(profiles) != 1 || profiles[0].ID != "dev2" {
		t.Fatalf("expected 1 profile (dev2), got %v", profiles)
	}
}

func TestDeleteNonexistent(t *testing.T) {
	store, _ := newTempStore(t)
	err := store.DeleteProfile("nonexistent")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

func TestFileReadError(t *testing.T) {
	store := NewJSONConfigStore(filepath.Join(os.TempDir(), "nonexistent_dir_"+t.Name(), "profiles.json"))
	_, err := store.LoadProfiles()
	if err != nil {
		t.Fatalf("expected nil for missing file, got %v", err)
	}
}

func TestConcurrentWrites(t *testing.T) {
	store, _ := newTempStore(t)
	done := make(chan bool, 10)

	for i := 0; i < 10; i++ {
		i := i
		go func() {
			id := "dev"
			id += string(rune('0' + i))
			_ = store.SaveProfile(core.TemperatureProfile{ID: id, Name: "test", Address: "x", Port: i})
			done <- true
		}()
	}

	for i := 0; i < 10; i++ {
		<-done
	}

	profiles, _ := store.LoadProfiles()
	if len(profiles) != 10 {
		t.Fatalf("expected 10 profiles, got %d", len(profiles))
	}
}
