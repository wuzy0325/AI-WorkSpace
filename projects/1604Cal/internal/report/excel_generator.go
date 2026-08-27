package report

import (
	"fmt"
	"math"
	"os"
	"strconv"
	"strings"

	"cal1604/internal/domain"

	"github.com/xuri/excelize/v2"
)

// ChannelBlock 表示模板中一个通道数据块的定位信息。
type ChannelBlock struct {
	Sheet     string
	HeaderRow int
	DataStart int
	DataEnd   int
}

// LoadTemplate 加载 Excel 模板文件并返回文件对象。
func LoadTemplate(path string) (*excelize.File, error) {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil, fmt.Errorf("template file not found: %s", path)
	}
	return excelize.OpenFile(path)
}

// FindChannelBlocks 扫描模板所有工作表的 A 列，查找包含"通道"或"Channel"关键字的单元格，
// 将连续的数据行合并为一个 ChannelBlock。
// DataStart 定位到第一个数值数据行（A 列可解析为 float64），
// DataEnd 定位到最后一个数值数据行之后的下一个空行。
func FindChannelBlocks(f *excelize.File) ([]ChannelBlock, error) {
	var blocks []ChannelBlock

	for _, sheet := range f.GetSheetList() {
		rows, err := f.GetCols(sheet)
		if err != nil || len(rows) == 0 {
			continue
		}

		colA := rows[0]
		var currentBlock *ChannelBlock

		for rowIdx, cell := range colA {
			rowNum := rowIdx + 1
			text := strings.TrimSpace(cell)

			if strings.Contains(text, "通道") || strings.Contains(text, "Channel") {
				if currentBlock != nil {
					blocks = append(blocks, *currentBlock)
				}
				currentBlock = &ChannelBlock{
					Sheet:     sheet,
					HeaderRow: rowNum,
				}
			} else if currentBlock != nil && currentBlock.DataStart == 0 {
				// 还未定位到数据起始行：跳过表头描述行，找到第一个数值行
				if isNumericRow(text) {
					currentBlock.DataStart = rowNum
				}
			} else if currentBlock != nil && text == "" {
				// 空行：如果已定位到数据起始行，正常终止 block
				if currentBlock.DataStart > 0 {
					currentBlock.DataEnd = rowNum - 1
				} else {
					// 模板异常：header 后未找到数值行，回退 DataStart
					currentBlock.DataStart = currentBlock.HeaderRow + 1
					currentBlock.DataEnd = currentBlock.DataStart
				}
				blocks = append(blocks, *currentBlock)
				currentBlock = nil
			}
		}

		if currentBlock != nil {
			if currentBlock.DataStart == 0 {
				// 未找到数值行时回退到 HeaderRow+1，至少保留一处可写位置
				currentBlock.DataStart = currentBlock.HeaderRow + 1
			}
			if currentBlock.DataEnd == 0 {
				currentBlock.DataEnd = len(colA)
			}
			blocks = append(blocks, *currentBlock)
		}
	}

	return blocks, nil
}

// isNumericRow 判断 A 列文本是否为数值（含正负号、小数点、科学计数法）。
func isNumericRow(text string) bool {
	if text == "" {
		return false
	}
	// 用 strconv 判断是否可解析为浮点数
	_, err := strconv.ParseFloat(text, 64)
	return err == nil
}

// FillStandardValues 将标准压力值填入指定通道块的指定列。
func FillStandardValues(f *excelize.File, block ChannelBlock, col string, standardValues []float64, unit string) error {
	axis := fmt.Sprintf("%s%d", col, block.HeaderRow)
	if err := f.SetCellValue(block.Sheet, axis, fmt.Sprintf("标准值(%s)", unit)); err != nil {
		return err
	}

	for i, val := range standardValues {
		cell := fmt.Sprintf("%s%d", col, block.DataStart+i)
		rounded := math.Round(val*100) / 100
		if err := f.SetCellValue(block.Sheet, cell, rounded); err != nil {
			return err
		}
	}
	return nil
}

// FillMeasureData 将采集到的测量数据填入指定通道块的指定列。
func FillMeasureData(f *excelize.File, block ChannelBlock, col string, header string, data []float64) error {
	axis := fmt.Sprintf("%s%d", col, block.HeaderRow)
	if err := f.SetCellValue(block.Sheet, axis, header); err != nil {
		return err
	}

	for i, val := range data {
		cell := fmt.Sprintf("%s%d", col, block.DataStart+i)
		rounded := math.Round(val*1e6) / 1e6
		if err := f.SetCellValue(block.Sheet, cell, rounded); err != nil {
			return err
		}
	}
	return nil
}

