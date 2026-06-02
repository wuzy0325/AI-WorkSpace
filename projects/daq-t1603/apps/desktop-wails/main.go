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

	"daq-t1603/adapters/config"
	"daq-t1603/adapters/hardware"
	"daq-t1603/adapters/recording"
	"daq-t1603/backend"
	"daq-t1603/ports"
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

	var devAdapter ports.DevicePort
	if os.Getenv("DAQ_T1603_MODE") == "simulated" {
		slog.Info("using simulated device adapter")
		devAdapter = hardware.NewSimulatedAdapter()
	} else {
		devAdapter = hardware.NewT1603Adapter()
	}
	recorder := recording.NewCSVRecorder()

	deviceUC := usecase.NewDeviceUsecase(devAdapter, cfgStore)
	recordUC := usecase.NewRecordingUsecase(recorder)

	app := backend.NewApp(deviceUC, recordUC)

	err := wails.Run(&options.App{
		Title:  "DAQ-T-1603 温度采集",
		Width:  1400,
		Height: 1000,
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
