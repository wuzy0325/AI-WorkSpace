// Package usecase — checkpoint 持久化相关方法（从 traversal.go 拆分）
//
// 包含：LoadCheckpoint / saveCheckpoint / ClearCheckpoint / ResumeFromCheckpoint。
// 这些方法都通过 ports.CheckpointStore 接口与文件系统交互，与采集主循环解耦后
// 便于测试与维护。
package usecase

import (
	"encoding/json"
	"fmt"
	"time"

	"wind-daq/services/api-go/internal/core/resourcelock"
	"wind-daq/services/api-go/internal/core/traversal"
)

func (m *TraversalManager) LoadCheckpoint() (*traversal.Checkpoint, error) {
	m.mu.RLock()
	path := m.lastCheckpointPath
	store := m.checkpointStore
	m.mu.RUnlock()

	if path == "" {
		return nil, nil
	}
	if store == nil {
		return nil, nil
	}
	exists, err := store.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("stat checkpoint: %w", err)
	}
	if !exists {
		// 文件已被外部清理，重置路径
		m.mu.Lock()
		m.lastCheckpointPath = ""
		m.mu.Unlock()
		return nil, nil
	}

	data, err := store.Read(path)
	if err != nil {
		return nil, fmt.Errorf("read checkpoint: %w", err)
	}
	var cp traversal.Checkpoint
	if err := json.Unmarshal(data, &cp); err != nil {
		return nil, fmt.Errorf("parse checkpoint: %w", err)
	}
	return &cp, nil
}

// saveCheckpoint 保存断点到 ${savePath}.checkpoint.json，使用原子写入
// 与 Cursor DAQ 的 atomicWriteJson 行为一致：先写临时文件再 rename，避免半写入状态
func (m *TraversalManager) saveCheckpoint(points []traversal.Point, completedCount int, savePath string) {
	m.mu.RLock()
	config := m.config
	taskID := m.status.TaskID
	configRaw := m.configRaw
	m.mu.RUnlock()

	if taskID == "" || savePath == "" {
		return
	}

	// 优先使用启动时保存的原始配置 JSON，便于前端完整恢复
	configPayload := configRaw
	if len(configPayload) == 0 {
		if raw, err := json.Marshal(config); err == nil {
			configPayload = raw
		}
	}

	var lastPoint *traversal.Point
	if completedCount > 0 && completedCount <= len(points) {
		lp := points[completedCount-1]
		lastPoint = &lp
	}

	checkpoint := traversal.Checkpoint{
		TaskID:          taskID,
		Config:          []byte(configPayload),
		CompletedPoints: completedCount,
		TotalPoints:     len(points),
		LastPoint:       lastPoint,
		SavePath:        savePath,
		CreatedAt:       time.Now().UnixMilli(),
	}

	data, err := json.MarshalIndent(checkpoint, "", "  ")
	if err != nil {
		return
	}

	checkpointPath := savePath + ".checkpoint.json"
	if m.checkpointStore == nil {
		return
	}
	if err := m.checkpointStore.Write(checkpointPath, data); err != nil {
		return
	}

	m.mu.Lock()
	m.lastCheckpointPath = checkpointPath
	m.mu.Unlock()
}

// ClearCheckpoint 删除断点文件并清空 lastCheckpointPath
// 测试成功完成后调用，避免残留断点文件
func (m *TraversalManager) ClearCheckpoint() {
	m.mu.Lock()
	path := m.lastCheckpointPath
	store := m.checkpointStore
	m.lastCheckpointPath = ""
	m.mu.Unlock()

	if path == "" || store == nil {
		return
	}
	if exists, err := store.Stat(path); err == nil && exists {
		// best-effort cleanup, non-critical：删除失败仅意味着断点文件残留，
		// 不会影响新任务（启动时按 taskId 重置 lastCheckpointPath）。
		_ = store.Remove(path)
	}
}

// ResumeFromCheckpoint 从断点恢复测试
// 与 Cursor DAQ 的 resumeFromCheckpoint 行为一致：
//  1. 复用原 taskId
//  2. 从 checkpoint.Config 恢复完整配置
//  3. 从 checkpoint.CompletedPoints 开始循环
func (m *TraversalManager) ResumeFromCheckpoint(cp traversal.Checkpoint) (string, error) {
	if cp.TaskID == "" {
		return "", fmt.Errorf("checkpoint taskId is required")
	}
	if cp.CompletedPoints < 0 || cp.CompletedPoints > cp.TotalPoints {
		return "", fmt.Errorf("checkpoint completedPoints out of range")
	}

	m.mu.RLock()
	currentState := m.status.State
	m.mu.RUnlock()
	if currentState == traversal.StateRunning || currentState == traversal.StatePaused {
		return "", fmt.Errorf("a traversal is already %s", currentState)
	}

	// 从断点的 Config 字段恢复完整配置
	var config traversal.Config
	if len(cp.Config) > 0 {
		if err := json.Unmarshal(cp.Config, &config); err != nil {
			return "", fmt.Errorf("parse checkpoint config: %w", err)
		}
	} else {
		return "", fmt.Errorf("checkpoint config is empty")
	}
	if len(config.Path) == 0 {
		return "", fmt.Errorf("checkpoint config path is empty")
	}
	if cp.CompletedPoints >= len(config.Path) {
		return "", fmt.Errorf("checkpoint already completed")
	}

	if err := resourcelock.Default().Acquire(traversalLockResource, cp.TaskID, 24*time.Hour); err != nil {
		return "", fmt.Errorf("acquire traversal lock: %w", err)
	}

	m.mu.Lock()
	m.config = config
	m.configRaw = append(json.RawMessage(nil), cp.Config...)
	m.isStopped = false
	m.isPaused = false
	m.motionPauseCancelled = false
	m.status = traversal.Status{
		TaskID:       cp.TaskID,
		State:        traversal.StateRunning,
		TotalPoints:  len(config.Path),
		CurrentPoint: cp.CompletedPoints, // 从已完成点数开始
		StartedAt:    cp.CreatedAt,
	}
	// 恢复已完成的点结果（从 store 中读取，若存在）
	if m.store != nil {
		if prev, ok := m.store.Get(cp.TaskID); ok && len(prev.Results) > 0 {
			m.status.Results = append([]traversal.PointResult(nil), prev.Results...)
		}
	}
	m.mu.Unlock()

	// 启动后台循环
	dwell := time.Duration(config.DwellTimeMs) * time.Millisecond
	if dwell <= 0 {
		dwell = 100 * time.Millisecond
	}
	go m.RunTraversalLoop(dwell)

	return cp.TaskID, nil
}

