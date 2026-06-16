package hardware

import "daq-p1604/core"

// SimulatedScanner 模拟设备扫描器
type SimulatedScanner struct{}

// NewSimulatedScanner 创建模拟设备扫描器
func NewSimulatedScanner() *SimulatedScanner {
	return &SimulatedScanner{}
}

// Scan 返回模拟扫描结果
func (s *SimulatedScanner) Scan() ([]core.ScanResult, error) {
	return []core.ScanResult{
		{
			ID:              "scan-daq-p-1604-192.168.3.101-9000",
			Name:            "Discovered DAQ-P-1604",
			Address:         "192.168.3.101",
			Port:            9000,
			MacAddress:      "aa:bb:cc:dd:ee:01",
			SerialNumber:    "SIM-P001",
			FirmwareVersion: "v1.0-sim",
		},
	}, nil
}
