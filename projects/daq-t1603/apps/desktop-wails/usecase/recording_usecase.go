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
