package backend

import (
	"bufio"
	"encoding/csv"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	wind_interp "ai-workspace/shared/algorithms/go/fivehole/interpolation"
)

// fiveHoleHelpDocFileName 是 5 孔用户说明书文件名（与 docs/ 目录下文件名一致）。
// 命名带 "five-hole-" 前缀，避免与 3/7 孔说明书冲突；Task 8 合并版说明书统一管理。
const fiveHoleHelpDocFileName = "five-hole-用户说明书.html"

// fiveHoleState 封装 5 孔探针插值的运行时状态，自带 RWMutex。
// 与 3 孔 / 7 孔的 state 隔离，避免锁混用（SPEC § Boundaries 要求）。
type fiveHoleState struct {
	mu          sync.RWMutex
	multiInterp *wind_interp.MultiPrbInterpolator
	prbFiles    []PrbFileInfo
}

// LoadPrbFiles 加载多个 5 孔 .prb 校准文件，构建 MultiPrbInterpolator 并缓存到 fiveHoleState。
//
// Win7 分支：文件路径由前端通过 Electron IPC 选择后传入，后端不再弹原生对话框。
// 返回加载结果（文件列表 + 马赫数范围 + 警告）供前端展示。
func (a *App) LoadPrbFiles(filePaths []string) LoadPrbResponse {
	if len(filePaths) == 0 {
		return LoadPrbResponse{Success: false, Error: "未选择文件"}
	}

	fileData := make([]wind_interp.PrbFileData, 0, len(filePaths))
	for _, fp := range filePaths {
		lines, err := ReadPrbLines(fp)
		if err != nil {
			return LoadPrbResponse{Success: false, Error: fmt.Sprintf("读取 %s 失败: %s", filepath.Base(fp), err.Error())}
		}
		fileData = append(fileData, wind_interp.PrbFileData{FilePath: fp, Lines: lines})
	}

	interpolator := wind_interp.NewMultiPrbInterpolator()
	result, err := interpolator.LoadPrbData(fileData, nil)
	if err != nil {
		return LoadPrbResponse{Success: false, Error: err.Error()}
	}

	prbFiles := make([]PrbFileInfo, 0, len(result.Files))
	for _, f := range result.Files {
		prbFiles = append(prbFiles, PrbFileInfo{
			FilePath:   f.FilePath,
			FileName:   f.FileName,
			MachNumber: 0,
			ValidRange: PrbValidRange{
				AlphaMin: f.ValidRange.AlphaMin,
				AlphaMax: f.ValidRange.AlphaMax,
				BetaMin:  f.ValidRange.BetaMin,
				BetaMax:  f.ValidRange.BetaMax,
				MachMin:  f.ValidRange.MachMin,
				MachMax:  f.ValidRange.MachMax,
			},
		})
	}
	for i, ma := range result.MachNumbers {
		if i < len(prbFiles) {
			prbFiles[i].MachNumber = ma
		}
	}

	minMa, maxMa := interpolator.GetMachRange()
	machRange := []float64{minMa, maxMa}

	// 写操作加写锁，与读路径（Calculate 等）互斥
	a.fiveHole.mu.Lock()
	a.fiveHole.multiInterp = interpolator
	a.fiveHole.prbFiles = prbFiles
	a.fiveHole.mu.Unlock()

	return LoadPrbResponse{
		Success: true,
		Data: &LoadPrbResult{
			Files:     prbFiles,
			MachRange: machRange,
			Warnings:  result.Warnings,
		},
	}
}

// IsPrbLoaded 返回 5 孔 .prb 是否已加载。
func (a *App) IsPrbLoaded() bool {
	a.fiveHole.mu.RLock()
	defer a.fiveHole.mu.RUnlock()
	return a.fiveHole.multiInterp != nil && a.fiveHole.multiInterp.IsLoaded()
}

// GetPrbFiles 返回已加载的 5 孔 .prb 文件列表。
func (a *App) GetPrbFiles() []PrbFileInfo {
	a.fiveHole.mu.RLock()
	defer a.fiveHole.mu.RUnlock()
	return a.fiveHole.prbFiles
}

