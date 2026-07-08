package calibration

import "strconv"

// 三孔 CSV 数据精度常量——与前端 ThreeHoleMain.vue 的 formatValue 调用保持一致，
// 确保 CSV 保存的数据精度与 UI 显示精度一致，避免操作员看到"界面 3 位、CSV 全位数"的割裂感。
const (
	threeHoleThetaPrecision    = 1 // θ 角度：前端 formatValue(coordinates['θ'], 1)
	threeHolePressurePrecision = 3 // 压力通道：前端 getProbeChannelPrecision 默认 3
	threeHoleCoeffPrecision    = 4 // 系数 Kb/Kt/Sb：前端 formatValue(Kb, 4)
	threeHoleStdDevPrecision   = 4 // 标准差：前端 formatValue(point.stdDev, 4)
)

func formatFloat(v float64) string {
	return strconv.FormatFloat(v, 'f', -1, 64)
}

func formatFloatWithPrecision(v float64, precision int) string {
	return strconv.FormatFloat(v, 'f', precision, 64)
}

func formatInt(v int) string {
	return strconv.Itoa(v)
}

func formatInt64(v int64) string {
	return strconv.FormatInt(v, 10)
}
