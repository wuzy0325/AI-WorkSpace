package backend

import (
	"context"
	"fmt"
	"log"
	"net"
	"os"

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

	port := a.findAvailablePort()
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

func (a *App) findAvailablePort() int {
	preferred := envInt("WIND_DAQ_PORT", 8080)
	ports := []int{preferred, 8081, 8082, 9090, 9091}
	for _, port := range ports {
		ln, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
		if err == nil {
			ln.Close()
			return port
		}
	}
	return 0
}

func envInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		var n int
		if _, err := fmt.Sscanf(v, "%d", &n); err == nil {
			return n
		}
	}
	return fallback
}
