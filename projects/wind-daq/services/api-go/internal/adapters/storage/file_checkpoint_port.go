package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"wind-daq/services/api-go/internal/core/traversal"
	"wind-daq/services/api-go/internal/ports"
)

// FileCheckpointPort 将底层 CheckpointStore（字节 I/O）适配为 TraversalCheckpointPort。
// 负责 JSON 序列化/反序列化和路径构造，使 usecase 层不直接操作文件路径。
//
// 资源语义：底层 CheckpointStore（字节 I/O）无持久句柄，每次 Save/Load 都通过
// store.Read/Write 短暂打开文件。Close 仅标记 closed 状态防止误用，不释放句柄。
type FileCheckpointPort struct {
	store    ports.CheckpointStore
	basePath string // checkpoint 文件基础路径（通常取自 SavePath）

	mu     sync.RWMutex
	closed bool // Close 后拒绝后续 Save/Load/Find/Unregister，避免使用已释放端口
}

// 编译期接口断言：确保 FileCheckpointPort 始终满足 ports.TraversalCheckpointPort 契约。
// 接口方法签名变更时立即在编译期暴露，避免运行期才发现缺失方法。
var _ ports.TraversalCheckpointPort = (*FileCheckpointPort)(nil)

// NewFileCheckpointPort 创建文件系统断点端口适配器。
// basePath 为 checkpoint 文件的基础路径（如 CSV 路径），实际文件放在 basePath 同目录的
// .traversal/ 隐藏子目录下，避免用户看到内部文件。
// 由于每个遍历任务的 CSV 路径不同，自然实现按任务隔离。
func NewFileCheckpointPort(store ports.CheckpointStore, basePath string) *FileCheckpointPort {
	return &FileCheckpointPort{store: store, basePath: basePath}
}

// path 返回 checkpoint 文件路径，放在 CSV 同目录的 .traversal/ 隐藏子目录下。
// 派生规则：basePath = dir/stem.csv → checkpointPath = dir/.traversal/stem.checkpoint.json
// 确保 .traversal/ 父目录存在，避免首次写入因目录缺失失败。
func (p *FileCheckpointPort) path() string {
	ext := filepath.Ext(p.basePath)
	base := strings.TrimSuffix(p.basePath, ext)
	dir := filepath.Dir(base)
	stem := filepath.Base(base)
	cpPath := filepath.Join(dir, ".traversal", stem + ".checkpoint.json")
	// 确保父目录存在
	_ = os.MkdirAll(filepath.Dir(cpPath), 0o755)
	return cpPath
}

// Save 原子写入断点文件（JSON 序列化 + CheckpointStore.Write 原子替换）
func (p *FileCheckpointPort) Save(ctx context.Context, checkpoint traversal.Checkpoint) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := p.checkOpen(); err != nil {
		return err
	}
	data, err := json.MarshalIndent(checkpoint, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal checkpoint: %w", err)
	}
	if err := p.store.Write(p.path(), data); err != nil {
		return fmt.Errorf("write checkpoint: %w", err)
	}
	return nil
}

// Load 读取并反序列化断点文件
func (p *FileCheckpointPort) Load(ctx context.Context, taskID string) (traversal.Checkpoint, error) {
	if err := ctx.Err(); err != nil {
		return traversal.Checkpoint{}, err
	}
	if err := p.checkOpen(); err != nil {
		return traversal.Checkpoint{}, err
	}
	exists, err := p.store.Stat(p.path())
	if err != nil {
		return traversal.Checkpoint{}, fmt.Errorf("stat checkpoint: %w", err)
	}
	if !exists {
		return traversal.Checkpoint{}, fmt.Errorf("checkpoint not found for task %s", taskID)
	}
	data, err := p.store.Read(p.path())
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
	if err := p.checkOpen(); err != nil {
		return ports.TraversalCheckpointRef{}, false, err
	}
	cpPath := p.path()
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
	if err := p.checkOpen(); err != nil {
		return err
	}
	if err := p.store.Remove(p.path()); err != nil {
		return fmt.Errorf("remove checkpoint: %w", err)
	}
	return nil
}

// Close 标记端口为已关闭。FileCheckpointPort 无底层句柄资源，Close 仅做状态标记，
// 防止任务结束后误用已 Close 的端口（如 finalizeSink 重入时再次 Save）。
// 幂等：重复调用返回 nil。
func (p *FileCheckpointPort) Close(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.closed = true
	return nil
}

// checkOpen 在锁外读取 closed 标志，已 Close 时返回错误。
// 用 RLock 而非 mu.Lock 是为了与 Save/Load 等读路径并发安全；
// closed 标志一旦置 true 不会回退，读后立即释放锁是安全的。
func (p *FileCheckpointPort) checkOpen() error {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if p.closed {
		return fmt.Errorf("checkpoint port already closed")
	}
	return nil
}
