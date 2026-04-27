package config

import (
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
)

// ==================== 配置管理器 ====================
// JSON文件配置持久化
// 与TS后端格式兼容

// Manager 配置持久化管理器
// 将配置保存为JSON文件
type Manager struct {
	mu  sync.RWMutex
	dir string // 配置目录
}

// NewManager 创建配置管理器
// 参数: dir 配置目录路径
// 返回: *Manager 管理器实例
func NewManager(dir string) *Manager {
	if err := os.MkdirAll(dir, 0755); err != nil {
		slog.Error("Failed to create config dir", "dir", dir, "err", err)
	}
	return &Manager{dir: dir}
}

// Load 加载配置文件
// 参数: name 配置名称(不含.json后缀), v 目标对象指针
// 返回: error 错误信息
func (m *Manager) Load(name string, v interface{}) error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	path := filepath.Join(m.dir, name+".json")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // 文件不存在,返回零值
		}
		return err
	}
	return json.Unmarshal(data, v)
}

// Save 保存配置文件
// 参数: name 配置名称, v 要保存的对象
// 返回: error 错误信息
func (m *Manager) Save(name string, v interface{}) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	path := filepath.Join(m.dir, name+".json")
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

// Dir 返回配置目录路径
// 返回: string 配置目录路径
func (m *Manager) Dir() string {
	return m.dir
}
