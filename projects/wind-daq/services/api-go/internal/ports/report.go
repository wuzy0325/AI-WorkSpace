package ports

import "wind-daq/services/api-go/internal/core/report"

type ReportGenerator interface {
	Generate(cfg report.ReportConfig, data [][]string, headers []string) (report.ReportResult, error)
}
