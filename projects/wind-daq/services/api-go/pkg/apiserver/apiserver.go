package apiserver

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"

	"wind-daq/services/api-go/api"
	calstore "wind-daq/services/api-go/internal/adapters/calstore"
	configadapter "wind-daq/services/api-go/internal/adapters/config"
	"wind-daq/services/api-go/internal/adapters/hardware"
	reportadapter "wind-daq/services/api-go/internal/adapters/report"
	"wind-daq/services/api-go/internal/adapters/scan"
	storageadapter "wind-daq/services/api-go/internal/adapters/storage"
	"wind-daq/services/api-go/internal/core/device"
	"wind-daq/services/api-go/internal/core/motion"
	"wind-daq/services/api-go/internal/ports"
	"wind-daq/services/api-go/internal/usecase"
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

	// 创建运动控制器配置存储
	motionProfileStore := configadapter.NewFileMotionProfileStore("config/motion-profiles.json")
	motionMgr := usecase.NewMotionManager(motionProfileStore, func(profile motion.MotionControllerProfile) ports.MotionController {
		return hardware.NewSimulatedMotionController(profile)
	})

	calMgr := usecase.NewCalibrationManager(hub, motionMgr, nil, calstore.NewMemoryResultStore())
	travMgr := usecase.NewTraversalManager(hub, motionMgr, nil, calstore.NewTraversalResultStore())
	dataSink := func(payload device.DataPayload) {
		hub.OnData(payload)
		_ = recorder.HandlePayload(payload)
	}
	manager, err := usecase.NewDeviceManager(store, deviceFactory{}, dataSink)
	if err != nil {
		return nil, err
	}
	if os.Getenv("WIND_DAQ_NETWORK_SCAN") == "true" {
		manager.SetScanner(scan.NewNetworkScanner())
	} else {
		manager.SetScanner(hardware.NewSimulatedScanner())
	}

	handler := api.NewRouter(api.Deps{
		DeviceManager:      manager,
		AcquisitionHub:     hub,
		ReportManager:      reportMgr,
		MotionManager:      motionMgr,
		CalibrationManager: calMgr,
		TraversalManager:   travMgr,
		StorageRecorder:    recorder,
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
