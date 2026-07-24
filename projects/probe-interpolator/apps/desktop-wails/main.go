// Win7 版应用入口（探针插值器 Probe Interpolator）
//
// 与 trunk 主分支的差异：
//   - 移除 Wails v3 依赖（Wails v3 内部用 log/slog + maps + slices，需要 Go 1.21+）
//   - 改为 net/http 启动 HTTP server，前端通过 fetch 调用
//   - GUI 由 Electron 22.3.27（最后兼容 Win7 的 Electron 主版本）承载
//
// 与 daq-p1604 / daq-t1603 Win7 版的差异：
//   - 端口 18183（daq-p1604 用 18182，daq-t1603 用 18181）
//   - 无硬件设备通信、无 WebSocket 事件流（纯请求-响应模式）
//   - 无 hub / eventbus / usecase 层（业务直接在 backend 内）
//   - 文件选择对话框由 Electron IPC 处理，后端仅接收文件路径参数
//
// 完整 HTTP handler 在 httpserver/ 包实现，main.go 仅做装配。

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

	"probe-interpolator/backend"
	"probe-interpolator/httpserver"
)

// listenAddr 是 Go HTTP server 监听地址。
// 端口 18183：与 daq-p1604（18182）/ daq-t1603（18181）区分，避免同机多开冲突。
// 仅监听 127.0.0.1，外部网络无法访问，无 CSRF 风险。
const listenAddr = "127.0.0.1:18183"

//go:embed all:frontend/dist
var frontendAssets embed.FS

func main() {
	// ---- 1. 实例化 App（共享探针选择状态 + 5/3/7 孔插值缓存）----
	app := backend.NewApp()

	// ---- 2. HTTP server ----
	mux := http.NewServeMux()
	httpserver.RegisterHandlers(mux, app)

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
		ReadHeaderTimeout: 10 * time.Second,
	}

	// ---- 3. 启动：注入应用 ctx 并调用 ServiceStartup ----
	appCtx, appCancel := context.WithCancel(context.Background())
	defer appCancel()

	if err := app.ServiceStartup(appCtx); err != nil {
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
	case <-appCtx.Done():
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
