package main

import (
	"embed"
	"log/slog"
	"os"
	"runtime"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/windows"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo})))

	app := NewApp()

	winOpts := windows.Options{
		WebviewIsTransparent: false,
		DisableWindowIcon:    false,
	}

	// Win7: point WebView2 to bundled v109 fixed runtime if available.
	if isWin7() {
		if p := findWebView2Runtime(); p != "" {
			winOpts.WebviewBrowserPath = p
			slog.Info("win7: using bundled WebView2 runtime", "path", p)
		}
	}

	err := wails.Run(&options.App{
		Title:     "DAQ MVP",
		Width:     1280,
		Height:    800,
		MinWidth:  960,
		MinHeight: 600,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		OnStartup:  app.Startup,
		OnShutdown: app.Shutdown,
		Bind: []interface{}{
			app,
		},
		Windows: &winOpts,
	})
	if err != nil {
		slog.Error("wails run failed", "err", err)
		os.Exit(1)
	}
}

func isWin7() bool {
	return runtime.GOOS == "windows" && os.Getenv("WIN7_COMPAT") == "1"
}

func findWebView2Runtime() string {
	paths := []string{
		".\\webview2\\msedgewebview2.exe",
		os.Getenv("WEBVIEW2_BROWSER_EXECUTABLE_FOLDER"),
		os.TempDir() + "\\daq-mvp-webview2\\msedgewebview2.exe",
	}
	for _, p := range paths {
		if p != "" {
			if _, err := os.Stat(p); err == nil {
				return p
			}
		}
	}
	return ""
}
