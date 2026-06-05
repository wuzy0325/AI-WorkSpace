package ports

import "daq-t1603/core"

type DeviceScanPort interface {
	Scan() ([]core.ScanResult, error)
}
