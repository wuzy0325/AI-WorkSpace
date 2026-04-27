package ports

import "wind-daq/services/api-go/internal/core/device"

type DeviceFactory interface {
	Create(config device.DeviceConfig) (Device, error)
}
