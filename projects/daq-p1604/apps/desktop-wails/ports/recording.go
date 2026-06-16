package ports

import "daq-p1604/core"

// RecordingPort 数据录制端口接口
type RecordingPort interface {
	Start(outputDir string, prefix string) error
	Write(snapshot core.PressureSnapshot) error
	Stop() error
	Status() core.RecordingSession
}
