package hardware

import (
	"fmt"
	"sync"

	sharedcore "shared.local/device-sdk/go/daq/core"
	sharedhw "shared.local/device-sdk/go/daq/hardware"

	"windlabx4/services/api-go/internal/core/device"
	"windlabx4/services/api-go/internal/ports"
)

// ============================================================
// DAQ-P-1603 thin wrapper
// ------------------------------------------------------------
// 职责：在 WindLabX4 项目类型（device.Profile/Status/DataPayload）与
// shared SDK 类型（sharedcore.Profile/Status/DataPayload）之间做翻译。
// 实际设备逻辑由 sharedhw.DAQP1603 承载，本适配器不持有任何硬件
// 协议细节，符合六边形架构"adapter 仅做协议翻译"的边界约束。
//
// 全部 ports.Device 方法（Connect/Disconnect/StartAcquisition/StopAcquisition/
// SetDataSink/Status）已委托到 driver。Acquire 切片（Task 9）已让 driver
// 真正启动采集；Resilience 切片（Task 14）将由 driver.OnReadLoopExit
// 转发异常退出回调到 SetOnError。
// ============================================================

// DAQP1603Adapter 是 sharedhw.DAQP1603 的 thin wrapper。
type DAQP1603Adapter struct {
	mu      sync.RWMutex
	driver  *sharedhw.DAQP1603
	profile device.Profile
	sink    device.DataSink
	// onError 由 SetOnError 设置，当前仅存储引用；
	// Phase 6 Resilience 切片将把 driver.OnReadLoopExit 转发到此回调。
	onError func(err error)
}

// 编译期断言：DAQP1603Adapter 实现 ports.Device 、 ports.DAQP1603Configurable
// 与 ports.TareConfigurable。归零能力与其他压力扫描设备（DAQ-P-1604 / DSA3217）对齐。
var (
	_ ports.Device               = (*DAQP1603Adapter)(nil)
	_ ports.DAQP1603Configurable = (*DAQP1603Adapter)(nil)
	_ ports.TareConfigurable     = (*DAQP1603Adapter)(nil)
)

// NewDAQP1603Adapter 构造一个 DAQ-P-1603 适配器。
// 调用 Connect 后才会创建底层 driver。
func NewDAQP1603Adapter(profile device.Profile) *DAQP1603Adapter {
	return &DAQP1603Adapter{
		profile: profile,
	}
}

// SetOnError 设置设备异常退出回调。
// 当前仅存储回调引用；driver 已有 readLoop（Task 9 完成），
// Phase 6 Resilience 切片会通过 driver.OnReadLoopExit 转发到此处。
func (a *DAQP1603Adapter) SetOnError(fn func(err error)) {
	a.mu.Lock()
	a.onError = fn
	a.mu.Unlock()
}

// ID 返回设备唯一标识。
func (a *DAQP1603Adapter) ID() string {
	return a.profile.ID
}

// Connect 创建 driver 并建立 TCP 连接。
// 重复调用安全：已连接时直接返回 nil。
func (a *DAQP1603Adapter) Connect() error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.driver != nil {
		return nil
	}

	dev := sharedhw.NewDAQP1603(mapToSharedProfileP1603(a.profile))

	// 若调用方在 Connect 之前已 SetDataSink，需转发到 driver
	if a.sink != nil {
		a.setSharedDataSinkLocked(dev)
	}

	// driver.Connect 内部走 DLL FFI 路径，不涉及回调重入，
	// 无需像 T1603Adapter 那样临时释放 a.mu。
	if err := dev.Connect(); err != nil {
		return fmt.Errorf("DAQ-P-1603 connect: %w", err)
	}

	a.driver = dev
	return nil
}

// Disconnect 释放设备连接。重复调用安全。
func (a *DAQP1603Adapter) Disconnect() error {
	a.mu.Lock()
	dev := a.driver
	a.driver = nil
	a.sink = nil
	a.mu.Unlock()

	if dev == nil {
		return nil
	}
	return dev.Disconnect()
}

// StartAcquisition 委托到 driver。driver 启动 AI_StartTask + AI_SendSoftTrig
// 并启动 readLoop goroutine 循环读取数据投递 sink。
func (a *DAQP1603Adapter) StartAcquisition() error {
	a.mu.RLock()
	dev := a.driver
	a.mu.RUnlock()

	if dev == nil {
		return fmt.Errorf("device not connected")
	}
	return dev.StartAcquisition()
}

