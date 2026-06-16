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

// CalibrationEventPublisher 校准事件发布端口
type CalibrationEventPublisher interface {
	PublishProgress(event calibration.ProgressEvent)
	PublishComplete(event calibration.CompleteEvent)
	PublishRealtime(event calibration.RealtimeEvent)
}

// CalibrationRuntime 校准运行时端口，提供通道读取和运动控制能力
type CalibrationRuntime interface {
	GetChannelValue(deviceID string, channelIndex int) (float64, bool)
	MoveToPosition(axisName string, position float64) error
	WaitForMotionComplete() error
}

// DeviceStatusProvider 设备状态查询端口
type DeviceStatusProvider interface {
	GetDeviceStatus(deviceID string) (connected bool, acquiring bool)
}
