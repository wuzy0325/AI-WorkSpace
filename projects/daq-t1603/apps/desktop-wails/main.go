// Win7 版应用入口（DAQ-T-1603）
//
// 与 trunk 主分支的差异：
//   - 移除 Wails v3 依赖（Wails v3 内部用 log/slog + maps + slices，需要 Go 1.21+）
//   - 改为 net/http 启动 HTTP server，前端通过 fetch + WebSocket 调用
//   - GUI 由 Electron 22.3.27（最后兼容 Win7 的 Electron 主版本）承载
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

	"daq-t1603/adapters/config"
	"daq-t1603/adapters/hardware"
	"daq-t1603/adapters/logging"
	"daq-t1603/adapters/recording"
	"daq-t1603/backend"
	"daq-t1603/core"
	"daq-t1603/httpserver"
	"daq-t1603/usecase"
)

const listenAddr = "127.0.0.1:18181"

//go:embed all:frontend/dist
var frontendAssets embed.FS

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

	// ---- 3.5 WebSocket 事件总线 ----
	// 必须在 ServiceStartup 之前注入：LogService.ServiceStartup 会通过
	// hub.EmitLog -> hub.EmitEvent 推送"应用已启动"日志，此时 EventBus 必须就绪，
	// 否则该日志会被 hub.EmitEvent 静默丢弃。
	wsHub := httpserver.NewWSHub()
	hub.SetEventBus(wsHub)

	// ---- 4. HTTP server ----
	mux := http.NewServeMux()
	httpserver.RegisterHandlers(mux, deviceService, recordingService, logService, hub, wsHub)
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

	// ---- 5. 启动：注入应用 ctx 并按依赖顺序调用 ServiceStartup ----
	// 启动顺序：LogService 必须第一个（负责 hub.SetContext + hub.SetEmitter），
	// 之后 RecordingService（注册背压/fatal 回调）和 DeviceService（仅缓存 ctx）。
	appCtx, appCancel := context.WithCancel(context.Background())
	defer appCancel()

	// 启动 wsHub 主循环，与 appCtx 同生命周期，应用关闭时自动收尾
	go wsHub.Run(appCtx)

	if err := logService.ServiceStartup(appCtx); err != nil {
		slog.Error("LogService startup failed", "error", err)
	}
	if err := recordingService.ServiceStartup(appCtx); err != nil {
		slog.Error("RecordingService startup failed", "error", err)
	}
	if err := deviceService.ServiceStartup(appCtx); err != nil {
		slog.Error("DeviceService startup failed", "error", err)
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

	// 关闭顺序与启动相反：Device → Recording → Log
	_ = deviceService.ServiceShutdown()
	_ = recordingService.ServiceShutdown()
	_ = logService.ServiceShutdown()
}
