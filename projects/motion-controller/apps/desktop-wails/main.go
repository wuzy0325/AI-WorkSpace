package main

import (
	"embed"
	"log"
	"log/slog"
	"os"

	"github.com/wailsapp/wails/v3/pkg/application"

	"motion-controller/apps/desktop-wails/backend"
	"motion-controller/services/api-go/pkg/appcontext"
)

//go:embed all:frontend/dist
var assets embed.FS

//go:embed build/appicon.png
var appIcon []byte

// fitFixedWindowToScreen 将固定尺寸窗口按主屏逻辑工作区钳制。
// 部分机器显示缩放较大（如 1920x1080@150% 时逻辑工作区仅约 1280x660），
// 固定 1350x740 的窗口会超出屏幕导致画面看不全且无法调整大小。
// 工作区放不下时把宽高连同 Min/Max 一并钳制到工作区，保持禁用缩放语义；
// 不做最大化：固定布局窗口放大无意义，缩小到可容纳尺寸即可完整可见。
func fitFixedWindowToScreen(app *application.App, opts application.WebviewWindowOptions) application.WebviewWindowOptions {
	screen := app.Screen.GetPrimary()
	if screen == nil || screen.WorkArea.Width <= 0 || screen.WorkArea.Height <= 0 {
		return opts
	}
	if screen.WorkArea.Width >= opts.Width && screen.WorkArea.Height >= opts.Height {
		return opts
	}
	w := min(opts.Width, screen.WorkArea.Width)
	h := min(opts.Height, screen.WorkArea.Height)
	opts.Width, opts.MinWidth, opts.MaxWidth = w, w, w
	opts.Height, opts.MinHeight, opts.MaxHeight = h, h, h
	return opts
}

func main() {
	appCtx, err := appcontext.NewAppContext("")
	if err != nil {
		println("Error initializing:", err.Error())
		return
	}

	app := backend.NewApp(appCtx)

	appOpts := application.Options{
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
	}
	if cdpPort := os.Getenv("MOTION_CONTROLLER_CDP_PORT"); cdpPort != "" {
		slog.Info("CDP debugging enabled (E2E test mode)", "port", cdpPort)
		// go-webview2 会清空 WEBVIEW2_ADDITIONAL_BROWSER_ARGUMENTS，必须走
		// WindowsOptions.AdditionalBrowserArgs（与 daq-p1604 的 E2E 机制一致）。
		appOpts.Windows.AdditionalBrowserArgs = []string{"--remote-debugging-port=" + cdpPort}
	}
	wailsApp := application.New(appOpts)

	wailsApp.Window.NewWithOptions(fitFixedWindowToScreen(wailsApp, application.WebviewWindowOptions{
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
	}))

	if err := wailsApp.Run(); err != nil {
		log.Fatal(err)
	}
}
