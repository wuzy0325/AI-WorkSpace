package config

import (
	"fmt"
	"reflect"
	"testing"

	"wind-daq/services/api-go/internal/core/device"
)

func TestDefaultSimulatedProfileHasEnabledChannels(t *testing.T) {
	profile := NewDefaultProfile("sim-1", device.DeviceSimulated)

	if profile.ID != "sim-1" {
		t.Fatalf("expected profile id sim-1, got %q", profile.ID)
	}
	if profile.Type != device.DeviceSimulated {
		t.Fatalf("expected simulated type, got %q", profile.Type)
	}
	if len(profile.Channels) != 18 {
		t.Fatalf("expected 18 default channels, got %d", len(profile.Channels))
	}
	for i, channel := range profile.Channels {
		if !channel.Enabled {
			t.Fatalf("expected channel %d to be enabled", channel.Index)
		}
		if channel.Unit == "" {
			t.Fatalf("expected channel %d to have a unit", channel.Index)
		}
		if i < 16 && channel.Name != fmt.Sprintf("CH%d", i+1) {
			t.Fatalf("expected channel %d name CH%d, got %q", i, i+1, channel.Name)
		}
	}
	if profile.Channels[16].Name != "大气压" {
		t.Fatalf("expected channel 16 name 大气压, got %q", profile.Channels[16].Name)
	}
	if profile.Channels[17].Name != "大气温度" {
		t.Fatalf("expected channel 17 name 大气温度, got %q", profile.Channels[17].Name)
	}
}

func TestDefaultDaqT1603ProfileHasTemperatureChannels(t *testing.T) {
	profile := NewDefaultProfile("temp-1", device.DeviceDaqT1603)

	if len(profile.Channels) != 16 {
		t.Fatalf("expected 16 default channels, got %d", len(profile.Channels))
	}
	if profile.Channels[15].Name != "TC16" {
		t.Fatalf("expected last channel name TC16, got %q", profile.Channels[15].Name)
	}
	if profile.DaqT1603Config.ThermocoupleTypes != "KKKKKKKKKKKKKKKK" {
		t.Fatalf("expected default thermocouple types, got %q", profile.DaqT1603Config.ThermocoupleTypes)
	}
}

// 测试前置：调用 NewDefaultProfile 生成 DAQ-P-1603 默认 profile。
// 期待结果：16 通道全部启用，单位 Pa，精度 3，地址留空（由 UI 手动输入），
// 采样率 500Hz。
func TestDefaultDaqP1603ProfileHasPressureChannels(t *testing.T) {
	profile := NewDefaultProfile("p1603-1", device.DeviceDAQP1603)

	if profile.Type != device.DeviceDAQP1603 {
		t.Fatalf("expected type DAQ-P-1603, got %q", profile.Type)
	}
	if len(profile.Channels) != 16 {
		t.Fatalf("expected 16 default channels, got %d", len(profile.Channels))
	}
	for i, ch := range profile.Channels {
		if !ch.Enabled {
			t.Fatalf("expected channel %d to be enabled", i)
		}
		if ch.Unit != "Pa" {
			t.Fatalf("expected channel %d unit Pa, got %q", i, ch.Unit)
		}
		if ch.Precision != 3 {
			t.Fatalf("expected channel %d precision 3, got %d", i, ch.Precision)
		}
		if ch.Name != fmt.Sprintf("CH%d", i+1) {
			t.Fatalf("expected channel %d name CH%d, got %q", i, i+1, ch.Name)
		}
	}
	// Address 留空：DLL 内部封装 TCP，IP 由用户在 UI 手动输入
	if profile.Address != "" {
		t.Fatalf("expected empty address for manual IP input, got %q", profile.Address)
	}
	if profile.SamplingRate != 500 {
		t.Fatalf("expected default sampling rate 500, got %d", profile.SamplingRate)
	}
}

