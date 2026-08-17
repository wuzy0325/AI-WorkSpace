package storage

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"windlabx4/services/api-go/internal/core/calibration"

	"golang.org/x/text/encoding/simplifiedchinese"
)

// seven_hole_export_writer_test.go — 七孔 18 列参考数据集格式导出的 writer 集成测试
//
// 验证 NewWriterTruncate + NewSevenHoleExportCsvSchema 组合：
//   - 文件以 GBK 编码落盘（无 UTF-8 BOM），与基准数据集 W532.202608.P.7H.1-01 一致
//   - 覆盖模式：重复导出到同一路径不产生重复数据
//   - 表头/数据行内容与 18 列位置契约一致（插值加载器按列位置解析）

func makeSevenHoleExportWriterTestPoint() *calibration.SevenHoleDataPoint {
	pt, ps, ma := 4117.517, -30.133, 0.242
	return &calibration.SevenHoleDataPoint{
		PointID:     1,
		Region:      "outer",
		Sector:      1,
		Coordinates: map[string]float64{"θ": 30.0, "φ": 330.0},
		RawData: calibration.SevenHoleRawData{
			P1: 3260.217, P2: -874.900, P3: -2771.350, P4: -2918.583,
			P5: -1093.750, P6: 2973.950, P7: 2168.100,
			PAtm: 98884.0, TAtm: 28.0,
			PTotal: &pt, PStatic: &ps,
		},
		Coefficients: calibration.SevenHoleCoefficients{
			Ktheta: 0.494, Kphi: 1.741, K0Outer: -0.207, KsOuter: -0.260,
			MachNumber: &ma,
		},
		SampleCount: 5,
	}
}

// TestSevenHoleExportWriter_GBKRoundTrip 验证导出文件 GBK 编码与内容往返
func TestSevenHoleExportWriter_GBKRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "W532(大角度1区).csv")

	factory := &CalibrationCsvWriter{}
	schema := calibration.NewSevenHoleExportCsvSchema(
		calibration.Config{Type: string(calibration.TypeSevenHole)}, "outer", 1)
	writer, err := factory.NewWriterTruncate(path, schema)
	if err != nil {
		t.Fatalf("NewWriterTruncate: %v", err)
	}
	if err := writer.AppendPoint(makeSevenHoleExportWriterTestPoint()); err != nil {
		t.Fatalf("AppendPoint: %v", err)
	}
	if err := writer.Flush(); err != nil {
		t.Fatalf("Flush: %v", err)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("读取导出文件: %v", err)
	}
	// 无 UTF-8 BOM（GBK 文件，与参考数据集一致）
	if strings.HasPrefix(string(raw), "\ufeff") {
		t.Error("GBK 导出文件不应包含 UTF-8 BOM")
	}
	// GB18030 解码（插值加载适配层的解码方式）后内容可读
	decoded, err := simplifiedchinese.GB18030.NewDecoder().Bytes(raw)
	if err != nil {
		t.Fatalf("GB18030 解码失败: %v", err)
	}
	lines := strings.Split(strings.TrimRight(string(decoded), "\r\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("应为表头+1 数据行, 实际 %d 行: %v", len(lines), lines)
	}
	if !strings.HasPrefix(lines[0], "滚转角φ,俯仰角θ,马赫数Ma,") {
		t.Errorf("外区表头前 3 列应为 滚转角φ/俯仰角θ/马赫数Ma, 实际: %s", lines[0])
	}
	if !strings.Contains(lines[0], "θ角度系数Kθ[1]") {
		t.Errorf("外区表头系数列应带扇区编号 [1], 实际: %s", lines[0])
	}
	if !strings.HasPrefix(lines[1], "330.000,30.000,0.242,") {
		t.Errorf("数据行前 3 列应为 φ/θ/Ma=330.000/30.000/0.242, 实际: %s", lines[1])
	}
}

// TestSevenHoleExportWriter_TruncateOnReexport 验证覆盖模式：重复导出到同一路径不追加
func TestSevenHoleExportWriter_TruncateOnReexport(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "W532(小角度区).csv")

	schema := calibration.NewSevenHoleExportCsvSchema(
		calibration.Config{Type: string(calibration.TypeSevenHole)}, "inner", 0)
	factory := &CalibrationCsvWriter{}

	for round := 1; round <= 2; round++ {
		writer, err := factory.NewWriterTruncate(path, schema)
		if err != nil {
			t.Fatalf("第 %d 轮 NewWriterTruncate: %v", round, err)
		}
		if err := writer.Flush(); err != nil {
			t.Fatalf("第 %d 轮 Flush: %v", round, err)
		}
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("读取导出文件: %v", err)
	}
	decoded, err := simplifiedchinese.GB18030.NewDecoder().Bytes(raw)
	if err != nil {
		t.Fatalf("GB18030 解码失败: %v", err)
	}
	lines := strings.Split(strings.TrimRight(string(decoded), "\r\n"), "\n")
	if len(lines) != 1 {
		t.Fatalf("覆盖模式重复导出后应只有 1 行表头, 实际 %d 行", len(lines))
	}
	if !strings.HasPrefix(lines[0], "侧滑角α,迎角β,马赫数Ma,") {
		t.Errorf("内区表头前 3 列应为 侧滑角α/迎角β/马赫数Ma, 实际: %s", lines[0])
	}
}
