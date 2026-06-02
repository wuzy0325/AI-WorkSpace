package ports

import "shared.local/device-sdk/go/daq/core"

type Device interface {
	ID() string
	Connect() error
	Disconnect() error
	StartAcquisition() error
	StopAcquisition() error
	SetDataSink(core.DataSink)
	Status() core.Status
}

type DaqT1603Configurable interface {
	GetDaqT1603Config() (core.DaqT1603HardwareConfig, error)
	ApplyDaqT1603Config(config core.DaqT1603HardwareConfig) error
}

type TareConfigurable interface {
	SetTare(channelIndex int, offset float64) error
	GetTare(channelIndex int) (float64, error)
	ClearTare(channelIndex int) error
}

type ProfileStore interface {
	LoadProfiles() ([]core.Profile, error)
	SaveProfiles(profiles []core.Profile) error
}

type DeviceFactory interface {
	Create(profile core.Profile) (Device, error)
}
