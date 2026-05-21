package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"wind-daq/services/api-go/internal/core/calibration"
	"wind-daq/services/api-go/internal/core/device"
	"wind-daq/services/api-go/internal/core/motion"
	"wind-daq/services/api-go/internal/core/storage"
	"wind-daq/services/api-go/internal/core/traversal"
	wind_report "wind-daq/services/api-go/internal/core/report"
	"wind-daq/services/api-go/internal/usecase"
)

type Deps struct {
	DeviceManager     *usecase.DeviceManager
	AcquisitionHub    *usecase.AcquisitionHub
	ReportManager     *usecase.ReportManager
	MotionManager     *usecase.MotionManager
	CalibrationManager *usecase.CalibrationManager
	TraversalManager  *usecase.TraversalManager
	StorageRecorder   *usecase.StorageRecorder
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
	postMotion := func(action string, fn func() error) func(w http.ResponseWriter, r *http.Request) {
		return func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPost { w.WriteHeader(http.StatusMethodNotAllowed); return }
			if err := fn(); err != nil { writeError(w, http.StatusBadRequest, err.Error()); return }
			writeJSON(w, http.StatusOK, map[string]bool{"success": true})
		}
	}
	mux.HandleFunc("/api/motion/connect", postMotion("connect", deps.MotionManager.Connect))
	mux.HandleFunc("/api/motion/disconnect", postMotion("disconnect", deps.MotionManager.Disconnect))
	mux.HandleFunc("/api/motion/home", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost { w.WriteHeader(http.StatusMethodNotAllowed); return }
		var body struct { Axis string `json:"axis"` }
		json.NewDecoder(r.Body).Decode(&body)
		if err := deps.MotionManager.Home(motion.AxisName(body.Axis)); err != nil { writeError(w, http.StatusBadRequest, err.Error()); return }
		writeJSON(w, http.StatusOK, map[string]bool{"success": true})
	})
	mux.HandleFunc("/api/motion/emergencyStop", postMotion("emergencyStop", deps.MotionManager.EmergencyStop))
	mux.HandleFunc("/api/motion/stop", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost { w.WriteHeader(http.StatusMethodNotAllowed); return }
		var body struct { Axis string `json:"axis"` }
		json.NewDecoder(r.Body).Decode(&body)
		if err := deps.MotionManager.Stop(motion.AxisName(body.Axis)); err != nil { writeError(w, http.StatusBadRequest, err.Error()); return }
		writeJSON(w, http.StatusOK, map[string]bool{"success": true})
	})
	mux.HandleFunc("/api/motion/status", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet { w.WriteHeader(http.StatusMethodNotAllowed); return }
		writeJSON(w, http.StatusOK, deps.MotionManager.Status())
	})
	mux.HandleFunc("/api/motion/moveTo", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost { w.WriteHeader(http.StatusMethodNotAllowed); return }
		var body struct { Axis string `json:"axis"`; Position float64 `json:"position"` }
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil { writeError(w, http.StatusBadRequest, err.Error()); return }
		if err := deps.MotionManager.MoveTo(motion.AxisName(body.Axis), body.Position); err != nil { writeError(w, http.StatusBadRequest, err.Error()); return }
		writeJSON(w, http.StatusOK, map[string]bool{"success": true})
	})
	mux.HandleFunc("/api/motion/moveBy", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost { w.WriteHeader(http.StatusMethodNotAllowed); return }
		var body struct { Axis string `json:"axis"`; Delta float64 `json:"delta"` }
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil { writeError(w, http.StatusBadRequest, err.Error()); return }
		if err := deps.MotionManager.MoveBy(motion.AxisName(body.Axis), body.Delta); err != nil { writeError(w, http.StatusBadRequest, err.Error()); return }
		writeJSON(w, http.StatusOK, map[string]bool{"success": true})
	})
	mux.HandleFunc("/api/motion/jog", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost { w.WriteHeader(http.StatusMethodNotAllowed); return }
		var body struct { Axis string `json:"axis"`; Velocity float64 `json:"velocity"` }
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil { writeError(w, http.StatusBadRequest, err.Error()); return }
		if err := deps.MotionManager.Jog(motion.AxisName(body.Axis), body.Velocity); err != nil { writeError(w, http.StatusBadRequest, err.Error()); return }
		writeJSON(w, http.StatusOK, map[string]bool{"success": true})
	})

	// ---- Calibration API ----
	mux.HandleFunc("/api/calibration/", func(w http.ResponseWriter, r *http.Request) {
		action := strings.TrimPrefix(r.URL.Path, "/api/calibration/")
		switch action {
		case "start":
			if r.Method != http.MethodPost { w.WriteHeader(http.StatusMethodNotAllowed); return }
			var body struct {
				TaskID         string  `json:"taskId"`
				DeviceID       string  `json:"deviceId"`
				Type           string  `json:"type"`
				Channels       []int   `json:"channels"`
				PressurePoints []float64 `json:"pressurePoints"`
				AverageSamples int     `json:"averageSamples"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil { writeError(w, http.StatusBadRequest, err.Error()); return }
			err := deps.CalibrationManager.Start(calibration.Config{
				TaskID: body.TaskID, DeviceID: body.DeviceID, Type: body.Type,
				Channels: body.Channels, PressurePoints: body.PressurePoints,
				AverageSamples: body.AverageSamples,
			})
			if err != nil { writeError(w, http.StatusBadRequest, err.Error()); return }
			writeJSON(w, http.StatusOK, map[string]bool{"success": true})
		case "status":
			if r.Method != http.MethodGet { w.WriteHeader(http.StatusMethodNotAllowed); return }
			writeJSON(w, http.StatusOK, deps.CalibrationManager.Status())
		case "collect":
			if r.Method != http.MethodPost { w.WriteHeader(http.StatusMethodNotAllowed); return }
			if err := deps.CalibrationManager.CollectCurrentPoint(); err != nil { writeError(w, http.StatusBadRequest, err.Error()); return }
			writeJSON(w, http.StatusOK, map[string]bool{"success": true})
		case "result":
			if r.Method != http.MethodGet { w.WriteHeader(http.StatusMethodNotAllowed); return }
			taskID := r.URL.Query().Get("taskId")
			if taskID == "" { writeError(w, http.StatusBadRequest, "taskId query parameter is required"); return }
			result, ok := deps.CalibrationManager.GetResult(taskID)
			if !ok { writeError(w, http.StatusNotFound, "calibration result not found"); return }
			writeJSON(w, http.StatusOK, result)
		case "pause":
			if r.Method != http.MethodPost { w.WriteHeader(http.StatusMethodNotAllowed); return }
			if err := deps.CalibrationManager.Pause(); err != nil { writeError(w, http.StatusBadRequest, err.Error()); return }
			writeJSON(w, http.StatusOK, map[string]bool{"success": true})
		case "resume":
			if r.Method != http.MethodPost { w.WriteHeader(http.StatusMethodNotAllowed); return }
			if err := deps.CalibrationManager.Resume(); err != nil { writeError(w, http.StatusBadRequest, err.Error()); return }
			writeJSON(w, http.StatusOK, map[string]bool{"success": true})
		case "stop":
			if r.Method != http.MethodPost { w.WriteHeader(http.StatusMethodNotAllowed); return }
			if err := deps.CalibrationManager.Stop(); err != nil { writeError(w, http.StatusBadRequest, err.Error()); return }
			writeJSON(w, http.StatusOK, map[string]bool{"success": true})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})

	// ---- Traversal API ----
	mux.HandleFunc("/api/traversal/", func(w http.ResponseWriter, r *http.Request) {
		action := strings.TrimPrefix(r.URL.Path, "/api/traversal/")
		switch action {
		case "start":
			if r.Method != http.MethodPost { w.WriteHeader(http.StatusMethodNotAllowed); return }
			var body struct {
				TaskID   string          `json:"taskId"`
				DeviceID string          `json:"deviceId"`
				Channels []int           `json:"channels"`
				Path     []traversal.Point `json:"path"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil { writeError(w, http.StatusBadRequest, err.Error()); return }
			err := deps.TraversalManager.Start(traversal.Config{TaskID: body.TaskID, DeviceID: body.DeviceID, Channels: body.Channels, Path: body.Path})
			if err != nil { writeError(w, http.StatusBadRequest, err.Error()); return }
			writeJSON(w, http.StatusOK, map[string]bool{"success": true})
		case "status":
			if r.Method != http.MethodGet { w.WriteHeader(http.StatusMethodNotAllowed); return }
			writeJSON(w, http.StatusOK, deps.TraversalManager.Status())
		case "runPoint":
			if r.Method != http.MethodPost { w.WriteHeader(http.StatusMethodNotAllowed); return }
			if err := deps.TraversalManager.RunCurrentPoint(); err != nil { writeError(w, http.StatusBadRequest, err.Error()); return }
			writeJSON(w, http.StatusOK, map[string]bool{"success": true})
		case "pause":
			if r.Method != http.MethodPost { w.WriteHeader(http.StatusMethodNotAllowed); return }
			if err := deps.TraversalManager.Pause(); err != nil { writeError(w, http.StatusBadRequest, err.Error()); return }
			writeJSON(w, http.StatusOK, map[string]bool{"success": true})
		case "resume":
			if r.Method != http.MethodPost { w.WriteHeader(http.StatusMethodNotAllowed); return }
			if err := deps.TraversalManager.Resume(); err != nil { writeError(w, http.StatusBadRequest, err.Error()); return }
			writeJSON(w, http.StatusOK, map[string]bool{"success": true})
		case "stop":
			if r.Method != http.MethodPost { w.WriteHeader(http.StatusMethodNotAllowed); return }
			if err := deps.TraversalManager.Stop(); err != nil { writeError(w, http.StatusBadRequest, err.Error()); return }
			writeJSON(w, http.StatusOK, map[string]bool{"success": true})
		case "result":
			if r.Method != http.MethodGet { w.WriteHeader(http.StatusMethodNotAllowed); return }
			taskID := r.URL.Query().Get("taskId")
			if taskID == "" { writeError(w, http.StatusBadRequest, "taskId query parameter is required"); return }
			result, ok := deps.TraversalManager.GetResult(taskID)
			if !ok { writeError(w, http.StatusNotFound, "traversal result not found"); return }
			writeJSON(w, http.StatusOK, result)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})

	// ---- Storage API ----
	mux.HandleFunc("/api/storage/status", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet { w.WriteHeader(http.StatusMethodNotAllowed); return }
		writeJSON(w, http.StatusOK, deps.StorageRecorder.Status())
	})
	mux.HandleFunc("/api/storage/start", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost { w.WriteHeader(http.StatusMethodNotAllowed); return }
		var body struct { OutputDir string `json:"outputDir"`; FilePrefix string `json:"filePrefix"` }
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil { writeError(w, http.StatusBadRequest, err.Error()); return }
		if err := deps.StorageRecorder.Start(storage.RecordingConfig{OutputDir: body.OutputDir, FilePrefix: body.FilePrefix}); err != nil { writeError(w, http.StatusBadRequest, err.Error()); return }
		writeJSON(w, http.StatusOK, map[string]bool{"success": true})
	})
	mux.HandleFunc("/api/storage/stop", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost { w.WriteHeader(http.StatusMethodNotAllowed); return }
		if err := deps.StorageRecorder.Stop(); err != nil { writeError(w, http.StatusBadRequest, err.Error()); return }
		writeJSON(w, http.StatusOK, map[string]bool{"success": true})
	})

	return mux
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
	default:
		writeError(w, http.StatusNotFound, "not found")
	}
}

func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]any{"success": false, "error": message})
}
