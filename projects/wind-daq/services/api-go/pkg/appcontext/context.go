// Package appcontext provides public access to core application components
package appcontext

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	hardware "shared.local/device-sdk/go/motion/adapters/hardware"
	"shared.local/device-sdk/go/motion/core"
	"shared.local/device-sdk/go/motion/ports"
	motionmanager "shared.local/motion-control/go/manager"
	motionprofile "shared.local/motion-control/go/profile"

	"wind-daq/services/api-go/internal/adapters/calstore"
	windaqconfig "wind-daq/services/api-go/internal/adapters/config"
	windaqhardware "wind-daq/services/api-go/internal/adapters/hardware"
	interpadapter "wind-daq/services/api-go/internal/adapters/interpolation"
	"wind-daq/services/api-go/internal/adapters/report"
	"wind-daq/services/api-go/internal/adapters/scan"
	"wind-daq/services/api-go/internal/adapters/storage"
	"wind-daq/services/api-go/internal/core/calibration"
	"wind-daq/services/api-go/internal/core/device"
	windaqports "wind-daq/services/api-go/internal/ports"
	"wind-daq/services/api-go/internal/usecase"
	"wind-daq/services/api-go/pkg/wiring"
)

// AppContext holds all core application services
type AppContext struct {
	DeviceManager    *usecase.DeviceManager
	AcquisitionHub   *usecase.AcquisitionHub
	ReportManager    *usecase.ReportManager
	MotionManager    windaqports.MotionManager
	CalibrationMgr   *usecase.CalibrationManager
	TraversalMgr     *usecase.TraversalManager
	StorageRecorder  *usecase.StorageRecorder
	ConfigManager    *usecase.ConfigManager
	MotionManagerRaw *motionmanager.MotionManager
	DataStreamRelay  *usecase.DataStreamRelay
	configDir        string
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

	if err := os.MkdirAll(configDir, 0755); err != nil {
		return nil, err
	}

	deviceProfilePath := filepath.Join(configDir, "device-profiles.json")
	motionProfilePath := filepath.Join(configDir, "motion-profiles.json")

	if err := copyDefaultProfilesIfNeeded(deviceProfilePath, motionProfilePath); err != nil {
		return nil, err
	}

	profileStore := windaqconfig.NewFileProfileStore(deviceProfilePath)
	motionProfileStore := motionprofile.NewFileMotionProfileStore(motionProfilePath)
	appConfigStore := windaqconfig.NewFileAppConfigStore(filepath.Join(configDir, "app"))
	configMgr := usecase.NewConfigManager(appConfigStore)

	hub := usecase.NewAcquisitionHub(noopPublisher{}, 20)
	recorder, err := NewStorageRecorderFromConfigStore(appConfigStore)
	if err != nil {
		return nil, err
	}
	reportMgr := usecase.NewReportManager(report.NewCSVReportWriter())
	rawMotionMgr := motionmanager.NewMotionManager(motionProfileStore, func(profile core.MotionControllerProfile) (ports.MotionController, error) {
		factory := hardware.NewDefaultMotionControllerFactory()
		return factory.Create(profile)
	})
	motionMgr := wiring.WrapMotionManager(rawMotionMgr)
	calStore := calstore.NewMemoryResultStore()
	calibrationMgr := usecase.NewCalibrationManager(hub, motionMgr, nil, calStore)
	calibrationMgr.SetCsvWriter(storage.NewCalibrationCsvWriter(calibration.Config{}))
	calibrationMgr.SetCsvWriterFactory(func(config calibration.Config) windaqports.CalibrationCsvWriter {
		return storage.NewCalibrationCsvWriterOverwrite(config)
	})
	travStore := calstore.NewTraversalResultStore()
	traversalMgr := usecase.NewTraversalManager(hub, motionMgr, nil, travStore, storage.NewFileCheckpointStore(), appConfigStore)
	// 注入插值器加载端口并异步恢复（通过 ports.InterpolatorLoader 解耦适配器依赖）
	traversalMgr.SetInterpolatorLoader(interpadapter.NewLoader())
	traversalMgr.RestoreInterpolatorFromPersistedConfig()

	dataSink := usecase.NewDataSink(hub, recorder)

	deviceMgr, err := usecase.NewDeviceManagerWithNormalizer(profileStore, deviceFactory{}, dataSink, windaqconfig.NewProfileNormalizer())
	if err != nil {
		return nil, err
	}
	deviceMgr.SetScanner(scan.NewNetworkScanner())

	return &AppContext{
		DeviceManager:    deviceMgr,
		AcquisitionHub:   hub,
		ReportManager:    reportMgr,
		MotionManager:    motionMgr,
		CalibrationMgr:   calibrationMgr,
		TraversalMgr:     traversalMgr,
		StorageRecorder:  recorder,
		ConfigManager:    configMgr,
		MotionManagerRaw: rawMotionMgr,
		DataStreamRelay:  usecase.NewDataStreamRelay(hub),
		configDir:        configDir,
	}, nil
}

