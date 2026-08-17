package storage

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"windlabx4/services/api-go/internal/ports"
)

const traversalActiveIndexVersion = 1

type traversalActiveIndexFile struct {
	Version int               `json:"version"`
	Tasks   map[string]string `json:"tasks"`
}

// TraversalActiveIndex 活动遍历任务索引（taskId → checkpointPath）。
//
// 编译期接口断言哨兵：确保 TraversalActiveIndex 始终满足 ports.TraversalActiveIndex
// 接口契约，方法签名漂移会在编译期立即暴露，而非运行期断言失败。
var _ ports.TraversalActiveIndex = (*TraversalActiveIndex)(nil)

type TraversalActiveIndex struct {
	mu    sync.Mutex
	path  string
	store *FileCheckpointStore
}

func NewTraversalActiveIndex(path, _ string) *TraversalActiveIndex {
	return &TraversalActiveIndex{path: filepath.Clean(path), store: NewFileCheckpointStore()}
}

func (i *TraversalActiveIndex) Register(ctx context.Context, taskID, checkpointPath string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if taskID == "" {
		return errors.New("活动遍历任务标识不能为空")
	}
	absolutePath, err := i.validatePath(checkpointPath)
	if err != nil {
		return err
	}
	i.mu.Lock()
	defer i.mu.Unlock()
	index, err := i.loadLocked()
	if err != nil {
		return err
	}
	index.Tasks[taskID] = absolutePath
	return i.saveLocked(index)
}

func (i *TraversalActiveIndex) Find(ctx context.Context, taskID string) (ports.TraversalCheckpointRef, bool, error) {
	if err := ctx.Err(); err != nil {
		return ports.TraversalCheckpointRef{}, false, err
	}
	i.mu.Lock()
	defer i.mu.Unlock()
	index, err := i.loadLocked()
	if err != nil {
		return ports.TraversalCheckpointRef{}, false, err
	}
	path, found := index.Tasks[taskID]
	if !found {
		return ports.TraversalCheckpointRef{}, false, nil
	}
	if _, err := i.validatePath(path); err != nil {
		return ports.TraversalCheckpointRef{}, false, err
	}
	return ports.TraversalCheckpointRef{TaskID: taskID, Path: path}, true, nil
}

func (i *TraversalActiveIndex) Unregister(ctx context.Context, taskID string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	i.mu.Lock()
	defer i.mu.Unlock()
	index, err := i.loadLocked()
	if err != nil {
		return err
	}
	if _, found := index.Tasks[taskID]; !found {
		return nil
	}
	delete(index.Tasks, taskID)
	return i.saveLocked(index)
}

// loadLocked 读取活动索引文件。
//
// 通过 store.Read 而非裸 os.ReadFile 读取：store 内部的 RWMutex 与 store.Write
// 互斥，避免读期间遇到原子替换的临时窗口（tmp rename 到正式路径的间隙）。
// mu.Lock 已保护本实例并发调用，但 store 锁跨实例共享，防御性更强。
func (i *TraversalActiveIndex) loadLocked() (traversalActiveIndexFile, error) {
	data, err := i.store.Read(i.path)
	if errors.Is(err, os.ErrNotExist) {
		return traversalActiveIndexFile{Version: traversalActiveIndexVersion, Tasks: make(map[string]string)}, nil
	}
	if err != nil {
		return traversalActiveIndexFile{}, fmt.Errorf("读取活动遍历索引失败: %w", err)
	}
	var index traversalActiveIndexFile
	if err := json.Unmarshal(data, &index); err != nil {
		return traversalActiveIndexFile{}, fmt.Errorf("解析活动遍历索引失败: %w", err)
	}
	if index.Version != traversalActiveIndexVersion || index.Tasks == nil {
		return traversalActiveIndexFile{}, errors.New("活动遍历索引版本或任务映射无效")
	}
	return index, nil
}

func (i *TraversalActiveIndex) saveLocked(index traversalActiveIndexFile) error {
	data, err := json.Marshal(index)
	if err != nil {
		return fmt.Errorf("编码活动遍历索引失败: %w", err)
	}
	if err := i.store.Write(i.path, data); err != nil {
		return fmt.Errorf("保存活动遍历索引失败: %w", err)
	}
	return nil
}

// validatePath 将 checkpoint 路径规范化为绝对路径，并限制为 checkpoint JSON 文件。
// checkpoint 与 CSV 同目录，用户可将遍历结果保存到配置目录之外，因此不能按
// 活动索引文件所在目录限制路径范围。路径由后端从实际 CSV 路径派生，不接受客户端
// 提供的任意文件类型。
func (i *TraversalActiveIndex) validatePath(path string) (string, error) {
	absolutePath, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("解析 checkpoint 路径失败: %w", err)
	}
	if !strings.HasSuffix(strings.ToLower(filepath.Base(absolutePath)), ".checkpoint.json") {
		return "", errors.New("活动遍历索引仅允许 checkpoint JSON 路径")
	}
	return absolutePath, nil
}
