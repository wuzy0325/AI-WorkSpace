package bootstrap

import (
	"net/http"

	"wind-daq/services/api-go/api"
	"wind-daq/services/api-go/internal/adapters/config"
	"wind-daq/services/api-go/internal/adapters/hardware"
	"wind-daq/services/api-go/internal/core/device"
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
	manager, err := usecase.NewDeviceManager(store, deviceFactory{}, hub.OnData)
	if err != nil {
		return APIServer{}, err
	}
	manager.SetScanner(hardware.NewSimulatedScanner())

	return APIServer{
		Address: cfg.Address,
		Handler: api.NewRouter(api.Deps{DeviceManager: manager, AcquisitionHub: hub}),
	}, nil
}

type deviceFactory struct{}

func (deviceFactory) Create(profile device.Profile) (ports.Device, error) {
	return hardware.NewSimulatedDevice(profile), nil
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
