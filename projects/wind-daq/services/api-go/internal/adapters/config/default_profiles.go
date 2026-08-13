package config

import (
	"fmt"

	"wind-daq/services/api-go/internal/core/device"
)

// NewDefaultProfile 创建设备默认配置（含硬件特定地址和通道）
// 注意：此函数包含基础设施默认值（IP地址、端口等），属于 adapter 层职责
func NewDefaultProfile(id string, deviceType device.Type) device.Profile {
	profile := device.Profile{
		Version:      device.CurrentProfileVersion,
		ID:           id,
		Name:         id,
		Type:         deviceType,
		Transport:    "tcp",
		SamplingRate: 20,
		AutoConnect:  true,
		BaudRate:     115200,
	}
	switch deviceType {
	case device.DeviceSimulated:
		profile.Channels = defaultSimulatedChannels()
	case device.DeviceDAQP1604:
		profile.Address = "192.168.3.101"
		profile.Port = 9000
		profile.Channels = defaultDAQP1604Channels()
	case device.DeviceDAQP1603:
		// DAQ-P-1603：DLL 内部封装 TCP，profile.Address 留空让用户在 UI 手动输入。
		// Port 字段对 1603 无意义（DLL 自管端口），保留 0 占位。
		// 默认采样率 100Hz（用户采样率=每秒数据条目数）。
		// 底层硬件采样率固定 1000Hz，100Hz 意味着每 10 个原始点取平均输出 1 条。
		profile.SamplingRate = 100
		profile.Channels = defaultDAQP1603Channels()
	case device.DeviceDaqT1603:
		profile.Address = "192.168.3.101"
		profile.Port = 9000
		profile.Channels = defaultDaqT1603Channels()
		profile.DaqT1603Config = device.DaqT1603HardwareConfig{
			ThermocoupleTypes: "KKKKKKKKKKKKKKKK",
			ChannelMask:       "FFFF",
			SamplingRate:      10,
			BinaryFormat:      false,
			AverageCount:      1,
		}
	case device.DeviceDaqT1602:
		// DAQ-T-1602：Modbus TCP 温度扫描阀，默认 IP/端口来自 spec-daq-t1602 真机实测。
		// 固件固定 ~100ms 采集周期，无采样率配置；默认 16 通道全 T 型（type code 2，
		// 与设备出厂现值一致）。
		profile.Address = "192.168.3.201"
		profile.Port = 502
		profile.Channels = defaultDaqT1602Channels()
		profile.DaqT1602Config = defaultDaqT1602Config()
	case device.DeviceDAQP1604Pre:
		// 实测 DAQ-P-1604Pre 默认 IP/Port（与 Cursor DAQ 一致）
		profile.Address = "192.168.3.232"
		profile.Port = 23
		profile.Channels = defaultDAQP1604PreChannels()
	case device.DeviceWTNPXI:
		profile.Address = "192.168.3.101"
		profile.Port = 9000
		profile.Channels = defaultWTNPXIChannels()
	case device.DeviceDSA3217:
		profile.Address = "192.168.1.254"
		profile.Port = 5000
		profile.Channels = defaultDSA3217Channels()
	}
	converter := device.NewUnitConverter()
	for i := range profile.Channels {
		// 默认规则：支持校零的单位 → 启用校零。
		// DAQ-P-1603 设备特殊默认：CH01/CH02（index 0/1）强制关闭校零应用，
		// 覆盖按单位推断的结果。前两路通常接入参与校零的传感器
		// （如总压/静压参考通道），与 DAQ-P-1604"全部启用"默认区分。
		profile.Channels[i].CalibrationEnabled = converter.SupportsZeroCalibration(profile.Channels[i].Unit)
		if profile.Type == device.DeviceDAQP1603 && i < 2 {
			profile.Channels[i].CalibrationEnabled = false
		}
	}
	return profile
}

