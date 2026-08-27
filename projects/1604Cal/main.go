package main

import (
	"embed"
	"log"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/windows"

	"shared.local/device-sdk/go/ffi"
)

//go:embed all:web/dist
var assets embed.FS

//go:embed templates/reports/2m.xlsx
//go:embed templates/reports/2s.xlsx
//go:embed templates/reports/3m.xlsx
//go:embed templates/reports/3s.xlsx
//go:embed templates/reports/4m.xlsx
//go:embed templates/reports/4s.xlsx
//go:embed templates/reports/5m.xlsx
//go:embed templates/reports/5s.xlsx
//go:embed templates/reports/6m.xlsx
//go:embed templates/reports/6s.xlsx
//go:embed templates/reports/7m.xlsx
//go:embed templates/reports/7s.xlsx
//go:embed templates/reports/8m.xlsx
//go:embed templates/reports/8s.xlsx
//go:embed templates/reports/9m.xlsx
//go:embed templates/reports/9s.xlsx
//go:embed templates/reports/10m.xlsx
//go:embed templates/reports/10s.xlsx
//go:embed templates/reports/11m.xlsx
//go:embed templates/reports/11s.xlsx
var templateAssets embed.FS

func main() {
	app := NewApp()

	// 初始化 WTNDAQ16H DLL（DAQ-P-1603 16 通道 AI 采集设备所需）。
	// 路径：环境变量 WTNDAQ16H_DLL_PATH 或可执行文件同目录（对齐 WindLabX4）。
	// DLL 缺失/加载失败仅告警不阻塞启动——未配置 P1603 时应用应正常使用。
	ffi.InitWTNDAQ16HFromEnv()

	err := wails.Run(&options.App{
		Title:             "Cal1604 校准系统",
		Width:             1660,
		Height:            1040,
		WindowStartState:  options.Maximised,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		OnStartup:  app.startup,
		OnShutdown: app.shutdown,
		Bind: []interface{}{
			app,
		},
		Windows: &windows.Options{
			WebviewIsTransparent: false,
			WindowIsTranslucent:  false,
		},
	})

	if err != nil {
		log.Fatalf("wails run failed: %v", err)
	}
}
