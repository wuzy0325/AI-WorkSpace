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

	seven_interp "ai-workspace/shared/algorithms/go/sevenhole/interpolation"
)

// sevenHoleHelpDocFileName 是 7 孔用户说明书文件名（与 docs/ 目录下文件名一致）。
// 命名带 "seven-hole-" 前缀，避免与 5/3 孔说明书冲突。
const sevenHoleHelpDocFileName = "seven-hole-用户说明书.html"

// sevenHoleState 封装 7 孔探针插值的运行时状态，自带 RWMutex。
// 与 5 孔 / 3 孔的 state 隔离，避免锁混用（SPEC § Boundaries 要求）。
type sevenHoleState struct {
	mu           sync.RWMutex
	interpolator *seven_interp.SevenHolePrbInterpolator
	prbFiles     []SevenHolePrbFileInfo
}

// LoadSevenHolePrbFiles 弹出多选文件对话框，让用户选择 7 个 .prb 校准文件
// （1.prb..7.prb），加载后构建 SevenHolePrbInterpolator 并缓存到 sevenHoleState。
//
// 文件名约定（与 shared/algorithms/go/sevenhole/interpolation/testdata/prb/ 一致）：
//   - "7.prb" → 内区网格（13×13=169 点，a/b ∈ [-30,30] 步长 5）
//   - "1.prb".."6.prb" → 外区扇区 1..6（每个 4×13=52 点，θ ∈ [30,45] 步长 5）
//
// 文件名 basename（不含扩展名）必须为 "1".."7" 之一的纯数字字符串；
// 不满足约定时返回错误，提示用户按规范命名。
func (a *App) LoadSevenHolePrbFiles() SevenHoleLoadPrbResponse {
	filePaths, err := a.openPrbFileDialog("选择 7 个 PRB 校准文件（1.prb..7.prb）").PromptForMultipleSelection()
	if err != nil {
		return SevenHoleLoadPrbResponse{Success: false, Error: err.Error()}
	}
	if len(filePaths) == 0 {
		return SevenHoleLoadPrbResponse{Success: false, Error: "已取消选择"}
	}

	// 按文件名解析 sector：basename（不含扩展名）必须为 "1".."7"。
	// 内区文件（7.prb）单独记录路径；外区 1..6 按扇区编号填入 outerPaths 数组。
	var innerPath string
	var outerPaths [6]string
	outerSeen := [6]bool{}
	var warnings []string

	for _, fp := range filePaths {
		base := strings.TrimSuffix(filepath.Base(fp), filepath.Ext(fp))
		n, err := strconv.Atoi(base)
		if err != nil || n < 1 || n > 7 {
			warnings = append(warnings, fmt.Sprintf("跳过非约定文件名: %s（期望 1.prb..7.prb）", filepath.Base(fp)))
			continue
		}
		if n == 7 {
			if innerPath != "" {
				return SevenHoleLoadPrbResponse{
					Success: false,
					Error:   "检测到多个 7.prb 内区文件，请只保留一个",
				}
			}
			innerPath = fp
		} else {
			idx := n - 1
			if outerSeen[idx] {
				return SevenHoleLoadPrbResponse{
					Success: false,
					Error:   fmt.Sprintf("检测到多个 %d.prb 外区文件，请只保留一个", n),
				}
			}
			outerPaths[idx] = fp
			outerSeen[idx] = true
		}
	}

	// 校验：7 个文件必须齐全。
	if innerPath == "" {
		return SevenHoleLoadPrbResponse{
			Success: false,
			Error:   "缺少 7.prb 内区文件（文件名 basename 必须为 '7'）",
		}
	}
	for i, p := range outerPaths {
		if p == "" {
			return SevenHoleLoadPrbResponse{
				Success: false,
				Error:   fmt.Sprintf("缺少 %d.prb 外区文件（文件名 basename 必须为 '%d'）", i+1, i+1),
			}
		}
	}

	// 内区必须最先加载：外区插值在边界场景会回查内区网格（spec §5）。
	interpolator := seven_interp.NewSevenHolePrbInterpolator()

	innerLines, err := ReadPrbLines(innerPath)
	if err != nil {
		return SevenHoleLoadPrbResponse{Success: false, Error: fmt.Sprintf("读取 7.prb 失败: %s", err.Error())}
	}
	if err := interpolator.LoadInnerPrbLines(innerLines, filepath.Base(innerPath)); err != nil {
		return SevenHoleLoadPrbResponse{Success: false, Error: fmt.Sprintf("解析 7.prb 失败: %s", err.Error())}
	}

	// 外区 1..6 按顺序加载，任一失败立即返回（避免错误被淹没）。
	for i, outerPath := range outerPaths {
		sector := i + 1
		outerLines, err := ReadPrbLines(outerPath)
		if err != nil {
			return SevenHoleLoadPrbResponse{Success: false, Error: fmt.Sprintf("读取 %d.prb 失败: %s", sector, err.Error())}
		}
		if err := interpolator.LoadOuterPrbLines(sector, outerLines, filepath.Base(outerPath)); err != nil {
			return SevenHoleLoadPrbResponse{Success: false, Error: fmt.Sprintf("解析 %d.prb 失败: %s", sector, err.Error())}
		}
	}

	// 构造前端展示用的文件列表：内区在前，外区 1..6 顺序跟随。
	prbFiles := make([]SevenHolePrbFileInfo, 0, 7)
	prbFiles = append(prbFiles, SevenHolePrbFileInfo{
		FilePath: innerPath,
		FileName: filepath.Base(innerPath),
		Sector:   0,
	})
	for i, p := range outerPaths {
		prbFiles = append(prbFiles, SevenHolePrbFileInfo{
			FilePath: p,
			FileName: filepath.Base(p),
			Sector:   i + 1,
		})
	}

	validRange := toSevenHoleValidRange(interpolator.GetValidRange())

	// 写操作加写锁，与读路径（Calculate 等）互斥。
	a.sevenHole.mu.Lock()
	a.sevenHole.interpolator = interpolator
	a.sevenHole.prbFiles = prbFiles
	a.sevenHole.mu.Unlock()

	return SevenHoleLoadPrbResponse{
		Success: true,
		Data: &SevenHoleLoadPrbResult{
			Files:      prbFiles,
			ValidRange: validRange,
			Warnings:   warnings,
		},
	}
}

