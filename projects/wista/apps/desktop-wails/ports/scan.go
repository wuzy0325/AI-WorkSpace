package ports

import "wista/core"

type DeviceScanPort interface {
	Scan() ([]core.ScanResult, error)
}
