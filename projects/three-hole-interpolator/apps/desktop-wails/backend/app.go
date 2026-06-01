package backend

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"

	three_interp "ai-workspace/shared/algorithms/go/threehole/interpolation"
	wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

const helpDocFileName = "用户说明书.html"

func getHelpDocPath() string {
	ex, err := os.Executable()
	if err != nil {
		return ""
	}
	exeDir := filepath.Dir(ex)

	// 收集所有可能的路径候选
	possiblePaths := []string{
		// 同级 docs 目录（打包后的标准结构）
		filepath.Join(exeDir, "docs", helpDocFileName),
		// 向上回溯两级（开发模式：backend 目录下运行）
		filepath.Join(exeDir, "..", "..", "docs", helpDocFileName),
		// 向上回溯三级（开发模式：frontend 目录下运行）
		filepath.Join(exeDir, "..", "..", "..", "docs", helpDocFileName),
		// 向上回溯四级（极端情况）
		filepath.Join(exeDir, "..", "..", "..", "..", "docs", helpDocFileName),
	}

	// 在 Windows 开发模式下，wails dev 可能使用临时目录，尝试从工作目录查找
	if runtime.GOOS == "windows" {
		if cwd, err := os.Getwd(); err == nil {
			possiblePaths = append(possiblePaths,
				filepath.Join(cwd, "docs", helpDocFileName),
				filepath.Join(cwd, "..", "docs", helpDocFileName),
				filepath.Join(cwd, "..", "..", "docs", helpDocFileName),
			)
		}
	}

	for _, p := range possiblePaths {
		// 使用 Clean 规范化路径（解析 .. 等）
		cleanPath := filepath.Clean(p)
		if info, err := os.Stat(cleanPath); err == nil && !info.IsDir() {
			return cleanPath
		}
	}
	return ""
}

type PrbFileInfo struct {
	FilePath   string        `json:"filePath"`
	FileName   string        `json:"fileName"`
	MachNumber float64       `json:"machNumber"`
	ValidRange PrbValidRange `json:"validRange"`
}

type PrbValidRange struct {
	AlphaMin float64 `json:"alphaMin"`
	AlphaMax float64 `json:"alphaMax"`
	MachMin  float64 `json:"machMin"`
	MachMax  float64 `json:"machMax"`
}

type InterpolationInput struct {
	P1           float64 `json:"P1"`
	P2           float64 `json:"P2"`
	P3           float64 `json:"P3"`
	Patm         float64 `json:"Patm"`
	Tatm         float64 `json:"Tatm"`
	PressureMode string  `json:"pressureMode"`
}

type InterpolationResult struct {
	Alpha          float64 `json:"alpha"`
	MachNumber     float64 `json:"machNumber"`
	TotalPressure  float64 `json:"P0"`
	StaticPressure float64 `json:"Ps"`
	IterationCount int     `json:"iterationCount"`
	IsValid        bool    `json:"isValid"`
	Warning        string  `json:"warning,omitempty"`
}

type GenericResponse struct {
	Success bool   `json:"success"`
	Error   string `json:"error,omitempty"`
}

type LoadPrbResponse struct {
	Success bool           `json:"success"`
	Error   string         `json:"error,omitempty"`
	Data    *LoadPrbResult `json:"data,omitempty"`
}

type LoadPrbResult struct {
	Files     []PrbFileInfo `json:"files"`
	MachRange []float64     `json:"machRange"`
	Warnings  []string      `json:"warnings"`
}

type MachRangeResponse struct {
	Success bool      `json:"success"`
	Error   string    `json:"error,omitempty"`
	Data    []float64 `json:"data,omitempty"`
}

type CalculateResponse struct {
	Success bool                 `json:"success"`
	Error   string               `json:"error,omitempty"`
	Data    *InterpolationResult `json:"data,omitempty"`
}

type BatchCalculateResponse struct {
	Success bool                   `json:"success"`
	Error   string                 `json:"error,omitempty"`
	Data    []*InterpolationResult `json:"data,omitempty"`
}

type ImportCsvDataResponse struct {
	Success bool                 `json:"success"`
	Error   string               `json:"error,omitempty"`
	Data    []InterpolationInput `json:"data,omitempty"`
}

type App struct {
	mu          sync.RWMutex
	ctx         context.Context
	multiInterp *three_interp.ThreeHoleInterpolator
	prbFiles    []PrbFileInfo
}

func NewApp() *App {
	return &App{}
}

func (a *App) Startup(ctx context.Context) {
	a.ctx = ctx
}

