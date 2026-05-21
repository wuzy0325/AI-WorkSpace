package ports

import "wind-daq/services/api-go/internal/core/device"

type Device interface {
	ID() string
	Connect() error
	Disconnect() error
	StartAcquisition() error
	StopAcquisition() error
	SetDataSink(device.DataSink)
	Status() device.Status
}

type UnitConfigurable interface {
	SetUnit(unit string) error
}

type DaqT1603Configurable interface {
	GetDaqT1603Config() (device.DaqT1603HardwareConfig, error)
	ApplyDaqT1603Config(config device.DaqT1603HardwareConfig) error
}

type Publisher interface {
	Publish(channel string, data any)
}

type ProfileStore interface {
	LoadProfiles() ([]device.Profile, error)
	SaveProfiles(profiles []device.Profile) error
}

type DeviceFactory interface {
	Create(profile device.Profile) (Device, error)
}

type DeviceScanner interface {
	Scan() ([]device.ScanResult, error)
}
