package main

import (
	"embed"
	"log"
	"log/slog"

	"probe-interpolator/apps/desktop-wails/backend"

	"github.com/wailsapp/wails/v3/pkg/application"
)

//go:embed all:frontend/dist
var assets embed.FS

//go:embed app_icon.png
var appIcon []byte

func main() {
	app := backend.NewApp()

	wailsApp := application.New(application.Options{
		Name:        "Probe Interpolator",
		Description: "3/5/7-hole probe interpolation desktop app",
		LogLevel:    slog.LevelInfo,
		Services: []application.Service{
			application.NewService(app),
		},
		Assets: application.AssetOptions{
			Handler: application.BundledAssetFileServer(assets),
		},
		Icon: appIcon,
		Mac: application.MacOptions{
			ApplicationShouldTerminateAfterLastWindowClosed: true,
		},
	})

	wailsApp.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:    "探针插值器",
		Width:    1400,
		Height:   900,
		MinWidth: 1100,
		MinHeight: 700,
		URL:       "/",
		Hidden:    false,
		BackgroundColour: application.RGBA{
			Red:   245,
			Green: 247,
			Blue:  250,
			Alpha: 1,
		},
	})

	if err := wailsApp.Run(); err != nil {
		log.Fatalf("启动应用失败: %v", err)
	}
}
