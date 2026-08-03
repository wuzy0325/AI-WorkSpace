package usecase

// This file centralizes acquisition state and readable device identity checks
// for every device referenced by a traversal configuration.

import (
	"sort"

	"wind-daq/services/api-go/internal/core/traversal"
	"wind-daq/services/api-go/internal/ports"
)

type acquisitionDeviceState struct {
	name      string
	connected bool
	acquiring bool
}

func referencedAcquisitionDevices(controller ports.AcquisitionController, config traversal.Config) []acquisitionDeviceState {
	if controller == nil {
		return nil
	}

	ids := make(map[string]struct{})
	for _, ref := range config.ResolvedChannelRefs() {
		if ref.DeviceID != "" {
			ids[ref.DeviceID] = struct{}{}
		}
	}
	if len(ids) == 0 && config.DeviceID != "" {
		ids[config.DeviceID] = struct{}{}
	}
	orderedIDs := make([]string, 0, len(ids))
	for id := range ids {
		orderedIDs = append(orderedIDs, id)
	}
	sort.Strings(orderedIDs)

	states := make([]acquisitionDeviceState, 0, len(orderedIDs))
	for _, id := range orderedIDs {
		states = append(states, acquisitionDeviceState{
			name:      acquisitionDeviceName(controller, id),
			connected: controller.IsConnected(id),
			acquiring: controller.IsAcquiring(id),
		})
	}
	return states
}

func acquisitionDeviceName(controller ports.AcquisitionController, id string) string {
	if name := controller.DeviceName(id); name != "" {
		return name
	}
	return id
}

func firstNonAcquiringDevice(controller ports.AcquisitionController, config traversal.Config) (string, bool) {
	for _, state := range referencedAcquisitionDevices(controller, config) {
		if !state.acquiring {
			return state.name, true
		}
	}
	return "", false
}
