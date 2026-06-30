// binary_sink.go 二进制存储适配器实现。
//
// BinaryRecordingSink 用于 1kHz × 10 设备以上场景下的高吞吐存储：
//   - 文件头：固定 16 字节，包含 magic + version + 保留字段
//   - 每帧：定长头（时间戳、设备时间戳、deviceId 长度、通道数）+ 变长 payload（deviceId + 通道索引 + 通道值）
//   - 通道值以 float32 LE 编码，相比 CSV 文本节省 ~50% 空间，且无格式化开销
//   - 异步 writer goroutine + bufio + 定时 Sync，与 CSVRecordingSink 同设计
//
// 文件格式：
//
//	[offset 0..3]   magic = "WDQ1"
//	[offset 4..5]   version (uint16 LE) = 1
//	[offset 6..7]   reserved (uint16 LE) = 0
//	[offset 8..15]  reserved (8 bytes zero)
//	后续每帧：
//	  [0..7]   timestamp (int64 LE)
//	  [8..15]  deviceTimestamp (int64 LE)
//	  [16..17] deviceIdLen (uint16 LE)
//	  [18..19] channelCount (uint16 LE)
//	  [20..N]  deviceId (UTF-8 字节)
//	  [N..M]   channelIndices (int32 LE × channelCount)
//	  [M..]    channelValues (float32 LE × channelCount)
package storage

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"log/slog"
	"math"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"wind-daq/services/api-go/internal/core/device"
	corestorage "wind-daq/services/api-go/internal/core/storage"
)

// 二进制文件格式常量
const (
	binaryMagic       = "WDQ1"
	binaryVersion     = uint16(1)
	binaryHeaderSize  = 16 // 文件头固定 16 字节
	binaryFrameHeader = 20 // 每帧定长头 20 字节
)

// BinarySinkConfig 二进制存储配置，字段含义同 CSVSinkConfig
type BinarySinkConfig = CSVSinkConfig

// DefaultBinarySinkConfig 返回适合 1kHz × 10 设备场景的默认配置
func DefaultBinarySinkConfig() BinarySinkConfig {
	return DefaultCSVSinkConfig()
}

// BinaryRecordingSink 二进制异步批量写存储适配器
type BinaryRecordingSink struct {
	cfg     BinarySinkConfig
	started atomic.Bool

	queue  chan device.DataPayload
	stopCh chan struct{}
	doneCh chan struct{}

	dropped atomic.Int64 // 队列满时丢弃计数（监控用）

	// drop 节流日志状态：与 CSVRecordingSink 对齐，
	// lastDropLogAt 用 atomic 存储 UnixNano 时间戳，
	// 避免每次丢弃都加锁；仅在节流间隔到达时才进入慢路径更新。
	droppedSinceLog atomic.Int64
	lastDropLogAt   atomic.Int64

	syncErrMu sync.RWMutex
	syncErr   error
}

// NewBinaryRecordingSink 创建使用默认配置的二进制 sink
func NewBinaryRecordingSink() *BinaryRecordingSink {
	return NewBinaryRecordingSinkWithConfig(DefaultBinarySinkConfig())
}

// NewBinaryRecordingSinkWithConfig 创建使用自定义配置的二进制 sink
func NewBinaryRecordingSinkWithConfig(cfg BinarySinkConfig) *BinaryRecordingSink {
	applyCSVSinkDefaults(&cfg)
	return &BinaryRecordingSink{cfg: cfg}
}