// IsSevenHolePrbLoaded 返回 7 孔 .prb 文件集是否已全部加载。
func (a *App) IsSevenHolePrbLoaded() bool {
	a.sevenHole.mu.RLock()
	defer a.sevenHole.mu.RUnlock()
	return a.sevenHole.interpolator != nil && a.sevenHole.interpolator.IsLoaded()
}

// GetSevenHolePrbFiles 返回已加载的 7 孔 .prb 文件列表。
func (a *App) GetSevenHolePrbFiles() []SevenHolePrbFileInfo {
	a.sevenHole.mu.RLock()
	defer a.sevenHole.mu.RUnlock()
	return a.sevenHole.prbFiles
}

// GetSevenHoleValidRange 返回内区网格的角度覆盖范围（±30°）。
// 注意：算法包明确要求此范围仅供 UI 展示，不得用于事后有效性拒绝（spec §2.2）。
func (a *App) GetSevenHoleValidRange() SevenHoleValidRangeResponse {
	a.sevenHole.mu.RLock()
	interpolator := a.sevenHole.interpolator
	a.sevenHole.mu.RUnlock()

	if interpolator == nil || !interpolator.IsLoaded() {
		return SevenHoleValidRangeResponse{Success: false, Error: "请先加载 7 个 PRB 文件"}
	}
	vr := interpolator.GetValidRange()
	return SevenHoleValidRangeResponse{Success: true, Data: toSevenHoleValidRange(vr)}
}

// CalculateSevenHole 执行 7 孔单点插值计算。
// 输入的所有 P1..P7 必须为表压（gauge），spec §1.1 强制要求。
func (a *App) CalculateSevenHole(input SevenHoleInterpolationInput) SevenHoleCalculateResponse {
	a.sevenHole.mu.RLock()
	interpolator := a.sevenHole.interpolator
	a.sevenHole.mu.RUnlock()

	if interpolator == nil || !interpolator.IsLoaded() {
		return SevenHoleCalculateResponse{Success: false, Error: "请先加载 7 个 PRB 文件"}
	}

	coreInput := toSevenHoleCoreInput(input)
	result, err := interpolator.Calculate(coreInput)
	if err != nil {
		return SevenHoleCalculateResponse{Success: false, Error: err.Error()}
	}

	return SevenHoleCalculateResponse{Success: true, Data: toSevenHoleAppResult(result)}
}

