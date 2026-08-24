package calibration

func readConfiguredRawDeviceChannels(reader ChannelValueReader, layouts []RawDeviceLayout) (map[string][]float64, map[string][]bool) {
	result := make(map[string][]float64, len(layouts))
	valid := make(map[string][]bool, len(layouts))
	for _, layout := range layouts {
		values := make([]float64, len(layout.Channels))
		available := make([]bool, len(layout.Channels))
		for i, channel := range layout.Channels {
			if value, ok := reader(layout.DeviceID, channel.Index); ok {
				values[i] = value
				available[i] = true
			}
		}
		result[layout.DeviceID] = values
		valid[layout.DeviceID] = available
	}
	return result, valid
}

func buildRawDeviceChannelHeaders(layouts []RawDeviceLayout) []string {
	headers := make([]string, 0)
	for _, layout := range layouts {
		// 列头用设备显示名（profile.Name）更易读；历史布局未填 DeviceName 时回退设备 ID
		deviceLabel := layout.DeviceName
		if deviceLabel == "" {
			deviceLabel = layout.DeviceID
		}
		for _, channel := range layout.Channels {
			header := deviceLabel + "_CH" + formatInt(channel.Index+1)
			if channel.Unit != "" {
				header += "(" + channel.Unit + ")"
			}
			headers = append(headers, header)
		}
	}
	return headers
}

func buildRawDeviceChannelValues(layouts []RawDeviceLayout, rawDeviceChannels map[string][]float64, rawDeviceValid map[string][]bool) []string {
	values := make([]string, 0)
	for _, layout := range layouts {
		deviceValues := rawDeviceChannels[layout.DeviceID]
		deviceValid := rawDeviceValid[layout.DeviceID]
		for i := range layout.Channels {
			value := ""
			if i < len(deviceValues) && (deviceValid == nil || (i < len(deviceValid) && deviceValid[i])) {
				value = formatFloatWithPrecision(deviceValues[i], 3)
			}
			values = append(values, value)
		}
	}
	return values
}
