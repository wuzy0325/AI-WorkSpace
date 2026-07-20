package calibration

import (
	"strings"
	"testing"

	"golang.org/x/text/encoding/simplifiedchinese"
)

// ==================== 七孔 CSV 表头测试 ====================

// TestSevenHoleInnerHeader 验证七孔内区 CSV 表头完整 27 列（spec §7.5 实际清单）
// 测试前置：构造七孔校准配置 + 内区 schema
// 测试步骤：调用 BuildHeader 获取内区表头
// 期待结果：27 列，列名与 spec §7.5 内区清单严格一致（含边界标记列）
func TestSevenHoleInnerHeader(t *testing.T) {
	config := Config{
		TaskID: "cal-7h",
		Type:   string(TypeSevenHole),
	}
	schema := NewSevenHoleCsvSchema(config, "inner", 0)
	header := schema.BuildHeader()

	expected := []string{
		"点位编号", "侧滑角α", "迎角β",
		"来流总压P0", "来流静压Ps",
		"P1", "P2", "P3", "P4", "P5", "P6", "P7",
		"大气压力", "大气温度", "采样次数",
		"Kα", "Kβ", "K0", "Ks",
		"马赫数", "速度", "标准差",
		"U(Kα)", "U(Kβ)", "U(K0)", "U(Ks)",
		"边界标记",
	}
	if len(header) != len(expected) {
		t.Fatalf("内区表头列数: expected %d, got %d: %v", len(expected), len(header), header)
	}
	for i, h := range expected {
		if header[i] != h {
			t.Fatalf("内区表头列 %d: expected %q, got %q", i, h, header[i])
		}
	}
}

// TestSevenHoleOuterHeader 验证七孔外区 CSV 表头完整 27 列（Kθ[n] 中 n 由 sector 替换）
// 测试前置：构造七孔校准配置 + 外区 sector=2 schema
// 测试步骤：调用 BuildHeader 获取外区表头
// 期待结果：27 列，Kθ[2]/Kφ[2]/K0[2]/Ks[2] 等系数列含 sector=2 编号
func TestSevenHoleOuterHeader(t *testing.T) {
	config := Config{
		TaskID: "cal-7h",
		Type:   string(TypeSevenHole),
	}
	schema := NewSevenHoleCsvSchema(config, "outer", 2)
	header := schema.BuildHeader()

	expected := []string{
		"点位编号", "滚转角φ", "俯仰角θ",
		"来流总压P0", "来流静压Ps",
		"P1", "P2", "P3", "P4", "P5", "P6", "P7",
		"大气压力", "大气温度", "采样次数",
		"Kθ[2]", "Kφ[2]", "K0[2]", "Ks[2]",
		"马赫数", "速度", "标准差",
		"U(Kθ[2])", "U(Kφ[2])", "U(K0[2])", "U(Ks[2])",
		"边界标记",
	}
	if len(header) != len(expected) {
		t.Fatalf("外区表头列数: expected %d, got %d: %v", len(expected), len(header), header)
	}
	for i, h := range expected {
		if header[i] != h {
			t.Fatalf("外区表头列 %d: expected %q, got %q", i, h, header[i])
		}
	}
}

// TestSevenHoleOuterHeader_SectorRouting 验证外区 sector 路由——不同 sector 替换 Kθ[n] 中 n
// 测试前置：构造 sector=1/3/6 的三个外区 schema
// 测试步骤：分别调用 BuildHeader，检查 Kθ[n] 列名
// 期待结果：sector=1→"Kθ[1]"，sector=3→"Kθ[3]"，sector=6→"Kθ[6]"
func TestSevenHoleOuterHeader_SectorRouting(t *testing.T) {
	config := Config{Type: string(TypeSevenHole)}
	cases := []int{1, 3, 6}
	for _, sector := range cases {
		schema := NewSevenHoleCsvSchema(config, "outer", sector)
		header := schema.BuildHeader()
		// Kθ[n] 在索引 15（点位编号+2角度+2总静压+7孔压+大气压力+大气温度+采样次数 = 15）
		expected := "Kθ[" + formatInt(sector) + "]"
		if header[15] != expected {
			t.Fatalf("sector=%d Kθ 列: expected %q, got %q", sector, expected, header[15])
		}
		// U(Kθ[n]) 在索引 22（Kθ[n] 之后还有 Kφ[n]/K0[n]/Ks[n]/马赫数/速度/标准差 共 6 列，15+6+1=22）
		expectedU := "U(Kθ[" + formatInt(sector) + "])"
		if header[22] != expectedU {
			t.Fatalf("sector=%d U(Kθ) 列: expected %q, got %q", sector, expectedU, header[22])
		}
	}
}