// FillRoundTripData 将回程测量数据填入指定列（正程+回程）。
// 不限制 DataEnd——回程模板中 D 列有 2x 行空间容纳正程+回程数据。
func FillRoundTripData(f *excelize.File, block ChannelBlock, col string, forwardData, backwardData []float64) error {
	axis := fmt.Sprintf("%s%d", col, block.HeaderRow)
	if err := f.SetCellValue(block.Sheet, axis, "回程测量值"); err != nil {
		return err
	}

	allData := append(forwardData, backwardData...)
	for i, val := range allData {
		cell := fmt.Sprintf("%s%d", col, block.DataStart+i)
		rounded := math.Round(val*1e6) / 1e6
		if err := f.SetCellValue(block.Sheet, cell, rounded); err != nil {
			return err
		}
	}
	return nil
}

// CreateFallbackWorkbook 当无模板文件时创建默认工作簿。
func CreateFallbackWorkbook(points []float64, channels [][]float64, unit string) *excelize.File {
	f := excelize.NewFile()
	sheet := "校准数据"
	f.SetSheetName("Sheet1", sheet)

	f.SetCellValue(sheet, "A1", "序号")
	f.SetCellValue(sheet, "B1", fmt.Sprintf("标准值(%s)", unit))

	for chIdx := range channels {
		col := fmt.Sprintf("%c", 'C'+chIdx)
		f.SetCellValue(sheet, fmt.Sprintf("%s1", col), fmt.Sprintf("通道%d", chIdx+1))
	}

	for i, p := range points {
		row := i + 2
		f.SetCellValue(sheet, fmt.Sprintf("A%d", row), i+1)
		f.SetCellValue(sheet, fmt.Sprintf("B%d", row), math.Round(p*100)/100)
		for chIdx, chData := range channels {
			if i < len(chData) {
				col := fmt.Sprintf("%c", 'C'+chIdx)
				f.SetCellValue(sheet, fmt.Sprintf("%s%d", col, row), math.Round(chData[i]*1e6)/1e6)
			}
		}
	}

	return f
}

// ResolveUnit 按优先级解析压力单位：deviceUnit > cachedUnit > dataUnit > defaultUnit。
func ResolveUnit(deviceUnit, cachedUnit, dataUnit, defaultUnit string) string {
	if deviceUnit != "" {
		return deviceUnit
	}
	if cachedUnit != "" {
		return cachedUnit
	}
	if dataUnit != "" {
		return dataUnit
	}
	if defaultUnit != "" {
		return defaultUnit
	}
	return "kPa"
}

