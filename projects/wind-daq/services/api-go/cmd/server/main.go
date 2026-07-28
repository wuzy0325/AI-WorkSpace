package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"shared.local/device-sdk/go/ffi"

	"wind-daq/services/api-go/internal/bootstrap"
	"wind-daq/services/api-go/pkg/debugserver"
	"wind-daq/services/api-go/pkg/logging"
)

// registryShutdownTimeout 有序关停总时限（须大于 registry hard deadline 10s，
// 保证有界 EmergencyStop 尝试结束或 hard deadline 到达后才触发 fatal exit）。
const registryShutdownTimeout = 15 * time.Second

func main() {
	logMgr, err := initLogging()
	if err == nil {
		defer func() { _ = logMgr.Close() }()
	}
	initAuxServices()

	server, err := bootstrap.BuildAPIServer(bootstrap.Config{
		Address:          envOrDefault("WIND_DAQ_ADDR", bootstrap.DefaultAddress),
		ProfileStorePath: envOrDefault("WIND_DAQ_PROFILE_PATH", bootstrap.DefaultProfileStorePath),
		LogRing:          logRingOf(logMgr),
		LogManager:       logMgr,
	})
	if err != nil {
		slog.Error("initialize api server", "err", err)
		os.Exit(1)
	}

	// signal.NotifyContext 拥有 http.Server 生命周期（spec FR9）：
	// 收到 SIGINT/SIGTERM 先执行 registry 有序关停，再关闭 HTTP/共享服务，
	// 不再裸 http.ListenAndServe 阻塞至进程被杀。
	signalCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	httpServer := &http.Server{Addr: server.Address, Handler: server.Handler}
	errCh := make(chan error, 1)
	go func() { errCh <- httpServer.ListenAndServe() }()

	slog.Info("wind-daq api server starting", "addr", server.Address)
	select {
	case <-signalCtx.Done():
		slog.Info("收到退出信号，开始有序关停", "component", "main")
		if code := runShutdown(server.TraversalRegistry, httpServer, registryShutdownTimeout); code != 0 {
			os.Exit(code)
		}
		// 正常路径经 return 退出，defer（logMgr.Close 等）照常执行
		return
	case err := <-errCh:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("server stopped", "err", err)
			os.Exit(1)
		}
	}
}

// initAuxServices 初始化 WTNDAQ16H DLL 与 pprof debug server（失败均不阻止启动）。
func initAuxServices() {
	// 初始化 WTNDAQ16H DLL（DAQ-P-1603 16 通道 AI 采集设备所需）。
	// 路径：环境变量 WTNDAQ16H_DLL_PATH 或可执行文件同目录。
	ffi.InitWTNDAQ16HFromEnv()

	// 按需启动 pprof debug server（受 WINDDAQ_PPROF_ADDR 环境变量控制）
	debugCtx, debugCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer debugCancel()
	if _, err := debugserver.Start(debugCtx); err != nil {
		slog.Warn("debug server start failed", "err", err)
	}
}

// initLogging 初始化日志系统（stderr + 文件轮转 + ring buffer，供 SSE 拉取）。
// 桌面模式（Wails）在 app.go 中初始化，独立服务器模式在此初始化。
func initLogging() (*logging.Manager, error) {
	logDir := envOrDefault("WIND_DAQ_LOG_DIR", filepath.Join("data", "logs"))
	logMgr, err := logging.Init(logging.Default(logDir))
	if err != nil {
		slog.Error("日志系统初始化失败，使用默认 stderr 输出", "error", err)
		return nil, err
	}
	slog.Info("日志系统已初始化", "component", "main", "logDir", logDir, "level", "info")
	return logMgr, nil
}

// logRingOf 返回日志 ring buffer（日志初始化失败时为 nil）。
func logRingOf(logMgr *logging.Manager) *logging.RingBuffer {
	if logMgr == nil {
		return nil
	}
	return logMgr.Ring()
}

// registryShutter registry 关停接口（*usecase.ManagerRegistry 满足；测试注入 fake）。
type registryShutter interface {
	Shutdown(ctx context.Context) error
}

// httpShutter HTTP 关停接口（*http.Server 满足；测试注入 fake 观测调用）。
type httpShutter interface {
	Shutdown(ctx context.Context) error
}

// runShutdown 收到信号后的有序关停序列（spec FR9）：
//  1. registry Shutdown（双 deadline + 有界 EmergencyStop）；
//     失败时记录 fatal、跳过共享服务 Close（HTTP 不再优雅关闭）并返回非零退出码；
//  2. 成功后优雅关闭 HTTP 监听。
func runShutdown(registry registryShutter, httpServer httpShutter, timeout time.Duration) int {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	if registry != nil {
		if err := registry.Shutdown(ctx); err != nil {
			slog.Error("fatal: traversal registry shutdown 失败，跳过共享服务关闭并以非零退出",
				"component", "main", "error", err)
			return 2
		}
	}
	shutdownHTTP(httpServer)
	return 0
}

// shutdownHTTP 优雅关闭 HTTP 监听（5s 有界）。
func shutdownHTTP(httpServer httpShutter) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := httpServer.Shutdown(ctx); err != nil {
		slog.Warn("http server shutdown", "component", "main", "err", err)
	}
}

func envOrDefault(key string, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
