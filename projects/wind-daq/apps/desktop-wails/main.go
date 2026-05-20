package main

import (
	"embed"

	"wind-daq/apps/desktop-wails/backend"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	app := backend.NewApp()

	err := wails.Run(&options.App{
		Title:         "Wind-DAQ",
		Width:         1440,
		Height:        900,
		MinWidth:      1280,
		MinHeight:     720,
		OnStartup:     app.Startup,
		Bind:          []interface{}{app},
		AssetServer:   &assetserver.Options{Assets: assets},
		StartHidden:   false,
		DisableResize: false,
		BackgroundColour: &options.RGBA{
			R: 7,
			G: 17,
			B: 31,
			A: 1,
		},
	})
	if err != nil {
		println("wails run error:", err.Error())
	}
}
