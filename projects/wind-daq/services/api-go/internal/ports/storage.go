// Package ports 定义 usecase 与 adapters 之间的接口契约。
//
// RecordingSink 抽象数据保存适配器，CSV/Binary 实现均满足该接口。
// usecase 通过该接口操作 sink，不依赖具体实现。
package ports

import (
	"wind-daq/services/api-go/internal/core/device"
	"wind-daq/services/api-go/internal/core/storage"
)

// RecordingSink 数据保存适配器接口。
//
// 生命周期：
//   - Start: 创建首个文件并启动 writer goroutine
//   - Write: 非阻塞投递 payload 到异步队列
//   - Stop: 通知 writer 退出并 drain 剩余数据
//
// 状态反馈：
//   - Status: 返回当前录制状态快照（文件、大小、记录数、错误等）
//   - Done: 返回 sink 自停止信号 channel（因停止条件或 I/O 错误触发）
type RecordingSink interface {
	Start(config storage.RecordingConfig) error
	Write(payload device.DataPayload) error
	Stop() error
	// Status 返回当前状态快照，供 UI 展示与错误反馈。
	Status() storage.RecordingStatus
	// Done 在 sink 自停止时被关闭；StorageRecorder 监听该信号同步 recording 状态。
	// 多次调用返回同一 channel；未 Start 时调用返回 nil（调用方需处理）。
	Done() <-chan struct{}
}
