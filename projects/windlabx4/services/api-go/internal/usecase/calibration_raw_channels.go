package usecase

import (
	"fmt"
	"sort"

	"windlabx4/services/api-go/internal/core/calibration"
	"windlabx4/services/api-go/internal/core/device"
)

func buildCalibrationRawDeviceLayouts(probeChannels []calibration.ProbeChannel, profiles []device.Profile) ([]calibration.RawDeviceLayout, error) {
	profileByID := make(map[string]device.Profile, len(profiles))
	for _, profile := range profiles {
		profileByID[profile.ID] = profile
	}

	seen := make(map[string]bool)
	layouts := make([]calibration.RawDeviceLayout, 0)
	for _, probeChannel := range probeChannels {
		deviceID := probeChannel.DeviceID
		if !probeChannel.Enabled || deviceID == "" || seen[deviceID] {
			continue
		}
		seen[deviceID] = true
		profile, ok := profileByID[deviceID]
		if !ok {
			return nil, fmt.Errorf("探针通道引用的设备配置不存在: %s", deviceID)
		}
		channels := append([]device.ChannelConfig(nil), profile.Channels...)
		sort.SliceStable(channels, func(i, j int) bool { return channels[i].Index < channels[j].Index })
		layout := calibration.RawDeviceLayout{DeviceID: deviceID, DeviceName: profile.Name, Channels: make([]calibration.RawDeviceChannel, 0, len(channels))}
		for _, channel := range channels {
			layout.Channels = append(layout.Channels, calibration.RawDeviceChannel{Index: channel.Index, Unit: channel.Unit})
		}
		layouts = append(layouts, layout)
	}
	return layouts, nil
}
