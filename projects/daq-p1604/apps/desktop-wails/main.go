package main

import (
	"embed"
	"log"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/wailsapp/wails/v3/pkg/application"

	"daq-p1604/adapters/config"
	"daq-p1604/adapters/hardware"
	"daq-p1604/adapters/logging"
	"daq-p1604/adapters/recording"
	"daq-p1604/backend"
	"daq-p1604/core"
	"daq-p1604/ports"
	"daq-p1604/usecase"
)

//go:embed all:frontend/dist
var assets embed.FS

//go:embed appicon.png
var appIcon []byte

func main() {
	configDir, _ := os.UserConfigDir()
	if configDir != "" {
		configDir = filepath.Join(configDir, "daq-p1604")
	} else {
		configDir = "config"
	}
	os.MkdirAll(configDir, 0755)
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

	wailsApp := application.New(application.Options{
		Name:        "DAQ-P-1604",
		Description: "DAQ-P-1604 pressure acquisition desktop app",
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
		Title:     "DAQ-P-1604 压力采集",
		Width:     1600,
		Height:    900,
		MinWidth:  1280,
		MinHeight: 720,
		URL:       "/",
		Hidden:    false,
	})

	if err := wailsApp.Run(); err != nil {
		log.Fatal(err)
	}
}
