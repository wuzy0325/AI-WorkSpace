// Package usecase — checkpoint 持久化相关方法（从 traversal.go 拆分）
//
// 包含：buildCheckpoint / saveCheckpoint / ClearCheckpoint。
// 这些方法都通过 ports.CheckpointStore 接口与文件系统交互，与采集主循环解耦后
// 便于测试与维护。
package usecase

import (
	"encoding/json"
	"math"
	"time"

	"shared.local/device-sdk/go/pkg/slog"

	"wind-daq/services/api-go/internal/core/traversal"
)

// buildCheckpoint 构造统一的 traversal.Checkpoint DTO（saveCheckpoint 与 commitPointV2 共享）。
//
// 抽出此 helper 的目的（Important-5）：
//   - 旧 saveCheckpoint 与 v2 commitPointV2 各自手写 traversal.Checkpoint 构造，
//     字段语义易分叉（如 CommittedPoints vs CommitSeq、TotalPoints 来源不一致）；
//   - 配置统一从 Snapshot.Config 持有（P0-C4），DTO 构造也必须走单一路径。
//
// 参数：
//   - taskID     任务标识
//   - snapshot   运行快照（Config/TotalPoints/CommittedPoints/CommitSeq/CSVPath/ResultLogPath）
//   - commitSeq  提交序号（权威水位；覆盖 snapshot.CommitSeq/CommittedPoints 与 cp.CompletedPoints，
//     保证三者严格一致：cp.Snapshot.CommitSeq == cp.Snapshot.CommittedPoints == cp.CompletedPoints）
//   - savePath   checkpoint 关联的 CSV 路径（cp.SavePath；通常 = snapshot.CSVPath）
//   - lastPoint  上次完成的点（可空，仅 saveCheckpoint 路径写入用于前端显示）
//   - state      checkpoint 状态（saveCheckpoint/commitPointV2 均用 StateRunning）
//   - probeID    managed 会话的 probe 身份（"" 表示 legacy）：非空时写入 v3
//     （DualCheckpointVersion + ProbeID；BoundControllerIDs 已在 snapshot 中随 Task 9 冻结），
//     为空时保持 v2（CheckpointVersion），版本语义不复用、不迁移（spec FR8）。
//
// 调用方仍需负责：lastPoint 的 NaN 清洗（saveCheckpoint 特有，commitPointV2 不写 lastPoint）。
// helper 不做该清洗，保持职责单一。
func buildCheckpoint(
	taskID string,
	snapshot traversal.TraversalRunSnapshot,
	commitSeq uint64,
	savePath string,
	lastPoint *traversal.Point,
	state traversal.State,
	probeID ProbeID,
) traversal.Checkpoint {
	version := traversal.CheckpointVersion
	probeIDStr := ""
	if probeID != "" {
		version = traversal.DualCheckpointVersion
		probeIDStr = string(probeID)
	}
	cp := traversal.Checkpoint{
		Version:         version,
		TaskID:          taskID,
		State:           state,
		Snapshot:        snapshot,
		CompletedPoints: int(commitSeq),
		TotalPoints:     snapshot.TotalPoints,
		LastPoint:       lastPoint,
		SavePath:        savePath,
		CreatedAt:       time.Now().UnixMilli(),
		ProbeID:         probeIDStr,
	}
	// 强制三者一致：调用方传入的 commitSeq 是权威水位，
	// 覆盖 snapshot 中可能滞后的值。commitPointV2 路径下 session.snapshot 中
	// CommittedPoints/CommitSeq 是上次提交的值，本次需更新为最新 commitSeq。
	cp.Snapshot.CommitSeq = commitSeq
	cp.Snapshot.CommittedPoints = int(commitSeq)
	return cp
}

