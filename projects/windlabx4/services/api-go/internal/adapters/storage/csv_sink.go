// Package storage 提供 WindLabX4 的存储适配器实现。
//
// CSVRecordingSink 采用异步批量写设计，支撑 1kHz × 10 设备的全量保存场景：
//   - 每设备一个 CSV 文件，按 deviceId 路由（避免多设备数据混杂在同一文件）
//   - 所有设备统一使用"宽格式"（每 payload 一行 = 一个时间戳 + N 个通道值）：
//     * DAQ-P-1604：固定 18 列（CH01..CH16 + CH17_AtmPressure + CH18_AtmTemp），对齐 daq-p1604 项目
//     * 其他设备：以首帧的通道 index 顺序动态生成表头 CH01..CHnn（列顺序在会话内固定）
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

	"windlabx4/services/api-go/internal/core/device"
	corestorage "windlabx4/services/api-go/internal/core/storage"
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
	deviceID   string
	deviceName string // 用于生成人类可读的文件名（比 UUID 友好）
	// fileSlug 是文件名中用于标识设备的段（sanitize 后的 deviceName，若与其他设备
	// 冲突则追加 -<deviceID 前 6 位>）。在 getOrCreateWriter 首次为该设备创建
	// 文件时确定，之后本会话内所有滚动文件复用同一 slug。
	fileSlug     string
	deviceType   device.Type
	isWideFormat bool // DAQ-P-1604 固定 18 列宽格式；其他设备用动态宽格式
	// channelConfigs 由 RecordingConfig.DeviceChannels 在首次创建 writer 时注入的通道元数据。
	// 仅用于生成带单位后缀的 CSV 表头（如 CH01_Pa, CH02_degC），不影响数据行写入路径。
	// 当前仅 DAQ-P-1603 设备会注入；为空时表头退化为通用 CH01..CHnn。
	channelConfigs []device.ChannelConfig
	// columnIndices 记录本设备"会话内"的列布局（通道 index 顺序）。
	// - DAQ-P-1604：在首次 openNewFileForLocked 时设为 [0..17]
	// - 其他设备：首帧到达时按 payload.ChannelIndices 冻结；之后**滚动文件也复用同一布局**，
	//   保证跨文件列一致，用户可以直接拼接分析
	columnIndices []int
	// channelPos 是 channelIndex -> columnIndices 位置的稠密反查表，
	// 让 writePayloadDynamicWide 每帧 O(1) 定位每列对应的 payload 通道下标。
	// 长度为 max(columnIndices)+1；未在 columnIndices 中的位置存 -1。
	// 与 columnIndices 一同在首帧冻结，滚动文件不重建。
	channelPos      []int
	headerWritten   bool // 表头是否已写入本文件（每次滚动到新文件都要重置为 false）
	file            *os.File
	bw              *bufio.Writer
	fileName        string
	fileSize        int64 // 当前文件累计字节
	fileCount       int64 // 该设备本会话文件数（含当前文件）
	fileStartedAt   time.Time
	fileRecordCount int64 // 该设备当前文件记录数（用于 MaxRecordCount 滚动判断）
	totalRecords    int64 // 该设备累计记录数
	// warnedIndicesMismatch 首次遇到与 columnIndices 不一致的帧时告警，避免刷屏
	warnedIndicesMismatch bool
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
		deviceName:   payload.DeviceName,
		deviceType:   payload.DeviceType,
		isWideFormat: payload.DeviceType == device.DeviceDAQP1604,
	}
	// 注入通道元数据（仅用于表头生成）。
	// RecordingConfig.DeviceChannels 由 server.go /api/storage/start 入口从 DeviceManager.GetProfiles()
	// 收集后填充；录制中后连接的设备若未在此映射中，channelConfigs 为空，表头退化为通用 CH01..CHnn。
	if cfgs, ok := s.config.DeviceChannels[payload.DeviceID]; ok && len(cfgs) > 0 {
		w.channelConfigs = cfgs
	}
	// 计算文件名 slug：sanitize(deviceName)，若与已有 writer 的 slug 冲突则
	// 追加 -<deviceID 前 6 位> 兜底（同名设备时保证文件名唯一但仍可读）。
	w.fileSlug = s.uniqueFileSlugLocked(payload.DeviceName, payload.DeviceID)
	if err := s.openNewFileForLocked(w); err != nil {
		return nil, err
	}
	s.writers[payload.DeviceID] = w
	slog.Info("CSVRecordingSink 为设备创建文件",
		"component", "CSVRecordingSink",
		"deviceId", payload.DeviceID,
		"deviceName", payload.DeviceName,
		"deviceType", payload.DeviceType,
		"wideFormat", w.isWideFormat,
		"file", w.fileName,
	)
	return w, nil
}

