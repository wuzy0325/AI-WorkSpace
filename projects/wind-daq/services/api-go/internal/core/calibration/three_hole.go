package calibration

import (
	"errors"
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

	// pTotal/pStatic 为可选：Kt/Sb 系数依赖二者，但公式层在缺失时有优雅降级（kt=0, sb=0）；
	// 此处不做硬校验，允许用户在不配置风洞总压/静压时仍能执行纯方向校准。
	requiredRoles := []string{
		"threeHole.p1", "threeHole.p2", "threeHole.p3",
		"threeHole.pAtm",
	}
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
//
// 注意：此方法签名不携带探针通道配置（ProbeChannels），无法正确读取三孔原始数据。
// 生产路径通过 AutomaticCalibration → AcquireDataWithConfig → AcquireDataWithChannels 调用，
// 此方法仅供接口契约兼容。若被直接调用，返回错误而非零值数据点，避免静默产出
// 物理上错误但表面合法的"圾数据"（Kb=0, Kt=0, Sb=0）。
func (a *ThreeHoleAlgorithm) AcquireData(point CalPoint, channelReader ChannelValueReader, samplesPerPoint int) (DataPoint, error) {
	return nil, fmt.Errorf("三孔探针 AcquireData 缺少探针通道配置，请通过 AcquireDataWithConfig 调用")
}

func (a *ThreeHoleAlgorithm) AcquireDataWithConfig(
	point CalPoint,
	channelReader ChannelValueReader,
	config Config,
	checkAbort func() bool,
) (DataPoint, error) {
	return a.AcquireDataWithChannels(point, channelReader, config.ProbeChannels, config.SamplesPerPoint, nil, checkAbort, config.TimestampReader)
}

// AcquireDataWithChannels 使用探针通道配置采集数据（推荐方式）
//
// onRealtime：可选实时回调，采样循环内节流 100ms 发布当前样本与瞬时系数，供前端实时监控。
// checkAbort：可选中止检查闭包，由上层注入；返回 true 时立即中止采集并返回 ErrPointAborted，
// 使 AutomaticCalibration.runCalibrationLoop 回退索引重跑该点（暂停/停止响应）。
func (a *ThreeHoleAlgorithm) AcquireDataWithChannels(
	point CalPoint,
	channelReader ChannelValueReader,
	probeChannels []ProbeChannel,
	samplesPerPoint int,
	onRealtime func(ThreeHoleRawData, ThreeHoleCoefficients),
	checkAbort func() bool,
	timestampReader TimestampReader,
) (*ThreeHoleDataPoint, error) {
	startTime := time.Now().UnixMilli()

	deviceIDs := collectUniqueDeviceIDs(probeChannels)
	lastTimestamps := make(map[string]int64)

	samples := make([]ThreeHoleRawData, 0, samplesPerPoint)
	realtimeIntervalMs := int64(100)
	lastRealtimeSentAt := int64(0)

	for i := 0; i < samplesPerPoint; i++ {
		if checkAbort != nil && checkAbort() {
			return nil, ErrPointAborted
		}

		if i > 0 {
			if timestampReader != nil {
				if err := waitForFreshData(deviceIDs, timestampReader, lastTimestamps, freshnessDefaultTimeout, checkAbort); err != nil {
					if errors.Is(err, ErrPointAborted) {
						return nil, err
					}
					return nil, fmt.Errorf("等待新数据帧超时: %w", err)
				}
			} else {
				// 无设备时间戳读取能力时退化为固定间隔 sleep，避免全速空转读到同一帧缓存
				time.Sleep(10 * time.Millisecond)
			}
		}

		rawData, err := ReadProbeChannelsToThreeHoleRaw(probeChannels, channelReader)
		if err != nil {
			return nil, fmt.Errorf("读取三孔探针通道失败: %w", err)
		}
		samples = append(samples, rawData)

		if timestampReader != nil {
			recordLastTimestamps(deviceIDs, timestampReader, lastTimestamps)
		}

		now := time.Now().UnixMilli()
		if onRealtime != nil && (now-lastRealtimeSentAt >= realtimeIntervalMs || i == samplesPerPoint-1) {
			realtimeCoeffs := CalculateThreeHoleCoefficients(rawData)
			onRealtime(rawData, realtimeCoeffs)
			lastRealtimeSentAt = now
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
