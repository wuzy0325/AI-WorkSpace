package hardware

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	sharedcore "shared.local/device-sdk/go/daq/core"
	sharedhw "shared.local/device-sdk/go/daq/hardware"

	"wind-daq/services/api-go/internal/core/device"
	"wind-daq/services/api-go/internal/ports"
)

// T1603Adapter is a thin wrapper around sharedhw.DAQT1603 that translates
// between wind-daq types and shared-core types.
type T1603Adapter struct {
	mu      sync.RWMutex
	driver  *sharedhw.DAQT1603
	profile device.Profile
	config  device.DaqT1603HardwareConfig
	sink    device.DataSink
	onError func(err error) // 设备异常退出通知回调
}

// compile-time interface checks
var _ ports.Device = (*T1603Adapter)(nil)
var _ ports.UnitConfigurable = (*T1603Adapter)(nil)
var _ ports.TareConfigurable = (*T1603Adapter)(nil)
var _ ports.DaqT1603Configurable = (*T1603Adapter)(nil)
var _ ports.ErrorNotifiable = (*T1603Adapter)(nil)

func NewT1603Adapter(profile device.Profile) *T1603Adapter {
	return &T1603Adapter{
		profile: profile,
		config:  profile.DaqT1603Config,
	}
}

// SetOnError 设置设备异常退出回调，实现 ports.ErrorNotifiable 接口
func (a *T1603Adapter) SetOnError(fn func(err error)) {
	a.mu.Lock()
	a.onError = fn
	a.mu.Unlock()
}

func (a *T1603Adapter) ID() string {
	return a.profile.ID
}

func (a *T1603Adapter) Connect() error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.driver != nil {
		return nil
	}

	dev := sharedhw.NewDAQT1603(mapToSharedProfile(a.profile))
	dev.OnLog(func(entry sharedhw.LogEntry) {
		// sharedhw 把收发命令 emitLog 为 debug 级别；
		// 适配器需统一降级到 Debug，避免高频收发把 ring buffer 与日志文件刷爆。
		// Connect/Disconnect 等链路事件仍走 INFO（sharedhw emitLog 时使用 info 级别）。
		level := slog.LevelInfo
		switch entry.Level {
		case "warn":
			level = slog.LevelWarn
		case "error":
			level = slog.LevelError
		case "debug":
			level = slog.LevelDebug
		}
		slog.LogAttrs(context.Background(), level, "DAQ-T-1603 "+entry.Message,
			slog.String("category", entry.Category),
			slog.String("component", "hardware"),
			slog.String("device", entry.DeviceID),
			slog.String("detail", entry.Detail),
		)
	})
	if a.sink != nil {
		a.setSharedDataSink(dev)
	}

	dev.OnConfigSynced(func(cfg sharedcore.DaqT1603HardwareConfig) {
		a.mu.Lock()
		a.config = mapFromSharedConfig(cfg)
		a.profile.DaqT1603Config = a.config
		a.mu.Unlock()
	})

	dev.OnReadLoopExit(func(err error) {
		a.mu.Lock()
		// readLoop 异常退出说明连接已损坏，必须清理 driver 引用，
		// 否则下次 Connect 会误以为设备仍在线而在坏连接上继续操作。
		a.driver = nil
		a.sink = nil
		fn := a.onError
		a.mu.Unlock()

		slog.Warn("T1603Adapter read loop exited unexpectedly", "device", a.profile.ID, "error", err)
		if fn != nil {
			fn(err)
		}
	})

	// dev.Connect() 会同步触发 OnConfigSynced 回调，回调内取 a.mu。
	// 必须先释放 a.mu，否则同 goroutine 重入 a.mu 自死锁。
	// 顶层 defer a.mu.Unlock() 负责最终释放；此处仅临时让锁。
	a.mu.Unlock()
	connectErr := dev.Connect()
	a.mu.Lock()
	if connectErr != nil {
		return fmt.Errorf("connect: %w", connectErr)
	}
	if cfg, err := dev.GetDaqT1603Config(); err == nil {
		a.config = mapFromSharedConfig(cfg)
		a.profile.DaqT1603Config = a.config
	}

	a.driver = dev
	return nil
}

func (a *T1603Adapter) Disconnect() error {
	a.mu.Lock()
	dev := a.driver
	a.driver = nil
	a.sink = nil
	a.mu.Unlock()

	if dev != nil {
		return dev.Disconnect()
	}
	return nil
}

func (a *T1603Adapter) StartAcquisition() error {
	a.mu.RLock()
	dev := a.driver
	a.mu.RUnlock()

	if dev == nil {
		return fmt.Errorf("device not connected")
	}
	return dev.StartAcquisition()
}

func (a *T1603Adapter) StopAcquisition() error {
	a.mu.RLock()
	dev := a.driver
	a.mu.RUnlock()

	if dev == nil {
		return nil
	}
	return dev.StopAcquisition()
}

func (a *T1603Adapter) SetDataSink(sink device.DataSink) {
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
	a.setSharedDataSink(dev)
}

func (a *T1603Adapter) setSharedDataSink(dev *sharedhw.DAQT1603) {
	dev.SetDataSink(func(payload sharedcore.DataPayload) {
		a.mu.RLock()
		fn := a.sink
		a.mu.RUnlock()

		if fn != nil {
			fn(mapToDevicePayload(payload, a.profile.Type))
		}
	})
}

