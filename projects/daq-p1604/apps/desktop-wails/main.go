// Win7 版应用入口（DAQ-P-1604）
//
// 与 trunk 主分支的差异：
//   - 移除 Wails v3 依赖（Wails v3 内部用 log/slog + maps + slices，需要 Go 1.21+）
//   - 改为 net/http 启动 HTTP server，前端通过 fetch + WebSocket 调用
//   - GUI 由 Electron 22.3.27（最后兼容 Win7 的 Electron 主版本）承载
//
// 与 daq-t1603 Win7 版的差异：
//   - 端口 18182（daq-t1603 用 18181）
//   - 保留单体 App（daq-t1603 拆为 3 个 Service），main.go 只调用一次 ServiceStartup/Shutdown
//   - 保留 simulated 模式（DAQ_P1604_MODE=simulated）用于无硬件场景开发
//   - 状态回调 SetStateSink 注入：硬件适配器 → app.EmitDeviceState → 前端 daq:device-state 事件
//
// 完整 HTTP handler 在 httpserver/ 包实现，main.go 仅做装配。

package main

import (
	"context"
	"embed"
	"io/fs"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"shared.local/device-sdk/go/pkg/slog"

	"daq-p1604/adapters/config"
	"daq-p1604/adapters/hardware"
	"daq-p1604/adapters/logging"
	"daq-p1604/adapters/recording"
	"daq-p1604/backend"
	"daq-p1604/core"
	"daq-p1604/httpserver"
	"daq-p1604/ports"
	"daq-p1604/usecase"
)

// listenAddr 是 Go HTTP server 监听地址。
// 端口 18182：与 daq-t1603（18181）区分，避免同机双开时端口冲突。
// 仅监听 127.0.0.1，外部网络无法访问，无 CSRF 风险。
const listenAddr = "127.0.0.1:18182"

//go:embed all:frontend/dist
var frontendAssets embed.FS

func main() {
	// ---- 1. 准备配置目录与日志目录 ----
	configDir, _ := os.UserConfigDir()
	if configDir != "" {
		configDir = filepath.Join(configDir, "daq-p1604")
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
	// 设备适配器支持 simulated 模式（无硬件场景开发）：
	//   - DAQ_P1604_MODE=simulated → 用 SimulatedAdapter + SimulatedScanner
	//   - 其他 → 用 P1604Adapter + P1604Scanner
	var devAdapter ports.DevicePort
	var scanner ports.DeviceScanPort
	var logCapableAdapter interface {
		SetLogSink(func(hardware.DeviceLogEntry))
	}
	var stateCapableAdapter interface {
		SetStateSink(func(id string, state core.DeviceState))
	}
	if os.Getenv("DAQ_P1604_MODE") == "simulated" {
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

	recorder := recording.NewCSVRecorder()
	logWriter := logging.NewLogFileWriter()

	deviceUC := usecase.NewDeviceUsecase(devAdapter, cfgStore, scanner)
	recordUC := usecase.NewRecordingUsecase(recorder)
	logUC := usecase.NewLogUsecase(logWriter)

	// ---- 3. 共享 Hub + 单体 App ----
	hub := core.NewHub()
	app := backend.NewApp(hub, deviceUC, recordUC, logUC, logDir)

	// 注入硬件日志回调：硬件适配器 → hub.EmitLog → 前端 daq:log 事件
	// 必须在 ServiceStartup 之前注入，确保启动期硬件日志不丢失
	if logCapableAdapter != nil {
		logCapableAdapter.SetLogSink(func(entry hardware.DeviceLogEntry) {
			hub.EmitLog(core.LogEvent{
				Level:    entry.Level,
				Category: entry.Category,
				DeviceID: entry.DeviceID,
				Source:   "hardware",
				Message:  entry.Message,
				Detail:   entry.Detail,
			})
		})
	}
	// 注入设备状态回调：硬件适配器 → app.EmitDeviceState → 前端 daq:device-state 事件
	// 用于设备断连等异步状态变更通知，避免前端轮询 GetStatus
	if stateCapableAdapter != nil {
		stateCapableAdapter.SetStateSink(func(id string, state core.DeviceState) {
			app.EmitDeviceState(id, state)
		})
	}

	// ---- 3.5 WebSocket 事件总线 ----
	// 必须在 ServiceStartup 之前注入：ServiceStartup 内 EmitLog 会通过
	// hub.EmitLog -> hub.EmitEvent 推送"应用已启动"日志，此时 EventBus 必须就绪，
	// 否则该日志会被 hub.EmitEvent 静默丢弃。
	wsHub := httpserver.NewWSHub()
	hub.SetEventBus(wsHub)

	// ---- 4. HTTP server ----
	mux := http.NewServeMux()
	httpserver.RegisterHandlers(mux, app, hub, wsHub)
	frontendFS, err := fs.Sub(frontendAssets, "frontend/dist")
	if err != nil {
		slog.Error("frontend assets unavailable", "error", err)
		os.Exit(1)
	}
	mux.Handle("/", http.FileServer(http.FS(frontendFS)))
	srv := &http.Server{
		Addr:              listenAddr,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}

	// ---- 5. 启动：注入应用 ctx 并调用 ServiceStartup ----
	appCtx, appCancel := context.WithCancel(context.Background())
	defer appCancel()

	// 启动 wsHub 主循环，与 appCtx 同生命周期，应用关闭时自动收尾
	go wsHub.Run(appCtx)

	if err := app.ServiceStartup(appCtx); err != nil {
		slog.Error("App startup failed", "error", err)
	}

	go func() {
		slog.Info("HTTP server listening", "addr", listenAddr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("HTTP server error", "error", err)
			appCancel()
		}
	}()

	// ---- 6. 优雅关闭 ----
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	select {
	case <-sigCh:
		slog.Info("received shutdown signal")
	case <-appCtx.Done():
	}

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		slog.Error("HTTP server shutdown error", "error", err)
	}

	// 单体 App 一次 Shutdown 即可，内部按顺序停止 record/log/relay/ctx
	if err := app.ServiceShutdown(); err != nil {
		slog.Error("App shutdown error", "error", err)
	}
}
