// Package storage 提供 wind-daq 的存储适配器实现。
//
// CSVRecordingSink 采用异步批量写设计，支撑 1kHz × 10 设备的全量保存场景：
//   - 每设备一个 CSV 文件，按 deviceId 路由（避免多设备数据混杂在同一文件）
//   - DAQ-P-1604 用宽格式（18 通道列，对齐 daq-p1604 项目）
//   - 其他设备用长格式（每通道一行：timestamp,deviceId,channelIndex,value）
//   - Write 仅把 payload 投递到带缓冲的 channel，立即返回，不阻塞设备 read loop
//   - 单独的 writer goroutine 消费 channel，使用 bufio.Writer 聚合写入
//   - 定时 Flush + 定时 Sync，避免每条记录 fsync 风暴
//   - 支持文件滚动（FileRotation）：按设备独立评估，达到阈值时切换该设备文件
//   - 支持自动停止条件（StopConditions）：跨设备汇总评估，任一设备触发则整体停止
//   - Stop 时 drain 剩余 payload 后关闭所有文件，保证数据完整落盘
//   - 文件名采用 <prefix>-<deviceId>-YYYYMMDD-HHMMSS-NNN.csv 格式，NNN 为该设备本会话滚动序号
package storage

import (
	"bufio"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"wind-daq/services/api-go/internal/core/device"
	corestorage "wind-daq/services/api-go/internal/core/storage"
)

// 默认配置常量
const (
	// defaultQueueCapacity 异步队列容量。
	// 取 32768 用于缓冲 1kHz × 10 设备（=1 万 payloads/sec）在 fsync stall 期间的积压：
	//   - 默认 2s fsync 间隔，单次 fsync 在机械盘可达 200-500ms，期间积压 2000-5000 payloads
	//   - 32768 可缓冲约 6-15 次 fsync 周期的积压，应对偶发 I/O stall（杀毒扫描、磁盘满）
	//   - 32k 容量 × 平均 200B/payload ≈ 6.4MB 内存占用，可接受
	defaultQueueCapacity = 32768
	// defaultBufferSize bufio 缓冲大小 1MB
	defaultBufferSize = 1 << 20
	// defaultFlushIntervalMs 定时 flush 间隔，平衡延迟与吞吐
	defaultFlushIntervalMs = 100
	// defaultSyncIntervalSec 定时 fsync 间隔，避免每条记录 fsync 风暴
	defaultSyncIntervalSec = 2
	// dropLogInterval 丢弃告警的节流间隔：至多每 5 秒输出一条聚合日志，
	// 避免 1kHz × 10 设备遇到慢速 I/O 时按数据速率刷屏（与 AcquisitionHub 对齐）。
	dropLogInterval = 5 * time.Second

	// daqP1604WideFormatChannels DAQ-P-1604 宽格式固定 18 通道列（16 压力 + 大气压力 + 大气温度）
	daqP1604WideFormatChannels = 18
	// csvFloatPrecision CSV 输出小数位数（与历史行为保持一致）
	csvFloatPrecision = 6
)

// CSVSinkConfig 异步 CSV 存储配置。
// 零值字段会被替换为默认值。
type CSVSinkConfig struct {
	// QueueCapacity 异步队列容量；队列满时丢弃新 payload 并计数
	QueueCapacity int
	// BufferSize bufio.Writer 缓冲大小
	BufferSize int
	// FlushInterval 定时把 bufio 缓冲刷到 OS 文件缓冲
	FlushInterval time.Duration
	// SyncInterval 定时调用 file.Sync()（fsync），保证崩溃时数据不丢
	SyncInterval time.Duration
}

// DefaultCSVSinkConfig 返回适合 1kHz × 10 设备场景的默认配置
func DefaultCSVSinkConfig() CSVSinkConfig {
	return CSVSinkConfig{
		QueueCapacity: defaultQueueCapacity,
		BufferSize:    defaultBufferSize,
		FlushInterval: defaultFlushIntervalMs * time.Millisecond,
		SyncInterval:  defaultSyncIntervalSec * time.Second,
	}
}

