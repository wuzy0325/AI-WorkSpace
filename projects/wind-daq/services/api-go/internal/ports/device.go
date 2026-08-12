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

// DAQP1603Configurable DAQ-P-1603 运行时配置接口。
// 与 DaqT1603Configurable 不同：1603 的配置直接以完整 profile 形式提交，
// 因为采样率、通道传感器类型、单位、精度都需要在已连接时同步到 DLL
// （通过 ReleaseTask → VerifyParam → InitTask 重新初始化任务）。
// GetDAQP1603Config 返回当前 profile 拷贝，避免外部修改污染内部状态。
type DAQP1603Configurable interface {
	GetDAQP1603Config() (device.Profile, error)
	ApplyDAQP1603Config(profile device.Profile) error
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

type Calibratable interface {
	Calibrate(deviceID string) ([]device.CalibrationResult, error)
	GetCalibration(channelIndex int) (device.CalibrationRecord, error)
	ClearCalibration(channelIndex int) error
}

type CalibrationEnabledConfigurable interface {
	SetCalibrationEnabled(channelIndex int, enabled bool) error
	GetCalibrationEnabled(channelIndex int) (bool, error)
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

// AcquisitionState 遍历视角的设备采集态（spec-traversal-acquisition-stop）：
//   - AcquisitionAcquiring：正在产帧，正常采样
//   - AcquisitionStopped：操作员主动停采（可恢复，无限期等待重启采集）
//   - AcquisitionReconnectRequired：掉线/未连接/已移除，需重连并启动采集
type AcquisitionState uint8

const (
	AcquisitionAcquiring AcquisitionState = iota
	AcquisitionStopped
	AcquisitionReconnectRequired
)

// AcquisitionStatus 设备采集态快照。
// State 来自单次 GetStatus（锁内一致性读取）；Name/LastError 为非关键展示元数据，
// 允许与 State 存在跨调用的小幅滞后。
type AcquisitionStatus struct {
	State     AcquisitionState
	Name      string // 设备显示名，诊断/错误文案用
	LastError string // 仅 State==AcquisitionReconnectRequired 且设备仍在 map 时有值；已移除为空
}

// AcquisitionController 设备采集控制端口。
//
// 用于遍历测试校验"目标设备正在采集"这一前提：
//   - IsConnected / IsAcquiring 供启动前检查与运行时采样校验真实反映设备状态；
//   - AcquisitionStatus 返回三态一致性快照，供遍历区分"停采"（可恢复等待）与
//     "掉线"（需重连）并展示针对性文案；
//   - StartAcquisition 是 DeviceManager 已有能力，但遍历测试不会调用它，采集生命周期
//     由操作员在设备管理中控制。
//
// 实现由 DeviceManager 提供（持有 devices map + GetStatus + StartAcquisition），
// 装配点通过 usecase.SetAcquisitionController 注入。
// nil 时调用方走降级路径（保持旧装配向后兼容）。
type AcquisitionController interface {
	IsConnected(id string) bool
	IsAcquiring(id string) bool
	StartAcquisition(id string) error
	DeviceName(id string) string
	// AcquisitionStatus 返回指定设备的采集态快照。
	AcquisitionStatus(id string) AcquisitionStatus
}
