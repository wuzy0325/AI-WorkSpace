package usecase

import (
	"fmt"
	"sync"

	"wind-daq/services/api-go/internal/core/device"
	"wind-daq/services/api-go/internal/ports"
)

type DeviceManager struct {
	mu       sync.RWMutex
	profiles []device.Profile
	devices  map[string]ports.Device
	store    ports.ProfileStore
	factory  ports.DeviceFactory
	dataSink device.DataSink
}

func NewDeviceManager(store ports.ProfileStore, factory ports.DeviceFactory, dataSink device.DataSink) (*DeviceManager, error) {
	profiles, err := store.LoadProfiles()
	if err != nil {
		return nil, err
	}
	return &DeviceManager{
		profiles: profiles,
		devices:  make(map[string]ports.Device),
		store:    store,
		factory:  factory,
		dataSink: dataSink,
	}, nil
}

func (m *DeviceManager) GetProfiles() []device.Profile {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return append([]device.Profile(nil), m.profiles...)
}

func (m *DeviceManager) UpsertProfile(profile device.Profile) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	for i := range m.profiles {
		if m.profiles[i].ID == profile.ID {
			m.profiles[i] = profile
			return m.store.SaveProfiles(m.profiles)
		}
	}
	m.profiles = append(m.profiles, profile)
	return m.store.SaveProfiles(m.profiles)
}

func (m *DeviceManager) Connect(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.devices[id]; ok {
		return nil
	}
	profile, ok := m.findProfileLocked(id)
	if !ok {
		return fmt.Errorf("device profile not found: %s", id)
	}
	dev, err := m.factory.Create(profile)
	if err != nil {
		return err
	}
	if m.dataSink != nil {
		dev.SetDataSink(m.dataSink)
	}
	if err := dev.Connect(); err != nil {
		return err
	}
	m.devices[id] = dev
	return nil
}

func (m *DeviceManager) StartAcquisition(id string) error {
	m.mu.RLock()
	dev, ok := m.devices[id]
	m.mu.RUnlock()
	if !ok {
		return fmt.Errorf("device not connected: %s", id)
	}
	return dev.StartAcquisition()
}

func (m *DeviceManager) StopAcquisition(id string) error {
	m.mu.RLock()
	dev, ok := m.devices[id]
	m.mu.RUnlock()
	if !ok {
		return nil
	}
	return dev.StopAcquisition()
}

func (m *DeviceManager) GetStatus(id string) (device.Status, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	dev, ok := m.devices[id]
	if !ok {
		return device.Status{}, false
	}
	return dev.Status(), true
}

func (m *DeviceManager) findProfileLocked(id string) (device.Profile, bool) {
	for _, profile := range m.profiles {
		if profile.ID == id {
			return profile, true
		}
	}
	return device.Profile{}, false
}
