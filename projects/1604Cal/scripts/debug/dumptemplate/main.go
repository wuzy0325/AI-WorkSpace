// dumptemplate 打印报告模板中与单位相关的单元格，用于排查导出单位问题。
// 用法: go run ./scripts/debug/dumptemplate [模板路径]
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/xuri/excelize/v2"
)

func main() {
	dir := "templates/reports"
	if len(os.Args) > 1 {
		dir = os.Args[1]
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		fmt.Println("read dir:", err)
		os.Exit(1)
	}
	for _, e := range entries {
		if e.IsDir() || !strings.EqualFold(filepath.Ext(e.Name()), ".xlsx") {
			continue
		}
		if e.Name() != "6s.xlsx" && e.Name() != "6m.xlsx" {
			continue
		}
		dump(filepath.Join(dir, e.Name()))
	}
}

func dump(path string) {
	fmt.Printf("===== %s =====\n", path)
	f, err := excelize.OpenFile(path)
	if err != nil {
		fmt.Println("open:", err)
		return
	}
	defer f.Close()

	for _, sheet := range f.GetSheetList() {
		fmt.Printf("--- sheet %q ---\n", sheet)
		rows, err := f.GetRows(sheet)
		if err != nil {
			fmt.Println("get rows:", err)
			continue
		}
		for r, row := range rows {
			for c, text := range row {
				text = strings.TrimSpace(text)
				if text == "" {
					continue
				}
				lower := strings.ToLower(text)
				// 打印所有含单位痕迹的单元格，以及前 8 行全部内容（元数据区）
				if strings.Contains(text, "单位") || strings.Contains(lower, "unit") ||
					strings.Contains(lower, "psi") || strings.Contains(text, "结果分析") || r < 8 {
					cell, _ := excelize.CoordinatesToCellName(c+1, r+1)
					formula, ferr := f.GetCellFormula(sheet, cell)
					if ferr == nil && formula != "" {
						fmt.Printf("  %s: %q  [公式: %s]\n", cell, text, formula)
					} else {
						fmt.Printf("  %s: %q\n", cell, text)
					}
				}
			}
		}
	}
}
