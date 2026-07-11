package main

import (
	"embed"
	"flag"
	"log"
	"log/slog"
	"os"
	"strconv"

	"shared.local/device-sdk/go/ffi"

	"wind-daq/apps/desktop-wails/backend"

	"github.com/wailsapp/wails/v3/pkg/application"
)

//go:embed all:frontend/dist
var assets embed.FS

//go:embed build/appicon.png
var appIcon []byte

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

	// 初始化 WTNDAQ16H DLL（DAQ-P-1603 设备所需）。
	// 运动控制器独立窗口不需要此 DLL，但 sync.Once 保证安全。
	ffi.InitWTNDAQ16HFromEnv()

	// 根据启动模式确定模式字符串与窗口参数
	mode := backend.ModeNormal
	title := "Wind-DAQ"
	width, height := 1600, 900
	minWidth, minHeight := 1440, 900
	// 独立窗口模式下禁用缩放，避免用户拉小到 4 列轴卡片断点以下导致 UI 错乱
	disableResize := false

	if *motionOnly || motionOnlyFromEnv {
		mode = backend.ModeMotion
		title = "运动控制器 - Wind-DAQ"
		// 独立窗口完全固定尺寸 + 禁用缩放，避免用户拉小到 4 列轴卡片断点以下导致 UI 错乱。
		// 对齐 motion-controller 独立窗口做法（Min=Max=Default + DisableResize）。
		// 布局依据：sidebar 244px + gap 16px + section padding 40px + 4 列卡片（lg 断点 ≥1024px）
		// 实测 1280×640 是容纳 4 列卡片 + 头部 + 操作区的最小舒适尺寸。
		width, height = 1280, 640
		minWidth, minHeight = 1280, 640
		disableResize = true
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
		Icon: appIcon,
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
		DisableResize: disableResize,
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
