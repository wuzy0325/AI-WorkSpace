package backend

import (
	"context"
	"fmt"
	"log"

	"wind-daq/services/api-go/pkg/apiserver"
)

type App struct {
	ctx    context.Context
	cancel context.CancelFunc
	port   int
}

type VersionInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	Port    int    `json:"port"`
}

func NewApp() *App {
	return &App{}
}

func (a *App) Startup(ctx context.Context) {
	a.ctx, a.cancel = context.WithCancel(ctx)

	port := apiserver.FindAvailablePort(apiserver.EnvInt("WIND_DAQ_PORT", 8080))
	if port == 0 {
		log.Printf("WARNING: no available port found, API server not started")
		return
	}

	_, err := apiserver.Start(a.ctx, fmt.Sprintf(":%d", port))
	if err != nil {
		log.Printf("API server start error on port %d: %v", port, err)
		return
	}
	a.port = port
	log.Printf("API server started on :%d", port)
}

func (a *App) Shutdown(ctx context.Context) {
	if a.cancel != nil {
		a.cancel()
	}
}

func (a *App) GetVersion() VersionInfo {
	return VersionInfo{Name: "Wind-DAQ", Version: "0.1.0-rebuild", Port: a.port}
}
