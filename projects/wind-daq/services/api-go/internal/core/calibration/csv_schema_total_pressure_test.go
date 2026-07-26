package calibration

import "testing"

// TestTotalPressureCsvSchemaChineseHeader 验证总压 CSV 表头为中文带单位，
// 与五孔/三孔风格对齐；列顺序与 buildTotalPressureRecord 严格一致。
// 去掉采样次数/标准差/开始时间/结束时间四列：操作员反馈这些列对校准结果分析无价值。
func TestTotalPressureCsvSchemaChineseHeader(t *testing.T) {
	config := Config{
		TaskID: "cal-tp",
		Type:   string(TypeTotalPressure),
		ProbeChannels: []ProbeChannel{
			{Role: "totalPressure.pProbeTotal", Name: "探针总压", DeviceID: "dev-1", ChannelIndex: 0, Enabled: true},
			{Role: "totalPressure.pTunnelTotal", Name: "风洞总压", DeviceID: "dev-1", ChannelIndex: 1, Enabled: true},
			{Role: "totalPressure.pTunnelStatic", Name: "风洞静压", DeviceID: "dev-1", ChannelIndex: 2, Enabled: true},
			{Role: "totalPressure.pAtm", Name: "大气压", DeviceID: "dev-1", ChannelIndex: 16, Enabled: true},
		},
		Points: []CalPoint{{ID: 1, Coordinates: map[string]float64{"α": 10}}},
	}
	schema := NewCsvSchema(config)
	header := schema.BuildHeader()

	expected := []string{
		"点位编号",
		"α(°)",
		"P∞(Pa)", "T∞(°C)", "Pt风洞(Pa)", "Ps风洞(Pa)", "T风洞(°C)", "Pt探针(Pa)",
		"CPT", "误差(%)", "马赫数(Ma)", "速度V(m/s)",
	}
	if len(header) != len(expected) {
		t.Fatalf("expected %d columns, got %d: %v", len(expected), len(header), header)
	}
	for i, h := range expected {
		if header[i] != h {
			t.Fatalf("column %d: expected %q, got %q", i, h, header[i])
		}
	}
}

// TestTotalPressureCsvRecordPrecision 验证总压 CSV 数据行精度与前端 UI 显示精度一致。
// 精度对应 TotalPressureMain.vue：
//   - α: formatValue(point.alpha, 1) → 1 位
//   - 压力通道: formatValue(latestRawData.pXxx, 1) → 1 位
//   - 温度通道: formatValue(latestRawData.tAtm, 1) → 1 位
//   - CPT/误差: formatValue(CPT, 4) / formatValue(error, 4) → 4 位
//   - 马赫数: machNumber.toFixed(3) → 3 位
//   - 速度: velocity.toFixed(3) → 3 位
func TestTotalPressureCsvRecordPrecision(t *testing.T) {
	mach := 0.8234567
	velocity := 278.96123
	dp := &TotalPressureDataPoint{
		PointID: 5,
		Alpha:   12.345678,
		RawData: TotalPressureRawData{
			PAtm:         101325.678,
			TAtm:         22.4567,
			PTunnelTotal: 105000.123,
			PTunnelStatic: 99500.987,
			TTunnel:      24.5678,
			PProbeTotal:  104800.456,
		},
		Coefficients: TotalPressureCoefficients{
			CPT:        0.987654321,
			Error:      0.1234567,
			MachNumber: &mach,
			Velocity:   &velocity,
		},
	}
	schema := NewCsvSchema(Config{Type: string(TypeTotalPressure)})
	record := schema.BuildRecord(dp)

	if len(record) != 12 {
		t.Fatalf("expected 12 columns, got %d: %v", len(record), record)
	}
	// 索引 0=PointID, 1=α, 2=PAtm, 3=TAtm, 4=PTunnelTotal, 5=PTunnelStatic, 6=TTunnel, 7=PProbeTotal,
	// 8=CPT, 9=Error, 10=Mach, 11=Velocity
	cases := []struct {
		index    int
		expected string
	}{
		{0, "5"},
		{1, "12.3"},
		{2, "101325.7"},
		{3, "22.5"},
		{4, "105000.1"},
		{5, "99501.0"},
		{6, "24.6"},
		{7, "104800.5"},
		{8, "0.9877"},
		{9, "0.1235"},
		{10, "0.823"},
		{11, "278.961"},
	}
	for _, c := range cases {
		if record[c.index] != c.expected {
			t.Errorf("column %d: expected %q, got %q", c.index, c.expected, record[c.index])
		}
	}
}

// TestTotalPressureCsvRecordNilMachVelocity 验证马赫数/速度为 nil 指针时写空字符串，
// 与三孔/五孔可选字段处理方式一致。
func TestTotalPressureCsvRecordNilMachVelocity(t *testing.T) {
	dp := &TotalPressureDataPoint{
		PointID: 1,
		Alpha:   5.0,
		RawData: TotalPressureRawData{
			PAtm: 101325.0,
		},
		Coefficients: TotalPressureCoefficients{
			CPT:        0.95,
			Error:      0.05,
			MachNumber: nil,
			Velocity:   nil,
		},
	}
	schema := NewCsvSchema(Config{Type: string(TypeTotalPressure)})
	record := schema.BuildRecord(dp)

	// 索引 10=Mach, 11=Velocity 应为空字符串
	if record[10] != "" {
		t.Errorf("expected empty Mach for nil pointer, got %q", record[10])
	}
	if record[11] != "" {
		t.Errorf("expected empty Velocity for nil pointer, got %q", record[11])
	}
}