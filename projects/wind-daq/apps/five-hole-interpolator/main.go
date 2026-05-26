package main

import (
	"embed"

	"wind-daq/apps/five-hole-interpolator/backend"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	app := backend.NewApp()

	err := wails.Run(&options.App{
		Title:     "五孔探针插值计算",
		Width:     1400,
		Height:    900,
		MinWidth:  1100,
		MinHeight: 700,
		OnStartup: app.Startup,
		Bind:      []interface{}{app},
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		BackgroundColour: &options.RGBA{
			R: 245,
			G: 247,
			B: 250,
			A: 1,
		},
	})
	if err != nil {
		println("Error:", err.Error())
	}
}