func TestNormalizeProfileRestoresDefaultChannels(t *testing.T) {
	profile := device.Profile{
		ID:           "legacy-sim",
		Name:         "Legacy Simulator",
		Type:         device.DeviceSimulated,
		SamplingRate: 20,
	}

	normalized := NormalizeProfile(profile)

	if len(normalized.Channels) != 18 {
		t.Fatalf("expected 18 default channels, got %d", len(normalized.Channels))
	}
	if normalized.Name != "Legacy Simulator" {
		t.Fatalf("expected name to be preserved, got %q", normalized.Name)
	}
}

func TestNormalizeProfileBackfillsDaqT1603ConfigWhenChannelsExist(t *testing.T) {
	profile := NewDefaultProfile("temp-1", device.DeviceDaqT1603)
	profile.DaqT1603Config = device.DaqT1603HardwareConfig{}

	normalized := NormalizeProfile(profile)

	if normalized.DaqT1603Config.ThermocoupleTypes != "KKKKKKKKKKKKKKKK" {
		t.Fatalf("expected default thermocouple types, got %q", normalized.DaqT1603Config.ThermocoupleTypes)
	}
	if normalized.DaqT1603Config.ChannelMask != "FFFF" {
		t.Fatalf("expected default channel mask, got %q", normalized.DaqT1603Config.ChannelMask)
	}
	if normalized.DaqT1603Config.SamplingRate != 10 {
		t.Fatalf("expected default sampling rate, got %d", normalized.DaqT1603Config.SamplingRate)
	}
	if normalized.DaqT1603Config.AverageCount != 1 {
		t.Fatalf("expected default average count, got %d", normalized.DaqT1603Config.AverageCount)
	}
}

// TestNormalizeProfileUpgradesP1604PreLegacy16Channels 验证 1604Pre 旧 profile（仅 16 通道）
// 在 NormalizeProfile 后自动补齐 Atm / AtmTemp 通道，且保留前 16 通道的用户自定义。
func TestNormalizeProfileUpgradesP1604PreLegacy16Channels(t *testing.T) {
	// 构造旧 profile：16 通道压力，单位改为 kPa 验证不被覆盖
	legacy := device.Profile{
		ID:   "legacy-1604pre",
		Name: "Legacy 1604Pre",
		Type: device.DeviceDAQP1604Pre,
	}
	legacy.Channels = make([]device.ChannelConfig, 16)
	for i := 0; i < 16; i++ {
		legacy.Channels[i] = device.ChannelConfig{
			Index: i, Name: "CH" + string(rune('A'+i)), Enabled: true,
			Unit: "kPa", Precision: 3,
		}
	}

	normalized := NormalizeProfile(legacy)

	if len(normalized.Channels) != 18 {
		t.Fatalf("expected 18 channels after upgrade, got %d", len(normalized.Channels))
	}
	// 前 16 通道自定义应保留
	if normalized.Channels[0].Unit != "kPa" {
		t.Fatalf("expected CH0 unit preserved as kPa, got %q", normalized.Channels[0].Unit)
	}
	if normalized.Channels[0].Name != "CHA" {
		t.Fatalf("expected CH0 name preserved as CHA, got %q", normalized.Channels[0].Name)
	}
	// 末两位应为 Atm / AtmTemp
	if normalized.Channels[16].Index != 16 || normalized.Channels[16].Name != "Atm" {
		t.Fatalf("expected Atm channel at index 16, got %+v", normalized.Channels[16])
	}
	if normalized.Channels[17].Index != 17 || normalized.Channels[17].Name != "AtmTemp" {
		t.Fatalf("expected AtmTemp channel at index 17, got %+v", normalized.Channels[17])
	}
	if normalized.Channels[16].SensorType != device.SensorPressure {
		t.Fatalf("expected Atm sensorType pressure, got %q", normalized.Channels[16].SensorType)
	}
	if normalized.Channels[17].SensorType != device.SensorTemperature {
		t.Fatalf("expected AtmTemp sensorType temperature, got %q", normalized.Channels[17].SensorType)
	}
}

