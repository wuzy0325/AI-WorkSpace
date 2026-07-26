// Package pressure 提供压力通道归一化工具。
//
// 单位换算委托 core/device.UnitConverter（已有 11 个压力单位注册表 + kgf/cm2 alias），
// 本包只额外负责"绝压→表压"减法逻辑，避免双套系数表。
// 后续校准模块复用同一入口，保证全应用压力数据语义统一。
package pressure

import (
	"fmt"

	"wind-daq/services/api-go/internal/core/device"
)

// PressureType 压力传感器类型。
type PressureType string

const (
	// PressureTypeGauge 表压（相对大气压）。
	PressureTypeGauge PressureType = "gauge"
	// PressureTypeAbsolute 绝压（相对真空）。
	PressureTypeAbsolute PressureType = "absolute"
)

// NormalizePressureToGaugePa 将任意单位/类型的压力值归一化为 Pa + 表压。
//
// 为什么这样设计：
//   - converter 必须非 nil，由调用方注入（DeviceManager 持有的单例）；
//   - unit 工程单位字符串（如 "kPa"/"MPa"/"psi"），未知单位返回 error；
//   - pressureType 为空或 "gauge" 时原样换算；"absolute" 时减去 patmPa；
//   - patmPa 已是 Pa 单位的绝压值（由调用方事先用 ConvertToPa 归一化）。
//
// 返回值：归一化后的 Pa+表压值；换算失败时返回 0 与包装后的 error。
func NormalizePressureToGaugePa(
	value float64,
	unit string,
	pressureType string,
	patmPa float64,
	converter *device.UnitConverter,
) (float64, error) {
	if converter == nil {
		return 0, fmt.Errorf("pressure: UnitConverter is nil")
	}
	if !converter.SupportsZeroCalibration(unit) {
		return 0, fmt.Errorf("pressure: unit %q is not a pressure unit", unit)
	}
	paValue, err := converter.ToBaseUnit(value, unit)
	if err != nil {
		return 0, fmt.Errorf("pressure: convert %q to Pa failed: %w", unit, err)
	}
	// 仅绝压类型需要减去大气压转为表压；空串与 "gauge" 行为一致。
	if PressureType(pressureType) == PressureTypeAbsolute {
		return paValue - patmPa, nil
	}
	return paValue, nil
}

// ConvertToPa 仅做单位换算，不做绝压→表压。
//
// Patm 通道专用：大气压本身就是绝对值，插值器需要的就是 Patm 绝压值，
// 不应再做类型转换。converter nil 时返回明确 error。
func ConvertToPa(value float64, unit string, converter *device.UnitConverter) (float64, error) {
	if converter == nil {
		return 0, fmt.Errorf("pressure: UnitConverter is nil")
	}
	if !converter.SupportsZeroCalibration(unit) {
		return 0, fmt.Errorf("pressure: unit %q is not a pressure unit", unit)
	}
	return converter.ToBaseUnit(value, unit)
}
