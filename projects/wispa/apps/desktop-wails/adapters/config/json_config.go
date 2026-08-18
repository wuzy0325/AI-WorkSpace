package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"wispa/core"

	"shared.local/device-sdk/go/pkg/slog"
)

// JSONConfigStore 基于 JSON 文件的配置存储
type JSONConfigStore struct {
	mu       sync.RWMutex
	filePath string
}

// NewJSONConfigStore 创建 JSON 配置存储
func NewJSONConfigStore(filePath string) *JSONConfigStore {
	return &JSONConfigStore{filePath: filePath}
}

// LoadProfiles 加载所有设备配置
func (s *JSONConfigStore) LoadProfiles() ([]core.PressureProfile, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.loadUnsafe()
}

// SaveProfile 保存设备配置（存在则更新，不存在则新增）
func (s *JSONConfigStore) SaveProfile(profile core.PressureProfile) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	profiles, err := s.loadUnsafe()
	if err != nil {
		return err
	}
	replaced := false
	for i, p := range profiles {
		if p.ID == profile.ID {
			profiles[i] = profile
			replaced = true
			break
		}
	}
	if !replaced {
		profiles = append(profiles, profile)
	}
	return s.saveUnsafe(profiles)
}

// DeleteProfile 删除设备配置
func (s *JSONConfigStore) DeleteProfile(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	profiles, err := s.loadUnsafe()
	if err != nil {
		return err
	}
	filtered := make([]core.PressureProfile, 0, len(profiles))
	for _, p := range profiles {
		if p.ID != id {
			filtered = append(filtered, p)
		}
	}
	return s.saveUnsafe(filtered)
}

func (s *JSONConfigStore) loadUnsafe() ([]core.PressureProfile, error) {
	data, err := os.ReadFile(s.filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var profiles []core.PressureProfile
	if err := json.Unmarshal(data, &profiles); err != nil {
		slog.Warn("配置文件 JSON 解析失败，将使用空配置", "path", s.filePath, "error", err)
		return nil, nil
	}
	return profiles, nil
}

// saveUnsafe 原子写入配置文件
func (s *JSONConfigStore) saveUnsafe(profiles []core.PressureProfile) error {
	dir := filepath.Dir(s.filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("创建配置目录失败: %w", err)
	}

	data, err := json.MarshalIndent(profiles, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化配置失败: %w", err)
	}

	tmpFile, err := os.CreateTemp(dir, "config-*.tmp")
	if err != nil {
		return fmt.Errorf("创建临时文件失败: %w", err)
	}
	tmpPath := tmpFile.Name()

	if _, err := tmpFile.Write(data); err != nil {
		tmpFile.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("写入临时文件失败: %w", err)
	}
	if err := tmpFile.Close(); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("关闭临时文件失败: %w", err)
	}

	if err := os.Rename(tmpPath, s.filePath); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("替换配置文件失败: %w", err)
	}

	return nil
}