// StopAcquisition 委托到 driver。driver 调用 AI_StopTask + AI_ReleaseTask，
// 并通过 close(stop) 通知 readLoop 1 秒内优雅退出。
func (a *DAQP1603Adapter) StopAcquisition() error {
	a.mu.RLock()
	dev := a.driver
	a.mu.RUnlock()

	if dev == nil {
		return nil
	}
	return dev.StopAcquisition()
}

// SetDataSink 注册数据回调。
// 已连接时立即转发到 driver；未连接时仅缓存，Connect 时再转发。
func (a *DAQP1603Adapter) SetDataSink(sink device.DataSink) {
	a.mu.Lock()
	a.sink = sink
	dev := a.driver
	a.mu.Unlock()

	if dev == nil {
		return
	}
	if sink == nil {
		dev.SetDataSink(nil)
		return
	}
	a.mu.Lock()
	a.setSharedDataSinkLocked(dev)
	a.mu.Unlock()
}

// setSharedDataSinkLocked 将 a.sink 包装成 sharedcore.DataSink 注册到 driver。
// 调用方必须持有 a.mu 写锁。
func (a *DAQP1603Adapter) setSharedDataSinkLocked(dev *sharedhw.DAQP1603) {
	dev.SetDataSink(func(payload sharedcore.DataPayload) {
		// 读取最新 sink 引用，避免 sink 被 SetDataSink(nil) 清空后仍调用旧引用
		a.mu.RLock()
		fn := a.sink
		a.mu.RUnlock()
		if fn != nil {
			fn(mapToDevicePayloadP1603(payload, a.profile.Type, a.profile.Name))
		}
	})
}

// Status 返回设备状态快照。未连接时返回 Disconnected 静态状态。
func (a *DAQP1603Adapter) Status() device.Status {
	a.mu.RLock()
	dev := a.driver
	a.mu.RUnlock()

	if dev == nil {
		return device.Status{
			ID:         a.profile.ID,
			Name:       a.profile.Name,
			Type:       a.profile.Type,
			Connection: device.ConnectionDisconnected,
		}
	}
	return mapToDeviceStatusP1603(dev.Status())
}

// GetDAQP1603Config 返回当前设备 profile 的拷贝。
// 未连接时返回构造时的 profile（不含运行时变更）；
// 已连接时返回 driver 内部最新 profile（ApplyConfig 后的值）。
func (a *DAQP1603Adapter) GetDAQP1603Config() (device.Profile, error) {
	a.mu.RLock()
	dev := a.driver
	profile := a.profile
	a.mu.RUnlock()

	if dev == nil {
		return profile, nil
	}

	// 从 driver 拿到最新 profile（含 ApplyConfig 后的变更）
	sharedProfile := dev.GetProfile()
	return mapFromSharedProfileP1603(sharedProfile, profile), nil
}

// ApplyDAQP1603Config 应用新的设备配置。
// 类型翻译：device.Profile → sharedcore.Profile，委托 driver.ApplyConfig 执行。
// 已连接设备会同步到硬件（ReleaseTask → VerifyParam → InitTask）；
// 未连接设备仅更新内部 profile。
func (a *DAQP1603Adapter) ApplyDAQP1603Config(profile device.Profile) error {
	a.mu.RLock()
	dev := a.driver
	a.mu.RUnlock()

	sharedProfile := mapToSharedProfileP1603(profile)

	if dev == nil {
		// 未连接：仅缓存 profile，待 Connect 时生效
		a.mu.Lock()
		a.profile = profile
		a.mu.Unlock()
		return nil
	}

	if err := dev.ApplyConfig(sharedProfile); err != nil {
		return err
	}

	// 同步更新 adapter 层缓存的 profile（用于 GetDAQP1603Config 未连接时返回）
	a.mu.Lock()
	a.profile = profile
	a.mu.Unlock()
	return nil
}

