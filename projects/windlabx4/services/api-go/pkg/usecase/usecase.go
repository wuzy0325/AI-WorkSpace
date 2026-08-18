// Package usecase exposes usecase types used by windlabx4 applications
package usecase

import (
	"windlabx4/services/api-go/internal/core/report"
	"windlabx4/services/api-go/internal/core/storage"
)

// Type aliases
type (
	StorageRecordingStatus = storage.RecordingStatus
	StorageRecordingConfig = storage.RecordingConfig
	ReportStatus           = report.ReportStatus
)
