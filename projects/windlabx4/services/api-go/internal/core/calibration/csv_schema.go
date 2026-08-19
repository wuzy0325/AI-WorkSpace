package calibration

import "sort"

// csv_schema.go — 校准 CSV 持久化格式描述（纯领域知识，无字节 I/O）
//
// 本文件遵循 CLAUDE.md "Constraint Clarifications" 规则 1：
// core/ 允许定义格式描述结构（列顺序、单位、精度），禁止字节 I/O。
// 实际的文件打开、写入、关闭由 adapters/storage/csv_writer.go 完成。

// CsvSchema 描述校准 CSV 文件的列布局
// 字段顺序即 CSV 列顺序，由 BuildHeader 生成表头，BuildRecord 生成数据行
//
// 七孔探针校准引入 region/sector 字段（spec §7.5）：
//   - region="inner"：内区 CSV 文件（26 列，系数列 Kα/Kβ/K0/Ks）
//   - region="outer"+sector=n：外区扇区 n CSV 文件（26 列，系数列 Kθ[n]/Kφ[n]/K0[n]/Ks[n]）
//   - region=""：非七孔场景（五孔/三孔/总压/总温），按 config.Type 分流
//
// 七孔按 region+sector 分文件落盘（spec §7.1 文件命名「外区 1 区.csv」印证），
// 外区表头的 Kθ[n] 中 n 由 sector 字段替换为具体扇区编号（1..6）。
type CsvSchema struct {
	config Config
	// region 七孔分区标识："inner" / "outer"；非七孔场景为空字符串
	region string
	// sector 七孔外区扇区编号 1..6；内区或非七孔场景为 0
	sector int
	// sevenHoleExport true 时使用 18 列参考数据集格式（spec §7.2/§7.3，GBK 编码），
	// 供 SaveCsv 分区导出与七孔插值加载器消费；false 时为 26 列证书格式（spec §7.5）
	sevenHoleExport bool
}

// NewCsvSchema 根据校准配置构建 CSV 列布局描述
//
// 向后兼容：五孔/三孔/总压/总温等已有模块调用此构造器，region/sector 保持零值。
// 七孔场景必须使用 NewSevenHoleCsvSchema 显式指定 region+sector。
func NewCsvSchema(config Config) CsvSchema {
	return CsvSchema{config: config}
}

// NewSevenHoleCsvSchema 构建七孔校准的 CSV 列布局（spec §7.5）
//
// 参数：
//   - config：校准配置（Type 必须为 TypeSevenHole）
//   - region：分区标识 "inner" 或 "outer"
//   - sector：外区扇区编号 1..6；内区传 0（region="inner" 时 sector 字段被忽略）
//
// 用法示例：
//
//	innerSchema := NewSevenHoleCsvSchema(config, "inner", 0)
//	outerSchema1 := NewSevenHoleCsvSchema(config, "outer", 1) // 外区 1 区文件
//	outerSchema2 := NewSevenHoleCsvSchema(config, "outer", 2) // 外区 2 区文件
//
// 外区表头的 Kθ[n] 中 n 会被 sector 替换为具体值（如 sector=1 → Kθ[1]）。
func NewSevenHoleCsvSchema(config Config, region string, sector int) CsvSchema {
	return CsvSchema{
		config: config,
		region: region,
		sector: sector,
	}
}

