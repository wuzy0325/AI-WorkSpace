package domain

import "math"

// 压力点状态常量，标定和计量模块共用。
const (
	PointStatusPending     = "pending"
	PointStatusPressurizing = "pressurizing"
	PointStatusStabilizing  = "stabilizing"
	PointStatusCollecting   = "collecting"
	PointStatusCompleted    = "completed"
	PointStatusError        = "error"
	PointStatusSkipped      = "skipped"
)

// AverageSamples 对多次采样的逐通道数据取平均，标定和计量模块共用。
// 各次采样的通道数可不完全一致——缺失通道不计入对应索引的平均值。
func AverageSamples(samples [][]float64) []float64 {
	if len(samples) == 0 {
		return nil
	}
	width := len(samples[0])
	result := make([]float64, width)
	counts := make([]int, width)
	for _, sample := range samples {
		for i := 0; i < len(sample) && i < width; i++ {
			result[i] += sample[i]
			counts[i]++
		}
	}
	for i := range result {
		if counts[i] > 0 {
			result[i] /= float64(counts[i])
		}
	}
	return result
}

// RoundToPrecision 按指定小数位精度对值进行四舍五入。
func RoundToPrecision(value float64, precision int) float64 {
	if precision <= 0 {
		return math.Round(value)
	}
	factor := math.Pow10(precision)
	return math.Round(value*factor) / factor
}

// EquidistantPoints 根据压力范围和点数生成等距压力点，含正程和可选回程。
// minPressure 和 maxPressure 的顺序会被自动修正。
func EquidistantPoints(minPressure, maxPressure float64, pointCount, precision int, roundTrip bool) []PressurePoint {
	if maxPressure < minPressure {
		minPressure, maxPressure = maxPressure, minPressure
	}
	if pointCount < 2 {
		pointCount = 2
	}

	step := (maxPressure - minPressure) / float64(pointCount-1)
	forward := make([]PressurePoint, pointCount)
	for i := 0; i < pointCount; i++ {
		forward[i] = PressurePoint{
			Index:          i + 1,
			TargetPressure: RoundToPrecision(minPressure+step*float64(i), precision),
			Direction:      "forward",
			Status:         PointStatusPending,
		}
	}

	if !roundTrip {
		return forward
	}

	backward := make([]PressurePoint, pointCount)
	for i := 0; i < pointCount; i++ {
		backward[i] = PressurePoint{
			Index:          pointCount + i + 1,
			TargetPressure: RoundToPrecision(maxPressure-step*float64(i), precision),
			Direction:      "backward",
			Status:         PointStatusPending,
		}
	}

	return append(forward, backward...)
}

// DevicePointData 表示单台计量设备在某个压力点的采集结果与状态。
// 多设备计量时，PressurePoint.CollectedByDevice 以设备 ID 为键存储每台设备的数据。
type DevicePointData struct {
	DeviceID    string    `json:"deviceId"`
	Collected   []float64 `json:"collected"`
	Status      string    `json:"status"` // completed | error | skipped
	CollectTime string    `json:"collectTime,omitempty"`
	SkipReason  string    `json:"skipReason,omitempty"`
	Error       string    `json:"error,omitempty"`
}

// PressurePoint 表示一个压力测试点及其采集状态。
// 标定和计量模块共用此类型。
// CollectedData 为单设备场景下的通道数据（兼容旧逻辑与旧数据）；
// CollectedByDevice 为多设备场景下的设备维度数据，优先读取。
type PressurePoint struct {
	ID               string                     `json:"id"`
	Index            int                        `json:"index"`
	TargetPressure   float64                    `json:"targetPressure"`
	Direction        string                     `json:"direction"` // forward | backward
	Status           string                     `json:"status"`
	ActualPressure   *float64                   `json:"actualPressure,omitempty"`
	CollectedData    []float64                  `json:"collectedData,omitempty"`
	CollectedByDevice map[string]DevicePointData `json:"collectedByDevice,omitempty"`
	CollectTime      string                     `json:"collectTime,omitempty"`
	ErrorMessage     string                     `json:"errorMessage,omitempty"`
}

// PointDataForDevice 返回指定设备的采集数据。
// 优先读取设备维度数据（CollectedByDevice），缺失时回退单设备字段 CollectedData，
// 以保证多设备新路径与单设备旧数据（JSON 反序列化）都能正确消费。
func (p *PressurePoint) PointDataForDevice(deviceID string) ([]float64, bool) {
	if len(p.CollectedByDevice) > 0 {
		if d, ok := p.CollectedByDevice[deviceID]; ok {
			return d.Collected, true
		}
		return nil, false
	}
	if len(p.CollectedData) > 0 {
		return p.CollectedData, true
	}
	return nil, false
}
