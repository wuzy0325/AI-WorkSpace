package hardware

import "wind-daq/services/api-go/internal/core/device"

type SimulatedScanner struct{}

func NewSimulatedScanner() SimulatedScanner {
	return SimulatedScanner{}
}

func (SimulatedScanner) Scan() ([]device.ScanResult, error) {
	return []device.ScanResult{
		{
			ID:        "sim-1",
			Name:      "Simulator 1",
			Type:      device.DeviceSimulated,
			Available: true,
			Address:   "simulated://sim-1",
		},
	}, nil
}
