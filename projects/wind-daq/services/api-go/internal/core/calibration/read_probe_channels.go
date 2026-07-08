package calibration

import (
	"fmt"
	"log"
	"time"
)

// ReadProbeChannels 从探针通道配置和通道读取器中读取原始数据
// 遍历 probeChannels，按 deviceId:channelIndex 查找值，
// 根据 roleToField 映射将值写入对应字段，最后校验 requiredFields。
// 返回字段值映射和校验错误
func ReadProbeChannels(
	probeChannels []ProbeChannel,
	reader ChannelValueReader,
	roleToField map[string]string,
	requiredFields []string,
	contextName string,
) (map[string]float64, error) {
	data := make(map[string]float64)
	assignedKeys := make(map[string]bool)
	var failedReads []string

	for _, ch := range probeChannels {
		if !ch.Enabled {
			continue
		}

		val, found := reader(ch.DeviceID, ch.ChannelIndex)
		if !found {
			log.Printf("[%s] 通道读取失败: %s (角色: %s)", contextName, ch.Name, ch.Role)
			failedReads = append(failedReads, ch.Role)
			continue
		}

		field, ok := roleToField[ch.Role]
		if !ok {
			if ch.Role != "" {
				log.Printf("[%s] 未知探针通道角色: %s", contextName, ch.Role)
			}
			continue
		}

		if assignedKeys[field] {
			log.Printf("[%s] 字段 %s 重复赋值，保留第一个值", contextName, field)
			continue
		}

		data[field] = val
		assignedKeys[field] = true
	}

	// 校验必需字段
	var missingFields []string
	for _, field := range requiredFields {
		if !assignedKeys[field] {
			missingFields = append(missingFields, field)
		}
	}

	if len(missingFields) > 0 {
		detail := ""
		if len(failedReads) > 0 {
			detail = fmt.Sprintf("; 读取失败: %v", uniqueStrings(failedReads))
		}
		return data, fmt.Errorf("%s缺少必要通道: %v%s", contextName, missingFields, detail)
	}

	return data, nil
}

// uniqueStrings 去重字符串切片
func uniqueStrings(input []string) []string {
	seen := make(map[string]bool)
	result := make([]string, 0)
	for _, s := range input {
		if !seen[s] {
			seen[s] = true
			result = append(result, s)
		}
	}
	return result
}

// ReadProbeChannelsToFiveHoleRaw 读取五孔探针原始数据（带校验）
func ReadProbeChannelsToFiveHoleRaw(
	probeChannels []ProbeChannel,
	reader ChannelValueReader,
) (FiveHoleRawData, error) {
	roleMap := map[string]string{
		"fiveHole.p1":            "p1",
		"fiveHole.p2":            "p2",
		"fiveHole.p3":            "p3",
		"fiveHole.p4":            "p4",
		"fiveHole.p5":            "p5",
		"fiveHole.pAtm":          "pAtm",
		"fiveHole.tAtm":          "tAtm",
		"fiveHole.pTotal":        "pTotal",
		"fiveHole.pTunnelStatic": "pStatic",
	}

	required := []string{"p1", "p2", "p3", "p4", "p5", "pAtm", "tAtm", "pTotal", "pStatic"}

	data, err := ReadProbeChannels(probeChannels, reader, roleMap, required, "五孔探针")
	if err != nil {
		return FiveHoleRawData{}, err
	}

	result := FiveHoleRawData{
		P1:   data["p1"],
		P2:   data["p2"],
		P3:   data["p3"],
		P4:   data["p4"],
		P5:   data["p5"],
		PAtm: data["pAtm"],
		TAtm: data["tAtm"],
	}

	if v, ok := data["pTotal"]; ok {
		result.PTotal = &v
	}
	if v, ok := data["pStatic"]; ok {
		result.PStatic = &v
	}

	return result, nil
}

