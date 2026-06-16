package calibration

import (
	"fmt"
	"time"
)

type TotalPressureAlgorithm struct{}

func NewTotalPressureAlgorithm() *TotalPressureAlgorithm {
	return &TotalPressureAlgorithm{}
}

func (a *TotalPressureAlgorithm) Type() CalibrationType {
	return TypeTotalPressure
}

func (a *TotalPressureAlgorithm) ValidateConfig(config Config) error {
	if len(config.ProbeChannels) == 0 {
		return fmt.Errorf("总压探针校准需要配置探针通道")
	}

	requiredRoles := []string{
		"totalPressure.pAtm",
		"totalPressure.pTunnelTotal",
		"totalPressure.pTunnelStatic",
		"totalPressure.pProbeTotal",
	}
	roleSet := make(map[string]bool)
	for _, ch := range config.ProbeChannels {
		roleSet[ch.Role] = true
	}
	for _, role := range requiredRoles {
		if !roleSet[role] {
			return fmt.Errorf("总压探针校准缺少必需通道角色: %s", role)
		}
	}

	if config.SamplesPerPoint <= 0 {
		return fmt.Errorf("samplesPerPoint 必须大于0")
	}

	return nil
}

// AcquireData 采集单个点位数据（实现 Algorithm 接口）
func (a *TotalPressureAlgorithm) AcquireData(point CalPoint, channelReader ChannelValueReader, samplesPerPoint int) (DataPoint, error) {
	alpha, ok := point.Coordinates["α"]
	if !ok {
		return nil, fmt.Errorf("测点缺少 α 坐标")
	}

	startTime := time.Now().UnixMilli()

	samples := make([]TotalPressureRawData, 0, samplesPerPoint)
	for i := 0; i < samplesPerPoint; i++ {
		rawData, err := ReadProbeChannelsToTotalPressureRaw(nil, channelReader)
		if err != nil {
			rawData = readTotalPressureFallback(channelReader)
		}
		samples = append(samples, rawData)
		if i < samplesPerPoint-1 {
			time.Sleep(10 * time.Millisecond)
		}
	}

	endTime := time.Now().UnixMilli()

	avgRawData := CalculateTotalPressureAverage(samples)
	coefficients := CalculateTotalPressureCoefficients(avgRawData)

	return &TotalPressureDataPoint{
		PointID:      point.ID,
		Alpha:        alpha,
		RawData:      avgRawData,
		Coefficients: coefficients,
		SampleCount:  samplesPerPoint,
		StartTime:    startTime,
		EndTime:      endTime,
	}, nil
}

// AcquireDataWithChannels 使用探针通道配置采集数据（推荐方式）
func (a *TotalPressureAlgorithm) AcquireDataWithChannels(
	point CalPoint,
	channelReader ChannelValueReader,
	probeChannels []ProbeChannel,
	samplesPerPoint int,
) (*TotalPressureDataPoint, error) {
	alpha, ok := point.Coordinates["α"]
	if !ok {
		return nil, fmt.Errorf("测点缺少 α 坐标")
	}

	startTime := time.Now().UnixMilli()

	samples := make([]TotalPressureRawData, 0, samplesPerPoint)
	for i := 0; i < samplesPerPoint; i++ {
		rawData, err := ReadProbeChannelsToTotalPressureRaw(probeChannels, channelReader)
		if err != nil {
			return nil, fmt.Errorf("读取总压探针通道失败: %w", err)
		}
		samples = append(samples, rawData)
		if i < samplesPerPoint-1 {
			time.Sleep(10 * time.Millisecond)
		}
	}

	endTime := time.Now().UnixMilli()

	avgRawData := CalculateTotalPressureAverage(samples)
	coefficients := CalculateTotalPressureCoefficients(avgRawData)

	return &TotalPressureDataPoint{
		PointID:      point.ID,
		Alpha:        alpha,
		RawData:      avgRawData,
		Coefficients: coefficients,
		SampleCount:  samplesPerPoint,
		StartTime:    startTime,
		EndTime:      endTime,
	}, nil
}

// readTotalPressureFallback 简化的通道读取回退方案
func readTotalPressureFallback(reader ChannelValueReader) TotalPressureRawData {
	return TotalPressureRawData{}
}