func applyCSVSinkDefaults(cfg *CSVSinkConfig) {
	if cfg.QueueCapacity <= 0 {
		cfg.QueueCapacity = defaultQueueCapacity
	}
	if cfg.BufferSize <= 0 {
		cfg.BufferSize = defaultBufferSize
	}
	if cfg.FlushInterval <= 0 {
		cfg.FlushInterval = defaultFlushIntervalMs * time.Millisecond
	}
	if cfg.SyncInterval <= 0 {
		cfg.SyncInterval = defaultSyncIntervalSec * time.Second
	}
}

// perDeviceWriter 单设备的写入上下文。
// 每个设备独立持有文件、缓冲、统计，设备间互不干扰。
// writer goroutine 单线程访问，无需额外同步（Status 读时通过 statsMu 互斥）。
type perDeviceWriter struct {
	deviceID        string
	deviceType      device.Type
	isWideFormat    bool // DAQ-P-1604 = true（18 通道宽格式），其他 = false（长格式）
	file            *os.File
	bw              *bufio.Writer
	fileName        string
	fileSize        int64 // 当前文件累计字节
	fileCount       int64 // 该设备本会话文件数（含当前文件）
	fileStartedAt   time.Time
	fileRecordCount int64 // 该设备当前文件记录数（用于 MaxRecordCount 滚动判断）
	totalRecords    int64 // 该设备累计记录数
}

// CSVRecordingSink 异步批量写 CSV 存储适配器。
//
// 设计要点：
//   - 每设备一个 CSV 文件，按 deviceId 路由（避免多设备数据混杂）
//   - DAQ-P-1604 用宽格式（18 通道列，对齐 daq-p1604 项目），其他设备用长格式
//   - Write 非阻塞投递，writer goroutine 串行写入，消除多设备锁争用
//   - 文件滚动、停止条件评估、错误反馈在 writer goroutine 内统一处理
type CSVRecordingSink struct {
	cfg     CSVSinkConfig
	started atomic.Bool // CAS 保护 Start/Stop 串行

	// 在 Start 时一次性创建，Stop 后不再修改；Write 通过 atomic 检查 started 后读取
	queue  chan device.DataPayload
	stopCh chan struct{}
	doneCh chan struct{}

	// autoDone 在 sink 因停止条件或 I/O 错误自停止时被关闭；
	// StorageRecorder 通过 Done() 监听该信号以同步自身 recording 状态。
	autoDone     chan struct{}
	autoDoneOnce sync.Once

	// writer goroutine 内部状态
	dropped atomic.Int64 // 队列满时丢弃计数（监控用）

	// drop 节流日志状态：lastDropLogAt 用 atomic 存储 UnixNano 时间戳，
	// 避免每次丢弃都加锁；仅在节流间隔到达时才进入慢路径更新。
	droppedSinceLog atomic.Int64
	lastDropLogAt   atomic.Int64

	// 运行时统计：writerLoop 写、Status 读，用 statsMu 保护。
	// 写入路径只在写完后短暂持锁更新计数，I/O 不持锁。
	statsMu     sync.RWMutex
	config      corestorage.RecordingConfig
	writers     map[string]*perDeviceWriter // deviceId -> writer（每设备一个文件）
	fileCount   int64                       // 跨设备汇总文件数
	recordCount int64                       // 跨设备汇总记录数
	startedAt   time.Time                   // 会话开始时间，用于 DurationMs 与 StopConditions.MaxDurationMs
	lastError   string

	// syncErr 在 writer goroutine 内部出错时设置，Stop 时返回给调用方
	syncErrMu sync.RWMutex
	syncErr   error
}

// NewCSVRecordingSink 创建使用默认配置的异步 CSV sink
func NewCSVRecordingSink() *CSVRecordingSink {
	return NewCSVRecordingSinkWithConfig(DefaultCSVSinkConfig())
}

// NewCSVRecordingSinkWithConfig 创建使用自定义配置的异步 CSV sink
func NewCSVRecordingSinkWithConfig(cfg CSVSinkConfig) *CSVRecordingSink {
	applyCSVSinkDefaults(&cfg)
	return &CSVRecordingSink{cfg: cfg}
}

