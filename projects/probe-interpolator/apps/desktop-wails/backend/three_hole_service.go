package backend

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	three_interp "ai-workspace/shared/algorithms/go/threehole/interpolation"
)

// threeHoleHelpDocFileName 是 3 孔用户说明书文件名（与 docs/ 目录下文件名一致）。
// 命名带 "three-hole-" 前缀，避免与 5/7 孔说明书冲突。
const threeHoleHelpDocFileName = "three-hole-用户说明书.html"

// threeHoleState 封装 3 孔探针插值的运行时状态，自带 RWMutex。
// 与 5 孔 / 7 孔的 state 隔离，避免锁混用（SPEC § Boundaries 要求）。
type threeHoleState struct {
	mu          sync.RWMutex
	multiInterp *three_interp.ThreeHoleInterpolator
	prbFiles    []ThreeHolePrbFileInfo
}

// LoadThreeHolePrbFiles 弹出文件选择对话框让用户选择 3 孔 .prb 校准文件（通常单文件），
// 加载后构建 ThreeHoleInterpolator 并缓存到 threeHoleState。
// 返回加载结果（文件列表 + 马赫数范围 + 警告）供前端展示。
func (a *App) LoadThreeHolePrbFiles() ThreeHoleLoadPrbResponse {
	filePaths, err := a.openPrbFileDialog("选择 PRB 校准文件").PromptForMultipleSelection()
	if err != nil {
		return ThreeHoleLoadPrbResponse{Success: false, Error: err.Error()}
	}
	if len(filePaths) == 0 {
		return ThreeHoleLoadPrbResponse{Success: false, Error: "已取消选择"}
	}

	fileData := make([]three_interp.PrbFileData, 0, len(filePaths))
	for _, fp := range filePaths {
		lines, err := ReadPrbLines(fp)
		if err != nil {
			return ThreeHoleLoadPrbResponse{Success: false, Error: fmt.Sprintf("读取 %s 失败: %s", filepath.Base(fp), err.Error())}
		}
		fileData = append(fileData, three_interp.PrbFileData{FilePath: fp, Lines: lines})
	}

	interpolator := three_interp.NewThreeHoleInterpolator()
	result, err := interpolator.LoadPrbData(fileData)
	if err != nil {
		return ThreeHoleLoadPrbResponse{Success: false, Error: err.Error()}
	}

	prbFiles := make([]ThreeHolePrbFileInfo, 0, len(result.Files))
	for _, f := range result.Files {
		prbFiles = append(prbFiles, ThreeHolePrbFileInfo{
			FilePath:   f.FilePath,
			FileName:   f.FileName,
			MachNumber: f.MachNumber,
			ValidRange: ThreeHolePrbValidRange{
				AlphaMin: f.ValidRange.AlphaMin,
				AlphaMax: f.ValidRange.AlphaMax,
				MachMin:  f.ValidRange.MachMin,
				MachMax:  f.ValidRange.MachMax,
			},
		})
	}

	minMa, maxMa := interpolator.GetMachRange()
	machRange := []float64{minMa, maxMa}

	// 写操作加写锁，与读路径（CalculateThreeHole 等）互斥
	a.threeHole.mu.Lock()
	a.threeHole.multiInterp = interpolator
	a.threeHole.prbFiles = prbFiles
	a.threeHole.mu.Unlock()

	return ThreeHoleLoadPrbResponse{
		Success: true,
		Data: &ThreeHoleLoadPrbResult{
			Files:     prbFiles,
			MachRange: machRange,
			Warnings:  result.Warnings,
		},
	}
}

// IsThreeHolePrbLoaded 返回 3 孔 .prb 是否已加载。
func (a *App) IsThreeHolePrbLoaded() bool {
	a.threeHole.mu.RLock()
	defer a.threeHole.mu.RUnlock()
	return a.threeHole.multiInterp != nil && a.threeHole.multiInterp.IsLoaded()
}

// GetThreeHolePrbFiles 返回已加载的 3 孔 .prb 文件列表。
func (a *App) GetThreeHolePrbFiles() []ThreeHolePrbFileInfo {
	a.threeHole.mu.RLock()
	defer a.threeHole.mu.RUnlock()
	return a.threeHole.prbFiles
}

// GetThreeHoleMachRange 返回已加载 .prb 的马赫数覆盖范围 [min, max]。
func (a *App) GetThreeHoleMachRange() ThreeHoleMachRangeResponse {
	a.threeHole.mu.RLock()
	interpolator := a.threeHole.multiInterp
	a.threeHole.mu.RUnlock()

	if interpolator == nil || !interpolator.IsLoaded() {
		return ThreeHoleMachRangeResponse{Success: false, Error: "请先加载PRB文件"}
	}
	min, max := interpolator.GetMachRange()
	return ThreeHoleMachRangeResponse{Success: true, Data: []float64{min, max}}
}

