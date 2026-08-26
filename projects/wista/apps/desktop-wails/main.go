// Wails v3 应用入口（WISTA）
//
// 架构总览：
//   - core/hub.go     ：跨 Service 共享的运行期状态（ctx、relays、LogEmitter）
//   - backend/log_service.go        ：日志 Service（兼任 core.LogEmitter）
//   - backend/recording_service.go  ：录制 Service（启动/停止/状态广播 + 写入热路径）
//   - backend/device_service.go     ：设备 Service（扫描/配置 CRUD/连接/采集 + 中继协程）
//
// 三个 Service 都通过 application.NewService(&...) 注册，
// 通过 wails3 generate bindings 自动生成 TS 端调用代码。

package main

import (
	"embed"
	"log"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/wailsapp/wails/v3/pkg/application"

	"wista/adapters/config"
	"wista/adapters/hardware"
	"wista/adapters/logging"
	"wista/adapters/recording"
	"wista/backend"
	"wista/core"
	"wista/usecase"
)

// 前端构建产物嵌入
//
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
	// ---- 1. 准备配置目录与日志目录 ----
	// 解析用户配置目录（含旧版本 %APPDATA%/daq-t1603 配置的自动迁移）
	configDir := resolveConfigDir()
	if err := os.MkdirAll(configDir, 0755); err != nil {
		slog.Warn("创建配置目录失败", "dir", configDir, "error", err)
	}
	cfgStore := config.NewJSONConfigStore(filepath.Join(configDir, "device-profiles.json"))

	logDir := filepath.Join(configDir, "logs")
	if err := os.MkdirAll(logDir, 0755); err != nil {
		slog.Warn("创建日志目录失败", "dir", logDir, "error", err)
	}

	// ---- 2. 实例化 adapters / usecases ----
	t1603Adapter := hardware.NewT1603Adapter()
	scanner := hardware.NewT1603Scanner()
	recorder := recording.NewCSVRecorder()
	logWriter := logging.NewLogFileWriter()

	deviceUC := usecase.NewDeviceUsecase(t1603Adapter, cfgStore, scanner)
	recordUC := usecase.NewRecordingUsecase(recorder)
	logUC := usecase.NewLogUsecase(logWriter)

	// ---- 3. 共享 Hub + 三个 Service ----
	hub := core.NewHub()
	logService := backend.NewLogService(hub, logUC, logDir)
	recordingService := backend.NewRecordingService(hub, recordUC)
	deviceService := backend.NewDeviceService(hub, deviceUC, recordingService)

	// 硬件适配器的日志回流到统一 EmitLog（途经 hub.emitter -> LogService.EmitLog）
	t1603Adapter.SetLogSink(func(entry hardware.DeviceLogEntry) {
		hub.EmitLog(core.LogEvent{
			Level:    entry.Level,
			Category: entry.Category,
			DeviceID: entry.DeviceID,
			Source:   "hardware",
			Message:  entry.Message,
			Detail:   entry.Detail,
		})
	})

	// 硬件适配器的状态变更回流到 hub.EmitDeviceState（ACQ-010/STB-003）：
	// adapter 在 OnReadLoopExit 等异步状态变化时调用此 sink，
	// hub 转发给 DeviceService.EmitDeviceState，最终通过 daq:device-state 事件
	// 让前端 statusMap 实时同步（如物理断网后从「采集中」直接变为「未连接」）。
	t1603Adapter.SetStateSink(func(deviceID string, state core.DeviceState) {
		hub.EmitDeviceState(deviceID, state)
	})

	// ---- 4. 构造 Wails v3 Application ----
	//
	// Services 注册顺序：LogService 必须最先注册，因为 ServiceStartup 是按顺序触发，
	// LogService.ServiceStartup 会调用 hub.SetContext/SetEmitter，让后续 Service 启动时
	// hub 已具备 ctx 与 emitter。
	app := application.New(application.Options{
		Name:        "WISTA",
		Description: "WISTA 温度采集",
		LogLevel:    slog.LevelInfo,
		Services: []application.Service{
			application.NewService(logService),
			application.NewService(recordingService),
			application.NewService(deviceService),
		},
		Assets: application.AssetOptions{
			// 必须使用 BundledAssetFileServer：它额外把 /wails/runtime.js 暴露出来，
			// @wailsio/runtime 在前端初始化时会去拉这个 endpoint，
			// 否则页面会卡在脚本加载阶段、WebView 只显示空白。
			Handler: application.BundledAssetFileServer(assets),
		},
		Icon: appIcon,
		Mac: application.MacOptions{
			ApplicationShouldTerminateAfterLastWindowClosed: true,
		},
	})

	// ---- 5. 主窗口 ----
	mainWindow := app.Window.NewWithOptions(fitWindowToScreen(app, application.WebviewWindowOptions{
		// 窗口默认标题用英文，避免首屏硬编码中文。
		// 前端 App.vue 在 onMounted 和 watch(locale) 时会通过 @wailsio/runtime 的 Window.SetTitle
		// 覆盖为当前语言对应的本地化标题（zh: "WISTA 温度采集" / en: "WISTA Temperature Acquisition"）。
		Title:           "WISTA Temperature Acquisition",
		URL:             "/",
		DevToolsEnabled: true,
		StartState:      application.WindowStateNormal,
		// Width/Height/MinWidth/MinHeight 由 fitWindowToScreen 按主屏逻辑工作区决定
	}))

	// 注册窗口 X 按钮拦截：未确认退出时弹确认对话框
	deviceService.RegisterExitConfirmationHook(mainWindow)

	// ---- 6. 启动事件循环 ----
	// panic 兜底：主 goroutine panic（如 Event.Emit 在退出阶段）记录后继续/退出，
	// 避免 Wails GUI 下进程直接闪退且无法诊断。
	defer func() {
		if r := recover(); r != nil {
			slog.Error("main panic recovered", "panic", r)
			log.Printf("panic: %v", r)
		}
	}()
	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}

// resolveConfigDir 解析应用配置目录并处理旧版本配置迁移。
// 应用由 DAQ-T-1603 更名为 WISTA 后，配置目录从 %APPDATA%/daq-t1603 迁至 %APPDATA%/wista。
// 为保证老用户升级后设备配置不丢失，当新目录尚不存在而旧目录存在时，自动将旧配置整体拷贝到新目录。
// 返回最终使用的配置目录绝对路径。
func resolveConfigDir() string {
	configDir, _ := os.UserConfigDir()
	if configDir == "" {
		// 无法获取用户配置目录时退回相对路径，保证程序可运行
		return "config"
	}
	newDir := filepath.Join(configDir, "wista")
	oldDir := filepath.Join(configDir, "daq-t1603")
	if _, err := os.Stat(oldDir); err == nil {
		if _, err := os.Stat(newDir); os.IsNotExist(err) {
			slog.Info("migrating config from daq-t1603 to wista", "from", oldDir, "to", newDir)
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
