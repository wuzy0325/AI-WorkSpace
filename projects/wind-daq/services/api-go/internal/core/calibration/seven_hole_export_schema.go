package calibration

// seven_hole_export_schema.go — 七孔校准"参考数据集格式"导出列布局（纯领域知识，无字节 I/O）
//
// 用途：SaveCsv 全量导出时，七孔结果按 region+sector 分区落盘为 7 份 CSV
// （1 小角度区 + 6 大角度区），列布局与基准数据集 W532.202608.P.7H.1-01 的
// 18 列基础格式逐列对齐（spec §7.2/§7.3）：
//
//	inner: 侧滑角α, 迎角β, 马赫数Ma, 来流总压P0, 来流静压Ps,
//	       P1..P7, α角度系数Kα, β角度系数Kβ, 总压系数K0, 静压系数Ks, 大气压力, 大气温度
//	outer: 滚转角φ, 俯仰角θ, 马赫数Ma, 来流总压P0, 来流静压Ps,
//	       P1..P7, θ角度系数Kθ[n], φ角度系数Kφ[n], 总压系数K0[n], 静压系数Ks[n], 大气压力, 大气温度
//
// 与 26 列证书格式（spec §7.5，逐点采集过程记录）的关系：
//   - 18 列导出格式是"校准交付文件"，供七孔插值加载器
//     （shared/algorithms/go/sevenhole/interpolation csv_loader.go）按位置契约消费——
//     加载器按 col0/1=角度、col3..11=压力重算系数，26 列格式首列是点位编号会错位解析
//   - 数值精度与参考数据集一致：全部列 3 位小数（参考文件系数列也是 3 位，
//     加载器用压力列全精度重算，不依赖 CSV 系数列精度）
//   - 文件编码 GBK（与参考数据集一致；插值加载适配层按 GB18030 解码）
//
// 外区表头命名遵循 spec §7.3（滚转角φ/俯仰角θ + Kθ[n]/Kφ[n]/K0[n]/Ks[n]），
// 不沿用参考数据集的历史遗留错误表头（侧滑角α/迎角β）——加载器按列位置读取，
// 表头名称不影响解析。

// sevenHoleExportPrecision 导出格式统一数值精度（参考数据集全列 3 位小数）
const sevenHoleExportPrecision = 3

// NewSevenHoleExportCsvSchema 构建七孔"参考数据集格式"导出的 CSV 列布局
//
// 参数与 NewSevenHoleCsvSchema 一致；区别仅在列布局（18 列基础格式 vs 26 列证书格式）
// 与文件编码（GBK vs UTF-8 BOM，见 UseGBKEncoding）。
func NewSevenHoleExportCsvSchema(config Config, region string, sector int) CsvSchema {
	return CsvSchema{
		config:          config,
		region:          region,
		sector:          sector,
		sevenHoleExport: true,
	}
}

// UseGBKEncoding 报告该 schema 的 CSV 文件是否应使用 GBK 编码（不写 UTF-8 BOM）
//
// 仅七孔参考数据集格式导出为 true（与基准数据集编码一致，插值加载适配层按
// GB18030 解码）；其余 schema 保持 UTF-8 BOM 现状。
func (s CsvSchema) UseGBKEncoding() bool {
	return s.sevenHoleExport
}

// buildSevenHoleExportHeader 构建 18 列导出格式表头（spec §7.2/§7.3）
func (s CsvSchema) buildSevenHoleExportHeader() []string {
	if s.region == "outer" {
		n := formatInt(s.sector)
		return []string{
			"滚转角φ", "俯仰角θ", "马赫数Ma", "来流总压P0", "来流静压Ps",
			"P1", "P2", "P3", "P4", "P5", "P6", "P7",
			"θ角度系数Kθ[" + n + "]", "φ角度系数Kφ[" + n + "]",
			"总压系数K0[" + n + "]", "静压系数Ks[" + n + "]",
			"大气压力", "大气温度",
		}
	}
	return []string{
		"侧滑角α", "迎角β", "马赫数Ma", "来流总压P0", "来流静压Ps",
		"P1", "P2", "P3", "P4", "P5", "P6", "P7",
		"α角度系数Kα", "β角度系数Kβ", "总压系数K0", "静压系数Ks",
		"大气压力", "大气温度",
	}
}