// TestSevenHoleHeaderGBKCompatible 验证七孔 CSV 表头字符均为 GBK 编码支持
// 测试前置：构造内区和外区 schema
// 测试步骤：将表头字符串拼接后用 GBK 编码，编码失败则报错
// 期待结果：所有表头字符均可被 GBK 编码（俄文系统 Excel 打开不乱码，project_memory §36）
func TestSevenHoleHeaderGBKCompatible(t *testing.T) {
	config := Config{Type: string(TypeSevenHole)}

	innerHeader := NewSevenHoleCsvSchema(config, "inner", 0).BuildHeader()
	outerHeader := NewSevenHoleCsvSchema(config, "outer", 1).BuildHeader()

	// GBK 编码器（与 adapters/storage/csv_writer 实际使用的编码一致）
	encoder := simplifiedchinese.GBK.NewEncoder()

	// 拼接内/外区表头为一个字符串，统一编码验证
	combined := strings.Join(innerHeader, ",") + "|" + strings.Join(outerHeader, ",")
	encoded, err := encoder.Bytes([]byte(combined))
	if err != nil {
		t.Fatalf("GBK 编码失败，存在 GBK 不支持的字符: %v, 原文=%q", err, combined)
	}
	// 反向解码验证一致性
	decoder := simplifiedchinese.GBK.NewDecoder()
	decoded, err := decoder.Bytes(encoded)
	if err != nil {
		t.Fatalf("GBK 解码失败: %v", err)
	}
	if string(decoded) != combined {
		t.Fatalf("GBK 编解码不一致: 原文=%q, 解码=%q", combined, string(decoded))
	}
}

// ==================== 七孔 CSV 数据行测试 ====================

