// Package usecase — checkpoint 持久化相关方法（从 traversal.go 拆分）
//
// 包含：LoadCheckpoint / saveCheckpoint / ClearCheckpoint / ResumeFromCheckpoint。
// 这些方法都通过 ports.CheckpointStore 接口与文件系统交互，与采集主循环解耦后
// 便于测试与维护。
package usecase

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"time"

	"wind-daq/services/api-go/internal/core/traversal"
	"wind-daq/services/api-go/internal/ports"
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
	// legacy 路由不读 dual v3（spec FR8：不自动迁移、不误加载）。
	// v1/v2 解码行为保持不变。
	var header struct {
		Version int `json:"version"`
	}
	if err := json.Unmarshal(data, &header); err == nil && header.Version == traversal.DualCheckpointVersion {
		return nil, fmt.Errorf("%w: legacy 路径不读取 dual v3 checkpoint", ports.ErrCheckpointVersionMismatch)
	}
	var cp traversal.Checkpoint
	if err := json.Unmarshal(data, &cp); err != nil {
		return nil, fmt.Errorf("parse checkpoint: %w", err)
	}
	return &cp, nil
}

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
	// 单源真相避免双份同步维护。ResumeFromCheckpoint 直接从 Snapshot.Config 还原。
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

	// managed 会话：通知 registry checkpoint 已落盘（dual recovery index 登记时机：
	// 映射存在 ⟺ checkpoint 文件存在）。
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

// ResumeFromCheckpoint legacy single 断点恢复入口：保持既有 lease ownership
// （自行 Acquire/Release 全局 workflow lease）。registry-managed 路径使用 ResumeManaged。
//
// 与 Start 的差异：
//   - 配置来源：从 cp.Snapshot.Config 还原，而非前端传入
//   - 起始点：CurrentPoint = cp.CompletedPoints，跳过已完成点
//   - v2 端口：CSV/结果日志以 TraversalOutputResume 模式 Open，
//     传入 CommittedSeq = cp.Snapshot.CommitSeq 让 CSV 截断到已提交水位，
//     避免崩溃前未提交的"半写入"行污染恢复后的数据
//   - CommittedPoints/CommitSeq：从 cp.Snapshot 继承，保证后续 commitPointV2
//     生成的 CommitSeq 严格递增（commitPointV2 会 +1 后再使用）
//
// 错误回滚与 Start 一致：使用 abortStartLocked 统一关闭已打开的端口、
// 释放工作流锁、设置错误状态。任何步骤失败都保证状态机回到 Idle/Error。
func (m *TraversalManager) ResumeFromCheckpoint(cp traversal.Checkpoint) (string, error) {
	return m.resumeInternal(cp, nil)
}

