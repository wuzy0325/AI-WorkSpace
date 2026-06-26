package main

import (
	"embed"
	"log"
	"log/slog"

	"five-hole-interpolator/apps/desktop-wails/backend"

	"github.com/wailsapp/wails/v3/pkg/application"
)

//go:embed all:frontend/dist
var assets embed.FS

//go:embed appicon.png
var appIcon []byte

func main() {
	app := backend.NewApp()

	wailsApp := application.New(application.Options{
		Name:        "Five Hole Interpolator",
		Description: "Five-hole probe interpolation desktop app",
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
		Title:     "五孔探针插值计算",
		Width:     1400,
		Height:    900,
		MinWidth:  1100,
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
