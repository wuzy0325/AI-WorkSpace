package ports

import "daq-t1603/core"

type RecordingPort interface {
	Start(outputDir string, prefix string) error
	Write(snapshot core.TemperatureSnapshot) error
	Stop() error
	Status() core.RecordingSession
	// IsActive 返回当前是否处于录制状态（实现需保证无锁访问，供热路径调用）。
	IsActive() bool
}
