package calibration

import (
	"fmt"
	"math"
	"time"
)

type FiveHoleAlgorithm struct{}

func NewFiveHoleAlgorithm() *FiveHoleAlgorithm {
	return &FiveHoleAlgorithm{}
}

type FiveHoleSnakePoint struct {
	ID          int                `json:"id"`
	Coordinates map[string]float64 `json:"coordinates"`
}

type FiveHolePointLayout struct {
	AlphaMin  float64 `json:"alphaMin"`
	AlphaMax  float64 `json:"alphaMax"`
	AlphaStep float64 `json:"alphaStep"`
	BetaMin   float64 `json:"betaMin"`
	BetaMax   float64 `json:"betaMax"`
	BetaStep  float64 `json:"betaStep"`
}

func GenerateFiveHoleSnakePoints(layout FiveHolePointLayout) ([]FiveHoleSnakePoint, error) {
	if layout.AlphaStep <= 0 || layout.BetaStep <= 0 {
		return nil, fmt.Errorf("step must be positive")
	}
	alphaCount := int(math.Round((layout.AlphaMax-layout.AlphaMin)/layout.AlphaStep)) + 1
	betaCount := int(math.Round((layout.BetaMax-layout.BetaMin)/layout.BetaStep)) + 1
	points := make([]FiveHoleSnakePoint, 0, alphaCount*betaCount)
	id := 1
	for bi := 0; bi < betaCount; bi++ {
		beta := math.Round((layout.BetaMin+float64(bi)*layout.BetaStep)*10) / 10
		reverse := bi%2 == 1
		for ai := 0; ai < alphaCount; ai++ {
			alphaIdx := ai
			if reverse {
				alphaIdx = alphaCount - 1 - ai
			}
			alpha := math.Round((layout.AlphaMin+float64(alphaIdx)*layout.AlphaStep)*10) / 10
			points = append(points, FiveHoleSnakePoint{
				ID: id,
				Coordinates: map[string]float64{"α": alpha, "β": beta},
			})
			id++
		}
	}
	return points, nil
}

func (a *FiveHoleAlgorithm) Type() CalibrationType {
	return TypeFiveHole
}

func (a *FiveHoleAlgorithm) ValidateConfig(config Config) error {
	if len(config.ProbeChannels) == 0 {
		return fmt.Errorf("五孔探针校准需要配置探针通道")
	}

	// 检查必需的通道角色
	requiredRoles := []string{
		"fiveHole.p1", "fiveHole.p2", "fiveHole.p3",
		"fiveHole.p4", "fiveHole.p5", "fiveHole.pAtm",
	}
	roleSet := make(map[string]bool)
	for _, ch := range config.ProbeChannels {
		roleSet[ch.Role] = true
	}
	for _, role := range requiredRoles {
		if !roleSet[role] {
			return fmt.Errorf("五孔探针校准缺少必需通道角色: %s", role)
		}
	}

	if config.SamplesPerPoint <= 0 {
		return fmt.Errorf("samplesPerPoint 必须大于0")
	}

	return nil
}

func (a *FiveHoleAlgorithm) AcquireData(point CalPoint, channelReader ChannelValueReader, samplesPerPoint int) (DataPoint, error) {
	startTime := time.Now().UnixMilli()

	// 角色到字段名的映射
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

	// 多次采样
	samples := make([]FiveHoleRawData, 0, samplesPerPoint)
	for i := 0; i < samplesPerPoint; i++ {
		rawData := a.readRawData(channelReader, roleMap)
		samples = append(samples, rawData)
		if i < samplesPerPoint-1 {
			time.Sleep(10 * time.Millisecond)
		}
	}

	endTime := time.Now().UnixMilli()

	// 计算平均值
	avgData := CalculateFiveHoleAverage(samples)

	// 计算系数
	coefficients := CalculateFiveHoleCoefficients(avgData)

	// 计算标准差（使用第一个压力通道作为参考）
	p1Values := make([]float64, len(samples))
	for i, s := range samples {
		p1Values[i] = s.P1
	}
	stdDev := StdDev(p1Values)

	// 读取扫描阀16通道原始数据
	rawDeviceChannels := a.readRawDeviceChannels(channelReader)

	return &FiveHoleDataPoint{
		PointID:           point.ID,
		Coordinates:       point.Coordinates,
		RawData:           avgData,
		Coefficients:      coefficients,
		SampleCount:       len(samples),
		StdDev:            stdDev,
		StartTime:         startTime,
		EndTime:           endTime,
		RawDeviceChannels: rawDeviceChannels,
	}, nil
}