// BatchCalculateSevenHole 批量计算，遇到错误不中断，标记失败行继续。
// 这样 CSV 中个别坏行不会让整个批次作废。
func (a *App) BatchCalculateSevenHole(inputs []SevenHoleInterpolationInput) SevenHoleBatchCalculateResponse {
	a.sevenHole.mu.RLock()
	interpolator := a.sevenHole.interpolator
	a.sevenHole.mu.RUnlock()

	if interpolator == nil || !interpolator.IsLoaded() {
		return SevenHoleBatchCalculateResponse{Success: false, Error: "请先加载 7 个 PRB 文件"}
	}

	results := make([]*SevenHoleInterpolationResult, len(inputs))
	var firstError string

	for i, input := range inputs {
		coreInput := toSevenHoleCoreInput(input)
		result, err := interpolator.Calculate(coreInput)
		if err != nil {
			results[i] = &SevenHoleInterpolationResult{
				IsValid: false,
				Warning: fmt.Sprintf("第%d行计算失败: %s", i+1, err.Error()),
			}
			if firstError == "" {
				firstError = fmt.Sprintf("第%d行计算失败: %s", i+1, err.Error())
			}
			continue
		}
		results[i] = toSevenHoleAppResult(result)
	}

	return SevenHoleBatchCalculateResponse{
		Success: firstError == "",
		Error:   firstError,
		Data:    results,
	}
}

// toSevenHoleCoreInput 将应用层输入转换为核心算法输入。
// 7 孔算法包统一使用表压（spec §1.1），应用层不提供 PressureMode，直接透传。
func toSevenHoleCoreInput(input SevenHoleInterpolationInput) seven_interp.InterpolationInput {
	return seven_interp.InterpolationInput{
		P1:   input.P1,
		P2:   input.P2,
		P3:   input.P3,
		P4:   input.P4,
		P5:   input.P5,
		P6:   input.P6,
		P7:   input.P7,
		PAtm: input.Patm,
		TAtm: input.Tatm,
	}
}

// toSevenHoleAppResult 将算法包结果转为应用层结果，字段一一映射。
// 注意 Alpha/Beta 在 7 孔里语义反转（Alpha=侧滑、Beta=迎角），但 JSON tag 与 5 孔一致。
func toSevenHoleAppResult(r seven_interp.InterpolationResult) *SevenHoleInterpolationResult {
	return &SevenHoleInterpolationResult{
		Alpha:           r.Alpha,
		Beta:            r.Beta,
		MachNumber:      r.MachNumber,
		Velocity:        r.Velocity,
		DynamicPressure: r.DynamicPressure,
		P0:              r.TotalPressure,
		Ps:              r.StaticPressure,
		IsValid:         r.IsValid,
		Warning:         r.Warning,
	}
}

// toSevenHoleValidRange 将算法包的 PrbValidRange 转为应用层类型。
func toSevenHoleValidRange(vr seven_interp.PrbValidRange) SevenHolePrbValidRange {
	return SevenHolePrbValidRange{
		AlphaMin: vr.AlphaMin,
		AlphaMax: vr.AlphaMax,
		BetaMin:  vr.BetaMin,
		BetaMax:  vr.BetaMax,
		MachMin:  vr.MachMin,
		MachMax:  vr.MachMax,
	}
}

