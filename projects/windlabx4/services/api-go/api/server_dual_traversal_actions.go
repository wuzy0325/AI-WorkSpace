package api

import (
	"encoding/json"
	"net/http"

	"windlabx4/services/api-go/internal/core/traversal"
	"windlabx4/services/api-go/internal/usecase"
)

// ---------------------------------------------------------------------------
// 非生命周期共享 action（GetOrCreate 选择 manager 后委托）
// ---------------------------------------------------------------------------

// sharedManagerFor 经 registry 选择 manager 并断言共享操作能力。
func sharedManagerFor(w http.ResponseWriter, deps Deps, probeID usecase.ProbeID) (traversalSharedManager, bool) {
	manager, err := deps.TraversalRegistry.GetOrCreate(probeID)
	if err != nil {
		writeRegistryError(w, err)
		return nil, false
	}
	shared, ok := manager.(traversalSharedManager)
	if !ok {
		writeError(w, http.StatusServiceUnavailable, "manager_creation_failed: probe manager 不支持共享操作")
		return nil, false
	}
	return shared, true
}

func dualConfig(w http.ResponseWriter, r *http.Request, deps Deps, probeID usecase.ProbeID) {
	manager, ok := sharedManagerFor(w, deps, probeID)
	if !ok {
		return
	}
	switch r.Method {
	case http.MethodGet:
		raw := manager.GetConfigRaw()
		if len(raw) == 0 {
			writeJSON(w, http.StatusOK, nil)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(raw)
	case http.MethodPost:
		var raw json.RawMessage
		if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		manager.SaveConfigRaw(raw)
		writeJSON(w, http.StatusOK, map[string]bool{"success": true})
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func dualStatus(w http.ResponseWriter, r *http.Request, deps Deps, probeID usecase.ProbeID) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	manager, ok := sharedManagerFor(w, deps, probeID)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, manager.BuildStatusResponse())
}

func dualResult(w http.ResponseWriter, r *http.Request, deps Deps, probeID usecase.ProbeID) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	taskID := r.URL.Query().Get("taskId")
	if taskID == "" {
		writeError(w, http.StatusBadRequest, "taskId query parameter is required")
		return
	}
	manager, ok := sharedManagerFor(w, deps, probeID)
	if !ok {
		return
	}
	result, found := manager.GetResult(taskID)
	if !found {
		writeError(w, http.StatusNotFound, "traversal result not found")
		return
	}
	writeJSON(w, http.StatusOK, result)
}

// importFileBody 单文件导入请求体。
type importFileBody struct {
	FilePath string `json:"filePath"`
}

// dualImport 单文件导入（importPrb / importCalibrationCsv）。
func dualImport(w http.ResponseWriter, r *http.Request, deps Deps, probeID usecase.ProbeID,
	call func(traversalSharedManager, importFileBody) (any, error)) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var body importFileBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	manager, ok := sharedManagerFor(w, deps, probeID)
	if !ok {
		return
	}
	res, err := call(manager, body)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func dualImportMultiPrb(w http.ResponseWriter, r *http.Request, deps Deps, probeID usecase.ProbeID) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		FilePaths         []string  `json:"filePaths"`
		MachNumbers       []float64 `json:"machNumbers"`
		InterpolationMode string    `json:"interpolationMode"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	manager, ok := sharedManagerFor(w, deps, probeID)
	if !ok {
		return
	}
	res, err := manager.ImportMultiPRB(body.FilePaths, body.MachNumbers, body.InterpolationMode)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, res)
}

// dualImportSevenHole 七孔 PRB/校准 CSV 导入（1 内区 + 6 扇区路径）。
func dualImportSevenHole(w http.ResponseWriter, r *http.Request, deps Deps, probeID usecase.ProbeID, calibration bool) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		InnerFilePath  string   `json:"innerFilePath"`
		OuterFilePaths []string `json:"outerFilePaths"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	manager, ok := sharedManagerFor(w, deps, probeID)
	if !ok {
		return
	}
	var res *usecase.SevenHoleImportResult
	var err error
	if calibration {
		res, err = manager.ImportSevenHoleCalibrationCSV(body.InnerFilePath, body.OuterFilePaths)
	} else {
		res, err = manager.ImportSevenHolePRB(body.InnerFilePath, body.OuterFilePaths)
	}
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func dualClearInterpolator(w http.ResponseWriter, r *http.Request, deps Deps, probeID usecase.ProbeID) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		ProbeType string `json:"probeType"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if body.ProbeType == "" {
		writeError(w, http.StatusBadRequest, "probeType 必填（five-hole / seven-hole）")
		return
	}
	manager, ok := sharedManagerFor(w, deps, probeID)
	if !ok {
		return
	}
	if err := manager.ClearProbeInterpolator(body.ProbeType); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"cleared": true})
}

func dualCalculateRealtime(w http.ResponseWriter, r *http.Request, deps Deps, probeID usecase.ProbeID) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		ProbeType string `json:"probeType"`
		Pressures struct {
			P1   float64  `json:"P1"`
			P2   float64  `json:"P2"`
			P3   float64  `json:"P3"`
			P4   float64  `json:"P4"`
			P5   float64  `json:"P5"`
			P6   *float64 `json:"P6"`
			P7   *float64 `json:"P7"`
			PAtm float64  `json:"Patm"`
			TAtm float64  `json:"Tatm"`
		} `json:"pressures"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	manager, ok := sharedManagerFor(w, deps, probeID)
	if !ok {
		return
	}
	result, err := manager.CalculateRealtimeForAPI(body.ProbeType, usecase.ProbePressureInput{
		P1:   body.Pressures.P1,
		P2:   body.Pressures.P2,
		P3:   body.Pressures.P3,
		P4:   body.Pressures.P4,
		P5:   body.Pressures.P5,
		P6:   body.Pressures.P6,
		P7:   body.Pressures.P7,
		PAtm: body.Pressures.PAtm,
		TAtm: body.Pressures.TAtm,
	})
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func dualCheckPreconditions(w http.ResponseWriter, r *http.Request, deps Deps, probeID usecase.ProbeID) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		Config json.RawMessage `json:"config"`
	}
	// 解析失败不阻断：无 config 时回退到 manager 当前配置（与 legacy 同语义）
	_ = json.NewDecoder(r.Body).Decode(&body)
	manager, ok := sharedManagerFor(w, deps, probeID)
	if !ok {
		return
	}
	var config *traversal.Config
	if len(body.Config) > 0 {
		cfg, err := manager.ParseConfig(body.Config)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		config = &cfg
	}
	writeJSON(w, http.StatusOK, manager.CheckPreconditions(config))
}