func (a *T1603Adapter) Status() device.Status {
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

	return mapToDeviceStatus(dev.Status())
}

func (a *T1603Adapter) SetUnit(unit string) error {
	a.mu.RLock()
	dev := a.driver
	a.mu.RUnlock()

	if dev == nil {
		return fmt.Errorf("device not connected")
	}
	return dev.SetUnit(unit)
}

func (a *T1603Adapter) SetTare(channelIndex int, offset float64) error {
	a.mu.RLock()
	dev := a.driver
	a.mu.RUnlock()

	if dev == nil {
		return fmt.Errorf("device not connected")
	}
	return dev.SetTare(channelIndex, offset)
}

func (a *T1603Adapter) GetTare(channelIndex int) (float64, error) {
	a.mu.RLock()
	dev := a.driver
	a.mu.RUnlock()

	if dev == nil {
		return 0, fmt.Errorf("device not connected")
	}
	return dev.GetTare(channelIndex)
}

func (a *T1603Adapter) ClearTare(channelIndex int) error {
	a.mu.RLock()
	dev := a.driver
	a.mu.RUnlock()

	if dev == nil {
		return fmt.Errorf("device not connected")
	}
	return dev.ClearTare(channelIndex)
}

func (a *T1603Adapter) GetDaqT1603Config() (device.DaqT1603HardwareConfig, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.config, nil
}

func (a *T1603Adapter) ApplyDaqT1603Config(cfg device.DaqT1603HardwareConfig) error {
	a.mu.Lock()
	a.config = cfg
	a.profile.DaqT1603Config = cfg
	dev := a.driver
	a.mu.Unlock()

	if dev == nil {
		return nil
	}
	return dev.ApplyDaqT1603Config(mapToSharedConfig(cfg))
}

// -- type conversion helpers --

func mapToSharedProfile(p device.Profile) sharedcore.Profile {
	ch := make([]sharedcore.ChannelConfig, 16)
	for i := range ch {
		ch[i] = sharedcore.ChannelConfig{
			Index:      i,
			Name:       fmt.Sprintf("TC%d", i+1),
			Enabled:    true,
			Unit:       "degC",
			Precision:  2,
			TareOffset: 0,
		}
		if i < len(p.Channels) {
			ch[i].Name = p.Channels[i].Name
			ch[i].Enabled = p.Channels[i].Enabled
			ch[i].Unit = p.Channels[i].Unit
			ch[i].Precision = p.Channels[i].Precision
			ch[i].RangeMin = p.Channels[i].RangeMin
			ch[i].RangeMax = p.Channels[i].RangeMax
			ch[i].TareOffset = p.Channels[i].TareOffset
		}
	}

	return sharedcore.Profile{
		ID:             p.ID,
		Name:           p.Name,
		Type:           sharedcore.DeviceDaqT1603,
		Transport:      p.Transport,
		Address:        p.Address,
		Port:           p.Port,
		SamplingRate:   p.SamplingRate,
		Channels:       ch,
		DaqT1603Config: mapToSharedConfig(p.DaqT1603Config),
	}
}

func mapToSharedConfig(cfg device.DaqT1603HardwareConfig) sharedcore.DaqT1603HardwareConfig {
	return sharedcore.DaqT1603HardwareConfig{
		ThermocoupleTypes: cfg.ThermocoupleTypes,
		ChannelMask:       cfg.ChannelMask,
		SamplingRate:      cfg.SamplingRate,
		BinaryFormat:      cfg.BinaryFormat,
		AverageCount:      cfg.AverageCount,
		TriggerMode:       cfg.TriggerMode,
		TriggerEdge:       cfg.TriggerEdge,
		TriggerCount:      cfg.TriggerCount,
		ShowTimestamp:     cfg.ShowTimestamp,
		ShowSequence:      cfg.ShowSequence,
		OpenCircuitCheck:  cfg.OpenCircuitCheck,
	}
}

func mapFromSharedConfig(cfg sharedcore.DaqT1603HardwareConfig) device.DaqT1603HardwareConfig {
	return device.DaqT1603HardwareConfig{
		ThermocoupleTypes: cfg.ThermocoupleTypes,
		ChannelMask:       cfg.ChannelMask,
		SamplingRate:      cfg.SamplingRate,
		BinaryFormat:      cfg.BinaryFormat,
		AverageCount:      cfg.AverageCount,
		TriggerMode:       cfg.TriggerMode,
		TriggerEdge:       cfg.TriggerEdge,
		TriggerCount:      cfg.TriggerCount,
		ShowTimestamp:     cfg.ShowTimestamp,
		ShowSequence:      cfg.ShowSequence,
		OpenCircuitCheck:  cfg.OpenCircuitCheck,
	}
}

func mapToDeviceStatus(s sharedcore.Status) device.Status {
	return device.Status{
		ID:         s.ID,
		Name:       s.Name,
		Type:       device.Type(s.Type),
		Connection: device.Connection(s.Connection),
		Acquiring:  s.Acquiring,
		LastError:  s.LastError,
	}
}

func mapToDevicePayload(p sharedcore.DataPayload, deviceType device.Type) device.DataPayload {
	return device.DataPayload{
		DeviceID:       p.DeviceID,
		DeviceType:     deviceType,
		Timestamp:      p.Timestamp,
		Channels:       p.Channels,
		ChannelIndices: p.ChannelIndices,
	}
}
