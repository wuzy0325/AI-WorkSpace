// storage_assembly.go 存储装配函数：从 AppConfigStore 读取 storage.json，
// 根据配置创建对应格式的 RecordingSink 并装配为 StorageRecorder。
//
// 装配职责：
//   - 读取 storage 配置文件（包含 sink 调优参数：format/queue/buffer/flush/sync）
//   - 通过 NewSinkFromConfig 创建 sink
//   - 用 sink 构造 StorageRecorder，由 usecase 层注入业务级 RecordingConfig
//
// 配置分层：
//   - sink 调优参数（format/queueCapacity/bufferSize/flush/sync）：装配时一次性读取，sink 生命周期内不变
//   - 业务级配置（outputDir/filePrefix/stopConditions/fileRotation）：每次 Start 由 UI 传入
package appcontext

import (
	"encoding/json"
	"log/slog"

	"wind-daq/services/api-go/internal/adapters/storage"
	"wind-daq/services/api-go/internal/ports"
	"wind-daq/services/api-go/internal/usecase"
)

const storageConfigName = "storage"
const defaultStorageFormat = "csv"

// storageAppConfig sink 调优参数配置，持久化在 storage.json。
// 业务级配置（outputDir/filePrefix/stopConditions/fileRotation）由 UI 通过
// storage-settings 配置键管理，在每次 Start 录制时传入，不在此处读取。
type storageAppConfig struct {
	Format          string `json:"format"`
	QueueCapacity   int    `json:"queueCapacity"`
	BufferSize      int    `json:"bufferSize"`
	FlushIntervalMs int    `json:"flushIntervalMs"`
	SyncIntervalSec int    `json:"syncIntervalSec"`
}

// NewStorageRecorderFromConfigStore 从配置存储装配 StorageRecorder。
// 读取 storage.json 拿到 sink 调优参数，创建 sink 并注入 StorageRecorder。
// 业务级 RecordingConfig 在每次 Start 时由调用方传入。
func NewStorageRecorderFromConfigStore(store ports.AppConfigStore) (*usecase.StorageRecorder, error) {
	storCfg := loadStorageConfigFromStore(store)

	sink, err := storage.NewSinkFromConfig(storage.SinkConfig{
		Format:          storCfg.Format,
		QueueCapacity:   storCfg.QueueCapacity,
		BufferSize:      storCfg.BufferSize,
		FlushIntervalMs: storCfg.FlushIntervalMs,
		SyncIntervalSec: storCfg.SyncIntervalSec,
	})
	if err != nil {
		return nil, err
	}
	return usecase.NewStorageRecorder(sink), nil
}

func loadStorageConfigFromStore(store ports.AppConfigStore) storageAppConfig {
	cfg := storageAppConfig{
		Format: defaultStorageFormat,
	}
	data, err := store.LoadConfig(storageConfigName)
	if err != nil {
		slog.Warn("装配: 读取存储配置失败，使用默认值",
			"component", "usecase",
			"error", err,
			"format", cfg.Format,
		)
		return cfg
	}
	if len(data) == 0 {
		return cfg
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		slog.Warn("装配: 解析存储配置失败，使用默认值",
			"component", "usecase",
			"error", err,
			"format", defaultStorageFormat,
		)
		return storageAppConfig{Format: defaultStorageFormat}
	}
	if cfg.Format == "" {
		cfg.Format = defaultStorageFormat
	}
	return cfg
}
