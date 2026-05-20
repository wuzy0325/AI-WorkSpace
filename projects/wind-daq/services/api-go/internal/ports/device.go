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
