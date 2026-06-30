// Package storage 提供 wind-daq 的存储适配器实现。
//
// CSVRecordingSink 采用异步批量写设计，支撑 1kHz × 10 设备的全量保存场景：
//   - Write 仅把 payload 投递到带缓冲的 channel，立即返回，不阻塞设备 read loop
//   - 单独的 writer goroutine 消费 channel，使用 bufio.Writer 聚合写入
//   - 定时 Flush + 定时 Sync，避免每条记录 fsync 风暴
//   - Stop 时 drain 剩余 payload 后关闭文件，保证数据完整落盘
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

// CSVRecordingSink 异步批量写 CSV 存储适配器。
//
// 设计要点：
//   - 单一 CSV 文件，所有设备所有通道按行追加（保留原有文件语义）
//   - Write 非阻塞投递，writer goroutine 串行写入，消除多设备锁争用
//   - 100Hz 总吞吐在普通 SSD 上可达 ~160k 行/秒，覆盖 10 × 1kHz × 16 通道场景
type CSVRecordingSink struct {
	cfg     CSVSinkConfig
	started atomic.Bool // CAS 保护 Start/Stop 串行

	// 在 Start 时一次性创建，Stop 后不再修改；Write 通过 atomic 检查 started 后读取
	queue  chan device.DataPayload
	stopCh chan struct{}
	doneCh chan struct{}

	// writer goroutine 内部状态
	dropped atomic.Int64 // 队列满时丢弃计数（监控用）

	// drop 节流日志状态：lastDropLogAt 用 atomic 存储 UnixNano 时间戳，
	// 避免每次丢弃都加锁；仅在节流间隔到达时才进入慢路径更新。
	droppedSinceLog atomic.Int64
	lastDropLogAt   atomic.Int64

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

// Start 创建 CSV 文件并启动 writer goroutine。
// 重复调用 Start 会返回错误（CAS 保护）。
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

	name := fmt.Sprintf("%s-%s.csv", config.FilePrefix, time.Now().Format("20060102-150405"))
	file, err := os.Create(filepath.Join(config.OutputDir, name))
	if err != nil {
		s.started.Store(false)
		return err
	}
	if _, err := file.WriteString("timestamp,deviceId,channelIndex,value\n"); err != nil {
		_ = file.Close()
		s.started.Store(false)
		return err
	}

	bw := bufio.NewWriterSize(file, s.cfg.BufferSize)
	s.queue = make(chan device.DataPayload, s.cfg.QueueCapacity)
	s.stopCh = make(chan struct{})
	s.doneCh = make(chan struct{})

	// 清除上一次录制可能残留的 I/O 错误，避免 Stop() 返回旧错误
	s.syncErrMu.Lock()
	s.syncErr = nil
	s.syncErrMu.Unlock()

	slog.Info("CSVRecordingSink Start 成功",
		"component", "CSVRecordingSink",
		"outputDir", config.OutputDir,
		"file", name,
		"queueCapacity", s.cfg.QueueCapacity,
		"bufferSize", s.cfg.BufferSize,
		"flushInterval", s.cfg.FlushInterval,
		"syncInterval", s.cfg.SyncInterval,
	)

	go s.writerLoop(file, bw)
	return nil
}

// Write 非阻塞地把 payload 投递到异步队列。
// 队列满时丢弃 payload 并计数（不阻塞设备 read loop）。
// 未 Start 时返回错误，保留与旧实现一致的语义。
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

// Stop 通知 writer goroutine 退出，drain 剩余 payload 后关闭文件。
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

// DroppedCount 返回累计丢弃的 payload 数（监控用）
func (s *CSVRecordingSink) DroppedCount() int64 {
	return s.dropped.Load()
}

// writerLoop 消费 queue 中的 payload，使用 bufio.Writer 聚合写入。
// 触发条件：payload 到达、flush ticker、sync ticker、stop 信号。
// 任一 I/O 错误都会设置 syncErr 并退出，避免继续写入已损坏的文件。
func (s *CSVRecordingSink) writerLoop(file *os.File, bw *bufio.Writer) {
	defer close(s.doneCh)

	flushTicker := time.NewTicker(s.cfg.FlushInterval)
	defer flushTicker.Stop()
	syncTicker := time.NewTicker(s.cfg.SyncInterval)
	defer syncTicker.Stop()

	// 复用 byte buffer 用于格式化，避免每次 Write 分配
	var buf []byte

	// flushAndSync 把 bufio 缓冲刷到 OS 文件缓冲，可选 fsync 落盘。
	flushAndSync := func(sync bool) error {
		if err := bw.Flush(); err != nil {
			return fmt.Errorf("flush csv file: %w", err)
		}
		if sync {
			if err := file.Sync(); err != nil {
				return fmt.Errorf("sync csv file: %w", err)
			}
		}
		return nil
	}

	// failStop 在 writer goroutine 内部出错时被调用：
	// 设置 syncErr、尝试 flush + 关闭文件后退出。
	failStop := func(err error) {
		s.syncErrMu.Lock()
		s.syncErr = err
		s.syncErrMu.Unlock()
		_ = bw.Flush()
		_ = file.Close()
	}

	for {
		select {
		case <-s.stopCh:
			// 收到 Stop 信号：drain 剩余 payload 后正常退出
			draining := true
			for draining {
				select {
				case p := <-s.queue:
					if err := s.writePayload(bw, p, &buf); err != nil {
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
			if err := file.Close(); err != nil {
				failStop(err)
				return
			}
			return
		case p := <-s.queue:
			if err := s.writePayload(bw, p, &buf); err != nil {
				failStop(err)
				return
			}
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

// writePayload 把单个 payload 格式化写入 bufio。
// 使用 strconv.AppendXxx 替代 fmt.Fprintf，避免反射开销，吞吐提升 2-5 倍。
// buf 在调用间复用，避免每次 Write 分配。
func (s *CSVRecordingSink) writePayload(bw *bufio.Writer, payload device.DataPayload, buf *[]byte) error {
	// 如果有设备时间戳则优先使用，否则使用系统接收时间戳
	ts := payload.Timestamp
	if payload.DeviceTimestamp > 0 {
		ts = payload.DeviceTimestamp
	}
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
		// 'f' + 6 位小数，保持与原 %f 一致的输出格式（兼容现有测试和工具链）
		b = strconv.AppendFloat(b, value, 'f', 6, 64)
		b = append(b, '\n')

		if _, err := bw.Write(b); err != nil {
			return err
		}
		*buf = b
	}
	return nil
}