// CalculateThreeHole 执行 3 孔单点插值计算。
func (a *App) CalculateThreeHole(input ThreeHoleInterpolationInput) ThreeHoleCalculateResponse {
	a.threeHole.mu.RLock()
	interpolator := a.threeHole.multiInterp
	a.threeHole.mu.RUnlock()

	if interpolator == nil || !interpolator.IsLoaded() {
		return ThreeHoleCalculateResponse{Success: false, Error: "请先加载PRB文件"}
	}

	coreInput := toThreeHoleCoreInput(input)
	result, err := interpolator.Calculate(coreInput)
	if err != nil {
		return ThreeHoleCalculateResponse{Success: false, Error: err.Error()}
	}

	return ThreeHoleCalculateResponse{Success: true, Data: toThreeHoleAppResult(result)}
}

// BatchCalculateThreeHole 批量计算，遇到错误不中断，标记失败行继续。
// 这样 CSV 中个别坏行不会让整个批次作废。
func (a *App) BatchCalculateThreeHole(datas []ThreeHoleInterpolationInput) ThreeHoleBatchCalculateResponse {
	a.threeHole.mu.RLock()
	interpolator := a.threeHole.multiInterp
	a.threeHole.mu.RUnlock()

	if interpolator == nil || !interpolator.IsLoaded() {
		return ThreeHoleBatchCalculateResponse{Success: false, Error: "请先加载PRB文件"}
	}

	results := make([]*ThreeHoleInterpolationResult, len(datas))
	var firstError string

	for i, input := range datas {
		coreInput := toThreeHoleCoreInput(input)
		result, err := interpolator.Calculate(coreInput)
		if err != nil {
			results[i] = &ThreeHoleInterpolationResult{
				IsValid: false,
				Warning: fmt.Sprintf("第%d行计算失败: %s", i+1, err.Error()),
			}
			if firstError == "" {
				firstError = fmt.Sprintf("第%d行计算失败: %s", i+1, err.Error())
			}
			continue
		}
		results[i] = toThreeHoleAppResult(result)
	}

	return ThreeHoleBatchCalculateResponse{
		Success: firstError == "",
		Error:   firstError,
		Data:    results,
	}
}

// toThreeHoleCoreInput 将应用层输入转换为核心算法输入。
// 核心算法内部统一使用表压，绝压输入时自动减去大气压转为表压。
func toThreeHoleCoreInput(input ThreeHoleInterpolationInput) three_interp.InterpolationInput {
	coreInput := three_interp.InterpolationInput{
		P1:   input.P1,
		P2:   input.P2,
		P3:   input.P3,
		PAtm: input.Patm,
		TAtm: input.Tatm,
	}

	if input.PressureMode == "absolute" {
		coreInput.P1 = input.P1 - input.Patm
		coreInput.P2 = input.P2 - input.Patm
		coreInput.P3 = input.P3 - input.Patm
	}

	return coreInput
}

// toThreeHoleAppResult 将核心算法结果转为应用层结果，字段一一映射。
func toThreeHoleAppResult(r three_interp.InterpolationResult) *ThreeHoleInterpolationResult {
	return &ThreeHoleInterpolationResult{
		Alpha:          r.Alpha,
		MachNumber:     r.MachNumber,
		TotalPressure:  r.TotalPressure,
		StaticPressure: r.StaticPressure,
		IterationCount: r.IterationCount,
		IsValid:        r.IsValid,
		Warning:        r.Warning,
	}
}

// ImportThreeHoleCsvData 弹出文件选择对话框让用户选 3 孔数据 CSV/TXT/DAT，
// 自动检测分隔符（Tab 或逗号），解析 P1 + P2 + P3 + Patm + Tatm 列。
// 与 5 孔不同：3 孔的 Patm/Tatm 列为必填（与旧 3 孔程序一致，保证向后兼容）。
func (a *App) ImportThreeHoleCsvData() ThreeHoleImportCsvDataResponse {
	// 3 孔数据文件支持 .csv/.txt/.dat（与旧 3 孔程序习惯一致），追加过滤器。
	filePath, err := a.openPrbFileDialog("选择数据文件").
		AddFilter("数据文件 (*.csv, *.txt, *.dat)", "*.csv;*.txt;*.dat").
		PromptForSingleSelection()
	if err != nil {
		return ThreeHoleImportCsvDataResponse{Success: false, Error: err.Error()}
	}
	if filePath == "" {
		return ThreeHoleImportCsvDataResponse{Success: false, Error: "已取消选择"}
	}

	records, err := readThreeHoleCsvFile(filePath)
	if err != nil {
		return ThreeHoleImportCsvDataResponse{Success: false, Error: err.Error()}
	}

	colMap, csvErr := parseThreeHoleCsvHeader(records[0])
	if csvErr != "" {
		return ThreeHoleImportCsvDataResponse{Success: false, Error: csvErr}
	}

	datas, warnings := parseThreeHoleCsvRows(records[1:], colMap)
	if len(warnings) > 0 {
		log.Printf("3 孔 CSV 导入警告: %s", strings.Join(warnings, "; "))
	}

	if len(datas) == 0 {
		errMsg := "所有数据行解析失败"
		if len(warnings) > 0 {
			errMsg = fmt.Sprintf("所有数据行解析失败: %s", strings.Join(warnings, "; "))
		}
		return ThreeHoleImportCsvDataResponse{Success: false, Error: errMsg}
	}

	return ThreeHoleImportCsvDataResponse{Success: true, Data: datas}
}

