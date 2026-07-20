package calibration

import (
	"fmt"
	"log"
	"time"
)

// AcquisitionCoordinator 校准采集协调器
// 负责校验设备采集状态、批量通道读取、资源释放
type AcquisitionCoordinator struct {
	getLatestData   func(deviceID string) (latestData, bool)
	getDeviceStatus func(deviceID string) (deviceStatus, bool)
}

// latestData 设备最新数据快照
type latestData struct {
	Channels       []float64
	ChannelIndices []int
	Timestamp      int64
}

// deviceStatus 设备状态信息
type deviceStatus struct {
	Connected bool
	Acquiring bool
}

// NewAcquisitionCoordinator 创建采集协调器
func NewAcquisitionCoordinator(
	getLatestData func(deviceID string) (latestData, bool),
	getDeviceStatus func(deviceID string) (deviceStatus, bool),
) *AcquisitionCoordinator {
	return &AcquisitionCoordinator{
		getLatestData:   getLatestData,
		getDeviceStatus: getDeviceStatus,
	}
}

// CollectAcquisitionDeviceIds 收集校准所需的所有设备ID
func (c *AcquisitionCoordinator) CollectAcquisitionDeviceIds(config Config) []string {
	deviceIdSet := make(map[string]bool)

	// 从探针通道收集
	for _, ch := range config.ProbeChannels {
		if ch.Enabled && ch.DeviceID != "" {
			deviceIdSet[ch.DeviceID] = true
		}
	}

	// 从球罐闸门配置收集
	if config.SphereTankGate != nil && config.SphereTankGate.Enabled {
		// 稳定时间通道：参与闸门判定，必须订阅
		if config.SphereTankGate.StableTimeChannel.DeviceID != "" {
			deviceIdSet[config.SphereTankGate.StableTimeChannel.DeviceID] = true
		}
		// 压力通道：仅用于前端实时显示压力值，不参与判定，但也需要订阅才能收到快照
		if config.SphereTankGate.PressureChannel.DeviceID != "" {
			deviceIdSet[config.SphereTankGate.PressureChannel.DeviceID] = true
		}
	}

	result := make([]string, 0, len(deviceIdSet))
	for id := range deviceIdSet {
		result = append(result, id)
	}
	return result
}

// AssertRequiredDevicesAcquiring 校验所有必需设备已连接且正在采集
func (c *AcquisitionCoordinator) AssertRequiredDevicesAcquiring(config Config) error {
	deviceIds := c.CollectAcquisitionDeviceIds(config)
	var unavailable []string

	for _, deviceID := range deviceIds {
		status, ok := c.getDeviceStatus(deviceID)
		if !ok || !status.Connected || !status.Acquiring {
			conn := "Disconnected"
			acq := "false"
			if ok {
				if status.Connected {
					conn = "Connected"
				}
				if status.Acquiring {
					acq = "true"
				}
			}
			unavailable = append(unavailable, fmt.Sprintf("%s (status=%s, acquiring=%s)", deviceID, conn, acq))
		}
	}

	if len(unavailable) > 0 {
		return fmt.Errorf("校准需要设备正在采集数据，以下设备未就绪: %v", unavailable)
	}
	return nil
}

// GetChannelValue 获取指定设备通道的当前值
func (c *AcquisitionCoordinator) GetChannelValue(deviceID string, channelIndex int) (float64, bool) {
	data, ok := c.getLatestData(deviceID)
	if !ok {
		log.Printf("[AcquisitionCoordinator] 设备 %s 无数据", deviceID)
		return 0, false
	}

	for i, idx := range data.ChannelIndices {
		if idx == channelIndex && i < len(data.Channels) {
			return data.Channels[i], true
		}
	}

	log.Printf("[AcquisitionCoordinator] 设备 %s 通道 %d 未找到", deviceID, channelIndex)
	return 0, false
}

// GetChannelBatch 批量读取通道数据，支持超时等待和数据新鲜度检查
func (c *AcquisitionCoordinator) GetChannelBatch(
	channels []ChannelRef,
	minDeviceTimestamps map[string]int64,
	timeoutMs int,
	maxAgeMs int,
) (map[string]float64, map[string]int64, error) {
	deadline := time.Now().Add(time.Duration(timeoutMs) * time.Millisecond)

	// 收集所有设备ID
	deviceIdSet := make(map[string]bool)
	for _, ch := range channels {
		deviceIdSet[ch.DeviceID] = true
	}

	for time.Now().Before(deadline) {
		// 检查所有设备数据是否就绪
		ready := true
		snapshots := make(map[string]latestData)
		now := time.Now().UnixMilli()

		for deviceID := range deviceIdSet {
			data, ok := c.getLatestData(deviceID)
			if !ok {
				ready = false
				break
			}

			// 检查时间戳是否更新
			if minTS, exists := minDeviceTimestamps[deviceID]; exists && data.Timestamp <= minTS {
				ready = false
				break
			}

			// 检查数据新鲜度
			if maxAgeMs > 0 && now-data.Timestamp > int64(maxAgeMs) {
				ready = false
				break
			}

			snapshots[deviceID] = data
		}

		if ready {
			// 提取通道值
			values := make(map[string]float64)
			deviceTimestamps := make(map[string]int64)

			for deviceID, data := range snapshots {
				deviceTimestamps[deviceID] = data.Timestamp
			}

			for _, ch := range channels {
				data, ok := snapshots[ch.DeviceID]
				if !ok {
					continue
				}
				key := fmt.Sprintf("%s:%d", ch.DeviceID, ch.ChannelIndex)
				for i, idx := range data.ChannelIndices {
					if idx == ch.ChannelIndex && i < len(data.Channels) {
						values[key] = data.Channels[i]
						break
					}
				}
			}

			return values, deviceTimestamps, nil
		}

		time.Sleep(10 * time.Millisecond)
	}

	return nil, nil, fmt.Errorf("批量通道读取超时（%d ms），设备: %v", timeoutMs, deviceIdSet)
}
