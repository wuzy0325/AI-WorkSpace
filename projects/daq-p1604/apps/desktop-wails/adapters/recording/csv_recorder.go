package recording

import (
	"encoding/csv"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"daq-p1604/core"
)

const (
	csvFlushRows     = 100
	csvFlushInterval = time.Second
	// numChannels 与 P1604 硬件通道数保持一致（16 路压力 + 大气压 + 大气温）
	numChannels = 18
	// numPressureChannels 仅压力通道数，用于 CSV 表头中 CH01..CH16
	numPressureChannels = 16
	// defaultPrecision CSV 输出默认小数位数
	defaultPrecision = 4
	// maxPrecision 用户允许配置的最大小数位数
	maxPrecision = 6
)

// CSVRecorder CSV 文件录制器
type CSVRecorder struct {
	mu          sync.RWMutex
	file        *os.File
	session     core.RecordingSession
	writer      *csv.Writer
	pendingRows int
	lastFlush   time.Time
	precisions  []int // 每个通道的保存精度（小数位数），按通道索引取值
}

// NewCSVRecorder 创建 CSV 录制器
func NewCSVRecorder() *CSVRecorder {
	return &CSVRecorder{}
}

// Start 开始录制，channels 用于确定每个通道的保存精度
func (r *CSVRecorder) Start(outputDir string, prefix string, channels []core.ChannelConfig) error {
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

	// 18 通道 CSV 表头
	header := make([]string, 0, numChannels+1)
	header = append(header, "Timestamp")
	for i := 0; i < numPressureChannels; i++ {
		header = append(header, fmt.Sprintf("CH%02d", i+1))
	}
	header = append(header, "CH17_AtmPressure")
	header = append(header, "CH18_AtmTemp")
	if err := r.writer.Write(header); err != nil {
		f.Close()
		return fmt.Errorf("write header: %w", err)
	}
	r.writer.Flush()
	if err := r.writer.Error(); err != nil {
		f.Close()
		return fmt.Errorf("flush header: %w", err)
	}

	// 提取每通道精度，未配置时默认 4 位小数
	r.precisions = make([]int, numChannels)
	for i := 0; i < numChannels; i++ {
		p := defaultPrecision
		if i < len(channels) {
			cp := channels[i].Precision
			if cp >= 0 && cp <= maxPrecision {
				p = cp
			}
		}
		r.precisions[i] = p
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

// Write 写入一条数据快照
func (r *CSVRecorder) Write(snapshot core.PressureSnapshot) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.session.Status != core.RecordingActive || r.writer == nil {
		return nil
	}

	var t time.Time
	if snapshot.HardwareTimestamp > 0 {
		sec := int64(snapshot.HardwareTimestamp)
		nsec := int64((snapshot.HardwareTimestamp - float64(sec)) * 1e9)
		t = time.Unix(sec, nsec)
	} else {
		t = time.UnixMilli(snapshot.Timestamp)
	}

	record := make([]string, 0, numChannels+1)
	record = append(record, t.Format("2006-01-02 15:04:05.000"))
	for i, v := range snapshot.Values {
		p := defaultPrecision
		if i < len(r.precisions) {
			p = r.precisions[i]
		}
		record = append(record, fmt.Sprintf("%.*f", p, v))
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

// Stop 停止录制
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

// Status 获取录制状态
func (r *CSVRecorder) Status() core.RecordingSession {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.session
}
