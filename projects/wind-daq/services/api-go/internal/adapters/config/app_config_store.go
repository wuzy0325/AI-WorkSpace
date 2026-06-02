package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"wind-daq/services/api-go/internal/ports"
)

// FileAppConfigStore 基于文件的应用配置存储
type FileAppConfigStore struct {
	baseDir string
}

// NewFileAppConfigStore 创建文件应用配置存储
func NewFileAppConfigStore(baseDir string) ports.AppConfigStore {
	return &FileAppConfigStore{baseDir: baseDir}
}

func (s *FileAppConfigStore) LoadConfig(key string) ([]byte, error) {
	path := s.configPath(key)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("读取配置失败: %w", err)
	}
	if !json.Valid(data) {
		return nil, fmt.Errorf("配置文件 %s 数据损坏（无效JSON）", path)
	}
	return data, nil
}

func (s *FileAppConfigStore) SaveConfig(key string, data []byte) error {
	if err := os.MkdirAll(s.baseDir, 0755); err != nil {
		return fmt.Errorf("创建配置目录失败: %w", err)
	}
	path := s.configPath(key)
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("保存配置失败: %w", err)
	}
	return nil
}

func (s *FileAppConfigStore) configPath(key string) string {
	return filepath.Join(s.baseDir, key+".json")
}
