// sink_factory.go 存储适配器工厂。
//
// 根据存储格式（csv/binary）创建对应的 RecordingSink，
// sink 调优参数（队列容量、缓冲大小、flush/sync 间隔）由 SinkConfig 传入。
//
// 调用方应在装配根根据 app_config 中的 storage 配置构造 SinkConfig，
// 然后通过 NewSinkFromConfig 创建 sink，注入到 StorageRecorder。
package storage

import (
	"fmt"
	"time"

	"wind-daq/services/api-go/internal/ports"
)

// 存储格式常量
const (
	// FormatCSV 文本 CSV 格式（默认，兼容现有工具链）
	FormatCSV = "csv"
	// FormatBinary 二进制紧凑格式（高吞吐场景）
	FormatBinary = "binary"
)

// SinkConfig 存储适配器统一配置，包含格式选择与异步参数。
// 用于在装配根根据 app_config 创建 sink。
type SinkConfig struct {
	// Format 存储格式："csv" 或 "binary"，零值默认 "csv"
	Format string
	// QueueCapacity 异步队列容量，0 表示用默认值
	QueueCapacity int
	// BufferSize bufio 缓冲大小，0 表示用默认值
	BufferSize int
	// FlushIntervalMs 定时 flush 间隔（毫秒），0 表示用默认值
	FlushIntervalMs int
	// SyncIntervalSec 定时 fsync 间隔（秒），0 表示用默认值
	SyncIntervalSec int
}

// NewSinkFromConfig 根据配置创建对应的 RecordingSink。
// format 为空或 "csv" 时返回 CSV sink；"binary" 时返回 Binary sink；其他值返回错误。
//
// 注意：sink 的 Start 接收完整的 RecordingConfig（含 StopConditions/FileRotation），
// 本工厂只决定 sink 类型与异步调优参数，业务配置由 Start 时传入。
func NewSinkFromConfig(cfg SinkConfig) (ports.RecordingSink, error) {
	format := cfg.Format
	if format == "" {
		format = FormatCSV
	}
	asyncCfg := CSVSinkConfig{
		QueueCapacity: cfg.QueueCapacity,
		BufferSize:    cfg.BufferSize,
		FlushInterval: time.Duration(cfg.FlushIntervalMs) * time.Millisecond,
		SyncInterval:  time.Duration(cfg.SyncIntervalSec) * time.Second,
	}
	switch format {
	case FormatCSV:
		return NewCSVRecordingSinkWithConfig(asyncCfg), nil
	case FormatBinary:
		return NewBinaryRecordingSinkWithConfig(asyncCfg), nil
	default:
		return nil, fmt.Errorf("unsupported storage format: %q (supported: csv, binary)", format)
	}
}