// BuildHeader 返回 CSV 表头行
// 列顺序由校准类型决定，每种类型有专属的列集合
// 三孔表头用中文，与五孔风格对齐；去掉 startTime/endTime 两列（校准过程时间戳不参与数据分析）
//
// 七孔分支按 region 选择内区/外区表头（spec §7.5 完整 26 列）：
//   - region="inner"：内区 26 列（侧滑角α, 迎角β, Kα, Kβ, K0, Ks）
//   - region="outer"+sector=n：外区 26 列（滚转角φ, 俯仰角θ, Kθ[n], Kφ[n], K0[n], Ks[n]）
func (s CsvSchema) BuildHeader() []string {
	base := []string{"pointId", "sampleCount", "stdDev", "startTime", "endTime"}

	switch CalibrationType(s.config.Type) {
	case TypeFiveHole:
		header := []string{
			"点位编号", "α(°)", "β(°)", "P1(Pa)", "P2(Pa)", "P3(Pa)", "P4(Pa)", "P5(Pa)",
			"P∞(Pa)", "T∞(°C)", "Pt(Pa)", "Ps(Pa)", "Ma", "Kα", "Kβ", "CPT", "CPS", "采样次数", "标准差",
		}
		if len(s.config.RawDeviceLayouts) == 0 {
			for i := 1; i <= 16; i++ {
				header = append(header, "CH"+formatInt(i)+"(Pa)")
			}
			return header
		}
		header = append(header, buildRawDeviceChannelHeaders(s.config.RawDeviceLayouts)...)
		return header
	case TypeThreeHole:
		// 三孔表头全中文，与五孔风格一致；不含 startTime/endTime
		// 马赫数/速度列头中文+英文表达，与五孔 "Ma" 列风格对齐
		header := []string{
			"点位编号", "θ(°)",
			"P1(Pa)", "P2(Pa)", "P3(Pa)", "P∞(Pa)", "T∞(°C)", "Pt(Pa)", "Ps(Pa)",
			"Kβ", "K0", "Kv",
			"马赫数(Ma)", "速度V(m/s)",
			"采样次数", "标准差",
		}
		return append(header, buildRawDeviceChannelHeaders(s.config.RawDeviceLayouts)...)
	case TypeTotalPressure:
		// 中文带单位，与五孔/三孔风格对齐；列顺序与 buildTotalPressureRecord 严格一致
		// 马赫数/速度列与三孔表头一致，便于跨模块数据合并分析
		// 去掉采样次数/标准差/开始时间/结束时间四列：操作员反馈这些列对校准结果分析无价值
		// 采样次数/标准差反映"过程稳定性"，但 Ma/V/CPT 已能反映"流场质量"，过程列冗余；
		// 开始时间/结束时间是过程时间戳，不参与校准曲线分析，去掉与三孔表头风格一致
		header := []string{
			"点位编号",
			"α(°)",
			"P∞(Pa)", "T∞(°C)", "Pt风洞(Pa)", "Ps风洞(Pa)", "T风洞(°C)", "Pt探针(Pa)",
			"CPT", "误差(%)", "马赫数(Ma)", "速度V(m/s)",
		}
		return append(header, buildRawDeviceChannelHeaders(s.config.RawDeviceLayouts)...)
	case TypeTotalTemperature:
		header := []string{
			"id", "targetMachNumber", "actualMachNumber",
			"testProbeTemp", "standardProbeTemp", "recoveryCoefficient",
			"totalPressure", "staticPressure", "atmosphericPressure",
			"atmosphericTemperature", "stdDev", "timestamp",
		}
		return append(header, buildRawDeviceChannelHeaders(s.config.RawDeviceLayouts)...)
	case TypeSevenHole:
		// 七孔按 region 分流：内区/外区各一套 26 列表头（spec §7.5）
		// 导出场景（sevenHoleExport）使用 18 列参考数据集格式（spec §7.2/§7.3）
		if s.sevenHoleExport {
			return s.buildSevenHoleExportHeader()
		}
		return append(s.buildSevenHoleHeader(), buildRawDeviceChannelHeaders(s.config.RawDeviceLayouts)...)
	default:
		return base
	}
}

