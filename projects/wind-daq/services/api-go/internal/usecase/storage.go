package usecase

import (
	"fmt"
	"strings"
	"sync"

	"wind-daq/services/api-go/internal/core/device"
	"wind-daq/services/api-go/internal/core/storage"
	"wind-daq/services/api-go/internal/ports"
)

type StorageRecorder struct {
	mu        sync.RWMutex
	sink      ports.RecordingSink
	config    storage.RecordingConfig
	recording bool
}

func NewStorageRecorder(sink ports.RecordingSink) *StorageRecorder {
	return &StorageRecorder{sink: sink}
}

func (r *StorageRecorder) Start(config storage.RecordingConfig) error {
	if strings.TrimSpace(config.OutputDir) == "" {
		return fmt.Errorf("outputDir is required")
	}
	if strings.TrimSpace(config.FilePrefix) == "" {
		return fmt.Errorf("filePrefix is required")
	}
	if r.sink == nil {
		return fmt.Errorf("recording sink is required")
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.sink.Start(config); err != nil {
		return err
	}
	r.config = config
	r.recording = true
	return nil
}

func (r *StorageRecorder) Stop() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.recording {
		return nil
	}
	if err := r.sink.Stop(); err != nil {
		return err
	}
	r.recording = false
	return nil
}

func (r *StorageRecorder) Status() storage.RecordingStatus {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return storage.RecordingStatus{
		Recording: r.recording,
		OutputDir: r.config.OutputDir,
	}
}

func (r *StorageRecorder) HandlePayload(payload device.DataPayload) error {
	r.mu.RLock()
	recording := r.recording
	sink := r.sink
	r.mu.RUnlock()
	if !recording {
		return nil
	}
	return sink.Write(payload)
}
