package apiserver

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"

	"shared.local/device-sdk/go/motion/adapters/hardware"
	"shared.local/device-sdk/go/motion/core"
	"shared.local/device-sdk/go/motion/ports"
	motionprofile "shared.local/motion-control/go/profile"

	"wind-daq/services/api-go/api"
	calstore "wind-daq/services/api-go/internal/adapters/calstore"
	configadapter "wind-daq/services/api-go/internal/adapters/config"
	windaqhardware "wind-daq/services/api-go/internal/adapters/hardware"
	reportadapter "wind-daq/services/api-go/internal/adapters/report"
	"wind-daq/services/api-go/internal/adapters/scan"
	storageadapter "wind-daq/services/api-go/internal/adapters/storage"
	"wind-daq/services/api-go/internal/core/calibration"
	"wind-daq/services/api-go/internal/core/device"
	windaqports "wind-daq/services/api-go/internal/ports"
	"wind-daq/services/api-go/internal/usecase"
	"wind-daq/services/api-go/pkg/wiring"
)

type Server struct {
	*http.Server
}

func Start(ctx context.Context, addr string) (*Server, error) {
	if addr == "" {
		addr = ":8080"
	}

	store := configadapter.NewFileProfileStore("config/device-profiles.json")
	if err := ensureDefaultProfiles(store); err != nil {
		return nil, err
	}

	hub := usecase.NewAcquisitionHub(noopPublisher{}, 20)
	recorder := usecase.NewStorageRecorder(storageadapter.NewCSVRecordingSink())
	reportMgr := usecase.NewReportManager(reportadapter.NewCSVReportWriter())

	motionProfileStore := motionprofile.NewFileMotionProfileStore("config/motion-profiles.json")
	motionMgr := wiring.NewMotionManager(motionProfileStore, func(profile core.MotionControllerProfile) (ports.MotionController, error) {
		factory := hardware.NewDefaultMotionControllerFactory()
		return factory.Create(profile)
	})

	calMgr := usecase.NewCalibrationManager(hub, motionMgr, nil, calstore.NewMemoryResultStore())
	calMgr.SetCsvWriter(storageadapter.NewCalibrationCsvWriter(calibration.Config{}))
	appConfigStore := configadapter.NewFileAppConfigStore("config/app")
	travMgr := usecase.NewTraversalManager(hub, motionMgr, nil, calstore.NewTraversalResultStore(), storageadapter.NewFileCheckpointStore(), appConfigStore)
	dataSink := func(payload device.DataPayload) {
		hub.OnData(payload)
		_ = recorder.HandlePayload(payload)
	}
	manager, err := usecase.NewDeviceManagerWithNormalizer(store, deviceFactory{}, dataSink, configadapter.NewProfileNormalizer())
	if err != nil {
		return nil, err
	}
	manager.SetScanner(scan.NewNetworkScanner())

	handler := api.NewRouter(api.Deps{
		DeviceManager:      manager,
		AcquisitionHub:     hub,
		ReportManager:      reportMgr,
		MotionManager:      motionMgr,
		CalibrationManager: calMgr,
		TraversalManager:   travMgr,
		StorageRecorder:    recorder,
		ConfigManager: usecase.NewConfigManager(
			appConfigStore,
		),
	})

	srv := &http.Server{Addr: addr, Handler: handler}
	go func() {
		<-ctx.Done()
		srv.Close()
	}()
	go func() {
		srv.ListenAndServe()
	}()

	return &Server{srv}, nil
}

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

type noopPublisher struct{}

func (noopPublisher) Publish(string, any) {}

func ensureDefaultProfiles(store windaqports.ProfileStore) error {
	profiles, err := store.LoadProfiles()
	if err != nil {
		return err
	}
	if len(profiles) > 0 {
		return nil
	}
	return store.SaveProfiles([]device.Profile{
		configadapter.NewDefaultProfile("sim-1", device.DeviceSimulated),
	})
}

func FindAvailablePort(preferred int) int {
	ports := []int{preferred, 8081, 8082, 9090, 9091}
	for _, port := range ports {
		ln, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
		if err == nil {
			ln.Close()
			return port
		}
	}
	return 0
}

func EnvInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		var n int
		if _, err := fmt.Sscanf(v, "%d", &n); err == nil {
			return n
		}
	}
	return fallback
}
