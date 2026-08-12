package calibration

import (
	"reflect"
	"testing"
)

// seven_hole_export_schema_test.go — 七孔 18 列参考数据集格式导出 schema 测试
//
// 验证 SaveCsv 分区导出使用的列布局与基准数据集 W532.202608.P.7H.1-01 逐列对齐
// （spec §7.2/§7.3）：内区 α/β + Kα/Kβ/K0/Ks，外区 φ/θ + Kθ[n]/Kφ[n]/K0[n]/Ks[n]，
// 全部数值 3 位小数，GBK 编码（UseGBKEncoding=true）。

func makeSevenHoleExportTestPoint(region string, sector int) *SevenHoleDataPoint {
	pt, ps, ma := 4117.517, -30.133, 0.242
	dp := &SevenHoleDataPoint{
		PointID: 1,
		Region:  region,
		Sector:  sector,
		RawData: SevenHoleRawData{
			P1: 3260.217, P2: -874.900, P3: -2771.350, P4: -2918.583,
			P5: -1093.750, P6: 2973.950, P7: 2168.100,
			PAtm: 98884.0, TAtm: 28.0,
			PTotal: &pt, PStatic: &ps,
		},
		SampleCount: 5,
	}
	if region == "outer" {
		dp.Coordinates = map[string]float64{"θ": 30.0, "φ": 330.0}
		dp.Coefficients = SevenHoleCoefficients{
			Ktheta: 0.494, Kphi: 1.741, K0Outer: -0.207, KsOuter: -0.260,
			MachNumber: &ma,
		}
	} else {
		dp.Coordinates = map[string]float64{"α": -30.0, "β": -30.0}
		dp.Coefficients = SevenHoleCoefficients{
			Kalpha: -4.988, Kbeta: -6.688, K0: -0.897, Ks: 0.106,
			MachNumber: &ma,
		}
	}
	return dp
}

// TestSevenHoleExportHeader_Inner 验证内区 18 列表头（spec §7.2）
func TestSevenHoleExportHeader_Inner(t *testing.T) {
	schema := NewSevenHoleExportCsvSchema(Config{Type: string(TypeSevenHole)}, "inner", 0)
	want := []string{
		"侧滑角α", "迎角β", "马赫数Ma", "来流总压P0", "来流静压Ps",
		"P1", "P2", "P3", "P4", "P5", "P6", "P7",
		"α角度系数Kα", "β角度系数Kβ", "总压系数K0", "静压系数Ks",
		"大气压力", "大气温度",
	}
	if got := schema.BuildHeader(); !reflect.DeepEqual(got, want) {
		t.Errorf("内区导出表头不一致:\n got=%v\nwant=%v", got, want)
	}
}

// TestSevenHoleExportHeader_Outer 验证外区 18 列表头（spec §7.3）
//
// 外区前 2 列必须是"滚转角φ, 俯仰角θ"（不是侧滑角α/迎角β），
// 系数列必须带扇区编号 [n]（sector=3 → Kθ[3] 等）。
func TestSevenHoleExportHeader_Outer(t *testing.T) {
	schema := NewSevenHoleExportCsvSchema(Config{Type: string(TypeSevenHole)}, "outer", 3)
	want := []string{
		"滚转角φ", "俯仰角θ", "马赫数Ma", "来流总压P0", "来流静压Ps",
		"P1", "P2", "P3", "P4", "P5", "P6", "P7",
		"θ角度系数Kθ[3]", "φ角度系数Kφ[3]", "总压系数K0[3]", "静压系数Ks[3]",
		"大气压力", "大气温度",
	}
	if got := schema.BuildHeader(); !reflect.DeepEqual(got, want) {
		t.Errorf("外区导出表头不一致:\n got=%v\nwant=%v", got, want)
	}
}

// TestSevenHoleExportRecord_Inner 验证内区数据行：α/β 坐标 + 内区系数 + 3 位小数
func TestSevenHoleExportRecord_Inner(t *testing.T) {
	schema := NewSevenHoleExportCsvSchema(Config{Type: string(TypeSevenHole)}, "inner", 0)
	dp := makeSevenHoleExportTestPoint("inner", 7)
	want := []string{
		"-30.000", "-30.000", "0.242", "4117.517", "-30.133",
		"3260.217", "-874.900", "-2771.350", "-2918.583", "-1093.750", "2973.950", "2168.100",
		"-4.988", "-6.688", "-0.897", "0.106",
		"98884.000", "28.000",
	}
	if got := schema.BuildRecord(dp); !reflect.DeepEqual(got, want) {
		t.Errorf("内区导出数据行不一致:\n got=%v\nwant=%v", got, want)
	}
}

// TestSevenHoleExportRecord_Outer 验证外区数据行：φ/θ 坐标 + 外区系数 + 3 位小数
//
// 列位置契约（插值加载器）：col0=φ, col1=θ, col2=Ma, col3=P0, col4=Ps, col5..11=P1..P7
func TestSevenHoleExportRecord_Outer(t *testing.T) {
	schema := NewSevenHoleExportCsvSchema(Config{Type: string(TypeSevenHole)}, "outer", 1)
	dp := makeSevenHoleExportTestPoint("outer", 1)
	want := []string{
		"330.000", "30.000", "0.242", "4117.517", "-30.133",
		"3260.217", "-874.900", "-2771.350", "-2918.583", "-1093.750", "2973.950", "2168.100",
		"0.494", "1.741", "-0.207", "-0.260",
		"98884.000", "28.000",
	}
	if got := schema.BuildRecord(dp); !reflect.DeepEqual(got, want) {
		t.Errorf("外区导出数据行不一致:\n got=%v\nwant=%v", got, want)
	}
}

// TestSevenHoleExportSchema_UseGBKEncoding 验证导出 schema 要求 GBK 编码，
// 常规 26 列证书 schema 保持 UTF-8 BOM 现状
func TestSevenHoleExportSchema_UseGBKEncoding(t *testing.T) {
	exportSchema := NewSevenHoleExportCsvSchema(Config{Type: string(TypeSevenHole)}, "inner", 0)
	if !exportSchema.UseGBKEncoding() {
		t.Error("18 列导出 schema 应使用 GBK 编码")
	}
	certSchema := NewSevenHoleCsvSchema(Config{Type: string(TypeSevenHole)}, "inner", 0)
	if certSchema.UseGBKEncoding() {
		t.Error("26 列证书 schema 应保持 UTF-8 BOM 编码")
	}
}
