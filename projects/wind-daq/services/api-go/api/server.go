package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

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
		case "generateGrid":
			if r.Method != http.MethodPost {
				w.WriteHeader(http.StatusMethodNotAllowed)
				return
			}
			var grid traversal.GridConfig
			if err := json.NewDecoder(r.Body).Decode(&grid); err != nil {
				writeError(w, http.StatusBadRequest, err.Error())
				return
			}
			path, err := traversal.GenerateGridPath(grid)
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
			var body struct {
				TaskID   string            `json:"taskId"`
				DeviceID string            `json:"deviceId"`
				Channels []int             `json:"channels"`
				Path     []traversal.Point `json:"path"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				writeError(w, http.StatusBadRequest, err.Error())
				return
			}
			err := deps.TraversalManager.Start(traversal.Config{TaskID: body.TaskID, DeviceID: body.DeviceID, Channels: body.Channels, Path: body.Path})
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
			writeJSON(w, http.StatusOK, deps.TraversalManager.Status())
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
		channelIndex := 0
		if idxStr := r.URL.Query().Get("channelIndex"); idxStr != "" {
			fmt.Sscanf(idxStr, "%d", &channelIndex)
		}
		offset, err := deps.DeviceManager.GetTare(id, channelIndex)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]float64{"offset": offset})
	case r.Method == http.MethodPost && action == "clearTare":
		channelIndex := 0
		if idxStr := r.URL.Query().Get("channelIndex"); idxStr != "" {
			fmt.Sscanf(idxStr, "%d", &channelIndex)
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