// readRawData 从通道读取器中读取五孔探针原始数据
func (a *FiveHoleAlgorithm) readRawData(reader ChannelValueReader, roleMap map[string]string) FiveHoleRawData {
	// 通过角色映射读取各通道值
	readValue := func(role string) float64 {
		// 需要从外部获取 ProbeChannel 配置，这里简化处理
		// 实际使用时由 CalibrationManager 注入
		val, _ := reader("", -1)
		_ = roleMap
		return val
	}

	// 注意：实际实现中，readRawData 需要访问 ProbeChannel 配置
	// 这里提供一个基于直接通道索引的简化版本
	return FiveHoleRawData{
		P1:   readValue("p1"),
		P2:   readValue("p2"),
		P3:   readValue("p3"),
		P4:   readValue("p4"),
		P5:   readValue("p5"),
		PAtm: readValue("pAtm"),
		TAtm: readValue("tAtm"),
	}
}

// readRawDeviceChannels 读取扫描阀16通道原始数据
func (a *FiveHoleAlgorithm) readRawDeviceChannels(reader ChannelValueReader) map[string][]float64 {
	// 简化实现：从默认设备读取16通道
	result := make(map[string][]float64)
	channels := make([]float64, 16)
	for ch := 0; ch < 16; ch++ {
		val, ok := reader("", ch)
		if !ok {
			val = 0
		}
		channels[ch] = val
	}
	result["default"] = channels
	return result
}

// AcquireDataWithChannels 使用探针通道配置采集数据（推荐方式）
func (a *FiveHoleAlgorithm) AcquireDataWithChannels(
	point CalPoint,
	channelReader ChannelValueReader,
	probeChannels []ProbeChannel,
	samplesPerPoint int,
) (*FiveHoleDataPoint, error) {
	startTime := time.Now().UnixMilli()

	// 角色到字段名的映射
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

	// 多次采样
	samples := make([]FiveHoleRawData, 0, samplesPerPoint)
	for i := 0; i < samplesPerPoint; i++ {
		rawData := readProbeChannels(channelReader, probeChannels, roleMap)
		samples = append(samples, rawData)
		if i < samplesPerPoint-1 {
			time.Sleep(10 * time.Millisecond)
		}
	}

	endTime := time.Now().UnixMilli()

	// 计算平均值
	avgData := CalculateFiveHoleAverage(samples)

	// 计算系数
	coefficients := CalculateFiveHoleCoefficients(avgData)

	// 计算标准差
	p1Values := make([]float64, len(samples))
	for i, s := range samples {
		p1Values[i] = s.P1
	}
	stdDev := StdDev(p1Values)

	// 读取扫描阀16通道原始数据
	rawDeviceChannels := readRawDeviceChannelsFromProbe(channelReader, probeChannels)

	return &FiveHoleDataPoint{
		PointID:           point.ID,
		Coordinates:       point.Coordinates,
		RawData:           avgData,
		Coefficients:      coefficients,
		SampleCount:       len(samples),
		StdDev:            stdDev,
		StartTime:         startTime,
		EndTime:           endTime,
		RawDeviceChannels: rawDeviceChannels,
	}, nil
}

// readProbeChannels 通过探针通道配置读取原始数据
func readProbeChannels(reader ChannelValueReader, probeChannels []ProbeChannel, roleMap map[string]string) FiveHoleRawData {
	// 构建角色到通道值的映射
	values := make(map[string]float64)
	for _, ch := range probeChannels {
		if !ch.Enabled {
			continue
		}
		fieldName, ok := roleMap[ch.Role]
		if !ok {
			continue
		}
		val, found := reader(ch.DeviceID, ch.ChannelIndex)
		if found {
			values[fieldName] = val
		}
	}

	return FiveHoleRawData{
		P1:   values["p1"],
		P2:   values["p2"],
		P3:   values["p3"],
		P4:   values["p4"],
		P5:   values["p5"],
		PAtm: values["pAtm"],
		TAtm: values["tAtm"],
	}
}

// readRawDeviceChannelsFromProbe 从探针通道配置中读取16通道原始数据
func readRawDeviceChannelsFromProbe(reader ChannelValueReader, probeChannels []ProbeChannel) map[string][]float64 {
	// 收集所有设备ID
	deviceIDs := make(map[string]bool)
	for _, ch := range probeChannels {
		if ch.Enabled && ch.DeviceID != "" {
			deviceIDs[ch.DeviceID] = true
		}
	}

	result := make(map[string][]float64)
	for deviceID := range deviceIDs {
		channels := make([]float64, 16)
		for ch := 0; ch < 16; ch++ {
			val, ok := reader(deviceID, ch)
			if !ok {
				val = 0
			}
			channels[ch] = val
		}
		result[deviceID] = channels
	}
	return result
}