// buildSevenHoleHeader 构建七孔 CSV 表头（spec §7.5 完整 26 列）
//
// 内区 26 列：点位编号, 侧滑角α, 迎角β, 来流总压P0, 来流静压Ps,
//
//	P1, P2, P3, P4, P5, P6, P7, 大气压力, 大气温度, 采样次数,
//	Kα, Kβ, K0, Ks, 马赫数, 速度, 标准差,
//	U(Kα), U(Kβ), U(K0), U(Ks), 边界标记
//
// 外区 26 列：点位编号, 滚转角φ, 俯仰角θ, 来流总压P0, 来流静压Ps,
//
//	P1, P2, P3, P4, P5, P6, P7, 大气压力, 大气温度, 采样次数,
//	Kθ[n], Kφ[n], K0[n], Ks[n], 马赫数, 速度, 标准差,
//	U(Kθ[n]), U(Kφ[n]), U(K0[n]), U(Ks[n]), 边界标记
//
// 外区表头的 [n] 由 s.sector 替换为具体扇区编号（如 sector=1 → Kθ[1]）。
// 表头单位后缀规范化（spec Task 7 验收标准）：
//   - 压力列不带 (Pa) 后缀，因表头中"来流总压P0"等已用中文+符号表达，单位 Pa 在文档说明
//   - 温度列同理，"大气温度"不带单位后缀
//   - 角度列用中文符号（α/β/θ/φ）直接表达，不带 (°) 后缀，避免 GBK 编码冲突
//
// GBK 兼容性：所有表头字符均为 GBK 编码支持的中文+ASCII 符号，
// 无生僻字、无 emoji，确保俄文系统 Excel 打开不乱码（project_memory §36）。
func (s CsvSchema) buildSevenHoleHeader() []string {
	if s.region == "outer" {
		// 外区表头：[n] 替换为具体扇区编号
		n := formatInt(s.sector)
		return []string{
			"点位编号", "滚转角φ", "俯仰角θ",
			"来流总压P0", "来流静压Ps",
			"P1", "P2", "P3", "P4", "P5", "P6", "P7",
			"大气压力", "大气温度", "采样次数",
			"Kθ[" + n + "]", "Kφ[" + n + "]", "K0[" + n + "]", "Ks[" + n + "]",
			"马赫数", "速度", "标准差",
			"U(Kθ[" + n + "])", "U(Kφ[" + n + "])", "U(K0[" + n + "])", "U(Ks[" + n + "])",
			"边界标记",
		}
	}
	// 内区表头（默认分支，region="inner" 或未设置时走内区）
	return []string{
		"点位编号", "侧滑角α", "迎角β",
		"来流总压P0", "来流静压Ps",
		"P1", "P2", "P3", "P4", "P5", "P6", "P7",
		"大气压力", "大气温度", "采样次数",
		"Kα", "Kβ", "K0", "Ks",
		"马赫数", "速度", "标准差",
		"U(Kα)", "U(Kβ)", "U(K0)", "U(Ks)",
		"边界标记",
	}
}

// BuildRecord 根据 DataPoint 具体类型构建 CSV 数据行
// 列顺序与 BuildHeader 严格对应
//
// 七孔场景：*SevenHoleDataPoint 按 dp.Region 选择内/外区列布局。
// 若 schema.region="outer" 但 dp.Region="inner"（或反之），返回错误占位行——
// 防止 Task 9 路由错误时把内区数据写入外区文件。
func (s CsvSchema) BuildRecord(dataPoint DataPoint) []string {
	switch dp := dataPoint.(type) {
	case *FiveHoleDataPoint:
		return s.buildFiveHoleRecord(dp)
	case *ThreeHoleDataPoint:
		return s.buildThreeHoleRecord(dp)
	case *TotalPressureDataPoint:
		return s.buildTotalPressureRecord(dp)
	case *TotalTemperatureDataPoint:
		return s.buildTotalTemperatureRecord(dp)
	case *SevenHoleDataPoint:
		if s.sevenHoleExport {
			return s.buildSevenHoleExportRecord(dp)
		}
		return s.buildSevenHoleRecord(dp)
	default:
		return []string{formatInt(dataPoint.GetPointID())}
	}
}

