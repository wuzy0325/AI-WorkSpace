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

// DSA3217Configurable DSA3217 扫描配置接口
type DSA3217Configurable interface {
	GetDsa3217ScanConfig() (device.DSA3217ScanConfig, error)
	ApplyDsa3217ScanConfig(avg int, period int) (device.DSA3217ScanConfig, error)
}

type TareConfigurable interface {
	SetTare(channelIndex int, offset float64) error
	GetTare(channelIndex int) (float64, error)
	ClearTare(channelIndex int) error
}

// ErrorNotifiable 设备异常通知接口
// 适配器实现此接口后，DeviceManager 可在设备 readLoop 异常退出时收到回调，
// 统一更新状态并通知前端，避免设备断开后状态仍显示为 Connected/Acquiring。
type ErrorNotifiable interface {
	SetOnError(fn func(err error))
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

// ProfileNormalizer 设备配置规范化端口
// 补全配置中缺失的字段（通道、地址、端口等硬件默认值）。
// 实现见 adapters/config.NormalizeProfile（依赖硬件特定默认值，属 adapter 职责）。
// usecase 通过此端口调用，避免直接依赖 adapters/config。
type ProfileNormalizer interface {
	Normalize(profile device.Profile) device.Profile
}
