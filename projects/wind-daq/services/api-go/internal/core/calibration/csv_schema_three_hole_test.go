package calibration

import "testing"

// TestThreeHoleCsvSchemaChineseHeader 验证三孔 CSV 表头为中文且不含 startTime/endTime
// 测试前置：构造三孔校准配置
// 测试步骤：调用 BuildHeader 获取表头
// 期待结果：表头全中文，列数 16，含马赫数(Ma)/速度V(m/s)，不含 startTime/endTime
func TestThreeHoleCsvSchemaChineseHeader(t *testing.T) {
	config := Config{
		TaskID: "cal-3h",
		Type:   string(TypeThreeHole),
		ProbeChannels: []ProbeChannel{
			{Role: "threeHole.p1", Name: "P1", DeviceID: "dev-1", ChannelIndex: 1, Enabled: true},
			{Role: "threeHole.p2", Name: "P2", DeviceID: "dev-1", ChannelIndex: 2, Enabled: true},
			{Role: "threeHole.p3", Name: "P3", DeviceID: "dev-1", ChannelIndex: 3, Enabled: true},
		},
		Points: []CalPoint{{ID: 1, Coordinates: map[string]float64{"θ": 15}}},
	}
	schema := NewCsvSchema(config)
	header := schema.BuildHeader()

	expected := []string{
		"点位编号", "θ(°)",
		"P1(Pa)", "P2(Pa)", "P3(Pa)", "P∞(Pa)", "T∞(°C)", "Pt(Pa)", "Ps(Pa)",
		"Kβ", "K0", "Kv",
		"马赫数(Ma)", "速度V(m/s)",
		"采样次数", "标准差",
	}
	if len(header) != len(expected) {
		t.Fatalf("expected %d columns, got %d: %v", len(expected), len(header), header)
	}
	for i, h := range expected {
		if header[i] != h {
			t.Fatalf("column %d: expected %q, got %q", i, h, header[i])
		}
	}
	// 确保不含 startTime/endTime
	for _, h := range header {
		if h == "startTime" || h == "endTime" {
			t.Fatalf("header should not contain startTime/endTime, got %v", header)
		}
	}
}

// TestThreeHoleCsvRecordPrecision 验证三孔 CSV 数据行精度与前端显示一致
// 测试前置：构造三孔数据点，θ=15.5°，压力值带多位小数，系数带多位小数，马赫数/速度带多位小数
// 测试步骤：调用 BuildRecord 获取数据行
// 期待结果：θ 1 位、压力 3 位、系数 4 位、马赫数 3 位、速度 1 位、标准差 4 位
func TestThreeHoleCsvRecordPrecision(t *testing.T) {
	pTotal := 80.123456
	pStatic := 15.987654
	ma := 0.234567
	v := 78.967
	dp := &ThreeHoleDataPoint{
		PointID:     3,
		Coordinates: map[string]float64{"θ": 15.567},
		RawData: ThreeHoleRawData{
			P1: 152.123456, P2: 280.987654, P3: 148.111111,
			PAtm: 101325.555, TAtm: 25.123,
			PTotal: &pTotal, PStatic: &pStatic,
		},
		Coefficients: ThreeHoleCoefficients{
			Kb: -0.123456789, K0: 0.83931234, Kv: 3.41978,
			MachNumber: &ma, Velocity: &v,
		},
		SampleCount: 10,
		StdDev:      0.01234,
	}

	config := Config{Type: string(TypeThreeHole)}
	record := NewCsvSchema(config).BuildRecord(dp)

	// θ 应为 15.6（1 位小数）
	if record[1] != "15.6" {
		t.Fatalf("θ precision: expected 15.6, got %s", record[1])
	}
	// P1 应为 152.123（3 位小数）
	if record[2] != "152.123" {
		t.Fatalf("P1 precision: expected 152.123, got %s", record[2])
	}
	// Kb 应为 -0.1235（4 位小数）
	if record[9] != "-0.1235" {
		t.Fatalf("Kb precision: expected -0.1235, got %s", record[9])
	}
	// 马赫数应为 0.235（3 位小数）
	if record[12] != "0.235" {
		t.Fatalf("MachNumber precision: expected 0.235, got %s", record[12])
	}
	// 速度应为 79.0（1 位小数）
	if record[13] != "79.0" {
		t.Fatalf("Velocity precision: expected 79.0, got %s", record[13])
	}
	// 标准差应为 0.0123（4 位小数）
	if record[15] != "0.0123" {
		t.Fatalf("StdDev precision: expected 0.0123, got %s", record[15])
	}
}