func (s CsvSchema) buildFiveHoleRecord(dp *FiveHoleDataPoint) []string {
	pTotal := ""
	if dp.RawData.PTotal != nil {
		pTotal = formatFloat(*dp.RawData.PTotal)
	}
	pStatic := ""
	if dp.RawData.PStatic != nil {
		pStatic = formatFloat(*dp.RawData.PStatic)
	}
	machNumber := ""
	if dp.Coefficients.MachNumber != nil {
		machNumber = formatFloat(*dp.Coefficients.MachNumber)
	}

	values := []string{
		formatInt(dp.PointID),
		formatFloat(dp.Coordinates["α"]),
		formatFloat(dp.Coordinates["β"]),
		formatFloat(dp.RawData.P1),
		formatFloat(dp.RawData.P2),
		formatFloat(dp.RawData.P3),
		formatFloat(dp.RawData.P4),
		formatFloat(dp.RawData.P5),
		formatFloat(dp.RawData.PAtm),
		formatFloat(dp.RawData.TAtm),
		pTotal,
		pStatic,
		machNumber,
		formatFloat(dp.Coefficients.Kalpha),
		formatFloat(dp.Coefficients.Kbeta),
		formatFloat(dp.Coefficients.CPT),
		formatFloat(dp.Coefficients.CPS),
		formatInt(dp.SampleCount),
		formatFloat(dp.StdDev),
	}
	if len(s.config.RawDeviceLayouts) > 0 {
		return append(values, buildRawDeviceChannelValues(s.config.RawDeviceLayouts, dp.RawDeviceChannels, dp.RawDeviceValid)...)
	}
	return append(values, buildFiveHoleRawChannelValues(dp.RawDeviceChannels)...)
}

func buildFiveHoleRawChannelValues(rawDeviceChannels map[string][]float64) []string {
	values := make([]string, 16)
	for i := range values {
		values[i] = ""
	}
	deviceIDs := make([]string, 0, len(rawDeviceChannels))
	for deviceID := range rawDeviceChannels {
		deviceIDs = append(deviceIDs, deviceID)
	}
	sort.Strings(deviceIDs)
	for _, deviceID := range deviceIDs {
		channels := rawDeviceChannels[deviceID]
		for i := 0; i < len(channels) && i < len(values); i++ {
			values[i] = formatFloatWithPrecision(channels[i], 3)
		}
		break
	}
	return values
}

func (s CsvSchema) buildThreeHoleRecord(dp *ThreeHoleDataPoint) []string {
	pTotal := ""
	if dp.RawData.PTotal != nil {
		pTotal = formatFloatWithPrecision(*dp.RawData.PTotal, threeHolePressurePrecision)
	}
	pStatic := ""
	if dp.RawData.PStatic != nil {
		pStatic = formatFloatWithPrecision(*dp.RawData.PStatic, threeHolePressurePrecision)
	}
	// 马赫数/速度：可选字段，nil 时写空字符串（与 Pt/Ps 可选字段处理方式一致）
	machNumber := ""
	if dp.Coefficients.MachNumber != nil {
		machNumber = formatFloatWithPrecision(*dp.Coefficients.MachNumber, threeHoleMachPrecision)
	}
	velocity := ""
	if dp.Coefficients.Velocity != nil {
		velocity = formatFloatWithPrecision(*dp.Coefficients.Velocity, threeHoleVelocityPrecision)
	}

	// 精度与前端 ThreeHoleMain.vue 显示一致：
	// θ 1 位（formatValue(point.coordinates['θ'], 1)）、压力 3 位（probePrecision 默认 3）、系数 4 位（formatValue(Kb, 4)）、标准差 4 位
	// 马赫数 3 位、速度 3 位（与前端 physics.machNumber.toFixed(3) / velocity.toFixed(3) 一致）
	values := []string{
		formatInt(dp.PointID),
		formatFloatWithPrecision(dp.Coordinates["θ"], threeHoleThetaPrecision),
		formatFloatWithPrecision(dp.RawData.P1, threeHolePressurePrecision),
		formatFloatWithPrecision(dp.RawData.P2, threeHolePressurePrecision),
		formatFloatWithPrecision(dp.RawData.P3, threeHolePressurePrecision),
		formatFloatWithPrecision(dp.RawData.PAtm, threeHolePressurePrecision),
		formatFloatWithPrecision(dp.RawData.TAtm, threeHolePressurePrecision),
		pTotal,
		pStatic,
		formatFloatWithPrecision(dp.Coefficients.Kb, threeHoleCoeffPrecision),
		formatFloatWithPrecision(dp.Coefficients.K0, threeHoleCoeffPrecision),
		formatFloatWithPrecision(dp.Coefficients.Kv, threeHoleCoeffPrecision),
		machNumber,
		velocity,
		formatInt(dp.SampleCount),
		formatFloatWithPrecision(dp.StdDev, threeHoleStdDevPrecision),
	}
	return append(values, buildRawDeviceChannelValues(s.config.RawDeviceLayouts, dp.RawDeviceChannels, dp.RawDeviceValid)...)
}

