package ports

import "daq-t1603/core"

type RecordingPort interface {
	Start(outputDir string, prefix string) error
	Write(snapshot core.TemperatureSnapshot) error
	Stop() error
	Status() core.RecordingSession
}
