package calibration

import (
	"errors"
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
	AlphaMin   float64 `json:"alphaMin"`
	AlphaMax   float64 `json:"alphaMax"`
	AlphaStep  float64 `json:"alphaStep"`
	BetaMin    float64 `json:"betaMin"`
	BetaMax    float64 `json:"betaMax"`
	BetaStep   float64 `json:"betaStep"`
	Serpentine bool    `json:"serpentine,omitempty"`
	// AngleConvert α 轴角度换算开关（spec-five-hole-angle-convert）：
	// true 时 α 目标角按 α' = arctan(tanα·cosβ) 换算为运动台实际走角，β 不变；
	// 预览布点、实际走点、CSV 落盘全部使用换算后坐标。
	// 兼容性：旧配置文件/旧请求体无此字段按 false 处理，行为与历史版本完全一致。
	AngleConvert bool `json:"angleConvert,omitempty"`
}

// convertFiveHoleAlpha 按 angleConvert 开关把 α 轴逻辑角换算为运动台实际目标角。
// 公式：α' = arctan(tan(α)·cos(β))（角度制输入输出），β 轴不变。
// 物理来源：五孔探针安装在两轴运动台上，α 轴承载于 β 轴之上——探针绕 β 偏航后，
// 其自身测量平面内的实际迎角被 cos(β) 因子衰减；运动台按换算角走点，才能保证
// 物理布点与配置的逻辑网格一致（docs/specs/spec-five-hole-angle-convert.md）。
// 数学前提：|α| < 90°（tan 在 ±90° 发散，|α|>90° 时 tan 变号导致符号翻转），
// GenerateFiveHoleSnakePoints 开启换算时已做前置校验。
func convertFiveHoleAlpha(alphaDeg, betaDeg float64) float64 {
	alphaRad := alphaDeg * math.Pi / 180.0
	betaRad := betaDeg * math.Pi / 180.0
	return math.Atan(math.Tan(alphaRad)*math.Cos(betaRad)) * 180.0 / math.Pi
}

