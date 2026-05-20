package main

import (
	"log/slog"
	"net/http"
	"os"

	"wind-daq/services/api-go/internal/bootstrap"
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