// buildSevenHoleExportRecord 构建 18 列导出格式数据行
//
// 列顺序与 buildSevenHoleExportHeader 严格对应，全部数值 3 位小数（与参考数据集一致）。
// 可选指针字段（PTotal/PStatic/MachNumber）nil 时写空字符串（与 26 列格式一致）。
func (s CsvSchema) buildSevenHoleExportRecord(dp *SevenHoleDataPoint) []string {
	pTotal := ""
	if dp.RawData.PTotal != nil {
		pTotal = formatFloatWithPrecision(*dp.RawData.PTotal, sevenHoleExportPrecision)
	}
	pStatic := ""
	if dp.RawData.PStatic != nil {
		pStatic = formatFloatWithPrecision(*dp.RawData.PStatic, sevenHoleExportPrecision)
	}
	machNumber := ""
	if dp.Coefficients.MachNumber != nil {
		machNumber = formatFloatWithPrecision(*dp.Coefficients.MachNumber, sevenHoleExportPrecision)
	}

	pressures := []string{
		pTotal,
		pStatic,
		formatFloatWithPrecision(dp.RawData.P1, sevenHoleExportPrecision),
		formatFloatWithPrecision(dp.RawData.P2, sevenHoleExportPrecision),
		formatFloatWithPrecision(dp.RawData.P3, sevenHoleExportPrecision),
		formatFloatWithPrecision(dp.RawData.P4, sevenHoleExportPrecision),
		formatFloatWithPrecision(dp.RawData.P5, sevenHoleExportPrecision),
		formatFloatWithPrecision(dp.RawData.P6, sevenHoleExportPrecision),
		formatFloatWithPrecision(dp.RawData.P7, sevenHoleExportPrecision),
	}
	tail := []string{
		formatFloatWithPrecision(dp.RawData.PAtm, sevenHoleExportPrecision),
		formatFloatWithPrecision(dp.RawData.TAtm, sevenHoleExportPrecision),
	}

	if s.region == "outer" {
		// 外区行：φ,θ + Kθ[n]/Kφ[n]/K0[n]/Ks[n]（列位置与参考数据集大角度区文件一致）
		record := []string{
			formatFloatWithPrecision(dp.Coordinates["φ"], sevenHoleExportPrecision),
			formatFloatWithPrecision(dp.Coordinates["θ"], sevenHoleExportPrecision),
			machNumber,
		}
		record = append(record, pressures...)
		record = append(record,
			formatFloatWithPrecision(dp.Coefficients.Ktheta, sevenHoleExportPrecision),
			formatFloatWithPrecision(dp.Coefficients.Kphi, sevenHoleExportPrecision),
			formatFloatWithPrecision(dp.Coefficients.K0Outer, sevenHoleExportPrecision),
			formatFloatWithPrecision(dp.Coefficients.KsOuter, sevenHoleExportPrecision),
		)
		return append(record, tail...)
	}

	// 内区行：α,β + Kα/Kβ/K0/Ks
	record := []string{
		formatFloatWithPrecision(dp.Coordinates["α"], sevenHoleExportPrecision),
		formatFloatWithPrecision(dp.Coordinates["β"], sevenHoleExportPrecision),
		machNumber,
	}
	record = append(record, pressures...)
	record = append(record,
		formatFloatWithPrecision(dp.Coefficients.Kalpha, sevenHoleExportPrecision),
		formatFloatWithPrecision(dp.Coefficients.Kbeta, sevenHoleExportPrecision),
		formatFloatWithPrecision(dp.Coefficients.K0, sevenHoleExportPrecision),
		formatFloatWithPrecision(dp.Coefficients.Ks, sevenHoleExportPrecision),
	)
	return append(record, tail...)
}
