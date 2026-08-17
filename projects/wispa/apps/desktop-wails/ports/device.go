package ports

import "wispa/core"

// DevicePort 设备操作端口接口
type DevicePort interface {
	Connect(profile core.PressureProfile) error
	Disconnect(id string) error
	StartAcquisition(id string) (<-chan core.PressureSnapshot, error)
	StopAcquisition(id string) error
	ZeroCalibration(id string) error
	Status(id string) (core.DeviceState, bool)
	ApplyConfig(id string, cfg core.P1604Config) error
	SetDataSink(id string, sink func(core.PressureSnapshot))
}
