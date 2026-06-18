package main

import (
	"embed"
	"log"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"

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
	recorder := recording.NewCSVRecorder()
	logWriter := logging.NewLogFileWriter()

	deviceUC := usecase.NewDeviceUsecase(devAdapter, cfgStore, scanner)
	recordUC := usecase.NewRecordingUsecase(recorder)
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

	err := wails.Run(&options.App{
		Title:     "DAQ-P-1604 压力采集",
		Width:     1600,
		Height:    900,
		MinWidth:  1280,
		MinHeight: 720,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		OnStartup:  app.Startup,
		OnShutdown: app.Shutdown,
		Bind: []interface{}{
			app,
		},
	})
	if err != nil {
		log.Fatal(err)
	}
}
