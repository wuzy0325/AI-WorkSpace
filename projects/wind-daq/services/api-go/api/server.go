package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	coreinterp "ai-workspace/shared/algorithms/go/fivehole/interpolation"
	motionhttp "shared.local/motion-control/go/httpapi"
	interpfiles "wind-daq/services/api-go/internal/adapters/interpolation"
	"wind-daq/services/api-go/internal/core/calibration"
	"wind-daq/services/api-go/internal/core/device"
	wind_report "wind-daq/services/api-go/internal/core/report"
	"wind-daq/services/api-go/internal/core/storage"
	"wind-daq/services/api-go/internal/core/traversal"
	"wind-daq/services/api-go/internal/ports"
	"wind-daq/services/api-go/internal/usecase"
)

type Deps struct {
	DeviceManager      *usecase.DeviceManager
	AcquisitionHub     *usecase.AcquisitionHub
	ReportManager      *usecase.ReportManager
	MotionManager      ports.MotionManager
	MotionService      motionhttp.MotionService
	CalibrationManager *usecase.CalibrationManager
	TraversalManager   *usecase.TraversalManager
	StorageRecorder    *usecase.StorageRecorder
	ConfigManager      *usecase.ConfigManager
}

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func NewRouter(deps Deps) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/device/profiles", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			writeJSON(w, http.StatusOK, deps.DeviceManager.GetProfiles())
		case http.MethodPut:
			var profile device.Profile
			if err := json.NewDecoder(r.Body).Decode(&profile); err != nil {
				writeError(w, http.StatusBadRequest, err.Error())
				return
			}
			if err := deps.DeviceManager.UpsertProfile(profile); err != nil {
				writeError(w, http.StatusBadRequest, err.Error())
				return
			}
			writeJSON(w, http.StatusOK, map[string]bool{"success": true})
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	})
	mux.HandleFunc("/api/device/scan", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		results, err := deps.DeviceManager.ScanDevices()
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, results)
	})
	mux.HandleFunc("/api/device/profiles/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		id := strings.TrimPrefix(r.URL.Path, "/api/device/profiles/")
		if id == "" {
			writeError(w, http.StatusNotFound, "not found")
			return
		}
		if err := deps.DeviceManager.DeleteProfile(id); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]bool{"success": true})
	})
	mux.HandleFunc("/api/device/", func(w http.ResponseWriter, r *http.Request) {
		handleDeviceByID(w, r, deps)
	})
	mux.HandleFunc("/api/daq/latest/", func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimPrefix(r.URL.Path, "/api/daq/latest/")
		payload, ok := deps.AcquisitionHub.GetLatestData(id)
		if !ok {
			writeJSON(w, http.StatusOK, device.DataPayload{DeviceID: id})
			return
		}
		writeJSON(w, http.StatusOK, payload)
	})
	mux.HandleFunc("/api/daq/stream/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		id := strings.TrimPrefix(r.URL.Path, "/api/daq/stream/")
		if id == "" {
			writeError(w, http.StatusNotFound, "not found")
			return
		}
		handleDaqStream(w, r, deps.AcquisitionHub, id)
	})
	mux.HandleFunc("/api/daq/publishRate", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			writeJSON(w, http.StatusOK, map[string]float64{"hz": deps.AcquisitionHub.PublishRate()})
		case http.MethodPut:
			var body struct {
				Hz float64 `json:"hz"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				writeError(w, http.StatusBadRequest, err.Error())
				return
			}
			if err := deps.AcquisitionHub.SetPublishRate(body.Hz); err != nil {
				writeError(w, http.StatusBadRequest, err.Error())
				return
			}
			writeJSON(w, http.StatusOK, map[string]bool{"success": true})
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	})
	mux.HandleFunc("/api/report/generate", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		var cfg struct {
			OutputDir  string `json:"outputDir"`
			FilePrefix string `json:"filePrefix"`
			DeviceID   string `json:"deviceId"`
		}
		if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		result, err := deps.ReportManager.Generate(wind_report.ReportConfig{OutputDir: cfg.OutputDir, FilePrefix: cfg.FilePrefix, DeviceID: cfg.DeviceID})
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, result)
	})
	mux.HandleFunc("/api/report/status", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		writeJSON(w, http.StatusOK, deps.ReportManager.Status())
	})

	// ---- Motion API ----
	if deps.MotionService != nil {
		motionhttp.RegisterMotionRoutes(mux, deps.MotionService)
	}

	// ---- Calibration API ----
	mux.HandleFunc("/api/calibration/", func(w http.ResponseWriter, r *http.Request) {
		action := strings.TrimPrefix(r.URL.Path, "/api/calibration/")
		switch action {
		case "start":
			if r.Method != http.MethodPost {
				w.WriteHeader(http.StatusMethodNotAllowed)
				return
			}
			var body struct {
				TaskID         string    `json:"taskId"`
				DeviceID       string    `json:"deviceId"`
				Type           string    `json:"type"`
				Channels       []int     `json:"channels"`
				PressurePoints []float64 `json:"pressurePoints"`
				AverageSamples int       `json:"averageSamples"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				writeError(w, http.StatusBadRequest, err.Error())
				return
			}
			err := deps.CalibrationManager.Start(calibration.Config{
				TaskID: body.TaskID, DeviceID: body.DeviceID, Type: body.Type,
				Channels: body.Channels, PressurePoints: body.PressurePoints,
				AverageSamples: body.AverageSamples,
			})
			if err != nil {
				writeError(w, http.StatusBadRequest, err.Error())
				return
			}
			writeJSON(w, http.StatusOK, map[string]bool{"success": true})
		case "status":
			if r.Method != http.MethodGet {
				w.WriteHeader(http.StatusMethodNotAllowed)
				return
			}
			writeJSON(w, http.StatusOK, deps.CalibrationManager.Status())
		case "collect":
			if r.Method != http.MethodPost {
				w.WriteHeader(http.StatusMethodNotAllowed)
				return
			}
			if err := deps.CalibrationManager.CollectCurrentPoint(); err != nil {
				writeError(w, http.StatusBadRequest, err.Error())
				return
			}
			writeJSON(w, http.StatusOK, map[string]bool{"success": true})
		case "result":
			if r.Method != http.MethodGet {
				w.WriteHeader(http.StatusMethodNotAllowed)
				return
			}
			taskID := r.URL.Query().Get("taskId")
			if taskID == "" {
				writeError(w, http.StatusBadRequest, "taskId query parameter is required")
				return
			}
			result, ok := deps.CalibrationManager.GetResult(taskID)
			if !ok {
				writeError(w, http.StatusNotFound, "calibration result not found")
				return
			}
			writeJSON(w, http.StatusOK, result)
		case "precisionDefaults":
			if r.Method != http.MethodGet {
				w.WriteHeader(http.StatusMethodNotAllowed)
				return
			}
			writeJSON(w, http.StatusOK, map[string]int{
				"probePrecision":    3,
				"machPrecision":     4,
				"velocityPrecision": 3,
			})
		case "fivehole":
			handleFiveholeSnakePoints(w, r)
		case "pause":
			if r.Method != http.MethodPost {
				w.WriteHeader(http.StatusMethodNotAllowed)
				return
			}
			if err := deps.CalibrationManager.Pause(); err != nil {
				writeError(w, http.StatusBadRequest, err.Error())
				return
			}
			writeJSON(w, http.StatusOK, map[string]bool{"success": true})
		case "resume":
			if r.Method != http.MethodPost {
				w.WriteHeader(http.StatusMethodNotAllowed)
				return
			}
			if err := deps.CalibrationManager.Resume(); err != nil {
				writeError(w, http.StatusBadRequest, err.Error())
				return
			}
			writeJSON(w, http.StatusOK, map[string]bool{"success": true})
		case "stop":
			if r.Method != http.MethodPost {
				w.WriteHeader(http.StatusMethodNotAllowed)
				return
			}
			if err := deps.CalibrationManager.Stop(); err != nil {
				writeError(w, http.StatusBadRequest, err.Error())
				return
			}
			writeJSON(w, http.StatusOK, map[string]bool{"success": true})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})

	// ---- Traversal API ----
	mux.HandleFunc("/api/traversal/", func(w http.ResponseWriter, r *http.Request) {
		action := strings.TrimPrefix(r.URL.Path, "/api/traversal/")
		switch action {
		case "config":
			if deps.TraversalManager == nil {
				writeError(w, http.StatusBadRequest, "traversal manager is required")
				return
			}
			switch r.Method {
			case http.MethodGet:
				raw := deps.TraversalManager.GetConfigRaw()
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
				deps.TraversalManager.SaveConfigRaw(raw)
				writeJSON(w, http.StatusOK, map[string]bool{"success": true})
			default:
				w.WriteHeader(http.StatusMethodNotAllowed)
			}
		case "importPrb":
			if r.Method != http.MethodPost {
				w.WriteHeader(http.StatusMethodNotAllowed)
				return
			}
			var body struct {
				FilePath string `json:"filePath"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				writeError(w, http.StatusBadRequest, err.Error())
				return
			}
			interpolator, err := interpfiles.LoadPrbFile(body.FilePath)
			if err != nil {
				writeError(w, http.StatusBadRequest, err.Error())
				return
			}
			deps.TraversalManager.SetInterpolator(interpolator)
			writeJSON(w, http.StatusOK, prbFileInfo(body.FilePath, interpolator.GetValidRange()))
		case "importCalibrationCsv":
			if r.Method != http.MethodPost {
				w.WriteHeader(http.StatusMethodNotAllowed)
				return
			}
			var body struct {
				FilePath string `json:"filePath"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				writeError(w, http.StatusBadRequest, err.Error())
				return
			}
			interpolator, err := interpfiles.LoadFiveHoleNewFile(body.FilePath)
			if err != nil {
				writeError(w, http.StatusBadRequest, err.Error())
				return
			}
			deps.TraversalManager.SetInterpolator(interpolator)
			rangeInfo := interpolator.GetValidRange()
			writeJSON(w, http.StatusOK, map[string]any{
				"filePath":   body.FilePath,
				"fileName":   filepath.Base(body.FilePath),
				"loadedAt":   time.Now().UnixMilli(),
				"validRange": rangeInfo,
				"pointCount": interpolator.GetPointCount(),
			})
		case "importMultiPrb":
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
			interpolator, result, err := interpfiles.LoadMultiPrbFiles(body.FilePaths, body.MachNumbers)
			if err != nil {
				writeError(w, http.StatusBadRequest, err.Error())
				return
			}
			if body.InterpolationMode != "" {
				interpolator.SetInterpolationMode(coreinterp.MultiPrbInterpolationMode(body.InterpolationMode))
			}
			deps.TraversalManager.SetInterpolator(interpolator)
			files := make([]map[string]any, 0, len(result.Files))
			for i, file := range result.Files {
				var machNumber any
				if i < len(result.MachNumbers) {
					machNumber = result.MachNumbers[i]
				}
				files = append(files, map[string]any{
					"filePath":   file.FilePath,
					"fileName":   file.FileName,
					"loadedAt":   time.Now().UnixMilli(),
					"validRange": file.ValidRange,
					"machNumber": machNumber,
				})
			}
			writeJSON(w, http.StatusOK, map[string]any{"files": files, "machNumbers": result.MachNumbers, "warnings": result.Warnings})
		case "calculateRealtime":
			if r.Method != http.MethodPost {
				w.WriteHeader(http.StatusMethodNotAllowed)
				return
			}
			var body struct {
				Pressures coreinterp.InterpolationInput `json:"pressures"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				writeError(w, http.StatusBadRequest, err.Error())
				return
			}
			result, err := deps.TraversalManager.CalculateRealtime(body.Pressures)
			if err != nil {
				writeError(w, http.StatusBadRequest, err.Error())
				return
			}
			writeJSON(w, http.StatusOK, result)
		case "checkPreconditions":
			if r.Method != http.MethodPost {
				w.WriteHeader(http.StatusMethodNotAllowed)
				return
			}
			writeJSON(w, http.StatusOK, deps.TraversalManager.CheckPreconditions())
		case "generateGrid":
			if r.Method != http.MethodPost {
				w.WriteHeader(http.StatusMethodNotAllowed)
				return
			}
			if deps.TraversalManager == nil {
				writeError(w, http.StatusBadRequest, "traversal manager is required")
				return
			}
			var grid traversal.GridConfig
			if err := json.NewDecoder(r.Body).Decode(&grid); err != nil {
				writeError(w, http.StatusBadRequest, err.Error())
				return
			}
			path, err := deps.TraversalManager.GenerateGridPath(grid)
			if err != nil {
				writeError(w, http.StatusBadRequest, err.Error())
				return
			}
			writeJSON(w, http.StatusOK, path)
		case "start":
			if r.Method != http.MethodPost {
				w.WriteHeader(http.StatusMethodNotAllowed)
				return
			}
			var raw json.RawMessage
			if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
				writeError(w, http.StatusBadRequest, err.Error())
				return
			}
			taskID, err := deps.TraversalManager.ParseAndStartTraversal(raw)
			if err != nil {
				writeError(w, http.StatusBadRequest, err.Error())
				return
			}
			writeJSON(w, http.StatusOK, map[string]string{"taskId": taskID})
		case "status":
			if r.Method != http.MethodGet {
				w.WriteHeader(http.StatusMethodNotAllowed)
				return
			}
			writeJSON(w, http.StatusOK, deps.TraversalManager.BuildStatusResponse())
		case "runPoint":
			if r.Method != http.MethodPost {
				w.WriteHeader(http.StatusMethodNotAllowed)
				return
			}
			if err := deps.TraversalManager.RunCurrentPoint(); err != nil {
				writeError(w, http.StatusBadRequest, err.Error())
				return
			}
			writeJSON(w, http.StatusOK, map[string]bool{"success": true})
		case "pause":
			if r.Method != http.MethodPost {
				w.WriteHeader(http.StatusMethodNotAllowed)
				return
			}
			if err := deps.TraversalManager.Pause(); err != nil {
				writeError(w, http.StatusBadRequest, err.Error())
				return
			}
			writeJSON(w, http.StatusOK, map[string]bool{"success": true})
		case "resume":
			if r.Method != http.MethodPost {
				w.WriteHeader(http.StatusMethodNotAllowed)
				return
			}
			if err := deps.TraversalManager.Resume(); err != nil {
				writeError(w, http.StatusBadRequest, err.Error())
				return
			}
			writeJSON(w, http.StatusOK, map[string]bool{"success": true})
		case "stop":
			if r.Method != http.MethodPost {
				w.WriteHeader(http.StatusMethodNotAllowed)
				return
			}
			if err := deps.TraversalManager.Stop(); err != nil {
				writeError(w, http.StatusBadRequest, err.Error())
				return
			}
			writeJSON(w, http.StatusOK, map[string]bool{"success": true})
		case "result":
			if r.Method != http.MethodGet {
				w.WriteHeader(http.StatusMethodNotAllowed)
				return
			}
			taskID := r.URL.Query().Get("taskId")
			if taskID == "" {
				writeError(w, http.StatusBadRequest, "taskId query parameter is required")
				return
			}
			result, ok := deps.TraversalManager.GetResult(taskID)
			if !ok {
				writeError(w, http.StatusNotFound, "traversal result not found")
				return
			}
			writeJSON(w, http.StatusOK, result)
		case "loadCheckpoint":
			// 加载断点恢复信息（前端启动时调用，判断是否需要展示"恢复"横幅）
			if r.Method != http.MethodGet {
				w.WriteHeader(http.StatusMethodNotAllowed)
				return
			}
			checkpoint, err := deps.TraversalManager.LoadCheckpoint()
			if err != nil {
				writeError(w, http.StatusBadRequest, err.Error())
				return
			}
			writeJSON(w, http.StatusOK, checkpoint)
		case "resumeFromCheckpoint":
			// 从断点恢复测试（复用原 taskId，从已完成点数继续）
			if r.Method != http.MethodPost {
				w.WriteHeader(http.StatusMethodNotAllowed)
				return
			}
			var cp traversal.Checkpoint
			if err := json.NewDecoder(r.Body).Decode(&cp); err != nil {
				writeError(w, http.StatusBadRequest, err.Error())
				return
			}
			taskID, err := deps.TraversalManager.ResumeFromCheckpoint(cp)
			if err != nil {
				writeError(w, http.StatusBadRequest, err.Error())
				return
			}
			writeJSON(w, http.StatusOK, map[string]string{"taskId": taskID})
		case "clearCheckpoint":
			// 清除断点文件（用户主动放弃恢复时调用）
			if r.Method != http.MethodPost {
				w.WriteHeader(http.StatusMethodNotAllowed)
				return
			}
			deps.TraversalManager.ClearCheckpoint()
			writeJSON(w, http.StatusOK, map[string]bool{"success": true})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})
	mux.HandleFunc("/api/config/", func(w http.ResponseWriter, r *http.Request) {
		if deps.ConfigManager == nil {
			writeError(w, http.StatusServiceUnavailable, "config manager not initialized")
			return
		}
		key := strings.TrimPrefix(r.URL.Path, "/api/config/")
		if key == "" {
			writeError(w, http.StatusBadRequest, "config key is required")
			return
		}
		switch r.Method {
		case http.MethodGet:
			data, err := deps.ConfigManager.LoadConfig(key)
			if err != nil {
				writeError(w, http.StatusInternalServerError, err.Error())
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"success": true, "data": data})
		case http.MethodPut:
			var raw json.RawMessage
			if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
				writeError(w, http.StatusBadRequest, err.Error())
				return
			}
			if err := deps.ConfigManager.SaveConfig(key, raw); err != nil {
				writeError(w, http.StatusInternalServerError, err.Error())
				return
			}
			writeJSON(w, http.StatusOK, map[string]bool{"success": true})
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	})

	// ---- Storage API ----
	mux.HandleFunc("/api/storage/status", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		writeJSON(w, http.StatusOK, deps.StorageRecorder.Status())
	})
	mux.HandleFunc("/api/storage/start", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		var body struct {
			OutputDir  string `json:"outputDir"`
			FilePrefix string `json:"filePrefix"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		if err := deps.StorageRecorder.Start(storage.RecordingConfig{OutputDir: body.OutputDir, FilePrefix: body.FilePrefix}); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]bool{"success": true})
	})
	mux.HandleFunc("/api/storage/stop", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if err := deps.StorageRecorder.Stop(); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]bool{"success": true})
	})

	return corsMiddleware(mux)
}

