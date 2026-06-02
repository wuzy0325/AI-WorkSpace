package ports

import "daq-t1603/core"

type DevicePort interface {
	Connect(profile core.TemperatureProfile) error
	Disconnect(id string) error
	StartAcquisition(id string) (<-chan core.TemperatureSnapshot, error)
	StopAcquisition(id string) error
	Status(id string) (core.DeviceState, bool)
	ApplyConfig(id string, cfg core.T1603Config) error
	SetDataSink(id string, sink func(core.TemperatureSnapshot))
}
