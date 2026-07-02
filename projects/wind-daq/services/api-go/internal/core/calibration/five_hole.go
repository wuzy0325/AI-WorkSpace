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

// AcquireData 采集单个点位数据（实现 Algorithm 接口）
// 使用探针通道配置读取原始数据，支持实时推送
func (a *FiveHoleAlgorithm) AcquireData(point CalPoint, channelReader ChannelValueReader, samplesPerPoint int) (DataPoint, error) {
	startTime := time.Now().UnixMilli()

	// 多次采样
	samples := make([]FiveHoleRawData, 0, samplesPerPoint)
	for i := 0; i < samplesPerPoint; i++ {
		// 使用带校验的探针通道读取
		rawData, err := ReadProbeChannelsToFiveHoleRaw(nil, channelReader)
		// 如果没有 probeChannels 配置，使用简化的直接读取
		if err != nil {
			rawData = readFiveHoleFromChannelReader(channelReader)
		}
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
	rawDeviceChannels := readRawDeviceChannelsFromReader(channelReader)

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

func (a *FiveHoleAlgorithm) AcquireDataWithConfig(point CalPoint, channelReader ChannelValueReader, config Config) (DataPoint, error) {
	return a.AcquireDataWithChannels(point, channelReader, config.ProbeChannels, config.SamplesPerPoint, nil)
}

// AcquireDataWithChannels 使用探针通道配置采集数据（推荐方式）
// 支持实时数据推送，供前端实时监控使用
func (a *FiveHoleAlgorithm) AcquireDataWithChannels(
	point CalPoint,
	channelReader ChannelValueReader,
	probeChannels []ProbeChannel,
	samplesPerPoint int,
	onRealtime func(FiveHoleRawData, FiveHoleCoefficients),
) (*FiveHoleDataPoint, error) {
	startTime := time.Now().UnixMilli()

	// 多次采样
	samples := make([]FiveHoleRawData, 0, samplesPerPoint)
	realtimeIntervalMs := int64(100)
	lastRealtimeSentAt := int64(0)

	for i := 0; i < samplesPerPoint; i++ {
		// 使用带校验的探针通道读取
		rawData, err := ReadProbeChannelsToFiveHoleRaw(probeChannels, channelReader)
		if err != nil {
			return nil, fmt.Errorf("读取五孔探针通道失败: %w", err)
		}
		samples = append(samples, rawData)

		// 实时推送当前样本的数据与瞬时系数（节流100ms）
		now := time.Now().UnixMilli()
		if onRealtime != nil && (now-lastRealtimeSentAt >= realtimeIntervalMs || i == samplesPerPoint-1) {
			realtimeCoeffs := CalculateFiveHoleCoefficients(rawData)
			onRealtime(rawData, realtimeCoeffs)
			lastRealtimeSentAt = now
		}

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

// readFiveHoleFromChannelReader 简化的通道读取（无 probeChannels 配置时的回退方案）
func readFiveHoleFromChannelReader(reader ChannelValueReader) FiveHoleRawData {
	// 使用默认角色映射直接读取
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

	values := make(map[string]float64)
	for _, field := range []string{"p1", "p2", "p3", "p4", "p5", "pAtm", "tAtm"} {
		for _, f := range roleMap {
			if f == field {
				// 尝试读取（无法确定具体设备ID和通道索引，返回0）
				if val, ok := reader("", -1); ok {
					values[field] = val
				}
				break
			}
		}
	}

	result := FiveHoleRawData{
		P1:   values["p1"],
		P2:   values["p2"],
		P3:   values["p3"],
		P4:   values["p4"],
		P5:   values["p5"],
		PAtm: values["pAtm"],
		TAtm: values["tAtm"],
	}
	return result
}

// readRawDeviceChannelsFromReader 从通道读取器中读取16通道原始数据
func readRawDeviceChannelsFromReader(reader ChannelValueReader) map[string][]float64 {
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
