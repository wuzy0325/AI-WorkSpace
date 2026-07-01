package recording

import (
	"bufio"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"daq-p1604/core"
)

const (
	// csvDefaultQueueCapacity 异步队列默认容量。
	// 1kHz × 10 设备 = 1 万 payloads/sec，2s fsync 间隔下偶发 I/O stall 可积压 2000-5000 payloads，
	// 32k 容量可缓冲 6-15 次 fsync 周期的积压。
	csvDefaultQueueCapacity = 32768
	// csvDefaultBufferSize bufio 缓冲区大小（1MB）
	csvDefaultBufferSize = 1 << 20
	// csvDefaultFlushIntervalMs bufio flush 间隔（仅刷到 OS 缓冲，廉价）
	csvDefaultFlushIntervalMs = 100
	// csvDefaultSyncIntervalSec fsync 间隔（真正落盘，昂贵）
	csvDefaultSyncIntervalSec = 2
	// csvDropLogInterval 丢弃日志节流间隔，避免高频丢弃时刷屏
	csvDropLogInterval = 5 * time.Second

	// numChannels 与 P1604 硬件通道数保持一致（16 路压力 + 大气压 + 大温）
	numChannels = 18
	// numPressureChannels 仅压力通道数，用于 CSV 表头中 CH01..CH16
	numPressureChannels = 16
	// defaultPrecision CSV 输出默认小数位数
	defaultPrecision = 4
	// maxPrecision 用户允许配置的最大小数位数
	maxPrecision = 6
)

// CSVRecorder CSV 异步批量录制器
//
// 设计要点：
//   - Write 通过 select-default 非阻塞投递到 queue channel，绝不阻塞设备 read loop
//   - 单一 writer goroutine 串行消费 channel 写入文件，消除多设备锁争用
//   - flush (100ms) 与 fsync (2s) 分离：flush 廉价（仅刷 OS 缓冲），fsync 昂贵（真正落盘）
//   - strconv.AppendFloat 替代 fmt.Sprintf，避免反射开销，吞吐提升 2-5 倍
//   - 支持 FileRotation（按大小/时长/记录数滚动到新文件）
//   - 支持 StopConditions（达到任一条件自动停止录制）
//   - Start/Stop 用 atomic.Bool CAS 保护，热路径 Write 无锁
type CSVRecorder struct {
	// 配置（Start 时设置，writer goroutine 只读）
	cfg core.RecordingConfig

	// 异步队列
	queue       chan core.PressureSnapshot
	stopCh      chan struct{}
	doneCh      chan struct{}
	started     atomic.Bool
	autoDone    chan struct{}   // 自动停止条件触发时关闭，用于通知 Stop 路径
	autoDoneOnce sync.Once      // 保证 close(autoDone) 幂等，避免重复关闭 panic

	// 运行时状态（writer goroutine 单线程更新，Status 读时加锁）
	statsMu         sync.RWMutex
	session         core.RecordingSession
	fileSize        int64 // 当前文件大小
	totalSize       int64 // 所有文件累计大小
	recordCount     int64 // 所有文件累计记录数
	fileCount       int   // 当前已创建的文件数
	fileRecordCount int64 // 当前文件记录数（用于 MaxRecordCount 滚动判断）
	currentStart    int64 // 当前文件起始时间（UnixMilli）

	// 丢弃计数（atomic 无锁）
	dropped          atomic.Int64
	droppedSinceLog  atomic.Int64
	lastDropLogAt    atomic.Int64

	// 错误（writer goroutine 写，Status 读）
	errMu      sync.RWMutex
	lastError  string

	// 写入位置（writer goroutine 独占，无需锁）
	file *os.File
	bw   *bufio.Writer
}

// NewCSVRecorder 创建 CSV 录制器
func NewCSVRecorder() *CSVRecorder {
	return &CSVRecorder{}
}

