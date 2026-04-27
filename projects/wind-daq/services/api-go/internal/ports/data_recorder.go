package ports

import "wind-daq/services/api-go/internal/core/device"

type DataRecorder interface {
	StartRecording() error
	StopRecording() error
	IsRecording() bool
	GetStatus() (bool, int, int64)
	OnData(payload device.DataPayload)
	SetBaseDir(dir string)
	GetBaseDir() string
}