// TestSevenHoleInnerRecord 验证七孔内区数据行字段顺序与精度
// 测试前置：构造内区数据点，α=5.123456°, β=-3.234567°, P1..P7 带多位小数, Kα..Ks 带多位小数
// 测试步骤：调用 BuildRecord 获取数据行
// 期待结果：27 列，角度 3 位、压力 3 位、系数 4 位、马赫数 4 位、速度 3 位、标准差 4 位
func TestSevenHoleInnerRecord(t *testing.T) {
	pTotal := 4073.123456
	pStatic := -32.789012
	ma := 0.2415678
	v := 82.34567
	uKalpha := 0.002345
	uKbeta := 0.003456
	uK0 := 0.008012
	uKs := 0.005678

	dp := &SevenHoleDataPoint{
		PointID: 42,
		Coordinates: map[string]float64{
			"α": 5.123456,
			"β": -3.234567,
		},
		Region:  "inner",
		Sector:  7,
		RawData: SevenHoleRawData{P1: 1192.111111, P2: 280.222222, P3: 148.333333, P4: 152.444444, P5: 280.555555, P6: 148.666666, P7: 4075.777777, PAtm: 98880.888889, TAtm: 25.123, PTotal: &pTotal, PStatic: &pStatic},
		Coefficients: SevenHoleCoefficients{
			Kalpha:     0.123456789,
			Kbeta:      -0.234567891,
			K0:         0.000561234,
			Ks:         -0.998877665,
			MachNumber: &ma,
			Velocity:   &v,
		},
		SampleCount:       10,
		StdDev:            0.012345,
		UncertaintyKalpha: &uKalpha,
		UncertaintyKbeta:  &uKbeta,
		UncertaintyK0:     &uK0,
		UncertaintyKs:     &uKs,
	}

	config := Config{Type: string(TypeSevenHole)}
	schema := NewSevenHoleCsvSchema(config, "inner", 0)
	record := schema.BuildRecord(dp)

	if len(record) != 27 {
		t.Fatalf("内区数据行列数: expected 27, got %d: %v", len(record), record)
	}
	// PointID
	if record[0] != "42" {
		t.Fatalf("PointID: expected 42, got %s", record[0])
	}
	// α 应为 5.123（3 位小数）
	if record[1] != "5.123" {
		t.Fatalf("α precision: expected 5.123, got %s", record[1])
	}
	// β 应为 -3.235（3 位小数，四舍五入）
	if record[2] != "-3.235" {
		t.Fatalf("β precision: expected -3.235, got %s", record[2])
	}
	// P0（来流总压）应为 4073.123（3 位小数）
	if record[3] != "4073.123" {
		t.Fatalf("P0 precision: expected 4073.123, got %s", record[3])
	}
	// Ps（来流静压）应为 -32.789（3 位小数）
	if record[4] != "-32.789" {
		t.Fatalf("Ps precision: expected -32.789, got %s", record[4])
	}
	// P1 应为 1192.111（3 位小数）
	if record[5] != "1192.111" {
		t.Fatalf("P1 precision: expected 1192.111, got %s", record[5])
	}
	// P7 应为 4075.778（3 位小数）
	if record[11] != "4075.778" {
		t.Fatalf("P7 precision: expected 4075.778, got %s", record[11])
	}
	// 大气温度应为 25.1（1 位小数）
	if record[13] != "25.1" {
		t.Fatalf("TAtm precision: expected 25.1, got %s", record[13])
	}
	// 采样次数
	if record[14] != "10" {
		t.Fatalf("SampleCount: expected 10, got %s", record[14])
	}
	// Kα 应为 0.1235（4 位小数）
	if record[15] != "0.1235" {
		t.Fatalf("Kα precision: expected 0.1235, got %s", record[15])
	}
	// Kβ 应为 -0.2346（4 位小数）
	if record[16] != "-0.2346" {
		t.Fatalf("Kβ precision: expected -0.2346, got %s", record[16])
	}
	// 马赫数应为 0.2416（4 位小数，spec §7.4 高于五孔 3 位）
	if record[19] != "0.2416" {
		t.Fatalf("Ma precision: expected 0.2416, got %s", record[19])
	}
	// 速度应为 82.346（3 位小数）
	if record[20] != "82.346" {
		t.Fatalf("Velocity precision: expected 82.346, got %s", record[20])
	}
	// 标准差应为 0.0123（4 位小数）
	if record[21] != "0.0123" {
		t.Fatalf("StdDev precision: expected 0.0123, got %s", record[21])
	}
	// U(Kα) 应为 0.0023（4 位小数）
	if record[22] != "0.0023" {
		t.Fatalf("U(Kα) precision: expected 0.0023, got %s", record[22])
	}
	// U(K0) 在索引 24（U(Kα)/U(Kβ)/U(K0)/U(Ks) 依次为 22/23/24/25），应为 0.0080（4 位小数）
	if record[24] != "0.0080" {
		t.Fatalf("U(K0) precision: expected 0.0080, got %s", record[24])
	}
	// U(Ks) 在索引 25，应为 0.0057（4 位小数）
	if record[25] != "0.0057" {
		t.Fatalf("U(Ks) precision: expected 0.0057, got %s", record[25])
	}
	// 边界标记：非边界点为空串
	if record[26] != "" {
		t.Fatalf("BoundaryFlag: expected empty, got %s", record[26])
	}
}

