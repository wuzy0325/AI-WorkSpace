package logging

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	logFlushInterval = 3 * time.Second
	logFlushRows     = 50
	// logBufferSize bufio 缓冲区大小（256KB）
	logBufferSize = 256 * 1024
)

// LogFileWriter 将日志条目追加写入到本地文件
//
// 优化点：
//   - 使用 strings.Builder + strconv 替代 fmt.Sprintf，避免反射开销
//   - bufio 缓冲 256KB，减少 syscall 次数
//   - 后台 flushLoop 定时刷盘，Write 路径仅写入 bufio 内存
type LogFileWriter struct {
	mu          sync.Mutex
	file        *os.File
	writer      *bufio.Writer
	active      bool
	outputDir   string
	filePrefix  string
	pendingRows int
	lastFlush   time.Time
	stopCh      chan struct{}
	doneCh      chan struct{}
}

// NewLogFileWriter 创建日志文件写入器
func NewLogFileWriter() *LogFileWriter {
	return &LogFileWriter{}
}

// Start 创建日志文件并开始写入
func (w *LogFileWriter) Start(outputDir string, prefix string) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.active {
		return fmt.Errorf("日志文件写入已在进行中")
	}

	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return fmt.Errorf("创建日志目录失败: %w", err)
	}

	filename := fmt.Sprintf("%s_%s.log", prefix, time.Now().Format("20060102-150405"))
	filePath := filepath.Join(outputDir, filename)

	f, err := os.Create(filePath)
	if err != nil {
		return fmt.Errorf("创建日志文件失败: %w", err)
	}

	w.writer = bufio.NewWriterSize(f, logBufferSize)
	w.file = f
	w.active = true
	w.outputDir = outputDir
	w.filePrefix = prefix
	w.pendingRows = 0
	w.lastFlush = time.Now()
	w.stopCh = make(chan struct{})
	w.doneCh = make(chan struct{})

	// 写入表头（TSV 格式：Timestamp \t Level \t Category \t DeviceID \t Source \t Message \t Detail）
	header := "# Timestamp\tLevel\tCategory\tDeviceID\tSource\tMessage\tDetail\n"
	if _, err := w.writer.WriteString(header); err != nil {
		f.Close()
		w.active = false
		return fmt.Errorf("写入日志文件头失败: %w", err)
	}
	_ = w.writer.Flush()

	go w.flushLoop()

	return nil
}

// Write 写入一条日志到文件
// 使用 strings.Builder + strconv 替代 fmt.Sprintf，避免反射开销
func (w *LogFileWriter) Write(timestamp int64, level string, category string, deviceID string, source string, message string, detail string) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if !w.active || w.writer == nil {
		return nil
	}

	t := time.UnixMilli(timestamp)

	// strings.Builder 零拷贝拼接，strconv 替代 fmt 反射
	var b strings.Builder
	b.Grow(256) // 预分配，避免多次扩容
	b.WriteString(t.Format("2006-01-02 15:04:05.000"))
	b.WriteByte('\t')
	b.WriteString(level)
	b.WriteByte('\t')
	b.WriteString(category)
	b.WriteByte('\t')
	b.WriteString(deviceID)
	b.WriteByte('\t')
	b.WriteString(source)
	b.WriteByte('\t')
	b.WriteString(message)
	b.WriteByte('\t')
	b.WriteString(detail)
	b.WriteByte('\n')

	if _, err := w.writer.WriteString(b.String()); err != nil {
		return err
	}

	w.pendingRows++
	if w.pendingRows >= logFlushRows || time.Since(w.lastFlush) >= logFlushInterval {
		if err := w.flushLocked(); err != nil {
			return err
		}
	}

	return nil
}

// Stop 停止写入并关闭文件
func (w *LogFileWriter) Stop() error {
	w.mu.Lock()

	if !w.active {
		w.mu.Unlock()
		return nil
	}

	if w.stopCh != nil {
		close(w.stopCh)
		w.stopCh = nil
	}

	w.mu.Unlock()

	if w.doneCh != nil {
		<-w.doneCh
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	if w.writer != nil {
		_ = w.flushLocked()
		w.writer = nil
	}
	if w.file != nil {
		_ = w.file.Close()
		w.file = nil
	}
	w.active = false
	w.doneCh = nil
	return nil
}

// IsActive 日志文件写入是否激活
func (w *LogFileWriter) IsActive() bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.active
}

// GetOutputDir 获取当前输出目录
func (w *LogFileWriter) GetOutputDir() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	if !w.active {
		return ""
	}
	return w.outputDir
}

func (w *LogFileWriter) flushLoop() {
	ticker := time.NewTicker(logFlushInterval)
	defer ticker.Stop()
	defer close(w.doneCh)

	for {
		select {
		case <-ticker.C:
			w.mu.Lock()
			if w.active && w.writer != nil {
				_ = w.flushLocked()
			}
			w.mu.Unlock()
		case <-w.stopCh:
			return
		}
	}
}

func (w *LogFileWriter) flushLocked() error {
	if w.writer == nil {
		return nil
	}
	if err := w.writer.Flush(); err != nil {
		return err
	}
	w.pendingRows = 0
	w.lastFlush = time.Now()
	return nil
}