// Start 启动 writer goroutine。
// 重复调用 Start 会返回错误（CAS 保护）。
// 文件创建延迟到第一个 payload 到达时按 deviceId 创建（避免空文件）。
func (s *CSVRecordingSink) Start(config corestorage.RecordingConfig) error {
	if !s.started.CompareAndSwap(false, true) {
		return fmt.Errorf("recording sink already started")
	}

	if strings.TrimSpace(config.OutputDir) == "" {
		s.started.Store(false)
		return fmt.Errorf("outputDir is required")
	}
	if strings.TrimSpace(config.FilePrefix) == "" {
		s.started.Store(false)
		return fmt.Errorf("filePrefix is required")
	}
	if err := os.MkdirAll(config.OutputDir, 0o755); err != nil {
		s.started.Store(false)
		return err
	}

	// 初始化运行时统计
	now := time.Now()
	s.statsMu.Lock()
	s.config = config
	s.writers = make(map[string]*perDeviceWriter)
	s.fileCount = 0
	s.recordCount = 0
	s.lastError = ""
	s.startedAt = now
	s.statsMu.Unlock()

	s.queue = make(chan device.DataPayload, s.cfg.QueueCapacity)
	s.stopCh = make(chan struct{})
	s.doneCh = make(chan struct{})
	s.autoDone = make(chan struct{})

	// 清除上一次录制可能残留的 I/O 错误，避免 Stop() 返回旧错误
	s.syncErrMu.Lock()
	s.syncErr = nil
	s.syncErrMu.Unlock()

	slog.Info("CSVRecordingSink Start 成功",
		"component", "CSVRecordingSink",
		"outputDir", config.OutputDir,
		"queueCapacity", s.cfg.QueueCapacity,
		"bufferSize", s.cfg.BufferSize,
		"flushInterval", s.cfg.FlushInterval,
		"syncInterval", s.cfg.SyncInterval,
		"rotationEnabled", config.FileRotation.Enabled,
		"stopConditions", config.StopConditions,
	)

	go s.writerLoop()
	return nil
}

// Write 非阻塞地把 payload 投递到异步队列。
// 队列满时丢弃 payload 并计数（不阻塞设备 read loop）。
// 未 Start 时返回错误，保留与旧实现一致的语义。
// sink 自停止（条件达到或 I/O 错误）后 Write 也返回错误，让上层感知。
//
// 丢弃日志使用时间节流（dropLogInterval=5s）+ atomic CAS 双检：
//   - 快路径：仅 atomic.Add 累计计数，无锁开销
//   - 慢路径：CAS 抢占日志权，输出聚合日志（自上次日志以来的累计丢弃数）
//
// 这避免了高丢弃速率下按 1000 次/条刷屏（10kHz 丢弃率 = 10 条/秒）。
func (s *CSVRecordingSink) Write(payload device.DataPayload) error {
	if !s.started.Load() {
		return fmt.Errorf("recording sink is not started")
	}
	queue := s.queue
	if queue == nil {
		return fmt.Errorf("recording sink is not started")
	}
	select {
	case queue <- payload:
	default:
		// 队列满：丢弃并计数（快路径，无锁）
		totalDropped := s.dropped.Add(1)
		s.droppedSinceLog.Add(1)

		// 节流检查：上次日志时间距今超过 dropLogInterval 才进入慢路径
		last := s.lastDropLogAt.Load()
		now := time.Now().UnixNano()
		if now-last < int64(dropLogInterval) {
			return nil
		}
		// CAS 抢占日志权：避免多 goroutine 同时输出
		if !s.lastDropLogAt.CompareAndSwap(last, now) {
			return nil
		}
		sinceLog := s.droppedSinceLog.Swap(0)
		slog.Warn("CSVRecordingSink 队列已满，丢弃 payload（节流聚合）",
			"component", "CSVRecordingSink",
			"deviceId", payload.DeviceID,
			"totalDropped", totalDropped,
			"droppedSinceLastLog", sinceLog,
			"queueCapacity", s.cfg.QueueCapacity,
		)
	}
	return nil
}

// Stop 通知 writer goroutine 退出，drain 剩余 payload 后关闭所有文件。
// 重复调用 Stop 是幂等的，返回 nil。
// 如果 writer goroutine 内部出错，Stop 返回该错误。
func (s *CSVRecordingSink) Stop() error {
	if !s.started.CompareAndSwap(true, false) {
		return nil
	}
	close(s.stopCh)
	<-s.doneCh

	s.syncErrMu.RLock()
	err := s.syncErr
	s.syncErrMu.RUnlock()
	return err
}

