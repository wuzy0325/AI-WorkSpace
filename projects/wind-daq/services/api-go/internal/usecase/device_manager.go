package usecase

import (
	"fmt"
	"strings"
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
	scanner  ports.DeviceScanner
	dataSink device.DataSink
}

func NewDeviceManager(store ports.ProfileStore, factory ports.DeviceFactory, dataSink device.DataSink) (*DeviceManager, error) {
	profiles, err := store.LoadProfiles()
	if err != nil {
		return nil, err
	}
	profiles = normalizeProfiles(profiles)
	return &DeviceManager{
		profiles: profiles,
		devices:  make(map[string]ports.Device),
		store:    store,
		factory:  factory,
		dataSink: dataSink,
	}, nil
}

func (m *DeviceManager) SetScanner(scanner ports.DeviceScanner) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.scanner = scanner
}

func (m *DeviceManager) ScanDevices() ([]device.ScanResult, error) {
	m.mu.RLock()
	scanner := m.scanner
	m.mu.RUnlock()
	if scanner == nil {
		return []device.ScanResult{}, nil
	}
	return scanner.Scan()
}

func (m *DeviceManager) GetProfiles() []device.Profile {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return append([]device.Profile(nil), m.profiles...)
}

func (m *DeviceManager) UpsertProfile(profile device.Profile) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	profile = device.NormalizeProfile(profile)
	for i := range m.profiles {
		if m.profiles[i].ID == profile.ID {
			m.profiles[i] = profile
			return m.store.SaveProfiles(m.profiles)
		}
	}
	m.profiles = append(m.profiles, profile)
	return m.store.SaveProfiles(m.profiles)
}

func normalizeProfiles(profiles []device.Profile) []device.Profile {
	normalized := make([]device.Profile, len(profiles))
	for i := range profiles {
		normalized[i] = device.NormalizeProfile(profiles[i])
	}
	return normalized
}

func (m *DeviceManager) DeleteProfile(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	profiles := m.profiles[:0]
	found := false
	for _, profile := range m.profiles {
		if profile.ID == id {
			found = true
			continue
		}
		profiles = append(profiles, profile)
	}
	if !found {
		return fmt.Errorf("device profile not found: %s", id)
	}
	if dev, ok := m.devices[id]; ok {
		_ = dev.StopAcquisition()
		_ = dev.Disconnect()
		delete(m.devices, id)
	}
	m.profiles = append([]device.Profile(nil), profiles...)
	return m.store.SaveProfiles(m.profiles)
}

