package backend

import (
	"bufio"
	"context"
	"encoding/csv"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"

	wind_interp "ai-workspace/shared/algorithms/go/fivehole/interpolation"
	"github.com/wailsapp/wails/v3/pkg/application"
)

// helpDocFileName 用户说明书文件名常量
const helpDocFileName = "用户说明书.html"

// getHelpDocPath 获取用户说明书文件路径
func getHelpDocPath() string {
	ex, err := os.Executable()
	if err != nil {
		return ""
	}
	exeDir := filepath.Dir(ex)

	possiblePaths := []string{
		filepath.Join(exeDir, "docs", helpDocFileName),
		filepath.Join(exeDir, "..", "docs", helpDocFileName),
		filepath.Join(exeDir, "..", "..", "docs", helpDocFileName),
	}

	for _, p := range possiblePaths {
		if _, err := os.Stat(p); err == nil {
			return p
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

// #16 修复：与核心库 PrbValidRange 对齐，补充 MachMin/MachMax 字段
type PrbValidRange struct {
	AlphaMin float64 `json:"alphaMin"`
	AlphaMax float64 `json:"alphaMax"`
	BetaMin  float64 `json:"betaMin"`
	BetaMax  float64 `json:"betaMax"`
	MachMin  float64 `json:"machMin"`
	MachMax  float64 `json:"machMax"`
}

type InterpolationInput struct {
	P1           float64 `json:"P1"`
	P2           float64 `json:"P2"`
	P3           float64 `json:"P3"`
	P4           float64 `json:"P4"`
	P5           float64 `json:"P5"`
	Patm         float64 `json:"Patm"`
	Tatm         float64 `json:"Tatm"`
	PressureMode string  `json:"pressureMode"`
}

type InterpolationResult struct {
	Alpha           float64 `json:"alpha"`
	Beta            float64 `json:"beta"`
	MachNumber      float64 `json:"machNumber"`
	V               float64 `json:"V"`
	Vx              float64 `json:"Vx"`
	Vy              float64 `json:"Vy"`
	Vz              float64 `json:"Vz"`
	Velocity        float64 `json:"velocity"`
	CAS             float64 `json:"cas"`
	SAT             float64 `json:"sat"`
	DynamicPressure float64 `json:"dynamicPressure"`
	Density         float64 `json:"density"`
	TotalPressure   float64 `json:"P0"`
	StaticPressure  float64 `json:"Ps"`
	IsValid         bool    `json:"isValid"`
	Warning         string  `json:"warning,omitempty"`
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

// #3 修复：添加读写互斥锁保护并发访问
type App struct {
	mu          sync.RWMutex
	ctx         context.Context
	app         *application.App
	multiInterp *wind_interp.MultiPrbInterpolator
	prbFiles    []PrbFileInfo
}

func NewApp() *App {
	return &App{}
}

func (a *App) ServiceStartup(ctx context.Context, options application.ServiceOptions) error {
	a.ctx = ctx
	a.app = application.Get()
	return nil
}

func (a *App) LoadPrbFiles() LoadPrbResponse {
	filePaths, err := a.openFileDialog("选择多个 PRB 校准文件").PromptForMultipleSelection()
	if err != nil {
		return LoadPrbResponse{Success: false, Error: err.Error()}
	}
	if len(filePaths) == 0 {
		return LoadPrbResponse{Success: false, Error: "已取消选择"}
	}

	fileData := make([]wind_interp.PrbFileData, 0, len(filePaths))
	for _, fp := range filePaths {
		lines, err := readPrbLines(fp)
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

	// #16 修复：完整映射核心库字段，包括 MachMin/MachMax
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

	// #3 修复：写操作加写锁
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

// #8 修复：批量计算不再遇到第一个错误即中断，而是标记失败行继续计算
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
			// 记录错误但继续计算后续行
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

// toCoreInput 将应用层输入转换为核心算法输入
// 核心算法内部统一使用表压，绝压输入时自动减去大气压转为表压
func toCoreInput(input InterpolationInput) wind_interp.InterpolationInput {
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

// #1 修复：CSV 解析不再静默吞掉错误，收集解析失败信息
func (a *App) ImportCsvData() ImportCsvDataResponse {
	filePath, err := a.openFileDialog("选择数据 CSV 文件").
		AddFilter("CSV Files (*.csv)", "*.csv").
		PromptForSingleSelection()
	if err != nil {
		return ImportCsvDataResponse{Success: false, Error: err.Error()}
	}
	if filePath == "" {
		return ImportCsvDataResponse{Success: false, Error: "已取消选择"}
	}

	file, err := os.Open(filePath)
	if err != nil {
		return ImportCsvDataResponse{Success: false, Error: fmt.Sprintf("打开文件失败: %s", err.Error())}
	}
	defer file.Close()

	reader := csv.NewReader(file)
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

	datas := make([]InterpolationInput, 0, len(records)-1)
	var parseWarnings []string

	for rowIdx := 1; rowIdx < len(records); rowIdx++ {
		row := records[rowIdx]
		if len(row) < len(header) {
			continue
		}

		// #1 修复：解析函数现在返回错误而非静默返回0
		parse := func(colIdx int) (float64, error) {
			val, err := strconv.ParseFloat(strings.TrimSpace(row[colIdx]), 64)
			if err != nil {
				return 0, fmt.Errorf("第%d行第%d列解析失败: %q", rowIdx+1, colIdx+1, row[colIdx])
			}
			return val, nil
		}

		// 解析必需列，任一失败则跳过该行并记录警告
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

		input := InterpolationInput{
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

	// 如果有解析警告但仍有有效数据，返回成功并附带警告
	if len(parseWarnings) > 0 && len(datas) > 0 {
		log.Printf("CSV导入警告: %s", strings.Join(parseWarnings, "; "))
	}

	// 如果所有行都解析失败，返回错误
	if len(datas) == 0 && len(parseWarnings) > 0 {
		return ImportCsvDataResponse{
			Success: false,
			Error:   fmt.Sprintf("所有数据行解析失败: %s", strings.Join(parseWarnings, "; ")),
		}
	}

	return ImportCsvDataResponse{Success: true, Data: datas}
}

func (a *App) openFileDialog(title string) *application.OpenFileDialogStruct {
	app := a.app
	if app == nil {
		app = application.Get()
	}
	return app.Dialog.OpenFile().
		SetTitle(title).
		AddFilter("PRB Files (*.prb)", "*.prb").
		AddFilter("All Files (*.*)", "*.*")
}

func toAppResult(r wind_interp.InterpolationResult) *InterpolationResult {
	return &InterpolationResult{
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

// #2 修复：检查 cmd.Start() 的返回错误
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