// Status 返回当前录制状态快照。
// 由 StorageRecorder 转发给上层用于 UI 展示与错误反馈。
// CurrentFile 返回所有设备当前文件名（逗号分隔），便于 UI 显示。
func (s *CSVRecordingSink) Status() corestorage.RecordingStatus {
	s.statsMu.RLock()
	defer s.statsMu.RUnlock()

	var durationMs int64
	if !s.startedAt.IsZero() {
		durationMs = time.Since(s.startedAt).Milliseconds()
	}

	// 聚合所有设备的文件名
	var currentFiles []string
	var totalFileSize int64
	for _, w := range s.writers {
		if w.fileName != "" {
			currentFiles = append(currentFiles, w.fileName)
		}
		totalFileSize += w.fileSize
	}

	return corestorage.RecordingStatus{
		Recording:    s.started.Load(),
		OutputDir:    s.config.OutputDir,
		CurrentFile:  strings.Join(currentFiles, ", "),
		FileSize:     totalFileSize,
		FileCount:    s.fileCount,
		RecordCount:  s.recordCount,
		DurationMs:   durationMs,
		DroppedCount: s.dropped.Load(),
		LastError:    s.lastError,
	}
}

// Done 返回一个 channel，在 sink 因停止条件或 I/O 错误自停止时被关闭。
// StorageRecorder 监听该信号以同步自身 recording 状态。
// 多次调用返回同一 channel。
func (s *CSVRecordingSink) Done() <-chan struct{} {
	return s.autoDone
}

// DroppedCount 返回累计丢弃的 payload 数（监控用）
func (s *CSVRecordingSink) DroppedCount() int64 {
	return s.dropped.Load()
}

