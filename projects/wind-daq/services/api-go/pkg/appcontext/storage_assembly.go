// storage_assembly.go 存储装配函数：从 AppConfigStore 读取 storage.json，
// 根据配置创建对应格式的 RecordingSink 并装配为 StorageRecorder。
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

type storageAppConfig struct {
	Format          string `json:"format"`
	QueueCapacity   int    `json:"queueCapacity"`
	BufferSize      int    `json:"bufferSize"`
	FlushIntervalMs int    `json:"flushIntervalMs"`
	SyncIntervalSec int    `json:"syncIntervalSec"`
}

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
