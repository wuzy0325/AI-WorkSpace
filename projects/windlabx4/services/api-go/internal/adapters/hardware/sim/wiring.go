package sim

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"testing"

	"windlabx4/services/api-go/internal/core/device"
)

// wiring.go 提供测试装配工具：按设备类型启动对应模拟器，并构造指向模拟器
// 地址的 device.Profile，让真实 adapter 连接模拟器进行端到端测试。
//
// 不改 bootstrap.go 的生产 deviceFactory：模拟器是测试工具，仅通过测试 wiring
// 装配。生产装配仍走 bootstrap.go 的 switch。

// SimulatorConfig 描述一种设备模拟器的装配配置。
type SimulatorConfig struct {
	Producer  FrameProducer
	Responder CommandResponder
	AutoStart bool
	Channels  int
}

// ConfigForType 返回设备类型的模拟器配置（不含 TCP 监听）。
func ConfigForType(devType device.Type, channels int) SimulatorConfig {
	if channels <= 0 {
		channels = DefaultChannelsForType(string(devType))
	}
	switch devType {
	case device.DeviceDAQP1604:
		// 默认 producer 与 adapter 默认行为对齐：
		// adapter UseDeviceTimestampEnabled() 默认 true（nil 视为 true），
		// 故模拟器默认用带时间戳的 producer，使 ParseStreamFrameEx(withDeviceTimestamp=true) 能正确解析。
		return SimulatorConfig{P1604BinaryFrameProducerWithDeviceTimestamp, NewP1604Responder(), false, channels}
	case device.DeviceDaqT1603:
		return SimulatorConfig{T1603BinaryFrameProducer, NewT1603Responder(), false, channels}
	case device.DeviceDSA3217:
		return SimulatorConfig{DSA3217FrameProducer, NewDSA3217Responder(), false, channels}
	case device.DeviceDAQP1604Pre:
		return SimulatorConfig{P1604PreFrameProducer, nil, true, channels}
	case device.DeviceWTNPXI:
		return SimulatorConfig{WTNPXIFrameProducer, nil, true, channels}
	default:
		return SimulatorConfig{Channels: channels}
	}
}

// StartSimulatorForDeviceType 按设备类型启动对应模拟器（含 producer + responder），
// 返回已 Start 的 Simulator。调用方负责 Close（通常用 t.Cleanup）。
func StartSimulatorForDeviceType(devType device.Type, channels int) (Simulator, error) {
	cfg := ConfigForType(devType, channels)
	if cfg.Producer == nil {
		return nil, fmt.Errorf("sim: unsupported device type %q", devType)
	}
	sim := NewTCPSimulator(cfg.Producer, cfg.Responder, cfg.AutoStart, cfg.Channels)
	if err := sim.Start(context.Background()); err != nil {
		return nil, fmt.Errorf("start simulator for %s: %w", devType, err)
	}
	return sim, nil
}

// SplitAddr 将 "host:port" 拆分为 host 和 port。用于从模拟器监听地址构造 Profile。
func SplitAddr(addr string) (string, int) {
	idx := strings.LastIndex(addr, ":")
	if idx < 0 {
		return addr, 0
	}
	host := addr[:idx]
	port, _ := strconv.Atoi(addr[idx+1:])
	return host, port
}

// ProfileForSim 构造指向模拟器地址的 device.Profile，供真实 adapter 连接。
// channels 为通道数；id 为设备标识（建议唯一）。
func ProfileForSim(sim Simulator, devType device.Type, id string, channels int) device.Profile {
	host, port := SplitAddr(sim.Addr())
	if channels <= 0 {
		channels = DefaultChannelsForType(string(devType))
	}
	return device.Profile{
		ID:           id,
		Name:         fmt.Sprintf("Sim-%s", devType),
		Type:         devType,
		Transport:    "tcp",
		Address:      host,
		Port:         port,
		SamplingRate: 100,
		Channels:     MakeChannelConfigs(channels, devType),
	}
}

// MakeChannelConfigs 生成指定通道数的 ChannelConfig 列表，单位按设备类型选择。
func MakeChannelConfigs(channels int, devType device.Type) []device.ChannelConfig {
	unit := "Pa"
	if devType == device.DeviceDaqT1603 {
		unit = "°C"
	}
	out := make([]device.ChannelConfig, channels)
	for i := 0; i < channels; i++ {
		out[i] = device.ChannelConfig{
			Index:     i,
			Name:      fmt.Sprintf("CH%d", i),
			Enabled:   true,
			Unit:      unit,
			Precision: 3,
		}
	}
	return out
}

// WithSimulatedDevice 是测试 helper：启动模拟器、构造指向模拟器地址的 profile、
// 执行 fn、自动 Close。利用 DeviceManager 的 per-id 串行化与不同端口实现多设备并发。
//
// 用法：
//
//	sim.WithSimulatedDevice(t, device.DeviceDAQP1604, func(sim sim.Simulator, profile device.Profile) {
//	    adapter := hardware.NewDAQP1604(profile.ID, profile, logger)
//	    adapter.SetDataSink(...)
//	    _ = adapter.Connect()
//	    _ = adapter.StartAcquisition()
//	    // 断言 sink 收到帧...
//	})
func WithSimulatedDevice(t *testing.T, devType device.Type, fn func(sim Simulator, profile device.Profile)) {
	t.Helper()
	channels := DefaultChannelsForType(string(devType))
	sim, err := StartSimulatorForDeviceType(devType, channels)
	if err != nil {
		t.Fatalf("start simulator for %s: %v", devType, err)
	}
	t.Cleanup(func() { _ = sim.Close() })
	id := "sim-" + strings.ToLower(strings.ReplaceAll(string(devType), "-", ""))
	profile := ProfileForSim(sim, devType, id, channels)
	fn(sim, profile)
}
