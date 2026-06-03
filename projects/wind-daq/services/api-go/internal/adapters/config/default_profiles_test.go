package config

import (
	"fmt"
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