// NormalizeProfile 补全配置中的缺失字段（使用硬件默认值填充）
// 注意：此函数依赖硬件特定默认值，属于 adapter 层职责。
//
// 执行顺序：先 v1→v2 迁移（TareOffset→CalibrationOffset），再补全缺失字段。
// 迁移幂等：Version >= 2 跳过。
func NormalizeProfile(profile device.Profile) device.Profile {
	// v1→v2 迁移：TareOffset → CalibrationOffset（在补全默认值之前执行，
	// 因为旧 TareOffset 需在通道被重置前完成迁移）
	profile = migrateProfile(profile)
	converter := device.NewUnitConverter()
	for i := range profile.Channels {
		supports := converter.SupportsZeroCalibration(profile.Channels[i].Unit)
		if profile.Type != device.DeviceDAQP1603 {
			profile.Channels[i].CalibrationEnabled = supports
		}
	}

	needsDefaultProfile := len(profile.Channels) == 0 || profile.Type == device.DeviceDaqT1603 || profile.Type == device.DeviceDaqT1602
	var defaultProfile device.Profile
	if needsDefaultProfile {
		defaultProfile = NewDefaultProfile(profile.ID, profile.Type)
	}

	if len(profile.Channels) == 0 {
		profile.Channels = defaultProfile.Channels
		if profile.Transport == "" {
			profile.Transport = defaultProfile.Transport
		}
		if profile.Address == "" {
			profile.Address = defaultProfile.Address
		}
		if profile.Port == 0 {
			profile.Port = defaultProfile.Port
		}
		if profile.BaudRate == 0 {
			profile.BaudRate = defaultProfile.BaudRate
		}
		if profile.SamplingRate == 0 {
			profile.SamplingRate = defaultProfile.SamplingRate
		}
	}
	if profile.Type == device.DeviceDaqT1603 {
		profile.DaqT1603Config = normalizeDaqT1603Config(profile.DaqT1603Config, defaultProfile.DaqT1603Config)
	}
	if profile.Type == device.DeviceDaqT1602 {
		profile.DaqT1602Config = normalizeDaqT1602Config(profile.DaqT1602Config, defaultProfile.DaqT1602Config)
	}
	// DAQ-P-1604Pre 兼容升级：旧 profile 仅 16 通道压力，不包含气象通道。
	// 此处补齐 Atm (Index=16) 与 AtmTemp (Index=17)，保留用户对前 16 通道的自定义。
	// 不整体重置 channels，避免覆盖用户已调整的精度/单位/量程。
	if profile.Type == device.DeviceDAQP1604Pre {
		profile.Channels = ensureP1604PreAtmosphericChannels(profile.Channels)
	}
	return profile
}

// ensureP1604PreAtmosphericChannels 确保 1604Pre profile 含 18 通道。
// 行为：
//   - 若已存在 Index==P1604PreAtmChannelIndex / Index==P1604PreAtmTempChannelIndex 的通道，认为已升级，直接返回
//   - 否则在末尾追加缺失的气象通道（默认配置由 defaultP1604PreAtmChannel / defaultP1604PreAtmTempChannel 提供）
//
// 设计原则：升级是幂等的，多次调用结果一致；不破坏用户对前 16 通道的自定义。
// 注意：判定仅看 Index 是否存在，不校验 Name/SensorType —— 用户若自定义了 Index=16 的非 Atm 通道，
// 视为已升级跳过追加（保留用户自定义优先于自动补齐）。
func ensureP1604PreAtmosphericChannels(channels []device.ChannelConfig) []device.ChannelConfig {
	hasAtm := false
	hasAtmTemp := false
	for i := range channels {
		switch channels[i].Index {
		case device.P1604PreAtmChannelIndex:
			hasAtm = true
		case device.P1604PreAtmTempChannelIndex:
			hasAtmTemp = true
		}
	}
	if hasAtm && hasAtmTemp {
		return channels
	}
	if !hasAtm {
		channels = append(channels, defaultP1604PreAtmChannel())
	}
	if !hasAtmTemp {
		channels = append(channels, defaultP1604PreAtmTempChannel())
	}
	return channels
}

func normalizeDaqT1603Config(config device.DaqT1603HardwareConfig, defaults device.DaqT1603HardwareConfig) device.DaqT1603HardwareConfig {
	if config.ThermocoupleTypes == "" {
		config.ThermocoupleTypes = defaults.ThermocoupleTypes
	}
	if config.ChannelMask == "" {
		config.ChannelMask = defaults.ChannelMask
	}
	if config.SamplingRate == 0 {
		config.SamplingRate = defaults.SamplingRate
	}
	if config.AverageCount == 0 {
		config.AverageCount = defaults.AverageCount
	}
	return config
}

// normalizeDaqT1602Config 回填缺失的 T1602 配置默认值。
// TypeCodes 是定长数组无空值语义，约定"全零"视为未设置（16 通道全 J 型的合法
// 配置与零值无法区分，实践中不会出现；且 Connect 后驱动会从设备读回实际类型
// 经 OnConfigSynced 覆盖本地值）。
func normalizeDaqT1602Config(config device.DaqT1602HardwareConfig, defaults device.DaqT1602HardwareConfig) device.DaqT1602HardwareConfig {
	allZero := true
	for _, code := range config.TypeCodes {
		if code != 0 {
			allZero = false
			break
		}
	}
	if allZero {
		return defaults
	}
	return config
}

