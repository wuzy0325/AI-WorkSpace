package config

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"

	"wista/core"
)

type JSONConfigStore struct {
	mu       sync.RWMutex
	filePath string
}

func NewJSONConfigStore(filePath string) *JSONConfigStore {
	return &JSONConfigStore{filePath: filePath}
}

func (s *JSONConfigStore) LoadProfiles() ([]core.TemperatureProfile, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.loadUnsafe()
}

func (s *JSONConfigStore) SaveProfile(profile core.TemperatureProfile) error {
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

func (s *JSONConfigStore) DeleteProfile(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	profiles, err := s.loadUnsafe()
	if err != nil {
		return err
	}
	filtered := make([]core.TemperatureProfile, 0, len(profiles))
	for _, p := range profiles {
		if p.ID != id {
			filtered = append(filtered, p)
		}
	}
	return s.saveUnsafe(filtered)
}

func (s *JSONConfigStore) loadUnsafe() ([]core.TemperatureProfile, error) {
	data, err := os.ReadFile(s.filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var profiles []core.TemperatureProfile
	if err := json.Unmarshal(data, &profiles); err != nil {
		// JSON 解析失败时记录警告并返回空列表，避免损坏文件阻塞后续保存
		slog.Warn("配置文件 JSON 解析失败，将使用空配置", "path", s.filePath, "error", err)
		return nil, nil
	}
	return profiles, nil
}

// saveUnsafe 原子写入配置文件：先写入临时文件，再通过 rename 替换目标文件，
// 防止写入过程中崩溃导致配置文件损坏。
func (s *JSONConfigStore) saveUnsafe(profiles []core.TemperatureProfile) error {
	dir := filepath.Dir(s.filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("创建配置目录失败: %w", err)
	}

	data, err := json.MarshalIndent(profiles, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化配置失败: %w", err)
	}

	// 写入临时文件
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

	// 原子替换目标文件
	if err := os.Rename(tmpPath, s.filePath); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("替换配置文件失败: %w", err)
	}

	return nil
}