func (a *App) LoadPrbFiles() LoadPrbResponse {
	filePaths, err := wailsRuntime.OpenMultipleFilesDialog(a.ctx, wailsRuntime.OpenDialogOptions{
		Title: "选择多个 PRB 校准文件",
		Filters: []wailsRuntime.FileFilter{
			{DisplayName: "PRB Files (*.prb)", Pattern: "*.prb"},
			{DisplayName: "All Files (*.*)", Pattern: "*.*"},
		},
	})
	if err != nil {
		return LoadPrbResponse{Success: false, Error: err.Error()}
	}
	if len(filePaths) == 0 {
		return LoadPrbResponse{Success: false, Error: "已取消选择"}
	}

	fileData := make([]three_interp.PrbFileData, 0, len(filePaths))
	for _, fp := range filePaths {
		lines, err := readPrbLines(fp)
		if err != nil {
			return LoadPrbResponse{Success: false, Error: fmt.Sprintf("读取 %s 失败: %s", filepath.Base(fp), err.Error())}
		}
		fileData = append(fileData, three_interp.PrbFileData{FilePath: fp, Lines: lines})
	}

	interpolator := three_interp.NewThreeHoleInterpolator()
	result, err := interpolator.LoadPrbData(fileData)
	if err != nil {
		return LoadPrbResponse{Success: false, Error: err.Error()}
	}

	prbFiles := make([]PrbFileInfo, 0, len(result.Files))
	for _, f := range result.Files {
		prbFiles = append(prbFiles, PrbFileInfo{
			FilePath:   f.FilePath,
			FileName:   f.FileName,
			MachNumber: f.MachNumber,
			ValidRange: PrbValidRange{
				AlphaMin: f.ValidRange.AlphaMin,
				AlphaMax: f.ValidRange.AlphaMax,
				MachMin:  f.ValidRange.MachMin,
				MachMax:  f.ValidRange.MachMax,
			},
		})
	}

	minMa, maxMa := interpolator.GetMachRange()
	machRange := []float64{minMa, maxMa}

	a.mu.Lock()
	a.multiInterp = interpolator
	a.prbFiles = prbFiles
	a.mu.Unlock()

	return LoadPrbResponse{
		Success: true,
		Data: &LoadPrbResult{
			Files:     prbFiles,
			MachRange: machRange,
			Warnings:  result.Warnings,
		},
	}
}

func readPrbLines(filePath string) ([]string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("open PRB file: %w", err)
	}
	defer file.Close()

	var lines []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" {
			lines = append(lines, line)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read PRB file: %w", err)
	}
	return lines, nil
}

func (a *App) IsPrbLoaded() bool {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.multiInterp != nil && a.multiInterp.IsLoaded()
}

func (a *App) GetPrbFiles() []PrbFileInfo {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.prbFiles
}

func (a *App) GetMachRange() MachRangeResponse {
	a.mu.RLock()
	interpolator := a.multiInterp
	a.mu.RUnlock()

	if interpolator == nil || !interpolator.IsLoaded() {
		return MachRangeResponse{Success: false, Error: "请先加载PRB文件"}
	}
	min, max := interpolator.GetMachRange()
	return MachRangeResponse{Success: true, Data: []float64{min, max}}
}

