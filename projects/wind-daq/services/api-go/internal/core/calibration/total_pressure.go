package calibration

import (
	"fmt"
	"time"
)

// TotalPressureAlgorithm 总压探针校准算法
type TotalPressureAlgorithm struct{}

// NewTotalPressureAlgorithm 创建总压探针校准算法实例
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

	// 检查必需的通道角色
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

func (a *TotalPressureAlgorithm) AcquireData(point CalPoint, channelReader ChannelValueReader, samplesPerPoint int) (DataPoint, error) {
	alpha, ok := point.Coordinates["α"]
	if !ok {
		return nil, fmt.Errorf("测点缺少 α 坐标")
	}

	startTime := time.Now().UnixMilli()

	// 多次采样
	samples := make([]TotalPressureRawData, 0, samplesPerPoint)
	for i := 0; i < samplesPerPoint; i++ {
		rawData := a.collectRawData(channelReader)
		samples = append(samples, rawData)
		if i < samplesPerPoint-1 {
			time.Sleep(10 * time.Millisecond)
		}
	}

	endTime := time.Now().UnixMilli()

	// 计算平均值
	avgRawData := CalculateTotalPressureAverage(samples)

	// 计算系数
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

	// 角色到字段名的映射
	roleMap := map[string]string{
		"totalPressure.pAtm":          "pAtm",
		"totalPressure.tAtm":          "tAtm",
		"totalPressure.pTunnelTotal":  "pTunnelTotal",
		"totalPressure.pTunnelStatic": "pTunnelStatic",
		"totalPressure.tTunnel":       "tTunnel",
		"totalPressure.pProbeTotal":   "pProbeTotal",
	}

	// 多次采样
	samples := make([]TotalPressureRawData, 0, samplesPerPoint)
	for i := 0; i < samplesPerPoint; i++ {
		rawData := a.readProbeChannels(channelReader, probeChannels, roleMap)
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

// collectRawData 从通道读取器中收集原始数据
func (a *TotalPressureAlgorithm) collectRawData(reader ChannelValueReader) TotalPressureRawData {
	// 简化实现
	return TotalPressureRawData{}
}

// readProbeChannels 通过探针通道配置读取原始数据
func (a *TotalPressureAlgorithm) readProbeChannels(reader ChannelValueReader, probeChannels []ProbeChannel, roleMap map[string]string) TotalPressureRawData {
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

	return TotalPressureRawData{
		PAtm:          values["pAtm"],
		TAtm:          values["tAtm"],
		PTunnelTotal:  values["pTunnelTotal"],
		PTunnelStatic: values["pTunnelStatic"],
		TTunnel:       values["tTunnel"],
		PProbeTotal:   values["pProbeTotal"],
	}
}
