package storage

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"wind-daq/services/api-go/internal/ports"
)

// DualTraversalRecoveryIndexVersion 双探针恢复索引 envelope 版本。
const DualTraversalRecoveryIndexVersion = 1

// DualTraversalRecoveryIndexFileName 默认索引文件名；与 legacy
// traversal-active-index.json 不同，两个文件互不读写、互不迁移。
const DualTraversalRecoveryIndexFileName = "dual-traversal-recovery-index.json"

// dualTraversalRecoveryIndexFile 索引 envelope：
// {"version":1,"probes":{"probe1":{"<taskId>":"<checkpointPath>"}}}
type dualTraversalRecoveryIndexFile struct {
	Version int                          `json:"version"`
	Probes  map[string]map[string]string `json:"probes"`
}

// DualTraversalRecoveryIndex 双探针恢复索引（probeId → taskId → checkpointPath）。
//
// 与 legacy TraversalActiveIndex 的原子替换流程一致（FileCheckpointStore 写
// tmp + rename），但文件独立：本类型不读取、不迁移、不覆盖 legacy 索引。
// 不变量：每个 probe 最多一个权威可恢复候选；task ID 全局唯一（spec FR4/FR8）。
var _ ports.DualTraversalRecoveryIndex = (*DualTraversalRecoveryIndex)(nil)

type DualTraversalRecoveryIndex struct {
	mu    sync.Mutex
	path  string
	store *FileCheckpointStore
}

// NewDualTraversalRecoveryIndex 创建索引；path 为索引文件完整路径。
func NewDualTraversalRecoveryIndex(path string) *DualTraversalRecoveryIndex {
	return &DualTraversalRecoveryIndex{path: filepath.Clean(path), store: NewFileCheckpointStore()}
}

// Register 登记 probe 的可恢复任务（语义见 ports.DualTraversalRecoveryIndex）。
func (i *DualTraversalRecoveryIndex) Register(ctx context.Context, probeID, taskID, checkpointPath string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if probeID == "" {
		return errors.New("probe 标识不能为空")
	}
	if taskID == "" {
		return errors.New("任务标识不能为空")
	}
	absolutePath, err := validateDualCheckpointPath(checkpointPath)
	if err != nil {
		return err
	}
	i.mu.Lock()
	defer i.mu.Unlock()
	index, err := i.loadLocked()
	if err != nil {
		return err
	}
	// task ID 全局唯一：已登记到其它 probe 时拒绝（spec FR8）
	for pid, tasks := range index.Probes {
		if _, exists := tasks[taskID]; exists && pid != probeID {
			return fmt.Errorf("%w: task %s 已注册到 %s", ports.ErrTaskIDRegisteredToOtherProbe, taskID, pid)
		}
	}
	// 每 probe 最多一个权威候选：已有其它 taskID 时拒绝
	for existing := range index.Probes[probeID] {
		if existing != taskID {
			return fmt.Errorf("%w: probe %s 已存在可恢复任务 %s", ports.ErrRecoverableTaskExists, probeID, existing)
		}
	}
	tasks := index.Probes[probeID]
	if tasks == nil {
		tasks = make(map[string]string)
		index.Probes[probeID] = tasks
	}
	tasks[taskID] = absolutePath
	return i.saveLocked(index)
}

// Find 返回该 probe 的唯一可恢复候选；不存在时 found=false 且 err=nil。
func (i *DualTraversalRecoveryIndex) Find(ctx context.Context, probeID string) (ports.TraversalCheckpointRef, bool, error) {
	if err := ctx.Err(); err != nil {
		return ports.TraversalCheckpointRef{}, false, err
	}
	i.mu.Lock()
	defer i.mu.Unlock()
	index, err := i.loadLocked()
	if err != nil {
		return ports.TraversalCheckpointRef{}, false, err
	}
	tasks := index.Probes[probeID]
	if len(tasks) == 0 {
		return ports.TraversalCheckpointRef{}, false, nil
	}
	if len(tasks) > 1 {
		return ports.TraversalCheckpointRef{}, false, fmt.Errorf("双探针恢复索引损坏: probe %s 存在 %d 个候选", probeID, len(tasks))
	}
	for taskID, path := range tasks {
		if _, err := validateDualCheckpointPath(path); err != nil {
			return ports.TraversalCheckpointRef{}, false, err
		}
		return ports.TraversalCheckpointRef{TaskID: taskID, Path: path}, true, nil
	}
	return ports.TraversalCheckpointRef{}, false, nil
}

