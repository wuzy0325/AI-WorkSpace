package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"

	"wind-daq/services/api-go/internal/core/traversal"
	"wind-daq/services/api-go/internal/ports"
)

// FileCheckpointPort 将底层 CheckpointStore（字节 I/O）适配为 TraversalCheckpointPort。
// 负责 JSON 序列化/反序列化和路径构造，使 usecase 层不直接操作文件路径。
type FileCheckpointPort struct {
	store    ports.CheckpointStore
	basePath string // checkpoint 文件基础路径（通常取自 SavePath）
}

// NewFileCheckpointPort 创建文件系统断点端口适配器。
// basePath 为 checkpoint 文件的基础路径（如 SavePath），实际文件名为 basePath + ".checkpoint.json"。
// 由于每个遍历任务的 SavePath 不同，自然实现按任务隔离。
func NewFileCheckpointPort(store ports.CheckpointStore, basePath string) *FileCheckpointPort {
	return &FileCheckpointPort{store: store, basePath: basePath}
}

// path 返回 checkpoint 文件路径。
// basePath 已包含任务唯一性（来自 SavePath），无需额外拼 taskID。
// taskID 参数保留用于未来扩展（如多任务同目录场景）。
func (p *FileCheckpointPort) path(taskID string) string {
	return p.basePath + ".checkpoint.json"
}

// Save 原子写入断点文件（JSON 序列化 + CheckpointStore.Write 原子替换）
func (p *FileCheckpointPort) Save(ctx context.Context, checkpoint traversal.Checkpoint) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	data, err := json.MarshalIndent(checkpoint, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal checkpoint: %w", err)
	}
	if err := p.store.Write(p.path(checkpoint.TaskID), data); err != nil {
		return fmt.Errorf("write checkpoint: %w", err)
	}
	return nil
}

// Load 读取并反序列化断点文件
func (p *FileCheckpointPort) Load(ctx context.Context, taskID string) (traversal.Checkpoint, error) {
	if err := ctx.Err(); err != nil {
		return traversal.Checkpoint{}, err
	}
	exists, err := p.store.Stat(p.path(taskID))
	if err != nil {
		return traversal.Checkpoint{}, fmt.Errorf("stat checkpoint: %w", err)
	}
	if !exists {
		return traversal.Checkpoint{}, fmt.Errorf("checkpoint not found for task %s", taskID)
	}
	data, err := p.store.Read(p.path(taskID))
	if err != nil {
		return traversal.Checkpoint{}, fmt.Errorf("read checkpoint: %w", err)
	}
	var cp traversal.Checkpoint
	if err := json.Unmarshal(data, &cp); err != nil {
		return traversal.Checkpoint{}, fmt.Errorf("parse checkpoint: %w", err)
	}
	return cp, nil
}

// Find 查找断点引用（存在性检查）
func (p *FileCheckpointPort) Find(ctx context.Context, taskID string) (ports.TraversalCheckpointRef, bool, error) {
	if err := ctx.Err(); err != nil {
		return ports.TraversalCheckpointRef{}, false, err
	}
	cpPath := p.path(taskID)
	exists, err := p.store.Stat(cpPath)
	if err != nil {
		return ports.TraversalCheckpointRef{}, false, fmt.Errorf("stat checkpoint: %w", err)
	}
	if !exists {
		return ports.TraversalCheckpointRef{}, false, nil
	}
	return ports.TraversalCheckpointRef{TaskID: taskID, Path: cpPath}, true, nil
}

// Unregister 删除断点文件
func (p *FileCheckpointPort) Unregister(ctx context.Context, taskID string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := p.store.Remove(p.path(taskID)); err != nil {
		return fmt.Errorf("remove checkpoint: %w", err)
	}
	return nil
}

// ResolveCheckpointPath 根据 basePath 和 taskID 解析 checkpoint 路径。
// 供装配根在注入 SetCheckpointPort 时使用，确保路径与 FileCheckpointPort 内部一致。
func ResolveCheckpointPath(basePath string) string {
	return filepath.Base(basePath) + ".checkpoint.json"
}
