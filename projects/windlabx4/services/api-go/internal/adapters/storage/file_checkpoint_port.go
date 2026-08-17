package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"windlabx4/services/api-go/internal/core/traversal"
	"windlabx4/services/api-go/internal/ports"
)

// FileCheckpointPort 将底层 CheckpointStore（字节 I/O）适配为 TraversalCheckpointPort。
// 负责 JSON 序列化/反序列化和路径构造，使 usecase 层不直接操作文件路径。
//
// 资源语义：底层 CheckpointStore（字节 I/O）无持久句柄，每次 Save/Load 都通过
// store.Read/Write 短暂打开文件。Close 仅标记 closed 状态防止误用，不释放句柄。
//
// basePath 在 csvPort.Open 后由 usecase 层通过 SetBasePath 更新为实际落盘的 CSV 路径
// （撞名 -2/-3 后与 ResolveOutputPath(config) 不同），保证 checkpoint stem 与 CSV stem 一致。
//
// codec 模式（spec FR8/Task 10）：显式区分 legacy（v1/v2）与 dual（v3）编解码，
// 不按文件名猜测版本：
//   - legacy：保持既有 v1/v2 解码与写入行为；Load 遇到 v3 拒绝（不读 v3）；
//   - dual：Save 要求 Version==3；Load 遇到 v1/v2 返回
//     ports.ErrCheckpointVersionMismatch（不自动迁移）。
type FileCheckpointPort struct {
	store    ports.CheckpointStore
	basePath string // 实际落盘的 CSV 路径（撞名后由 SetBasePath 更新）
	mode     CheckpointCodecMode

	mu     sync.RWMutex
	closed bool // Close 后拒绝后续 Save/Load/Find/Unregister，避免使用已释放端口
}

// CheckpointCodecMode checkpoint 编解码模式（显式版本路由）。
type CheckpointCodecMode int

const (
	// CheckpointCodecLegacy legacy single：v1/v2 读写（现状）。
	CheckpointCodecLegacy CheckpointCodecMode = iota
	// CheckpointCodecDual 双探针：v3 读写。
	CheckpointCodecDual
)

// 编译期接口断言：确保 FileCheckpointPort 始终满足 ports.TraversalCheckpointPort 契约。
// 接口方法签名变更时立即在编译期暴露，避免运行期才发现缺失方法。
var _ ports.TraversalCheckpointPort = (*FileCheckpointPort)(nil)

// NewFileCheckpointPort 创建文件系统断点端口适配器（legacy v1/v2 codec）。
// basePath 初始为 ResolveOutputPath(config) 返回的预期 CSV 路径；
// csvPort.Open 后 usecase 层应调 SetBasePath 更新为实际撞名后路径。
func NewFileCheckpointPort(store ports.CheckpointStore, basePath string) *FileCheckpointPort {
	return &FileCheckpointPort{store: store, basePath: basePath, mode: CheckpointCodecLegacy}
}

// NewDualCheckpointPort 创建双探针 v3 codec 的断点端口适配器（spec FR8）。
// dual manager 的 checkpoint port factory 应使用本构造函数（Task 14 装配）。
func NewDualCheckpointPort(store ports.CheckpointStore, basePath string) *FileCheckpointPort {
	return &FileCheckpointPort{store: store, basePath: basePath, mode: CheckpointCodecDual}
}

// SetBasePath 更新 basePath 为 csvPort 实际落盘的 CSV 路径。
// 在 csvPort.Open(Create) 成功后调用，把 basePath 从"预期路径"切换为"实际路径"，
// 保证 checkpoint 文件名 stem 与 CSV 撞名后的 stem 一致，避免 Resume 打开错误 CSV。
func (p *FileCheckpointPort) SetBasePath(csvPath string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.basePath = csvPath
}

// path 返回 checkpoint 文件路径，放在 CSV 同目录的 .traversal/ 隐藏子目录下。
// 派生规则由 traversal.ResolveCheckpointPathFromCSV 单一函数定义，本方法仅包装。
// 父目录由 store.Write → atomicReplaceFile 内部 MkdirAll 兜底创建，无需在此重复。
func (p *FileCheckpointPort) path() string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return traversal.ResolveCheckpointPathFromCSV(p.basePath)
}

// Save 原子写入断点文件（JSON 序列化 + CheckpointStore.Write 原子替换）
func (p *FileCheckpointPort) Save(ctx context.Context, checkpoint traversal.Checkpoint) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := p.checkOpen(); err != nil {
		return err
	}
	if p.mode == CheckpointCodecDual && checkpoint.Version != traversal.DualCheckpointVersion {
		return fmt.Errorf("%w: dual checkpoint port 仅写入 v3（got v%d）",
			ports.ErrCheckpointVersionMismatch, checkpoint.Version)
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

// Load 读取并反序列化断点文件；按 codec 模式显式校验版本（不按文件名猜测）。
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
	version, err := peekCheckpointVersion(data)
	if err != nil {
		return traversal.Checkpoint{}, err
	}
	if err := p.checkVersion(version); err != nil {
		return traversal.Checkpoint{}, err
	}
	var cp traversal.Checkpoint
	if err := json.Unmarshal(data, &cp); err != nil {
		return traversal.Checkpoint{}, fmt.Errorf("parse checkpoint: %w", err)
	}
	return cp, nil
}

func peekCheckpointVersion(data []byte) (int, error) {
	var header struct {
		Version int `json:"version"`
	}
	if err := json.Unmarshal(data, &header); err != nil {
		return 0, fmt.Errorf("探测 checkpoint 版本失败: %w", err)
	}
	return header.Version, nil
}

// checkVersion 按 codec 模式校验版本：dual 不读 v1/v2（不自动迁移），legacy 不读 v3。
func (p *FileCheckpointPort) checkVersion(version int) error {
	if p.mode == CheckpointCodecDual && version != traversal.DualCheckpointVersion {
		return fmt.Errorf("%w: dual 路径仅读取 v3（got v%d）", ports.ErrCheckpointVersionMismatch, version)
	}
	if p.mode == CheckpointCodecLegacy && version == traversal.DualCheckpointVersion {
		return fmt.Errorf("%w: legacy 路径不读取 v3", ports.ErrCheckpointVersionMismatch)
	}
	return nil
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
