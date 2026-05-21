package bootstrap

import (
	"net/http"
	"os"

	"wind-daq/services/api-go/api"
	calstore "wind-daq/services/api-go/internal/adapters/calstore"
	"wind-daq/services/api-go/internal/adapters/config"
	"wind-daq/services/api-go/internal/adapters/hardware"
	reportadapter "wind-daq/services/api-go/internal/adapters/report"
	"wind-daq/services/api-go/internal/adapters/scan"
	storageadapter "wind-daq/services/api-go/internal/adapters/storage"
	"wind-daq/services/api-go/internal/core/device"
	"wind-daq/services/api-go/internal/core/motion"
	"wind-daq/services/api-go/internal/ports"
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

	store := config.NewFileProfileStore(cfg.ProfileStorePath)
	if err := ensureDefaultProfiles(store); err != nil {
		return APIServer{}, err
	}

	hub := usecase.NewAcquisitionHub(noopPublisher{}, 20)
	recorder := usecase.NewStorageRecorder(storageadapter.NewCSVRecordingSink())
	reportMgr := usecase.NewReportManager(reportadapter.NewCSVReportWriter())
	motionMgr := usecase.NewMotionManager(hardware.NewSimulatedMotionController("sim-motion", []motion.AxisName{"X", "Y", "Z"}))
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
	if os.Getenv("WIND_DAQ_NETWORK_SCAN") == "true" {
		manager.SetScanner(scan.NewNetworkScanner())
	} else {
		manager.SetScanner(hardware.NewSimulatedScanner())
	}

	return APIServer{
		Address: cfg.Address,
		Handler: api.NewRouter(api.Deps{
			DeviceManager:      manager,
			AcquisitionHub:     hub,
			ReportManager:      reportMgr,
			MotionManager:      motionMgr,
			CalibrationManager: calMgr,
			TraversalManager:   travMgr,
			StorageRecorder:    recorder,
		}),
	}, nil
}

type deviceFactory struct{}

func (deviceFactory) Create(profile device.Profile) (ports.Device, error) {
	switch profile.Type {
	case device.DeviceDAQP1604:
		return hardware.NewDAQP1604(profile), nil
	case device.DeviceDaqT1603:
		return hardware.NewDAQT1603(profile), nil
	default:
		return hardware.NewSimulatedDevice(profile), nil
	}
}

type noopPublisher struct{}

func (noopPublisher) Publish(string, any) {}

func ensureDefaultProfiles(store ports.ProfileStore) error {
	profiles, err := store.LoadProfiles()
	if err != nil {
		return err
	}
	if len(profiles) > 0 {
		return nil
	}
	return store.SaveProfiles([]device.Profile{
		device.NewDefaultProfile("sim-1", device.DeviceSimulated),
	})
}
