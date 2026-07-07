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
		return append(base,
			"p1", "p2", "p3", "pAtm", "pTotal",
			"K", "Cv", "Cp",
		)
	case TypeTotalPressure:
		return append(base,
			"alpha",
			"pAtm", "tAtm", "pTunnelTotal", "pTunnelStatic", "tTunnel", "pProbeTotal",
			"CPT", "error", "machNumber",
		)
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
		pTotal = formatFloat(*dp.RawData.PTotal)
	}

	return []string{
		formatInt(dp.PointID),
		formatInt(dp.SampleCount),
		formatFloat(dp.StdDev),
		formatInt64(dp.StartTime),
		formatInt64(dp.EndTime),
		formatFloat(dp.RawData.P1),
		formatFloat(dp.RawData.P2),
		formatFloat(dp.RawData.P3),
		formatFloat(dp.RawData.PAtm),
		pTotal,
		formatFloat(dp.Coefficients.K),
		formatFloat(dp.Coefficients.Cv),
		formatFloat(dp.Coefficients.Cp),
	}
}

func (s CsvSchema) buildTotalPressureRecord(dp *TotalPressureDataPoint) []string {
	return []string{
		formatInt(dp.PointID),
		formatInt(dp.SampleCount),
		formatFloat(dp.StdDev),
		formatInt64(dp.StartTime),
		formatInt64(dp.EndTime),
		formatFloat(dp.Alpha),
		formatFloat(dp.RawData.PAtm),
		formatFloat(dp.RawData.TAtm),
		formatFloat(dp.RawData.PTunnelTotal),
		formatFloat(dp.RawData.PTunnelStatic),
		formatFloat(dp.RawData.TTunnel),
		formatFloat(dp.RawData.PProbeTotal),
		formatFloat(dp.Coefficients.CPT),
		formatFloat(dp.Coefficients.Error),
		formatFloat(dp.Coefficients.MachNumber),
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