// ReadProbeChannelsToThreeHoleRaw 读取三孔探针原始数据（带校验）
func ReadProbeChannelsToThreeHoleRaw(
	probeChannels []ProbeChannel,
	reader ChannelValueReader,
) (ThreeHoleRawData, error) {
	roleMap := map[string]string{
		"threeHole.p1":      "p1",
		"threeHole.p2":      "p2",
		"threeHole.p3":      "p3",
		"threeHole.pAtm":    "pAtm",
		"threeHole.tAtm":    "tAtm",
		"threeHole.pTotal":  "pTotal",
		"threeHole.pStatic": "pStatic",
	}

	required := []string{"p1", "p2", "p3", "pAtm"}

	data, err := ReadProbeChannels(probeChannels, reader, roleMap, required, "三孔探针")
	if err != nil {
		return ThreeHoleRawData{}, err
	}

	result := ThreeHoleRawData{
		P1:   data["p1"],
		P2:   data["p2"],
		P3:   data["p3"],
		PAtm: data["pAtm"],
		TAtm: data["tAtm"],
	}

	if v, ok := data["pTotal"]; ok {
		result.PTotal = &v
	}
	if v, ok := data["pStatic"]; ok {
		result.PStatic = &v
	}

	return result, nil
}

// ReadProbeChannelsToTotalPressureRaw 读取总压探针原始数据（带校验）
func ReadProbeChannelsToTotalPressureRaw(
	probeChannels []ProbeChannel,
	reader ChannelValueReader,
) (TotalPressureRawData, error) {
	roleMap := map[string]string{
		"totalPressure.pAtm":          "pAtm",
		"totalPressure.tAtm":          "tAtm",
		"totalPressure.pTunnelTotal":  "pTunnelTotal",
		"totalPressure.pTunnelStatic": "pTunnelStatic",
		"totalPressure.tTunnel":       "tTunnel",
		"totalPressure.pProbeTotal":   "pProbeTotal",
	}

	required := []string{"pAtm", "pTunnelTotal", "pTunnelStatic", "pProbeTotal"}

	data, err := ReadProbeChannels(probeChannels, reader, roleMap, required, "总压探针")
	if err != nil {
		return TotalPressureRawData{}, err
	}

	return TotalPressureRawData{
		PAtm:          data["pAtm"],
		TAtm:          data["tAtm"],
		PTunnelTotal:  data["pTunnelTotal"],
		PTunnelStatic: data["pTunnelStatic"],
		TTunnel:       data["tTunnel"],
		PProbeTotal:   data["pProbeTotal"],
	}, nil
}

const (
	freshnessPollInterval   = 10 * time.Millisecond
	freshnessDefaultTimeout = 5 * time.Second
)

func collectUniqueDeviceIDs(probeChannels []ProbeChannel) []string {
	seen := make(map[string]bool)
	ids := make([]string, 0)
	for _, ch := range probeChannels {
		if ch.Enabled && ch.DeviceID != "" && !seen[ch.DeviceID] {
			seen[ch.DeviceID] = true
			ids = append(ids, ch.DeviceID)
		}
	}
	return ids
}

func waitForFreshData(
	deviceIDs []string,
	timestampReader TimestampReader,
	lastTimestamps map[string]int64,
	timeout time.Duration,
	checkAbort func() bool,
) error {
	if len(deviceIDs) == 0 {
		return nil
	}
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if checkAbort != nil && checkAbort() {
			return ErrPointAborted
		}
		allFresh := true
		for _, deviceID := range deviceIDs {
			ts, ok := timestampReader(deviceID)
			if !ok {
				allFresh = false
				break
			}
			if lastTS, exists := lastTimestamps[deviceID]; exists && ts <= lastTS {
				allFresh = false
				break
			}
		}
		if allFresh {
			return nil
		}
		time.Sleep(freshnessPollInterval)
	}
	return fmt.Errorf("等待设备新数据超时 (%v)，设备: %v", timeout, deviceIDs)
}

func recordLastTimestamps(deviceIDs []string, timestampReader TimestampReader, dst map[string]int64) {
	for _, deviceID := range deviceIDs {
		if ts, ok := timestampReader(deviceID); ok {
			dst[deviceID] = ts
		}
	}
}
