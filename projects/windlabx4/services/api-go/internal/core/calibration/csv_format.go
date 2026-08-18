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

// 七孔 CSV 数据精度常量（spec §7.4 数值精度要求）
//
// 与 spec §7.4 表格严格对齐：
//   - 压力 Pa：3 位小数
//   - 马赫数：4 位小数（spec §7.4 明确要求 4 位，与五孔/三孔 3 位不同）
//   - 速度：3 位小数
//   - 系数（Kα/Kβ/K0/Ks/Kθ[n] 等）：4 位小数
//   - 角度：3 位小数（spec §7.4 明确要求 3 位，高于五孔/三孔 1 位）
//   - 大气温度：1 位小数（与五孔 T∞ 对齐，温度通道 UI 默认 1 位）
//   - 标准差：4 位小数（与三孔/五孔对齐，便于跨模块统一分析）
//
// 设计权衡：spec §7.4 角度精度 3 位高于数据集 round 到 1 位的精度——
// 数据集 round 是为避免浮点累积误差，CSV 落盘保留 3 位是为校准证书导出时
// 给出更高分辨率的角度数据，便于后处理工具读取精确值。
const (
	sevenHoleAnglePrecision    = 3 // 角度 α/β/θ/φ：spec §7.4 要求 3 位小数
	sevenHolePressurePrecision = 3 // 压力 P1~P7/P0/Ps/大气压力：spec §7.4 要求 3 位小数
	sevenHoleTempPrecision     = 1 // 温度 大气温度：与五孔 T∞ 对齐
	sevenHoleCoeffPrecision    = 4 // 系数 Kα/Kβ/K0/Ks/Kθ[n] 等：spec §7.4 要求 4 位小数
	sevenHoleMachPrecision     = 4 // 马赫数：spec §7.4 要求 4 位小数（高于五孔 3 位）
	sevenHoleVelocityPrecision = 3 // 速度 m/s：spec §7.4 要求 3 位小数
	sevenHoleStdDevPrecision   = 4 // 标准差：与三孔/五孔对齐
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