// ImportSevenHoleCsvData 弹出文件选择对话框让用户选 7 孔数据 CSV，
// 解析 P1-P7 + Patm + Tatm 列（共 9 列，全部必需）。
//
// 与 5 孔 CSV 的区别：
//   - 5 孔：P1-P5 必需，Patm/Tatm/PressureMode 可选
//   - 7 孔：P1-P7 + Patm + Tatm 全部必需（spec §1.1 强制表压，无 PressureMode 列）
//
// 支持 .csv/.txt/.dat 三种扩展名（与 3 孔一致），分隔符自动检测 tab vs 逗号。
func (a *App) ImportSevenHoleCsvData() SevenHoleImportCsvDataResponse {
	filePath, err := a.openPrbFileDialog("选择 7 孔数据 CSV 文件").
		AddFilter("CSV/TXT/DAT (*.csv;*.txt;*.dat)", "*.csv;*.txt;*.dat").
		AddFilter("CSV Files (*.csv)", "*.csv").
		AddFilter("All Files (*.*)", "*.*").
		PromptForSingleSelection()
	if err != nil {
		return SevenHoleImportCsvDataResponse{Success: false, Error: err.Error()}
	}
	if filePath == "" {
		return SevenHoleImportCsvDataResponse{Success: false, Error: "已取消选择"}
	}

	records, err := readSevenHoleCsvFile(filePath)
	if err != nil {
		return SevenHoleImportCsvDataResponse{Success: false, Error: err.Error()}
	}

	if len(records) < 2 {
		return SevenHoleImportCsvDataResponse{Success: false, Error: "CSV 文件为空或只有表头"}
	}

	header := records[0]
	colMap := make(map[string]int)
	for i, col := range header {
		colMap[strings.TrimSpace(col)] = i
	}

	// 7 孔 CSV 必需列：P1..P7 + Patm + Tatm 共 9 列，缺一不可。
	required := []string{"P1", "P2", "P3", "P4", "P5", "P6", "P7", "Patm", "Tatm"}
	// 用必需列的最大索引作为列数校验阈值，
	// 避免用户 CSV 含可选备注列但数据行缺该列时整行被丢弃。
	maxRequiredIdx := 0
	for _, name := range required {
		idx, ok := colMap[name]
		if !ok {
			return SevenHoleImportCsvDataResponse{
				Success: false,
				Error:   fmt.Sprintf("缺少必要列: %s（7 孔 CSV 需包含 P1-P7 + Patm + Tatm 共 9 列）", name),
			}
		}
		if idx > maxRequiredIdx {
			maxRequiredIdx = idx
		}
	}

	datas := make([]SevenHoleInterpolationInput, 0, len(records)-1)
	var parseWarnings []string

	for rowIdx := 1; rowIdx < len(records); rowIdx++ {
		row := records[rowIdx]
		if len(row) <= maxRequiredIdx {
			parseWarnings = append(parseWarnings, fmt.Sprintf("第%d行列数不足，已跳过", rowIdx+1))
			continue
		}

		// parse 返回错误而非静默返回 0，便于定位坏行。
		parse := func(colIdx int) (float64, error) {
			val, err := strconv.ParseFloat(strings.TrimSpace(row[colIdx]), 64)
			if err != nil {
				return 0, fmt.Errorf("第%d行第%d列解析失败: %q", rowIdx+1, colIdx+1, row[colIdx])
			}
			return val, nil
		}

		p1, err1 := parse(colMap["P1"])
		p2, err2 := parse(colMap["P2"])
		p3, err3 := parse(colMap["P3"])
		p4, err4 := parse(colMap["P4"])
		p5, err5 := parse(colMap["P5"])
		p6, err6 := parse(colMap["P6"])
		p7, err7 := parse(colMap["P7"])
		patm, err8 := parse(colMap["Patm"])
		tatm, err9 := parse(colMap["Tatm"])

		if err1 != nil || err2 != nil || err3 != nil || err4 != nil || err5 != nil || err6 != nil || err7 != nil || err8 != nil || err9 != nil {
			for _, e := range []error{err1, err2, err3, err4, err5, err6, err7, err8, err9} {
				if e != nil {
					parseWarnings = append(parseWarnings, e.Error())
				}
			}
			continue
		}

		datas = append(datas, SevenHoleInterpolationInput{
			P1: p1, P2: p2, P3: p3, P4: p4, P5: p5, P6: p6, P7: p7,
			Patm: patm, Tatm: tatm,
		})
	}

	if len(parseWarnings) > 0 && len(datas) > 0 {
		log.Printf("7 孔 CSV 导入警告: %s", strings.Join(parseWarnings, "; "))
	}

	if len(datas) == 0 && len(parseWarnings) > 0 {
		return SevenHoleImportCsvDataResponse{
			Success: false,
			Error:   fmt.Sprintf("所有数据行解析失败: %s", strings.Join(parseWarnings, "; ")),
		}
	}

	return SevenHoleImportCsvDataResponse{Success: true, Data: datas}
}

// readSevenHoleCsvFile 读取 CSV 文件并自动检测分隔符（tab 或逗号）。
// 与 3 孔 readThreeHoleCsvFile 一致：剥离 UTF-8 BOM、跳过空行与 # 注释行。
// 7 孔 CSV 通常用 tab 分隔（与校准数据导出格式一致），但也支持逗号分隔。
func readSevenHoleCsvFile(filePath string) ([][]string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("打开文件失败: %s", err.Error())
	}
	defer file.Close()

	// 剥离 UTF-8 BOM：Excel "另存为 CSV UTF-8" 会在首字节加 BOM，
	// 导致首列名变成 "\uFEFFP1"，colMap["P1"] 查找失败。
	br := bufio.NewReader(file)
	StripUTF8BOM(br)

	// 按行读取并跳过空行与 # 注释行（与用户说明书承诺一致）。
	scanner := bufio.NewScanner(br)
	var dataLines []string
	for scanner.Scan() {
		trimmed := strings.TrimSpace(scanner.Text())
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

	// 检测分隔符：统计第一行 tab 与逗号出现次数，多的为主分隔符。
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

// OpenSevenHoleHelpDoc 用系统默认程序打开 7 孔用户说明书 HTML 文件。
// 路径解析与跨平台打开逻辑抽到共享工具 GetHelpDocPath / OpenHelpDocByPath。
func (a *App) OpenSevenHoleHelpDoc() error {
	return OpenHelpDocByPath(GetHelpDocPath(sevenHoleHelpDocFileName))
}
