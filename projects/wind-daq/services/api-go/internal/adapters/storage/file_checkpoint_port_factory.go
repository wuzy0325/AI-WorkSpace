package storage

import (
	"context"

	"wind-daq/services/api-go/internal/ports"
)

// FileCheckpointPortFactory 按 SavePath 动态创建 FileCheckpointPort。
// 装配根注入此工厂，TraversalManager 在 Start 时调用 Create(ctx, savePath) 获得端口实例。
//
// 编译期接口断言哨兵：确保工厂始终满足 ports.TraversalCheckpointPortFactory 接口契约。
var _ ports.TraversalCheckpointPortFactory = (*FileCheckpointPortFactory)(nil)

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
// 当前实现仅做字段赋值，始终返回 nil error；ctx 与 error 参数为未来扩展保留
// （如基于 mmap / DB 的实现需要初始化连接）。
func (f *FileCheckpointPortFactory) Create(ctx context.Context, basePath string) (ports.TraversalCheckpointPort, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return NewFileCheckpointPort(f.store, basePath), nil
}
