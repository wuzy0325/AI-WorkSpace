package usecase

import (
	"encoding/json"

	"wind-daq/services/api-go/internal/ports"
)

type ConfigManager struct {
	store ports.AppConfigStore
}

func NewConfigManager(store ports.AppConfigStore) *ConfigManager {
	return &ConfigManager{store: store}
}

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

func (m *ConfigManager) SaveConfig(key string, config json.RawMessage) error {
	return m.store.SaveConfig(key, []byte(config))
}
