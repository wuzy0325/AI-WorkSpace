package ports

import "daq-p1604/core"

// RecordingPort 数据录制端口接口
type RecordingPort interface {
	// Start 开始录制，channels 用于确定每个通道的保存精度
	Start(outputDir string, prefix string, channels []core.ChannelConfig) error
	Write(snapshot core.PressureSnapshot) error
	Stop() error
	Status() core.RecordingSession
}
