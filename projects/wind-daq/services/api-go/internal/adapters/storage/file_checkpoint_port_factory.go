package storage

import (
	"wind-daq/services/api-go/internal/ports"
)

// FileCheckpointPortFactory 按 SavePath 动态创建 FileCheckpointPort。
// 装配根注入此工厂，TraversalManager 在 Start 时调用 Create(savePath) 获得端口实例。
type FileCheckpointPortFactory struct {
	store ports.CheckpointStore
}

// NewFileCheckpointPortFactory 创建断点端口工厂。
// store 为底层原子文件存储，由装配根注入（通常为 NewFileCheckpointStore()）。
func NewFileCheckpointPortFactory(store ports.CheckpointStore) *FileCheckpointPortFactory {
	return &FileCheckpointPortFactory{store: store}
}

// Create 按 basePath 创建 FileCheckpointPort 实例。
// basePath 通常为 config.SavePath，由此保证不同任务的断点文件互不覆盖。
func (f *FileCheckpointPortFactory) Create(basePath string) ports.TraversalCheckpointPort {
	return NewFileCheckpointPort(f.store, basePath)
}
