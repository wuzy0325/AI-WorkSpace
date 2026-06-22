package recording

import (
	"encoding/csv"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"daq-t1603/core"
)

const (
	// csvFlushRows 增大到 2000，在 1000Hz 多设备场景下让 csvFlushInterval（1s）主导 flush 节奏，
	// 避免每 0.1s 高频磁盘同步。单设备 1000Hz 时 2000 行 = 2s，多设备 N×1000Hz 时更快触发。
	csvFlushRows     = 2000
	csvFlushInterval = time.Second
)

// CSVRecorder 将采集数据写入 CSV 文件。
// 多设备 1000Hz 场景下，Write 是热路径，优化要点：
//   - strconv.FormatFloat 替代 fmt.Sprintf，避免格式串解析开销
//   - csvFlushRows 提高到 2000，让时间间隔主导 flush
//   - sync.Mutex 替代 sync.RWMutex（Status 调用频率低，读写锁的额外开销不值得）
type CSVRecorder struct {
	mu          sync.Mutex
	file        *os.File
	session     core.RecordingSession
	writer      *csv.Writer
	pendingRows int
	lastFlush   time.Time
	// active 是 session.Status == RecordingActive 的无锁镜像。
	// 热路径（relayStream）每条 snapshot 都需要判断是否录制，走 mu.Lock 会与
	// 1000Hz×N 设备的 Write 调用产生严重锁竞争；用 atomic.Bool 让热路径完全无锁，
	// 同时保证 Start/Stop 的语义对其他 goroutine 立即可见。
	active atomic.Bool
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

	// 写入 CSV 表头：Timestamp + CH01~CH16
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
	// 在 session 全部就绪后才标记 active，避免热路径看到 active=true 但 writer 未初始化。
	r.active.Store(true)
	return nil
}

// IsActive 返回当前是否处于录制状态（无锁热路径）。
// 用于 relayStream 等高频调用方避免每次 Status() 调用产生锁竞争。
func (r *CSVRecorder) IsActive() bool {
	return r.active.Load()
}

// Write 将一次温度采样快照写入 CSV 文件。
// 多设备 1000Hz 场景下此方法是热路径，优化要点：
//   - strconv.FormatFloat 替代 fmt.Sprintf，每秒节省 16000 次格式串解析
//   - 锁持有时间最小化，仅在 csv.Writer.Write 和 flush 判断期间持锁
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

	// 构建 CSV 行：用 strconv.FormatFloat 替代 fmt.Sprintf，
	// 避免格式串 "%.3f" 的解析开销。1000Hz × 16 通道 = 每秒 16000 次调用
	record := make([]string, 17)
	record[0] = t.Format("2006-01-02 15:04:05.000")
	for i, v := range snapshot.Values {
		record[i+1] = strconv.FormatFloat(v, 'f', 3, 64)
	}

	if err := r.writer.Write(record); err != nil {
		return err
	}
	r.pendingRows++

	// flush 策略：优先按时间间隔（1s），行数阈值（2000）作为兜底
	// 多设备 1000Hz 时，时间间隔会先触发，保证 flush 频率可控
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

// Stop 停止录制，最终 flush 并关闭文件，保证缓冲数据不丢失
func (r *CSVRecorder) Stop() error {
	// 先翻转 active，让热路径立即停止 Write 调用，避免在 flush 期间继续追加。
	r.active.Store(false)
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

// Status 返回当前录制会话状态
func (r *CSVRecorder) Status() core.RecordingSession {
	r.mu.Lock()
	s := r.session
	r.mu.Unlock()
	return s
}