// writerLoop 消费 queue 中的 payload，按 deviceId 路由到独立文件。
// 触发条件：payload 到达、flush ticker、sync ticker、stop 信号。
// 任一 I/O 错误都会设置 syncErr 并退出，避免继续写入已损坏的文件。
// 每个 payload 后评估停止条件（跨设备汇总）与文件滚动条件（按设备），必要时自停止或切换该设备文件。
func (s *CSVRecordingSink) writerLoop() {
	defer close(s.doneCh)

	flushTicker := time.NewTicker(s.cfg.FlushInterval)
	defer flushTicker.Stop()
	syncTicker := time.NewTicker(s.cfg.SyncInterval)
	defer syncTicker.Stop()

	// 复用 byte buffer 用于格式化，避免每次 Write 分配
	var buf []byte

	// flushAndSync 遍历所有设备文件执行 flush（+sync）
	flushAndSync := func(sync bool) error {
		s.statsMu.RLock()
		writers := make([]*perDeviceWriter, 0, len(s.writers))
		for _, w := range s.writers {
			writers = append(writers, w)
		}
		s.statsMu.RUnlock()
		for _, w := range writers {
			if err := w.bw.Flush(); err != nil {
				return fmt.Errorf("flush csv file %s: %w", w.fileName, err)
			}
			if sync && w.file != nil {
				if err := w.file.Sync(); err != nil {
					return fmt.Errorf("sync csv file %s: %w", w.fileName, err)
				}
			}
		}
		return nil
	}

	// failStop 在 writer goroutine 内部出错时被调用：
	// 设置 lastError + syncErr、尝试 flush + 关闭所有文件后退出，
	// 并关闭 autoDone 通知 StorageRecorder。
	failStop := func(err error) {
		s.syncErrMu.Lock()
		s.syncErr = err
		s.syncErrMu.Unlock()
		s.statsMu.Lock()
		s.lastError = err.Error()
		for _, w := range s.writers {
			_ = w.bw.Flush()
			_ = w.file.Close()
		}
		s.statsMu.Unlock()
		s.started.Store(false)
		s.signalAutoDone()
	}

	// closeAllFiles 关闭所有设备文件（正常退出路径）
	closeAllFiles := func() error {
		s.statsMu.Lock()
		defer s.statsMu.Unlock()
		for _, w := range s.writers {
			if err := w.bw.Flush(); err != nil {
				return fmt.Errorf("flush csv file %s: %w", w.fileName, err)
			}
			if err := w.file.Close(); err != nil {
				return fmt.Errorf("close csv file %s: %w", w.fileName, err)
			}
		}
		return nil
	}

	for {
		select {
		case <-s.stopCh:
			// 收到 Stop 信号：drain 剩余 payload 后正常退出
			draining := true
			for draining {
				select {
				case p := <-s.queue:
					if err := s.writePayload(&buf, p); err != nil {
						failStop(err)
						return
					}
				default:
					draining = false
				}
			}
			if err := flushAndSync(true); err != nil {
				failStop(err)
				return
			}
			if err := closeAllFiles(); err != nil {
				failStop(err)
				return
			}
			return
		case p := <-s.queue:
			if err := s.writePayload(&buf, p); err != nil {
				failStop(err)
				return
			}
			if s.shouldAutoStop() {
				// 自停止：flush + close 所有文件 + 通知上层
				if err := flushAndSync(true); err != nil {
					failStop(err)
					return
				}
				if err := closeAllFiles(); err != nil {
					failStop(err)
					return
				}
				s.started.Store(false)
				s.signalAutoDone()
				// 自停止后 writerLoop 不再写，writers map 与计数稳定，可直接读字段
				slog.Info("CSVRecordingSink 因停止条件自停止",
					"component", "CSVRecordingSink",
					"fileCount", s.fileCount,
					"recordCount", s.recordCount,
				)
				return
			}
			// 按设备评估滚动：只滚动该设备的文件。
			// 整段持写锁，避免与 Status() 并发读写 w.fileName/w.fileSize/s.fileCount。
			// rotation 不频繁（达阈值才触发），持锁做 flush/close/open 可接受。
			s.statsMu.Lock()
			w := s.writers[p.DeviceID]
			if w != nil && s.shouldRotateLocked(w) {
				if err := w.bw.Flush(); err != nil {
					s.statsMu.Unlock()
					failStop(fmt.Errorf("flush csv file %s: %w", w.fileName, err))
					return
				}
				if err := w.file.Close(); err != nil {
					s.statsMu.Unlock()
					failStop(fmt.Errorf("close csv file %s: %w", w.fileName, err))
					return
				}
				if err := s.openNewFileForLocked(w); err != nil {
					s.statsMu.Unlock()
					failStop(err)
					return
				}
			}
			s.statsMu.Unlock()
		case <-flushTicker.C:
			if err := flushAndSync(false); err != nil {
				failStop(err)
				return
			}
		case <-syncTicker.C:
			if err := flushAndSync(true); err != nil {
				failStop(err)
				return
			}
		}
	}
}

// shouldRotateLocked 评估是否应滚动到新文件（按设备独立评估）。
// 调用方必须持有 statsMu 锁（RLock 或 Lock），否则与 Status() 并发读 w.fileSize 有 race。
// 使用该设备的 fileStartedAt，确保滚动后单文件时长重新计时。
func (s *CSVRecordingSink) shouldRotateLocked(w *perDeviceWriter) bool {
	if !s.config.FileRotation.Enabled {
		return false
	}
	fr := s.config.FileRotation
	if fr.MaxFileSizeBytes > 0 && w.fileSize >= fr.MaxFileSizeBytes {
		return true
	}
	if fr.MaxDurationMs > 0 && time.Since(w.fileStartedAt).Milliseconds() >= fr.MaxDurationMs {
		return true
	}
	return false
}

// getOrCreateWriter 按 deviceId 找/建 perDeviceWriter。
// 仅在 writerLoop 单线程调用，但 Status() 可能并发读 writers map，故持锁。
func (s *CSVRecordingSink) getOrCreateWriter(payload device.DataPayload) (*perDeviceWriter, error) {
	s.statsMu.Lock()
	defer s.statsMu.Unlock()
	if w, ok := s.writers[payload.DeviceID]; ok {
		return w, nil
	}
	w := &perDeviceWriter{
		deviceID:     payload.DeviceID,
		deviceType:   payload.DeviceType,
		isWideFormat: payload.DeviceType == device.DeviceDAQP1604,
	}
	if err := s.openNewFileForLocked(w); err != nil {
		return nil, err
	}
	s.writers[payload.DeviceID] = w
	slog.Info("CSVRecordingSink 为设备创建文件",
		"component", "CSVRecordingSink",
		"deviceId", payload.DeviceID,
		"deviceType", payload.DeviceType,
		"wideFormat", w.isWideFormat,
		"file", w.fileName,
	)
	return w, nil
}