// resumeInternal ResumeFromCheckpoint/ResumeManaged 共享的恢复实现。
// ownership 语义与 startInternal 一致：opts == nil 为 legacy（Acquire + legacy
// activeIndex），opts != nil 为 managed（不触碰 workflow lease 与 legacy index，
// opts 冻结进 session 快照）。两条路径都启动后台 RunTraversalLoop。
func (m *TraversalManager) resumeInternal(cp traversal.Checkpoint, opts *ManagedSessionOptions) (string, error) {
	managed := opts != nil
	slog.Info("traversal resuming from checkpoint",
		"component", "traversal",
		"task_id", cp.TaskID,
		"completed_points", cp.CompletedPoints,
		"total_points", cp.TotalPoints,
	)

	if cp.TaskID == "" {
		err := fmt.Errorf("checkpoint taskId is required")
		slog.Error("traversal checkpoint resume failed", "component", "traversal", "error", err)
		return "", err
	}
	if cp.CompletedPoints < 0 || cp.CompletedPoints > cp.TotalPoints {
		err := fmt.Errorf("checkpoint completedPoints out of range")
		slog.Error("traversal checkpoint resume failed", "component", "traversal", "task_id", cp.TaskID, "error", err)
		return "", err
	}

	m.mu.RLock()
	currentState := m.status.State
	m.mu.RUnlock()
	if currentState == traversal.StateRunning || currentState == traversal.StatePaused {
		err := fmt.Errorf("a traversal is already %s", currentState)
		slog.Error("traversal checkpoint resume failed", "component", "traversal", "task_id", cp.TaskID, "error", err)
		return "", err
	}

	// 从 Snapshot.Config 还原配置（P0-C4：单一真相源）
	var config traversal.Config
	if cp.Snapshot.Config.TaskID != "" {
		config = cp.Snapshot.Config
	} else {
		err := fmt.Errorf("checkpoint config is empty")
		slog.Error("traversal checkpoint resume failed", "component", "traversal", "task_id", cp.TaskID, "error", err)
		return "", err
	}
	if len(config.Path) == 0 {
		err := fmt.Errorf("checkpoint config path is empty")
		slog.Error("traversal checkpoint resume failed", "component", "traversal", "task_id", cp.TaskID, "error", err)
		return "", err
	}
	if cp.CompletedPoints >= len(config.Path) {
		err := fmt.Errorf("checkpoint already completed")
		slog.Error("traversal checkpoint resume failed", "component", "traversal", "task_id", cp.TaskID, "error", err)
		return "", err
	}
	// 旧 checkpoint 可能保存了矩形/线型布局的隐藏 Z/U 绑定；恢复前按路径语义清理。
	config.MotionAxes = motionAxesForPath(config.MotionAxes, config.Path)

	// v2：基于 cp.Snapshot 构建恢复期 snapshot。
	// CommittedPoints / CommitSeq 沿用 checkpoint 中的值，确保 commitPointV2 后续 +1 严格递增。
	// CSVPath / ResultLogPath 优先用 snapshot 中的（v2 写入）；
	// 为空时（旧格式 checkpoint，SavePath 可能是目录）必须通过 ResolveOutputPath 重算，
	// 禁止直接用 cp.SavePath 当文件路径——否则 csvPort.Open 会把目录当文件创建，
	// O_EXCL 报 "is a directory"，Resume 起不来。
	snapshot := cp.Snapshot
	snapshot.Config = config
	if snapshot.CSVPath == "" {
		snapshot.CSVPath = traversal.ResolveOutputPath(config)
	}
	if snapshot.ResultLogPath == "" {
		snapshot.ResultLogPath = traversal.ResolveResultLogPath(config)
	}
	// 旧 checkpoint 无 BoundControllerIDs 字段：按还原配置重算冻结（与旧行为一致——
	// 停机目标本来就是从该配置读取的绑定集合）。
	if len(snapshot.BoundControllerIDs) == 0 {
		snapshot.BoundControllerIDs = boundControllerIDs(config)
	}

	// v2：beginSession 活动会话门禁 + resetFinalizeOnce 武装关闭流程
	parentCtx := context.Background()
	session, err := m.beginSession(parentCtx, cp.TaskID, snapshot)
	if err != nil {
		slog.Error("traversal checkpoint resume failed", "component", "traversal", "task_id", cp.TaskID, "error", err)
		return "", err
	}
	// ownership mode 冻结为 session 快照字段（同一 session 不得混用两种模式）。
	session.managedOpts = opts
	m.resetFinalizeOnce()

	if !managed {
		if err := m.lockService.Acquire(traversalLockResource, cp.TaskID, 24*time.Hour); err != nil {
			// 锁获取失败：回滚 session 状态，不调 abortStartLocked（端口尚未打开）
			session.Cancel()
			session.MarkDone()
			slog.Error("traversal checkpoint resume failed", "component", "traversal", "task_id", cp.TaskID, "error", err)
			return "", fmt.Errorf("acquire traversal lock: %w", err)
		}
	}

	// snapshot.MotionSafety 为 nil（旧 checkpoint 或前端未配置）时填充默认值。
	// 这里的填充只作用于本次恢复后的运行态 m.config，不回写 checkpoint 文件，
	// 避免老 checkpoint 在恢复时被静默改写。下游 EvaluateMotionSafety / Resolve
	// 虽然也能处理 nil，但显式填充让"使用 DefaultMotionSafety"语义可观察、可单测。
	//
	// 关键不变量：m.config.MotionSafety 始终来源于 cp.Snapshot.Config，
	// 不重新读取前端当前配置——避免前端在崩溃后修改配置导致恢复行为漂移。
	if config.MotionSafety == nil {
		defaultSafety := traversal.DefaultMotionSafety()
		config.MotionSafety = &defaultSafety
	}

	m.mu.Lock()
	m.config = config
	// 重新序列化 Snapshot.Config 得到 configRaw，用于持久化精确还原前端原始 JSON。
	// 失败时降级为空切片：不会阻塞恢复，只影响后续 SaveConfigRaw 的回写精度。
	if raw, mErr := json.Marshal(config); mErr == nil {
		m.configRaw = append(json.RawMessage(nil), raw...)
	} else {
		m.configRaw = nil
	}
	m.isStopped = false
	m.isPaused = false
	m.status = traversal.Status{
		TaskID:          cp.TaskID,
		State:           traversal.StateRunning,
		TotalPoints:     len(config.Path),
		CurrentPoint:    cp.CompletedPoints, // 从已完成点数开始
		CommittedPoints: int(snapshot.CommitSeq),
		StartedAt:       cp.CreatedAt,
	}
	// 恢复已完成的点结果（从 store 中读取，若存在）
	if m.store != nil {
		if prev, ok := m.store.Get(cp.TaskID); ok && len(prev.Results) > 0 {
			m.status.Results = append([]traversal.PointResult(nil), prev.Results...)
		}
	}
	// 快照 v2 端口（可能为 nil，表示未注入 v2 组件）
	// csvPort 用于 sinkIsCsvPort 同实例检测；resultLogPort 由 openReliabilityPorts 内部读取，
	// 不在此处快照，避免未使用变量。
	csvPort := m.csvPort
	activeIndex := m.activeIndex
	checkpointPortFactory := m.checkpointPortFactory
	sink := m.sink
	m.mu.Unlock()

	// v2：通过工厂按 snapshot.CSVPath 动态创建 checkpointPort（每个任务路径不同）。
	// 用解析后的 CSVPath 而非 cp.SavePath：旧格式 checkpoint 的 SavePath 可能是目录，
	// factory.Create 会基于该路径派生 .checkpoint.json，目录会让派生路径错乱。
	// snapshot.CSVPath 已在上方通过 ResolveOutputPath 重算，保证是完整文件路径。
	var checkpointPort ports.TraversalCheckpointPort
	if checkpointPortFactory != nil && snapshot.CSVPath != "" {
		var cpErr error
		checkpointPort, cpErr = checkpointPortFactory.Create(session.ctx, snapshot.CSVPath)
		if cpErr != nil {
			m.abortStartLocked(session, cp.TaskID, fmt.Sprintf("create checkpoint port: %v", cpErr), traversal.ErrSaveFailed)
			return "", cpErr
		}
		m.mu.Lock()
		m.checkpointPort = checkpointPort
		m.mu.Unlock()
	}

	// v2 存储初始化：CSV 与结果日志以 Resume 模式 Open，helper 内部对结果日志
	// 执行 ValidateTail + TruncateAfter，让水位严格对齐 CommittedSeq
	// （CSV 截断由 csvPort.Open 内部根据 CommittedSeq 完成）。
	// openReliabilityPorts 内部会在撞名 -2/-3 时回写 session.snapshot.CSVPath / ResultLogPath，
	// 调用方在函数返回后用 session.snapshot.CSVPath 即可拿到实际落盘路径。
	if err := m.openReliabilityPorts(session, ports.TraversalOutputResume, config); err != nil {
		m.abortStartLocked(session, cp.TaskID, err.Error(), traversal.ErrSaveFailed)
		return "", err
	}
	// 同步实际落盘 CSV 路径到 m.status.CSVPath：
	// Resume 模式 csvPort.Open 不会撞名（不创建新文件），session.snapshot.CSVPath
	// 即为恢复目标文件路径。同步到 status 让前端侧边栏显示与 Start 一致的真实路径，
	// 而非 checkpoint 中可能为旧格式的 SavePath。
	actualCSVPath := session.snapshot.CSVPath
	if actualCSVPath != "" {
		m.mu.Lock()
		m.status.CSVPath = actualCSVPath
		m.mu.Unlock()
	}
	// 注册活动索引，支持进程重启发现（仅 legacy single 路径；
	// managed 双探针使用 dual recovery index，由 registry 负责登记，spec FR3/FR4）。
	// checkpointPath 派生规则收敛到 ResolveCheckpointPathFromCSV 单一真相源，
	// 与 FileCheckpointPort.path() / saveCheckpoint / commitPointV2 fallback 保持一致。
	// 用 session.snapshot.CSVPath（撞名回写后的实际路径）派生，避免与实际 CSV stem 错位。
	if !managed && activeIndex != nil && checkpointPort != nil {
		checkpointPath := traversal.ResolveCheckpointPathFromCSV(session.snapshot.CSVPath)
		if err := activeIndex.Register(session.ctx, cp.TaskID, checkpointPath); err != nil {
			slog.Warn("traversal active index register failed",
				"component", "traversal", "task_id", cp.TaskID, "error", err)
			// 非阻塞：索引注册失败不影响任务启动，仅影响重启发现
		}
	}

	// 在锁外调用旧 sink.Initialize（向后兼容）。
	// sink 与 csvPort 是同一实例时跳过：csvPort.Open 已完成文件初始化，
	// 再次 InitializeTraversal 会触发双重初始化防御（P1-I6）返回错误。
	if sink != nil && !sinkIsCsvPort(sink, csvPort) {
		if err := sink.InitializeTraversal(config); err != nil {
			m.abortStartLocked(session, cp.TaskID, fmt.Sprintf("sink init failed: %v", err), traversal.ErrSaveFailed)
			return "", err
		}
	}

	// 启动后台循环
	go m.RunTraversalLoop()

	slog.Info("traversal checkpoint resume success",
		"component", "traversal",
		"task_id", cp.TaskID,
		"resume_from", cp.CompletedPoints,
		"total_points", cp.TotalPoints,
	)
	return cp.TaskID, nil
}