// uniqueFileSlugLocked 生成用于文件名的设备段：
//   - 空 deviceName 或 sanitize 后为空时回退到 "device"
//   - 若与其他设备已用 slug 冲突，追加 -<deviceID 前 6 位> 保证唯一
//
// 调用方必须持 statsMu 锁（读 writers map）。
func (s *CSVRecordingSink) uniqueFileSlugLocked(deviceName, deviceID string) string {
	slug := sanitizeFileSegment(deviceName)
	if slug == "" {
		slug = "device"
	}
	// 遍历现有 writer 检查是否有 slug 冲突（同名设备）
	conflict := false
	for _, other := range s.writers {
		if other.fileSlug == slug {
			conflict = true
			break
		}
	}
	if !conflict {
		return slug
	}
	// 冲突时追加 deviceID 前 6 位（UUID 前 6 位重复概率极低），
	// 若 deviceID 也短则整体使用
	suffix := deviceID
	if len(suffix) > 6 {
		suffix = suffix[:6]
	}
	if suffix == "" {
		return slug
	}
	return slug + "-" + suffix
}

// sanitizeFileSegment 把设备名规范化为可用作文件名的段：
//   - 替换 Windows/POSIX 非法字符 \/:*?"<>| 及控制字符为 '_'
//   - 折叠首尾空白和多余的 '_'/'-'
//   - 限制长度到 40，避免拼上时间戳后超出 Windows MAX_PATH
//
// 保留中文、数字、字母、连字符、下划线、点。
func sanitizeFileSegment(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return ""
	}
	var b strings.Builder
	b.Grow(len(name))
	for _, r := range name {
		switch {
		case r < 0x20 || r == 0x7f:
			b.WriteByte('_')
		case r == '\\' || r == '/' || r == ':' || r == '*' || r == '?' ||
			r == '"' || r == '<' || r == '>' || r == '|':
			b.WriteByte('_')
		case r == ' ' || r == '\t':
			b.WriteByte('_')
		default:
			b.WriteRune(r)
		}
	}
	out := b.String()
	// 折叠连续的下划线/连字符
	for strings.Contains(out, "__") {
		out = strings.ReplaceAll(out, "__", "_")
	}
	out = strings.Trim(out, "_-.")
	if len(out) > 40 {
		// 按 rune 截断，避免砍出半个中文字符
		runes := []rune(out)
		if len(runes) > 40 {
			runes = runes[:40]
		}
		out = string(runes)
		out = strings.Trim(out, "_-.")
	}
	return out
}

// signalAutoDone 关闭 autoDone channel，幂等。
func (s *CSVRecordingSink) signalAutoDone() {
	s.autoDoneOnce.Do(func() {
		close(s.autoDone)
	})
}

// buildChannelPos 构造 channelIndex -> columnIndices 位置 的稠密反查表。
// 长度 = max(columnIndices)+1，未在 columnIndices 中的位置存 -1。
// 用于 writePayloadDynamicWide 每帧 O(1) 定位 payload 中每列对应的通道下标。
func buildChannelPos(columnIndices []int) []int {
	maxIdx := -1
	for _, ci := range columnIndices {
		if ci > maxIdx {
			maxIdx = ci
		}
	}
	if maxIdx < 0 {
		return nil
	}
	pos := make([]int, maxIdx+1)
	for i := range pos {
		pos[i] = -1
	}
	for slot, ci := range columnIndices {
		pos[ci] = slot
	}
	return pos
}

