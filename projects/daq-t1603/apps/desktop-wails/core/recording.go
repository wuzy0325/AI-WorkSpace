package core

type RecordingStatus int

const (
	RecordingIdle RecordingStatus = iota
	RecordingActive
)

type RecordingSession struct {
	ID            string          `json:"id"`
	OutputDir     string          `json:"outputDir"`
	FilePrefix    string          `json:"filePrefix"`
	StartTimeMs   int64           `json:"startTimeMs"`
	SnapshotCount int             `json:"snapshotCount"`
	Status        RecordingStatus `json:"status"`
}
