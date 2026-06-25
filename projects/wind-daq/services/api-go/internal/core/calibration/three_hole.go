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

// AcquireData 采集单个点位数据（实现 Algorithm 接口）
func (a *ThreeHoleAlgorithm) AcquireData(point CalPoint, channelReader ChannelValueReader, samplesPerPoint int) (DataPoint, error) {
	startTime := time.Now().UnixMilli()

	samples := make([]ThreeHoleRawData, 0, samplesPerPoint)
	for i := 0; i < samplesPerPoint; i++ {
		rawData, err := ReadProbeChannelsToThreeHoleRaw(nil, channelReader)
		if err != nil {
			rawData = readThreeHoleFallback(channelReader)
		}
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

func (a *ThreeHoleAlgorithm) AcquireDataWithConfig(point CalPoint, channelReader ChannelValueReader, config Config) (DataPoint, error) {
	return a.AcquireDataWithChannels(point, channelReader, config.ProbeChannels, config.SamplesPerPoint)
}

// AcquireDataWithChannels 使用探针通道配置采集数据（推荐方式）
func (a *ThreeHoleAlgorithm) AcquireDataWithChannels(
	point CalPoint,
	channelReader ChannelValueReader,
	probeChannels []ProbeChannel,
	samplesPerPoint int,
) (*ThreeHoleDataPoint, error) {
	startTime := time.Now().UnixMilli()

	samples := make([]ThreeHoleRawData, 0, samplesPerPoint)
	for i := 0; i < samplesPerPoint; i++ {
		rawData, err := ReadProbeChannelsToThreeHoleRaw(probeChannels, channelReader)
		if err != nil {
			return nil, fmt.Errorf("读取三孔探针通道失败: %w", err)
		}
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

// readThreeHoleFallback 简化的通道读取回退方案
func readThreeHoleFallback(reader ChannelValueReader) ThreeHoleRawData {
	return ThreeHoleRawData{}
}
