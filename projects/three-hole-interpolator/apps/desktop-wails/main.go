package main

import (
	"embed"
	"log"
	"log/slog"

	"three-hole-interpolator/apps/desktop-wails/backend"

	"github.com/wailsapp/wails/v3/pkg/application"
)

//go:embed all:frontend/dist
var assets embed.FS

//go:embed appicon.png
var appIcon []byte

// 窗口设计尺寸与最小可交互尺寸（逻辑像素，非物理像素）。
const (
	windowDesignWidth  = 1400
	windowDesignHeight = 900
	windowMinWidth     = 1024
	windowMinHeight    = 640
)

// fitWindowToScreen 按主屏逻辑工作区钳制窗口初始尺寸。
// 部分机器显示缩放较大（如 1920x1080@150% 时逻辑工作区仅约 1280x660），
// 固定 1400x900 的初始窗口会超出屏幕导致画面看不全。
// 工作区放不下设计尺寸时：初始尺寸钳制到工作区并最大化启动，
// 同时放宽最小尺寸，保证还原后仍可手动缩小窗口。
func fitWindowToScreen(app *application.App, opts application.WebviewWindowOptions) application.WebviewWindowOptions {
	opts.MinWidth = windowMinWidth
	opts.MinHeight = windowMinHeight
	screen := app.Screen.GetPrimary()
	if screen == nil || screen.WorkArea.Width <= 0 || screen.WorkArea.Height <= 0 {
		return opts
	}
	if screen.WorkArea.Width < windowDesignWidth || screen.WorkArea.Height < windowDesignHeight {
		opts.Width = min(windowDesignWidth, screen.WorkArea.Width)
		opts.Height = min(windowDesignHeight, screen.WorkArea.Height)
		opts.StartState = application.WindowStateMaximised
	}
	return opts
}

func main() {
	app := backend.NewApp()

	wailsApp := application.New(application.Options{
		Name:        "Three Hole Interpolator",
		Description: "Three-hole probe interpolation desktop app",
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

	wailsApp.Window.NewWithOptions(fitWindowToScreen(wailsApp, application.WebviewWindowOptions{
		Title: "三孔探针插值计算",
		URL:   "/",
		// Width/Height/MinWidth/MinHeight 由 fitWindowToScreen 按主屏逻辑工作区决定
		Hidden: false,
		BackgroundColour: application.RGBA{
			Red:   245,
			Green: 247,
			Blue:  250,
			Alpha: 1,
		},
	}))

	if err := wailsApp.Run(); err != nil {
		log.Fatalf("启动应用失败: %v", err)
	}
}
