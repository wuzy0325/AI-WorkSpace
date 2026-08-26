package main

import (
	"embed"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"time"

	"shared.local/device-sdk/go/ffi"

	"windlabx4/apps/desktop-wails/backend"

	"github.com/wailsapp/wails/v3/pkg/application"
)

//go:embed all:frontend/dist
var assets embed.FS

//go:embed build/appicon.png
var appIcon []byte

// fitWindowToScreen 按主屏逻辑工作区钳制窗口初始尺寸。
// 部分机器显示缩放较大（如 1920x1080@150% 时逻辑工作区仅约 1280x660），
// 固定初始窗口会超出屏幕导致画面看不全。
// 可缩放模式（normal）：初始尺寸钳制到工作区并最大化启动；
// 最小尺寸保持不变，保护 4 列轴卡片布局断点。
// 固定尺寸模式（motion-only，DisableResize）：宽高连同 Min/Max 一并钳制到工作区，
// 保持禁用缩放语义，不做最大化。
func fitWindowToScreen(app *application.App, opts application.WebviewWindowOptions) application.WebviewWindowOptions {
	screen := app.Screen.GetPrimary()
	if screen == nil || screen.WorkArea.Width <= 0 || screen.WorkArea.Height <= 0 {
		return opts
	}
	if opts.DisableResize {
		if screen.WorkArea.Width >= opts.Width && screen.WorkArea.Height >= opts.Height {
			return opts
		}
		w := min(opts.Width, screen.WorkArea.Width)
		h := min(opts.Height, screen.WorkArea.Height)
		opts.Width, opts.MinWidth, opts.MaxWidth = w, w, w
		opts.Height, opts.MinHeight, opts.MaxHeight = h, h, h
		return opts
	}
	if screen.WorkArea.Width < opts.Width || screen.WorkArea.Height < opts.Height {
		opts.Width = min(opts.Width, screen.WorkArea.Width)
		opts.Height = min(opts.Height, screen.WorkArea.Height)
		opts.StartState = application.WindowStateMaximised
	}
	return opts
}