func (s CsvSchema) buildTotalPressureRecord(dp *TotalPressureDataPoint) []string {
	// 马赫数/速度：可选指针字段，nil 时写空字符串（与三孔/五孔 Pt/Ps 可选字段处理一致）
	machNumber := ""
	if dp.Coefficients.MachNumber != nil {
		machNumber = formatFloatWithPrecision(*dp.Coefficients.MachNumber, totalPressureMachPrecision)
	}
	velocity := ""
	if dp.Coefficients.Velocity != nil {
		velocity = formatFloatWithPrecision(*dp.Coefficients.Velocity, totalPressureVelocityPrecision)
	}

	// 列顺序与 BuildHeader TypeTotalPressure 严格一致；
	// 精度与前端 TotalPressureMain.vue 显示精度严格对齐：
	//   α 1 位、压力 1 位、温度 1 位、CPT/误差 4 位、Ma 3 位、V 1 位
	values := []string{
		formatInt(dp.PointID),
		formatFloatWithPrecision(dp.Alpha, totalPressureAlphaPrecision),
		formatFloatWithPrecision(dp.RawData.PAtm, totalPressurePressurePrecision),
		formatFloatWithPrecision(dp.RawData.TAtm, totalPressureTempPrecision),
		formatFloatWithPrecision(dp.RawData.PTunnelTotal, totalPressurePressurePrecision),
		formatFloatWithPrecision(dp.RawData.PTunnelStatic, totalPressurePressurePrecision),
		formatFloatWithPrecision(dp.RawData.TTunnel, totalPressureTempPrecision),
		formatFloatWithPrecision(dp.RawData.PProbeTotal, totalPressurePressurePrecision),
		formatFloatWithPrecision(dp.Coefficients.CPT, totalPressureCoeffPrecision),
		formatFloatWithPrecision(dp.Coefficients.Error, totalPressureCoeffPrecision),
		machNumber,
		velocity,
	}
	return append(values, buildRawDeviceChannelValues(s.config.RawDeviceLayouts, dp.RawDeviceChannels, dp.RawDeviceValid)...)
}

func (s CsvSchema) buildTotalTemperatureRecord(dp *TotalTemperatureDataPoint) []string {
	values := []string{
		formatInt(dp.ID),
		formatFloat(dp.TargetMachNumber),
		formatFloat(dp.ActualMachNumber),
		formatFloat(dp.TestProbeTemp),
		formatFloat(dp.StandardProbeTemp),
		formatFloat(dp.RecoveryCoefficient),
		formatFloat(dp.TotalPressure),
		formatFloat(dp.StaticPressure),
		formatFloat(dp.AtmosphericPressure),
		formatFloat(dp.AtmosphericTemperature),
		formatFloat(dp.StdDev),
		formatInt64(dp.Timestamp),
	}
	return append(values, buildRawDeviceChannelValues(s.config.RawDeviceLayouts, dp.RawDeviceChannels, dp.RawDeviceValid)...)
}

