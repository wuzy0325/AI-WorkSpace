package ports

import (
	"wind-daq/services/api-go/internal/core/device"
	"wind-daq/services/api-go/internal/core/storage"
)

type RecordingSink interface {
	Start(config storage.RecordingConfig) error
	Write(payload device.DataPayload) error
	Stop() error
}
