package usecase

import (
	"daq-t1603/core"
	"daq-t1603/ports"
)

type RecordingUsecase struct {
	recorder ports.RecordingPort
}

func NewRecordingUsecase(recorder ports.RecordingPort) *RecordingUsecase {
	return &RecordingUsecase{recorder: recorder}
}

func (uc *RecordingUsecase) Start(outputDir string, prefix string) error {
	return uc.recorder.Start(outputDir, prefix)
}

func (uc *RecordingUsecase) Write(snapshot core.TemperatureSnapshot) error {
	return uc.recorder.Write(snapshot)
}

func (uc *RecordingUsecase) Stop() error {
	return uc.recorder.Stop()
}

func (uc *RecordingUsecase) Status() core.RecordingSession {
	return uc.recorder.Status()
}

// IsActive 无锁查询当前是否处于录制状态，供热路径（如 relayStream）使用，
// 避免每条 snapshot 都调用 Status() 引起的锁竞争。
func (uc *RecordingUsecase) IsActive() bool {
	return uc.recorder.IsActive()
}