// readThreeHoleCsvFile 读取 3 孔数据文件，跳过空行与 # 注释行，
// 自动检测 Tab / 逗号分隔符并切分为记录数组。
func readThreeHoleCsvFile(filePath string) ([][]string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("打开文件失败: %s", err.Error())
	}
	defer file.Close()

	// 剥离 UTF-8 BOM：Excel "另存为 CSV UTF-8" 会在首字节加 BOM，
	// 导致首列名变成 "\uFEFFP1"，colMap["P1"] 查找失败。
	br := bufio.NewReader(file)
	StripUTF8BOM(br)

	scanner := bufio.NewScanner(br)
	var dataLines []string
	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		dataLines = append(dataLines, trimmed)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("读取文件失败: %s", err.Error())
	}

	if len(dataLines) < 2 {
		return nil, fmt.Errorf("文件为空或只有表头")
	}

	delimiter := DetectDelimiter(dataLines[0])

	records := make([][]string, 0, len(dataLines))
	for _, line := range dataLines {
		fields := strings.Split(line, string(delimiter))
		for i := range fields {
			fields[i] = strings.TrimSpace(fields[i])
		}
		records = append(records, fields)
	}

	return records, nil
}

// parseThreeHoleCsvHeader 校验表头必须包含 P1/P2/P3/Patm/Tatm 五列，
// 返回列名 → 列索引映射供 parseThreeHoleCsvRows 使用。
// 检测重复列名：避免 colMap 后写覆盖导致 parseThreeHoleCsvRows 越界 panic。
func parseThreeHoleCsvHeader(header []string) (map[string]int, string) {
	colMap := make(map[string]int)
	for i, col := range header {
		name := strings.TrimSpace(col)
		if _, exists := colMap[name]; exists {
			return nil, fmt.Sprintf("表头存在重复列名: %s", name)
		}
		colMap[name] = i
	}

	requiredCols := []string{"P1", "P2", "P3", "Patm", "Tatm"}
	for _, name := range requiredCols {
		if _, ok := colMap[name]; !ok {
			return nil, fmt.Sprintf("缺少必要列: %s", name)
		}
	}

	return colMap, ""
}

// parseThreeHoleCsvRows 解析数据行，跳过列数不足或字段解析失败的行并收集警告。
// 与 5 孔不同：3 孔的 Patm/Tatm 是必填字段，缺失则跳过该行（与旧 3 孔程序一致）。
func parseThreeHoleCsvRows(rows [][]string, colMap map[string]int) ([]ThreeHoleInterpolationInput, []string) {
	colCount := len(colMap)
	patmIdx := colMap["Patm"]
	tatmIdx := colMap["Tatm"]

	datas := make([]ThreeHoleInterpolationInput, 0, len(rows))
	var warnings []string

	for rowIdx := 0; rowIdx < len(rows); rowIdx++ {
		row := rows[rowIdx]
		csvLine := rowIdx + 2

		if len(row) < colCount {
			warnings = append(warnings, fmt.Sprintf("第%d行列数不足，已跳过", csvLine))
			continue
		}

		// parseField 返回错误而非静默返回 0，便于定位坏行
		parseField := func(colIdx int, fieldName string) (float64, error) {
			val, err := strconv.ParseFloat(strings.TrimSpace(row[colIdx]), 64)
			if err != nil {
				return 0, fmt.Errorf("第%d行%s解析失败: %q", csvLine, fieldName, row[colIdx])
			}
			return val, nil
		}

		p1, err1 := parseField(colMap["P1"], "P1")
		p2, err2 := parseField(colMap["P2"], "P2")
		p3, err3 := parseField(colMap["P3"], "P3")

		if err1 != nil || err2 != nil || err3 != nil {
			for _, e := range []error{err1, err2, err3} {
				if e != nil {
					warnings = append(warnings, e.Error())
				}
			}
			continue
		}

		patm, errPatm := parseField(patmIdx, "Patm")
		if errPatm != nil {
			warnings = append(warnings, errPatm.Error())
			continue
		}

		tatm, errTatm := parseField(tatmIdx, "Tatm")
		if errTatm != nil {
			warnings = append(warnings, errTatm.Error())
			continue
		}

		input := ThreeHoleInterpolationInput{
			P1:   p1,
			P2:   p2,
			P3:   p3,
			Patm: patm,
			Tatm: tatm,
		}

		datas = append(datas, input)
	}

	return datas, warnings
}

// OpenThreeHoleHelpDoc 用系统默认程序打开 3 孔用户说明书 HTML 文件。
// 路径解析与跨平台打开逻辑抽到共享工具 GetHelpDocPath / OpenHelpDocByPath。
func (a *App) OpenThreeHoleHelpDoc() error {
	return OpenHelpDocByPath(GetHelpDocPath(threeHoleHelpDocFileName))
}