// Start 开始录制
func (r *CSVRecorder) Start(config core.RecordingConfig) error {
	if !r.started.CompareAndSwap(false, true) {
		return fmt.Errorf("recording already in progress")
	}

	// 应用默认值
	cfg := applyCSVDefaults(config)

	if err := os.MkdirAll(cfg.OutputDir, 0755); err != nil {
		r.started.Store(false)
		return fmt.Errorf("create output dir: %w", err)
	}

	r.cfg = cfg
	r.queue = make(chan core.PressureSnapshot, cfg.QueueCapacity)
	r.stopCh = make(chan struct{})
	r.doneCh = make(chan struct{})
	r.autoDone = make(chan struct{})
	r.autoDoneOnce = sync.Once{} // 重置 Once，允许本次会话重新 close(autoDone)
	r.dropped.Store(0)
	r.droppedSinceLog.Store(0)
	r.lastDropLogAt.Store(0)
	r.fileSize = 0
	r.totalSize = 0
	r.recordCount = 0
	r.fileCount = 0
	// 重置上次会话的 lastError，避免新会话 Status() 返回旧错误
	r.errMu.Lock()
	r.lastError = ""
	r.errMu.Unlock()

	now := time.Now().UnixMilli()
	r.statsMu.Lock()
	r.session = core.RecordingSession{
		ID:          fmt.Sprintf("rec_%d", now),
		OutputDir:   cfg.OutputDir,
		FilePrefix:  cfg.FilePrefix,
		Format:      core.RecordingFormatCSV,
		StartTimeMs: now,
		Status:      core.RecordingActive,
	}
	r.statsMu.Unlock()

	// 同步创建首个文件：保证 Start 返回时 session.CurrentFile 已设置，
	// 后端 emit 状态时前端可立即显示完整文件路径；失败时直接返回错误，
	// 避免启动 writer goroutine 后再异步报告错误。
	if err := r.openNewFile(); err != nil {
		r.started.Store(false)
		return err
	}

	go r.writerLoop()
	return nil
}

// applyCSVDefaults 填充默认值
func applyCSVDefaults(cfg core.RecordingConfig) core.RecordingConfig {
	if cfg.FlushIntervalMs <= 0 {
		cfg.FlushIntervalMs = csvDefaultFlushIntervalMs
	}
	if cfg.SyncIntervalSec <= 0 {
		cfg.SyncIntervalSec = csvDefaultSyncIntervalSec
	}
	if cfg.QueueCapacity <= 0 {
		cfg.QueueCapacity = csvDefaultQueueCapacity
	}
	return cfg
}

// Write 异步投递一条快照到队列（非阻塞，队列满时丢弃并计数）
func (r *CSVRecorder) Write(snapshot core.PressureSnapshot) error {
	if !r.started.Load() {
		return fmt.Errorf("recording sink is not started")
	}
	select {
	case r.queue <- snapshot:
	default:
		// 队列满：丢弃并计数（快路径，无锁）
		r.dropped.Add(1)
		r.droppedSinceLog.Add(1)
		// 节流检查：上次日志时间距今超过 csvDropLogInterval 才进入慢路径
		last := r.lastDropLogAt.Load()
		now := time.Now().UnixNano()
		if now-last < int64(csvDropLogInterval) {
			return nil
		}
		// CAS 抢占日志权：避免多 goroutine 同时输出
		if !r.lastDropLogAt.CompareAndSwap(last, now) {
			return nil
		}
		sinceLog := r.droppedSinceLog.Swap(0)
		slog.Warn("CSVRecorder 队列已满，丢弃 payload（节流聚合）",
			"droppedSinceLog", sinceLog,
			"totalDropped", r.dropped.Load())
	}
	return nil
}

// Stop 停止录制：drain 队列后关闭文件
func (r *CSVRecorder) Stop() error {
	if !r.started.CompareAndSwap(true, false) {
		return nil
	}
	close(r.stopCh)
	<-r.doneCh

	// 同步最终状态
	r.statsMu.Lock()
	r.session.Status = core.RecordingIdle
	r.session.DroppedCount = int(r.dropped.Load())
	r.session.FileCount = r.fileCount
	r.session.SnapshotCount = int(r.recordCount)
	r.errMu.RLock()
	r.session.LastError = r.lastError
	r.errMu.RUnlock()
	r.statsMu.Unlock()
	return nil
}

