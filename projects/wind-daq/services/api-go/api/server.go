package api

import (
	"encoding/json"
	"net/http"
	"strings"

	"wind-daq/services/api-go/internal/core/device"
	"wind-daq/services/api-go/internal/usecase"
)

type Deps struct {
	DeviceManager  *usecase.DeviceManager
	AcquisitionHub *usecase.AcquisitionHub
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
	return mux
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