// Unregister 注销该 probe 的恢复映射；映射不存在时幂等成功，
// taskID 与登记的候选不一致时返回错误（防止错误 taskID 清除候选）。
func (i *DualTraversalRecoveryIndex) Unregister(ctx context.Context, probeID, taskID string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	i.mu.Lock()
	defer i.mu.Unlock()
	index, err := i.loadLocked()
	if err != nil {
		return err
	}
	tasks := index.Probes[probeID]
	if len(tasks) == 0 {
		return nil
	}
	if _, found := tasks[taskID]; !found {
		return fmt.Errorf("注销任务与登记候选不一致: probe=%s task=%s", probeID, taskID)
	}
	delete(tasks, taskID)
	if len(tasks) == 0 {
		delete(index.Probes, probeID)
	}
	return i.saveLocked(index)
}

// ListProbeTaskIDs 返回该 probe 已登记的全部 taskID（字典序；不变量下最多一个）。
func (i *DualTraversalRecoveryIndex) ListProbeTaskIDs(ctx context.Context, probeID string) ([]string, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	i.mu.Lock()
	defer i.mu.Unlock()
	index, err := i.loadLocked()
	if err != nil {
		return nil, err
	}
	taskIDs := make([]string, 0, len(index.Probes[probeID]))
	for taskID := range index.Probes[probeID] {
		taskIDs = append(taskIDs, taskID)
	}
	sort.Strings(taskIDs)
	return taskIDs, nil
}

// loadLocked 读取索引文件；文件不存在时返回空 envelope。
// 与 legacy 一致通过 store.Read 读取，与 store.Write 的原子替换互斥。
func (i *DualTraversalRecoveryIndex) loadLocked() (dualTraversalRecoveryIndexFile, error) {
	data, err := i.store.Read(i.path)
	if errors.Is(err, os.ErrNotExist) {
		return dualTraversalRecoveryIndexFile{Version: DualTraversalRecoveryIndexVersion, Probes: make(map[string]map[string]string)}, nil
	}
	if err != nil {
		return dualTraversalRecoveryIndexFile{}, fmt.Errorf("读取双探针恢复索引失败: %w", err)
	}
	var index dualTraversalRecoveryIndexFile
	if err := json.Unmarshal(data, &index); err != nil {
		return dualTraversalRecoveryIndexFile{}, fmt.Errorf("解析双探针恢复索引失败: %w", err)
	}
	if index.Version != DualTraversalRecoveryIndexVersion || index.Probes == nil {
		return dualTraversalRecoveryIndexFile{}, errors.New("双探针恢复索引版本或探针映射无效")
	}
	return index, nil
}

func (i *DualTraversalRecoveryIndex) saveLocked(index dualTraversalRecoveryIndexFile) error {
	data, err := json.Marshal(index)
	if err != nil {
		return fmt.Errorf("编码双探针恢复索引失败: %w", err)
	}
	if err := i.store.Write(i.path, data); err != nil {
		return fmt.Errorf("保存双探针恢复索引失败: %w", err)
	}
	return nil
}

// validateDualCheckpointPath 将 checkpoint 路径规范化为绝对路径，并限制为
// checkpoint JSON 文件（规则与 legacy 活动索引一致）。
func validateDualCheckpointPath(path string) (string, error) {
	absolutePath, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("解析 checkpoint 路径失败: %w", err)
	}
	if !strings.HasSuffix(strings.ToLower(filepath.Base(absolutePath)), ".checkpoint.json") {
		return "", errors.New("双探针恢复索引仅允许 checkpoint JSON 路径")
	}
	return absolutePath, nil
}