// CreateMeasurementFallbackWorkbook 创建计量报告默认工作簿（无模板时使用）。
// 版式参考 1604V2 模板：多通道块、中英文标题、边框、公式。
// forwardChannels 为每通道的正程显示值；backwardByTarget 仅在回程模式下使用，
// 按 (通道, 标准压力) 索引匹配，可正确处理回程点缺失场景；为 nil 表示单程模式。
func CreateMeasurementFallbackWorkbook(standardValues []float64, forwardChannels [][]float64, backwardByTarget []map[float64]float64, unit string, points []domain.PressurePoint, config domain.WorkflowConfig) *excelize.File {
	f := excelize.NewFile()
	sheet := "校准结果"
	f.SetSheetName("Sheet1", sheet)

	numChannels := len(forwardChannels)
	if numChannels == 0 {
		numChannels = 1
	}
	basePointCount := len(standardValues)
	dataRowCount := basePointCount
	if dataRowCount < 6 {
		dataRowCount = 6
	}
	isRoundTrip := config.PressureMode == domain.PressureModeRoundTrip

	// 列宽
	widths := []float64{21.125, 13, 13, 13, 4, 14.375, 13.625, 13.125, 13.25, 14.25, 13.5, 7.625}
	for idx, w := range widths {
		f.SetColWidth(sheet, colName(idx+1), colName(idx+1), w)
	}

	// 顶部结构
	f.MergeCell(sheet, "I1", "I2")
	f.MergeCell(sheet, "A3", "D3")
	f.MergeCell(sheet, "A4", "D4")
	f.MergeCell(sheet, "F3", "J3")
	f.MergeCell(sheet, "F4", "J4")

	f.SetRowHeight(sheet, 1, 26.45)
	f.SetRowHeight(sheet, 2, 15)
	f.SetRowHeight(sheet, 3, 23.1)
	f.SetRowHeight(sheet, 4, 11.1)
	f.SetRowHeight(sheet, 5, 39.75)
	f.SetRowHeight(sheet, 6, 15)

	f.SetCellValue(sheet, "C1", "证书号：")
	f.SetCellValue(sheet, "C2", "Certificate No.")
	f.SetCellValue(sheet, "A3", "校准结果")
	f.SetCellValue(sheet, "A4", "Results of Calibration")

	f.SetCellValue(sheet, "F3", "结果分析")
	f.SetCellValue(sheet, "F4", "Results analysis")
	f.SetCellValue(sheet, "F5", "设备编号：")
	f.SetCellValue(sheet, "H5", "校准日期：")
	f.SetCellValue(sheet, "J5", "校准单位：")
	f.SetCellValue(sheet, "K5", unit)
	f.SetCellValue(sheet, "F6", "准确度等级：")
	f.SetCellValue(sheet, "G6", fmt.Sprintf("%.2f", config.PrecisionLevel*100))
	f.SetCellValue(sheet, "H6", "Min 量程：")
	f.SetCellValue(sheet, "J6", "Max 量程：")

	if len(standardValues) > 0 {
		minVal, maxVal := standardValues[0], standardValues[0]
		for _, v := range standardValues {
			if v < minVal {
				minVal = v
			}
			if v > maxVal {
				maxVal = v
			}
		}
		f.SetCellValue(sheet, "I6", minVal)
		f.SetCellValue(sheet, "K6", maxVal)
	}

	// 通道块
	blockStep := 1 + 2 + dataRowCount + 1 // 标题 + 表头(2) + 数据 + 间隔

	for chIdx := 0; chIdx < numChannels; chIdx++ {
		blockStart := 7 + chIdx*blockStep
		headerRow := blockStart + 1
		dataStartRow := blockStart + 3
		dataEndRow := dataStartRow + dataRowCount - 1

		f.SetRowHeight(sheet, blockStart, 32.25)

		f.SetCellValue(sheet, cellName(1, blockStart), "通道编号：")
		f.SetCellValue(sheet, cellName(2, blockStart), fmt.Sprintf("CH%d", chIdx+1))
		f.SetCellValue(sheet, cellName(3, blockStart), "单位：")
		f.SetCellFormula(sheet, cellName(4, blockStart), "$K$5")
		f.SetCellFormula(sheet, cellName(7, blockStart), fmt.Sprintf("B%d", blockStart))

		// 左侧表头
		f.MergeCell(sheet, cellName(1, headerRow), cellName(1, headerRow+1))
		f.SetCellValue(sheet, cellName(1, headerRow), "标准压力值")

		f.MergeCell(sheet, cellName(2, headerRow), cellName(2, headerRow+1))
		f.SetCellValue(sheet, cellName(2, headerRow), "被校设备显示值")

		if isRoundTrip {
			f.MergeCell(sheet, cellName(3, headerRow), cellName(3, headerRow+1))
			f.SetCellValue(sheet, cellName(3, headerRow), "回程值")
		} else {
			f.MergeCell(sheet, cellName(3, headerRow), cellName(3, headerRow+1))
			f.SetCellValue(sheet, cellName(3, headerRow), "示值误差")
		}

		f.MergeCell(sheet, cellName(4, headerRow), cellName(4, headerRow+1))
		f.SetCellValue(sheet, cellName(4, headerRow), "U（%FS）\n（k=2）")

		// 右侧表头
		f.SetCellValue(sheet, cellName(6, headerRow), "标准压力")
		f.SetCellValue(sheet, cellName(7, headerRow), "丨Indication error丨\n/FS")
		f.SetCellValue(sheet, cellName(8, headerRow), "max")
		f.SetCellValue(sheet, cellName(9, headerRow), "Notes")
		f.SetCellValue(sheet, cellName(10, headerRow), "Notes")
		f.SetCellValue(sheet, cellName(11, headerRow), "Notes")

		f.SetCellFormula(sheet, cellName(6, headerRow+1), fmt.Sprintf(`"（"&$K$5&"）"`))
		f.SetCellValue(sheet, cellName(7, headerRow+1), "丨Indication error丨\n/FS")
		f.SetCellValue(sheet, cellName(8, headerRow+1), "max")
		f.SetCellValue(sheet, cellName(9, headerRow+1), "Notes")
		f.SetCellValue(sheet, cellName(10, headerRow+1), "Notes")
		f.SetCellValue(sheet, cellName(11, headerRow+1), "Notes")

		// 合并右侧表头
		f.MergeCell(sheet, cellName(7, headerRow), cellName(7, headerRow+1))
		f.MergeCell(sheet, cellName(8, headerRow), cellName(8, headerRow+1))
		f.MergeCell(sheet, cellName(9, headerRow), cellName(11, headerRow+1))

		rightPanelStart := dataStartRow
		rightPanelEnd := rightPanelStart + 5
		f.MergeCell(sheet, cellName(8, rightPanelStart), cellName(8, rightPanelEnd))
		f.MergeCell(sheet, cellName(9, rightPanelStart), cellName(11, rightPanelEnd))

		f.SetCellFormula(sheet, cellName(8, dataStartRow), fmt.Sprintf("MAX(G%d:G%d)", dataStartRow, dataEndRow))

		// 数据行填充
		for row := dataStartRow; row <= dataEndRow; row++ {
			rowOffset := row - dataStartRow

			for col := 1; col <= 8; col++ {
				// 第 3 列：示值误差（单程模式用公式）
				if col == 3 && !isRoundTrip {
					f.SetCellFormula(sheet, cellName(col, row), fmt.Sprintf("B%d-A%d", row, row))
				}
				// 第 4 列：不确定度
				if col == 4 {
					f.SetCellValue(sheet, cellName(col, row), 0.013)
				}
				// 第 6 列：标准压力引用
				if col == 6 {
					f.SetCellFormula(sheet, cellName(col, row), fmt.Sprintf("A%d", row))
				}
				// 第 7 列：示值误差/FS
				if col == 7 {
					f.SetCellFormula(sheet, cellName(col, row), fmt.Sprintf("ABS(C%d)/($K$6-$I$6)", row))
				}

				// 对齐
				hAlign := "center"
				if col <= 4 {
					hAlign = "right"
				}
				cell := cellName(col, row)
				style, _ := f.NewStyle(&excelize.Style{
					Alignment: &excelize.Alignment{Horizontal: hAlign, Vertical: "middle", WrapText: true},
				})
				f.SetCellStyle(sheet, cell, cell, style)
			}

			// 填充标准压力值（第 1 列）和测量值（第 2 列）
			if rowOffset < basePointCount {
				stdVal := standardValues[rowOffset]
				f.SetCellValue(sheet, cellName(1, row), math.Round(stdVal*100)/100)
				if chIdx < len(forwardChannels) && rowOffset < len(forwardChannels[chIdx]) {
					f.SetCellValue(sheet, cellName(2, row), math.Round(forwardChannels[chIdx][rowOffset]*1e6)/1e6)
				}
				// 回程模式：第 3 列按标准压力精确匹配回程显示值。
				if isRoundTrip && chIdx < len(backwardByTarget) {
					if val, ok := backwardByTarget[chIdx][stdVal]; ok {
						f.SetCellValue(sheet, cellName(3, row), math.Round(val*1e6)/1e6)
					}
				}
			}
		}

		// 左侧边框
		thinStyle, _ := f.NewStyle(&excelize.Style{
			Border: []excelize.Border{
				{Type: "left", Color: "000000", Style: 1},
				{Type: "right", Color: "000000", Style: 1},
				{Type: "top", Color: "000000", Style: 1},
				{Type: "bottom", Color: "000000", Style: 1},
			},
			Alignment: &excelize.Alignment{Horizontal: "right", Vertical: "middle"},
		})
		headerStyle, _ := f.NewStyle(&excelize.Style{
			Border: []excelize.Border{
				{Type: "left", Color: "000000", Style: 1},
				{Type: "right", Color: "000000", Style: 1},
				{Type: "top", Color: "000000", Style: 1},
				{Type: "bottom", Color: "000000", Style: 1},
			},
			Font:      &excelize.Font{Bold: true},
			Alignment: &excelize.Alignment{Horizontal: "center", Vertical: "middle", WrapText: true},
		})
		rightStyle, _ := f.NewStyle(&excelize.Style{
			Border: []excelize.Border{
				{Type: "left", Color: "000000", Style: 1},
				{Type: "right", Color: "000000", Style: 1},
				{Type: "top", Color: "000000", Style: 1},
				{Type: "bottom", Color: "000000", Style: 1},
			},
			Alignment: &excelize.Alignment{Horizontal: "center", Vertical: "middle", WrapText: true},
		})

		for row := blockStart; row <= dataEndRow; row++ {
			for col := 1; col <= 4; col++ {
				cell := cellName(col, row)
				if row <= headerRow+1 {
					f.SetCellStyle(sheet, cell, cell, headerStyle)
				} else {
					f.SetCellStyle(sheet, cell, cell, thinStyle)
				}
			}
		}
		for row := headerRow; row <= rightPanelEnd; row++ {
			for col := 6; col <= 11; col++ {
				cell := cellName(col, row)
				f.SetCellStyle(sheet, cell, cell, rightStyle)
			}
		}
	}

	return f
}

// colName 返回 Excel 列字母（1-based：1→A, 2→B, ...）。
func colName(col int) string {
	if col <= 26 {
		return string(rune('A' - 1 + col))
	}
	return string(rune('A'-1+col/26)) + string(rune('A'-1+col%26))
}

// cellName 返回 Excel 单元格名称（如 A1, B2）。
func cellName(col, row int) string {
	return fmt.Sprintf("%s%d", colName(col), row)
}
