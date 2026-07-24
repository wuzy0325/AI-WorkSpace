package backend

import (
	"context"
	"net/http"
	"strings"

	"motion-controller/services/api-go/pkg/appcontext"
	motionhttp "shared.local/motion-control/go/httpapi"

	"shared.local/device-sdk/go/pkg/slog"
)

// App 是运动控制器后端的核心装配类。
//
// Win7 版与 trunk 主分支的差异：
//   - 移除 Wails v3 application.Service 接口依赖
//   - 不再启动独立 HTTP server goroutine，改为 RegisterRoutes 注册到 main.go 持有的 mux
//   - HTTP server 的生命周期由 main.go 控制（main.go 负责 ListenAndServe + Shutdown）
//   - 移除 Wails 绑定方法（MotionUpsertProfile / MotionConnect / ...），motion 全部走 HTTP
//
// 业务逻辑（运动控制器连接、回零、移动、急停等）由 appCtx.MotionManager 提供，
// HTTP 路由由 shared.local/motion-control/go/httpapi.RegisterMotionRoutes 挂载。
type App struct {
	ctx    context.Context
	appCtx *appcontext.AppContext
}

// NewApp 创建后端 App 实例。
// appCtx 由调用方负责初始化（在 main.go 中通过 appcontext.NewAppContext 创建）。
func NewApp(appCtx *appcontext.AppContext) *App {
	return &App{appCtx: appCtx}
}

// ServiceStartup 完成应用启动时的初始化工作：
//   - 加载已保存的运动控制器配置（motion-profiles.json）
//
// 与 trunk 主分支的差异：不再启动 HTTP server goroutine，路由注册由 RegisterRoutes 完成，
// HTTP server 启动由 main.go 控制。
func (a *App) ServiceStartup(ctx context.Context) error {
	a.ctx = ctx
	a.appCtx.MotionManager.LoadProfiles()
	return nil
}

// RegisterRoutes 把所有 motion HTTP 路由挂载到传入的 mux 上，并套上 CORS 中间件。
//
// 路由清单（由 shared.local/motion-control/go/httpapi.RegisterMotionRoutes 注册）：
//   - GET    /api/motion/profiles      列出所有控制器配置
//   - PUT    /api/motion/profiles      新增/更新控制器配置
//   - DELETE /api/motion/profiles/{id} 删除控制器配置
//   - GET    /api/motion/status        获取所有控制器实时状态
//   - POST   /api/motion/connect       连接控制器
//   - POST   /api/motion/disconnect    断开控制器
//   - POST   /api/motion/home          回零
//   - POST   /api/motion/moveTo        绝对移动
//   - POST   /api/motion/moveBy        相对移动
//   - POST   /api/motion/jog           点动
//   - POST   /api/motion/stop          停止
//   - POST   /api/motion/emergencyStop 紧急停止
//   - POST   /api/motion/resetEmergencyStop 解除紧急停止
//   - POST   /api/motion/definePosition 定义当前位置
//
// CORS 中间件作用：
//   - 开发态：Vite dev server (http://127.0.0.1:9245) 跨域请求后端时需要 CORS 头
//   - 生产态：Electron 加载 http://127.0.0.1:16888 与后端同源，CORS 不触发
//   - origin 为空（如 curl）：返回 Access-Control-Allow-Origin: null，允许同源 request
func (a *App) RegisterRoutes(mux *http.ServeMux) error {
	corsMux := http.NewServeMux()
	motionhttp.RegisterMotionRoutes(corsMux, a.appCtx.MotionManager)

	mux.Handle("/api/motion/", corsMiddleware(corsMux))
	return nil
}

// corsMiddleware 为内层 handler 添加 CORS 响应头，处理跨域预检请求。
//
// 安全设计：
//   - 仅允许本机 origin（http://localhost:* / http://127.0.0.1:* / 空 origin）
//   - 其他 origin 返回 403 Forbidden，防止 DNS 重绑定攻击
//   - OPTIONS 预检请求直接返回 204 No Content
func corsMiddleware(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin == "" {
			// 同源请求（Electron 生产态、curl）：返回 null 允许同源 fetch
			w.Header().Set("Access-Control-Allow-Origin", "null")
		} else if isAllowedMotionOrigin(origin) {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Vary", "Origin")
		} else {
			http.Error(w, "origin not allowed", http.StatusForbidden)
			return
		}
		w.Header().Set("Access-Control-Allow-Methods", "GET, PUT, POST, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		h.ServeHTTP(w, r)
	})
}

// isAllowedMotionOrigin 判断 origin 是否在白名单中。
//
// Win7 版与 trunk 主分支的差异：移除 wails://wails.localhost / http://wails.localhost 等
// Wails 专属协议白名单（Electron 用 http://127.0.0.1:16888 加载前端，与后端同源）。
func isAllowedMotionOrigin(origin string) bool {
	return strings.HasPrefix(origin, "http://localhost:") ||
		strings.HasPrefix(origin, "http://127.0.0.1:")
}

// ServiceShutdown 在应用退出时清理资源。
//
// Win7 版与 trunk 主分支的差异：不再 Shutdown HTTP server（由 main.go 统一管理），
// 这里仅作为生命周期钩子保留，便于未来扩展（如关闭硬件连接、flush 日志等）。
func (a *App) ServiceShutdown() error {
	slog.Info("[App] shutting down")
	return nil
}
