package ports

import "wista/core"

type RecordingPort interface {
	Start(outputDir string, prefix string) error
	Write(snapshot core.TemperatureSnapshot) error
	Stop() error
	Status() core.RecordingSession
	// IsActive 返回当前是否处于录制状态（实现需保证无锁访问，供热路径调用）。
	IsActive() bool
	// SetBackpressureHandler 注入背压回调。
	// 当录制队列饱和丢单帧被丢弃时，实现需同步调用 handler。
	// handler 内部禁止回调本接口任何方法（避免死锁），
	// 应仅做日志/事件转发。
	SetBackpressureHandler(handler func(core.BackpressureEvent))
	// SetFatalErrorHandler 注入不可恢复 I/O 错误回调。
	// 当 deviceWriter 发生 bufio.Write 失败或文件创建失败时被调用。
	// handler 内部禁止回调本接口任何方法。
	SetFatalErrorHandler(handler func(deviceID string, err error))
	// SetDeviceProfile 注入某设备的通道配置（含 enabled 标志），
	// 用于在 CSV 中将禁用通道的列写空值（REC-006）。
	// 在设备 Connect / StartAcquisition 时调用；未调用时默认全通道启用。
	SetDeviceProfile(deviceID string, channels []core.ChannelConfig)
}
