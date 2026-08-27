package report

import (
	"testing"

	"cal1604/internal/domain"

	"github.com/xuri/excelize/v2"
)

// 模拟真实模板的单位结构：
//   - J5 "Unit" 标签右侧 K5 为字面量主单位格（6s 模板布局）
//   - D7、F9 为引用主格的公式格（通道块 Unit、结果分析表头 "（psi）"）
func buildUnitTemplateFixture() *excelize.File {
	f := excelize.NewFile()
	sheet := f.GetSheetName(0)
	f.SetCellValue(sheet, "J5", "Unit")
	f.SetCellValue(sheet, "K5", "psi")
	f.SetCellFormula(sheet, "D7", "$K$5")
	f.SetCellFormula(sheet, "F9", `"（"&$K$5&"）"`)
	return f
}

func TestApplyReportTemplateUnitWritesMasterCellOnly(t *testing.T) {
	f := buildUnitTemplateFixture()
	sheet := f.GetSheetName(0)

	applyReportTemplateUnit(f, "kPa")

	// 主单位格必须替换为设备真实单位
	if unit, _ := f.GetCellValue(sheet, "K5"); unit != "kPa" {
		t.Fatalf("expected master unit cell K5 = kPa, got %q", unit)
	}
	// 公式引用格必须保留公式，直接覆盖会破坏模板联动
	if formula, _ := f.GetCellFormula(sheet, "D7"); formula != "$K$5" {
		t.Fatalf("expected D7 formula preserved, got %q", formula)
	}
	if formula, _ := f.GetCellFormula(sheet, "F9"); formula != `"（"&$K$5&"）"` {
		t.Fatalf("expected F9 formula preserved, got %q", formula)
	}
	// 必须设置打开时重算，否则公式格仍显示模板缓存值 psi
	calc, err := f.GetCalcProps()
	if err != nil {
		t.Fatalf("GetCalcProps: %v", err)
	}
	if calc.FullCalcOnLoad == nil || !*calc.FullCalcOnLoad {
		t.Fatalf("expected FullCalcOnLoad=true, got %+v", calc)
	}
}

func TestFillMeasurementWorksheetMetadataOverwritesTemplateUnit(t *testing.T) {
	f := buildUnitTemplateFixture()
	sheet := f.GetSheetName(0)

	fillMeasurementWorksheetMetadata(f, "kPa", nil, domain.WorkflowConfig{})

	if unit, _ := f.GetCellValue(sheet, "K5"); unit != "kPa" {
		t.Fatalf("expected exported unit kPa, got %q", unit)
	}
	if formula, _ := f.GetCellFormula(sheet, "D7"); formula != "$K$5" {
		t.Fatalf("expected D7 formula preserved, got %q", formula)
	}
}

// 回程模板（6m）主单位格在 N5（第 13 列），验证扫描范围覆盖并正确替换。
func TestApplyReportTemplateUnitRoundTripTemplateLayout(t *testing.T) {
	f := excelize.NewFile()
	sheet := f.GetSheetName(0)
	f.SetCellValue(sheet, "M5", "Unit")
	f.SetCellValue(sheet, "N5", "psi")
	f.SetCellValue(sheet, "D7", "Unit")
	f.SetCellFormula(sheet, "E7", "$N$5")
	f.SetCellFormula(sheet, "I9", `"（"&$N$5&"）"`)

	applyReportTemplateUnit(f, "MPa")

	if unit, _ := f.GetCellValue(sheet, "N5"); unit != "MPa" {
		t.Fatalf("expected master unit cell N5 = MPa, got %q", unit)
	}
	if formula, _ := f.GetCellFormula(sheet, "E7"); formula != "$N$5" {
		t.Fatalf("expected E7 formula preserved, got %q", formula)
	}
	if formula, _ := f.GetCellFormula(sheet, "I9"); formula != `"（"&$N$5&"）"` {
		t.Fatalf("expected I9 formula preserved, got %q", formula)
	}
}
