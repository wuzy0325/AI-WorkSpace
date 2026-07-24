// probe_handler.go 把 backend.App 的所有方法包装成 HTTP endpoint。
//
// 设计要点：
//   - 保持原 Wails binding 的返回结构（LoadPrbResponse/CalculateResponse 等含 success/error/data），
//     前端 adapter 仅需把 WailsApp.XXX() 替换为 fetch 调用，错误处理逻辑不变。
//   - 对于 bool/string/void 返回值（IsPrbLoaded/GetActiveProbe/OpenHelpDoc 等），
//     包装为简单 JSON 对象（{isLoaded: bool} / {kind: string} / {}），前端 adapter 按字段读取。
//   - 业务失败（Response.Success=false）仍返回 200 + 原Response结构，
//     让前端按 resp.success 判断（与原 Wails binding 行为一致）。
//   - 参数解析失败返回 400 + {error: "..."}。
//
// 文件选择对话框由前端 Electron IPC 处理，后端 LoadPrb/ImportCsv 接收文件路径参数。

package httpserver

import (
	"net/http"

	"probe-interpolator/backend"
)

// ==================== 请求体类型 ====================

// filePathsRequest 用于 LoadPrbFiles（多选文件路径）。
type filePathsRequest struct {
	FilePaths []string `json:"filePaths"`
}

// filePathRequest 用于 ImportCsvData（单选文件路径）。
type filePathRequest struct {
	FilePath string `json:"filePath"`
}

// probeKindRequest 用于 SetActiveProbe。
type probeKindRequest struct {
	Kind string `json:"kind"`
}

// fiveHoleCalculateRequest 包装 5 孔单点计算输入。
// 直接用 FiveHoleInterpolationInput 作为请求体也可，但包一层更清晰。
type fiveHoleCalculateRequest = backend.FiveHoleInterpolationInput

// fiveHoleBatchRequest 包装 5 孔批量计算输入数组。
type fiveHoleBatchRequest struct {
	Datas []backend.FiveHoleInterpolationInput `json:"datas"`
}

// threeHoleCalculateRequest = backend.ThreeHoleInterpolationInput
type threeHoleCalculateRequest = backend.ThreeHoleInterpolationInput

// threeHoleBatchRequest 包装 3 孔批量计算输入数组。
type threeHoleBatchRequest struct {
	Datas []backend.ThreeHoleInterpolationInput `json:"datas"`
}

// sevenHoleCalculateRequest = backend.SevenHoleInterpolationInput
type sevenHoleCalculateRequest = backend.SevenHoleInterpolationInput

// sevenHoleBatchRequest 包装 7 孔批量计算输入数组。
type sevenHoleBatchRequest struct {
	Inputs []backend.SevenHoleInterpolationInput `json:"inputs"`
}

// ==================== 通用响应包装 ====================

// isLoadedResponse 包装 bool 返回值为 JSON 对象（前端按 .isLoaded 读取）。
type isLoadedResponse struct {
	IsLoaded bool `json:"isLoaded"`
}

// probeKindResponse 包装 ProbeKind 返回值为 JSON 对象（前端按 .kind 读取）。
type probeKindResponse struct {
	Kind string `json:"kind"`
}

// errorResponse 包装 error 返回值为 JSON 对象（前端按 .error 读取，空字符串表示成功）。
type errorResponse struct {
	Error string `json:"error,omitempty"`
}

// ==================== Probe selector handlers ====================

// handleProbeAvailable GET /api/probe/available → []ProbeInfo
func (s *Server) handleProbeAvailable(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	writeOK(w, s.app.GetAvailableProbes())
}

// handleProbeActive GET /api/probe/active → {kind: string}
// handleProbeActive POST /api/probe/active {kind: string} → {}
func (s *Server) handleProbeActive(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		kind, _ := s.app.GetActiveProbe()
		writeOK(w, probeKindResponse{Kind: string(kind)})
	case http.MethodPost:
		var body probeKindRequest
		if !decodeJSON(w, r, &body) {
			return
		}
		if err := s.app.SetActiveProbe(backend.ProbeKind(body.Kind)); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeOK(w, nil)
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// handleProbeClear POST /api/probe/clear → {}
func (s *Server) handleProbeClear(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	s.app.ClearActiveProbe()
	writeOK(w, nil)
}

// ==================== 5 孔 handlers ====================

// handleFiveLoadPrb POST /api/five/load-prb {filePaths: []string} → LoadPrbResponse
func (s *Server) handleFiveLoadPrb(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var body filePathsRequest
	if !decodeJSON(w, r, &body) {
		return
	}
	writeOK(w, s.app.LoadPrbFiles(body.FilePaths))
}

// handleFiveIsLoaded GET /api/five/is-loaded → {isLoaded: bool}
func (s *Server) handleFiveIsLoaded(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	writeOK(w, isLoadedResponse{IsLoaded: s.app.IsPrbLoaded()})
}

// handleFivePrbFiles GET /api/five/prb-files → []PrbFileInfo
func (s *Server) handleFivePrbFiles(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	writeOK(w, s.app.GetPrbFiles())
}

// handleFiveMachRange GET /api/five/mach-range → MachRangeResponse
func (s *Server) handleFiveMachRange(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	writeOK(w, s.app.GetMachRange())
}

// handleFiveCalculate POST /api/five/calculate {FiveHoleInterpolationInput} → CalculateResponse
func (s *Server) handleFiveCalculate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var body fiveHoleCalculateRequest
	if !decodeJSON(w, r, &body) {
		return
	}
	writeOK(w, s.app.Calculate(body))
}

