package backend

import (
	"context"
	"encoding/csv"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"
	wind_interp "wind-daq/services/api-go/pkg/interpolation"
)

// getHelpDocPath 获取用户说明书文件路径
func getHelpDocPath() string {
	// 获取当前可执行文件所在目录
	ex, err := os.Executable()
	if err != nil {
		return ""
	}
	exeDir := filepath.Dir(ex)

	// 尝试多个可能的路径
	possiblePaths := []string{
		filepath.Join(exeDir, "docs", "用户说明书.html"),
		filepath.Join(exeDir, "..", "docs", "用户说明书.html"),
		filepath.Join(exeDir, "..", "..", "docs", "用户说明书.html"),
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

type PrbValidRange struct {
	AlphaMin float64 `json:"alphaMin"`
	AlphaMax float64 `json:"alphaMax"`
	BetaMin  float64 `json:"betaMin"`
	BetaMax  float64 `json:"betaMax"`
}

type InterpolationInput struct {
	P1   float64 `json:"P1"`
	P2   float64 `json:"P2"`
	P3   float64 `json:"P3"`
	P4   float64 `json:"P4"`
	P5   float64 `json:"P5"`
	Patm float64 `json:"Patm"`
	Tatm float64 `json:"Tatm"`
}

type InterpolationResult struct {
	Alpha           float64 `json:"alpha"`
	Beta            float64 `json:"beta"`
	MachNumber      float64 `json:"machNumber"`
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

type LoadPrbResult struct {
	Files      []PrbFileInfo `json:"files"`
	MachRange  []float64     `json:"machRange"`
	Warnings   []string      `json:"warnings"`
}

type App struct {
	ctx           context.Context
	multiInterp   *wind_interp.MultiPrbInterpolator
	prbFiles      []PrbFileInfo
}

func NewApp() *App {
	return &App{}
}

func (a *App) Startup(ctx context.Context) {
	a.ctx = ctx
}

func (a *App) LoadPrbFiles() (GenericResponse, *LoadPrbResult) {
	filePaths, err := runtime.OpenMultipleFilesDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "选择多个 PRB 校准文件",
		Filters: []runtime.FileFilter{
			{DisplayName: "PRB Files (*.prb)", Pattern: "*.prb"},
			{DisplayName: "All Files (*.*)", Pattern: "*.*"},
		},
	})
	if err != nil {
		return GenericResponse{Success: false, Error: err.Error()}, nil
	}
	if len(filePaths) == 0 {
		return GenericResponse{Success: false, Error: "已取消选择"}, nil
	}

	fileData := make([]wind_interp.PrbFileData, 0, len(filePaths))
	for _, fp := range filePaths {
		lines, err := wind_interp.ReadPrbLines(fp)
		if err != nil {
			return GenericResponse{Success: false, Error: fmt.Sprintf("读取 %s 失败: %s", filepath.Base(fp), err.Error())}, nil
		}
		fileData = append(fileData, wind_interp.PrbFileData{FilePath: fp, Lines: lines})
	}

	interpolator := wind_interp.NewMultiPrbInterpolator()
	result, err := interpolator.LoadPrbData(fileData, nil)
	if err != nil {
		return GenericResponse{Success: false, Error: err.Error()}, nil
	}

	a.multiInterp = interpolator
	a.prbFiles = nil
	for _, f := range result.Files {
		a.prbFiles = append(a.prbFiles, PrbFileInfo{
			FilePath: f.FilePath,
			FileName: f.FileName,
			MachNumber: 0,
			ValidRange: PrbValidRange{
				AlphaMin: f.ValidRange.AlphaMin,
				AlphaMax: f.ValidRange.AlphaMax,
				BetaMin:  f.ValidRange.BetaMin,
				BetaMax:  f.ValidRange.BetaMax,
			},
		})
	}
	for i, ma := range result.MachNumbers {
		if i < len(a.prbFiles) {
			a.prbFiles[i].MachNumber = ma
		}
	}

	minMa, maxMa := interpolator.GetMachRange()
	machRange := []float64{minMa, maxMa}

	return GenericResponse{Success: true}, &LoadPrbResult{
		Files:     a.prbFiles,
		MachRange: machRange,
		Warnings:  result.Warnings,
	}
}

func (a *App) IsPrbLoaded() bool {
	return a.multiInterp != nil && a.multiInterp.IsLoaded()
}

func (a *App) GetPrbFiles() []PrbFileInfo {
	return a.prbFiles
}

func (a *App) GetMachRange() (GenericResponse, []float64) {
	if !a.IsPrbLoaded() {
		return GenericResponse{Success: false, Error: "请先加载PRB文件"}, nil
	}
	min, max := a.multiInterp.GetMachRange()
	return GenericResponse{Success: true}, []float64{min, max}
}

func (a *App) Calculate(input InterpolationInput) (GenericResponse, *InterpolationResult) {
	if !a.IsPrbLoaded() {
		return GenericResponse{Success: false, Error: "请先加载PRB文件"}, nil
	}

	coreInput := wind_interp.InterpolationInput{
		P1: input.P1, P2: input.P2, P3: input.P3,
		P4: input.P4, P5: input.P5,
		PAtm: input.Patm, TAtm: input.Tatm,
	}

	result, err := a.multiInterp.Calculate(coreInput)
	if err != nil {
		return GenericResponse{Success: false, Error: err.Error()}, nil
	}

	return GenericResponse{Success: true}, toAppResult(result)
}

func (a *App) BatchCalculate(datas []InterpolationInput) (GenericResponse, []*InterpolationResult) {
	if !a.IsPrbLoaded() {
		return GenericResponse{Success: false, Error: "请先加载PRB文件"}, nil
	}

	results := make([]*InterpolationResult, 0, len(datas))
	for i, input := range datas {
		coreInput := wind_interp.InterpolationInput{
			P1: input.P1, P2: input.P2, P3: input.P3,
			P4: input.P4, P5: input.P5,
			PAtm: input.Patm, TAtm: input.Tatm,
		}
		result, err := a.multiInterp.Calculate(coreInput)
		if err != nil {
			return GenericResponse{Success: false, Error: fmt.Sprintf("第%d行计算失败: %s", i+1, err.Error())}, nil
		}
		results = append(results, toAppResult(result))
	}

	return GenericResponse{Success: true}, results
}

func (a *App) ImportCsvData() (GenericResponse, []InterpolationInput) {
	filePath, err := runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "选择数据 CSV 文件",
		Filters: []runtime.FileFilter{
			{DisplayName: "CSV Files (*.csv)", Pattern: "*.csv"},
			{DisplayName: "All Files (*.*)", Pattern: "*.*"},
		},
	})
	if err != nil {
		return GenericResponse{Success: false, Error: err.Error()}, nil
	}
	if filePath == "" {
		return GenericResponse{Success: false, Error: "已取消选择"}, nil
	}

	file, err := os.Open(filePath)
	if err != nil {
		return GenericResponse{Success: false, Error: fmt.Sprintf("打开文件失败: %s", err.Error())}, nil
	}
	defer file.Close()

	reader := csv.NewReader(file)
	records, err := reader.ReadAll()
	if err != nil {
		return GenericResponse{Success: false, Error: fmt.Sprintf("读取CSV失败: %s", err.Error())}, nil
	}

	if len(records) < 2 {
		return GenericResponse{Success: false, Error: "CSV文件为空或只有表头"}, nil
	}

	header := records[0]
	colMap := make(map[string]int)
	for i, col := range header {
		colMap[strings.TrimSpace(col)] = i
	}

	required := []string{"P1", "P2", "P3", "P4", "P5"}
	for _, name := range required {
		if _, ok := colMap[name]; !ok {
			return GenericResponse{Success: false, Error: fmt.Sprintf("缺少必要列: %s", name)}, nil
		}
	}

	patmIdx, hasPatm := colMap["Patm"]
	tatmIdx, hasTatm := colMap["Tatm"]

	datas := make([]InterpolationInput, 0, len(records)-1)
	for rowIdx := 1; rowIdx < len(records); rowIdx++ {
		row := records[rowIdx]
		if len(row) < len(header) {
			continue
		}

		parse := func(colIdx int) float64 {
			val, _ := strconv.ParseFloat(strings.TrimSpace(row[colIdx]), 64)
			return val
		}

		input := InterpolationInput{
			P1: parse(colMap["P1"]), P2: parse(colMap["P2"]),
			P3: parse(colMap["P3"]), P4: parse(colMap["P4"]), P5: parse(colMap["P5"]),
		}
		if hasPatm {
			input.Patm = parse(patmIdx)
		}
		if hasTatm {
			input.Tatm = parse(tatmIdx)
		}
		datas = append(datas, input)
	}

	return GenericResponse{Success: true}, datas
}

func toAppResult(r wind_interp.InterpolationResult) *InterpolationResult {
	return &InterpolationResult{
		Alpha: r.Alpha, Beta: r.Beta,
		MachNumber: r.MachNumber, Velocity: r.Velocity,
		CAS: r.CAS, SAT: r.SAT,
		DynamicPressure: r.DynamicPressure, Density: r.Density,
		TotalPressure: r.TotalPressure, StaticPressure: r.StaticPressure,
		IsValid: r.IsValid, Warning: r.Warning,
	}
}

// OpenHelpDoc 打开用户说明书HTML文件
func (a *App) OpenHelpDoc() error {
	helpPath := getHelpDocPath()
	if helpPath == "" {
		return fmt.Errorf("未找到用户说明书文件")
	}

	// 使用系统默认浏览器打开HTML文件
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("cmd", "/c", "start", "", helpPath)
	case "darwin":
		cmd = exec.Command("open", helpPath)
	default: // linux
		cmd = exec.Command("xdg-open", helpPath)
	}
	return cmd.Start()
}
