package hardware

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	sharedcore "shared.local/device-sdk/go/daq/core"
	sharedhw "shared.local/device-sdk/go/daq/hardware"

	"wind-daq/services/api-go/internal/core/device"
	"wind-daq/services/api-go/internal/ports"
)

// T1602Adapter is a thin wrapper around sharedhw.DAQT1602 that translates
// between wind-daq types and shared-core types.
type T1602Adapter struct {
	mu      sync.RWMutex
	driver  *sharedhw.DAQT1602
	profile device.Profile
	config  device.DaqT1602HardwareConfig
	sink    device.DataSink
	onError func(err error) // 设备异常退出通知回调
}

// compile-time interface checks
var _ ports.Device = (*T1602Adapter)(nil)
var _ ports.DaqT1602Configurable = (*T1602Adapter)(nil)
var _ ports.ErrorNotifiable = (*T1602Adapter)(nil)

func NewT1602Adapter(profile device.Profile) *T1602Adapter {
	return &T1602Adapter{
		profile: profile,
		config:  profile.DaqT1602Config,
	}
}

// SetOnError 设置设备异常退出回调，实现 ports.ErrorNotifiable 接口
func (a *T1602Adapter) SetOnError(fn func(err error)) {
	a.mu.Lock()
	a.onError = fn
	a.mu.Unlock()
}

func (a *T1602Adapter) ID() string {
	return a.profile.ID
}

func (a *T1602Adapter) Connect() error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.driver != nil {
		return nil
	}

	dev := sharedhw.NewDAQT1602(mapToSharedT1602Profile(a.profile))
	// T1602 采集/保存频率来自设备专属配置 SampleRateHz（独立于全局刷新频率）：
	// 驱动轮询间隔 = 1000/hz ms，每次采样前动态读取（hz<=0 视为全速）。
	// 配置变更（ApplyDaqT1602Config）后下一帧即生效，无需重启采集。
	dev.SetPollIntervalFn(func() time.Duration {
		a.mu.RLock()
		hz := a.config.SampleRateHz
		a.mu.RUnlock()
		if hz <= 0 {
			return 0
		}
		return time.Duration(float64(time.Second) / hz)
	})
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
		slog.LogAttrs(context.Background(), level, "DAQ-T-1602 "+entry.Message,
			slog.String("category", entry.Category),
			slog.String("component", "hardware"),
			slog.String("device", entry.DeviceID),
			slog.String("detail", entry.Detail),
		)
	})
	if a.sink != nil {
		a.setSharedT1602DataSink(dev)
	}

	dev.OnConfigSynced(func(cfg sharedcore.DaqT1602HardwareConfig) {
		a.mu.Lock()
		a.config = mapFromSharedT1602Config(cfg)
		a.profile.DaqT1602Config = a.config
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

		slog.Warn("T1602Adapter read loop exited unexpectedly", "device", a.profile.ID, "error", err)
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
	if cfg, err := dev.GetDaqT1602Config(); err == nil {
		a.config = mapFromSharedT1602Config(cfg)
		a.profile.DaqT1602Config = a.config
	}

	a.driver = dev
	return nil
}

func (a *T1602Adapter) Disconnect() error {
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

func (a *T1602Adapter) StartAcquisition() error {
	a.mu.RLock()
	dev := a.driver
	a.mu.RUnlock()

	if dev == nil {
		return fmt.Errorf("device not connected")
	}
	return dev.StartAcquisition()
}

func (a *T1602Adapter) StopAcquisition() error {
	a.mu.RLock()
	dev := a.driver
	a.mu.RUnlock()

	if dev == nil {
		return nil
	}
	return dev.StopAcquisition()
}

func (a *T1602Adapter) SetDataSink(sink device.DataSink) {
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
	a.setSharedT1602DataSink(dev)
}

func (a *T1602Adapter) setSharedT1602DataSink(dev *sharedhw.DAQT1602) {
	dev.SetDataSink(func(payload sharedcore.DataPayload) {
		a.mu.RLock()
		fn := a.sink
		a.mu.RUnlock()

		if fn != nil {
			fn(mapToDevicePayload(payload, a.profile.Type, a.profile.Name))
		}
	})
}

func (a *T1602Adapter) Status() device.Status {
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

func (a *T1602Adapter) GetDaqT1602Config() (device.DaqT1602HardwareConfig, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.config, nil
}

func (a *T1602Adapter) ApplyDaqT1602Config(cfg device.DaqT1602HardwareConfig) error {
	a.mu.Lock()
	a.config = cfg
	a.profile.DaqT1602Config = cfg
	dev := a.driver
	a.mu.Unlock()

	if dev == nil {
		return nil
	}
	return dev.ApplyDaqT1602Config(mapToSharedT1602Config(cfg))
}

// -- type conversion helpers --

func mapToSharedT1602Profile(p device.Profile) sharedcore.Profile {
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
		Type:           sharedcore.DeviceDaqT1602,
		Transport:      p.Transport,
		Address:        p.Address,
		Port:           p.Port,
		SamplingRate:   p.SamplingRate,
		Channels:       ch,
		DaqT1602Config: mapToSharedT1602Config(p.DaqT1602Config),
	}
}

func mapToSharedT1602Config(cfg device.DaqT1602HardwareConfig) sharedcore.DaqT1602HardwareConfig {
	return sharedcore.DaqT1602HardwareConfig{
		TypeCodes:    cfg.TypeCodes,
		SampleRateHz: cfg.SampleRateHz,
	}
}

func mapFromSharedT1602Config(cfg sharedcore.DaqT1602HardwareConfig) device.DaqT1602HardwareConfig {
	return device.DaqT1602HardwareConfig{
		TypeCodes:    cfg.TypeCodes,
		SampleRateHz: cfg.SampleRateHz,
	}
}
