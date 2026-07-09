package calibration

import "sort"

// csv_schema.go — 校准 CSV 持久化格式描述（纯领域知识，无字节 I/O）
//
// 本文件遵循 CLAUDE.md "Constraint Clarifications" 规则 1：
// core/ 允许定义格式描述结构（列顺序、单位、精度），禁止字节 I/O。
// 实际的文件打开、写入、关闭由 adapters/storage/csv_writer.go 完成。

// CsvSchema 描述校准 CSV 文件的列布局
// 字段顺序即 CSV 列顺序，由 BuildHeader 生成表头，BuildRecord 生成数据行
type CsvSchema struct {
	config Config
}

// NewCsvSchema 根据校准配置构建 CSV 列布局描述
func NewCsvSchema(config Config) CsvSchema {
	return CsvSchema{config: config}
}

// BuildHeader 返回 CSV 表头行
// 列顺序由校准类型决定，每种类型有专属的列集合
// 三孔表头用中文，与五孔风格对齐；去掉 startTime/endTime 两列（校准过程时间戳不参与数据分析）
func (s CsvSchema) BuildHeader() []string {
	base := []string{"pointId", "sampleCount", "stdDev", "startTime", "endTime"}

	switch CalibrationType(s.config.Type) {
	case TypeFiveHole:
		header := []string{
			"点位编号", "α(°)", "β(°)", "P1(Pa)", "P2(Pa)", "P3(Pa)", "P4(Pa)", "P5(Pa)",
			"P∞(Pa)", "T∞(°C)", "Pt(Pa)", "Ps(Pa)", "Ma", "Kα", "Kβ", "CPT", "CPS", "采样次数", "标准差",
		}
		for i := 1; i <= 16; i++ {
			header = append(header, "CH"+formatInt(i)+"(Pa)")
		}
		return header
	case TypeThreeHole:
		// 三孔表头全中文，与五孔风格一致；不含 startTime/endTime
		// 马赫数/速度列头中文+英文表达，与五孔 "Ma" 列风格对齐
		return []string{
			"点位编号", "θ(°)",
			"P1(Pa)", "P2(Pa)", "P3(Pa)", "P∞(Pa)", "T∞(°C)", "Pt(Pa)", "Ps(Pa)",
			"Kβ", "K0", "Kv",
			"马赫数(Ma)", "速度V(m/s)",
			"采样次数", "标准差",
		}
	case TypeTotalPressure:
		// 中文带单位，与五孔/三孔风格对齐；列顺序与 buildTotalPressureRecord 严格一致
		// 马赫数/速度列与三孔表头一致，便于跨模块数据合并分析
		// 去掉采样次数/标准差/开始时间/结束时间四列：操作员反馈这些列对校准结果分析无价值
		// 采样次数/标准差反映"过程稳定性"，但 Ma/V/CPT 已能反映"流场质量"，过程列冗余；
		// 开始时间/结束时间是过程时间戳，不参与校准曲线分析，去掉与三孔表头风格一致
		return []string{
			"点位编号",
			"α(°)",
			"P∞(Pa)", "T∞(°C)", "Pt风洞(Pa)", "Ps风洞(Pa)", "T风洞(°C)", "Pt探针(Pa)",
			"CPT", "误差(%)", "马赫数(Ma)", "速度V(m/s)",
		}
	case TypeTotalTemperature:
		return []string{
			"id", "targetMachNumber", "actualMachNumber",
			"testProbeTemp", "standardProbeTemp", "recoveryCoefficient",
			"totalPressure", "staticPressure", "atmosphericPressure",
			"atmosphericTemperature", "stdDev", "timestamp",
		}
	default:
		return base
	}
}

// BuildRecord 根据 DataPoint 具体类型构建 CSV 数据行
// 列顺序与 BuildHeader 严格对应
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
	// 马赫数 3 位、速度 1 位（与前端 physics.machNumber.toFixed(3) / velocity.toFixed(1) 一致）
	return []string{
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
	return []string{
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
}

func (s CsvSchema) buildTotalTemperatureRecord(dp *TotalTemperatureDataPoint) []string {
	return []string{
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
}
