package main

import (
	"embed"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"

	"motion-controller/apps/desktop-wails/backend"
	"motion-controller/services/api-go/pkg/appcontext"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	appCtx, err := appcontext.NewAppContext("")
	if err != nil {
		println("Error initializing:", err.Error())
		return
	}

	app := backend.NewApp(appCtx)

	err = wails.Run(&options.App{
		Title:  "Motion Controller",
		Width:  1280,
		Height: 800,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		OnStartup:  app.Startup,
		OnShutdown: app.Shutdown,
		Bind: []interface{}{
			app,
		},
	})
	if err != nil {
		println("Error:", err.Error())
	}
}
