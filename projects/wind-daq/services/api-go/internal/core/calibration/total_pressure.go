package calibration

import (
	"errors"
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

	// 采样间隔与超时联动校验：避免 BatchPollIntervalMs > BatchTimeoutMs 导致每帧都超时
	if config.AcquisitionSampling != nil {
		intervalMs := config.AcquisitionSampling.BatchPollIntervalMs
		timeoutMs := config.AcquisitionSampling.BatchTimeoutMs
		if intervalMs > 0 && timeoutMs > 0 && intervalMs > timeoutMs {
			return fmt.Errorf("采样间隔(BatchPollIntervalMs=%dms)不能大于每帧超时(BatchTimeoutMs=%dms)", intervalMs, timeoutMs)
		}
	}

	return nil
}

// AcquireDataWithConfig 自动校准引擎调用入口：使用完整配置（含 ProbeChannels）采集单点。
// 采样间隔优先取 AcquisitionSampling.BatchPollIntervalMs，缺省回退到 10ms（与 five-hole 一致）。
func (a *TotalPressureAlgorithm) AcquireDataWithConfig(point CalPoint, channelReader ChannelValueReader, config Config, checkAbort func() bool, onSampleProgress func(current, total int)) (DataPoint, error) {
	return a.AcquireDataWithChannels(point, channelReader, config.ProbeChannels, config.SamplesPerPoint, config.AcquisitionSampling, checkAbort, config.TimestampReader, onSampleProgress)
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
//
//		samplesPerPoint 次读取，每次间隔 10ms（默认兜底，与 five-hole/three-hole 一致）；
//		当 timestampReader 可用时优先等待设备新帧，不可用时退化为固定间隔 sleep 避免全速空转。
//	  - 任一次读取失败立即返回错误，避免用零值静默填充样本（曾导致 CPT=0、马赫数=0 无告警）
//	  - 采集完成后计算平均值、系数、探针总压样本标准差
func (a *TotalPressureAlgorithm) AcquireDataWithChannels(
	point CalPoint,
	channelReader ChannelValueReader,
	probeChannels []ProbeChannel,
	samplesPerPoint int,
	sampling *AcquisitionSamplingConfig,
	checkAbort func() bool,
	timestampReader TimestampReader,
	onSampleProgress func(current, total int),
) (*TotalPressureDataPoint, error) {
	alpha, ok := point.Coordinates["α"]
	if !ok {
		return nil, fmt.Errorf("测点缺少 α 坐标")
	}

	startTime := time.Now().UnixMilli()

	deviceIDs := collectUniqueDeviceIDs(probeChannels)
	lastTimestamps := make(map[string]int64)

	perSampleTimeout := freshnessDefaultTimeout
	if sampling != nil && sampling.BatchTimeoutMs > 0 {
		perSampleTimeout = time.Duration(sampling.BatchTimeoutMs) * time.Millisecond
	}

	sampleIntervalMs := 10
	if sampling != nil && sampling.BatchPollIntervalMs > 0 {
		sampleIntervalMs = sampling.BatchPollIntervalMs
	}
	sampleInterval := time.Duration(sampleIntervalMs) * time.Millisecond

	samples := make([]TotalPressureRawData, 0, samplesPerPoint)
	for i := 0; i < samplesPerPoint; i++ {
		if checkAbort != nil && checkAbort() {
			return nil, ErrPointAborted
		}

		if i > 0 {
			if timestampReader != nil {
				if err := waitForFreshData(deviceIDs, timestampReader, lastTimestamps, perSampleTimeout, checkAbort); err != nil {
					if errors.Is(err, ErrPointAborted) {
						return nil, err
					}
					return nil, fmt.Errorf("等待新数据帧超时: %w", err)
				}
			} else {
				// 无设备时间戳读取能力时退化为间隔 sleep，避免全速空转读到同一帧缓存
				time.Sleep(sampleInterval)
			}
		}

		rawData, err := ReadProbeChannelsToTotalPressureRaw(probeChannels, channelReader)
		if err != nil {
			return nil, fmt.Errorf("读取总压探针通道失败: %w", err)
		}
		samples = append(samples, rawData)

		// 采样进度回调：每次采完一个样本通知上层，驱动 UI 显示"当前点采样 i+1/N"
		if onSampleProgress != nil {
			onSampleProgress(i+1, samplesPerPoint)
		}

		if timestampReader != nil {
			recordLastTimestamps(deviceIDs, timestampReader, lastTimestamps)
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