func (m *DeviceManager) SetUnit(id string, unit string) error {
	unit = strings.TrimSpace(unit)
	if unit == "" {
		return fmt.Errorf("unit is required")
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	profileIndex, ok := m.findProfileIndexLocked(id)
	if !ok {
		return fmt.Errorf("device profile not found: %s", id)
	}
	if dev, ok := m.devices[id]; ok {
		configurable, ok := dev.(ports.UnitConfigurable)
		if !ok {
			return fmt.Errorf("device does not support unit configuration: %s", id)
		}
		if err := configurable.SetUnit(unit); err != nil {
			return err
		}
	}
	for i := range m.profiles[profileIndex].Channels {
		m.profiles[profileIndex].Channels[i].Unit = unit
	}
	return m.store.SaveProfiles(m.profiles)
}

func (m *DeviceManager) GetDaqT1603Config(id string) (device.DaqT1603HardwareConfig, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if dev, ok := m.devices[id]; ok {
		if configurable, ok := dev.(ports.DaqT1603Configurable); ok {
			return configurable.GetDaqT1603Config()
		}
	}
	profile, ok := m.findProfileLocked(id)
	if !ok {
		return device.DaqT1603HardwareConfig{}, fmt.Errorf("device profile not found: %s", id)
	}
	return profile.DaqT1603Config, nil
}

func (m *DeviceManager) ApplyDaqT1603Config(id string, config device.DaqT1603HardwareConfig) error {
	if err := validateDaqT1603Config(config); err != nil {
		return err
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	profileIndex, ok := m.findProfileIndexLocked(id)
	if !ok {
		return fmt.Errorf("device profile not found: %s", id)
	}
	if dev, ok := m.devices[id]; ok {
		configurable, ok := dev.(ports.DaqT1603Configurable)
		if !ok {
			return fmt.Errorf("device does not support DAQ-T-1603 configuration: %s", id)
		}
		if err := configurable.ApplyDaqT1603Config(config); err != nil {
			return err
		}
	}
	m.profiles[profileIndex].DaqT1603Config = config
	return m.store.SaveProfiles(m.profiles)
}

// GetDsa3217ScanConfig 获取 DSA3217 设备的扫描配置
func (m *DeviceManager) GetDsa3217ScanConfig(id string) (device.DSA3217ScanConfig, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if dev, ok := m.devices[id]; ok {
		if configurable, ok := dev.(ports.DSA3217Configurable); ok {
			return configurable.GetDsa3217ScanConfig()
		}
	}
	return device.DSA3217ScanConfig{}, fmt.Errorf("device not connected or does not support DSA3217 configuration: %s", id)
}

// ApplyDsa3217ScanConfig 写入 DSA3217 扫描配置并回读验证
func (m *DeviceManager) ApplyDsa3217ScanConfig(id string, avg int, period int) (device.DSA3217ScanConfig, error) {
	if avg < 1 || avg > 240 {
		return device.DSA3217ScanConfig{}, fmt.Errorf("AVG must be between 1 and 240")
	}
	if period < 73 || period > 65535 {
		return device.DSA3217ScanConfig{}, fmt.Errorf("PERIOD must be between 73 and 65535")
	}

	m.mu.RLock()
	dev, ok := m.devices[id]
	m.mu.RUnlock()

	if !ok {
		return device.DSA3217ScanConfig{}, fmt.Errorf("device not connected: %s", id)
	}
	configurable, ok := dev.(ports.DSA3217Configurable)
	if !ok {
		return device.DSA3217ScanConfig{}, fmt.Errorf("device does not support DSA3217 configuration: %s", id)
	}
	return configurable.ApplyDsa3217ScanConfig(avg, period)
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

func (m *DeviceManager) Disconnect(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	dev, ok := m.devices[id]
	if !ok {
		return nil
	}
	if err := dev.StopAcquisition(); err != nil {
		return err
	}
	if err := dev.Disconnect(); err != nil {
		return err
	}
	delete(m.devices, id)
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

func (m *DeviceManager) findProfileIndexLocked(id string) (int, bool) {
	for i := range m.profiles {
		if m.profiles[i].ID == id {
			return i, true
		}
	}
	return 0, false
}

func (m *DeviceManager) SetTare(id string, channelIndex int, offset float64) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	profileIndex, ok := m.findProfileIndexLocked(id)
	if !ok {
		return fmt.Errorf("device profile not found: %s", id)
	}
	if dev, ok := m.devices[id]; ok {
		if configurable, ok := dev.(ports.TareConfigurable); ok {
			if err := configurable.SetTare(channelIndex, offset); err != nil {
				return err
			}
		}
	}
	if channelIndex >= 0 && channelIndex < len(m.profiles[profileIndex].Channels) {
		m.profiles[profileIndex].Channels[channelIndex].TareOffset = offset
	}
	return m.store.SaveProfiles(m.profiles)
}

func (m *DeviceManager) GetTare(id string, channelIndex int) (float64, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if dev, ok := m.devices[id]; ok {
		if configurable, ok := dev.(ports.TareConfigurable); ok {
			return configurable.GetTare(channelIndex)
		}
	}
	profile, ok := m.findProfileLocked(id)
	if !ok {
		return 0, fmt.Errorf("device profile not found: %s", id)
	}
	if channelIndex < 0 || channelIndex >= len(profile.Channels) {
		return 0, fmt.Errorf("invalid channel index: %d", channelIndex)
	}
	return profile.Channels[channelIndex].TareOffset, nil
}

func (m *DeviceManager) ClearTare(id string, channelIndex int) error {
	return m.SetTare(id, channelIndex, 0)
}

func validateDaqT1603Config(config device.DaqT1603HardwareConfig) error {
	if strings.TrimSpace(config.ThermocoupleType) == "" {
		return fmt.Errorf("thermocoupleType is required")
	}
	if strings.TrimSpace(config.ColdJunction) == "" {
		return fmt.Errorf("coldJunction is required")
	}
	if config.FilterHz <= 0 {
		return fmt.Errorf("filterHz must be greater than zero")
	}
	return nil
}
