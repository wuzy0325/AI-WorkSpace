package calibration

import (
	"fmt"
	"time"
)

// 采用手动控制模式：用户手动调整风洞工况，系统实时监控马赫数和温度稳定性
type TotalTemperatureAlgorithm struct{}

func NewTotalTemperatureAlgorithm() *TotalTemperatureAlgorithm {
	return &TotalTemperatureAlgorithm{}
}

func (a *TotalTemperatureAlgorithm) Type() CalibrationType {
	return TypeTotalTemperature
}

func (a *TotalTemperatureAlgorithm) ValidateConfig(config Config) error {
	if len(config.ProbeChannels) == 0 {
		return fmt.Errorf("总温探针校准需要配置探针通道")
	}

	// 检查必需的通道角色
	requiredRoles := []string{
		"testProbe",
		"standardProbe",
		"totalPressure",
		"staticPressure",
		"atmosphericPressure",
		"atmosphericTemperature",
	}
	roleSet := make(map[string]bool)
	for _, ch := range config.ProbeChannels {
		roleSet[ch.Role] = true
	}
	for _, role := range requiredRoles {
		if !roleSet[role] {
			return fmt.Errorf("总温探针校准缺少必需通道角色: %s", role)
		}
	}

	if config.SamplesPerPoint <= 0 {
		return fmt.Errorf("samplesPerPoint 必须大于0")
	}

	return nil
}

func (a *TotalTemperatureAlgorithm) AcquireData(point CalPoint, channelReader ChannelValueReader, samplesPerPoint int) (DataPoint, error) {
	// 总温校准使用 AcquireDataWithChannels 更合适
	return nil, fmt.Errorf("总温校准请使用 AcquireDataWithChannels 方法")
}

// AcquireDataWithChannels 使用探针通道配置采集数据
// 总温校准不使用运动控制，而是用户手动调整工况后触发采集
func (a *TotalTemperatureAlgorithm) AcquireDataWithChannels(
	point CalPoint,
	channelReader ChannelValueReader,
	probeChannels []ProbeChannel,
	samplesPerPoint int,
	sampleInterval time.Duration,
) (*TotalTemperatureDataPoint, error) {
	targetMa, ok := point.Coordinates["Ma"]
	if !ok {
		return nil, fmt.Errorf("测点缺少 Ma 坐标")
	}

	// 构建通道角色映射
	channels := make(map[string]ChannelRef)
	for _, ch := range probeChannels {
		if ch.Enabled {
			channels[ch.Role] = ChannelRef{
				DeviceID:     ch.DeviceID,
				ChannelIndex: ch.ChannelIndex,
			}
		}
	}

	// 采集多次取平均
	var testSamples []float64
	var standardSamples []float64

	for i := 0; i < samplesPerPoint; i++ {
		testTemp := a.getChannelData(channelReader, channels, "testProbe")
		standardTemp := a.getChannelData(channelReader, channels, "standardProbe")

		if testTemp != nil {
			testSamples = append(testSamples, *testTemp)
		}
		if standardTemp != nil {
			standardSamples = append(standardSamples, *standardTemp)
		}

		if i < samplesPerPoint-1 {
			time.Sleep(sampleInterval)
		}
	}

	// 计算平均值
	testProbeTemp := Mean(testSamples)
	standardProbeTemp := Mean(standardSamples)

	// 读取其他测点数据
	totalPressure := a.getChannelData(channelReader, channels, "totalPressure")
	staticPressure := a.getChannelData(channelReader, channels, "staticPressure")
	atmosphericPressure := a.getChannelData(channelReader, channels, "atmosphericPressure")
	atmosphericTemperature := a.getChannelData(channelReader, channels, "atmosphericTemperature")

	if totalPressure == nil || staticPressure == nil || atmosphericPressure == nil || atmosphericTemperature == nil {
		return nil, fmt.Errorf("缺少必要的测点数据")
	}

	// 计算马赫数和恢复系数
	actualMachNumber, err := CalculateMachNumber(*totalPressure, *staticPressure)
	if err != nil {
		return nil, fmt.Errorf("计算马赫数失败: %w", err)
	}

	recoveryCoefficient, err := CalculateRecoveryCoefficient(testProbeTemp, standardProbeTemp)
	if err != nil {
		return nil, fmt.Errorf("计算恢复系数失败: %w", err)
	}

	// 计算标准差
	stdDev := StdDev(testSamples)

	return &TotalTemperatureDataPoint{
		ID:                     point.ID,
		TargetMachNumber:       targetMa,
		ActualMachNumber:       actualMachNumber,
		TestProbeTemp:          testProbeTemp,
		StandardProbeTemp:      standardProbeTemp,
		RecoveryCoefficient:    recoveryCoefficient,
		TotalPressure:          *totalPressure,
		StaticPressure:         *staticPressure,
		AtmosphericPressure:    *atmosphericPressure,
		AtmosphericTemperature: *atmosphericTemperature,
		StdDev:                 stdDev,
		Timestamp:              time.Now().UnixMilli(),
	}, nil
}

// CheckStability 检测温度稳定性
func (a *TotalTemperatureAlgorithm) CheckStability(
	channelReader ChannelValueReader,
	channels map[string]ChannelRef,
	sampleCount int,
	sampleInterval time.Duration,
	maxStdDev float64,
) (bool, float64, error) {
	var samples []float64

	for i := 0; i < sampleCount; i++ {
		temp := a.getChannelData(channelReader, channels, "testProbe")
		if temp != nil {
			samples = append(samples, *temp)
		}
		if i < sampleCount-1 {
			time.Sleep(sampleInterval)
		}
	}

	stable, stdDev := CheckTemperatureStability(samples, maxStdDev)
	return stable, stdDev, nil
}

// ReadMachNumber 实时读取当前马赫数
func (a *TotalTemperatureAlgorithm) ReadMachNumber(
	channelReader ChannelValueReader,
	channels map[string]ChannelRef,
) (float64, error) {
	totalPressure := a.getChannelData(channelReader, channels, "totalPressure")
	staticPressure := a.getChannelData(channelReader, channels, "staticPressure")

	if totalPressure == nil || staticPressure == nil {
		return 0, fmt.Errorf("无法读取压力数据")
	}

	return CalculateMachNumber(*totalPressure, *staticPressure)
}

// getChannelData 从通道读取器中获取指定角色的数据
func (a *TotalTemperatureAlgorithm) getChannelData(
	reader ChannelValueReader,
	channels map[string]ChannelRef,
	role string,
) *float64 {
	ref, ok := channels[role]
	if !ok {
		return nil
	}
	val, found := reader(ref.DeviceID, ref.ChannelIndex)
	if !found {
		return nil
	}
	return &val
}