// buildDynamicHeader 生成宽格式 CSV 表头。
//   - isWideFormat=true（DAQ-P-1604）：使用 CH01..CH16 + CH17_AtmPressure + CH18_AtmTemp（对齐 daq-p1604 项目）
//   - DAQ-P-1603 且 channelConfigs 非空：按 columnIndices 顺序查 channelConfigs 的 SensorType 生成带单位后缀表头
//     （pressure→CHxx_Pa，temperature→CHxx_degC），避免直接用 Unit 字段（含 ℃ 等 ASCII 字符导致 Excel 编码问题）
//   - 其他设备：Timestamp,CH01,CH02,...,CHnn（编号根据 columnIndices+1）
func buildDynamicHeader(columnIndices []int, isWideFormat bool, channelConfigs []device.ChannelConfig) string {
	if isWideFormat && len(columnIndices) == daqP1604WideFormatChannels {
		return "Timestamp,CH01,CH02,CH03,CH04,CH05,CH06,CH07,CH08,CH09,CH10,CH11,CH12,CH13,CH14,CH15,CH16,CH17_AtmPressure,CH18_AtmTemp\n"
	}
	var b strings.Builder
	b.Grow(16 + len(columnIndices)*10)
	b.WriteString("Timestamp")
	for _, idx := range columnIndices {
		b.WriteByte(',')
		fmt.Fprintf(&b, "CH%02d", idx+1)
		// 仅在注入了通道元数据时追加单位后缀。
		// 避免每次循环都线性扫描 channelConfigs：实际通道数 ≤16，开销可忽略；
		// 若未来扩展到更多通道可改为预先构建 idx→ChannelConfig 映射。
		if cfg := findChannelConfig(channelConfigs, idx); cfg != nil {
			b.WriteString(channelSuffixBySensorType(cfg.SensorType))
		}
	}
	b.WriteByte('\n')
	return b.String()
}

// findChannelConfig 在 channelConfigs 中按 Index 查找对应通道配置。
// 通道数通常 ≤16，线性扫描开销可忽略；返回 nil 表示未找到（表头退化为无后缀）。
func findChannelConfig(channelConfigs []device.ChannelConfig, idx int) *device.ChannelConfig {
	for i := range channelConfigs {
		if channelConfigs[i].Index == idx {
			return &channelConfigs[i]
		}
	}
	return nil
}

// channelSuffixBySensorType 按 SensorType 返回规范化的表头后缀。
// 不直接用 ChannelConfig.Unit 字段（可能含 ℃ 等 ASCII 字符），
// 统一英文标识保证 CSV 表头在 Excel 默认 GBK 解析下不乱码。
// 未知 SensorType（不应出现，反序列化已兜底为 pressure）返回空串。
func channelSuffixBySensorType(t device.ChannelSensorType) string {
	switch t {
	case device.SensorTemperature:
		return "_degC"
	case device.SensorPressure:
		return "_Pa"
	default:
		return ""
	}
}

