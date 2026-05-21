package storage

type RecordingConfig struct {
	OutputDir  string `json:"outputDir"`
	FilePrefix string `json:"filePrefix"`
}

type RecordingStatus struct {
	Recording bool   `json:"recording"`
	OutputDir string `json:"outputDir,omitempty"`
}
