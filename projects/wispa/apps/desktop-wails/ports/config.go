package ports

import "wispa/core"

// ConfigPort 配置持久化端口接口
type ConfigPort interface {
	LoadProfiles() ([]core.PressureProfile, error)
	SaveProfile(profile core.PressureProfile) error
	DeleteProfile(id string) error
}
