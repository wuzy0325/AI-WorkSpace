package usecase

import (
	"context"
	"fmt"

	"shared.local/device-sdk/go/pkg/slog"

	"wind-daq/services/api-go/internal/core/traversal"
)

// registry probe-scoped checkpoint 收尾（原 Task 11 恢复 façade 的留存部分）。
//
// 停止后恢复功能已移除（HTTP 端点 loadCheckpoint/resumeFromCheckpoint/clearCheckpoint、
// 前端断点横幅、registry LoadCheckpoint/ResumeFromCheckpoint/ClearCheckpoint façade
// 均已删除）：运行期不再向 dual recovery index 登记任何映射，checkpoint 仅是
// 运行期提交锚点，终态后不再作为可恢复工作暴露。
//
// 终态清理由 completeRecoveryMapping 在每个终态（completed/stopped/error）执行：
// 注销本 probe 的全部残留映射（含旧版本以其它 taskID 登记的遗留项）并删除
// checkpoint 文件。dual 路径不读写 legacy traversal-active-index.json
// （manager ownership 分支保证）。

// checkpointSavedCallbackFor 生成 session 的 CheckpointSavedCallback：仅记录
// 运行期 checkpoint 路径供终态清理删除文件，不把运行登记为可恢复任务。
func (r *ManagerRegistry) checkpointSavedCallbackFor(session *registrySession) func(string) {
	return func(checkpointPath string) {
		r.mu.Lock()
		session.recoveryPath = checkpointPath
		r.mu.Unlock()
	}
}

// completeRecoveryMapping 终态清理恢复映射与 checkpoint 文件（每个终态都执行，
// runSessionCleanup 调用）。停止后恢复功能已移除：checkpoint 仅是运行期提交锚点，
// 终态后不再作为可恢复工作暴露。
//
// 注销按索引实际登记的 taskID 进行（adapter 要求 taskID 与登记候选一致），而非
// 本 session 的 taskID：旧版本可能以其它 taskID 残留映射（"停止后保留可恢复任务"
// 时代的遗留索引，含问题电脑的索引-文件不一致脏状态），直接以 session.taskID
// 注销会被 adapter 拒绝，导致每个终态都 completion_failed、probe 被永久占用。
func (r *ManagerRegistry) completeRecoveryMapping(session *registrySession) error {
	ctx := context.Background()
	r.mu.Lock()
	recoveryPath := session.recoveryPath
	r.mu.Unlock()
	if recoveryPath == "" {
		status := session.manager.Status()
		if status.CSVPath != "" {
			recoveryPath = traversal.ResolveCheckpointPathFromCSV(status.CSVPath)
		}
	}
	ref, found, err := r.recoveryIndex.Find(ctx, string(session.probeID))
	if err != nil {
		return fmt.Errorf("查询双探针恢复索引失败: %w", err)
	}
	if found {
		if err := r.recoveryIndex.Unregister(ctx, string(session.probeID), ref.TaskID); err != nil {
			return fmt.Errorf("注销 dual recovery 映射失败: %w", err)
		}
		// 残留映射对应的旧 checkpoint 文件一并清理（best-effort，残留仅浪费磁盘）。
		if r.checkpointStore != nil && ref.Path != recoveryPath {
			if err := r.checkpointStore.Remove(ref.Path); err != nil {
				slog.Warn("删除旧版本残留 checkpoint 失败", "path", ref.Path, "error", err)
			}
		}
	}
	if recoveryPath != "" && r.checkpointStore != nil {
		if err := r.checkpointStore.Remove(recoveryPath); err != nil {
			slog.Warn("删除终态 traversal checkpoint 失败", "path", recoveryPath, "error", err)
		}
	}
	return nil
}
