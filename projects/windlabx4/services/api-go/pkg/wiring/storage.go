// Package wiring 中的 storage 装配辅助：把 adapters/storage 的实现暴露为 ports 接口。
package wiring

import (
	"windlabx4/services/api-go/internal/adapters/storage"
	"windlabx4/services/api-go/internal/ports"
)

// NewFileCheckpointStore 装配基于本机文件系统的断点存储为 ports.CheckpointStore。
// 测试和装配根均可调用本函数，避免直接 import adapters/storage。
func NewFileCheckpointStore() ports.CheckpointStore {
	return storage.NewFileCheckpointStore()
}
