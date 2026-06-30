// sink_factory.go 存储适配器工厂。
//
// 根据存储格式（csv/binary）创建对应的 RecordingSink，
// 配置由调用方传入，可从 app_config 文件读取后注入。
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
