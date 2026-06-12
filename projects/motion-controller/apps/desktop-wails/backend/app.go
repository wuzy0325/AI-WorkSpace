package backend

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"

	"shared.local/device-sdk/go/motion/core"
	"motion-controller/services/api-go/pkg/appcontext"
)

// motionHTTPPort 是运动状态 HTTP API 的监听端口。
// 前端通过此端口轮询获取状态数据，绕开 Wails v2.12.0 的 reflect 序列化 bug。
const motionHTTPPort = ":16888"

type App struct {
	ctx        context.Context
	appCtx     *appcontext.AppContext
	httpServer *http.Server
}

func NewApp(appCtx *appcontext.AppContext) *App {
	return &App{appCtx: appCtx}
}

func (a *App) Startup(ctx context.Context) {
	a.ctx = ctx
	a.appCtx.MotionManager.LoadProfiles()
	a.startStatusHTTPServer()
}

func (a *App) startStatusHTTPServer() {
	mux := http.NewServeMux()

	// CORS 中间件：允许 WebView2 跨域请求
	cors := func(h http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Access-Control-Allow-Origin", "*")
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			h(w, r)
		}
	}

	mux.HandleFunc("/api/motion/status", cors(func(w http.ResponseWriter, r *http.Request) {
		statuses := a.appCtx.MotionManager.StatusAll(a.ctx)
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(statuses); err != nil {
			slog.Error("[App] status HTTP encode error", "err", err)
		}
	}))

	mux.HandleFunc("/api/motion/profiles", cors(func(w http.ResponseWriter, r *http.Request) {
		profiles := a.appCtx.MotionManager.GetProfiles()
		w.Header().Set("Content-Type", "application/json")
		slog.Info("[App] HTTP /profiles", "count", len(profiles))
		if err := json.NewEncoder(w).Encode(profiles); err != nil {
			slog.Error("[App] profiles HTTP encode error", "err", err)
		}
	}))

	go func() {
		a.httpServer = &http.Server{Addr: motionHTTPPort, Handler: mux}
		slog.Info("[App] starting status HTTP server", "addr", motionHTTPPort)
		if err := a.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("[App] status HTTP server failed", "err", err)
		}
	}()
}

func (a *App) Shutdown(ctx context.Context) {
	if a.httpServer != nil {
		slog.Info("[App] shutting down HTTP server")
		a.httpServer.Shutdown(ctx)
	}
}

func (a *App) MotionUpsertProfile(profile core.MotionControllerProfile) error {
	return a.appCtx.MotionManager.UpsertProfile(profile)
}

func (a *App) MotionDeleteProfile(id string) error {
	return a.appCtx.MotionManager.DeleteProfile(id)
}

func (a *App) MotionConnect(id string) error {
	slog.Info("[App] MotionConnect", "id", id)
	err := a.appCtx.MotionManager.Connect(a.ctx, id)
	if err != nil {
		slog.Error("[App] MotionConnect failed", "id", id, "err", err)
	} else {
		slog.Info("[App] MotionConnect success", "id", id)
	}
	return err
}

func (a *App) MotionDisconnect(id string) error {
	return a.appCtx.MotionManager.Disconnect(a.ctx, id)
}

func (a *App) MotionMoveTo(id string, axis string, position float64) error {
	slog.Info("[App] MotionMoveTo", "id", id, "axis", axis, "position", position)
	return a.appCtx.MotionManager.MoveTo(a.ctx, id, core.AxisName(axis), position)
}

func (a *App) MotionMoveBy(id string, axis string, delta float64) error {
	slog.Info("[App] MotionMoveBy", "id", id, "axis", axis, "delta", delta)
	return a.appCtx.MotionManager.MoveBy(a.ctx, id, core.AxisName(axis), delta)
}

func (a *App) MotionJog(id string, axis string, velocity float64) error {
	slog.Info("[App] MotionJog", "id", id, "axis", axis, "velocity", velocity)
	return a.appCtx.MotionManager.Jog(a.ctx, id, core.AxisName(axis), velocity)
}

func (a *App) MotionHome(id string, axis string) error {
	slog.Info("[App] MotionHome", "id", id, "axis", axis)
	return a.appCtx.MotionManager.Home(a.ctx, id, core.AxisName(axis))
}

func (a *App) MotionStop(id string, axis string) error {
	slog.Info("[App] MotionStop", "id", id, "axis", axis)
	return a.appCtx.MotionManager.Stop(a.ctx, id, core.AxisName(axis))
}

func (a *App) MotionEmergencyStop(id string) error {
	slog.Info("[App] MotionEmergencyStop", "id", id)
	return a.appCtx.MotionManager.EmergencyStop(a.ctx, id)
}

func (a *App) MotionResetEmergencyStop(id string) error {
	return a.appCtx.MotionManager.ResetEmergencyStop(a.ctx, id)
}

func (a *App) MotionDefinePosition(id string, axis string, position float64) error {
	return a.appCtx.MotionManager.DefinePosition(a.ctx, id, core.AxisName(axis), position)
}

// MotionGetProfiles 保留为 Wails 绑定兼容（返回空数组）。
// 实际数据通过 HTTP API (http://localhost:16888/api/motion/profiles) 获取，
// 以绕开 Wails v2.12.0 的 reflect 序列化 bug。
func (a *App) MotionGetProfiles() string {
	profiles := a.appCtx.MotionManager.GetProfiles()
	data, _ := json.Marshal(profiles)
	return string(data)
}

// MotionGetStatus 保留为 Wails 绑定兼容（返回空数组）。
// 实际数据通过 HTTP API (http://localhost:16888/api/motion/status) 获取，
// 以绕开 Wails v2.12.0 的 reflect 序列化 bug。
func (a *App) MotionGetStatus() string {
	statuses := a.appCtx.MotionManager.StatusAll(a.ctx)
	data, _ := json.Marshal(statuses)
	return string(data)
}