type noopPublisher struct{}

func (noopPublisher) Publish(string, any) {}

type deviceFactory struct{}

func (deviceFactory) Create(profile device.Profile) (windaqports.Device, error) {
	switch profile.Type {
	case device.DeviceDAQP1604:
		return windaqhardware.NewDAQP1604(profile), nil
	case device.DeviceDAQP1064Pre:
		return windaqhardware.NewDAQP1064Pre(profile), nil
	case device.DeviceDaqT1603:
		return windaqhardware.NewT1603Adapter(profile), nil
	case device.DeviceWTNPXI:
		return windaqhardware.NewWTNPXI(profile), nil
	case device.DeviceDSA3217:
		return windaqhardware.NewDSA3217(profile), nil
	default:
		return windaqhardware.NewSimulatedDevice(profile), nil
	}
}

func copyDefaultProfilesIfNeeded(devicePath, motionPath string) error {
	if err := ensureJSONFile(devicePath, defaultDeviceProfiles()); err != nil {
		return err
	}
	return ensureJSONFile(motionPath, defaultMotionProfiles())
}

func ensureJSONFile(path string, fallback any) error {
	data, err := os.ReadFile(path)
	if err == nil {
		if json.Valid(bytes.TrimSpace(data)) {
			return nil
		}
		backupPath, backupErr := moveInvalidConfigAside(path)
		if backupErr != nil {
			return backupErr
		}
		if err := writeDefaultJSON(path, fallback); err != nil {
			return err
		}
		slog.Warn("replaced invalid config, backup saved",
			"component", "AppContext",
			"path", path,
			"backupPath", backupPath,
		)
		return nil
	}
	if !os.IsNotExist(err) {
		return err
	}
	return writeDefaultJSON(path, fallback)
}

func moveInvalidConfigAside(path string) (string, error) {
	backupPath := path + ".invalid"
	if _, err := os.Stat(backupPath); err == nil {
		backupPath = fmt.Sprintf("%s.invalid.%d", path, time.Now().Unix())
	}
	if err := os.Rename(path, backupPath); err != nil {
		return "", err
	}
	return backupPath, nil
}

func writeDefaultJSON(path string, value any) error {
	dir := filepath.Dir(path)
	if dir != "" {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return err
		}
	}
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0644)
}

func defaultDeviceProfiles() []device.Profile {
	return []device.Profile{
		windaqconfig.NewDefaultProfile("sim-1", device.DeviceSimulated),
	}
}

func defaultMotionProfiles() []core.MotionControllerProfile {
	return []core.MotionControllerProfile{
		{
			ID:          "sim-motion-1",
			Name:        "Simulated Motion Controller",
			Type:        core.ControllerTypeSimulated,
			Address:     "127.0.0.1",
			Port:        9000,
			AutoConnect: false,
			Axes: []core.AxisConfig{
				{Name: core.AxisX, Enabled: true, Kind: core.AxisKindLinear, MaxSpeed: core.PtrFloat64(10)},
				{Name: core.AxisY, Enabled: true, Kind: core.AxisKindLinear, MaxSpeed: core.PtrFloat64(10)},
				{Name: core.AxisZ, Enabled: true, Kind: core.AxisKindLinear, MaxSpeed: core.PtrFloat64(10)},
				{Name: core.AxisU, Enabled: true, Kind: core.AxisKindRotary, MaxSpeed: core.PtrFloat64(10)},
			},
		},
	}
}