func (a *App) Calculate(input InterpolationInput) CalculateResponse {
	a.mu.RLock()
	interpolator := a.multiInterp
	a.mu.RUnlock()

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

func (a *App) BatchCalculate(datas []InterpolationInput) BatchCalculateResponse {
	a.mu.RLock()
	interpolator := a.multiInterp
	a.mu.RUnlock()

	if interpolator == nil || !interpolator.IsLoaded() {
		return BatchCalculateResponse{Success: false, Error: "请先加载PRB文件"}
	}

	results := make([]*InterpolationResult, len(datas))
	var firstError string

	for i, input := range datas {
		coreInput := toCoreInput(input)
		result, err := interpolator.Calculate(coreInput)
		if err != nil {
			results[i] = &InterpolationResult{
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

func toCoreInput(input InterpolationInput) three_interp.InterpolationInput {
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

func toAppResult(r three_interp.InterpolationResult) *InterpolationResult {
	return &InterpolationResult{
		Alpha:          r.Alpha,
		MachNumber:     r.MachNumber,
		TotalPressure:  r.TotalPressure,
		StaticPressure: r.StaticPressure,
		IterationCount: r.IterationCount,
		IsValid:        r.IsValid,
		Warning:        r.Warning,
	}
}

// ==================== CSV 导入相关方法 ====================

func (a *App) ImportCsvData() ImportCsvDataResponse {
	filePath, err := wailsRuntime.OpenFileDialog(a.ctx, wailsRuntime.OpenDialogOptions{
		Title: "选择数据文件",
		Filters: []wailsRuntime.FileFilter{
			{DisplayName: "数据文件 (*.csv, *.txt)", Pattern: "*.csv;*.txt"},
			{DisplayName: "All Files (*.*)", Pattern: "*.*"},
		},
	})
	if err != nil {
		return ImportCsvDataResponse{Success: false, Error: err.Error()}
	}
	if filePath == "" {
		return ImportCsvDataResponse{Success: false, Error: "已取消选择"}
	}

	records, err := readCsvFile(filePath)
	if err != nil {
		return ImportCsvDataResponse{Success: false, Error: err.Error()}
	}

	colMap, csvErr := parseCsvHeader(records[0])
	if csvErr != "" {
		return ImportCsvDataResponse{Success: false, Error: csvErr}
	}

	datas, warnings := parseCsvRows(records[1:], colMap)
	if len(warnings) > 0 {
		log.Printf("CSV导入警告: %s", strings.Join(warnings, "; "))
	}

	if len(datas) == 0 {
		errMsg := "所有数据行解析失败"
		if len(warnings) > 0 {
			errMsg = fmt.Sprintf("所有数据行解析失败: %s", strings.Join(warnings, "; "))
		}
		return ImportCsvDataResponse{Success: false, Error: errMsg}
	}

	return ImportCsvDataResponse{Success: true, Data: datas}
}

func readCsvFile(filePath string) ([][]string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("打开文件失败: %s", err.Error())
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
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

	delimiter := detectDelimiter(dataLines[0])

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

func detectDelimiter(line string) rune {
	tabCount := strings.Count(line, "\t")
	commaCount := strings.Count(line, ",")
	if tabCount > commaCount {
		return '\t'
	}
	return ','
}

func parseCsvHeader(header []string) (map[string]int, string) {
	colMap := make(map[string]int)
	for i, col := range header {
		colMap[strings.TrimSpace(col)] = i
	}

	requiredCols := []string{"P1", "P2", "P3"}
	for _, name := range requiredCols {
		if _, ok := colMap[name]; !ok {
			return nil, fmt.Sprintf("缺少必要列: %s", name)
		}
	}

	return colMap, ""
}

func parseCsvRows(rows [][]string, colMap map[string]int) ([]InterpolationInput, []string) {
	colCount := len(colMap)

	patmIdx, hasPatm := colMap["Patm"]
	tatmIdx, hasTatm := colMap["Tatm"]
	pmIdx, hasPm := colMap["PressureMode"]

	datas := make([]InterpolationInput, 0, len(rows))
	var warnings []string

	for rowIdx := 0; rowIdx < len(rows); rowIdx++ {
		row := rows[rowIdx]
		csvLine := rowIdx + 2 // CSV行号 = 数据行索引 + 表头行(1) + 1

		if len(row) < colCount {
			warnings = append(warnings, fmt.Sprintf("第%d行列数不足，已跳过", csvLine))
			continue
		}

		parseField := func(colIdx int, fieldName string) (float64, error) {
			val, err := strconv.ParseFloat(strings.TrimSpace(row[colIdx]), 64)
			if err != nil {
				return 0, fmt.Errorf("第%d行%s解析失败: %q", csvLine, fieldName, row[colIdx])
			}
			return val, nil
		}

		// 必须列：解析失败则跳过整行
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

		input := InterpolationInput{P1: p1, P2: p2, P3: p3}

		// 可选列：解析失败也跳过整行，避免静默使用零值
		if hasPatm {
			val, err := parseField(patmIdx, "Patm")
			if err != nil {
				warnings = append(warnings, err.Error())
				continue
			}
			input.Patm = val
		}
		if hasTatm {
			val, err := parseField(tatmIdx, "Tatm")
			if err != nil {
				warnings = append(warnings, err.Error())
				continue
			}
			input.Tatm = val
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

	return datas, warnings
}

// ==================== 帮助文档 ====================

func (a *App) OpenHelpDoc() error {
	helpPath := getHelpDocPath()
	if helpPath == "" {
		return fmt.Errorf("未找到用户说明书文件")
	}

	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("cmd", "/c", "start", "", helpPath)
	case "darwin":
		cmd = exec.Command("open", helpPath)
	default:
		cmd = exec.Command("xdg-open", helpPath)
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("打开帮助文档失败: %w", err)
	}
	return nil
}