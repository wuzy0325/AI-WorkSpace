package ports

import "wispa/core"

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
	// StopWithError 先填充错误原因再停止录制，CAS 保护原子性：
	//   - 录制活跃：写入 lastError，drain 队列并关闭文件，返回 true
	//   - 录制已停止（用户已主动 Stop）：不修改 lastError，返回 false
	// 用于设备断连自动停止场景，避免与用户主动 StopRecording 竞争时
	// 把"用户主动停止"误覆盖为"设备断连自动停止"。
	StopWithError(msg string) bool
}
