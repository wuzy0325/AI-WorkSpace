package usecase

import (
	"wista/core"
	"wista/ports"
)

// RecordingUsecase 是录制业务逻辑层。
//
// 它本身不持有并发状态，所有调用直接透传到 RecordingPort 实现，
// 保持 usecase 层的薄壳特性（hard constraint: usecase 不直接接触硬件/IO）。
type RecordingUsecase struct {
	recording ports.RecordingPort
}

// NewRecordingUsecase 创建录制业务用例。
func NewRecordingUsecase(recording ports.RecordingPort) *RecordingUsecase {
	return &RecordingUsecase{recording: recording}
}

// Start 启动录制。
func (uc *RecordingUsecase) Start(outputDir string, prefix string) error {
	return uc.recording.Start(outputDir, prefix)
}

// Write 投递一帧快照到录制队列。
func (uc *RecordingUsecase) Write(snapshot core.TemperatureSnapshot) error {
	return uc.recording.Write(snapshot)
}

// Stop 停止录制并 flush 所有缓冲。
func (uc *RecordingUsecase) Stop() error {
	return uc.recording.Stop()
}

// Status 返回当前录制会话状态。
func (uc *RecordingUsecase) Status() core.RecordingSession {
	return uc.recording.Status()
}

// IsActive 热路径无锁判活。
func (uc *RecordingUsecase) IsActive() bool {
	return uc.recording.IsActive()
}

// SetBackpressureHandler 透传背压回调到 recorder 实现。
// handler 内部禁止回调 usecase/ports 任何方法，避免死锁。
func (uc *RecordingUsecase) SetBackpressureHandler(handler func(core.BackpressureEvent)) {
	uc.recording.SetBackpressureHandler(handler)
}

// SetFatalErrorHandler 透传不可恢复错误回调到 recorder 实现。
// handler 内部禁止回调 usecase/ports 任何方法，避免死锁。
func (uc *RecordingUsecase) SetFatalErrorHandler(handler func(deviceID string, err error)) {
	uc.recording.SetFatalErrorHandler(handler)
}

// SetDeviceProfile 透传设备通道配置到 recorder 实现（REC-006）。
// 由 backend 在设备 Connect / StartAcquisition 时调用，
// 让 recorder 在 CSV 写入时将禁用通道的列写空值。
func (uc *RecordingUsecase) SetDeviceProfile(deviceID string, channels []core.ChannelConfig) {
	uc.recording.SetDeviceProfile(deviceID, channels)
}
