package ports

import "wind-daq/services/api-go/internal/core/device"

type ProfileStore interface {
	LoadProfiles() ([]device.DeviceProfile, error)
	SaveProfiles(profiles []device.DeviceProfile) error
}
