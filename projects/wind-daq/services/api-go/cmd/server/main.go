package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"

	"wind-daq/services/api-go/internal/bootstrap"
	"wind-daq/services/api-go/pkg/debugserver"
)

func main() {
	server, err := bootstrap.BuildAPIServer(bootstrap.Config{
		Address:          envOrDefault("WIND_DAQ_ADDR", bootstrap.DefaultAddress),
		ProfileStorePath: envOrDefault("WIND_DAQ_PROFILE_PATH", bootstrap.DefaultProfileStorePath),
	})
	if err != nil {
		slog.Error("initialize api server", "err", err)
		os.Exit(1)
	}

	// 按需启动 pprof debug server（受 WINDDAQ_PPROF_ADDR 环境变量控制）
	debugCtx, debugCancel := context.WithCancel(context.Background())
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
