package main

import (
	"embed"
	"log"

	"three-hole-interpolator/apps/desktop-wails/backend"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	app := backend.NewApp()

	err := wails.Run(&options.App{
		Title:     "三孔探针插值计算",
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
			R: 245, G: 247, B: 250, A: 1,
		},
	})
	if err != nil {
		log.Fatalf("启动应用失败: %v", err)
	}
}
