package logging

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	// logFlushInterval 日志文件刷新间隔
	logFlushInterval = 3 * time.Second
	// logFlushRows 累积条数达到此值时刷新
	logFlushRows = 50
	// maxLogFileCount 日志文件数量上限（LOG-015）：超过此值时删除最旧的同前缀日志文件
	// 7 是测试用例中报告的"超过 7 个"问题的对应上限
	maxLogFileCount = 7
)

// LogFileWriter 将日志条目追加写入到本地文件
// 文件格式为每行一条，以制表符分隔字段，便于用 Excel 或文本编辑器查看
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
	doneCh      chan struct{} // flushLoop 退出信号
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

	// 文件名格式：prefix_20060102-150405.log
	filename := fmt.Sprintf("%s_%s.log", prefix, time.Now().Format("20060102-150405"))
	filePath := filepath.Join(outputDir, filename)

	f, err := os.Create(filePath)
	if err != nil {
		return fmt.Errorf("创建日志文件失败: %w", err)
	}

	w.writer = bufio.NewWriter(f)
	w.file = f
	w.active = true
	w.outputDir = outputDir
	w.filePrefix = prefix
	w.pendingRows = 0
	w.lastFlush = time.Now()
	w.stopCh = make(chan struct{})
	w.doneCh = make(chan struct{})

	// 写入文件头
	header := "# Timestamp\tLevel\tCategory\tDeviceID\tSource\tMessage\tDetail\n"
	if _, err := w.writer.WriteString(header); err != nil {
		f.Close()
		w.active = false
		return fmt.Errorf("写入日志文件头失败: %w", err)
	}
	_ = w.writer.Flush()

	// LOG-015 轮转：新建文件后立即清理目录下同前缀的旧日志，
	// 保证日志文件总数不超过 maxLogFileCount 个。删除失败不阻塞当前会话。
	pruneOldLogFiles(outputDir, prefix, maxLogFileCount)

	// 启动定时刷新协程
	go w.flushLoop()

	return nil
}

// Write 写入一条日志到文件
func (w *LogFileWriter) Write(timestamp int64, level string, category string, deviceID string, source string, message string, detail string) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if !w.active || w.writer == nil {
		return nil
	}

	// 格式化时间戳
	t := time.UnixMilli(timestamp)
	line := fmt.Sprintf("%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
		t.Format("2006-01-02 15:04:05.000"),
		level,
		category,
		deviceID,
		source,
		message,
		detail,
	)

	if _, err := w.writer.WriteString(line); err != nil {
		return err
	}

	w.pendingRows++
	// 累积到一定条数时刷新
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

	// 已停止时直接返回，防止重复 close(stopCh) 导致 panic
	if !w.active {
		w.mu.Unlock()
		return nil
	}

	// 通知刷新协程退出
	if w.stopCh != nil {
		close(w.stopCh)
		w.stopCh = nil
	}

	// 先释放锁，等待 flushLoop 退出
	// flushLoop 退出后不会再访问 writer/file
	w.mu.Unlock()

	// 等待 flushLoop 协程退出
	if w.doneCh != nil {
		<-w.doneCh
	}

	// 重新获取锁，安全清理资源
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

// GetOutputDir 获取当前输出目录，未激活时返回空字符串
func (w *LogFileWriter) GetOutputDir() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	if !w.active {
		return ""
	}
	return w.outputDir
}

// flushLoop 定时刷新缓冲区到磁盘
func (w *LogFileWriter) flushLoop() {
	ticker := time.NewTicker(logFlushInterval)
	defer ticker.Stop()
	defer close(w.doneCh) // 通知 Stop 协程已退出

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

// flushLocked 在已持有锁的情况下刷新缓冲区
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

// pruneOldLogFiles 删除目录下同前缀的旧日志文件，保留最新的 maxCount 个。
// 文件名格式 `prefix_YYYYMMDD-HHMMSS.log` 包含时间戳，按文件名升序排序即等同按创建时间排序。
// 删除失败时静默忽略，避免影响当前会话；只匹配 .log 后缀避免误删其他文件。
//
// LOG-015：用户反馈长时间运行后日志文件总数超过 7 个，
// 这里在每次 Start 新文件后立即清理，确保总数 <= maxCount。
func pruneOldLogFiles(outputDir string, prefix string, maxCount int) {
	if maxCount <= 0 || outputDir == "" || prefix == "" {
		return
	}
	entries, err := os.ReadDir(outputDir)
	if err != nil {
		return
	}
	// 收集同前缀的 .log 文件名
	prefixWithUnderscore := prefix + "_"
	var names []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasPrefix(name, prefixWithUnderscore) {
			continue
		}
		if !strings.HasSuffix(name, ".log") {
			continue
		}
		names = append(names, name)
	}
	// 数量未超限，无需清理
	if len(names) <= maxCount {
		return
	}
	// 升序排序：文件名时间戳早的排前面，即最旧的
	sort.Strings(names)
	// 删除最旧的 (len - maxCount) 个
	toDelete := len(names) - maxCount
	for i := 0; i < toDelete; i++ {
		_ = os.Remove(filepath.Join(outputDir, names[i]))
	}
}
