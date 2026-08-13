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

func TestDefaultDaqT1602ProfileHasTemperatureChannels(t *testing.T) {
	profile := NewDefaultProfile("temp-t1602", device.DeviceDaqT1602)

	if profile.Address != "192.168.3.201" {
		t.Fatalf("expected default address 192.168.3.201, got %q", profile.Address)
	}
	if profile.Port != 502 {
		t.Fatalf("expected default port 502, got %d", profile.Port)
	}
	if len(profile.Channels) != 16 {
		t.Fatalf("expected 16 default channels, got %d", len(profile.Channels))
	}
	if profile.Channels[15].Name != "TC16" {
		t.Fatalf("expected last channel name TC16, got %q", profile.Channels[15].Name)
	}
	for i, code := range profile.DaqT1602Config.TypeCodes {
		if code != 2 {
			t.Fatalf("expected default typeCode 2 (T) at channel %d, got %d", i, code)
		}
	}
}

func TestNormalizeProfileBackfillsDaqT1602Config(t *testing.T) {
	profile := NewDefaultProfile("temp-t1602", device.DeviceDaqT1602)
	profile.DaqT1602Config = device.DaqT1602HardwareConfig{} // 模拟缺失 daqT1602Config 的 profile

	normalized := NormalizeProfile(profile)

	for i, code := range normalized.DaqT1602Config.TypeCodes {
		if code != 2 {
			t.Fatalf("expected backfilled typeCode 2 (T) at channel %d, got %d", i, code)
		}
	}
}

func TestNormalizeProfilePreservesDaqT1602ConfigValues(t *testing.T) {
	profile := NewDefaultProfile("temp-t1602", device.DeviceDaqT1602)
	custom := defaultDaqT1602Config()
	custom.TypeCodes[0] = 1 // K 型
	custom.TypeCodes[8] = 3 // E 型（卡2 CH0）
	profile.DaqT1602Config = custom

	normalized := NormalizeProfile(profile)

	if normalized.DaqT1602Config != custom {
		t.Fatalf("expected config to be preserved, got %+v", normalized.DaqT1602Config)
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
// 采样率 100Hz（用户采样率=每秒数据条目数，底层硬件采样率固定 1000Hz）。
// 设备特殊默认：CH01/CH02（index 0/1）默认不应用校零（CalibrationEnabled=false），
// 其余通道（index 2~15）启用校零。
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
		// DAQ-P-1603 设备特殊默认：CH01/CH02（index 0/1）不应用校零
		wantCalibrationEnabled := i >= 2
		if ch.CalibrationEnabled != wantCalibrationEnabled {
			t.Fatalf("expected channel %d calibrationEnabled=%v, got %v", i, wantCalibrationEnabled, ch.CalibrationEnabled)
		}
	}
	if profile.Version != device.CurrentProfileVersion {
		t.Fatalf("expected profile version %d, got %d", device.CurrentProfileVersion, profile.Version)
	}
	// Address 留空：DLL 内部封装 TCP，IP 由用户在 UI 手动输入
	if profile.Address != "" {
		t.Fatalf("expected empty address for manual IP input, got %q", profile.Address)
	}
	if profile.SamplingRate != 100 {
		t.Fatalf("expected default sampling rate 100, got %d", profile.SamplingRate)
	}
}

func TestNormalizeProfileMigratesTareOffsetToBaseUnit(t *testing.T) {
	profile := device.Profile{
		ID:      "legacy",
		Type:    device.DeviceDAQP1604,
		Version: 1,
		Channels: []device.ChannelConfig{{
			Index: 0, Enabled: true, Unit: "kPa", TareOffset: 1.25,
		}},
	}

	got := NormalizeProfile(profile)
	channel := got.Channels[0]
	if channel.CalibrationOffset != 1250 {
		t.Fatalf("expected 1.25 kPa migrated to 1250 Pa, got %v", channel.CalibrationOffset)
	}
	if channel.TareOffset != 0 || channel.CalibrationUnit != "kPa" || !channel.CalibrationEnabled {
		t.Fatalf("unexpected migrated channel: %+v", channel)
	}
	if got.Version != device.CurrentProfileVersion {
		t.Fatalf("expected version %d, got %d", device.CurrentProfileVersion, got.Version)
	}
}

func TestNormalizeProfileDropsLegacyTemperatureTare(t *testing.T) {
	profile := device.Profile{
		ID:      "legacy-temperature",
		Type:    device.DeviceDaqT1603,
		Version: 1,
		Channels: []device.ChannelConfig{{
			Index: 0, Enabled: true, Unit: "degC", TareOffset: 25,
		}},
	}

	got := NormalizeProfile(profile).Channels[0]
	if got.TareOffset != 0 || got.CalibrationOffset != 0 || got.CalibrationEnabled {
		t.Fatalf("temperature tare must be removed during migration, got %+v", got)
	}
}

