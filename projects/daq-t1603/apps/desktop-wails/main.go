// Wails v3 应用入口（DAQ-T-1603）
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

	"daq-t1603/adapters/config"
	"daq-t1603/adapters/hardware"
	"daq-t1603/adapters/logging"
	"daq-t1603/adapters/recording"
	"daq-t1603/backend"
	"daq-t1603/core"
	"daq-t1603/usecase"
)

// 前端构建产物嵌入
//
//go:embed all:frontend/dist
var assets embed.FS

//go:embed appicon.ico
var appIcon []byte

func main() {
	// ---- 1. 准备配置目录与日志目录 ----
	configDir, _ := os.UserConfigDir()
	if configDir != "" {
		configDir = filepath.Join(configDir, "daq-t1603")
	} else {
		configDir = "config"
	}
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

	// ---- 4. 构造 Wails v3 Application ----
	//
	// Services 注册顺序：LogService 必须最先注册，因为 ServiceStartup 是按顺序触发，
	// LogService.ServiceStartup 会调用 hub.SetContext/SetEmitter，让后续 Service 启动时
	// hub 已具备 ctx 与 emitter。
	app := application.New(application.Options{
		Name:        "DAQ-T-1603",
		Description: "DAQ-T-1603 温度采集",
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
	app.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:           "DAQ-T-1603 温度采集",
		Width:           1600,
		Height:          900,
		MinWidth:        1280,
		MinHeight:       720,
		URL:             "/",
		DevToolsEnabled: true,
		StartState:      application.WindowStateNormal,
	})

	// ---- 6. 启动事件循环 ----
	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
