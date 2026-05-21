package report

type ReportConfig struct {
	OutputDir  string `json:"outputDir"`
	FilePrefix string `json:"filePrefix"`
	DeviceID   string `json:"deviceId,omitempty"`
}

type ReportResult struct {
	Path    string `json:"path"`
	Size    int64  `json:"size"`
	Records int    `json:"records"`
}

type ReportStatus struct {
	Generating bool   `json:"generating"`
	LastResult string `json:"lastResult,omitempty"`
}