// defaultDaqT1602Config 默认 16 通道全为 T 型热电偶（type code 2，与设备出厂现值一致）。
func defaultDaqT1602Config() device.DaqT1602HardwareConfig {
	var cfg device.DaqT1602HardwareConfig
	for i := range cfg.TypeCodes {
		cfg.TypeCodes[i] = 2
	}
	return cfg
}

func defaultSimulatedChannels() []device.ChannelConfig {
	channels := make([]device.ChannelConfig, 18)
	for i := 0; i < 16; i++ {
		channels[i] = device.ChannelConfig{
			Index:     i,
			Name:      fmt.Sprintf("CH%d", i+1),
			Enabled:   true,
			Unit:      "V",
			Precision: 3,
			RangeMin:  -10,
			RangeMax:  10,
		}
	}
	channels[16] = device.ChannelConfig{
		Index: 16, Name: "大气压", Enabled: true, Unit: "Pa",
		Precision: 1, RangeMin: 99000, RangeMax: 106000,
	}
	channels[17] = device.ChannelConfig{
		Index: 17, Name: "大气温度", Enabled: true, Unit: "degC",
		Precision: 2, RangeMin: 20, RangeMax: 25,
	}
	return channels
}

func defaultDaqT1603Channels() []device.ChannelConfig {
	channels := make([]device.ChannelConfig, 16)
	for i := range channels {
		channels[i] = device.ChannelConfig{
			Index:     i,
			Name:      fmt.Sprintf("TC%d", i+1),
			Enabled:   true,
			Unit:      "degC",
			Precision: 2,
		}
	}
	return channels
}

// defaultDaqT1602Channels 生成 DAQ-T-1602 的 16 通道默认配置（TC1..TC16，degC）。
// 索引 0~7 为卡1（Unit ID 1），索引 8~15 为卡2（Unit ID 2）。
func defaultDaqT1602Channels() []device.ChannelConfig {
	channels := make([]device.ChannelConfig, 16)
	for i := range channels {
		channels[i] = device.ChannelConfig{
			Index:     i,
			Name:      fmt.Sprintf("TC%d", i+1),
			Enabled:   true,
			Unit:      "degC",
			Precision: 2,
		}
	}
	return channels
}

func defaultDAQP1604Channels() []device.ChannelConfig {
	channels := make([]device.ChannelConfig, 18)
	for i := 0; i < 16; i++ {
		channels[i] = device.ChannelConfig{
			Index:     i,
			Name:      fmt.Sprintf("CH%d", i+1),
			Enabled:   true,
			Unit:      "Pa",
			Precision: 2,
			RangeMin:  -5000,
			RangeMax:  5000,
		}
	}
	channels[16] = device.ChannelConfig{Index: 16, Name: "大气压", Enabled: false, Unit: "Pa", Precision: 2}
	channels[17] = device.ChannelConfig{Index: 17, Name: "大气温度", Enabled: false, Unit: "degC", Precision: 2}
	return channels
}

// defaultDAQP1604PreChannels 生成 DAQ-P-1604Pre 的 18 通道默认配置。
//
// 通道布局（与 1604Pre 协议数据帧一致）：
//   - CH1..CH16：16 路压力通道（payload[8..71]，小端 float32，单位 Pa）
//   - Atm：大气压通道（payload[0..3]，单位 Pa，典型值 ~101325）
//   - AtmTemp：大气温度通道（payload[4..7]，单位 °C，典型值 ~25）
//
// 设计说明：
//   - 气象通道放在通道列表末尾（Index=16/17），保持 16 路压力通道索引与历史 profile 兼容
//   - adapter handleAcquisitionDataLocked 按 Index 决定 payload 偏移：
//     Index<16 读 payload[8+i*4]，Index==16 读 payload[0]，Index==17 读 payload[4]
//   - 旧 profile（仅 16 通道）仍可工作，只是不显示气象数据
//   - 气象通道构造复用 defaultP1604PreAtmChannel / defaultP1604PreAtmTempChannel，
//     ensureP1604PreAtmosphericChannels 也引用这两个函数，保证单点修改
func defaultDAQP1604PreChannels() []device.ChannelConfig {
	channels := make([]device.ChannelConfig, 0, device.P1604PrePressureChannelCount+2)
	// 16 路压力通道
	for i := 0; i < device.P1604PrePressureChannelCount; i++ {
		channels = append(channels, device.ChannelConfig{
			Index:     i,
			Name:      fmt.Sprintf("CH%d", i+1),
			Enabled:   true,
			Unit:      "Pa",
			Precision: 2,
			RangeMin:  -5000,
			RangeMax:  5000,
		})
	}
	channels = append(channels, defaultP1604PreAtmChannel())
	channels = append(channels, defaultP1604PreAtmTempChannel())
	return channels
}

