package ports

import "wista/core"

type ConfigPort interface {
	LoadProfiles() ([]core.TemperatureProfile, error)
	SaveProfile(profile core.TemperatureProfile) error
	DeleteProfile(id string) error
}
