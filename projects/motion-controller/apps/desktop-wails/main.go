// Win7 版应用入口（运动控制器 Motion Controller）
//
// 与 trunk 主分支的差异：
//   - 移除 Wails v3 依赖（Wails v3 内部用 log/slog + maps + slices，需要 Go 1.21+）
//   - 改为 net/http 启动 HTTP server，前端通过 fetch 调用
//   - GUI 由 Electron 22.3.27（最后兼容 Win7 的 Electron 主版本）承载
//
// 与 wind-daq / probe-interpolator Win7 版的差异：
//   - 端口 16888（wind-daq 主进程用 8900、motion-only 用 8901，probe-interpolator 用 18183）
//   - 后端 motion HTTP API 由 shared.local/motion-control/go/httpapi.RegisterMotionRoutes 注册，
//     含 profiles / status / connect / disconnect / home / moveTo / moveBy / jog / stop /
//     emergencyStop / resetEmergencyStop / definePosition 全套动作命令
//   - 无 WebSocket 事件流，前端通过 200ms 轮询 /api/motion/status 获取实时状态
//   - 无文件选择对话框（motion 配置存于 %AppData%/motion-controller/motion-profiles.json，
//     不暴露给用户手选文件）
//
// 完整 motion 路由由 backend.App.startStatusHTTPServer() 注册，main.go 仅做装配。

package main

import (
	"context"
	"embed"
	"io/fs"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"shared.local/device-sdk/go/pkg/slog"

	"motion-controller/apps/desktop-wails/backend"
	"motion-controller/services/api-go/pkg/appcontext"
)

// listenAddr 是 Go HTTP server 监听地址。
// 端口 16888：与 wind-daq（8900/8901）/ daq-t1603（18181）/ daq-p1604（18182）/ probe-interpolator（18183）区分，
// 避免同机多开冲突。仅监听 127.0.0.1，外部网络无法访问，无 CSRF 风险。
const listenAddr = "127.0.0.1:16888"

//go:embed all:frontend/dist
var frontendAssets embed.FS

func main() {
	// ---- 1. 实例化应用上下文 + 后端 App ----
	// 传空字符串使用默认配置目录（%AppData%/motion-controller/motion-profiles.json）
	appCtx, err := appcontext.NewAppContext("")
	if err != nil {
		slog.Error("AppContext 初始化失败", "error", err)
		os.Exit(1)
	}

	app := backend.NewApp(appCtx)

	// ---- 2. HTTP server ----
	// mux 在 backend.App.registerRoutes 内挂载 /api/motion/* 路由 + CORS 中间件，
	// 这里再补 /api/health（供 Electron 健康检查）和 / 静态资源。
	mux := http.NewServeMux()
	if err := app.RegisterRoutes(mux); err != nil {
		slog.Error("注册路由失败", "error", err)
		os.Exit(1)
	}

	mux.HandleFunc("/api/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})

	// 前端静态资源嵌入到 exe，由 Go http.FileServer 直接提供
	frontendFS, err := fs.Sub(frontendAssets, "frontend/dist")
	if err != nil {
		slog.Error("frontend assets unavailable", "error", err)
		os.Exit(1)
	}
	mux.Handle("/", http.FileServer(http.FS(frontendFS)))

	srv := &http.Server{
		Addr:              listenAddr,
		Handler:           mux,
		ReadHeaderTimeout: 2 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
	}

	// ---- 3. 启动：注入应用 ctx 并调用 ServiceStartup ----
	appCtxGo, appCancel := context.WithCancel(context.Background())
	defer appCancel()

	if err := app.ServiceStartup(appCtxGo); err != nil {
		slog.Error("App startup failed", "error", err)
	}

	go func() {
		slog.Info("HTTP server listening", "addr", listenAddr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("HTTP server error", "error", err)
			appCancel()
		}
	}()

	// ---- 4. 优雅关闭 ----
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	select {
	case <-sigCh:
		slog.Info("received shutdown signal")
	case <-appCtxGo.Done():
	}

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		slog.Error("HTTP server shutdown error", "error", err)
	}

	if err := app.ServiceShutdown(); err != nil {
		slog.Error("App shutdown error", "error", err)
	}
}