func main() {
	// 早期 panic recovery：捕获所有 main goroutine 启动期 panic 并写入 crash log。
	// 必要性：windowsgui 子系统下 stderr 不可见，Wails 日志系统尚未初始化时
	// 任何 panic 都会让进程静默退出，用户看不到任何错误。crash log 是唯一的诊断手段。
	defer func() {
		if r := recover(); r != nil {
			writeCrashLog(fmt.Sprintf("启动 panic: %v", r))
			os.Exit(1)
		}
	}()

	// 解析命令行参数：
	//   --motion-only 启动运动控制器独立窗口
	//   --parent-pid  仅子进程使用，传入父进程 PID，父进程消失时子进程自杀
	motionOnly := flag.Bool("motion-only", false, "以运动控制器独立窗口模式启动")
	parentPID := flag.Int("parent-pid", 0, "父进程 PID（仅 motion-only 模式下使用，父进程退出时子进程一并退出）")
	flag.Parse()
	motionOnlyFromEnv := os.Getenv("WINDLABX4_MOTION_ONLY") == "1"
	parentPIDValue := *parentPID
	if envParentPID := os.Getenv("WINDLABX4_PARENT_PID"); envParentPID != "" {
		if pid, err := strconv.Atoi(envParentPID); err == nil {
			parentPIDValue = pid
		}
	}

	// 初始化 WTNDAQ16H DLL（DAQ-P-1603 设备所需）。
	// 运动控制器独立窗口不需要此 DLL，但 sync.Once 保证安全。
	ffi.InitWTNDAQ16HFromEnv()

	// 根据启动模式确定模式字符串与窗口参数
	mode := backend.ModeNormal
	title := "WindLabX4"
	width, height := 1600, 900
	minWidth, minHeight := 1440, 900
	// 独立窗口模式下禁用缩放，避免用户拉小到 4 列轴卡片断点以下导致 UI 错乱
	disableResize := false

	if *motionOnly || motionOnlyFromEnv {
		mode = backend.ModeMotion
		title = "运动控制器 - WindLabX4"
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
		Name:        "WindLabX4",
		Description: "WindLabX4 wind tunnel data acquisition desktop app",
		LogLevel:    slog.LevelInfo,
		// Wails 已完成 ServiceShutdown、窗口和托盘清理后直接结束宿主进程。
		// 避免特定 Windows/WebView2 环境中消息循环不返回而残留 windlabx4.exe。
		PostShutdown: func() {
			os.Exit(0)
		},
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

	mainWindow := wailsApp.Window.NewWithOptions(fitWindowToScreen(wailsApp, application.WebviewWindowOptions{
		Title:         title,
		Width:         width,
		Height:        height,
		MinWidth:      minWidth,
		MinHeight:     minHeight,
		URL:           "/",
		Hidden:        false,
		DisableResize: disableResize,
		// fitWindowToScreen 会在主屏逻辑工作区放不下时钳制以上尺寸
		BackgroundColour: application.RGBA{
			Red:   7,
			Green: 17,
			Blue:  31,
			Alpha: 1,
		},
	}))

	// 注册窗口关闭确认 hook：拦截 X 按钮关闭，由前端弹出确认对话框。
	// 仅在主窗口注册（normal 模式）；motion-only 模式保持现状，关闭即退出子进程。
	if mode == backend.ModeNormal {
		app.RegisterExitConfirmationHook(mainWindow)
	}

	// wailsApp.Run() 失败时写 crash log 而非 log.Fatal。
	// windowsgui 子系统下 stderr 不可见，log.Fatal 输出用户看不到。
	// 常见失败原因：WebView2 Runtime 缺失（日志会显示 "no webview2 found"）。
	if err := wailsApp.Run(); err != nil {
		writeCrashLog(fmt.Sprintf("wailsApp.Run() 失败: %v", err))
		os.Exit(1)
	}
}

// writeCrashLog 将启动期 panic 或致命错误写入 crash log 文件。
// 路径优先级：%APPDATA%\windlabx4\crash-YYYYMMDD-HHMMSS.log → exe 同目录 → 当前工作目录。
// 设计意图：windowsgui 子系统下 stderr 不可见，Wails 日志系统可能尚未初始化，
// crash log 是启动失败时唯一的诊断手段。
func writeCrashLog(message string) {
	// 获取调用栈
	buf := make([]byte, 64*1024)
	n := runtime.Stack(buf, false)
	stackTrace := string(buf[:n])

	// 构造 crash 信息
	timestamp := time.Now().Format("2006-01-02 15:04:05.000")
	content := fmt.Sprintf(
		"WindLabX4 启动崩溃报告\n"+
			"时间: %s\n"+
			"错误: %s\n\n"+
			"调用栈:\n%s\n",
		timestamp, message, stackTrace)

	// 确定日志目录：优先 %APPDATA%\windlabx4（老用户从 wind-daq 迁移），回退到 exe 同目录
	var crashDir string
	if configDir, err := os.UserConfigDir(); err == nil {
		crashDir = filepath.Join(configDir, "windlabx4")
	} else if exePath, err := os.Executable(); err == nil {
		crashDir = filepath.Dir(exePath)
	} else {
		crashDir = "."
	}
	_ = os.MkdirAll(crashDir, 0o755)

	// 写入 crash log（文件名含时间戳，避免覆盖历史 crash 记录）
	crashFile := filepath.Join(crashDir,
		fmt.Sprintf("crash-%s.log", time.Now().Format("20060102-150405")))
	if err := os.WriteFile(crashFile, []byte(content), 0o644); err != nil {
		// 写入失败时只能放弃，避免因日志写入失败再次 panic
		return
	}

	// stderr 兜底输出（windowsgui 下不可见，但 console 模式下可见）
	fmt.Fprintf(os.Stderr, "WindLabX4 启动崩溃: %s\n崩溃日志已写入: %s\n", message, crashFile)
}
