package usecase

import (
	"daq-p1604/core"
	"daq-p1604/ports"
)

// RecordingUsecase 录制业务逻辑
type RecordingUsecase struct {
	recorder ports.RecordingPort
}

// NewRecordingUsecase 创建录制 usecase
func NewRecordingUsecase(recorder ports.RecordingPort) *RecordingUsecase {
	return &RecordingUsecase{recorder: recorder}
}

// Start 开始录制
func (uc *RecordingUsecase) Start(outputDir string, prefix string) error {
	return uc.recorder.Start(outputDir, prefix)
}

// Write 写入数据快照
func (uc *RecordingUsecase) Write(snapshot core.PressureSnapshot) error {
	return uc.recorder.Write(snapshot)
}

// Stop 停止录制
func (uc *RecordingUsecase) Stop() error {
	return uc.recorder.Stop()
}

// Status 获取录制状态
func (uc *RecordingUsecase) Status() core.RecordingSession {
	return uc.recorder.Status()
}