// Start 创建二进制文件并写入文件头，启动 writer goroutine
func (s *BinaryRecordingSink) Start(config corestorage.RecordingConfig) error {
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

	name := fmt.Sprintf("%s-%s.bin", config.FilePrefix, time.Now().Format("20060102-150405"))
	file, err := os.Create(filepath.Join(config.OutputDir, name))
	if err != nil {
		s.started.Store(false)
		return err
	}

	// 写入文件头：magic + version + reserved (10 bytes zero)
	header := make([]byte, binaryHeaderSize)
	copy(header[0:4], binaryMagic)
	binary.LittleEndian.PutUint16(header[4:6], binaryVersion)
	// header[6:16] 保留字段，全 0
	if _, err := file.Write(header); err != nil {
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

	slog.Info("BinaryRecordingSink Start 成功",
		"component", "BinaryRecordingSink",
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

// Write 非阻塞投递 payload 到队列，队列满时丢弃并计数。
// 丢弃日志使用时间节流（dropLogInterval=5s）+ atomic CAS 双检，
// 与 CSVRecordingSink 对齐：避免高丢弃速率下按 1000 次/条刷屏。
func (s *BinaryRecordingSink) Write(payload device.DataPayload) error {
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
		slog.Warn("BinaryRecordingSink 队列已满，丢弃 payload（节流聚合）",
			"component", "BinaryRecordingSink",
			"deviceId", payload.DeviceID,
			"totalDropped", totalDropped,
			"droppedSinceLastLog", sinceLog,
			"queueCapacity", s.cfg.QueueCapacity,
		)
	}
	return nil
}

// Stop 通知 writer goroutine 退出，drain 剩余 payload 后关闭文件
func (s *BinaryRecordingSink) Stop() error {
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
func (s *BinaryRecordingSink) DroppedCount() int64 {
	return s.dropped.Load()
}

// writerLoop 与 CSVRecordingSink.writerLoop 结构相同，仅 writePayload 实现不同
func (s *BinaryRecordingSink) writerLoop(file *os.File, bw *bufio.Writer) {
	defer close(s.doneCh)

	flushTicker := time.NewTicker(s.cfg.FlushInterval)
	defer flushTicker.Stop()
	syncTicker := time.NewTicker(s.cfg.SyncInterval)
	defer syncTicker.Stop()

	// 复用 byte buffer 用于编码单帧
	var frame []byte

	flushAndSync := func(sync bool) error {
		if err := bw.Flush(); err != nil {
			return fmt.Errorf("flush binary file: %w", err)
		}
		if sync {
			if err := file.Sync(); err != nil {
				return fmt.Errorf("sync binary file: %w", err)
			}
		}
		return nil
	}

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
			draining := true
			for draining {
				select {
				case p := <-s.queue:
					if err := s.writePayload(bw, p, &frame); err != nil {
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
			if err := s.writePayload(bw, p, &frame); err != nil {
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

// writePayload 编码单个 payload 写入 bufio。
// 帧格式见文件顶部注释。
func (s *BinaryRecordingSink) writePayload(bw *bufio.Writer, payload device.DataPayload, frame *[]byte) error {
	ts := payload.Timestamp
	if payload.DeviceTimestamp > 0 {
		ts = payload.DeviceTimestamp
	}

	// 通道数取 Channels 切片长度；ChannelIndices 可能为空，使用 len(Channels) 兜底
	channelCount := len(payload.Channels)
	deviceIDBytes := []byte(payload.DeviceID)
	deviceIDLen := len(deviceIDBytes)
	if deviceIDLen > 0xFFFF {
		return fmt.Errorf("deviceId too long: %d bytes (max 65535)", deviceIDLen)
	}
	if channelCount > 0xFFFF {
		return fmt.Errorf("channel count too large: %d (max 65535)", channelCount)
	}

	// 计算整帧大小，复用 frame buffer
	frameSize := binaryFrameHeader + deviceIDLen + channelCount*4 + channelCount*4
	if cap(*frame) < frameSize {
		*frame = make([]byte, frameSize)
	}
	b := (*frame)[:frameSize]

	// 帧定长头
	binary.LittleEndian.PutUint64(b[0:8], uint64(ts))
	binary.LittleEndian.PutUint64(b[8:16], uint64(payload.DeviceTimestamp))
	binary.LittleEndian.PutUint16(b[16:18], uint16(deviceIDLen))
	binary.LittleEndian.PutUint16(b[18:20], uint16(channelCount))

	// deviceId
	offset := binaryFrameHeader
	copy(b[offset:offset+deviceIDLen], deviceIDBytes)
	offset += deviceIDLen

	// channelIndices (int32 LE)
	for i := 0; i < channelCount; i++ {
		idx := int32(i)
		if i < len(payload.ChannelIndices) {
			idx = int32(payload.ChannelIndices[i])
		}
		binary.LittleEndian.PutUint32(b[offset:offset+4], uint32(idx))
		offset += 4
	}

	// channelValues (float32 LE)
	for i := 0; i < channelCount; i++ {
		binary.LittleEndian.PutUint32(b[offset:offset+4], math.Float32bits(float32(payload.Channels[i])))
		offset += 4
	}

	if _, err := bw.Write(b); err != nil {
		return err
	}
	*frame = b
	return nil
}