// SetTare 设置指定通道的归零偏移。
//
// 双写策略：
//   - 已连接：委托 driver.SetTare 写入 shared SDK 内部 profile。该旧字段仅用于
//     v1 profile 兼容，新校零由 usecase CalibrationApplier 统一应用。
//   - 未连接：直接写 adapter 缓存的 profile，Connect 时通过 mapToSharedProfileP1603
//     传递到 driver，避免归零在连接前丢失
//
// 同时更新 adapter 缓存 profile，保证 GetDAQP1603Config 在 driver 释放后仍返回最新值。
// 通道越界校验在 Lock 内前置执行：避免先写 driver 后才发现 channelIndex 非法，
// 导致硬件已写入错误通道而 adapter 缓存却因越界未更新（数据不一致）。
func (a *DAQP1603Adapter) SetTare(channelIndex int, offset float64) error {
	a.mu.Lock()
	// 越界校验前置：driver.SetTare 成功后再发现越界已无法回滚硬件状态。
	if channelIndex < 0 || channelIndex >= len(a.profile.Channels) {
		a.mu.Unlock()
		return fmt.Errorf("DAQ-P-1603 set tare: invalid channel index %d (valid 0-%d)",
			channelIndex, len(a.profile.Channels)-1)
	}
	dev := a.driver
	a.profile.Channels[channelIndex].TareOffset = offset
	a.mu.Unlock()

	// driver 调用不持 adapter 锁：driver 内部自有锁，持 adapter 锁嵌套会阻塞其他 adapter 操作。
	// driver.SetTare 失败不影响 profile 缓存——这是设计意图（未连接时缓存，连接时通过 driver 同步）。
	if dev != nil {
		if err := dev.SetTare(channelIndex, offset); err != nil {
			return err
		}
	}
	return nil
}

// GetTare 读取指定通道的归零偏移。
// 已连接时优先从 driver 读取（反映运行时最新值）；未连接时回退到 adapter 缓存 profile。
// 越界校验与 profile 读取必须在同一 RLock 区间内：释放锁后读 profile.Channels[idx]
// 会与 SetTare 的写操作竞争底层数组（race），slice header 复制不等于底层数组隔离。
func (a *DAQP1603Adapter) GetTare(channelIndex int) (float64, error) {
	a.mu.RLock()
	dev := a.driver
	if channelIndex < 0 || channelIndex >= len(a.profile.Channels) {
		a.mu.RUnlock()
		return 0, fmt.Errorf("DAQ-P-1603 get tare: invalid channel index %d (valid 0-%d)",
			channelIndex, len(a.profile.Channels)-1)
	}
	tare := a.profile.Channels[channelIndex].TareOffset
	a.mu.RUnlock()

	// driver 调用不持 adapter 锁：driver 内部自有锁，避免锁嵌套。
	if dev != nil {
		return dev.GetTare(channelIndex)
	}
	return tare, nil
}

// ClearTare 清除指定通道的归零偏移（置 0）。
// 等价于 SetTare(channelIndex, 0)，保留独立方法以匹配 ports.TareConfigurable 接口。
func (a *DAQP1603Adapter) ClearTare(channelIndex int) error {
	return a.SetTare(channelIndex, 0)
}

// ---- 类型转换辅助 ----

// mapToSharedProfileP1603 将 WindLabX4 的 device.Profile 转为 shared SDK 的 Profile。
// 与 T1603Adapter.mapToSharedProfile 不同：DAQ-P-1603 不硬编码 16 通道名，
// 而是按 profile.Channels 实际内容映射，显式传递 SensorType 信息。
// 若 profile.Channels 为空，则补齐 16 个默认通道（与 DLL 16 通道能力对齐）。
func mapToSharedProfileP1603(p device.Profile) sharedcore.Profile {
	// 16 = WTNDAQ16H_AI_MAX_CHANNELS，DLL 硬件能力上限。
	// 此处不直接引用 ffi 包，避免 adapter 层依赖 FFI 细节。
	const maxChannels = 16

	channels := make([]sharedcore.ChannelConfig, 0, len(p.Channels))
	for i, ch := range p.Channels {
		if i >= maxChannels {
			break
		}
		// 显式传递 SensorType：旧注释"由 UnmarshalJSON 兜底"的假设有缺陷——
		// 若用户在前端把通道改为 temperature 但 WindLabX4 ChannelConfig.SensorType
		// 未显式传递，sharedcore 拿到的会是空字符串（被兜底为 pressure），
		// 用户配置丢失。此处必须显式映射。
		sensorType := sharedcore.SensorPressure
		if ch.SensorType == device.SensorTemperature {
			sensorType = sharedcore.SensorTemperature
		}
		channels = append(channels, sharedcore.ChannelConfig{
			Index:              ch.Index,
			Name:               ch.Name,
			Enabled:            ch.Enabled,
			Unit:               ch.Unit,
			Precision:          ch.Precision,
			RangeMin:           ch.RangeMin,
			RangeMax:           ch.RangeMax,
			TareOffset:         ch.TareOffset,
			SensorType:         sensorType,
			CalibrationOffset:  ch.CalibrationOffset,
			CalibrationUnit:    ch.CalibrationUnit,
			CalibrationAt:      ch.CalibrationAt,
			CalibrationEnabled: ch.CalibrationEnabled,
		})
	}

	// profile 无通道时补齐 16 个默认通道，保证 driver.buildAIParamLocked
	// 能完成 InitTask（DLL 要求至少 1 个启用通道）。
	if len(channels) == 0 {
		channels = make([]sharedcore.ChannelConfig, maxChannels)
		for i := range channels {
			channels[i] = sharedcore.ChannelConfig{
				Index:      i,
				Name:       fmt.Sprintf("CH%d", i+1),
				Enabled:    true,
				Unit:       "Pa",
				Precision:  3,
				SensorType: sharedcore.SensorPressure,
			}
		}
	}

	return sharedcore.Profile{
		ID:           p.ID,
		Name:         p.Name,
		Type:         sharedcore.DeviceDAQP1603,
		Transport:    p.Transport,
		Address:      p.Address,
		Port:         p.Port,
		SamplingRate: p.SamplingRate,
		Channels:     channels,
	}
}

