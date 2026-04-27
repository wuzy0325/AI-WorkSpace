package storage

import (
	"encoding/csv"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"

	"wind-daq/services/api-go/internal/core/device"
)

// Service 数据存储服务
type Service struct {
	mu         sync.Mutex
	baseDir    string
	recording  bool
	fileMap    map[string]*os.File    // deviceID -> file
	writerMap  map[string]*csv.Writer // deviceID -> csv writer
	fileCount  int
	totalBytes int64
}

// NewService 创建数据存储服务
func NewService(baseDir string) *Service {
	return &Service{
		baseDir:   baseDir,
		fileMap:   make(map[string]*os.File),
		writerMap: make(map[string]*csv.Writer),
	}
}

// StartRecording 开始录制
func (s *Service) StartRecording() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.recording {
		return fmt.Errorf("already recording")
	}

	s.recording = true
	s.fileCount = 0
	s.totalBytes = 0
	slog.Info("Data recording started", "dir", s.baseDir)
	return nil
}

// StopRecording 停止录制
func (s *Service) StopRecording() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.recording {
		return nil
	}

	// 关闭所有文件
	for id, f := range s.fileMap {
		if w, ok := s.writerMap[id]; ok {
			w.Flush()
		}
		f.Close()
	}
	s.fileMap = make(map[string]*os.File)
	s.writerMap = make(map[string]*csv.Writer)
	s.recording = false
	slog.Info("Data recording stopped", "files", s.fileCount, "bytes", s.totalBytes)
	return nil
}

// IsRecording 是否正在录制
func (s *Service) IsRecording() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.recording
}

// GetStatus 获取存储状态
func (s *Service) GetStatus() (bool, int, int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.recording, s.fileCount, s.totalBytes
}

// OnData 数据接收回调（作为 DataSink 使用）
func (s *Service) OnData(payload device.DataPayload) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.recording {
		return
	}

	w, err := s.getOrCreateWriter(payload.DeviceID)
	if err != nil {
		slog.Error("Storage write error", "device", payload.DeviceID, "err", err)
		return
	}

	// 写入 CSV 行: timestamp, ch0, ch1, ch2, ...
	record := make([]string, 0, len(payload.Channels)+1)
	record = append(record, time.UnixMilli(payload.Timestamp).Format("2006-01-02 15:04:05.000"))
	for _, ch := range payload.Channels {
		record = append(record, fmt.Sprintf("%.4f", ch))
	}

	if err := w.Write(record); err != nil {
		slog.Error("CSV write error", "device", payload.DeviceID, "err", err)
		return
	}
	w.Flush()
	s.totalBytes += int64(len(record)) // 近似
}

// getOrCreateWriter 获取或创建 CSV writer
func (s *Service) getOrCreateWriter(deviceID string) (*csv.Writer, error) {
	if w, ok := s.writerMap[deviceID]; ok {
		return w, nil
	}

	// 创建文件
	now := time.Now().Format("20060102_150405")
	filename := fmt.Sprintf("%s_%s.csv", deviceID, now)
	dir := filepath.Join(s.baseDir, "data")
	os.MkdirAll(dir, 0755)
	path := filepath.Join(dir, filename)

	f, err := os.Create(path)
	if err != nil {
		return nil, fmt.Errorf("create file %s: %w", path, err)
	}

	w := csv.NewWriter(f)
	s.fileMap[deviceID] = f
	s.writerMap[deviceID] = w
	s.fileCount++

	// 写入 CSV 头
	header := []string{"timestamp"}
	for i := 0; i < 16; i++ {
		header = append(header, fmt.Sprintf("ch%d", i))
	}
	w.Write(header)
	w.Flush()

	return w, nil
}

// SetBaseDir 设置基础目录
func (s *Service) SetBaseDir(dir string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.baseDir = dir
}

// GetBaseDir 获取基础目录
func (s *Service) GetBaseDir() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.baseDir
}
