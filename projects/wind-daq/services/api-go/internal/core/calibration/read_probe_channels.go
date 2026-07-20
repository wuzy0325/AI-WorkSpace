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

// ReadProbeChannelsToSevenHoleRaw 读取七孔探针原始数据（带校验，11 通道）
//
// 角色映射（spec §6.1，与 SevenHoleAlgorithm.ValidateConfig 严格对应）：
//   - sevenHole.p1 ~ p7：7 个压力孔（外围 6 孔 + 中心孔 P7）
//   - sevenHole.pAtm：大气压力（绝压，A→C 边界转换用）
//   - sevenHole.tAtm：大气温度（静温/真空速计算用）
//   - sevenHole.pTotal：风洞参考总压（K0/Ks/Ma 公式分母来源）
//   - sevenHole.pTunnelStatic：风洞参考静压（Ks/Ma 公式分母来源）
//
// 角色命名说明：pTunnelStatic 与五孔保持一致（避免前端 ProbeChannelRole 枚举分裂），
// 内部映射到 SevenHoleRawData.PStatic 字段（指针类型，缺失时为 nil）。
//
// 必需字段全部 11 项——任一缺失返回错误，避免静默用零值产出误导性系数。
func ReadProbeChannelsToSevenHoleRaw(
	probeChannels []ProbeChannel,
	reader ChannelValueReader,
) (SevenHoleRawData, error) {
	roleMap := map[string]string{
		"sevenHole.p1":            "p1",
		"sevenHole.p2":            "p2",
		"sevenHole.p3":            "p3",
		"sevenHole.p4":            "p4",
		"sevenHole.p5":            "p5",
		"sevenHole.p6":            "p6",
		"sevenHole.p7":            "p7",
		"sevenHole.pAtm":          "pAtm",
		"sevenHole.tAtm":          "tAtm",
		"sevenHole.pTotal":        "pTotal",
		"sevenHole.pTunnelStatic": "pStatic",
	}

	required := []string{"p1", "p2", "p3", "p4", "p5", "p6", "p7", "pAtm", "tAtm", "pTotal", "pStatic"}

	data, err := ReadProbeChannels(probeChannels, reader, roleMap, required, "七孔探针")
	if err != nil {
		return SevenHoleRawData{}, err
	}

	result := SevenHoleRawData{
		P1:   data["p1"],
		P2:   data["p2"],
		P3:   data["p3"],
		P4:   data["p4"],
		P5:   data["p5"],
		P6:   data["p6"],
		P7:   data["p7"],
		PAtm: data["pAtm"],
		TAtm: data["tAtm"],
	}

	// 指针字段：风洞总压/静压/温度缺失时保持 nil（与五孔/三孔语义一致）
	if v, ok := data["pTotal"]; ok {
		result.PTotal = &v
	}
	if v, ok := data["pStatic"]; ok {
		result.PStatic = &v
	}

	return result, nil
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
