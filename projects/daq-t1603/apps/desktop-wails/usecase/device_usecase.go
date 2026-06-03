package usecase

import (
	"fmt"
	"log/slog"

	"daq-t1603/core"
	"daq-t1603/ports"
)

type DeviceUsecase struct {
	device ports.DevicePort
	config ports.ConfigPort
	scanner ports.DeviceScanPort
}

func NewDeviceUsecase(device ports.DevicePort, config ports.ConfigPort) *DeviceUsecase {
	return &DeviceUsecase{device: device, config: config}
}

func (uc *DeviceUsecase) SetScanner(scanner ports.DeviceScanPort) {
	uc.scanner = scanner
}

func (uc *DeviceUsecase) GetProfiles() []core.TemperatureProfile {
	profiles, err := uc.config.LoadProfiles()
	if err != nil {
		slog.Warn("load profiles", "error", err)
		return nil
	}
	if profiles == nil {
		return []core.TemperatureProfile{}
	}
	return profiles
}

func (uc *DeviceUsecase) UpsertProfile(profile core.TemperatureProfile) error {
	return uc.config.SaveProfile(profile)
}

func (uc *DeviceUsecase) DeleteProfile(id string) error {
	_ = uc.device.Disconnect(id)
	return uc.config.DeleteProfile(id)
}

func (uc *DeviceUsecase) Connect(id string) error {
	profiles, err := uc.config.LoadProfiles()
	if err != nil {
		return fmt.Errorf("load profiles: %w", err)
	}
	var profile *core.TemperatureProfile
	for i := range profiles {
		if profiles[i].ID == id {
			profile = &profiles[i]
			break
		}
	}
	if profile == nil {
		return fmt.Errorf("profile %s not found", id)
	}
	return uc.device.Connect(*profile)
}

func (uc *DeviceUsecase) Disconnect(id string) error {
	return uc.device.Disconnect(id)
}

func (uc *DeviceUsecase) StartAcquisition(id string) (<-chan core.TemperatureSnapshot, error) {
	return uc.device.StartAcquisition(id)
}

func (uc *DeviceUsecase) StopAcquisition(id string) error {
	return uc.device.StopAcquisition(id)
}

func (uc *DeviceUsecase) GetStatus(id string) (core.DeviceState, bool) {
	return uc.device.Status(id)
}

func (uc *DeviceUsecase) ScanDevices() ([]core.ScanResult, error) {
	if uc.scanner == nil {
		return []core.ScanResult{}, nil
	}
	return uc.scanner.Scan()
}

func (uc *DeviceUsecase) ApplyConfig(id string, cfg core.T1603Config) error {
	return uc.device.ApplyConfig(id, cfg)
}