// signalAutoDone 关闭 autoDone channel，幂等。
func (s *CSVRecordingSink) signalAutoDone() {
	s.autoDoneOnce.Do(func() {
		close(s.autoDone)
	})
}

// shouldAutoStop 评估是否满足停止条件（跨设备汇总）。
// 在 writerLoop 内单线程调用，但读 writers map 时持锁以与 Status() 互斥。
func (s *CSVRecordingSink) shouldAutoStop() bool {
	s.statsMu.RLock()
	defer s.statsMu.RUnlock()
	sc := s.config.StopConditions
	if sc.MaxDurationMs > 0 && time.Since(s.startedAt).Milliseconds() >= sc.MaxDurationMs {
		return true
	}
	// MaxFileSizeBytes：未启用滚动时，按设备单文件评估，任一设备触发则停止
	if sc.MaxFileSizeBytes > 0 && !s.config.FileRotation.Enabled {
		for _, w := range s.writers {
			if w.fileSize >= sc.MaxFileSizeBytes {
				return true
			}
		}
	}
	if sc.MaxRecordCount > 0 && s.recordCount >= sc.MaxRecordCount {
		return true
	}
	return false
}

// writePayload 把单个 payload 格式化写入对应设备的文件。
// 按 deviceType 分派宽格式（DAQ-P-1604）或长格式（其他）。
func (s *CSVRecordingSink) writePayload(buf *[]byte, payload device.DataPayload) error {
	w, err := s.getOrCreateWriter(payload)
	if err != nil {
		return err
	}
	if w.isWideFormat {
		return s.writePayloadWide(buf, w, payload)
	}
	return s.writePayloadLong(buf, w, payload)
}

// writePayloadWide 写入 DAQ-P-1604 宽格式（18 通道列，对齐 daq-p1604 项目）。
// 行格式：'YYYY-MM-DD HH:MM:SS.mmm,v1,v2,...,v18\n
// 时间来源：DeviceTimestamp > 0 用硬件时间戳，否则系统时间戳。
// 前缀单引号强制 Excel 按文本显示，带毫秒保证秒和毫秒均完整可见（与 daq-p1604 CSV recorder 一致）。
// 时区说明：使用 time.UnixMilli 默认本地时区，与 daq-p1604 项目对齐。
// 跨时区部署时 CSV 时间戳为部署机本地时间，非 UTC。
func (s *CSVRecordingSink) writePayloadWide(buf *[]byte, w *perDeviceWriter, payload device.DataPayload) error {
	// 通道数校验：宽格式表头固定 18 列，通道数不符则跳帧避免列错位
	if len(payload.Channels) != daqP1604WideFormatChannels {
		slog.Warn("DAQ-P-1604 通道数与宽格式不符，跳过此帧避免 CSV 列错位",
			"component", "CSVRecordingSink",
			"deviceId", payload.DeviceID,
			"expected", daqP1604WideFormatChannels,
			"actual", len(payload.Channels),
		)
		return nil
	}

	// 时间戳：硬件优先，否则系统
	var t time.Time
	if payload.DeviceTimestamp > 0 {
		t = time.UnixMilli(payload.DeviceTimestamp)
	} else {
		t = time.UnixMilli(payload.Timestamp)
	}

	b := (*buf)[:0]
	// 前缀单引号强制 Excel 按文本显示，带毫秒保证秒和毫秒均完整可见（对齐 daq-p1604）
	b = append(b, '\'')
	b = t.AppendFormat(b, "2006-01-02 15:04:05.000")
	b = append(b, ',')

	// 写入所有通道值（DAQ-P-1604 固定 18 通道）
	for _, v := range payload.Channels {
		b = strconv.AppendFloat(b, v, 'f', csvFloatPrecision, 64)
		b = append(b, ',')
	}
	// 去掉末尾逗号，写入换行
	if len(b) > 0 {
		b[len(b)-1] = '\n'
	} else {
		b = append(b, '\n')
	}

	if _, err := w.bw.Write(b); err != nil {
		return err
	}
	written := int64(len(b))
	*buf = b

	// 更新统计（writerLoop 单线程，但仍持锁以与 Status() 互斥）
	s.statsMu.Lock()
	w.fileSize += written
	w.fileRecordCount++
	w.totalRecords++
	s.recordCount++
	s.statsMu.Unlock()
	return nil
}

