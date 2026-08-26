package main

import (
	"embed"
	"log"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/wailsapp/wails/v3/pkg/application"

	"wispa/adapters/config"
	"wispa/adapters/hardware"
	"wispa/adapters/logging"
	"wispa/adapters/recording"
	"wispa/backend"
	"wispa/core"
	"wispa/ports"
	"wispa/usecase"
)

//go:embed all:frontend/dist
var assets embed.FS

//go:embed appicon.png
var appIcon []byte

// 窗口设计尺寸与最小可交互尺寸（逻辑像素，非物理像素）。
const (
	windowDesignWidth  = 1600
	windowDesignHeight = 900
	windowMinWidth     = 1024
	windowMinHeight    = 640
)

// fitWindowToScreen 按主屏逻辑工作区钳制窗口初始尺寸。
// 部分机器显示缩放较大（如 1920x1080@150% 时逻辑工作区仅约 1280x660），
// 固定 1600x900 的初始窗口会超出屏幕导致画面看不全。
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
	// 解析用户配置目录（含旧版本 %APPDATA%/daq-p1604 配置的自动迁移）
	configDir := resolveConfigDir()
	cfgStore := config.NewJSONConfigStore(filepath.Join(configDir, "device-profiles.json"))

	// 日志文件保存目录
	logDir := filepath.Join(configDir, "logs")
	os.MkdirAll(logDir, 0755)

	var devAdapter ports.DevicePort
	var scanner ports.DeviceScanPort
	var logCapableAdapter interface {
		SetLogSink(func(hardware.DeviceLogEntry))
	}
	var stateCapableAdapter interface {
		SetStateSink(func(id string, state core.DeviceState))
	}
	if os.Getenv("WISPA_MODE") == "simulated" {
		slog.Info("using simulated device adapter")
		simAdapter := hardware.NewSimulatedAdapter()
		devAdapter = simAdapter
		logCapableAdapter = simAdapter
		scanner = hardware.NewSimulatedScanner()
	} else {
		p1604Adapter := hardware.NewP1604Adapter()
		devAdapter = p1604Adapter
		logCapableAdapter = p1604Adapter
		stateCapableAdapter = p1604Adapter
		scanner = hardware.NewP1604Scanner()
	}
	logWriter := logging.NewLogFileWriter()

	deviceUC := usecase.NewDeviceUsecase(devAdapter, cfgStore, scanner)
	// 注入 CSV 录制器（Binary 格式已移除）
	recordUC := usecase.NewRecordingUsecase(recording.NewCSVRecorder())
	logUC := usecase.NewLogUsecase(logWriter)

	app := backend.NewApp(deviceUC, recordUC, logUC, logDir)
	if logCapableAdapter != nil {
		logCapableAdapter.SetLogSink(func(entry hardware.DeviceLogEntry) {
			app.EmitLog(backend.LogEvent{
				Level:    entry.Level,
				Category: entry.Category,
				DeviceID: entry.DeviceID,
				Source:   "hardware",
				Message:  entry.Message,
				Detail:   entry.Detail,
			})
		})
	}
	// 设置状态变更回调，连接断开等事件通知前端
	if stateCapableAdapter != nil {
		stateCapableAdapter.SetStateSink(func(id string, state core.DeviceState) {
			app.EmitDeviceState(id, state)
		})
	}

	// E2E 测试专用：通过环境变量 WISPA_CDP_PORT 注入 WebView2 CDP 调试端口。
	// 设为空时（生产路径）不影响任何行为；设为端口号时（如 9222），
	// Playwright 可通过 connect_over_cdp("http://localhost:9222") 连接 WebView2
	// 进行真实 E2E 测试。参考：
	//   - Wails v3 application.Options.AdditionalBrowserArgs
	//   - https://playwright.dev/docs/webview2
	appOpts := application.Options{
		Name:        "WISPA",
		Description: "WISPA (WindTuner Intelligent-Pressure Scanning Application) desktop app",
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
	if cdpPort := os.Getenv("WISPA_CDP_PORT"); cdpPort != "" {
		slog.Info("CDP debugging enabled (E2E test mode)", "port", cdpPort)
		// 字段挂在 WindowsOptions 下（非 Options 顶层）：
		// 参见 wails/v3/pkg/application/application_options.go 中 WindowsOptions.AdditionalBrowserArgs
		appOpts.Windows.AdditionalBrowserArgs = []string{"--remote-debugging-port=" + cdpPort}
	}
	wailsApp := application.New(appOpts)

	mainWindow := wailsApp.Window.NewWithOptions(fitWindowToScreen(wailsApp, application.WebviewWindowOptions{
		// 窗口默认标题用英文，避免首屏硬编码中文。
		// 前端 App.vue 在 onMounted 和 watch(locale) 时会通过 @wailsio/runtime 的 Window.SetTitle
		// 覆盖为当前语言对应的本地化标题（zh: "WISPA 温特纳智能压力扫描应用" / en: "WISPA Pressure Acquisition"）。
		Title: "WISPA Pressure Acquisition",
		URL:   "/",
		// Width/Height/StartState 由 fitWindowToScreen 按主屏逻辑工作区决定
		Hidden: false,
	}))

	// 注册窗口 X 按钮拦截：未确认退出时弹确认对话框
	app.RegisterExitConfirmationHook(mainWindow)

	if err := wailsApp.Run(); err != nil {
		log.Fatal(err)
	}
}

// resolveConfigDir 解析应用配置目录并处理旧版本配置迁移。
// 应用由 DAQ-P-1604 更名为 WISPA 后，配置目录从 %APPDATA%/daq-p1604 迁至 %APPDATA%/wispa。
// 为保证老用户升级后设备配置不丢失，当新目录尚不存在而旧目录存在时，自动将旧配置整体拷贝到新目录。
// 返回最终使用的配置目录绝对路径。
func resolveConfigDir() string {
	configDir, _ := os.UserConfigDir()
	if configDir == "" {
		// 无法获取用户配置目录时退回相对路径，保证程序可运行
		return "config"
	}
	newDir := filepath.Join(configDir, "wispa")
	oldDir := filepath.Join(configDir, "daq-p1604")
	if _, err := os.Stat(oldDir); err == nil {
		if _, err := os.Stat(newDir); os.IsNotExist(err) {
			slog.Info("migrating config from daq-p1604 to wispa", "from", oldDir, "to", newDir)
			migrateConfigDir(oldDir, newDir)
		}
	}
	os.MkdirAll(newDir, 0755)
	return newDir
}

// migrateConfigDir 将旧配置目录递归拷贝到新目录。
// 单个文件拷贝失败仅记录警告并跳过，不中断整体迁移，保证尽量恢复用户数据。
func migrateConfigDir(oldDir, newDir string) {
	entries, err := os.ReadDir(oldDir)
	if err != nil {
		slog.Warn("migrate config: read old dir failed", "dir", oldDir, "error", err)
		return
	}
	if err := os.MkdirAll(newDir, 0755); err != nil {
		slog.Warn("migrate config: mkdir new dir failed", "dir", newDir, "error", err)
		return
	}
	for _, e := range entries {
		src := filepath.Join(oldDir, e.Name())
		dst := filepath.Join(newDir, e.Name())
		if e.IsDir() {
			// 递归拷贝子目录（如 logs）
			migrateConfigDir(src, dst)
			continue
		}
		data, err := os.ReadFile(src)
		if err != nil {
			slog.Warn("migrate config: read file failed", "file", src, "error", err)
			continue
		}
		if err := os.WriteFile(dst, data, 0644); err != nil {
			slog.Warn("migrate config: write file failed", "file", dst, "error", err)
		}
	}
}
