package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"wind-daq/services/api-go/api"
	"wind-daq/services/api-go/internal/adapters/config"
	"wind-daq/services/api-go/internal/adapters/hardware"
	"wind-daq/services/api-go/internal/adapters/logger"
	"wind-daq/services/api-go/internal/adapters/report"
	"wind-daq/services/api-go/internal/adapters/scan"
	"wind-daq/services/api-go/internal/adapters/storage"
	"wind-daq/services/api-go/internal/adapters/ws"
	"wind-daq/services/api-go/internal/core/device"
	"wind-daq/services/api-go/internal/core/motion"
	"wind-daq/services/api-go/internal/ports"
	"wind-daq/services/api-go/internal/usecase"
)

type deviceFactoryAdapter struct{}

func (d *deviceFactoryAdapter) Create(config device.DeviceConfig) (ports.Device, error) {
	return hardware.CreateDevice(config)
}

type motionFactoryAdapter struct{}

func (m *motionFactoryAdapter) Create(profile motion.MotionControllerProfile) ports.MotionController {
	return hardware.CreateMotionController(profile)
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	port := 8080
	if p := os.Getenv("PORT"); p != "" {
		port = parseInt(p, port)
	}
	dev := os.Getenv("DEV") == "true"

	configDir := os.Getenv("CONFIG_DIR")
	if configDir == "" {
		configDir = "./config"
	}
	dataDir := os.Getenv("DATA_DIR")
	if dataDir == "" {
		dataDir = "./data"
	}

	if err := os.MkdirAll(dataDir, 0755); err != nil {
		slog.Error("Failed to create data dir", "dir", dataDir, "err", err)
	}

	configMgr := config.NewManager(configDir)
	deviceStore := config.NewDeviceStore(configMgr)
	acqStore := config.NewAcquisitionStore(configMgr)
	wsHub := ws.NewHub()

	logCfg := logger.LoadConfig(configDir + "/logging.json")
	logger.Init(logCfg, wsHub)

	reportSvc := report.NewService(dataDir)

	var devFactory ports.DeviceFactory = &deviceFactoryAdapter{}
	var motFactory ports.MotionFactory = &motionFactoryAdapter{}

	deviceManager := usecase.NewDeviceManager(deviceStore, wsHub, devFactory)
	motionManager := usecase.NewMotionManager(wsHub, motFactory)
	acqHub := usecase.NewAcquisitionHub(wsHub, acqStore)
	storageAdapter := storage.NewService(dataDir)
	storageSvc := usecase.NewStorageService(storageAdapter)
	calibSvc := usecase.NewCalibrationService(deviceManager, motionManager, wsHub)
	travSvc := usecase.NewTraversalService(deviceManager, motionManager, wsHub)
	scanSvc := usecase.NewScanService(scan.NewDefaultScanService())

	deps := api.Deps{
		DeviceManager: deviceManager,
		AcqHub:        acqHub,
		MotionManager: motionManager,
		CalibService:  calibSvc,
		TraversalSvc:  travSvc,
		StorageSvc:    storageSvc,
		ReportSvc:     reportSvc,
		ConfigMgr:     configMgr,
		ScanSvc:       scanSvc,
	}

	server := api.NewServer(port, dev, wsHub, deps)
	server.SetupRoutesWithDeps(deps)

	slog.Info("Server starting", "port", port, "dev", dev)
	if err := server.Start(ctx); err != nil {
		slog.Error("Server exited with error", "err", err)
		os.Exit(1)
	}

	slog.Info("Server stopped gracefully")
}

func parseInt(s string, fallback int) int {
	var n int
	_, err := fmt.Sscanf(s, "%d", &n)
	if err != nil {
		return fallback
	}
	return n
}
