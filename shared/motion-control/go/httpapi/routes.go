package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"shared.local/device-sdk/go/motion/core"
)

// MotionService is the application-level motion API used by HTTP routes.
type MotionService interface {
	LoadProfiles() ([]core.MotionControllerProfile, error)
	GetProfiles() []core.MotionControllerProfile
	UpsertProfile(profile core.MotionControllerProfile) error
	DeleteProfile(id string) error
	Connect(ctx context.Context, id string) error
	Disconnect(ctx context.Context, id string) error
	StatusAll(ctx context.Context) []core.ControllerStatus
	Home(ctx context.Context, id string, axis core.AxisName) error
	MoveTo(ctx context.Context, id string, axis core.AxisName, position float64) error
	MoveBy(ctx context.Context, id string, axis core.AxisName, delta float64) error
	Jog(ctx context.Context, id string, axis core.AxisName, velocity float64) error
	DefinePosition(ctx context.Context, id string, axis core.AxisName, position float64) error
	Stop(ctx context.Context, id string, axis core.AxisName) error
	EmergencyStop(ctx context.Context, id string) error
	ResetEmergencyStop(ctx context.Context, id string) error
}

// RegisterMotionRoutes registers shared /api/motion/* HTTP routes.
func RegisterMotionRoutes(mux *http.ServeMux, motion MotionService) {
	mux.HandleFunc("/api/motion/profiles", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			profiles, err := motion.LoadProfiles()
			if err != nil {
				writeError(w, http.StatusInternalServerError, err.Error())
				return
			}
			writeJSON(w, http.StatusOK, profiles)
		case http.MethodPut:
			var profile core.MotionControllerProfile
			if !decodeBody(w, r, &profile) {
				return
			}
			if err := motion.UpsertProfile(profile); err != nil {
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
		if err := motion.DeleteProfile(id); err != nil {
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
		writeJSON(w, http.StatusOK, motion.StatusAll(r.Context()))
	})

	handleCommand := func(pattern string, fn func(context.Context, string, core.AxisName, motionBody) error) {
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
			if err := fn(r.Context(), body.ID, core.AxisName(body.Axis), body); err != nil {
				writeError(w, http.StatusBadRequest, err.Error())
				return
			}
			writeJSON(w, http.StatusOK, map[string]bool{"success": true})
		})
	}
	handleCommand("/api/motion/home", func(ctx context.Context, id string, axis core.AxisName, body motionBody) error {
		return motion.Home(ctx, id, axis)
	})
	handleCommand("/api/motion/moveTo", func(ctx context.Context, id string, axis core.AxisName, body motionBody) error {
		return motion.MoveTo(ctx, id, axis, body.Position)
	})
	handleCommand("/api/motion/moveBy", func(ctx context.Context, id string, axis core.AxisName, body motionBody) error {
		return motion.MoveBy(ctx, id, axis, body.Delta)
	})
	handleCommand("/api/motion/jog", func(ctx context.Context, id string, axis core.AxisName, body motionBody) error {
		return motion.Jog(ctx, id, axis, body.Velocity)
	})
	handleCommand("/api/motion/definePosition", func(ctx context.Context, id string, axis core.AxisName, body motionBody) error {
		return motion.DefinePosition(ctx, id, axis, body.Position)
	})

	handleSimple := func(pattern string, fn func(context.Context, string) error) {
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
			if err := fn(r.Context(), body.ID); err != nil {
				writeError(w, http.StatusBadRequest, err.Error())
				return
			}
			writeJSON(w, http.StatusOK, map[string]bool{"success": true})
		})
	}
	handleSimple("/api/motion/connect", motion.Connect)
	handleSimple("/api/motion/disconnect", motion.Disconnect)
	handleSimple("/api/motion/emergencyStop", motion.EmergencyStop)
	handleSimple("/api/motion/resetEmergencyStop", motion.ResetEmergencyStop)

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
		if err := motion.Stop(r.Context(), body.ID, core.AxisName(body.Axis)); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]bool{"success": true})
	})
}

type motionBody struct {
	ID       string  `json:"id"`
	Axis     string  `json:"axis"`
	Position float64 `json:"position"`
	Delta    float64 `json:"delta"`
	Velocity float64 `json:"velocity"`
}

func decodeBody(w http.ResponseWriter, r *http.Request, v any) bool {
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
