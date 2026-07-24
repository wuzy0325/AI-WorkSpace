// Win7 版应用入口（Wind-DAQ）
//
// 与 trunk 主分支的差异：
//   - 移除 Wails v3 依赖（Wails v3 内部用 log/slog + maps + slices，需 Go 1.21+）
//   - 改为 net/http 启动 HTTP server，前端由 Electron 22.3.27 加载
//   - GUI 由 Electron 主进程承载；Go binary 只负责 HTTP API + 静态资源 serve
//   - motion-only 子进程也通过同一 main.go 入口启动，监听独立端口 8901（避免与主进程 8900 冲突）
//
// HTTP 路由由 api.NewRouter 提供（在 services/api-go/api 包），main.go 仅做装配：
//   - /api/* → api.NewRouter 返回的 handler（已包装 CORS/recover/metrics 中间件）
//   - /      → frontend/dist 静态资源（前端 SPA）

package main

import (
	"context"
	"embed"
	"flag"
	"io/fs"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"shared.local/device-sdk/go/pkg/slog"

	"wind-daq/apps/desktop-wails/backend"
	"wind-daq/services/api-go/api"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	// 解析命令行参数：
	//   --motion-only 启动运动控制器独立窗口（子进程，监听独立端口 8901）
	//   --parent-pid  仅 motion-only 子进程使用，传入父进程 PID，父进程消失时子进程自杀
	motionOnly := flag.Bool("motion-only", false, "以运动控制器独立窗口模式启动")
	parentPID := flag.Int("parent-pid", 0, "父进程 PID（仅 motion-only 模式下使用，父进程退出时子进程一并退出）")
	flag.Parse()
	motionOnlyFromEnv := os.Getenv("WIND_DAQ_MOTION_ONLY") == "1"
	parentPIDValue := *parentPID
	if envParentPID := os.Getenv("WIND_DAQ_PARENT_PID"); envParentPID != "" {
		if pid, err := strconv.Atoi(envParentPID); err == nil {
			parentPIDValue = pid
		}
	}
	isMotionOnly := *motionOnly || motionOnlyFromEnv

	mode := backend.ModeNormal
	if isMotionOnly {
		mode = backend.ModeMotion
	}

	// 实例化 App 并注入父进程 PID（仅 motion-only 子进程需要 watchdog）
	app := backend.NewApp(mode)
	if mode == backend.ModeMotion && parentPIDValue > 0 {
		app.SetParentPID(parentPIDValue)
	}

	// 应用级 ctx，所有后台 goroutine 派生自它，收到信号时统一取消
	appCtx, appCancel := context.WithCancel(context.Background())
	defer appCancel()

	// 启动后台服务（日志、appContext、数据中继、自动连接等）
	// 注意：HTTP server 不在 Start 内启动，由 main.go 自己创建以挂载静态资源
	if err := app.Start(appCtx); err != nil {
		slog.Error("应用启动失败", "component", "main", "error", err)
		os.Exit(1)
	}

	// 监听端口：motion-only 子进程用 8901，避免与主进程 8900 冲突
	listenAddr := "127.0.0.1:8900"
	if isMotionOnly {
		listenAddr = "127.0.0.1:8901"
	}

	// 创建 mux：/api/* 走 API handler，/ 走静态资源
	apiHandler := api.NewRouter(app.NewDeps())
	mux := http.NewServeMux()
	mux.Handle("/api/", apiHandler)

	// 静态资源：frontend/dist 由 embed.FS 嵌入二进制
	// 失败说明构建时未执行 npm run build，直接退出避免运行时 panic
	frontendFS, err := fs.Sub(assets, "frontend/dist")
	if err != nil {
		slog.Error("frontend assets unavailable", "component", "main", "error", err)
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

	// 启动 HTTP server；失败时取消 appCtx 触发优雅关闭
	go func() {
		slog.Info("HTTP server listening", "component", "main", "addr", "http://"+listenAddr, "mode", mode)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("HTTP server error", "component", "main", "error", err)
			appCancel()
		}
	}()

	// 优雅关闭：等待 SIGINT/SIGTERM 或 appCtx.Done
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	select {
	case <-sigCh:
		slog.Info("received shutdown signal", "component", "main")
	case <-appCtx.Done():
	}

	// 先 Shutdown HTTP server（拒绝新连接、等待在途请求完成），
	// 再调用 app.Stop 关闭后台服务（数据中继、运动窗口子进程、日志）
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		slog.Error("HTTP server shutdown error", "component", "main", "error", err)
	}
	_ = app.Stop()
}
