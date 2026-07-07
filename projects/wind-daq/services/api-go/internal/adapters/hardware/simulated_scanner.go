package hardware

import "wind-daq/services/api-go/internal/core/device"

type SimulatedScanner struct{}

func NewSimulatedScanner() SimulatedScanner {
	return SimulatedScanner{}
}

func (SimulatedScanner) Scan() ([]device.ScanResult, error) {
	return []device.ScanResult{
		{
			ID:        "scan-daq-p-1604-sim-192.168.1.100-9000",
			Name:      "Discovered DAQ-P-1604 (Sim)",
			Type:      device.DeviceDAQP1604,
			Available: true,
			Address:   "192.168.1.100",
			Port:      9000,
		},
		{
			ID:        "scan-daq-t-1603-sim-192.168.1.101-9000",
			Name:      "Discovered DAQ-T-1603 (Sim)",
			Type:      device.DeviceDaqT1603,
			Available: true,
			Address:   "192.168.1.101",
			Port:      9000,
		},
		{
			ID:        "scan-daq-p-1604pre-sim-192.168.1.102-23",
			Name:      "Discovered DAQ-P-1604Pre (Sim)",
			Type:      device.DeviceDAQP1604Pre,
			Available: true,
			Address:   "192.168.1.102",
			Port:      23,
		},
	}, nil
}
