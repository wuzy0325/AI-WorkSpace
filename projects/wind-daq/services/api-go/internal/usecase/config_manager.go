package usecase

import (
	"encoding/json"

	"wind-daq/services/api-go/internal/ports"
)

// ConfigManager 应用配置管理器
type ConfigManager struct {
	store ports.AppConfigStore
}

// NewConfigManager 创建配置管理器
func NewConfigManager(store ports.AppConfigStore) *ConfigManager {
	return &ConfigManager{store: store}
}

// LoadConfig 加载配置
func (m *ConfigManager) LoadConfig(key string) (json.RawMessage, error) {
	data, err := m.store.LoadConfig(key)
	if err != nil {
		return nil, err
	}
	if data == nil {
		return nil, nil
	}
	return json.RawMessage(data), nil
}

// SaveConfig 保存配置
func (m *ConfigManager) SaveConfig(key string, config json.RawMessage) error {
	return m.store.SaveConfig(key, []byte(config))
}