// Status 获取录制状态
func (r *CSVRecorder) Status() core.RecordingSession {
	r.statsMu.RLock()
	defer r.statsMu.RUnlock()
	s := r.session
	s.SnapshotCount = int(r.recordCount)
	s.DroppedCount = int(r.dropped.Load())
	s.FileCount = r.fileCount
	r.errMu.RLock()
	s.LastError = r.lastError
	r.errMu.RUnlock()
	return s
}

// writerLoop 单 writer goroutine 串行消费队列
func (r *CSVRecorder) writerLoop() {
	// 捕获本次会话的 doneCh 到局部变量：auto-stop 翻转 started 后到此处 defer 之间，
	// 若被 usecase 重入 Start 重新赋值 r.doneCh，旧 goroutine 关闭的仍是自己的 channel，
	// 不会误关新会话的 doneCh 导致新 writerLoop 提前退出。
	doneCh := r.doneCh
	defer close(doneCh)

	flushTicker := time.NewTicker(time.Duration(r.cfg.FlushIntervalMs) * time.Millisecond)
	syncTicker := time.NewTicker(time.Duration(r.cfg.SyncIntervalSec) * time.Second)
	defer flushTicker.Stop()
	defer syncTicker.Stop()

	var buf []byte // 复用 byte buffer 用于格式化

	// 首个文件已在 Start 中同步创建，此处无需再调用 openNewFile
	for {
		select {
		case <-r.stopCh:
			// drain 剩余 payload
			r.drainQueue(&buf)
			r.flushAndSync(true)
			r.closeFile()
			return
		case p := <-r.queue:
			if err := r.writePayload(&buf, p); err != nil {
				// I/O 错误：先 flush+close 文件，最后才翻转 started（与 wind-daq 对齐），
				// 避免 started=false 但 writerLoop 仍在做 I/O 的窗口。
				r.recordError(err)
				r.flushAndSync(true)
				r.closeFile()
				r.markStopped()
				return
			}
			// 评估停止条件 / 文件滚动
			if r.shouldAutoStop() {
				// 自停止：drain 剩余数据 → flush+close → markStopped。
				// 全部 I/O 清理完成后才翻转 started，避免与 Stop/Start 重入竞争。
				r.drainQueue(&buf)
				r.flushAndSync(true)
				r.closeFile()
				r.markStopped()
				return
			} else if r.shouldRotate() {
				if err := r.rotateFile(); err != nil {
					r.recordError(err)
					r.flushAndSync(true)
					r.closeFile()
					r.markStopped()
					return
				}
			}
		case <-flushTicker.C:
			r.flushAndSync(false)
		case <-syncTicker.C:
			r.flushAndSync(true)
		}
	}
}

// signalAutoDone 关闭 autoDone channel。
// 仅负责关闭信号 channel（幂等），不修改 started / session 状态——
// 状态翻转由 markStopped 在 I/O 清理完成后统一执行，避免出现
// "started=false 但 writerLoop 仍在做 I/O" 的窗口（曾导致与 Start 重入竞争）。
func (r *CSVRecorder) signalAutoDone() {
	r.autoDoneOnce.Do(func() {
		close(r.autoDone)
	})
}

// markStopped 在 writerLoop 完成所有 I/O 清理（flush+closeFile）后调用，
// 是翻转 started 标志与 session 状态的唯一出口。保证 writerLoop 退出前
// 文件已关闭，Stop/Start 重入不会撞上正在运行的 I/O。
func (r *CSVRecorder) markStopped() {
	r.statsMu.Lock()
	r.session.Status = core.RecordingIdle
	r.statsMu.Unlock()
	r.started.Store(false)
	r.signalAutoDone()
}

