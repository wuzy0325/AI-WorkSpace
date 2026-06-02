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
	"wind-daq/services/api-go/internal/usecase"
)

type Deps struct {
	DeviceManager      *usecase.DeviceManager
	AcquisitionHub     *usecase.AcquisitionHub
	ReportManager      *usecase.ReportManager
	MotionManager      *usecase.MotionManager
	CalibrationManager *usecase.CalibrationManager
	TraversalManager   *usecase.TraversalManager
	StorageRecorder    *usecase.StorageRecorder
	ConfigManager      *usecase.ConfigManager
}

type traversalAPIConfig struct {
	Name   string `json:"name"`
	Layout struct {
		Pattern string `json:"pattern"`
		Line    *struct {
			StartX        float64                 `json:"startX"`
			StartY        float64                 `json:"startY"`
			EndX          float64                 `json:"endX"`
			EndY          float64                 `json:"endY"`
			XStepSegments []traversal.StepSegment `json:"xStepSegments"`
			YStepSegments []traversal.StepSegment `json:"yStepSegments"`
		} `json:"line"`
		Rectangle *struct {
			XMin          float64                 `json:"xMin"`
			XMax          float64                 `json:"xMax"`
			XStepSegments []traversal.StepSegment `json:"xStepSegments"`
			YMin          float64                 `json:"yMin"`
			YMax          float64                 `json:"yMax"`
			YStepSegments []traversal.StepSegment `json:"yStepSegments"`
		} `json:"rectangle"`
		Sector *struct {
			CenterX             float64                 `json:"centerX"`
			CenterY             float64                 `json:"centerY"`
			RadiusMin           float64                 `json:"radiusMin"`
			RadiusMax           float64                 `json:"radiusMax"`
			RadialStepSegments  []traversal.StepSegment `json:"radialStepSegments"`
			AngleStart          float64                 `json:"angleStart"`
			AngleEnd            float64                 `json:"angleEnd"`
			AngularStepSegments []traversal.StepSegment `json:"angularStepSegments"`
		} `json:"sector"`
		Custom *struct {
			Points []struct {
				X float64 `json:"x"`
				Y float64 `json:"y"`
			} `json:"points"`
		} `json:"custom"`
	} `json:"layout"`
	Channels struct {
		ProbeChannels []struct {
			Channel struct {
				DeviceID     string `json:"deviceId"`
				ChannelIndex int    `json:"channelIndex"`
			} `json:"channel"`
			Enabled bool `json:"enabled"`
		} `json:"probeChannels"`
	} `json:"channels"`
	DwellTimeMs int `json:"dwellTimeMs"`
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
	if deps.MotionManager != nil {
		motionhttp.RegisterMotionRoutes(mux, deps.MotionManager)
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
			checks := []map[string]any{
				{"name": "PRB", "passed": deps.TraversalManager != nil && deps.TraversalManager.HasLoadedInterpolator(), "message": "Load PRB or calibration CSV before running interpolation"},
				{"name": "Motion", "passed": deps.MotionManager != nil, "message": "Motion manager is available"},
				{"name": "DAQ", "passed": deps.AcquisitionHub != nil, "message": "DAQ acquisition hub is available"},
			}
			allPassed := true
			for _, check := range checks {
				if passed, _ := check["passed"].(bool); !passed {
					allPassed = false
				}
			}
			writeJSON(w, http.StatusOK, map[string]any{"allPassed": allPassed, "checks": checks})
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
			config, dwell, err := traversalConfigFromRequest(raw)
			if err != nil {
				writeError(w, http.StatusBadRequest, err.Error())
				return
			}
			err = deps.TraversalManager.Start(config)
			if err != nil {
				writeError(w, http.StatusBadRequest, err.Error())
				return
			}
			deps.TraversalManager.SaveConfigRaw(raw)
			if dwell >= 0 {
				go deps.TraversalManager.RunTraversalLoop(dwell)
			}
			writeJSON(w, http.StatusOK, map[string]string{"taskId": config.TaskID})
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
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})

	// ---- Config API ----
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

