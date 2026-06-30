package main

import (
	"embed"
	"log"
	"log/slog"

	"github.com/wailsapp/wails/v3/pkg/application"

	"motion-controller/apps/desktop-wails/backend"
	"motion-controller/services/api-go/pkg/appcontext"
)

//go:embed all:frontend/dist
var assets embed.FS

//go:embed build/appicon.png
var appIcon []byte

func main() {
	appCtx, err := appcontext.NewAppContext("")
	if err != nil {
		println("Error initializing:", err.Error())
		return
	}

	app := backend.NewApp(appCtx)

	wailsApp := application.New(application.Options{
		Name:        "Motion Controller",
		Description: "Standalone motion controller desktop app",
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
		Title:         "Motion Controller",
		Width:         1350,
		Height:        740,
		MinWidth:      1350,
		MinHeight:     740,
		MaxWidth:      1350,
		MaxHeight:     740,
		URL:           "/",
		Hidden:        false,
		DisableResize: true,
	})

	if err := wailsApp.Run(); err != nil {
		log.Fatal(err)
	}
}
