package hardware

import "daq-t1603/core"

type SimulatedScanner struct{}

func NewSimulatedScanner() *SimulatedScanner {
	return &SimulatedScanner{}
}

func (s *SimulatedScanner) Scan() ([]core.ScanResult, error) {
	return []core.ScanResult{
		{
			ID:              "scan-daq-t-1603-192.168.1.7-9000",
			Name:            "Discovered DAQ-T-1603",
			Address:         "192.168.1.7",
			Port:            9000,
			MacAddress:      "aa:bb:cc:dd:ee:01",
			SerialNumber:    "SIM-001",
			FirmwareVersion: "v1.0-sim",
		},
		{
			ID:              "scan-daq-t-1603-192.168.1.8-9000",
			Name:            "Discovered DAQ-T-1603",
			Address:         "192.168.1.8",
			Port:            9000,
			MacAddress:      "aa:bb:cc:dd:ee:02",
			SerialNumber:    "SIM-002",
			FirmwareVersion: "v1.0-sim",
		},
	}, nil
}
