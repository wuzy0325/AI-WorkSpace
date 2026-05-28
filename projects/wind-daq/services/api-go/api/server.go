package api

import (
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	coreinterp "ai-workspace/shared/algorithms/go/fivehole/interpolation"
	interpfiles "wind-daq/services/api-go/internal/adapters/interpolation"
	"wind-daq/services/api-go/internal/core/calibration"
	"wind-daq/services/api-go/internal/core/device"
	"wind-daq/services/api-go/internal/core/motion"
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
}

type traversalAPIConfig struct {
	Name   string `json:"name"`
	Layout struct {
		Pattern string `json:"pattern"`
		Line    *struct {
			StartX        float64       `json:"startX"`
			StartY        float64       `json:"startY"`
			EndX          float64       `json:"endX"`
			EndY          float64       `json:"endY"`
			XStepSegments []stepSegment `json:"xStepSegments"`
			YStepSegments []stepSegment `json:"yStepSegments"`
		} `json:"line"`
		Rectangle *struct {
			XMin          float64       `json:"xMin"`
			XMax          float64       `json:"xMax"`
			XStepSegments []stepSegment `json:"xStepSegments"`
			YMin          float64       `json:"yMin"`
			YMax          float64       `json:"yMax"`
			YStepSegments []stepSegment `json:"yStepSegments"`
		} `json:"rectangle"`
		Sector *struct {
			CenterX             float64       `json:"centerX"`
			CenterY             float64       `json:"centerY"`
			RadiusMin           float64       `json:"radiusMin"`
			RadiusMax           float64       `json:"radiusMax"`
			RadialStepSegments  []stepSegment `json:"radialStepSegments"`
			AngleStart          float64       `json:"angleStart"`
			AngleEnd            float64       `json:"angleEnd"`
			AngularStepSegments []stepSegment `json:"angularStepSegments"`
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

type stepSegment struct {
	Start float64 `json:"start"`
	End   float64 `json:"end"`
	Step  float64 `json:"step"`
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
	mux.HandleFunc("/api/motion/profiles", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			profiles, err := deps.MotionManager.LoadProfiles()
			if err != nil {
				writeError(w, http.StatusInternalServerError, err.Error())
				return
			}
			writeJSON(w, http.StatusOK, profiles)
		case http.MethodPut:
			var profile motion.MotionControllerProfile
			if !decodeBody(w, r, &profile) {
				return
			}
			if err := deps.MotionManager.UpsertProfile(profile); err != nil {
				writeError(w, http.StatusBadRequest, err.Error())
				return
			}
			writeJSON(w, http.StatusOK, map[string]bool{"success": true})
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	})
	mux.HandleFunc("/api/motion/profiles/", func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimPrefix(r.URL.Path, "/api/motion/profiles/")
		if id == "" {
			writeError(w, http.StatusBadRequest, "profile id is required")
			return
		}
		if r.Method != http.MethodDelete {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if err := deps.MotionManager.DeleteProfile(id); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]bool{"success": true})
	})
	mux.HandleFunc("/api/motion/status", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		writeJSON(w, http.StatusOK, deps.MotionManager.StatusAll())
	})

	handleMotionCmd := func(pattern string, fn func(id string, axis motion.AxisName, body motionBody) error) {
		mux.HandleFunc(pattern, func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPost {
				w.WriteHeader(http.StatusMethodNotAllowed)
				return
			}
			var body motionBody
			if !decodeBody(w, r, &body) {
				return
			}
			if body.ID == "" || body.Axis == "" {
				writeError(w, http.StatusBadRequest, "id and axis are required")
				return
			}
			if err := fn(body.ID, motion.AxisName(body.Axis), body); err != nil {
				writeError(w, http.StatusBadRequest, err.Error())
				return
			}
			writeJSON(w, http.StatusOK, map[string]bool{"success": true})
		})
	}
	handleMotionCmd("/api/motion/home", func(id string, axis motion.AxisName, body motionBody) error {
		return deps.MotionManager.Home(id, axis)
	})
	handleMotionCmd("/api/motion/moveTo", func(id string, axis motion.AxisName, body motionBody) error {
		return deps.MotionManager.MoveTo(id, axis, body.Position)
	})
	handleMotionCmd("/api/motion/moveBy", func(id string, axis motion.AxisName, body motionBody) error {
		return deps.MotionManager.MoveBy(id, axis, body.Delta)
	})
	handleMotionCmd("/api/motion/jog", func(id string, axis motion.AxisName, body motionBody) error {
		return deps.MotionManager.Jog(id, axis, body.Velocity)
	})
	handleMotionCmd("/api/motion/definePosition", func(id string, axis motion.AxisName, body motionBody) error {
		return deps.MotionManager.DefinePosition(id, axis, body.Position)
	})

	handleMotionSimple := func(pattern string, fn func(id string) error) {
		mux.HandleFunc(pattern, func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPost {
				w.WriteHeader(http.StatusMethodNotAllowed)
				return
			}
			var body struct {
				ID string `json:"id"`
			}
			if !decodeBody(w, r, &body) {
				return
			}
			if body.ID == "" {
				writeError(w, http.StatusBadRequest, "id is required")
				return
			}
			if err := fn(body.ID); err != nil {
				writeError(w, http.StatusBadRequest, err.Error())
				return
			}
			writeJSON(w, http.StatusOK, map[string]bool{"success": true})
		})
	}
	handleMotionSimple("/api/motion/connect", deps.MotionManager.Connect)
	handleMotionSimple("/api/motion/disconnect", deps.MotionManager.Disconnect)
	handleMotionSimple("/api/motion/emergencyStop", deps.MotionManager.EmergencyStop)
	mux.HandleFunc("/api/motion/stop", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		var body motionBody
		if !decodeBody(w, r, &body) {
			return
		}
		if body.ID == "" {
			writeError(w, http.StatusBadRequest, "id is required")
			return
		}
		if err := deps.MotionManager.Stop(body.ID, motion.AxisName(body.Axis)); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]bool{"success": true})
	})

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
				go runTraversalTask(deps.TraversalManager, dwell)
			}
			writeJSON(w, http.StatusOK, map[string]string{"taskId": config.TaskID})
		case "status":
			if r.Method != http.MethodGet {
				w.WriteHeader(http.StatusMethodNotAllowed)
				return
			}
			writeJSON(w, http.StatusOK, traversalStatusResponse(deps.TraversalManager.Status(), deps.TraversalManager))
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