// GetMachRange 返回已加载 .prb 的马赫数覆盖范围 [min, max]。
func (a *App) GetMachRange() MachRangeResponse {
	a.fiveHole.mu.RLock()
	interpolator := a.fiveHole.multiInterp
	a.fiveHole.mu.RUnlock()

	if interpolator == nil || !interpolator.IsLoaded() {
		return MachRangeResponse{Success: false, Error: "请先加载PRB文件"}
	}
	min, max := interpolator.GetMachRange()
	return MachRangeResponse{Success: true, Data: []float64{min, max}}
}

// Calculate 执行 5 孔单点插值计算。
func (a *App) Calculate(input FiveHoleInterpolationInput) CalculateResponse {
	a.fiveHole.mu.RLock()
	interpolator := a.fiveHole.multiInterp
	a.fiveHole.mu.RUnlock()

	if interpolator == nil || !interpolator.IsLoaded() {
		return CalculateResponse{Success: false, Error: "请先加载PRB文件"}
	}

	coreInput := toCoreInput(input)

	result, err := interpolator.Calculate(coreInput)
	if err != nil {
		return CalculateResponse{Success: false, Error: err.Error()}
	}

	return CalculateResponse{Success: true, Data: toAppResult(result)}
}

// BatchCalculate 批量计算，遇到错误不中断，标记失败行继续。
// 这样 CSV 中个别坏行不会让整个批次作废。
func (a *App) BatchCalculate(datas []FiveHoleInterpolationInput) BatchCalculateResponse {
	a.fiveHole.mu.RLock()
	interpolator := a.fiveHole.multiInterp
	a.fiveHole.mu.RUnlock()

	if interpolator == nil || !interpolator.IsLoaded() {
		return BatchCalculateResponse{Success: false, Error: "请先加载PRB文件"}
	}

	results := make([]*FiveHoleInterpolationResult, len(datas))
	var firstError string

	for i, input := range datas {
		coreInput := toCoreInput(input)
		result, err := interpolator.Calculate(coreInput)
		if err != nil {
			results[i] = &FiveHoleInterpolationResult{
				IsValid: false,
				Warning: fmt.Sprintf("第%d行计算失败: %s", i+1, err.Error()),
			}
			if firstError == "" {
				firstError = fmt.Sprintf("第%d行计算失败: %s", i+1, err.Error())
			}
			continue
		}
		results[i] = toAppResult(result)
	}

	return BatchCalculateResponse{
		Success: firstError == "",
		Error:   firstError,
		Data:    results,
	}
}

// toCoreInput 将应用层输入转换为核心算法输入。
// 核心算法内部统一使用表压，绝压输入时自动减去大气压转为表压。
func toCoreInput(input FiveHoleInterpolationInput) wind_interp.InterpolationInput {
	coreInput := wind_interp.InterpolationInput{
		P1:   input.P1,
		P2:   input.P2,
		P3:   input.P3,
		P4:   input.P4,
		P5:   input.P5,
		PAtm: input.Patm,
		TAtm: input.Tatm,
	}

	if input.PressureMode == "absolute" {
		coreInput.P1 = input.P1 - input.Patm
		coreInput.P2 = input.P2 - input.Patm
		coreInput.P3 = input.P3 - input.Patm
		coreInput.P4 = input.P4 - input.Patm
		coreInput.P5 = input.P5 - input.Patm
	}

	return coreInput
}