// defaultP1604PreAtmChannel 返回 1604Pre 大气压通道默认配置。
// 单独提取以供 defaultDAQP1604PreChannels 与 ensureP1604PreAtmosphericChannels 复用，
// 避免通道属性散落多处导致修改时漂移。
func defaultP1604PreAtmChannel() device.ChannelConfig {
	return device.ChannelConfig{
		Index:      device.P1604PreAtmChannelIndex,
		Name:       "Atm",
		Enabled:    true,
		Unit:       "Pa",
		Precision:  1,
		RangeMin:   80000,
		RangeMax:   120000,
		SensorType: device.SensorPressure,
	}
}

// defaultP1604PreAtmTempChannel 返回 1604Pre 大气温度通道默认配置。
func defaultP1604PreAtmTempChannel() device.ChannelConfig {
	return device.ChannelConfig{
		Index:      device.P1604PreAtmTempChannelIndex,
		Name:       "AtmTemp",
		Enabled:    true,
		Unit:       "°C",
		Precision:  2,
		RangeMin:   -40,
		RangeMax:   85,
		SensorType: device.SensorTemperature,
	}
}

// defaultDAQP1603Channels 生成 DAQ-P-1603 的 16 通道默认配置。
// 与 DAQ-P-1604 不同：1603 不包含大气压/温度通道（用户决策无大气数据），
// 每通道默认单位 Pa，精度 3 位小数（适配 ±10V 量程下的小信号）。
// 通道传感器类型（pressure/temperature）由前端 DaqP1603Config.vue 配置，
// 不在默认 profile 中预设，避免与 ChannelConfig.SensorType 反序列化兜底逻辑冲突。
//
// 设备特殊默认：CH01/CH02（index 0/1）默认不应用校零（CalibrationEnabled=false）。
// 业务原因：DAQ-P-1603 前两路通常接入不参与校零的传感器（如总压/静压参考通道），
// 与其他 16 通道压力设备（DAQ-P-1604）的"全部启用校零"默认行为有意区分。
// 注意：此处的值会被下方 NewDefaultProfile 末尾的"按单位 SupportsZeroCalibration 重置"
// 覆盖，因此把 1603 的关闭逻辑挪到该重置循环中按 Type 判断，确保不被覆盖。
func defaultDAQP1603Channels() []device.ChannelConfig {
	channels := make([]device.ChannelConfig, 16)
	for i := range channels {
		channels[i] = device.ChannelConfig{
			Index:     i,
			Name:      fmt.Sprintf("CH%d", i+1),
			Enabled:   true,
			Unit:      "Pa",
			Precision: 3,
			RangeMin:  -5000,
			RangeMax:  5000,
		}
	}
	return channels
}

func defaultWTNPXIChannels() []device.ChannelConfig {
	// 通道 3 是球罐稳定时间（秒），由 sphere tank gate 逻辑读取并解析为稳定时间，
	// 通道 4~7 是 4 路球罐温度。原"球罐温度1"位置被稳定时间通道占用，温度顺延。
	names := []string{"球罐压力", "球罐总压", "球罐静压", "球罐稳定时间", "球罐温度1", "球罐温度2", "球罐温度3", "球罐温度4"}
	units := []string{"Pa", "Pa", "Pa", "s", "degC", "degC", "degC", "degC"}
	channels := make([]device.ChannelConfig, 8)
	for i := range channels {
		channels[i] = device.ChannelConfig{
			Index:     i,
			Name:      names[i],
			Enabled:   true,
			Unit:      units[i],
			Precision: 2,
		}
	}
	return channels
}

func defaultDSA3217Channels() []device.ChannelConfig {
	channels := make([]device.ChannelConfig, 16)
	for i := range channels {
		channels[i] = device.ChannelConfig{
			Index:     i,
			Name:      fmt.Sprintf("CH%d", i+1),
			Enabled:   true,
			Unit:      "Pa",
			Precision: 2,
			RangeMin:  -5000,
			RangeMax:  5000,
		}
	}
	return channels
}
