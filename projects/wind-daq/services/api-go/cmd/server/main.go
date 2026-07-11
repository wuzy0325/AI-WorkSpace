package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"shared.local/device-sdk/go/ffi"

	"wind-daq/services/api-go/internal/bootstrap"
	"wind-daq/services/api-go/pkg/debugserver"
	"wind-daq/services/api-go/pkg/logging"
)

func main() {
	// 初始化日志系统：stderr + 文件轮转 + ring buffer（供 SSE 拉取）
	// 桌面模式（Wails）在 app.go 中初始化，独立服务器模式在此初始化。
	logDir := envOrDefault("WIND_DAQ_LOG_DIR", filepath.Join("data", "logs"))
	logMgr, err := logging.Init(logging.Default(logDir))
	if err != nil {
		slog.Error("日志系统初始化失败，使用默认 stderr 输出", "error", err)
	} else {
		defer func() {
			_ = logMgr.Close()
		}()
		slog.Info("日志系统已初始化", "component", "main", "logDir", logDir, "level", "info")
	}

	// 初始化 WTNDAQ16H DLL（DAQ-P-1603 16 通道 AI 采集设备所需）。
	// 路径：环境变量 WTNDAQ16H_DLL_PATH 或可执行文件同目录。
	// 加载失败不阻止启动——无 DAQ-P-1603 设备时仍可正常使用其他设备类型。
	ffi.InitWTNDAQ16HFromEnv()

	// 获取 ring buffer 和 manager，传递给 API server（用于日志 SSE 端点和分类开关）
	var ringBuf *logging.RingBuffer
	var mgr *logging.Manager
	if logMgr != nil {
		ringBuf = logMgr.Ring()
		mgr = logMgr
	}

	server, err := bootstrap.BuildAPIServer(bootstrap.Config{
		Address:          envOrDefault("WIND_DAQ_ADDR", bootstrap.DefaultAddress),
		ProfileStorePath: envOrDefault("WIND_DAQ_PROFILE_PATH", bootstrap.DefaultProfileStorePath),
		LogRing:          ringBuf,
		LogManager:       mgr,
	})
	if err != nil {
		slog.Error("initialize api server", "err", err)
		os.Exit(1)
	}

	// 按需启动 pprof debug server（受 WINDDAQ_PPROF_ADDR 环境变量控制）
	debugCtx, debugCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer debugCancel()
	if _, err := debugserver.Start(debugCtx); err != nil {
		slog.Warn("debug server start failed", "err", err)
	}

	slog.Info("wind-daq api server starting", "addr", server.Address)
	if err := http.ListenAndServe(server.Address, server.Handler); err != nil {
		slog.Error("server stopped", "err", err)
		os.Exit(1)
	}
}

func envOrDefault(key string, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
