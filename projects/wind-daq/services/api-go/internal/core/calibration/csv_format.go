package calibration

import "strconv"

// 三孔 CSV 数据精度常量——与前端 ThreeHoleMain.vue 的 formatValue 调用保持一致，
// 确保 CSV 保存的数据精度与 UI 显示精度一致，避免操作员看到"界面 3 位、CSV 全位数"的割裂感。
const (
	threeHoleThetaPrecision    = 1 // θ 角度：前端 formatValue(coordinates['θ'], 1)
	threeHolePressurePrecision = 3 // 压力通道：前端 getProbeChannelPrecision 默认 3
	threeHoleCoeffPrecision    = 4 // 系数 Kb/K0/Kv：前端 formatValue(Kb, 4)
	threeHoleStdDevPrecision   = 4 // 标准差：前端 formatValue(point.stdDev, 4)
	threeHoleMachPrecision     = 3 // 马赫数：前端 physics.machNumber.toFixed(3)
	threeHoleVelocityPrecision = 3 // 速度 m/s：前端 physics.velocity.toFixed(3)，与马赫数精度对齐
)

// 总压 CSV 数据精度常量——与前端 TotalPressureMain.vue 的 formatValue 调用保持一致，
// 确保 CSV 与 UI 显示精度严格对齐。表头去掉了采样次数/标准差/开始时间/结束时间四列，
// 剩余列精度按 UI 概览页"原始数据卡片 / 系数卡片 / 顶部状态栏"的 toFixed 调用确定。
const (
	totalPressureAlphaPrecision    = 1 // α 角度：前端 formatValue(point.alpha, 1)
	totalPressurePressurePrecision = 1 // 压力通道：前端 formatValue(latestRawData.pXxx, 1)
	totalPressureTempPrecision     = 1 // 温度通道：前端 formatValue(latestRawData.tAtm, 1)
	totalPressureCoeffPrecision    = 4 // 系数 CPT/误差：前端 formatValue(CPT, 4) / formatValue(error, 4)
	totalPressureMachPrecision     = 3 // 马赫数：前端 machNumber.toFixed(3)
	totalPressureVelocityPrecision = 3 // 速度 m/s：前端 velocity.toFixed(3)，与马赫数精度对齐
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
