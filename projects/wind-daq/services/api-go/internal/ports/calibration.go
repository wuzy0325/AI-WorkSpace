package ports

import (
	"wind-daq/services/api-go/internal/core/calibration"
	"wind-daq/services/api-go/internal/core/device"
)

type LatestDataReader interface {
	GetLatestData(deviceID string) (device.DataPayload, bool)
}

type CalibrationPointSink interface {
	WriteCalibrationPoint(point calibration.PointResult) error
}

type CalibrationResultStore interface {
	Save(taskID string, status calibration.Status) error
	Get(taskID string) (calibration.Status, bool)
}
