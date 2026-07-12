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

	"wind-daq/services/api-go/internal/ports"
)

const traversalActiveIndexVersion = 1

type traversalActiveIndexFile struct {
	Version int               `json:"version"`
	Tasks   map[string]string `json:"tasks"`
}

type TraversalActiveIndex struct {
	mu      sync.Mutex
	path    string
	dataDir string
	store   *FileCheckpointStore
}

func NewTraversalActiveIndex(path, dataDir string) *TraversalActiveIndex {
	return &TraversalActiveIndex{path: filepath.Clean(path), dataDir: filepath.Clean(dataDir), store: NewFileCheckpointStore()}
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

func (i *TraversalActiveIndex) loadLocked() (traversalActiveIndexFile, error) {
	data, err := os.ReadFile(i.path)
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

func (i *TraversalActiveIndex) validatePath(path string) (string, error) {
	absoluteDataDir, err := filepath.Abs(i.dataDir)
	if err != nil {
		return "", fmt.Errorf("解析活动遍历数据目录失败: %w", err)
	}
	absolutePath, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("解析 checkpoint 路径失败: %w", err)
	}
	relative, err := filepath.Rel(absoluteDataDir, absolutePath)
	if err != nil {
		return "", fmt.Errorf("校验 checkpoint 路径失败: %w", err)
	}
	if relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) || filepath.IsAbs(relative) {
		return "", errors.New("checkpoint 路径超出活动遍历数据目录")
	}
	return absolutePath, nil
}
