package usecase

import (
	"testing"
	"time"

	"wind-daq/services/api-go/internal/adapters/hardware"
	"wind-daq/services/api-go/internal/core/device"
	"wind-daq/services/api-go/internal/ports"
)

type memoryProfileStore struct {
	profiles []device.Profile
}

func (s *memoryProfileStore) LoadProfiles() ([]device.Profile, error) {
	return append([]device.Profile(nil), s.profiles...), nil
}

func (s *memoryProfileStore) SaveProfiles(profiles []device.Profile) error {
	s.profiles = append([]device.Profile(nil), profiles...)
	return nil
}

type simulatedFactory struct{}

func (simulatedFactory) Create(profile device.Profile) (ports.Device, error) {
	return hardware.NewSimulatedDevice(profile), nil
}

func TestDeviceManagerLoadsProfilesFromStore(t *testing.T) {
	store := &memoryProfileStore{profiles: []device.Profile{
		device.NewDefaultProfile("sim-1", device.DeviceSimulated),
	}}
	manager, err := NewDeviceManager(store, simulatedFactory{}, nil)
	if err != nil {
		t.Fatalf("NewDeviceManager returned error: %v", err)
	}

	profiles := manager.GetProfiles()
	if len(profiles) != 1 {
		t.Fatalf("expected one profile, got %d", len(profiles))
	}
	if profiles[0].ID != "sim-1" {
		t.Fatalf("expected profile sim-1, got %q", profiles[0].ID)
	}
}

func TestDeviceManagerUpsertProfilePersistsProfile(t *testing.T) {
	store := &memoryProfileStore{}
	manager, err := NewDeviceManager(store, simulatedFactory{}, nil)
	if err != nil {
		t.Fatalf("NewDeviceManager returned error: %v", err)
	}

	profile := device.NewDefaultProfile("sim-1", device.DeviceSimulated)
	profile.Name = "Simulator 1"
	if err := manager.UpsertProfile(profile); err != nil {
		t.Fatalf("UpsertProfile returned error: %v", err)
	}

	if len(store.profiles) != 1 {
		t.Fatalf("expected one saved profile, got %d", len(store.profiles))
	}
	if store.profiles[0].Name != "Simulator 1" {
		t.Fatalf("expected saved name Simulator 1, got %q", store.profiles[0].Name)
	}
}

func TestDeviceManagerConnectsProfileAndReportsStatus(t *testing.T) {
	store := &memoryProfileStore{profiles: []device.Profile{
		device.NewDefaultProfile("sim-1", device.DeviceSimulated),
	}}
	manager, err := NewDeviceManager(store, simulatedFactory{}, nil)
	if err != nil {
		t.Fatalf("NewDeviceManager returned error: %v", err)
	}

	if err := manager.Connect("sim-1"); err != nil {
		t.Fatalf("Connect returned error: %v", err)
	}

	status, ok := manager.GetStatus("sim-1")
	if !ok {
		t.Fatal("expected status for connected device")
	}
	if status.Connection != device.ConnectionConnected {
		t.Fatalf("expected connected status, got %q", status.Connection)
	}
}

func TestDeviceManagerAcquisitionFeedsAcquisitionHub(t *testing.T) {
	store := &memoryProfileStore{profiles: []device.Profile{
		device.NewDefaultProfile("sim-1", device.DeviceSimulated),
	}}
	hub := NewAcquisitionHub(&capturePublisher{}, 20)
	manager, err := NewDeviceManager(store, simulatedFactory{}, hub.OnData)
	if err != nil {
		t.Fatalf("NewDeviceManager returned error: %v", err)
	}
	if err := manager.Connect("sim-1"); err != nil {
		t.Fatalf("Connect returned error: %v", err)
	}
	if err := manager.StartAcquisition("sim-1"); err != nil {
		t.Fatalf("StartAcquisition returned error: %v", err)
	}
	defer manager.StopAcquisition("sim-1")

	deadline := time.After(700 * time.Millisecond)
	for {
		if payload, ok := hub.GetLatestData("sim-1"); ok && len(payload.Channels) > 0 {
			return
		}
		select {
		case <-deadline:
			t.Fatal("timed out waiting for acquisition hub data")
		case <-time.After(20 * time.Millisecond):
		}
	}
}
