package main

import (
	"embed"
	"log"
	"os"
	"path/filepath"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"

	"daq-t1603/adapters/config"
	"daq-t1603/adapters/hardware"
	"daq-t1603/adapters/logging"
	"daq-t1603/adapters/recording"
	"daq-t1603/backend"
	"daq-t1603/usecase"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	configDir, _ := os.UserConfigDir()
	if configDir != "" {
		configDir = filepath.Join(configDir, "daq-t1603")
	} else {
		configDir = "config"
	}
	os.MkdirAll(configDir, 0755)
	cfgStore := config.NewJSONConfigStore(filepath.Join(configDir, "device-profiles.json"))

	// 日志文件保存目录
	logDir := filepath.Join(configDir, "logs")
	os.MkdirAll(logDir, 0755)

	t1603Adapter := hardware.NewT1603Adapter()
	devAdapter := t1603Adapter
	scanner := hardware.NewT1603Scanner()
	logCapableAdapter := t1603Adapter
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

	err := wails.Run(&options.App{
		Title:     "DAQ-T-1603 温度采集",
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
