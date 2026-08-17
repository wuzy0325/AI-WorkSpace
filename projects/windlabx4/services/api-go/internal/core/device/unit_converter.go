package device

import "fmt"

// UnitFamily 单位族：基单位 + 互转系数表。
// 校零值落库时强制转为基单位存储；展示时按当前单位换算。
type UnitFamily struct {
	BaseUnit string
	Factors  map[string]float64
}

// UnitConverter 单位换算注册表。
// 设计原因：不同设备使用不同单位（Pa/kPa/mmH2O/℃/℉），
// 注册表模式让新增单位只需注册因子，无需修改校准逻辑。
type UnitConverter struct {
	families []UnitFamily
}

// NewUnitConverter 创建含默认压力+温度单位族的换算器。
func NewUnitConverter() *UnitConverter {
	return &UnitConverter{
		families: []UnitFamily{
			pressureFamily(),
			temperatureFamily(),
		},
	}
}

// pressureFamily 压力单位族（基单位 Pa）。
// mmH2O 换算系数基于 4℃ 水密度 999.972 kg/m³，g=9.80665 m/s²。
// kgf/cm2 与 kgfcm2 同时注册为 alias：前端 DaqP1603Config.vue 使用 'kgf/cm2'（带斜杠），
// 历史代码使用 'kgfcm2'（无斜杠），两者指向同一系数 98066.5，保证字面量差异不影响换算结果。
func pressureFamily() UnitFamily {
	return UnitFamily{
		BaseUnit: "Pa",
		Factors: map[string]float64{
			"Pa":      1.0,
			"kPa":     1000.0,
			"MPa":     1e6,
			"mmH2O":   9.80665,
			"mmHg":    133.322,
			"psi":     6894.757,
			"bar":     1e5,
			"mbar":    100.0,
			"inH2O":   249.0889,
			"inHg":    3386.389,
			"kgfcm2":  98066.5,
			"kgf/cm2": 98066.5, // alias：与 kgfcm2 等价，兼容前端带斜杠字面量
		},
	}
}

// temperatureFamily 温度单位族（基单位 ℃）。
// ℉↔℃ 为线性变换（非比例），用特殊分支处理。
func temperatureFamily() UnitFamily {
	return UnitFamily{
		BaseUnit: "℃",
		Factors: map[string]float64{
			"℃":    1.0,
			"°C":   1.0,
			"degC": 1.0,
		},
	}
}

// ToBaseUnit 将工程量 value 从 srcUnit 转为所属单位族的基单位。
// srcUnit 可以是 "Pa"/"kPa"/"℃"/"℉" 等。若单位未注册则返回错误。
func (uc *UnitConverter) ToBaseUnit(value float64, srcUnit string) (float64, error) {
	if isFahrenheit(srcUnit) {
		return (value - 32) * 5 / 9, nil
	}
	family, factor := uc.lookup(srcUnit)
	if family == nil {
		return 0, fmt.Errorf("unit converter: unknown unit %q", srcUnit)
	}
	if factor == 0 {
		return value, nil
	}
	return value * factor, nil
}

// FromBaseUnit 将基单位的 value 转为 dstUnit。
func (uc *UnitConverter) FromBaseUnit(value float64, dstUnit string) (float64, error) {
	if isFahrenheit(dstUnit) {
		return value*9/5 + 32, nil
	}
	family, factor := uc.lookup(dstUnit)
	if family == nil {
		return 0, fmt.Errorf("unit converter: unknown unit %q", dstUnit)
	}
	if factor == 0 {
		return value, nil
	}
	return value / factor, nil
}

// ToBaseDelta converts a value used as an offset. Temperature offsets scale
// without applying the absolute Fahrenheit/Celsius origin shift.
func (uc *UnitConverter) ToBaseDelta(value float64, srcUnit string) (float64, error) {
	if isFahrenheit(srcUnit) {
		return value * 5 / 9, nil
	}
	return uc.ToBaseUnit(value, srcUnit)
}

func (uc *UnitConverter) FromBaseDelta(value float64, dstUnit string) (float64, error) {
	if isFahrenheit(dstUnit) {
		return value * 9 / 5, nil
	}
	return uc.FromBaseUnit(value, dstUnit)
}

func isFahrenheit(unit string) bool {
	return unit == "℉" || unit == "°F" || unit == "degF"
}

// lookup 查找单位所属族及其换算系数。
// 返回 (family, factor)：
//   - family 为 nil 表示未找到
//   - factor 为 FromSource→Base 的乘数（e.g. kPa→Pa = ×1000）
func (uc *UnitConverter) lookup(unit string) (*UnitFamily, float64) {
	for i := range uc.families {
		if factor, ok := uc.families[i].Factors[unit]; ok {
			return &uc.families[i], factor
		}
	}
	return nil, 0
}

// BaseUnitFor 返回指定单位所属族的基单位（如 "Pa" → "Pa"、"kPa" → "Pa"、"℃" → "℃"）。
func (uc *UnitConverter) BaseUnitFor(unit string) (string, bool) {
	if isFahrenheit(unit) {
		return "℃", true
	}
	family, _ := uc.lookup(unit)
	if family == nil {
		return "", false
	}
	return family.BaseUnit, true
}

// SupportsZeroCalibration reports whether a unit belongs to the pressure family.
// Temperature and non-pressure engineering units must never be zero-calibrated.
func (uc *UnitConverter) SupportsZeroCalibration(unit string) bool {
	baseUnit, ok := uc.BaseUnitFor(unit)
	return ok && baseUnit == "Pa"
}