// ImportCsvData 解析 5 孔数据 CSV，提取 P1-P5 + Patm + Tatm + PressureMode 列。
// 必须包含 P1-P5 列，Patm/Tatm/PressureMode 可选（缺失时用零值）。
//
// Win7 分支：文件路径由前端通过 Electron IPC 选择后传入，后端不再弹原生对话框。
func (a *App) ImportCsvData(filePath string) ImportCsvDataResponse {
	if filePath == "" {
		return ImportCsvDataResponse{Success: false, Error: "未选择文件"}
	}

	file, err := os.Open(filePath)
	if err != nil {
		return ImportCsvDataResponse{Success: false, Error: fmt.Sprintf("打开文件失败: %s", err.Error())}
	}
	defer file.Close()

	// 剥离 UTF-8 BOM：Excel 在 Windows 下"另存为 CSV UTF-8"会在首字节加 BOM，
	// 导致首列名变成 "\uFEFFP1"，colMap["P1"] 查找失败。
	br := bufio.NewReader(file)
	StripUTF8BOM(br)

	reader := csv.NewReader(br)
	records, err := reader.ReadAll()
	if err != nil {
		return ImportCsvDataResponse{Success: false, Error: fmt.Sprintf("读取CSV失败: %s", err.Error())}
	}

	if len(records) < 2 {
		return ImportCsvDataResponse{Success: false, Error: "CSV文件为空或只有表头"}
	}

	header := records[0]
	colMap := make(map[string]int)
	for i, col := range header {
		colMap[strings.TrimSpace(col)] = i
	}

	required := []string{"P1", "P2", "P3", "P4", "P5"}
	for _, name := range required {
		if _, ok := colMap[name]; !ok {
			return ImportCsvDataResponse{Success: false, Error: fmt.Sprintf("缺少必要列: %s", name)}
		}
	}

	patmIdx, hasPatm := colMap["Patm"]
	tatmIdx, hasTatm := colMap["Tatm"]
	pmIdx, hasPm := colMap["PressureMode"]

	datas := make([]FiveHoleInterpolationInput, 0, len(records)-1)
	var parseWarnings []string

	for rowIdx := 1; rowIdx < len(records); rowIdx++ {
		row := records[rowIdx]
		if len(row) < len(header) {
			continue
		}

		// parse 函数返回错误而非静默返回 0，便于定位坏行
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

		if err1 != nil || err2 != nil || err3 != nil || err4 != nil || err5 != nil {
			for _, e := range []error{err1, err2, err3, err4, err5} {
				if e != nil {
					parseWarnings = append(parseWarnings, e.Error())
				}
			}
			continue
		}

		input := FiveHoleInterpolationInput{
			P1: p1, P2: p2, P3: p3, P4: p4, P5: p5,
		}
		if hasPatm {
			if val, err := parse(patmIdx); err != nil {
				parseWarnings = append(parseWarnings, err.Error())
			} else {
				input.Patm = val
			}
		}
		if hasTatm {
			if val, err := parse(tatmIdx); err != nil {
				parseWarnings = append(parseWarnings, err.Error())
			} else {
				input.Tatm = val
			}
		}
		if hasPm {
			pm := strings.TrimSpace(row[pmIdx])
			if pm == "absolute" {
				input.PressureMode = "absolute"
			} else {
				input.PressureMode = "gauge"
			}
		}
		datas = append(datas, input)
	}

	if len(parseWarnings) > 0 && len(datas) > 0 {
		log.Printf("CSV导入警告: %s", strings.Join(parseWarnings, "; "))
	}

	if len(datas) == 0 && len(parseWarnings) > 0 {
		return ImportCsvDataResponse{
			Success: false,
			Error:   fmt.Sprintf("所有数据行解析失败: %s", strings.Join(parseWarnings, "; ")),
		}
	}

	return ImportCsvDataResponse{Success: true, Data: datas}
}

// toAppResult 将核心算法结果转为应用层结果，字段一一映射。
func toAppResult(r wind_interp.InterpolationResult) *FiveHoleInterpolationResult {
	return &FiveHoleInterpolationResult{
		Alpha: r.Alpha, Beta: r.Beta,
		MachNumber: r.MachNumber,
		V:          r.V, Vx: r.Vx, Vy: r.Vy, Vz: r.Vz,
		Velocity: r.Velocity,
		CAS:      r.CAS, SAT: r.SAT,
		DynamicPressure: r.DynamicPressure, Density: r.Density,
		TotalPressure: r.TotalPressure, StaticPressure: r.StaticPressure,
		IsValid: r.IsValid, Warning: r.Warning,
	}
}

// OpenHelpDoc 用系统默认程序打开 5 孔用户说明书 HTML 文件。
// 路径解析与跨平台打开逻辑抽到共享工具 GetHelpDocPath / OpenHelpDocByPath。
func (a *App) OpenHelpDoc() error {
	return OpenHelpDocByPath(GetHelpDocPath(fiveHoleHelpDocFileName))
}