func GenerateFiveHoleSnakePoints(layout FiveHolePointLayout) ([]FiveHoleSnakePoint, error) {
	if layout.AlphaStep <= 0 || layout.BetaStep <= 0 {
		return nil, fmt.Errorf("step must be positive")
	}
	// 角度换算的数学前提防呆：tan 在 ±90° 发散，|α|>90° 时 tan 变号会使换算角
	// 符号翻转（点位跨象限错乱）；β 同理——|β|≥90° 时 cosβ 非正，β=±90° 使整行
	// α' 退化为 0（点位重叠落盘），|β|>90° 时 α' 符号翻转。开关开启时提前拒绝
	// 非法区间——报错优于静默产出错点。
	if layout.AngleConvert && (math.Abs(layout.AlphaMin) >= 90 || math.Abs(layout.AlphaMax) >= 90) {
		return nil, fmt.Errorf("angleConvert 开启时 α 范围必须在 ±90° 以内（不含 ±90°）")
	}
	if layout.AngleConvert && (math.Abs(layout.BetaMin) >= 90 || math.Abs(layout.BetaMax) >= 90) {
		return nil, fmt.Errorf("angleConvert 开启时 β 范围必须在 ±90° 以内（不含 ±90°）")
	}
	alphaCount := int(math.Round((layout.AlphaMax-layout.AlphaMin)/layout.AlphaStep)) + 1
	betaCount := int(math.Round((layout.BetaMax-layout.BetaMin)/layout.BetaStep)) + 1
	points := make([]FiveHoleSnakePoint, 0, alphaCount*betaCount)
	id := 1
	for bi := 0; bi < betaCount; bi++ {
		beta := roundTo1Decimal(layout.BetaMin + float64(bi)*layout.BetaStep)
		// 蛇形走位：奇数行反向遍历 α；默认（raster）每行都从 αMin 升序遍历
		reverse := layout.Serpentine && bi%2 == 1
		for ai := 0; ai < alphaCount; ai++ {
			alphaIdx := ai
			if reverse {
				alphaIdx = alphaCount - 1 - ai
			}
			alpha := roundTo1Decimal(layout.AlphaMin + float64(alphaIdx)*layout.AlphaStep)
			// 换算在舍入后的网格角上进行：先得到逻辑网格角（1 位小数），
			// 再换算并同样保留 1 位小数，保证预览/走点/CSV 三处坐标完全一致
			if layout.AngleConvert {
				alpha = roundTo1Decimal(convertFiveHoleAlpha(alpha, beta))
			}
			points = append(points, FiveHoleSnakePoint{
				ID:          id,
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

	requiredRoles := []string{
		"fiveHole.p1", "fiveHole.p2", "fiveHole.p3",
		"fiveHole.p4", "fiveHole.p5", "fiveHole.pAtm",
		"fiveHole.tAtm", "fiveHole.pTotal", "fiveHole.pTunnelStatic",
	}
	roleSet := make(map[string]bool)
	for _, ch := range config.ProbeChannels {
		roleSet[ch.Role] = true
	}
	var missingRoles []string
	for _, role := range requiredRoles {
		if !roleSet[role] {
			missingRoles = append(missingRoles, role)
		}
	}
	if len(missingRoles) > 0 {
		return fmt.Errorf("五孔探针校准缺少必需通道角色: %v", missingRoles)
	}

	if config.SamplesPerPoint <= 0 {
		return fmt.Errorf("samplesPerPoint 必须大于0")
	}

	return nil
}

// AcquireData 旧接口实现，仅作 Algorithm 接口兼容。
//
// 五孔自动校准流程实际走 AcquireDataWithConfig（携带 ProbeChannels），
// 此入口因缺乏通道配置无法完成真实采集——直接返回明确错误，
// 避免悄悄走"零值 fallback"老路径导致 Kα=0/Kβ=0/CPT=0 无告警。
func (a *FiveHoleAlgorithm) AcquireData(point CalPoint, channelReader ChannelValueReader, samplesPerPoint int) (DataPoint, error) {
	return nil, fmt.Errorf("五孔探针 AcquireData 缺少探针通道配置，请通过 AcquireDataWithConfig 调用")
}

func (a *FiveHoleAlgorithm) AcquireDataWithConfig(point CalPoint, channelReader ChannelValueReader, config Config, checkAbort func() bool, onSampleProgress func(current, total int)) (DataPoint, error) {
	return a.AcquireDataWithChannels(point, channelReader, config.ProbeChannels, config.SamplesPerPoint, nil, checkAbort, config.TimestampReader, config.AcquisitionStateProvider, onSampleProgress)
}

// AcquireDataWithChannels 使用探针通道配置采集数据（推荐方式）
// 支持实时数据推送，供前端实时监控使用
//
// acquiringCheck：可选设备采集态查询，超时后若任一设备未在采集则继续等待恢复（用户停采集可恢复）；
// 为 nil 时维持原超时失败行为（手动模式或未注入场景）。
func (a *FiveHoleAlgorithm) AcquireDataWithChannels(
	point CalPoint,
	channelReader ChannelValueReader,
	probeChannels []ProbeChannel,
	samplesPerPoint int,
	onRealtime func(FiveHoleRawData, FiveHoleCoefficients),
	checkAbort func() bool,
	timestampReader TimestampReader,
	acquiringCheck AcquisitionStateProvider,
	onSampleProgress func(current, total int),
) (*FiveHoleDataPoint, error) {
	startTime := time.Now().UnixMilli()

	deviceIDs := collectUniqueDeviceIDs(probeChannels)
	lastTimestamps := make(map[string]int64)

	samples := make([]FiveHoleRawData, 0, samplesPerPoint)
	realtimeIntervalMs := int64(100)
	lastRealtimeSentAt := int64(0)

	for i := 0; i < samplesPerPoint; i++ {
		if checkAbort != nil && checkAbort() {
			return nil, ErrPointAborted
		}

		if i > 0 {
			if timestampReader != nil {
				if err := waitForFreshData(deviceIDs, timestampReader, lastTimestamps, freshnessDefaultTimeout, checkAbort, acquiringCheck); err != nil {
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

		rawData, err := ReadProbeChannelsToFiveHoleRaw(probeChannels, channelReader)
		if err != nil {
			return nil, fmt.Errorf("读取五孔探针通道失败: %w", err)
		}
		samples = append(samples, rawData)

		// 采样进度回调：每次采完一个样本通知上层，驱动 UI 显示"当前点采样 i+1/N"
		if onSampleProgress != nil {
			onSampleProgress(i+1, samplesPerPoint)
		}

		if timestampReader != nil {
			recordLastTimestamps(deviceIDs, timestampReader, lastTimestamps)
		}

		now := time.Now().UnixMilli()
		if onRealtime != nil && (now-lastRealtimeSentAt >= realtimeIntervalMs || i == samplesPerPoint-1) {
			realtimeCoeffs := CalculateFiveHoleCoefficients(rawData)
			onRealtime(rawData, realtimeCoeffs)
			lastRealtimeSentAt = now
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