func traversalConfigFromRequest(raw json.RawMessage) (traversal.Config, time.Duration, error) {
	var legacy struct {
		TaskID   string            `json:"taskId"`
		DeviceID string            `json:"deviceId"`
		Channels []int             `json:"channels"`
		Path     []traversal.Point `json:"path"`
	}
	if err := json.Unmarshal(raw, &legacy); err == nil && legacy.TaskID != "" {
		return traversal.Config{TaskID: legacy.TaskID, DeviceID: legacy.DeviceID, Channels: legacy.Channels, Path: legacy.Path}, -1, nil
	}

	var cfg traversalAPIConfig
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return traversal.Config{}, 0, err
	}
	points := traversal.PointsFromLayout(traversal.LayoutConfig{
		Pattern: cfg.Layout.Pattern,
		Line: func() *traversal.LineLayout {
			if cfg.Layout.Line == nil {
				return nil
			}
			return &traversal.LineLayout{
				StartX:        cfg.Layout.Line.StartX,
				StartY:        cfg.Layout.Line.StartY,
				EndX:          cfg.Layout.Line.EndX,
				EndY:          cfg.Layout.Line.EndY,
				XStepSegments: cfg.Layout.Line.XStepSegments,
				YStepSegments: cfg.Layout.Line.YStepSegments,
			}
		}(),
		Rectangle: func() *traversal.RectangleLayout {
			if cfg.Layout.Rectangle == nil {
				return nil
			}
			return &traversal.RectangleLayout{
				XMin:          cfg.Layout.Rectangle.XMin,
				XMax:          cfg.Layout.Rectangle.XMax,
				XStepSegments: cfg.Layout.Rectangle.XStepSegments,
				YMin:          cfg.Layout.Rectangle.YMin,
				YMax:          cfg.Layout.Rectangle.YMax,
				YStepSegments: cfg.Layout.Rectangle.YStepSegments,
			}
		}(),
		Sector: func() *traversal.SectorLayout {
			if cfg.Layout.Sector == nil {
				return nil
			}
			return &traversal.SectorLayout{
				CenterX:             cfg.Layout.Sector.CenterX,
				CenterY:             cfg.Layout.Sector.CenterY,
				RadiusMin:           cfg.Layout.Sector.RadiusMin,
				RadiusMax:           cfg.Layout.Sector.RadiusMax,
				RadialStepSegments:  cfg.Layout.Sector.RadialStepSegments,
				AngleStart:          cfg.Layout.Sector.AngleStart,
				AngleEnd:            cfg.Layout.Sector.AngleEnd,
				AngularStepSegments: cfg.Layout.Sector.AngularStepSegments,
			}
		}(),
		Custom: func() *traversal.CustomLayout {
			if cfg.Layout.Custom == nil {
				return nil
			}
			cl := &traversal.CustomLayout{}
			for _, p := range cfg.Layout.Custom.Points {
				cl.Points = append(cl.Points, struct {
					X float64 `json:"x"`
					Y float64 `json:"y"`
				}{X: p.X, Y: p.Y})
			}
			return cl
		}(),
	})
	channels := make([]int, 0, len(cfg.Channels.ProbeChannels))
	deviceID := ""
	for _, probe := range cfg.Channels.ProbeChannels {
		if !probe.Enabled || probe.Channel.ChannelIndex < 0 {
			continue
		}
		if deviceID == "" {
			deviceID = probe.Channel.DeviceID
		}
		channels = append(channels, probe.Channel.ChannelIndex)
	}
	if deviceID == "" {
		return traversal.Config{}, 0, fmt.Errorf("deviceId is required")
	}
	if len(channels) == 0 {
		return traversal.Config{}, 0, fmt.Errorf("channels are required")
	}
	if len(points) == 0 {
		return traversal.Config{}, 0, fmt.Errorf("path is required")
	}
	dwell := time.Duration(cfg.DwellTimeMs) * time.Millisecond
	if dwell < 0 {
		dwell = 0
	}
	return traversal.Config{TaskID: fmt.Sprintf("trav-%d", time.Now().UnixMilli()), DeviceID: deviceID, Channels: channels, Path: points}, dwell, nil
}

func prbFileInfo(filePath string, validRange coreinterp.PrbValidRange) map[string]any {
	return map[string]any{
		"filePath":   filePath,
		"fileName":   filepath.Base(filePath),
		"loadedAt":   time.Now().UnixMilli(),
		"validRange": validRange,
	}
}