// mapFromSharedProfileP1603 将 shared SDK 的 Profile 转回 WindLabX4 的 Profile。
// fallback 用于补全 WindLabX4 端独有字段（如 SerialPort、BaudRate、AutoConnect 等），
// 这些字段 shared SDK 不持有，从 adapter 缓存的旧 profile 取。
func mapFromSharedProfileP1603(s sharedcore.Profile, fallback device.Profile) device.Profile {
	const maxChannels = 16

	channels := make([]device.ChannelConfig, 0, len(s.Channels))
	for i, ch := range s.Channels {
		if i >= maxChannels {
			break
		}
		sensorType := device.SensorPressure
		if ch.SensorType == sharedcore.SensorTemperature {
			sensorType = device.SensorTemperature
		}
		channels = append(channels, device.ChannelConfig{
			Index:              ch.Index,
			Name:               ch.Name,
			Enabled:            ch.Enabled,
			Unit:               ch.Unit,
			Precision:          ch.Precision,
			RangeMin:           ch.RangeMin,
			RangeMax:           ch.RangeMax,
			TareOffset:         ch.TareOffset,
			SensorType:         sensorType,
			CalibrationOffset:  ch.CalibrationOffset,
			CalibrationUnit:    ch.CalibrationUnit,
			CalibrationAt:      ch.CalibrationAt,
			CalibrationEnabled: ch.CalibrationEnabled,
		})
	}

	return device.Profile{
		Version:                    fallback.Version,
		ID:                         s.ID,
		Name:                       s.Name,
		Type:                       device.DeviceDAQP1603,
		Transport:                  fallback.Transport,
		Address:                    s.Address,
		Port:                       fallback.Port,
		SerialPort:                 fallback.SerialPort,
		BaudRate:                   fallback.BaudRate,
		AutoConnect:                fallback.AutoConnect,
		MacAddress:                 fallback.MacAddress,
		SamplingRate:               s.SamplingRate,
		Channels:                   channels,
		DaqP1604UseDeviceTimestamp: fallback.DaqP1604UseDeviceTimestamp,
		DaqT1603Config:             fallback.DaqT1603Config,
	}
}

// mapToDeviceStatusP1603 将 shared SDK 的 Status 转为 WindLabX4 的 Status。
// 类型字段做 string→Type 强类型转换，两者字面量保持一致。
func mapToDeviceStatusP1603(s sharedcore.Status) device.Status {
	return device.Status{
		ID:         s.ID,
		Name:       s.Name,
		Type:       device.Type(s.Type),
		Connection: device.Connection(s.Connection),
		Acquiring:  s.Acquiring,
		LastError:  s.LastError,
	}
}

// mapToDevicePayloadP1603 将 shared SDK 的 DataPayload 转为 WindLabX4 的 DataPayload。
// DeviceType/DeviceName 由 adapter 注入（driver 不知道 WindLabX4 项目层的设备名）。
func mapToDevicePayloadP1603(p sharedcore.DataPayload, deviceType device.Type, deviceName string) device.DataPayload {
	return device.DataPayload{
		DeviceID:       p.DeviceID,
		DeviceType:     deviceType,
		DeviceName:     deviceName,
		Timestamp:      p.Timestamp,
		Channels:       p.Channels,
		ChannelIndices: p.ChannelIndices,
	}
}
