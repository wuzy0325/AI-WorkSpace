package calibration

import "fmt"

// IsFiveHoleDataPoint 判断数据点是否为五孔探针类型
func IsFiveHoleDataPoint(p DataPoint) bool {
	fh, ok := p.(*FiveHoleDataPoint)
	if !ok {
		return false
	}
	return fh.RawData.P1 != 0 || fh.RawData.P2 != 0 ||
		fh.RawData.P3 != 0 || fh.RawData.P4 != 0 || fh.RawData.P5 != 0 ||
		fh.Coordinates != nil
}

// IsThreeHoleDataPoint 判断数据点是否为三孔探针类型
func IsThreeHoleDataPoint(p DataPoint) bool {
	_, ok := p.(*ThreeHoleDataPoint)
	return ok
}

// IsTotalPressureDataPoint 判断数据点是否为总压探针类型
func IsTotalPressureDataPoint(p DataPoint) bool {
	_, ok := p.(*TotalPressureDataPoint)
	return ok
}

// IsTotalTemperatureDataPoint 判断数据点是否为总温探针类型
func IsTotalTemperatureDataPoint(p DataPoint) bool {
	_, ok := p.(*TotalTemperatureDataPoint)
	return ok
}

// FilterCalibrationPoints 按校准类型过滤数据点
func FilterCalibrationPoints(calType CalibrationType, points []DataPoint) []DataPoint {
	var guard func(DataPoint) bool

	switch calType {
	case TypeFiveHole:
		guard = IsFiveHoleDataPoint
	case TypeThreeHole:
		guard = IsThreeHoleDataPoint
	case TypeTotalPressure:
		guard = IsTotalPressureDataPoint
	case TypeTotalTemperature:
		guard = IsTotalTemperatureDataPoint
	default:
		return points
	}

	result := make([]DataPoint, 0)
	for _, p := range points {
		if guard(p) {
			result = append(result, p)
		}
	}
	return result
}

// ValidateDataPoint 验证数据点有效性
func ValidateDataPoint(p DataPoint) error {
	if p == nil {
		return fmt.Errorf("数据点不能为nil")
	}
	if p.GetPointID() < 0 {
		return fmt.Errorf("数据点ID不能为负数: %d", p.GetPointID())
	}
	return nil
}

// ValidateDataPoints 批量验证数据点
func ValidateDataPoints(points []DataPoint) error {
	for i, p := range points {
		if err := ValidateDataPoint(p); err != nil {
			return fmt.Errorf("数据点 %d 无效: %w", i, err)
		}
	}
	return nil
}
