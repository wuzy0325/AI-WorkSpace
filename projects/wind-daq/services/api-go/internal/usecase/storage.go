package usecase

import (
	"log/slog"

	"wind-daq/services/api-go/internal/core/device"
	"wind-daq/services/api-go/internal/ports"
)

type StorageService struct {
	recorder ports.DataRecorder
}

func NewStorageService(recorder ports.DataRecorder) *StorageService {
	return &StorageService{recorder: recorder}
}

func (s *StorageService) StartRecording() error {
	slog.Info("StorageService: StartRecording")
	return s.recorder.StartRecording()
}

func (s *StorageService) StopRecording() error {
	slog.Info("StorageService: StopRecording")
	return s.recorder.StopRecording()
}

func (s *StorageService) IsRecording() bool {
	return s.recorder.IsRecording()
}

func (s *StorageService) GetStatus() (bool, int, int64) {
	return s.recorder.GetStatus()
}

func (s *StorageService) OnData(payload device.DataPayload) {
	s.recorder.OnData(payload)
}

func (s *StorageService) SetBaseDir(dir string) {
	s.recorder.SetBaseDir(dir)
}

func (s *StorageService) GetBaseDir() string {
	return s.recorder.GetBaseDir()
}