func TestNormalizeProfileClearsVersionTwoTemperatureCalibration(t *testing.T) {
	profile := device.Profile{
		ID:      "temperature-v2",
		Type:    device.DeviceDAQP1603,
		Version: device.CurrentProfileVersion,
		Channels: []device.ChannelConfig{{
			Index: 0, Unit: "℃", SensorType: device.SensorTemperature,
			CalibrationOffset: 25, CalibrationUnit: "℃", CalibrationAt: 123,
			CalibrationEnabled: true,
		}},
	}

	got := NormalizeProfile(profile).Channels[0]
	if got.CalibrationOffset != 25 || got.CalibrationUnit != "℃" || got.CalibrationAt != 123 {
		t.Fatalf("calibration data must be preserved (safety enforced by Applier/Sampler/API), got %+v", got)
	}
	if !got.CalibrationEnabled {
		t.Fatalf("DAQ-P-1603 calibration enabled must be preserved as user-configured, got %+v", got)
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

// TestMigrateProfile_DaqP1603_PreservesTareOffsetOnFirstChannels 验证 Critical
// 回归场景：v1 时代用户对 DAQ-P-1603 CH01（index 0，本应默认关闭校零）做过校零，
// profile 中 TareOffset 非零。v1→v2 迁移必须
//   - 将 TareOffset 换算为 CalibrationOffset（kPa → Pa）
//   - 强制 CalibrationEnabled = true（用户显式校零优先于"CH01/CH02 默认关闭"规则）
//   - 清空 TareOffset，补齐 CalibrationUnit / CalibrationAt
// 回归症状：若迁移逻辑把"默认关闭"放在 TareOffset 迁移之前且不重新置 true，
// CalibrationApplier 会因 CalibrationEnabled=false 跳过该通道，用户历史校零静默失效。
func TestMigrateProfile_DaqP1603_PreservesTareOffsetOnFirstChannels(t *testing.T) {
	profile := device.Profile{
		ID:      "p1603-legacy",
		Type:    device.DeviceDAQP1603,
		Version: 1,
		Channels: []device.ChannelConfig{
			{Index: 0, Enabled: true, Unit: "kPa", TareOffset: 1.25}, // CH01，用户曾校零
			{Index: 1, Enabled: true, Unit: "Pa", TareOffset: 0},     // CH02，未校零 → 默认关闭
			{Index: 2, Enabled: true, Unit: "Pa", TareOffset: 0},     // CH03，未校零 → 默认启用
		},
	}

	got := NormalizeProfile(profile)

	if got.Version != device.CurrentProfileVersion {
		t.Fatalf("expected version %d, got %d", device.CurrentProfileVersion, got.Version)
	}

	ch0 := got.Channels[0]
	if ch0.CalibrationOffset != 1250 {
		t.Errorf("CH01: expected 1.25 kPa migrated to 1250 Pa, got %v", ch0.CalibrationOffset)
	}
	if ch0.TareOffset != 0 {
		t.Errorf("CH01: expected TareOffset cleared, got %v", ch0.TareOffset)
	}
	if ch0.CalibrationUnit != "kPa" {
		t.Errorf("CH01: expected CalibrationUnit kPa, got %q", ch0.CalibrationUnit)
	}
	if ch0.CalibrationAt == 0 {
		t.Errorf("CH01: expected CalibrationAt set, got 0")
	}
	if !ch0.CalibrationEnabled {
		t.Errorf("CH01: expected CalibrationEnabled=true (user explicit calibration must override device default), got false")
	}

	// CH02：v1 未校零，应用 1603 设备特殊默认（关闭）
	if got.Channels[1].CalibrationEnabled {
		t.Errorf("CH02: expected CalibrationEnabled=false (DAQ-P-1603 default), got true")
	}
	if got.Channels[1].CalibrationOffset != 0 {
		t.Errorf("CH02: expected CalibrationOffset=0, got %v", got.Channels[1].CalibrationOffset)
	}

	// CH03：v1 未校零，按单位推断 → 启用
	if !got.Channels[2].CalibrationEnabled {
		t.Errorf("CH03: expected CalibrationEnabled=true (Pa supports zero calibration), got false")
	}
}

// TestMigrateProfile_DaqP1603_DefaultDisablesFirstTwoChannels 验证 v1 1603 profile
// 在没有任何 TareOffset 的情况下迁移后，CH01/CH02 应用设备特殊默认（关闭校零），
// 其余通道按单位推断启用。覆盖"新设备从未校零"的常见路径。
func TestMigrateProfile_DaqP1603_DefaultDisablesFirstTwoChannels(t *testing.T) {
	profile := device.Profile{
		ID:      "p1603-clean",
		Type:    device.DeviceDAQP1603,
		Version: 1,
		Channels: []device.ChannelConfig{
			{Index: 0, Enabled: true, Unit: "Pa", TareOffset: 0},
			{Index: 1, Enabled: true, Unit: "Pa", TareOffset: 0},
			{Index: 2, Enabled: true, Unit: "Pa", TareOffset: 0},
			{Index: 3, Enabled: true, Unit: "Pa", TareOffset: 0},
		},
	}

	got := NormalizeProfile(profile)

	for i, ch := range got.Channels {
		wantEnabled := i >= 2
		if ch.CalibrationEnabled != wantEnabled {
			t.Errorf("channel %d: expected CalibrationEnabled=%v, got %v", i, wantEnabled, ch.CalibrationEnabled)
		}
		if ch.CalibrationOffset != 0 {
			t.Errorf("channel %d: expected CalibrationOffset=0, got %v", i, ch.CalibrationOffset)
		}
	}
}