// TestSevenHoleOuterRecord 验证七孔外区数据行字段顺序与精度
// 测试前置：构造外区数据点，φ=60.123456°, θ=35.234567°, Kθ/Kφ/K0Outer/KsOuter 带多位小数
// 测试步骤：调用 BuildRecord 获取数据行
// 期待结果：27 列，角度 3 位、压力 3 位、系数 4 位，外区系数字段正确路由
func TestSevenHoleOuterRecord(t *testing.T) {
	pTotal := 5000.123456
	pStatic := 100.234567
	ma := 0.3501234
	v := 118.56789

	dp := &SevenHoleDataPoint{
		PointID: 200,
		Coordinates: map[string]float64{
			"φ": 60.123456,
			"θ": 35.234567,
		},
		Region:  "outer",
		Sector:  2,
		RawData: SevenHoleRawData{P1: 1000.111111, P2: 3260.222222, P3: 1500.333333, P4: 800.444444, P5: 700.555555, P6: 1100.666666, P7: 600.777777, PAtm: 98880.888889, TAtm: 24.567, PTotal: &pTotal, PStatic: &pStatic},
		Coefficients: SevenHoleCoefficients{
			Ktheta:     0.456789123,
			Kphi:       -0.234567891,
			K0Outer:    0.890123456,
			KsOuter:    -0.135724680,
			MachNumber: &ma,
			Velocity:   &v,
		},
		SampleCount: 10,
		StdDev:      0.023456,
	}

	config := Config{Type: string(TypeSevenHole)}
	schema := NewSevenHoleCsvSchema(config, "outer", 2)
	record := schema.BuildRecord(dp)

	if len(record) != 27 {
		t.Fatalf("外区数据行列数: expected 27, got %d: %v", len(record), record)
	}
	// PointID
	if record[0] != "200" {
		t.Fatalf("PointID: expected 200, got %s", record[0])
	}
	// φ 应为 60.123（3 位小数）
	if record[1] != "60.123" {
		t.Fatalf("φ precision: expected 60.123, got %s", record[1])
	}
	// θ 应为 35.235（3 位小数）
	if record[2] != "35.235" {
		t.Fatalf("θ precision: expected 35.235, got %s", record[2])
	}
	// P0（来流总压）
	if record[3] != "5000.123" {
		t.Fatalf("P0 precision: expected 5000.123, got %s", record[3])
	}
	// Ps（来流静压）
	if record[4] != "100.235" {
		t.Fatalf("Ps precision: expected 100.235, got %s", record[4])
	}
	// P2 应为 3260.222（外区 P2 最大，扇区 2）
	if record[6] != "3260.222" {
		t.Fatalf("P2 precision: expected 3260.222, got %s", record[6])
	}
	// 采样次数
	if record[14] != "10" {
		t.Fatalf("SampleCount: expected 10, got %s", record[14])
	}
	// Kθ[2]（外区系数 Ktheta）应为 0.4568（4 位小数）
	if record[15] != "0.4568" {
		t.Fatalf("Kθ precision: expected 0.4568, got %s", record[15])
	}
	// Kφ[2]（外区系数 Kphi）应为 -0.2346（4 位小数）
	if record[16] != "-0.2346" {
		t.Fatalf("Kφ precision: expected -0.2346, got %s", record[16])
	}
	// K0[2]（外区系数 K0Outer）应为 0.8901（4 位小数）
	if record[17] != "0.8901" {
		t.Fatalf("K0[2] precision: expected 0.8901, got %s", record[17])
	}
	// Ks[2]（外区系数 KsOuter）应为 -0.1357（4 位小数）
	if record[18] != "-0.1357" {
		t.Fatalf("Ks[2] precision: expected -0.1357, got %s", record[18])
	}
	// 马赫数应为 0.3501（4 位小数）
	if record[19] != "0.3501" {
		t.Fatalf("Ma precision: expected 0.3501, got %s", record[19])
	}
	// 速度应为 118.568（3 位小数）
	if record[20] != "118.568" {
		t.Fatalf("Velocity precision: expected 118.568, got %s", record[20])
	}
}

// TestSevenHoleRecord_NilPointers 验证 nil 指针字段（PTotal/PStatic/Ma/V/不确定度）写空字符串
// 测试前置：构造内区数据点，所有可选指针字段为 nil
// 测试步骤：调用 BuildRecord 获取数据行
// 期待结果：P0/Ps/Ma/V/U(Kα) 等列为空字符串，其他非指针字段正常写入
func TestSevenHoleRecord_NilPointers(t *testing.T) {
	dp := &SevenHoleDataPoint{
		PointID: 7,
		Coordinates: map[string]float64{
			"α": 0.0,
			"β": 0.0,
		},
		Region:       "inner",
		Sector:       7,
		RawData:      SevenHoleRawData{P1: 100.123, P2: 100.234, P3: 100.345, P4: 100.456, P5: 100.567, P6: 100.678, P7: 200.789, PAtm: 101325.111, TAtm: 20.0},
		Coefficients: SevenHoleCoefficients{Kalpha: 0.0, Kbeta: 0.0, K0: 0.0, Ks: 0.0},
		SampleCount:  5,
		StdDev:       0.001,
		// PTotal/PStatic/MachNumber/Velocity/UncertaintyXxx 均为 nil
	}

	config := Config{Type: string(TypeSevenHole)}
	schema := NewSevenHoleCsvSchema(config, "inner", 0)
	record := schema.BuildRecord(dp)

	// P0 列（索引 3）应为空字符串
	if record[3] != "" {
		t.Fatalf("P0 nil: expected empty, got %s", record[3])
	}
	// Ps 列（索引 4）应为空字符串
	if record[4] != "" {
		t.Fatalf("Ps nil: expected empty, got %s", record[4])
	}
	// 马赫数列（索引 19）应为空字符串
	if record[19] != "" {
		t.Fatalf("Ma nil: expected empty, got %s", record[19])
	}
	// 速度列（索引 20）应为空字符串
	if record[20] != "" {
		t.Fatalf("Velocity nil: expected empty, got %s", record[20])
	}
	// U(Kα) 列（索引 22）应为空字符串
	if record[22] != "" {
		t.Fatalf("U(Kα) nil: expected empty, got %s", record[22])
	}
	// U(K0) 列（索引 25）应为空字符串
	if record[25] != "" {
		t.Fatalf("U(K0) nil: expected empty, got %s", record[25])
	}
	// 非指针字段应正常写入——P1 仍为 100.123
	if record[5] != "100.123" {
		t.Fatalf("P1 should still be written: expected 100.123, got %s", record[5])
	}
	// P7 仍为 200.789
	if record[11] != "200.789" {
		t.Fatalf("P7 should still be written: expected 200.789, got %s", record[11])
	}
}