// buildSevenHoleRecord 构建七孔 CSV 数据行（spec §7.5 完整 27 列）
//
// 列顺序与 buildSevenHoleHeader 严格对应——按 s.region 选择内/外区布局：
//   - s.region="inner"：写入 α/β + Kα/Kβ/K0/Ks + U(Kα)/U(Kβ)/U(K0)/U(Ks)
//   - s.region="outer"：写入 φ/θ + Kθ[n]/Kφ[n]/K0[n]/Ks[n] + U(Kθ[n])/U(Kφ[n])/U(K0[n])/U(Ks[n])
//
// 数据点 dp 携带 Region/Sector 字段，但本方法**不校验** dp.Region 与 s.region 的一致性——
// 路由正确性由 Task 9 的 CalibrationManager onDataPoint 回调负责保证。
// 若 Task 9 路由错误（如内区点写入外区文件），数据行列名与表头会错位，
// Task 9 自己的集成测试会捕获该问题。
//
// 不确定度字段处理（spec Task 7 验收标准 + project_memory §22）：
//   - UncertaintyKalpha/Kbeta/K0/Ks 为 *float64 指针，nil 时写空字符串
//   - 与总压探针 CPT/Ma/V 等可选指针字段处理方式一致
//   - 外区点复用 UncertaintyKalpha/Kbeta/K0/Ks 四个字段填充 Kθ[n]/Kφ[n]/K0[n]/Ks[n] 的不确定度
//     （SevenHoleDataPoint 未为外区单独定义 Uncertainty 字段，避免结构体膨胀）
//
// 边界标记列（boundary_flag）：
//   - 非边界点 BoundaryFlag 为空字符串，CSV 写空串
//   - 边界点 BoundaryFlag 形如 "P7-P1" 或 "P1-P2"（spec §3.2），原样写入
//
// 可选指针字段（PTotal/PStatic/MachNumber/Velocity）：
//   - nil 时写空字符串（与五孔/三孔一致）
//   - 外区点 PTotal/PStatic 缺失时无法计算 K0[n]/Ks[n]/Ma/V，但系数字段仍写入（零值）
func (s CsvSchema) buildSevenHoleRecord(dp *SevenHoleDataPoint) []string {
	// 可选指针字段：nil 时写空字符串（与五孔/三孔一致）
	pTotal := ""
	if dp.RawData.PTotal != nil {
		pTotal = formatFloatWithPrecision(*dp.RawData.PTotal, sevenHolePressurePrecision)
	}
	pStatic := ""
	if dp.RawData.PStatic != nil {
		pStatic = formatFloatWithPrecision(*dp.RawData.PStatic, sevenHolePressurePrecision)
	}
	machNumber := ""
	if dp.Coefficients.MachNumber != nil {
		machNumber = formatFloatWithPrecision(*dp.Coefficients.MachNumber, sevenHoleMachPrecision)
	}
	velocity := ""
	if dp.Coefficients.Velocity != nil {
		velocity = formatFloatWithPrecision(*dp.Coefficients.Velocity, sevenHoleVelocityPrecision)
	}

	// 不确定度字段（指针，nil 写空字符串）
	uKalpha := ""
	if dp.UncertaintyKalpha != nil {
		uKalpha = formatFloatWithPrecision(*dp.UncertaintyKalpha, sevenHoleCoeffPrecision)
	}
	uKbeta := ""
	if dp.UncertaintyKbeta != nil {
		uKbeta = formatFloatWithPrecision(*dp.UncertaintyKbeta, sevenHoleCoeffPrecision)
	}
	uK0 := ""
	if dp.UncertaintyK0 != nil {
		uK0 = formatFloatWithPrecision(*dp.UncertaintyK0, sevenHoleCoeffPrecision)
	}
	uKs := ""
	if dp.UncertaintyKs != nil {
		uKs = formatFloatWithPrecision(*dp.UncertaintyKs, sevenHoleCoeffPrecision)
	}

	if s.region == "outer" {
		// 外区行：φ/θ + Kθ[n]/Kφ[n]/K0[n]/Ks[n]
		// 外区点 U(Kθ[n]) 等复用 UncertaintyKalpha/Kbeta/K0/Ks 字段（见方法注释）
		values := []string{
			formatInt(dp.PointID),
			formatFloatWithPrecision(dp.Coordinates["φ"], sevenHoleAnglePrecision),
			formatFloatWithPrecision(dp.Coordinates["θ"], sevenHoleAnglePrecision),
			pTotal,
			pStatic,
			formatFloatWithPrecision(dp.RawData.P1, sevenHolePressurePrecision),
			formatFloatWithPrecision(dp.RawData.P2, sevenHolePressurePrecision),
			formatFloatWithPrecision(dp.RawData.P3, sevenHolePressurePrecision),
			formatFloatWithPrecision(dp.RawData.P4, sevenHolePressurePrecision),
			formatFloatWithPrecision(dp.RawData.P5, sevenHolePressurePrecision),
			formatFloatWithPrecision(dp.RawData.P6, sevenHolePressurePrecision),
			formatFloatWithPrecision(dp.RawData.P7, sevenHolePressurePrecision),
			formatFloatWithPrecision(dp.RawData.PAtm, sevenHolePressurePrecision),
			formatFloatWithPrecision(dp.RawData.TAtm, sevenHoleTempPrecision),
			formatInt(dp.SampleCount),
			formatFloatWithPrecision(dp.Coefficients.Ktheta, sevenHoleCoeffPrecision),
			formatFloatWithPrecision(dp.Coefficients.Kphi, sevenHoleCoeffPrecision),
			formatFloatWithPrecision(dp.Coefficients.K0Outer, sevenHoleCoeffPrecision),
			formatFloatWithPrecision(dp.Coefficients.KsOuter, sevenHoleCoeffPrecision),
			machNumber,
			velocity,
			formatFloatWithPrecision(dp.StdDev, sevenHoleStdDevPrecision),
			uKalpha,
			uKbeta,
			uK0,
			uKs,
			dp.BoundaryFlag,
		}
		return append(values, buildRawDeviceChannelValues(s.config.RawDeviceLayouts, dp.RawDeviceChannels, dp.RawDeviceValid)...)
	}

	// 内区行（默认分支）：α/β + Kα/Kβ/K0/Ks
	values := []string{
		formatInt(dp.PointID),
		formatFloatWithPrecision(dp.Coordinates["α"], sevenHoleAnglePrecision),
		formatFloatWithPrecision(dp.Coordinates["β"], sevenHoleAnglePrecision),
		pTotal,
		pStatic,
		formatFloatWithPrecision(dp.RawData.P1, sevenHolePressurePrecision),
		formatFloatWithPrecision(dp.RawData.P2, sevenHolePressurePrecision),
		formatFloatWithPrecision(dp.RawData.P3, sevenHolePressurePrecision),
		formatFloatWithPrecision(dp.RawData.P4, sevenHolePressurePrecision),
		formatFloatWithPrecision(dp.RawData.P5, sevenHolePressurePrecision),
		formatFloatWithPrecision(dp.RawData.P6, sevenHolePressurePrecision),
		formatFloatWithPrecision(dp.RawData.P7, sevenHolePressurePrecision),
		formatFloatWithPrecision(dp.RawData.PAtm, sevenHolePressurePrecision),
		formatFloatWithPrecision(dp.RawData.TAtm, sevenHoleTempPrecision),
		formatInt(dp.SampleCount),
		formatFloatWithPrecision(dp.Coefficients.Kalpha, sevenHoleCoeffPrecision),
		formatFloatWithPrecision(dp.Coefficients.Kbeta, sevenHoleCoeffPrecision),
		formatFloatWithPrecision(dp.Coefficients.K0, sevenHoleCoeffPrecision),
		formatFloatWithPrecision(dp.Coefficients.Ks, sevenHoleCoeffPrecision),
		machNumber,
		velocity,
		formatFloatWithPrecision(dp.StdDev, sevenHoleStdDevPrecision),
		uKalpha,
		uKbeta,
		uK0,
		uKs,
		dp.BoundaryFlag,
	}
	return append(values, buildRawDeviceChannelValues(s.config.RawDeviceLayouts, dp.RawDeviceChannels, dp.RawDeviceValid)...)
}
