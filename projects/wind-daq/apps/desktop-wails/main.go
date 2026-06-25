package main

import (
	"embed"
	"flag"
	"log"
	"log/slog"
	"os"
	"strconv"

	"wind-daq/apps/desktop-wails/backend"

	"github.com/wailsapp/wails/v3/pkg/application"
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
	motionOnlyFromEnv := os.Getenv("WIND_DAQ_MOTION_ONLY") == "1"
	parentPIDValue := *parentPID
	if envParentPID := os.Getenv("WIND_DAQ_PARENT_PID"); envParentPID != "" {
		if pid, err := strconv.Atoi(envParentPID); err == nil {
			parentPIDValue = pid
		}
	}

	// 根据启动模式确定模式字符串与窗口参数
	mode := backend.ModeNormal
	title := "Wind-DAQ"
	width, height := 1600, 900
	minWidth, minHeight := 1440, 900

	if *motionOnly || motionOnlyFromEnv {
		mode = backend.ModeMotion
		title = "运动控制器 - Wind-DAQ"
		// 窗口尺寸根据实际内容调整：4 列轴卡片 + 侧边栏 + 头部约 620px 高
		// 宽度 1280 足够 4 列卡片舒适显示，高度 640 贴合内容避免空白
		width, height = 1280, 640
		minWidth, minHeight = 1100, 580
	}

	app := backend.NewApp(mode)
	if mode == backend.ModeMotion && parentPIDValue > 0 {
		app.SetParentPID(parentPIDValue)
	}

	wailsApp := application.New(application.Options{
		Name:        "Wind-DAQ",
		Description: "Wind-DAQ wind tunnel data acquisition desktop app",
		LogLevel:    slog.LevelInfo,
		Services: []application.Service{
			application.NewService(app),
		},
		Assets: application.AssetOptions{
			Handler: application.BundledAssetFileServer(assets),
		},
		Mac: application.MacOptions{
			ApplicationShouldTerminateAfterLastWindowClosed: true,
		},
	})

	wailsApp.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:         title,
		Width:         width,
		Height:        height,
		MinWidth:      minWidth,
		MinHeight:     minHeight,
		URL:           "/",
		Hidden:        false,
		DisableResize: false,
		BackgroundColour: application.RGBA{
			Red:   7,
			Green: 17,
			Blue:  31,
			Alpha: 1,
		},
	})

	if err := wailsApp.Run(); err != nil {
		log.Fatal(err)
	}
}