// drainQueue 排空队列中剩余 payload
func (r *CSVRecorder) drainQueue(buf *[]byte) {
	for {
		select {
		case p := <-r.queue:
			if err := r.writePayload(buf, p); err != nil {
				r.recordError(err)
				return
			}
		default:
			return
		}
	}
}

// writePayload 写入一条快照（writer goroutine 独占，无需锁）
// 格式：单快照单行 18 列（Timestamp + 16 压力 + 大气压 + 大温）
// 使用 strconv.AppendFloat 替代 fmt.Sprintf，避免反射开销，吞吐提升 2-5 倍。
//
// 时间来源：硬件时间戳优先（snapshot.HardwareTimestamp > 0 时使用，更精确），否则用系统毫秒时间戳。
// Timestamp 列带毫秒（2006-01-02 15:04:05.000），Excel 会识别为文本类型，
// 避免被默认的 "yyyy/m/d h:mm" 格式隐藏秒；1000Hz 采样下相邻样本靠毫秒部分区分。
func (r *CSVRecorder) writePayload(buf *[]byte, snapshot core.PressureSnapshot) error {
	var t time.Time
	if snapshot.HardwareTimestamp > 0 {
		sec := int64(snapshot.HardwareTimestamp)
		nsec := int64((snapshot.HardwareTimestamp - float64(sec)) * 1e9)
		t = time.Unix(sec, nsec)
	} else {
		t = time.UnixMilli(snapshot.Timestamp)
	}

	// 复用 buf，避免每次分配
	b := (*buf)[:0]
	// 可读时间戳：前缀单引号强制 Excel 按文本显示，带毫秒保证秒和毫秒均完整可见。
	// 单引号是 Excel 的"文本前缀"语法，仅影响显示格式，不进入单元格内容。
	b = append(b, '\'')
	b = t.AppendFormat(b, "2006-01-02 15:04:05.000")
	b = append(b, ',')

	for i, v := range snapshot.Values {
		p := defaultPrecision
		if i < len(r.cfg.Channels) {
			cp := r.cfg.Channels[i].Precision
			if cp >= 0 && cp <= maxPrecision {
				p = cp
			}
		}
		// strconv.AppendFloat 复用底层数组，零分配
		b = strconv.AppendFloat(b, v, 'f', p, 64)
		b = append(b, ',')
	}

	// 去掉末尾逗号，写入换行
	b[len(b)-1] = '\n'
	if _, err := r.bw.Write(b); err != nil {
		return err
	}
	written := int64(len(b))
	*buf = b

	// 更新统计（writer goroutine 单线程，但仍持锁以与 Status() 互斥）
	r.statsMu.Lock()
	r.recordCount++
	r.fileRecordCount++
	r.fileSize += written
	r.totalSize += written
	r.statsMu.Unlock()
	return nil
}

// shouldRotate 评估是否需要滚动到新文件
func (r *CSVRecorder) shouldRotate() bool {
	rot := r.cfg.Rotation
	if rot.MaxSizeBytes > 0 && r.fileSize >= rot.MaxSizeBytes {
		return true
	}
	if rot.MaxRecordCount > 0 && r.fileRecordCount >= rot.MaxRecordCount {
		return true
	}
	if rot.MaxDurationMs > 0 {
		now := time.Now().UnixMilli()
		if now-r.currentStart >= rot.MaxDurationMs {
			return true
		}
	}
	return false
}

// shouldAutoStop 评估是否触发自动停止
func (r *CSVRecorder) shouldAutoStop() bool {
	sc := r.cfg.StopConditions
	r.statsMu.RLock()
	totalSize := r.totalSize
	recordCount := r.recordCount
	startTime := r.session.StartTimeMs
	r.statsMu.RUnlock()

	if sc.MaxFileSizeBytes > 0 && totalSize >= sc.MaxFileSizeBytes {
		return true
	}
	if sc.MaxRecordCount > 0 && recordCount >= sc.MaxRecordCount {
		return true
	}
	if sc.MaxDurationMs > 0 {
		now := time.Now().UnixMilli()
		if now-startTime >= sc.MaxDurationMs {
			return true
		}
	}
	return false
}