// handleFiveBatchCalculate POST /api/five/batch-calculate {datas: []FiveHoleInterpolationInput} → BatchCalculateResponse
func (s *Server) handleFiveBatchCalculate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var body fiveHoleBatchRequest
	if !decodeJSON(w, r, &body) {
		return
	}
	writeOK(w, s.app.BatchCalculate(body.Datas))
}

// handleFiveImportCsv POST /api/five/import-csv {filePath: string} → ImportCsvDataResponse
func (s *Server) handleFiveImportCsv(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var body filePathRequest
	if !decodeJSON(w, r, &body) {
		return
	}
	writeOK(w, s.app.ImportCsvData(body.FilePath))
}

// handleFiveHelpDoc POST /api/five/help-doc → {} 或 {error: string}
func (s *Server) handleFiveHelpDoc(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if err := s.app.OpenHelpDoc(); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeOK(w, nil)
}

// ==================== 3 孔 handlers ====================

// handleThreeLoadPrb POST /api/three/load-prb {filePaths: []string} → ThreeHoleLoadPrbResponse
func (s *Server) handleThreeLoadPrb(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var body filePathsRequest
	if !decodeJSON(w, r, &body) {
		return
	}
	writeOK(w, s.app.LoadThreeHolePrbFiles(body.FilePaths))
}

// handleThreeIsLoaded GET /api/three/is-loaded → {isLoaded: bool}
func (s *Server) handleThreeIsLoaded(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	writeOK(w, isLoadedResponse{IsLoaded: s.app.IsThreeHolePrbLoaded()})
}

// handleThreePrbFiles GET /api/three/prb-files → []ThreeHolePrbFileInfo
func (s *Server) handleThreePrbFiles(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	writeOK(w, s.app.GetThreeHolePrbFiles())
}

// handleThreeMachRange GET /api/three/mach-range → ThreeHoleMachRangeResponse
func (s *Server) handleThreeMachRange(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	writeOK(w, s.app.GetThreeHoleMachRange())
}

// handleThreeCalculate POST /api/three/calculate {ThreeHoleInterpolationInput} → ThreeHoleCalculateResponse
func (s *Server) handleThreeCalculate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var body threeHoleCalculateRequest
	if !decodeJSON(w, r, &body) {
		return
	}
	writeOK(w, s.app.CalculateThreeHole(body))
}

// handleThreeBatchCalculate POST /api/three/batch-calculate {datas: []ThreeHoleInterpolationInput} → ThreeHoleBatchCalculateResponse
func (s *Server) handleThreeBatchCalculate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var body threeHoleBatchRequest
	if !decodeJSON(w, r, &body) {
		return
	}
	writeOK(w, s.app.BatchCalculateThreeHole(body.Datas))
}

// handleThreeImportCsv POST /api/three/import-csv {filePath: string} → ThreeHoleImportCsvDataResponse
func (s *Server) handleThreeImportCsv(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var body filePathRequest
	if !decodeJSON(w, r, &body) {
		return
	}
	writeOK(w, s.app.ImportThreeHoleCsvData(body.FilePath))
}

// handleThreeHelpDoc POST /api/three/help-doc → {} 或 {error: string}
func (s *Server) handleThreeHelpDoc(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if err := s.app.OpenThreeHoleHelpDoc(); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeOK(w, nil)
}

// ==================== 7 孔 handlers ====================

// handleSevenLoadPrb POST /api/seven/load-prb {filePaths: []string} → SevenHoleLoadPrbResponse
func (s *Server) handleSevenLoadPrb(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var body filePathsRequest
	if !decodeJSON(w, r, &body) {
		return
	}
	writeOK(w, s.app.LoadSevenHolePrbFiles(body.FilePaths))
}

// handleSevenIsLoaded GET /api/seven/is-loaded → {isLoaded: bool}
func (s *Server) handleSevenIsLoaded(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	writeOK(w, isLoadedResponse{IsLoaded: s.app.IsSevenHolePrbLoaded()})
}

// handleSevenPrbFiles GET /api/seven/prb-files → []SevenHolePrbFileInfo
func (s *Server) handleSevenPrbFiles(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	writeOK(w, s.app.GetSevenHolePrbFiles())
}

// handleSevenValidRange GET /api/seven/valid-range → SevenHoleValidRangeResponse
func (s *Server) handleSevenValidRange(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	writeOK(w, s.app.GetSevenHoleValidRange())
}

// handleSevenCalculate POST /api/seven/calculate {SevenHoleInterpolationInput} → SevenHoleCalculateResponse
func (s *Server) handleSevenCalculate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var body sevenHoleCalculateRequest
	if !decodeJSON(w, r, &body) {
		return
	}
	writeOK(w, s.app.CalculateSevenHole(body))
}

// handleSevenBatchCalculate POST /api/seven/batch-calculate {inputs: []SevenHoleInterpolationInput} → SevenHoleBatchCalculateResponse
func (s *Server) handleSevenBatchCalculate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var body sevenHoleBatchRequest
	if !decodeJSON(w, r, &body) {
		return
	}
	writeOK(w, s.app.BatchCalculateSevenHole(body.Inputs))
}

// handleSevenImportCsv POST /api/seven/import-csv {filePath: string} → SevenHoleImportCsvDataResponse
func (s *Server) handleSevenImportCsv(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var body filePathRequest
	if !decodeJSON(w, r, &body) {
		return
	}
	writeOK(w, s.app.ImportSevenHoleCsvData(body.FilePath))
}

// handleSevenHelpDoc POST /api/seven/help-doc → {} 或 {error: string}
func (s *Server) handleSevenHelpDoc(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if err := s.app.OpenSevenHoleHelpDoc(); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeOK(w, nil)
}