type motionBody struct {
	ID       string  `json:"id"`
	Axis     string  `json:"axis"`
	Position float64 `json:"position"`
	Delta    float64 `json:"delta"`
	Velocity float64 `json:"velocity"`
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
	points := traversalPointsFromLayout(cfg)
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

func traversalPointsFromLayout(cfg traversalAPIConfig) []traversal.Point {
	switch cfg.Layout.Pattern {
	case "line":
		if cfg.Layout.Line == nil {
			return nil
		}
		xs := traversalStepValues(cfg.Layout.Line.StartX, cfg.Layout.Line.EndX, cfg.Layout.Line.XStepSegments)
		ys := traversalStepValues(cfg.Layout.Line.StartY, cfg.Layout.Line.EndY, cfg.Layout.Line.YStepSegments)
		if len(ys) == 0 {
			ys = []float64{cfg.Layout.Line.StartY}
		}
		return gridPoints(xs, ys)
	case "rectangle":
		if cfg.Layout.Rectangle == nil {
			return nil
		}
		return gridPoints(
			traversalStepValues(cfg.Layout.Rectangle.XMin, cfg.Layout.Rectangle.XMax, cfg.Layout.Rectangle.XStepSegments),
			traversalStepValues(cfg.Layout.Rectangle.YMin, cfg.Layout.Rectangle.YMax, cfg.Layout.Rectangle.YStepSegments),
		)
	case "sector":
		if cfg.Layout.Sector == nil {
			return nil
		}
		var points []traversal.Point
		radii := traversalStepValues(cfg.Layout.Sector.RadiusMin, cfg.Layout.Sector.RadiusMax, cfg.Layout.Sector.RadialStepSegments)
		angles := traversalStepValues(cfg.Layout.Sector.AngleStart, cfg.Layout.Sector.AngleEnd, cfg.Layout.Sector.AngularStepSegments)
		for _, radius := range radii {
			for _, angle := range angles {
				radian := angle * math.Pi / 180
				points = append(points, traversal.Point{
					X: cfg.Layout.Sector.CenterX + radius*math.Cos(radian),
					Y: cfg.Layout.Sector.CenterY + radius*math.Sin(radian),
				})
			}
		}
		return points
	case "custom":
		if cfg.Layout.Custom == nil {
			return nil
		}
		points := make([]traversal.Point, 0, len(cfg.Layout.Custom.Points))
		for _, point := range cfg.Layout.Custom.Points {
			points = append(points, traversal.Point{X: point.X, Y: point.Y})
		}
		return points
	default:
		return nil
	}
}

func traversalStepValues(start, end float64, segments []stepSegment) []float64 {
	var values []float64
	for _, segment := range segments {
		if segment.Step <= 0 {
			continue
		}
		actualStart := math.Max(segment.Start, start)
		actualEnd := math.Min(segment.End, end)
		for value := actualStart; value <= actualEnd+1e-9; value += segment.Step {
			if !containsFloat(values, value) {
				values = append(values, value)
			}
		}
	}
	if len(values) == 0 {
		if start == end {
			return []float64{start}
		}
		return []float64{start, end}
	}
	return values
}

func gridPoints(xs, ys []float64) []traversal.Point {
	points := make([]traversal.Point, 0, len(xs)*len(ys))
	for _, x := range xs {
		for _, y := range ys {
			points = append(points, traversal.Point{X: x, Y: y})
		}
	}
	return points
}

func containsFloat(values []float64, needle float64) bool {
	for _, value := range values {
		if math.Abs(value-needle) < 1e-9 {
			return true
		}
	}
	return false
}

func traversalStatusResponse(status traversal.Status, manager *usecase.TraversalManager) map[string]any {
	state := string(status.State)
	if status.State == traversal.StateIdle && status.TotalPoints > 0 && status.CurrentPoint >= status.TotalPoints {
		state = "completed"
	}
	progress := 0.0
	if status.TotalPoints > 0 {
		progress = float64(status.CurrentPoint) / float64(status.TotalPoints) * 100
	}
	var currentPoint map[string]float64
	if status.CurrentPointCoordinates != nil {
		point := *status.CurrentPointCoordinates
		currentPoint = map[string]float64{"alpha": point.X, "beta": point.Y}
	}
	dataPoints := traversalDataPoints(status.Results, manager)
	var latestData any
	if len(dataPoints) > 0 {
		latestData = dataPoints[len(dataPoints)-1]
	}
	return map[string]any{
		"taskId":                  status.TaskID,
		"state":                   string(status.State),
		"status":                  state,
		"currentPoint":            status.CurrentPoint,
		"currentPointCoordinates": currentPoint,
		"completedPoints":         status.CurrentPoint,
		"totalPoints":             status.TotalPoints,
		"progress":                progress,
		"startTime":               status.StartedAt,
		"lastError":               status.LastError,
		"results":                 status.Results,
		"dataPoints":              dataPoints,
		"latestData":              latestData,
	}
}

func traversalDataPoints(results []traversal.PointResult, manager *usecase.TraversalManager) []map[string]any {
	dataPoints := make([]map[string]any, 0, len(results))
	for _, result := range results {
		rawPressure, input, ok := traversalRawPressure(result.Values)
		interpolationResult := coreinterp.InterpolationResult{IsValid: false}
		if ok && manager != nil {
			calculated, err := manager.CalculateRealtime(input)
			if err == nil {
				interpolationResult = calculated
			} else {
				interpolationResult.Warning = err.Error()
			}
		}
		dataPoints = append(dataPoints, map[string]any{
			"pointId":             result.PointIndex + 1,
			"coordinates":         map[string]float64{"alpha": result.Point.X, "beta": result.Point.Y},
			"rawPressure":         rawPressure,
			"interpolationResult": interpolationResult,
			"sampleCount":         1,
			"timestamp":           result.Timestamp,
			"dwellTimeElapsed":    0,
		})
	}
	return dataPoints
}

func traversalRawPressure(values map[int]float64) (map[string]float64, coreinterp.InterpolationInput, bool) {
	orderedKeys := make([]int, 0, len(values))
	for key := range values {
		orderedKeys = append(orderedKeys, key)
	}
	sort.Ints(orderedKeys)
	raw := make(map[string]float64, 7)
	labels := []string{"P1", "P2", "P3", "P4", "P5", "Patm", "Tatm"}
	for i, label := range labels {
		if i >= len(orderedKeys) {
			continue
		}
		raw[label] = values[orderedKeys[i]]
	}
	input := coreinterp.InterpolationInput{
		P1:   raw["P1"],
		P2:   raw["P2"],
		P3:   raw["P3"],
		P4:   raw["P4"],
		P5:   raw["P5"],
		PAtm: raw["Patm"],
		TAtm: raw["Tatm"],
	}
	_, hasP1 := raw["P1"]
	_, hasP2 := raw["P2"]
	_, hasP3 := raw["P3"]
	_, hasP4 := raw["P4"]
	_, hasP5 := raw["P5"]
	_, hasPatm := raw["Patm"]
	_, hasTatm := raw["Tatm"]
	return raw, input, hasP1 && hasP2 && hasP3 && hasP4 && hasP5 && hasPatm && hasTatm
}

func runTraversalTask(manager *usecase.TraversalManager, dwell time.Duration) {
	if dwell <= 0 {
		dwell = 100 * time.Millisecond
	}
	for {
		status := manager.Status()
		switch status.State {
		case traversal.StateRunning:
			if status.TotalPoints > 0 && status.CurrentPoint >= status.TotalPoints {
				return
			}
			if err := manager.RunCurrentPoint(); err != nil {
				return
			}
			time.Sleep(dwell)
		case traversal.StatePaused:
			time.Sleep(200 * time.Millisecond)
		default:
			return
		}
	}
}

func prbFileInfo(filePath string, validRange coreinterp.PrbValidRange) map[string]any {
	return map[string]any{
		"filePath":   filePath,
		"fileName":   filepath.Base(filePath),
		"loadedAt":   time.Now().UnixMilli(),
		"validRange": validRange,
	}
}
