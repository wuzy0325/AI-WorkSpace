package usecase

import (
	"fmt"
	"log/slog"
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
	slog.Info("StorageRecorder Start 开始", "component", "StorageRecorder", "outputDir", config.OutputDir, "filePrefix", config.FilePrefix)

	if strings.TrimSpace(config.OutputDir) == "" {
		err := fmt.Errorf("outputDir is required")
		slog.Error("StorageRecorder Start 失败", "component", "StorageRecorder", "error", err)
		return err
	}
	if strings.TrimSpace(config.FilePrefix) == "" {
		err := fmt.Errorf("filePrefix is required")
		slog.Error("StorageRecorder Start 失败", "component", "StorageRecorder", "error", err)
		return err
	}
	if r.sink == nil {
		err := fmt.Errorf("recording sink is required")
		slog.Error("StorageRecorder Start 失败", "component", "StorageRecorder", "error", err)
		return err
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.sink.Start(config); err != nil {
		slog.Error("StorageRecorder Start 失败", "component", "StorageRecorder", "error", err)
		return err
	}
	r.config = config
	r.recording = true
	slog.Info("StorageRecorder Start 成功", "component", "StorageRecorder", "outputDir", config.OutputDir, "filePrefix", config.FilePrefix)
	return nil
}

func (r *StorageRecorder) Stop() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.recording {
		slog.Warn("StorageRecorder Stop 跳过：未在录制", "component", "StorageRecorder")
		return nil
	}
	slog.Info("StorageRecorder Stop 开始", "component", "StorageRecorder", "outputDir", r.config.OutputDir)
	if err := r.sink.Stop(); err != nil {
		slog.Error("StorageRecorder Stop 失败", "component", "StorageRecorder", "error", err)
		return err
	}
	r.recording = false
	slog.Info("StorageRecorder Stop 成功", "component", "StorageRecorder", "outputDir", r.config.OutputDir)
	return nil
}

func (r *StorageRecorder) Status() storage.RecordingStatus {
	r.mu.RLock()
	defer r.mu.RUnlock()
	status := storage.RecordingStatus{
		Recording: r.recording,
		OutputDir: r.config.OutputDir,
	}
	slog.Info("StorageRecorder Status 查询", "component", "StorageRecorder", "recording", status.Recording, "outputDir", status.OutputDir)
	return status
}

func (r *StorageRecorder) HandlePayload(payload device.DataPayload) error {
	r.mu.RLock()
	recording := r.recording
	sink := r.sink
	r.mu.RUnlock()
	if !recording {
		return nil
	}
	if err := sink.Write(payload); err != nil {
		slog.Error("StorageRecorder HandlePayload 写入失败", "component", "StorageRecorder", "deviceID", payload.DeviceID, "error", err)
		return err
	}
	return nil
}
