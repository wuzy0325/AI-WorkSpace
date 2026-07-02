package ports

import "daq-p1604/core"

// RecordingPort 数据录制端口接口
type RecordingPort interface {
	// Start 开始录制，配置包含输出目录、格式、滚动条件、自动停止条件等
	Start(config core.RecordingConfig) error
	// Write 异步投递一条快照到录制队列，队列满时丢弃并计数，绝不阻塞调用方
	Write(snapshot core.PressureSnapshot) error
	// Stop 停止录制，drain 队列后关闭文件
	Stop() error
	// IsActive 无锁热路径判活。relayStream 每帧调用，避免 Status() 的锁与
	// writer goroutine 争用。语义等价于 Status().Status == RecordingActive。
	IsActive() bool
	// Status 获取录制会话运行时状态（含丢弃计数、文件数、错误信息等）
	Status() core.RecordingSession
}
