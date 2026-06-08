package calibration

import (
	"fmt"
	"time"
)

type ThreeHoleAlgorithm struct{}

func NewThreeHoleAlgorithm() *ThreeHoleAlgorithm {
	return &ThreeHoleAlgorithm{}
}

func (a *ThreeHoleAlgorithm) Type() CalibrationType {
	return TypeThreeHole
}

func (a *ThreeHoleAlgorithm) ValidateConfig(config Config) error {
	if len(config.ProbeChannels) == 0 {
		return fmt.Errorf("三孔探针校准需要配置探针通道")
	}

	// 检查必需的通道角色
	requiredRoles := []string{"threeHole.p1", "threeHole.p2", "threeHole.p3", "threeHole.pAtm"}
	roleSet := make(map[string]bool)
	for _, ch := range config.ProbeChannels {
		roleSet[ch.Role] = true
	}
	for _, role := range requiredRoles {
		if !roleSet[role] {
			return fmt.Errorf("三孔探针校准缺少必需通道角色: %s", role)
		}
	}

	if config.SamplesPerPoint <= 0 {
		return fmt.Errorf("samplesPerPoint 必须大于0")
	}

	return nil
}

func (a *ThreeHoleAlgorithm) AcquireData(point CalPoint, channelReader ChannelValueReader, samplesPerPoint int) (DataPoint, error) {
	startTime := time.Now().UnixMilli()

	// 角色到字段名的映射
	roleMap := map[string]string{
		"threeHole.p1":     "p1",
		"threeHole.p2":     "p2",
		"threeHole.p3":     "p3",
		"threeHole.pAtm":   "pAtm",
		"threeHole.pTotal": "pTotal",
	}

	// 多次采样
	samples := make([]ThreeHoleRawData, 0, samplesPerPoint)
	for i := 0; i < samplesPerPoint; i++ {
		rawData := a.readRawData(channelReader, roleMap)
		samples = append(samples, rawData)
		if i < samplesPerPoint-1 {
			time.Sleep(10 * time.Millisecond)
		}
	}

	endTime := time.Now().UnixMilli()

	// 计算平均值
	avgData := CalculateThreeHoleAverage(samples)

	// 计算系数
	coefficients := CalculateThreeHoleCoefficients(avgData)

	// 计算标准差
	p1Values := make([]float64, len(samples))
	for i, s := range samples {
		p1Values[i] = s.P1
	}
	stdDev := StdDev(p1Values)

	return &ThreeHoleDataPoint{
		PointID:      point.ID,
		Coordinates:  point.Coordinates,
		RawData:      avgData,
		Coefficients: coefficients,
		SampleCount:  len(samples),
		StdDev:       stdDev,
		StartTime:    startTime,
		EndTime:      endTime,
	}, nil
}

// AcquireDataWithChannels 使用探针通道配置采集数据（推荐方式）
func (a *ThreeHoleAlgorithm) AcquireDataWithChannels(
	point CalPoint,
	channelReader ChannelValueReader,
	probeChannels []ProbeChannel,
	samplesPerPoint int,
) (*ThreeHoleDataPoint, error) {
	startTime := time.Now().UnixMilli()

	roleMap := map[string]string{
		"threeHole.p1":     "p1",
		"threeHole.p2":     "p2",
		"threeHole.p3":     "p3",
		"threeHole.pAtm":   "pAtm",
		"threeHole.pTotal": "pTotal",
	}

	// 多次采样
	samples := make([]ThreeHoleRawData, 0, samplesPerPoint)
	for i := 0; i < samplesPerPoint; i++ {
		rawData := a.readProbeChannels(channelReader, probeChannels, roleMap)
		samples = append(samples, rawData)
		if i < samplesPerPoint-1 {
			time.Sleep(10 * time.Millisecond)
		}
	}

	endTime := time.Now().UnixMilli()

	avgData := CalculateThreeHoleAverage(samples)
	coefficients := CalculateThreeHoleCoefficients(avgData)

	p1Values := make([]float64, len(samples))
	for i, s := range samples {
		p1Values[i] = s.P1
	}
	stdDev := StdDev(p1Values)

	return &ThreeHoleDataPoint{
		PointID:      point.ID,
		Coordinates:  point.Coordinates,
		RawData:      avgData,
		Coefficients: coefficients,
		SampleCount:  len(samples),
		StdDev:       stdDev,
		StartTime:    startTime,
		EndTime:      endTime,
	}, nil
}

// readRawData 从通道读取器中读取三孔探针原始数据
func (a *ThreeHoleAlgorithm) readRawData(reader ChannelValueReader, roleMap map[string]string) ThreeHoleRawData {
	// 简化实现
	return ThreeHoleRawData{}
}

// readProbeChannels 通过探针通道配置读取原始数据
func (a *ThreeHoleAlgorithm) readProbeChannels(reader ChannelValueReader, probeChannels []ProbeChannel, roleMap map[string]string) ThreeHoleRawData {
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

	result := ThreeHoleRawData{
		P1:   values["p1"],
		P2:   values["p2"],
		P3:   values["p3"],
		PAtm: values["pAtm"],
	}
	if v, ok := values["pTotal"]; ok {
		result.PTotal = &v
	}
	return result
}
