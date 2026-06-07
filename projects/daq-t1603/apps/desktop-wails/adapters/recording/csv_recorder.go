package recording

import (
	"encoding/csv"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"daq-t1603/core"
)

const (
	csvFlushRows     = 100
	csvFlushInterval = time.Second
)

type CSVRecorder struct {
	mu          sync.RWMutex
	file        *os.File
	session     core.RecordingSession
	writer      *csv.Writer
	pendingRows int
	lastFlush   time.Time
}

func NewCSVRecorder() *CSVRecorder {
	return &CSVRecorder{}
}

func (r *CSVRecorder) Start(outputDir string, prefix string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.session.Status == core.RecordingActive {
		return fmt.Errorf("recording already in progress")
	}

	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return fmt.Errorf("create output dir: %w", err)
	}

	filename := fmt.Sprintf("%s_%s.csv", prefix, time.Now().Format("20060102-150405"))
	filePath := filepath.Join(outputDir, filename)
	f, err := os.Create(filePath)
	if err != nil {
		return fmt.Errorf("create file: %w", err)
	}

	r.writer = csv.NewWriter(f)

	header := make([]string, 0, 17)
	header = append(header, "Timestamp")
	for i := 0; i < 16; i++ {
		header = append(header, fmt.Sprintf("CH%02d", i+1))
	}
	if err := r.writer.Write(header); err != nil {
		f.Close()
		return fmt.Errorf("write header: %w", err)
	}
	r.writer.Flush()
	if err := r.writer.Error(); err != nil {
		f.Close()
		return fmt.Errorf("flush header: %w", err)
	}

	r.file = f
	r.pendingRows = 0
	r.lastFlush = time.Now()
	r.session = core.RecordingSession{
		ID:          fmt.Sprintf("rec_%d", time.Now().UnixNano()),
		OutputDir:   outputDir,
		FilePrefix:  prefix,
		StartTimeMs: time.Now().UnixMilli(),
		Status:      core.RecordingActive,
	}
	return nil
}

func (r *CSVRecorder) Write(snapshot core.TemperatureSnapshot) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.session.Status != core.RecordingActive || r.writer == nil {
		return nil
	}

	// 优先使用设备硬件时间戳，否则使用电脑本地时间
	var t time.Time
	if snapshot.HardwareTimestamp > 0 {
		sec := int64(snapshot.HardwareTimestamp)
		nsec := int64((snapshot.HardwareTimestamp - float64(sec)) * 1e9)
		t = time.Unix(sec, nsec)
	} else {
		t = time.UnixMilli(snapshot.Timestamp)
	}

	record := make([]string, 0, 17)
	record = append(record, t.Format("2006-01-02 15:04:05.000"))
	for _, v := range snapshot.Values {
		record = append(record, fmt.Sprintf("%.3f", v))
	}
	if err := r.writer.Write(record); err != nil {
		return err
	}
	r.pendingRows++
	if r.pendingRows >= csvFlushRows || time.Since(r.lastFlush) >= csvFlushInterval {
		if err := r.flushLocked(); err != nil {
			return err
		}
	}
	r.session.SnapshotCount++
	return nil
}

func (r *CSVRecorder) flushLocked() error {
	if r.writer == nil {
		return nil
	}
	r.writer.Flush()
	if err := r.writer.Error(); err != nil {
		return err
	}
	r.pendingRows = 0
	r.lastFlush = time.Now()
	return nil
}

func (r *CSVRecorder) Stop() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.writer != nil {
		if err := r.flushLocked(); err != nil {
			return err
		}
		r.writer = nil
	}
	if r.file != nil {
		r.file.Close()
		r.file = nil
	}
	r.session.Status = core.RecordingIdle
	return nil
}

func (r *CSVRecorder) Status() core.RecordingSession {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.session
}
