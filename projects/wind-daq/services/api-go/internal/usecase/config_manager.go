package usecase

import (
	"encoding/json"
	"log/slog"

	"wind-daq/services/api-go/internal/ports"
)

type ConfigManager struct {
	store ports.AppConfigStore
}

func NewConfigManager(store ports.AppConfigStore) *ConfigManager {
	return &ConfigManager{store: store}
}

func (m *ConfigManager) LoadConfig(key string) (json.RawMessage, error) {
	slog.Info("ConfigManager LoadConfig 开始", "component", "ConfigManager", "key", key)
	data, err := m.store.LoadConfig(key)
	if err != nil {
		slog.Error("ConfigManager LoadConfig 失败", "component", "ConfigManager", "key", key, "error", err)
		return nil, err
	}
	if data == nil {
		slog.Warn("ConfigManager LoadConfig 未找到配置", "component", "ConfigManager", "key", key)
		return nil, nil
	}
	slog.Info("ConfigManager LoadConfig 成功", "component", "ConfigManager", "key", key, "size", len(data))
	return json.RawMessage(data), nil
}

func (m *ConfigManager) SaveConfig(key string, config json.RawMessage) error {
	slog.Info("ConfigManager SaveConfig 开始", "component", "ConfigManager", "key", key, "size", len(config))
	err := m.store.SaveConfig(key, []byte(config))
	if err != nil {
		slog.Error("ConfigManager SaveConfig 失败", "component", "ConfigManager", "key", key, "error", err)
		return err
	}
	slog.Info("ConfigManager SaveConfig 成功", "component", "ConfigManager", "key", key)
	return nil
}