// writePayloadLong 写入长格式（每通道一行：timestamp,deviceId,channelIndex,value）。
// 保留 wind-daq 历史行为，适用于非 DAQ-P-1604 设备。
// 时间来源：DeviceTimestamp > 0 用硬件时间戳，否则系统时间戳。
// 使用 strconv.AppendXxx 替代 fmt.Fprintf，避免反射开销。
func (s *CSVRecordingSink) writePayloadLong(buf *[]byte, w *perDeviceWriter, payload device.DataPayload) error {
	ts := payload.Timestamp
	if payload.DeviceTimestamp > 0 {
		ts = payload.DeviceTimestamp
	}
	written := int64(0)
	for i, value := range payload.Channels {
		channelIndex := i
		if i < len(payload.ChannelIndices) {
			channelIndex = payload.ChannelIndices[i]
		}

		// 复用 buf，避免每次分配
		b := (*buf)[:0]
		b = strconv.AppendInt(b, ts, 10)
		b = append(b, ',')
		b = append(b, payload.DeviceID...)
		b = append(b, ',')
		b = strconv.AppendInt(b, int64(channelIndex), 10)
		b = append(b, ',')
		b = strconv.AppendFloat(b, value, 'f', csvFloatPrecision, 64)
		b = append(b, '\n')

		if _, err := w.bw.Write(b); err != nil {
			return err
		}
		written += int64(len(b))
		*buf = b
	}
	// 更新统计（writerLoop 单线程，但仍持锁以与 Status() 互斥）
	s.statsMu.Lock()
	w.fileSize += written
	w.fileRecordCount += int64(len(payload.Channels))
	w.totalRecords += int64(len(payload.Channels))
	s.recordCount += int64(len(payload.Channels))
	s.statsMu.Unlock()
	return nil
}

// openNewFileForLocked 为指定设备创建新文件并写入表头（调用方持 statsMu 锁）。
// 文件名格式：<prefix>-<deviceId>-YYYYMMDD-HHMMSS-NNN.csv，NNN 从该设备当前 fileCount+1 开始递增。
// 若目标文件已存在（极少见，需同一秒内重启且序号也撞上），序号 +1 重试。
func (s *CSVRecordingSink) openNewFileForLocked(w *perDeviceWriter) error {
	config := s.config
	w.fileCount++
	fileCount := w.fileCount

	base := fmt.Sprintf("%s-%s-%s", config.FilePrefix, w.deviceID, time.Now().Format("20060102-150405"))
	var name string
	var file *os.File
	var err error
	for seq := fileCount; seq < fileCount+1000; seq++ {
		name = fmt.Sprintf("%s-%03d.csv", base, seq)
		full := filepath.Join(config.OutputDir, name)
		// O_CREATE|O_EXCL 保证不覆盖已存在文件
		file, err = os.OpenFile(full, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
		if err == nil {
			break
		}
		if os.IsExist(err) {
			continue
		}
		return err
	}
	if err != nil {
		return err
	}

	// 表头按格式分派
	var header string
	if w.isWideFormat {
		// 18 列宽格式：Timestamp + CH01..CH16 + CH17_AtmPressure + CH18_AtmTemp（对齐 daq-p1604）
		header = "Timestamp,CH01,CH02,CH03,CH04,CH05,CH06,CH07,CH08,CH09,CH10,CH11,CH12,CH13,CH14,CH15,CH16,CH17_AtmPressure,CH18_AtmTemp\n"
	} else {
		// 长格式：每通道一行
		header = "timestamp,deviceId,channelIndex,value\n"
	}
	if _, err := file.WriteString(header); err != nil {
		_ = file.Close()
		return err
	}

	w.file = file
	w.bw = bufio.NewWriterSize(file, s.cfg.BufferSize)
	w.fileName = name
	w.fileSize = 0
	w.fileRecordCount = 0
	w.fileStartedAt = time.Now()

	// 跨设备汇总文件数
	s.fileCount++
	return nil
}
