package report

import (
	"encoding/csv"
	"fmt"
	"os"
	"path/filepath"

	"windlabx4/services/api-go/internal/core/report"
)

type CSVReportWriter struct{}

func NewCSVReportWriter() *CSVReportWriter {
	return &CSVReportWriter{}
}

func (w *CSVReportWriter) Generate(cfg report.ReportConfig, data [][]string, headers []string) (report.ReportResult, error) {
	if err := os.MkdirAll(cfg.OutputDir, 0755); err != nil {
		return report.ReportResult{}, fmt.Errorf("create output dir: %w", err)
	}

	path := filepath.Join(cfg.OutputDir, cfg.FilePrefix+".csv")
	f, err := os.Create(path)
	if err != nil {
		return report.ReportResult{}, fmt.Errorf("create file: %w", err)
	}
	defer f.Close()

	writer := csv.NewWriter(f)
	defer writer.Flush()

	if err := writer.Write(headers); err != nil {
		return report.ReportResult{}, fmt.Errorf("write header: %w", err)
	}
	for i, row := range data {
		if err := writer.Write(row); err != nil {
			return report.ReportResult{}, fmt.Errorf("write row %d: %w", i, err)
		}
	}
	writer.Flush()
	if err := writer.Error(); err != nil {
		return report.ReportResult{}, fmt.Errorf("flush: %w", err)
	}

	fi, err := f.Stat()
	if err != nil {
		return report.ReportResult{}, fmt.Errorf("stat: %w", err)
	}
	return report.ReportResult{
		Path:    path,
		Size:    fi.Size(),
		Records: len(data),
	}, nil
}