// openNewFile 创建新文件并写入表头
func (r *CSVRecorder) openNewFile() error {
	filename := fmt.Sprintf("%s_%s.csv", r.cfg.FilePrefix, time.Now().Format("20060102-150405"))
	filePath := filepath.Join(r.cfg.OutputDir, filename)
	f, err := os.Create(filePath)
	if err != nil {
		return fmt.Errorf("create file: %w", err)
	}

	bw := bufio.NewWriterSize(f, csvDefaultBufferSize)

	// 18 列 CSV 表头：Timestamp（带毫秒）+ 16 压力 + 大气压 + 大温
	header := make([]string, 0, numChannels+1)
	header = append(header, "Timestamp")
	for i := 0; i < numPressureChannels; i++ {
		header = append(header, fmt.Sprintf("CH%02d", i+1))
	}
	header = append(header, "CH17_AtmPressure")
	header = append(header, "CH18_AtmTemp")
	if _, err := bw.Write([]byte(joinCSVHeader(header))); err != nil {
		f.Close()
		return fmt.Errorf("write header: %w", err)
	}
	if err := bw.Flush(); err != nil {
		f.Close()
		return fmt.Errorf("flush header: %w", err)
	}

	r.file = f
	r.bw = bw
	r.fileSize = 0
	r.fileRecordCount = 0
	r.currentStart = time.Now().UnixMilli()
	r.fileCount++

	r.statsMu.Lock()
	r.session.FileCount = r.fileCount
	r.session.CurrentFile = filePath
	r.statsMu.Unlock()
	return nil
}

// joinCSVHeader 简单的 CSV 表头拼接（避免引入 encoding/csv 的反射开销）
func joinCSVHeader(fields []string) string {
	var b []byte
	for i, f := range fields {
		if i > 0 {
			b = append(b, ',')
		}
		b = append(b, f...)
	}
	b = append(b, '\n')
	return string(b)
}

// rotateFile 滚动到新文件
func (r *CSVRecorder) rotateFile() error {
	if err := r.flushAndSync(true); err != nil {
		return err
	}
	r.closeFile()
	return r.openNewFile()
}

// closeFile 关闭当前文件
func (r *CSVRecorder) closeFile() {
	if r.bw != nil {
		_ = r.bw.Flush()
		r.bw = nil
	}
	if r.file != nil {
		_ = r.file.Close()
		r.file = nil
	}
}

// flushAndSync flush bufio 到 OS 缓冲；sync=true 时额外 fsync 落盘
func (r *CSVRecorder) flushAndSync(sync bool) error {
	if r.bw == nil {
		return nil
	}
	if err := r.bw.Flush(); err != nil {
		return err
	}
	if sync && r.file != nil {
		if err := r.file.Sync(); err != nil {
			return err
		}
	}
	return nil
}

// recordError 记录最近一次 I/O 错误（lastError + 日志）。
// 仅记录错误信息，不翻转 started/session 状态——状态翻转由调用方在
// 完成 flush+closeFile 后通过 markStopped 统一执行，避免出现
// "started=false 但 writerLoop 仍在做 I/O" 的窗口。
func (r *CSVRecorder) recordError(err error) {
	r.errMu.Lock()
	r.lastError = err.Error()
	r.errMu.Unlock()
	r.statsMu.Lock()
	r.session.LastError = err.Error()
	r.statsMu.Unlock()
	slog.Error("CSVRecorder 写入失败", "error", err)
}

// IsActive 无锁热路径判活。relayStream 每帧调用，避免 Status() 的双 RLock
// 与 writer goroutine 的 statsMu 写锁在 1kHz×N 设备下争用。
func (r *CSVRecorder) IsActive() bool {
	return r.started.Load()
}