// saveCheckpoint 保存断点到 ${savePath}.checkpoint.json，使用原子写入
// 与 Cursor DAQ 的 atomicWriteJson 行为一致：先写临时文件再 rename，避免半写入状态
func (m *TraversalManager) saveCheckpoint(points []traversal.Point, completedCount int, savePath string) {
	m.mu.RLock()
	config := m.config
	taskID := m.status.TaskID
	session := m.session
	m.mu.RUnlock()

	if taskID == "" || savePath == "" {
		return
	}
	// managed 会话的 probe 身份（v3 元数据来源）；legacy 为空（v2）。
	var probeID ProbeID
	var boundControllers []string
	if session != nil {
		if session.managedOpts != nil {
			probeID = session.managedOpts.ProbeID
		}
		boundControllers = session.snapshot.BoundControllerIDs
	}

	var lastPoint *traversal.Point
	if completedCount > 0 && completedCount <= len(points) {
		lp := points[completedCount-1]
		// 清洗 NaN：line/rectangle/sector 模式通过 markAxesNaN 将未配置轴标记为 NaN，
		// encoding/json 不支持 NaN 序列化会导致 checkpoint 写入失败。
		// LastPoint 仅用于显示"上次跑到哪个点"，恢复运动用 Config.Path 而非 LastPoint，
		// 所以 NaN→0 不影响恢复正确性（availableAxisTargets 仍按 Path 中的 NaN 跳过对应轴）。
		if math.IsNaN(lp.X) {
			lp.X = 0
		}
		if math.IsNaN(lp.Y) {
			lp.Y = 0
		}
		if math.IsNaN(lp.Z) {
			lp.Z = 0
		}
		if math.IsNaN(lp.U) {
			lp.U = 0
		}
		lastPoint = &lp
	}

	// 配置仅通过 Snapshot.Config 持有（P0-C4）：移除顶层 Config []byte 冗余字段，
	// 单源真相避免双份同步维护。
	snapshot := traversal.TraversalRunSnapshot{
		Config:             config,
		TotalPoints:        len(points),
		CommittedPoints:    completedCount,
		CommitSeq:          uint64(completedCount),
		CSVPath:            savePath,
		BoundControllerIDs: boundControllers,
	}
	// 通过 buildCheckpoint 统一构造 DTO（Important-5），与 commitPointV2 共享逻辑，
	// 保证字段语义一致（CompletedPoints/CommitSeq/Snapshot 三者对齐）。
	checkpoint := buildCheckpoint(taskID, snapshot, uint64(completedCount), savePath, lastPoint, traversal.StateRunning, probeID)

	data, err := json.MarshalIndent(checkpoint, "", "  ")
	if err != nil {
		return
	}

	// 检查点放在 CSV 同目录的 .traversal/ 隐藏子目录，避免用户看到内部文件。
	// 路径派生收敛到 ResolveCheckpointPathFromCSV 单一真相源，
	// 与 FileCheckpointPort.path() / activeIndex.Register / commitPointV2 fallback 保持一致。
	checkpointPath := traversal.ResolveCheckpointPathFromCSV(savePath)
	if m.checkpointStore == nil {
		return
	}
	if err := m.checkpointStore.Write(checkpointPath, data); err != nil {
		slog.Warn("traversal checkpoint save failed",
			"component", "traversal",
			"task_id", taskID,
			"path", checkpointPath,
			"completed_points", completedCount,
			"error", err,
		)
		return
	}

	m.mu.Lock()
	m.lastCheckpointPath = checkpointPath
	m.mu.Unlock()

	// managed 会话：通知 registry checkpoint 已落盘（记录路径供终态清理删除）。
	notifyManagedCheckpointSaved(session, checkpointPath)

	slog.Info("traversal checkpoint saved",
		"component", "traversal",
		"task_id", taskID,
		"path", checkpointPath,
		"completed_points", completedCount,
		"total_points", len(points),
	)
}

// ClearCheckpoint 删除断点文件并清空 lastCheckpointPath
// 测试成功完成后调用，避免残留断点文件
func (m *TraversalManager) ClearCheckpoint() {
	m.mu.Lock()
	path := m.lastCheckpointPath
	if path == "" && m.status.CSVPath != "" {
		path = traversal.ResolveCheckpointPathFromCSV(m.status.CSVPath)
	}
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
		slog.Info("traversal checkpoint cleared",
			"component", "traversal",
			"path", path,
		)
	}
}
