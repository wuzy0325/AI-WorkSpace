// verifyunit 端到端验证：用真实报告模板导出，检查单位主格替换、公式联动保留与打开重算标志。
// 用法: go run ./scripts/debug/verifyunit
package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"cal1604/internal/domain"
	"cal1604/internal/report"

	"github.com/xuri/excelize/v2"
)

func main() {
	dir, _ := os.MkdirTemp("", "verifyunit-*")
	defer os.RemoveAll(dir)

	svc := report.NewService("templates/reports")

	for _, mode := range []struct {
		name string
		cfg  domain.WorkflowConfig
	}{
		{"6s", domain.WorkflowConfig{PointCount: 6, PressureMode: domain.PressureModeSingle, PrecisionLevel: 0.001}},
		{"6m", domain.WorkflowConfig{PointCount: 6, PressureMode: domain.PressureModeRoundTrip, PrecisionLevel: 0.001}},
	} {
		points := make([]domain.PressurePoint, 0, 6)
		for i := 0; i < 6; i++ {
			p := domain.PressurePoint{
				Index: i + 1, TargetPressure: float64(i * 10), Direction: "forward",
				Status: "completed", CollectedData: []float64{float64(i*10) + 0.1},
			}
			points = append(points, p)
		}
		out := filepath.Join(dir, mode.name+".xlsx")
		if _, err := svc.ExportMeasurementReport(context.Background(), points, mode.cfg, out, "kPa"); err != nil {
			fmt.Println(mode.name, "export:", err)
			os.Exit(1)
		}
		check(out, mode.name)
	}
	fmt.Println("OK: 所有模板单位替换验证通过")
}

func check(path, name string) {
	f, err := excelize.OpenFile(path)
	if err != nil {
		fmt.Println(name, "open:", err)
		os.Exit(1)
	}
	defer f.Close()
	sheet := f.GetSheetList()[0]

	// 主单位格：6s→K5，6m→N5
	master := map[string]string{"6s": "K5", "6m": "N5"}[name]
	if v, _ := f.GetCellValue(sheet, master); v != "kPa" {
		fmt.Printf("%s: 主格 %s = %q, 期望 kPa\n", name, master, v)
		os.Exit(1)
	}

	// 公式引用格必须保留（结果分析表头 / 通道块 Unit）
	formulas := map[string]string{
		"6s": "F9",
		"6m": "I9",
	}
	if fm, _ := f.GetCellFormula(sheet, formulas[name]); fm == "" {
		fmt.Printf("%s: 公式格 %s 被破坏\n", name, formulas[name])
		os.Exit(1)
	}

	// 打开时必须强制重算
	calc, err := f.GetCalcProps()
	if err != nil || calc.FullCalcOnLoad == nil || !*calc.FullCalcOnLoad {
		fmt.Printf("%s: FullCalcOnLoad 未设置 (%v, %v)\n", name, calc, err)
		os.Exit(1)
	}
	fmt.Printf("%s: 主格 %s=kPa ✓  公式保留 ✓  FullCalcOnLoad ✓\n", name, master)
}
