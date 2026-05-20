package backend

import "context"

type App struct {
	ctx context.Context
}

type VersionInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

func NewApp() *App {
	return &App{}
}

func (a *App) Startup(ctx context.Context) {
	a.ctx = ctx
}

func (a *App) GetVersion() VersionInfo {
	return VersionInfo{Name: "Wind-DAQ", Version: "0.1.0-rebuild"}
}
