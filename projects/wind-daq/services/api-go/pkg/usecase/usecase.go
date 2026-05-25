// Package usecase exposes usecase types used by Wind-DAQ applications
package usecase

import (
	"wind-daq/services/api-go/internal/core/report"
	"wind-daq/services/api-go/internal/core/storage"
)

// Type aliases
type (
	StorageRecordingStatus = storage.RecordingStatus
	StorageRecordingConfig = storage.RecordingConfig
	ReportStatus           = report.ReportStatus
)