// TestSevenHoleRecord_BoundaryFlag 验证边界标记列写入
// 测试前置：构造外区边界点（BoundaryFlag="P1-P2"）和非边界点（BoundaryFlag=""）
// 测试步骤：分别调用 BuildRecord，检查最后一列
// 期待结果：边界点写入 "P1-P2"，非边界点写入空串
func TestSevenHoleRecord_BoundaryFlag(t *testing.T) {
	dp1 := &SevenHoleDataPoint{
		PointID:      100,
		Coordinates:  map[string]float64{"φ": 30.0, "θ": 30.0},
		Region:       "outer",
		Sector:       1,
		BoundaryFlag: "P1-P2",
		RawData:      SevenHoleRawData{P1: 1, P2: 2, P3: 3, P4: 4, P5: 5, P6: 6, P7: 7, PAtm: 101325.0, TAtm: 20.0},
	}
	dp2 := &SevenHoleDataPoint{
		PointID:      101,
		Coordinates:  map[string]float64{"φ": 0.0, "θ": 35.0},
		Region:       "outer",
		Sector:       1,
		BoundaryFlag: "", // 非边界点
		RawData:      SevenHoleRawData{P1: 1, P2: 2, P3: 3, P4: 4, P5: 5, P6: 6, P7: 7, PAtm: 101325.0, TAtm: 20.0},
	}

	config := Config{Type: string(TypeSevenHole)}
	schema := NewSevenHoleCsvSchema(config, "outer", 1)

	r1 := schema.BuildRecord(dp1)
	if r1[26] != "P1-P2" {
		t.Fatalf("边界点 BoundaryFlag: expected P1-P2, got %s", r1[26])
	}

	r2 := schema.BuildRecord(dp2)
	if r2[26] != "" {
		t.Fatalf("非边界点 BoundaryFlag: expected empty, got %s", r2[26])
	}
}

// TestSevenHoleRecord_HeaderRecordAlignment 验证数据行列数与表头列数严格对齐
// 测试前置：构造内区和外区 schema + 数据点
// 测试步骤：分别获取表头和数据行，比较长度
// 期待结果：内区表头 27 列 = 数据行 27 列；外区表头 27 列 = 数据行 27 列
// 防止 Task 7 后续维护时新增列只改表头或数据行导致错位
func TestSevenHoleRecord_HeaderRecordAlignment(t *testing.T) {
	config := Config{Type: string(TypeSevenHole)}

	innerSchema := NewSevenHoleCsvSchema(config, "inner", 0)
	outerSchema := NewSevenHoleCsvSchema(config, "outer", 3)

	innerHeader := innerSchema.BuildHeader()
	outerHeader := outerSchema.BuildHeader()

	dpInner := &SevenHoleDataPoint{
		PointID:     1,
		Coordinates: map[string]float64{"α": 1.0, "β": 2.0},
		Region:      "inner",
		Sector:      7,
		RawData:     SevenHoleRawData{P1: 1, P2: 2, P3: 3, P4: 4, P5: 5, P6: 6, P7: 7, PAtm: 101325.0, TAtm: 20.0},
	}
	dpOuter := &SevenHoleDataPoint{
		PointID:     2,
		Coordinates: map[string]float64{"φ": 60.0, "θ": 40.0},
		Region:      "outer",
		Sector:      3,
		RawData:     SevenHoleRawData{P1: 1, P2: 2, P3: 3, P4: 4, P5: 5, P6: 6, P7: 7, PAtm: 101325.0, TAtm: 20.0},
	}

	innerRecord := innerSchema.BuildRecord(dpInner)
	outerRecord := outerSchema.BuildRecord(dpOuter)

	if len(innerHeader) != len(innerRecord) {
		t.Fatalf("内区表头/数据行错位: header=%d, record=%d", len(innerHeader), len(innerRecord))
	}
	if len(outerHeader) != len(outerRecord) {
		t.Fatalf("外区表头/数据行错位: header=%d, record=%d", len(outerHeader), len(outerRecord))
	}
}
