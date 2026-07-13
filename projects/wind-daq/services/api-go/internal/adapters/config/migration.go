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
		// 先按单位推断默认值：支持校零的单位 → 启用；温度等不支持单位 → 关闭。
		ch.CalibrationEnabled = converter.SupportsZeroCalibration(ch.Unit)

		if ch.TareOffset != 0 {
			// v1 用户曾显式校零过该通道：迁移 TareOffset → CalibrationOffset。
			// 不支持校零的单位（如温度）直接清空历史偏移，并关闭使能。
			if !converter.SupportsZeroCalibration(ch.Unit) {
				ch.TareOffset = 0
				ch.CalibrationOffset = 0
				ch.CalibrationUnit = ""
				ch.CalibrationAt = 0
				ch.CalibrationEnabled = false
				continue
			}
			converted, err := converter.ToBaseUnit(ch.TareOffset, ch.Unit)
			if err != nil {
				// 单位换算失败：保留 TareOffset 不清空（避免数据丢失），
				// 但跳过本通道的 CalibrationOffset 迁移；CalibrationEnabled 保持上方按单位推断的值。
				continue
			}
			ch.CalibrationOffset = converted
			ch.CalibrationUnit = ch.Unit
			ch.CalibrationAt = now
			ch.TareOffset = 0
			// 显式标记为启用：用户曾校零的通道必须应用偏移，
			// 即便该通道是 DAQ-P-1603 CH01/CH02 默认关闭通道——
			// 用户显式校零行为优先于设备默认规则，避免历史校零静默失效。
			ch.CalibrationEnabled = true
			continue
		}

		// TareOffset == 0：用户在 v1 时代未校零过该通道。
		// DAQ-P-1603 设备特殊默认：CH01/CH02（index 0/1）默认不应用校零，
		// 与 NewDefaultProfile 的默认规则保持一致；其余通道沿用按单位推断的值。
		if p.Type == device.DeviceDAQP1603 && j < 2 {
			ch.CalibrationEnabled = false
		}
	}
	p.Version = device.CurrentProfileVersion
	return p
}
