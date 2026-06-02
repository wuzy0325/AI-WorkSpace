package config

import (
	"fmt"

	"wind-daq/services/api-go/internal/core/device"
)

// NewDefaultProfile 创建设备默认配置（含硬件特定地址和通道）
// 注意：此函数包含基础设施默认值（IP地址、端口等），属于 adapter 层职责
func NewDefaultProfile(id string, deviceType device.Type) device.Profile {
	profile := device.Profile{
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
	case device.DeviceDaqT1603:
		profile.Address = "192.168.3.101"
		profile.Port = 9000
		profile.Channels = defaultDaqT1603Channels()
		profile.DaqT1603Config = device.DaqT1603HardwareConfig{
			ThermocoupleType: "K",
			ColdJunction:     "internal",
			FilterHz:         50,
		}
	case device.DeviceDAQP1064Pre:
		profile.Address = "192.168.1.100"
		profile.Port = 5000
		profile.Channels = defaultDAQP1064PreChannels()
	case device.DeviceWTNPXI:
		profile.Address = "192.168.3.101"
		profile.Port = 9000
		profile.Channels = defaultWTNPXIChannels()
	case device.DeviceDSA3217:
		profile.Address = "192.168.1.254"
		profile.Port = 5000
		profile.Channels = defaultDSA3217Channels()
	}
	return profile
}

// NormalizeProfile 补全配置中的缺失字段（使用硬件默认值填充）
// 注意：此函数依赖硬件特定默认值，属于 adapter 层职责
func NormalizeProfile(profile device.Profile) device.Profile {
	if len(profile.Channels) == 0 {
		defaultProfile := NewDefaultProfile(profile.ID, profile.Type)
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
	return profile
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
		Index: 16, Name: "大气压", Enabled: true, Unit: "kPa",
		Precision: 3, RangeMin: 99, RangeMax: 106,
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

func defaultDAQP1064PreChannels() []device.ChannelConfig {
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

func defaultWTNPXIChannels() []device.ChannelConfig {
	names := []string{"球罐压力", "球罐总压", "球罐静压", "球罐温度1", "球罐温度2", "球罐温度3", "球罐温度4", "球罐温度5"}
	units := []string{"Pa", "Pa", "Pa", "degC", "degC", "degC", "degC", "degC"}
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
