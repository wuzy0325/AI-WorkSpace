package config

import (
	"time"

	"wind-daq/services/api-go/internal/core/device"
)

// MigrateProfiles 将 profile 数组从旧格式迁移到新格式（v1→v2）。
// 幂等：Version >= 2 的 profile 跳过。迁移在 DeviceManager 加载 profile 时调用。
//
// 迁移内容：
//   - TareOffset → CalibrationOffset（如非零）
//   - 补齐 CalibrationEnabled = true（所有通道默认启用校零）
//   - 补齐 CalibrationUnit（取通道当前 Unit 作为校零原始单位）
//   - 设置 Version = 2
func MigrateProfiles(profiles []device.Profile) []device.Profile {
	for i := range profiles {
		profiles[i] = migrateProfile(profiles[i])
	}
	return profiles
}

// migrateProfile 单 profile 迁移（幂等）。
func migrateProfile(p device.Profile) device.Profile {
	if p.Version >= device.CurrentProfileVersion {
		return p
	}
	now := time.Now().UnixMilli()
	converter := device.NewUnitConverter()
	for j := range p.Channels {
		ch := &p.Channels[j]
		ch.CalibrationEnabled = converter.SupportsZeroCalibration(ch.Unit)
		if ch.TareOffset != 0 {
			if !converter.SupportsZeroCalibration(ch.Unit) {
				ch.TareOffset = 0
				ch.CalibrationOffset = 0
				ch.CalibrationUnit = ""
				ch.CalibrationAt = 0
				continue
			}
			converted, err := converter.ToBaseUnit(ch.TareOffset, ch.Unit)
			if err != nil {
				continue
			}
			ch.CalibrationOffset = converted
			ch.CalibrationUnit = ch.Unit
			ch.CalibrationAt = now
			ch.TareOffset = 0
		}
	}
	p.Version = device.CurrentProfileVersion
	return p
}
