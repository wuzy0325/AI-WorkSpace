package bootstrap

import (
	"net/http"
	"path/filepath"

	"shared.local/device-sdk/go/motion/adapters/hardware"
	"shared.local/device-sdk/go/motion/core"
	"shared.local/device-sdk/go/motion/ports"
	motionprofile "shared.local/motion-control/go/profile"

	"wind-daq/services/api-go/api"
	calstore "wind-daq/services/api-go/internal/adapters/calstore"
	windaqconfig "wind-daq/services/api-go/internal/adapters/config"
	windaqhardware "wind-daq/services/api-go/internal/adapters/hardware"
	reportadapter "wind-daq/services/api-go/internal/adapters/report"
	"wind-daq/services/api-go/internal/adapters/scan"
	storageadapter "wind-daq/services/api-go/internal/adapters/storage"
	"wind-daq/services/api-go/internal/core/device"
	windaqports "wind-daq/services/api-go/internal/ports"
	"wind-daq/services/api-go/internal/usecase"
)

const (
	DefaultAddress          = ":8080"
	DefaultProfileStorePath = "config/device-profiles.json"
)

type Config struct {
	Address          string
	ProfileStorePath string
}

type APIServer struct {
	Address string
	Handler http.Handler
}

func BuildAPIServer(cfg Config) (APIServer, error) {
	if cfg.Address == "" {
		cfg.Address = DefaultAddress
	}
	if cfg.ProfileStorePath == "" {
		cfg.ProfileStorePath = DefaultProfileStorePath
	}

	store := windaqconfig.NewFileProfileStore(cfg.ProfileStorePath)
	appConfigStore := windaqconfig.NewFileAppConfigStore(filepath.Join(filepath.Dir(cfg.ProfileStorePath), "app"))
	configMgr := usecase.NewConfigManager(appConfigStore)
	if err := ensureDefaultProfiles(store); err != nil {
		return APIServer{}, err
	}

	hub := usecase.NewAcquisitionHub(noopPublisher{}, 20)
	recorder := usecase.NewStorageRecorder(storageadapter.NewCSVRecordingSink())
	reportMgr := usecase.NewReportManager(reportadapter.NewCSVReportWriter())
	profileStore := motionprofile.NewMemoryMotionProfileStore()
	motionMgr := usecase.NewMotionManager(profileStore, func(profile core.MotionControllerProfile) (ports.MotionController, error) {
		if profile.Type == core.ControllerTypeWTNMC4A {
			return windaqhardware.NewWTNMC4AMotionController(profile), nil
		}
		factory := hardware.NewDefaultMotionControllerFactory()
		return factory.Create(profile)
	})
	calMgr := usecase.NewCalibrationManager(hub, motionMgr, nil, calstore.NewMemoryResultStore())
	travMgr := usecase.NewTraversalManager(hub, motionMgr, nil, calstore.NewTraversalResultStore())
	dataSink := func(payload device.DataPayload) {
		hub.OnData(payload)
		_ = recorder.HandlePayload(payload)
	}
	manager, err := usecase.NewDeviceManager(store, deviceFactory{}, dataSink)
	if err != nil {
		return APIServer{}, err
	}
	manager.SetScanner(scan.NewNetworkScanner())

	router := api.NewRouter(api.Deps{
		DeviceManager:      manager,
		AcquisitionHub:     hub,
		ReportManager:      reportMgr,
		MotionManager:      motionMgr,
		CalibrationManager: calMgr,
		TraversalManager:   travMgr,
		StorageRecorder:    recorder,
		ConfigManager:      configMgr,
	})
	return APIServer{Address: cfg.Address, Handler: router}, nil
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

func ensureDefaultProfiles(store windaqports.ProfileStore) error {
	profiles, err := store.LoadProfiles()
	if err != nil {
		return err
	}
	if len(profiles) > 0 {
		return nil
	}
	return store.SaveProfiles([]device.Profile{
		windaqconfig.NewDefaultProfile("sim-1", device.DeviceSimulated),
	})
}
