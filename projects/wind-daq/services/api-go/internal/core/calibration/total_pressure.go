package calibration

import (
	"fmt"
	"time"
)

// TotalPressureAlgorithm 总压探针校准算法
type TotalPressureAlgorithm struct{}

func NewTotalPressureAlgorithm() *TotalPressureAlgorithm {
	return &TotalPressureAlgorithm{}
}

func (a *TotalPressureAlgorithm) Type() CalibrationType {
	return TypeTotalPressure
}

// ValidateConfig 校验总压探针校准配置
// 必需通道角色与 read_probe_channels.go 中 roleMap 严格对应：
//   - totalPressure.pAtm           大气压
//   - totalPressure.tAtm           大气温度
//   - totalPressure.pTunnelTotal   风洞总压（CPT 公式分母来源）
//   - totalPressure.pTunnelStatic  风洞静压（马赫数计算来源）
//   - totalPressure.pProbeTotal    探针总压（CPT 公式分子来源）
//
// 注意：前端 ProbeChannelRole 枚举曾错误地使用 pTotal/pStatic，已被弃用，
// 任何配置仍残留这两个角色将被此处校验拦截。
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

// AcquireDataWithConfig 自动校准引擎调用入口：使用完整配置（含 ProbeChannels）采集单点。
// 采样间隔优先取 AcquisitionSampling.BatchPollIntervalMs，缺省回退到 10ms（与 five-hole 一致）。
func (a *TotalPressureAlgorithm) AcquireDataWithConfig(point CalPoint, channelReader ChannelValueReader, config Config) (DataPoint, error) {
	return a.AcquireDataWithChannels(point, channelReader, config.ProbeChannels, config.SamplesPerPoint, config.AcquisitionSampling)
}

// AcquireData 旧接口实现，仅作 Algorithm 接口兼容。
//
// 自动校准流程实际走 AcquireDataWithConfig（携带 ProbeChannels），
// 此入口因缺乏通道配置无法完成真实采集——直接返回明确错误，
// 避免悄悄走"零值 fallback"老路径导致 CPT=0、马赫数=0 无告警。
// 调用方应改用 AcquireDataWithConfig 或 AcquireDataWithChannels。
func (a *TotalPressureAlgorithm) AcquireData(point CalPoint, channelReader ChannelValueReader, samplesPerPoint int) (DataPoint, error) {
	return nil, fmt.Errorf("总压探针 AcquireData 旧接口不支持无通道配置采集，请改用 AcquireDataWithConfig")
}

// AcquireDataWithChannels 使用探针通道配置采集数据（推荐方式）
//
// 采样策略：
//   - samplesPerPoint 次读取，每次间隔 sampleIntervalMs（默认 10ms，与 five-hole/three-hole 一致；
//     可由 AcquisitionSampling.BatchPollIntervalMs 覆盖——该字段原义为批量读取轮询间隔，
//     此处复用为多次采样间的睡眠间隔，语义相近：控制对设备的轮询频率）
//   - 任一次读取失败立即返回错误，避免用零值静默填充样本（曾导致 CPT=0、马赫数=0 无告警）
//   - 采集完成后计算平均值、系数、探针总压样本标准差
func (a *TotalPressureAlgorithm) AcquireDataWithChannels(
	point CalPoint,
	channelReader ChannelValueReader,
	probeChannels []ProbeChannel,
	samplesPerPoint int,
	sampling *AcquisitionSamplingConfig,
) (*TotalPressureDataPoint, error) {
	alpha, ok := point.Coordinates["α"]
	if !ok {
		return nil, fmt.Errorf("测点缺少 α 坐标")
	}

	startTime := time.Now().UnixMilli()

	// 默认 10ms 与 five-hole.go / three-hole.go 保持一致，确保各校准类型采样行为统一。
	sampleIntervalMs := 10
	if sampling != nil && sampling.BatchPollIntervalMs > 0 {
		sampleIntervalMs = sampling.BatchPollIntervalMs
	}
	sampleInterval := time.Duration(sampleIntervalMs) * time.Millisecond

	samples := make([]TotalPressureRawData, 0, samplesPerPoint)
	for i := 0; i < samplesPerPoint; i++ {
		rawData, err := ReadProbeChannelsToTotalPressureRaw(probeChannels, channelReader)
		if err != nil {
			return nil, fmt.Errorf("读取总压探针通道失败: %w", err)
		}
		samples = append(samples, rawData)
		if i < samplesPerPoint-1 {
			time.Sleep(sampleInterval)
		}
	}

	endTime := time.Now().UnixMilli()

	avgRawData := CalculateTotalPressureAverage(samples)
	coefficients := CalculateTotalPressureCoefficients(avgRawData)
	stdDev := CalculateTotalPressureStdDev(samples)

	return &TotalPressureDataPoint{
		PointID:      point.ID,
		Alpha:        alpha,
		RawData:      avgRawData,
		Coefficients: coefficients,
		SampleCount:  samplesPerPoint,
		StdDev:       stdDev,
		StartTime:    startTime,
		EndTime:      endTime,
	}, nil
}
