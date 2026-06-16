package ports

import "daq-p1604/core"

// DeviceScanPort 设备扫描端口接口
type DeviceScanPort interface {
	Scan() ([]core.ScanResult, error)
}