// appendCSVTimestamp 追加 CSV 时间戳段（前缀单引号 + 秒级精度）。
// 时间来源：DeviceTimestamp > 0 用硬件时间戳，否则系统时间戳。
// 截断到秒级：DAQ-P-1604 等设备时间戳存在固件 bug（fractional 字段递增不正确），
// 且系统毫秒时间戳在 1000Hz 下精度不足。统一秒级避免展示错误的时间细分。
// 前缀单引号强制 Excel 按文本显示，避免被默认 "yyyy/m/d h:mm" 格式隐藏秒。
func appendCSVTimestamp(b []byte, payload device.DataPayload) []byte {
	var t time.Time
	if payload.DeviceTimestamp > 0 {
		t = time.UnixMilli(payload.DeviceTimestamp)
	} else {
		t = time.UnixMilli(payload.Timestamp)
	}
	b = append(b, '\'')
	b = t.AppendFormat(b, "2006-01-02 15:04:05")
	return b
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

// writePayload 把单个 payload 格式化写入对应设备的文件（统一宽格式）。
// - DAQ-P-1604：走固定 18 列的严格校验路径
// - 其他设备：走动态宽格式，列布局由首帧确定
func (s *CSVRecordingSink) writePayload(buf *[]byte, payload device.DataPayload) error {
	w, err := s.getOrCreateWriter(payload)
	if err != nil {
		return err
	}
	if w.isWideFormat {
		return s.writePayloadWide(buf, w, payload)
	}
	return s.writePayloadDynamicWide(buf, w, payload)
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
	b := (*buf)[:0]
	b = appendCSVTimestamp(b, payload)
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

// writePayloadDynamicWide 写入非 DAQ-P-1604 设备的动态宽格式。
//
// 列布局策略：
//   - 首帧到达时按 payload.ChannelIndices（若为空则用位置索引）**冻结**列布局，
//     构造反查数组 channelPos 供后续 O(1) 查找。
//   - 后续帧和滚动到的新文件全部沿用同一布局，保证跨文件列一致。
//   - 若后续帧的 ChannelIndices 集合与首帧不一致（用户改配置但未重启录制等情形），
//     首次不一致时写 warn 日志，但仍按已冻结的布局输出：命中列写值，未命中列空。
//
// 行格式：'YYYY-MM-DD HH:MM:SS.mmm,v1,v2,...,vN\n
// 时间来源：DeviceTimestamp > 0 用硬件时间戳，否则系统时间戳。
// 前缀单引号强制 Excel 按文本显示（与 DAQ-P-1604 保持一致）。
func (s *CSVRecordingSink) writePayloadDynamicWide(buf *[]byte, w *perDeviceWriter, payload device.DataPayload) error {
	// 首次为该设备写数据时冻结列布局；同一会话内滚动文件不再重建，仅 headerWritten 被重置
	if w.columnIndices == nil {
		indices := make([]int, len(payload.Channels))
		if len(payload.ChannelIndices) == len(payload.Channels) {
			copy(indices, payload.ChannelIndices)
		} else {
			for i := range indices {
				indices[i] = i
			}
		}
		s.statsMu.Lock()
		w.columnIndices = indices
		w.channelPos = buildChannelPos(indices)
		s.statsMu.Unlock()
	}

	// 当前文件的表头（可能因为滚动而尚未写入本文件）
	if !w.headerWritten {
		hb := buildDynamicHeader(w.columnIndices, w.isWideFormat, w.channelConfigs)
		if _, err := w.bw.Write([]byte(hb)); err != nil {
			return err
		}
		s.statsMu.Lock()
		w.headerWritten = true
		w.fileSize += int64(len(hb))
		s.statsMu.Unlock()
	}

	// 通道抖动告警（一次性）：payload 中出现了 columnIndices 覆盖不到的通道
	if !w.warnedIndicesMismatch && payloadHasUnknownChannel(payload, w.channelPos) {
		slog.Warn("payload 通道 index 与首帧不一致，超出列布局的通道将被丢弃",
			"component", "CSVRecordingSink",
			"deviceId", w.deviceID,
			"columnIndices", w.columnIndices,
			"payloadIndices", payload.ChannelIndices,
		)
		w.warnedIndicesMismatch = true
	}

	// 组装行：时间戳 + 每列（缺失列输出空）
	b := (*buf)[:0]
	b = appendCSVTimestamp(b, payload)

	hasIndices := len(payload.ChannelIndices) == len(payload.Channels)
 	for _, colIdx := range w.columnIndices {
		b = append(b, ',')
		payloadPos := -1
		if hasIndices {
			// 用 channelPos 反查 payload 中该 colIdx 的位置。
			// 但反查表是 "columnIndices 位置 -> payload 位置"，此处需反向：
			// 直接在 payload.ChannelIndices 上线性扫描代价 O(cols*payload_channels)；
			// 更高效的做法：借助 channelPos 已用 columnIndex 反查。这里 payload 的
			// 每条 index 都要能反查到自己在 payload 里的位置，所以还是要遍历 payload 一次
			// 建立一次性映射，或让 payload 本身按 columnIndices 顺序（大多数硬件都是这样）。
			// 由于本调用点热路径每帧 16-64 通道，建议按 payload 顺序快速命中：
			// 若 payload.ChannelIndices == w.columnIndices 顺序一致，直接同下标取值。
			if colIdxLE := colIdx; colIdxLE < len(payload.ChannelIndices) && payload.ChannelIndices[colIdxLE] == colIdx {
				payloadPos = colIdxLE
			} else {
				// 顺序不一致才做全表扫描（罕见）
				for i, pi := range payload.ChannelIndices {
					if pi == colIdx {
						payloadPos = i
						break
					}
				}
			}
		} else if colIdx < len(payload.Channels) {
			// payload 无 ChannelIndices 时按位置读
			payloadPos = colIdx
		}
		if payloadPos >= 0 && payloadPos < len(payload.Channels) {
			b = strconv.AppendFloat(b, payload.Channels[payloadPos], 'f', csvFloatPrecision, 64)
		}
		// 缺失列 → 空串，保持列对齐
	}
	b = append(b, '\n')

	if _, err := w.bw.Write(b); err != nil {
		return err
	}
	written := int64(len(b))
	*buf = b

	s.statsMu.Lock()
	w.fileSize += written
	w.fileRecordCount++
	w.totalRecords++
	s.recordCount++
	s.statsMu.Unlock()
	return nil
}

// payloadHasUnknownChannel 判断 payload 是否包含 channelPos 覆盖不到的通道 index。
// 用于一次性触发通道抖动告警。
func payloadHasUnknownChannel(payload device.DataPayload, channelPos []int) bool {
	if len(payload.ChannelIndices) != len(payload.Channels) {
		return false
	}
	for _, pi := range payload.ChannelIndices {
		if pi < 0 || pi >= len(channelPos) || channelPos[pi] < 0 {
			return true
		}
	}
	return false
}

// openNewFileForLocked 为指定设备创建新文件并写入表头（调用方持 statsMu 锁）。
// 文件名格式：<prefix>-<deviceId>-YYYYMMDD-HHMMSS-NNN.csv，NNN 从该设备当前 fileCount+1 开始递增。
// 若目标文件已存在（极少见，需同一秒内重启且序号也撞上），序号 +1 重试。
func (s *CSVRecordingSink) openNewFileForLocked(w *perDeviceWriter) error {
	config := s.config
	w.fileCount++
	fileCount := w.fileCount

	base := fmt.Sprintf("%s-%s-%s", config.FilePrefix, w.fileSlug, time.Now().Format("20060102-150405"))
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

	// 表头分派：
	// - DAQ-P-1604：本设备首次开文件时 columnIndices 为空，此处初始化 [0..17] 并写入固定表头
	// - 动态宽格式设备的滚动文件：columnIndices 已在首帧冻结，此处按同布局立即写表头
	// - 动态宽格式设备的首个文件：columnIndices 仍为空，表头延迟到首帧写入
	w.headerWritten = false
	w.fileSize = 0
	if w.isWideFormat && w.columnIndices == nil {
		indices := make([]int, daqP1604WideFormatChannels)
		for i := range indices {
			indices[i] = i
		}
		w.columnIndices = indices
		w.channelPos = buildChannelPos(indices)
	}
	if w.columnIndices != nil {
		hb := buildDynamicHeader(w.columnIndices, w.isWideFormat, w.channelConfigs)
		if _, err := file.WriteString(hb); err != nil {
			_ = file.Close()
			return err
		}
		w.headerWritten = true
		w.fileSize = int64(len(hb))
	}

	w.file = file
	w.bw = bufio.NewWriterSize(file, s.cfg.BufferSize)
	w.fileName = name
	w.fileRecordCount = 0
	w.fileStartedAt = time.Now()

	// 跨设备汇总文件数
	s.fileCount++
	return nil
}
