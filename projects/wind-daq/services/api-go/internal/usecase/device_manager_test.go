package usecase

import (
	"testing"
	"time"

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
	return &fakeDevice{id: profile.ID}, nil
}

type fakeDevice struct {
	id       string
	conn     device.Connection
	dataSink device.DataSink
	emitDone chan struct{}
}

func (d *fakeDevice) ID() string { return d.id }

func (d *fakeDevice) Status() device.Status {
	return device.Status{ID: d.id, Connection: d.conn}
}

func (d *fakeDevice) Connect() error  { d.conn = device.ConnectionConnected; return nil }
func (d *fakeDevice) Disconnect() error {
	if d.emitDone != nil {
		close(d.emitDone)
		d.emitDone = nil
	}
	d.conn = device.ConnectionDisconnected
	return nil
}
func (d *fakeDevice) StartAcquisition() error {
	d.conn = device.ConnectionAcquiring
	d.emitDone = make(chan struct{})
	go func() {
		ticker := time.NewTicker(50 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-d.emitDone:
				return
			case <-ticker.C:
				if d.dataSink != nil {
					d.dataSink(device.DataPayload{DeviceID: d.id, Timestamp: time.Now().UnixMilli(), Channels: []float64{1, 2, 3, 4}, ChannelIndices: []int{0, 1, 2, 3}})
				}
			}
		}
	}()
	return nil
}
func (d *fakeDevice) StopAcquisition() error {
	if d.emitDone != nil {
		close(d.emitDone)
		d.emitDone = nil
	}
	d.conn = device.ConnectionConnected
	return nil
}
func (d *fakeDevice) SetDataSink(sink device.DataSink) { d.dataSink = sink }

type fakeScanner struct {
	results []device.ScanResult
}

func (s fakeScanner) Scan() ([]device.ScanResult, error) {
	return append([]device.ScanResult(nil), s.results...), nil
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

func TestDeviceManagerScansDevices(t *testing.T) {
	manager, err := NewDeviceManager(&memoryProfileStore{}, simulatedFactory{}, nil)
	if err != nil {
		t.Fatalf("NewDeviceManager returned error: %v", err)
	}
	manager.SetScanner(fakeScanner{results: []device.ScanResult{
		{ID: "sim-1", Name: "Simulator 1", Type: device.DeviceSimulated, Available: true},
	}})

	results, err := manager.ScanDevices()
	if err != nil {
		t.Fatalf("ScanDevices returned error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected one scan result, got %d", len(results))
	}
	if results[0].ID != "sim-1" || !results[0].Available {
		t.Fatalf("unexpected scan result: %+v", results[0])
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

func TestDeviceManagerDisconnectsConnectedDevice(t *testing.T) {
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

	if err := manager.Disconnect("sim-1"); err != nil {
		t.Fatalf("Disconnect returned error: %v", err)
	}
	if _, ok := manager.GetStatus("sim-1"); ok {
		t.Fatal("expected disconnected device to be removed from active statuses")
	}
}

func TestDeviceManagerDeleteProfileDisconnectsAndPersistsRemoval(t *testing.T) {
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

	if err := manager.DeleteProfile("sim-1"); err != nil {
		t.Fatalf("DeleteProfile returned error: %v", err)
	}
	if len(manager.GetProfiles()) != 0 {
		t.Fatalf("expected no profiles, got %d", len(manager.GetProfiles()))
	}
	if len(store.profiles) != 0 {
		t.Fatalf("expected persisted profile removal, got %d saved profiles", len(store.profiles))
	}
	if _, ok := manager.GetStatus("sim-1"); ok {
		t.Fatal("expected deleted profile to disconnect active device")
	}
}

func TestDeviceManagerSetUnitUpdatesAllChannelsAndPersists(t *testing.T) {
	profile := device.NewDefaultProfile("sim-1", device.DeviceSimulated)
	store := &memoryProfileStore{profiles: []device.Profile{profile}}
	manager, err := NewDeviceManager(store, simulatedFactory{}, nil)
	if err != nil {
		t.Fatalf("NewDeviceManager returned error: %v", err)
	}

	if err := manager.SetUnit("sim-1", "kPa"); err != nil {
		t.Fatalf("SetUnit returned error: %v", err)
	}

	saved := store.profiles[0]
	for _, channel := range saved.Channels {
		if channel.Unit != "kPa" {
			t.Fatalf("expected channel %d unit kPa, got %q", channel.Index, channel.Unit)
		}
	}
}

func TestDeviceManagerDaqT1603ConfigPersistsProfileConfig(t *testing.T) {
	profile := device.NewDefaultProfile("temp-1", device.DeviceDaqT1603)
	store := &memoryProfileStore{profiles: []device.Profile{profile}}
	manager, err := NewDeviceManager(store, simulatedFactory{}, nil)
	if err != nil {
		t.Fatalf("NewDeviceManager returned error: %v", err)
	}

	config := device.DaqT1603HardwareConfig{
		ThermocoupleType: "K",
		ColdJunction:     "internal",
		FilterHz:         50,
	}
	if err := manager.ApplyDaqT1603Config("temp-1", config); err != nil {
		t.Fatalf("ApplyDaqT1603Config returned error: %v", err)
	}

	got, err := manager.GetDaqT1603Config("temp-1")
	if err != nil {
		t.Fatalf("GetDaqT1603Config returned error: %v", err)
	}
	if got != config {
		t.Fatalf("expected config %+v, got %+v", config, got)
	}
	if store.profiles[0].DaqT1603Config != config {
		t.Fatalf("expected persisted config %+v, got %+v", config, store.profiles[0].DaqT1603Config)
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
