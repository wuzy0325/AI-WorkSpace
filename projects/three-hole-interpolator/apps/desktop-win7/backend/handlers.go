package backend

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	three_interp "ai-workspace/shared/algorithms/go/threehole/interpolation"
)

func (s *Server) handleLoadPrb(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, LoadPrbResponse{Success: false, Error: "仅支持POST方法"})
		return
	}

	if err := r.ParseMultipartForm(64 << 20); err != nil {
		writeJSON(w, http.StatusBadRequest, LoadPrbResponse{Success: false, Error: "解析上传文件失败"})
		return
	}

	files := r.MultipartForm.File["files"]
	if len(files) == 0 {
		writeJSON(w, http.StatusBadRequest, LoadPrbResponse{Success: false, Error: "未选择文件"})
		return
	}

	fileData := make([]three_interp.PrbFileData, 0, len(files))
	for _, fh := range files {
		f, err := fh.Open()
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, LoadPrbResponse{
				Success: false, Error: fmt.Sprintf("打开 %s 失败", fh.Filename),
			})
			return
		}
		lines, err := readLinesFromReader(f)
		f.Close()
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, LoadPrbResponse{
				Success: false, Error: fmt.Sprintf("读取 %s 失败: %s", fh.Filename, err.Error()),
			})
			return
		}
		fileData = append(fileData, three_interp.PrbFileData{
			FilePath: fh.Filename,
			Lines:    lines,
		})
	}

	interpolator := three_interp.NewThreeHoleInterpolator()
	result, err := interpolator.LoadPrbData(fileData)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, LoadPrbResponse{Success: false, Error: err.Error()})
		return
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

	s.mu.Lock()
	s.multiInterp = interpolator
	s.prbFiles = prbFiles
	s.mu.Unlock()

	writeJSON(w, http.StatusOK, LoadPrbResponse{
		Success: true,
		Data: &LoadPrbResult{
			Files:     prbFiles,
			MachRange: machRange,
			Warnings:  result.Warnings,
		},
	})
}

func (s *Server) handleIsPrbLoaded(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, GenericResponse{Success: false, Error: "仅支持GET方法"})
		return
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	loaded := s.multiInterp != nil && s.multiInterp.IsLoaded()
	writeJSON(w, http.StatusOK, map[string]bool{"loaded": loaded})
}

func (s *Server) handleGetPrbFiles(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, GenericResponse{Success: false, Error: "仅支持GET方法"})
		return
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	writeJSON(w, http.StatusOK, map[string]interface{}{"files": s.prbFiles})
}

func (s *Server) handleGetMachRange(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, MachRangeResponse{Success: false, Error: "仅支持GET方法"})
		return
	}
	s.mu.RLock()
	interpolator := s.multiInterp
	s.mu.RUnlock()

	if interpolator == nil || !interpolator.IsLoaded() {
		writeJSON(w, http.StatusBadRequest, MachRangeResponse{Success: false, Error: "请先加载PRB文件"})
		return
	}

	min, max := interpolator.GetMachRange()
	writeJSON(w, http.StatusOK, MachRangeResponse{Success: true, Data: []float64{min, max}})
}

func (s *Server) handleCalculate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, CalculateResponse{Success: false, Error: "仅支持POST方法"})
		return
	}

	var input InterpolationInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeJSON(w, http.StatusBadRequest, CalculateResponse{Success: false, Error: "请求数据格式错误"})
		return
	}

	s.mu.RLock()
	interpolator := s.multiInterp
	s.mu.RUnlock()

	if interpolator == nil || !interpolator.IsLoaded() {
		writeJSON(w, http.StatusBadRequest, CalculateResponse{Success: false, Error: "请先加载PRB文件"})
		return
	}

	coreInput := toCoreInput(input)
	result, err := interpolator.Calculate(coreInput)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, CalculateResponse{Success: false, Error: err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, CalculateResponse{Success: true, Data: toAppResult(result)})
}

func (s *Server) handleBatchCalculate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, BatchCalculateResponse{Success: false, Error: "仅支持POST方法"})
		return
	}

	var datas []InterpolationInput
	if err := json.NewDecoder(r.Body).Decode(&datas); err != nil {
		writeJSON(w, http.StatusBadRequest, BatchCalculateResponse{Success: false, Error: "请求数据格式错误"})
		return
	}

	s.mu.RLock()
	interpolator := s.multiInterp
	s.mu.RUnlock()

	if interpolator == nil || !interpolator.IsLoaded() {
		writeJSON(w, http.StatusBadRequest, BatchCalculateResponse{Success: false, Error: "请先加载PRB文件"})
		return
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

	writeJSON(w, http.StatusOK, BatchCalculateResponse{
		Success: firstError == "",
		Error:   firstError,
		Data:    results,
	})
}

func (s *Server) handleImportCsv(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, ImportCsvDataResponse{Success: false, Error: "仅支持POST方法"})
		return
	}

	file, _, err := r.FormFile("file")
	if err != nil {
		writeJSON(w, http.StatusBadRequest, ImportCsvDataResponse{Success: false, Error: "未找到上传文件"})
		return
	}
	defer file.Close()

	records, err := readCsvFromReader(file)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, ImportCsvDataResponse{Success: false, Error: err.Error()})
		return
	}

	colMap, csvErr := parseCsvHeader(records[0])
	if csvErr != "" {
		writeJSON(w, http.StatusBadRequest, ImportCsvDataResponse{Success: false, Error: csvErr})
		return
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
		writeJSON(w, http.StatusBadRequest, ImportCsvDataResponse{Success: false, Error: errMsg})
		return
	}

	writeJSON(w, http.StatusOK, ImportCsvDataResponse{Success: true, Data: datas})
}

func (s *Server) handleHelp(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, GenericResponse{Success: false, Error: "仅支持GET方法"})
		return
	}
	helpPath := getHelpDocPath()
	if helpPath == "" {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte("<html><body><h2>未找到用户说明书</h2><p>请确保 docs 目录下存在 用户说明书.html 文件</p></body></html>"))
		return
	}
	http.ServeFile(w, r, helpPath)
}

// ==================== 工具函数 ====================

func readLinesFromReader(r io.Reader) ([]string, error) {
	scanner := bufio.NewScanner(r)
	var lines []string
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" {
			lines = append(lines, line)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return lines, nil
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

const helpDocFileName = "用户说明书.html"

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
		cleanPath := filepath.Clean(p)
		if info, err := os.Stat(cleanPath); err == nil && !info.IsDir() {
			return cleanPath
		}
	}
	return ""
}

func readCsvFromReader(r io.Reader) ([][]string, error) {
	scanner := bufio.NewScanner(r)
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

	requiredCols := []string{"P1", "P2", "P3", "Patm", "Tatm"}
	for _, name := range requiredCols {
		if _, ok := colMap[name]; !ok {
			return nil, fmt.Sprintf("缺少必要列: %s", name)
		}
	}

	return colMap, ""
}

func parseCsvRows(rows [][]string, colMap map[string]int) ([]InterpolationInput, []string) {
	colCount := len(colMap)
	patmIdx := colMap["Patm"]
	tatmIdx := colMap["Tatm"]

	datas := make([]InterpolationInput, 0, len(rows))
	var warnings []string

	for rowIdx := 0; rowIdx < len(rows); rowIdx++ {
		row := rows[rowIdx]
		csvLine := rowIdx + 2

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

		input := InterpolationInput{
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
