package main

import (
	"embed"
	"flag"

	"wind-daq/apps/desktop-wails/backend"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	// 解析命令行参数：
	//   --motion-only 启动运动控制器独立窗口
	//   --parent-pid  仅子进程使用，传入父进程 PID，父进程消失时子进程自杀
	motionOnly := flag.Bool("motion-only", false, "以运动控制器独立窗口模式启动")
	parentPID := flag.Int("parent-pid", 0, "父进程 PID（仅 motion-only 模式下使用，父进程退出时子进程一并退出）")
	flag.Parse()

	// 根据启动模式确定模式字符串与窗口参数
	mode := backend.ModeNormal
	title := "Wind-DAQ"
	width, height := 1600, 900
	minWidth, minHeight := 1440, 900

	if *motionOnly {
		mode = backend.ModeMotion
		title = "运动控制器 - Wind-DAQ"
		// 窗口尺寸根据实际内容调整：4 列轴卡片 + 侧边栏 + 头部约 620px 高
		// 宽度 1280 足够 4 列卡片舒适显示，高度 640 贴合内容避免空白
		width, height = 1280, 640
		minWidth, minHeight = 1100, 580
	}

	app := backend.NewApp(mode)
	if *motionOnly && *parentPID > 0 {
		app.SetParentPID(*parentPID)
	}

	err := wails.Run(&options.App{
		Title:         title,
		Width:         width,
		Height:        height,
		MinWidth:      minWidth,
		MinHeight:     minHeight,
		OnStartup:     app.Startup,
		OnShutdown:    app.Shutdown,
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