func handleDaqStream(w http.ResponseWriter, r *http.Request, hub *usecase.AcquisitionHub, deviceID string) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "streaming not supported")
		return
	}
	subscription, unsubscribe := hub.Subscribe(deviceID, 16)
	defer unsubscribe()

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	for {
		select {
		case <-r.Context().Done():
			return
		case payload := <-subscription:
			data, err := json.Marshal(payload)
			if err != nil {
				continue
			}
			_, _ = fmt.Fprintf(w, "event: payload\ndata: %s\n\n", data)
			flusher.Flush()
		}
	}
}

func handleDeviceByID(w http.ResponseWriter, r *http.Request, deps Deps) {
	path := strings.TrimPrefix(r.URL.Path, "/api/device/")
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) != 2 {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	id, action := parts[0], parts[1]

	switch {
	case r.Method == http.MethodPost && action == "connect":
		if err := deps.DeviceManager.Connect(id); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]bool{"success": true})
	case r.Method == http.MethodPost && action == "disconnect":
		if err := deps.DeviceManager.Disconnect(id); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]bool{"success": true})
	case r.Method == http.MethodPost && action == "startAcquisition":
		if err := deps.DeviceManager.StartAcquisition(id); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]bool{"success": true})
	case r.Method == http.MethodPost && action == "stopAcquisition":
		if err := deps.DeviceManager.StopAcquisition(id); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]bool{"success": true})
	case r.Method == http.MethodGet && action == "status":
		status, ok := deps.DeviceManager.GetStatus(id)
		if !ok {
			writeError(w, http.StatusNotFound, "device not connected")
			return
		}
		writeJSON(w, http.StatusOK, status)
	case r.Method == http.MethodPut && action == "unit":
		var body struct {
			Unit string `json:"unit"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		if err := deps.DeviceManager.SetUnit(id, body.Unit); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]bool{"success": true})
	case r.Method == http.MethodGet && action == "daqT1603Config":
		config, err := deps.DeviceManager.GetDaqT1603Config(id)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, config)
	case r.Method == http.MethodPut && action == "daqT1603Config":
		var config device.DaqT1603HardwareConfig
		if err := json.NewDecoder(r.Body).Decode(&config); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		if err := deps.DeviceManager.ApplyDaqT1603Config(id, config); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]bool{"success": true})
	case r.Method == http.MethodGet && action == "dsa3217ScanConfig":
		config, err := deps.DeviceManager.GetDsa3217ScanConfig(id)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"success": true, "data": config})
	case r.Method == http.MethodPut && action == "dsa3217ScanConfig":
		var body struct {
			Avg    int `json:"avg"`
			Period int `json:"period"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		verify, err := deps.DeviceManager.ApplyDsa3217ScanConfig(id, body.Avg, body.Period)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"success": true, "data": verify})
	case r.Method == http.MethodPut && action == "tare":
		var body struct {
			ChannelIndex int     `json:"channelIndex"`
			Offset       float64 `json:"offset"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		if err := deps.DeviceManager.SetTare(id, body.ChannelIndex, body.Offset); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]bool{"success": true})
	case r.Method == http.MethodGet && action == "tare":
		channelIndex, err := parseChannelIndex(r)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		offset, err := deps.DeviceManager.GetTare(id, channelIndex)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]float64{"offset": offset})
	case r.Method == http.MethodPost && action == "clearTare":
		channelIndex, err := parseChannelIndex(r)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		if err := deps.DeviceManager.ClearTare(id, channelIndex); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]bool{"success": true})
	default:
		writeError(w, http.StatusNotFound, "not found")
	}
}

func parseChannelIndex(r *http.Request) (int, error) {
	raw := strings.TrimSpace(r.URL.Query().Get("channelIndex"))
	if raw == "" {
		return 0, nil
	}
	channelIndex, err := strconv.Atoi(raw)
	if err != nil || channelIndex < 0 {
		return 0, fmt.Errorf("channelIndex must be a non-negative integer")
	}
	return channelIndex, nil
}

func decodeBody(w http.ResponseWriter, r *http.Request, v any) (ok bool) {
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return false
	}
	return true
}

func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]any{"success": false, "error": message})
}

func handleFiveholeSnakePoints(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var body calibration.FiveHolePointLayout
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	points, err := calibration.GenerateFiveHoleSnakePoints(body)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, points)
}

func prbFileInfo(filePath string, validRange coreinterp.PrbValidRange) map[string]any {
	return map[string]any{
		"filePath":   filePath,
		"fileName":   filepath.Base(filePath),
		"loadedAt":   time.Now().UnixMilli(),
		"validRange": validRange,
	}
}
