package storage

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"wind-daq/services/api-go/internal/core/device"
	corestorage "wind-daq/services/api-go/internal/core/storage"
)

type CSVRecordingSink struct {
	mu         sync.Mutex
	file       *os.File
	writeCount int
	syncEvery  int // 每写入多少条记录执行一次 Sync
}

func NewCSVRecordingSink() *CSVRecordingSink {
	return &CSVRecordingSink{syncEvery: 100}
}

func (s *CSVRecordingSink) Start(config corestorage.RecordingConfig) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if strings.TrimSpace(config.OutputDir) == "" {
		return fmt.Errorf("outputDir is required")
	}
	if strings.TrimSpace(config.FilePrefix) == "" {
		return fmt.Errorf("filePrefix is required")
	}
	if err := os.MkdirAll(config.OutputDir, 0o755); err != nil {
		return err
	}
	if s.file != nil {
		if err := s.file.Close(); err != nil {
			return err
		}
		s.file = nil
	}
	name := fmt.Sprintf("%s-%s.csv", config.FilePrefix, time.Now().Format("20060102-150405"))
	file, err := os.Create(filepath.Join(config.OutputDir, name))
	if err != nil {
		return err
	}
	if _, err := file.WriteString("timestamp,deviceId,channelIndex,value\n"); err != nil {
		_ = file.Close()
		return err
	}
	s.file = file
	return nil
}

func (s *CSVRecordingSink) Write(payload device.DataPayload) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.file == nil {
		return fmt.Errorf("recording sink is not started")
	}
	for i, value := range payload.Channels {
		channelIndex := i
		if i < len(payload.ChannelIndices) {
			channelIndex = payload.ChannelIndices[i]
		}
		if _, err := fmt.Fprintf(s.file, "%d,%s,%d,%f\n", payload.Timestamp, payload.DeviceID, channelIndex, value); err != nil {
			return err
		}
		s.writeCount++
		// 定期执行文件同步，防止系统崩溃时数据丢失
		if s.writeCount >= s.syncEvery {
			s.writeCount = 0
			if err := s.file.Sync(); err != nil {
				return fmt.Errorf("failed to sync csv file: %w", err)
			}
		}
	}
	return nil
}

func (s *CSVRecordingSink) Stop() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.file == nil {
		return nil
	}
	if err := s.file.Close(); err != nil {
		return err
	}
	s.file = nil
	return nil
}