// TestNormalizeProfileP1604PreIdempotent 验证升级是幂等的：
// 已含 18 通道的 profile 再次 Normalize 不会重复追加或改变通道属性。
// 断言用 reflect.DeepEqual 验证内容（而非仅长度），覆盖二次调用意外修改 Name/Precision/SensorType 等场景。
func TestNormalizeProfileP1604PreIdempotent(t *testing.T) {
	profile := NewDefaultProfile("pre-1", device.DeviceDAQP1604Pre)

	normalized := NormalizeProfile(profile)
	if len(normalized.Channels) != 18 {
		t.Fatalf("expected 18 channels, got %d", len(normalized.Channels))
	}
	// 二次 Normalize，验证通道内容完全一致（含 Name/Index/SensorType/Unit/Precision/Range 等）
	normalized2 := NormalizeProfile(normalized)
	if !reflect.DeepEqual(normalized.Channels, normalized2.Channels) {
		t.Fatalf("expected idempotent normalize, channels changed on second call:\nfirst:  %+v\nsecond: %+v",
			normalized.Channels, normalized2.Channels)
	}
}

// TestNormalizeProfileP1604PrePreservesCustomIndex16 验证边界情况：
// 用户已自定义 Index=16 为非 Atm 通道时，Normalize 不会覆盖用户自定义（视为已升级跳过追加）。
// 这是文档化的设计行为：判定仅看 Index 是否存在，保留用户自定义优先于自动补齐。
func TestNormalizeProfileP1604PrePreservesCustomIndex16(t *testing.T) {
	profile := device.Profile{
		ID:   "custom-1604pre",
		Name: "Custom 1604Pre",
		Type: device.DeviceDAQP1604Pre,
	}
	// 16 路压力 + 用户自定义 Index=16 为 "MyCustom"（非 Atm）
	profile.Channels = make([]device.ChannelConfig, 17)
	for i := 0; i < 16; i++ {
		profile.Channels[i] = device.ChannelConfig{
			Index: i, Name: fmt.Sprintf("CH%d", i+1), Enabled: true,
			Unit: "Pa", Precision: 2,
		}
	}
	profile.Channels[16] = device.ChannelConfig{
		Index: 16, Name: "MyCustom", Enabled: true, Unit: "V", Precision: 3,
	}

	normalized := NormalizeProfile(profile)

	// Index=16 用户自定义应保留，不被 Atm 覆盖
	if normalized.Channels[16].Name != "MyCustom" {
		t.Fatalf("expected custom channel at index 16 preserved, got name %q", normalized.Channels[16].Name)
	}
	if normalized.Channels[16].Unit != "V" {
		t.Fatalf("expected custom channel unit V preserved, got %q", normalized.Channels[16].Unit)
	}
	// Index=17 应被追加（AtmTemp）
	if len(normalized.Channels) != 18 {
		t.Fatalf("expected 18 channels (custom + AtmTemp appended), got %d", len(normalized.Channels))
	}
	if normalized.Channels[17].Index != device.P1604PreAtmTempChannelIndex || normalized.Channels[17].Name != "AtmTemp" {
		t.Fatalf("expected AtmTemp appended at index 17, got %+v", normalized.Channels[17])
	}
}

func TestNormalizeProfilePreservesDaqT1603ConfigValues(t *testing.T) {
	profile := NewDefaultProfile("temp-1", device.DeviceDaqT1603)
	profile.DaqT1603Config = device.DaqT1603HardwareConfig{
		ThermocoupleTypes: "SSSSSSSSSSSSSSSS",
		ChannelMask:       "00FF",
		SamplingRate:      20,
		AverageCount:      4,
	}

	normalized := NormalizeProfile(profile)

	if normalized.DaqT1603Config != profile.DaqT1603Config {
		t.Fatalf("expected config to be preserved, got %+v", normalized.DaqT1603Config)
	}
}
