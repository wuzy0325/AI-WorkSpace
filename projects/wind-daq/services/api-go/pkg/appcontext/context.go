// Package appcontext provides public access to core application components
package appcontext

import (
	"os"
	"path/filepath"

	"wind-daq/services/api-go/internal/adapters/calstore"
	"wind-daq/services/api-go/internal/adapters/config"
	"wind-daq/services/api-go/internal/adapters/hardware"
	"wind-daq/services/api-go/internal/adapters/report"
	"wind-daq/services/api-go/internal/adapters/scan"
	"wind-daq/services/api-go/internal/adapters/storage"
	"wind-daq/services/api-go/internal/core/device"
	"wind-daq/services/api-go/internal/core/motion"
	"wind-daq/services/api-go/internal/ports"
	"wind-daq/services/api-go/internal/usecase"
)

// AppContext holds all core application services
type AppContext struct {
	DeviceManager   *usecase.DeviceManager
	AcquisitionHub  *usecase.AcquisitionHub
	ReportManager   *usecase.ReportManager
	MotionManager   *usecase.MotionManager
	CalibrationMgr  *usecase.CalibrationManager
	TraversalMgr    *usecase.TraversalManager
	StorageRecorder *usecase.StorageRecorder
	configDir       string
}

// NewAppContext creates and initializes all core services
func NewAppContext(configDir string) (*AppContext, error) {
	if configDir == "" {
		var err error
		configDir, err = os.UserConfigDir()
		if err != nil {
			configDir = "config"
		} else {
			configDir = filepath.Join(configDir, "wind-daq")
		}
	}

	// Ensure config directory exists
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return nil, err
	}

	deviceProfilePath := filepath.Join(configDir, "device-profiles.json")
	motionProfilePath := filepath.Join(configDir, "motion-profiles.json")

	// Copy default profiles if they don't exist
	copyDefaultProfilesIfNeeded(deviceProfilePath, motionProfilePath)

	profileStore := config.NewFileProfileStore(deviceProfilePath)
	motionProfileStore := config.NewFileMotionProfileStore(motionProfilePath)

	hub := usecase.NewAcquisitionHub(noopPublisher{}, 20)
	recorder := usecase.NewStorageRecorder(storage.NewCSVRecordingSink())
	reportMgr := usecase.NewReportManager(report.NewCSVReportWriter())
	motionMgr := usecase.NewMotionManager(motionProfileStore, func(profile motion.MotionControllerProfile) ports.MotionController {
		return hardware.NewSimulatedMotionController(profile)
	})
	calStore := calstore.NewMemoryResultStore()
	calibrationMgr := usecase.NewCalibrationManager(hub, motionMgr, nil, calStore)
	travStore := calstore.NewTraversalResultStore()
	traversalMgr := usecase.NewTraversalManager(hub, motionMgr, nil, travStore)

	dataSink := func(payload device.DataPayload) {
		hub.OnData(payload)
		_ = recorder.HandlePayload(payload)
	}

	deviceMgr, err := usecase.NewDeviceManager(profileStore, deviceFactory{}, dataSink)
	if err != nil {
		return nil, err
	}
	if os.Getenv("WIND_DAQ_NETWORK_SCAN") == "true" {
		deviceMgr.SetScanner(scan.NewNetworkScanner())
	} else {
		deviceMgr.SetScanner(hardware.NewSimulatedScanner())
	}

	return &AppContext{
		DeviceManager:   deviceMgr,
		AcquisitionHub:  hub,
		ReportManager:   reportMgr,
		MotionManager:   motionMgr,
		CalibrationMgr:  calibrationMgr,
		TraversalMgr:    traversalMgr,
		StorageRecorder: recorder,
		configDir:       configDir,
	}, nil
}

type noopPublisher struct{}

func (noopPublisher) Publish(string, any) {}

type deviceFactory struct{}

func (deviceFactory) Create(profile device.Profile) (ports.Device, error) {
	switch profile.Type {
	case device.DeviceDAQP1604:
		return hardware.NewDAQP1604(profile), nil
	case device.DeviceDAQP1064Pre:
		return hardware.NewDAQP1064Pre(profile), nil
	case device.DeviceDaqT1603:
		return hardware.NewDAQT1603(profile), nil
	case device.DeviceWTNPXI:
		return hardware.NewWTNPXI(profile), nil
	case device.DeviceDSA3217:
		return hardware.NewDSA3217(profile), nil
	default:
		return hardware.NewSimulatedDevice(profile), nil
	}
}

func copyDefaultProfilesIfNeeded(devicePath, motionPath string) {
	if _, err := os.Stat(devicePath); os.IsNotExist(err) {
		defaultContent := `[{"id":"sim-1","name":"仿真设备","type":"SIMULATED"}]`
		_ = os.WriteFile(devicePath, []byte(defaultContent), 0644)
	}
	if _, err := os.Stat(motionPath); os.IsNotExist(err) {
		defaultContent := `[{"id":"sim-motion-1","name":"仿真运动控制器"}]`
		_ = os.WriteFile(motionPath, []byte(defaultContent), 0644)
	}
}
