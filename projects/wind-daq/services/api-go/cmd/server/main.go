package main

import (
	"log/slog"
	"net/http"
	"os"

	"wind-daq/services/api-go/api"
	"wind-daq/services/api-go/internal/adapters/config"
	"wind-daq/services/api-go/internal/adapters/hardware"
	"wind-daq/services/api-go/internal/core/device"
	"wind-daq/services/api-go/internal/ports"
	"wind-daq/services/api-go/internal/usecase"
)

type deviceFactory struct{}

func (deviceFactory) Create(profile device.Profile) (ports.Device, error) {
	return hardware.NewSimulatedDevice(profile), nil
}

type noopPublisher struct{}

func (noopPublisher) Publish(string, any) {}

func main() {
	store := config.NewFileProfileStore(profileStorePath())
	if err := ensureDefaultProfiles(store); err != nil {
		slog.Error("initialize device profiles", "err", err)
		os.Exit(1)
	}
	hub := usecase.NewAcquisitionHub(noopPublisher{}, 20)
	manager, err := usecase.NewDeviceManager(store, deviceFactory{}, hub.OnData)
	if err != nil {
		slog.Error("create device manager", "err", err)
		os.Exit(1)
	}
	manager.SetScanner(hardware.NewSimulatedScanner())

	router := api.NewRouter(api.Deps{DeviceManager: manager, AcquisitionHub: hub})
	addr := ":8080"
	if envAddr := os.Getenv("WIND_DAQ_ADDR"); envAddr != "" {
		addr = envAddr
	}
	slog.Info("wind-daq api server starting", "addr", addr)
	if err := http.ListenAndServe(addr, router); err != nil {
		slog.Error("server stopped", "err", err)
		os.Exit(1)
	}
}

func profileStorePath() string {
	if path := os.Getenv("WIND_DAQ_PROFILE_PATH"); path != "" {
		return path
	}
	return "config/device-profiles.json"
}

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
