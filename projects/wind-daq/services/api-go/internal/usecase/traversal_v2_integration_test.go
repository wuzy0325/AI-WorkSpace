package usecase

import (
	"bytes"
	"context"
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"wind-daq/services/api-go/internal/adapters/storage"
	"wind-daq/services/api-go/internal/core/device"
	"wind-daq/services/api-go/internal/core/traversal"
	"wind-daq/services/api-go/internal/ports"
	"wind-daq/services/api-go/pkg/wiring"
)

// newV2IntegrationManager 构造注入真实 v2 端口的 TraversalManager。
//
// 用于验证 Resume 路径下 openReliabilityPorts 的端到端集成（Critical-2 / Critical-3 回归网）。
// 与 newCheckpointTestManager 的差异：
//   - 注入真实 csvPort / resultLogPort / checkpointPortFactory / activeIndex
//   - sink 仍用 mock，与 csvPort 不同实例，走独立 sink.InitializeTraversal 路径
//   - tmpDir 由调用方管理（与 activeIndex 的 dataDir 必须一致，validatePath 才能通过）
func newV2IntegrationManager(t *testing.T, tmpDir string) *TraversalManager {
	t.Helper()
	reader := &mockLatestDataReader{data: device.DataPayload{Channels: []float64{1, 2, 3, 4, 5}}}
	motionAccess := &mockMotionAccess{}
	sink := &mockTraversalPointSink{}
	store := newMockTraversalResultStore()
	checkpointStore := wiring.NewFileCheckpointStore()
	mgr := NewTraversalManager(reader, motionAccess, sink, store, checkpointStore)

	csvPort := storage.NewTraversalCsvWriter()
	resultLogPort := storage.NewTraversalResultLog()
	factory := storage.NewFileCheckpointPortFactory(checkpointStore)
	activeIndex := storage.NewTraversalActiveIndex(filepath.Join(tmpDir, "active.json"), tmpDir)

	mgr.SetCsvPort(csvPort)
	mgr.SetResultLogPort(resultLogPort)
	mgr.SetCheckpointPortFactory(factory)
	mgr.SetActiveIndex(activeIndex)

	return mgr
}

// makeV2Result 构造一个最小可用的 PointResult，CommitSeq 由调用方指定。
// 用于 v2 端口集成测试的写入数据。
func makeV2Result(taskID string, commitSeq uint64) traversal.PointResult {
	return traversal.PointResult{
		TaskID:     taskID,
		PointIndex: int(commitSeq) - 1,
		CommitSeq:  commitSeq,
		Timestamp:  int64(commitSeq) * 1000,
		Point:      traversal.Point{X: float64(commitSeq)},
		Values:     map[int]float64{0: float64(commitSeq), 1: 1, 2: 2, 3: 3, 4: 4},
		SampleCount: 1,
		Calculated:  &traversal.CalculatedResult{Valid: true, Alpha: 0.1, Beta: 0.2, Pt: 100, Ps: 90, Mach: 0.3},
	}
}

// v2ChannelLabels v2 集成测试统一使用的通道标签（5 通道 → P1..P5）
var v2ChannelLabels = map[int]string{0: "P1", 1: "P2", 2: "P3", 3: "P4", 4: "P5"}

// splitNonEmptyLines 按换行符切分字节流，丢弃空行（如末尾 trailing newline）。
// 用于断言 CSV / 结果日志的行数。
func splitNonEmptyLines(data []byte) [][]byte {
	var lines [][]byte
	for _, line := range bytes.Split(data, []byte{'\n'}) {
		if len(line) > 0 {
			lines = append(lines, line)
		}
	}
	return lines
}

// TestResumeReliabilityPorts_TruncatesResultLogTailToCommittedSeq 验证 Resume 模式下
// openReliabilityPorts 对结果日志执行 ValidateTail + TruncateAfter，丢弃崩溃前未 Sync 的
// 半写入记录（commitSeq=3），让水位严格对齐 snapshot.CommitSeq=2（Critical-2 回归网）。
//
// 测试前置：
//   - tempdir 准备 csvPath / logPath
//   - csvPort Open(Create) + Append 3 行（commitSeq=1,2,3）+ Sync + Close
//   - resultLogPort Open(Create) + AppendPrepared 3 行（commitSeq=1,2,3）+ Sync + Close
//   - snapshot.CommitSeq = 2
//   - beginSession
//
// 测试步骤：
//   - 调用 openReliabilityPorts(session, Resume, snapshot, config)
//
// 期待结果：
//   - 不返回 error
//   - result log 文件行数 = 2（commitSeq=3 的半写入被 ValidateTail+TruncateAfter 丢弃）
//   - CSV 文件非空行数 = 3（BOM+表头 + 2 数据，第 3 行被 csvPort.Open Resume 内部截断）
func TestResumeReliabilityPorts_TruncatesResultLogTailToCommittedSeq(t *testing.T) {
	tmpDir := t.TempDir()
	csvPath := filepath.Join(tmpDir, "result.csv")
	logPath := csvPath + ".results.jsonl"
	ctx := context.Background()

	// 前置 1：csvPort 写入 3 行（模拟崩溃前 commitSeq=3 已写入 CSV 但 checkpoint 未提交）
	csvWriter := storage.NewTraversalCsvWriter()
	createSession := ports.TraversalOutputSession{
		TaskID:        "task-v2-trunc",
		Mode:          ports.TraversalOutputCreate,
		Path:          csvPath,
		Channels:      []int{0, 1, 2, 3, 4},
		ChannelLabels: v2ChannelLabels,
	}
	if err := csvWriter.Open(ctx, createSession); err != nil {
		t.Fatalf("csv open create: %v", err)
	}
	for seq := uint64(1); seq <= 3; seq++ {
		if _, err := csvWriter.Append(ctx, makeV2Result("task-v2-trunc", seq)); err != nil {
			t.Fatalf("csv append seq=%d: %v", seq, err)
		}
	}
	if err := csvWriter.Sync(ctx); err != nil {
		t.Fatalf("csv sync: %v", err)
	}
	if err := csvWriter.Close(ctx); err != nil {
		t.Fatalf("csv close: %v", err)
	}

	// 前置 2：resultLogPort 写入 3 行（commitSeq=3 模拟崩溃前 AppendPrepared 已完成但
	// Checkpoint 阶段未执行的半提交记录）
	resultLog := storage.NewTraversalResultLog()
	if err := resultLog.Open(ctx, ports.TraversalOutputSession{
		TaskID: "task-v2-trunc", Mode: ports.TraversalOutputCreate, Path: logPath,
	}); err != nil {
		t.Fatalf("result log open create: %v", err)
	}
	for seq := uint64(1); seq <= 3; seq++ {
		if err := resultLog.AppendPrepared(ctx, makeV2Result("task-v2-trunc", seq)); err != nil {
			t.Fatalf("result log append seq=%d: %v", seq, err)
		}
	}
	if err := resultLog.Sync(ctx); err != nil {
		t.Fatalf("result log sync: %v", err)
	}
	if err := resultLog.Close(ctx); err != nil {
		t.Fatalf("result log close: %v", err)
	}

	// 前置 3：构造 manager 注入真实 v2 端口
	mgr := newV2IntegrationManager(t, tmpDir)

	// 前置 4：snapshot.CommitSeq=2（checkpoint 已提交到 commitSeq=2，第 3 行是崩溃前半提交）
	config := traversal.Config{
		TaskID:          "task-v2-trunc",
		DeviceID:        "sim-1",
		Channels:        []int{0, 1, 2, 3, 4},
		Path:            []traversal.Point{{X: 0}, {X: 1}, {X: 2}},
		ChannelLabels:   v2ChannelLabels,
		DwellTimeMs:     10,
		SamplesPerPoint: 1,
		SavePath:        csvPath,
		SaveFileName:    "result",
	}
	snapshot := traversal.TraversalRunSnapshot{
		Config:          config,
		TotalPoints:     3,
		CommittedPoints: 2,
		CommitSeq:       2,
		CSVPath:         csvPath,
		ResultLogPath:   logPath,
	}

	session, err := mgr.beginSession(context.Background(), config.TaskID, snapshot)
	if err != nil {
		t.Fatalf("beginSession: %v", err)
	}

	// 测试步骤：调用 openReliabilityPorts(Resume)
	if err := mgr.openReliabilityPorts(session, ports.TraversalOutputResume, snapshot, config); err != nil {
		t.Fatalf("openReliabilityPorts resume: %v", err)
	}

	// 期待结果 1：result log 行数 = 2（commitSeq=3 被截断）
	logData, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read result log: %v", err)
	}
	logLines := splitNonEmptyLines(logData)
	if len(logLines) != 2 {
		t.Errorf("expected result log 2 lines after truncate, got %d", len(logLines))
	}

	// 期待结果 2：CSV 非空行数 = 3（BOM+表头 + 2 数据，第 3 行被 csvPort.Open Resume 内部截断）
	csvData, err := os.ReadFile(csvPath)
	if err != nil {
		t.Fatalf("read csv: %v", err)
	}
	csvLines := splitNonEmptyLines(csvData)
	if len(csvLines) != 3 {
		t.Errorf("expected csv 3 non-empty lines (header + 2 data), got %d", len(csvLines))
	}

	// 关闭端口（避免临时文件残留 / 句柄泄漏）
	_ = mgr.csvPort.Close(ctx)
	_ = mgr.resultLogPort.Close(ctx)
}

// TestResumeReliabilityPorts_RejectsGapInResultLog 验证 ValidateTail 检测到
// CommitSeq 缺口时返回错误，避免水位漂移后继续追加。
//
// 测试前置：
//   - csv 写 commitSeq=1,2（2 行）
//   - log 写 commitSeq=1,3（缺 2，模拟日志损坏）
//
// 测试步骤：
//   - 调用 openReliabilityPorts(session, Resume, snapshot{CommitSeq=2}, config)
//
// 期待结果：
//   - 返回 error（ValidateTail 检测到 "提交序号缺口: got=3 want=2"）
func TestResumeReliabilityPorts_RejectsGapInResultLog(t *testing.T) {
	tmpDir := t.TempDir()
	csvPath := filepath.Join(tmpDir, "result.csv")
	logPath := csvPath + ".results.jsonl"
	ctx := context.Background()

	// 前置 1：csv 写 commitSeq=1,2
	csvWriter := storage.NewTraversalCsvWriter()
	if err := csvWriter.Open(ctx, ports.TraversalOutputSession{
		TaskID: "task-gap", Mode: ports.TraversalOutputCreate, Path: csvPath,
		Channels: []int{0, 1, 2, 3, 4}, ChannelLabels: v2ChannelLabels,
	}); err != nil {
		t.Fatalf("csv open: %v", err)
	}
	if _, err := csvWriter.Append(ctx, makeV2Result("task-gap", 1)); err != nil {
		t.Fatalf("csv append 1: %v", err)
	}
	if _, err := csvWriter.Append(ctx, makeV2Result("task-gap", 2)); err != nil {
		t.Fatalf("csv append 2: %v", err)
	}
	_ = csvWriter.Sync(ctx)
	_ = csvWriter.Close(ctx)

	// 前置 2：log 写 commitSeq=1,3（缺 2）
	resultLog := storage.NewTraversalResultLog()
	if err := resultLog.Open(ctx, ports.TraversalOutputSession{
		TaskID: "task-gap", Mode: ports.TraversalOutputCreate, Path: logPath,
	}); err != nil {
		t.Fatalf("log open: %v", err)
	}
	_ = resultLog.AppendPrepared(ctx, makeV2Result("task-gap", 1))
	_ = resultLog.AppendPrepared(ctx, makeV2Result("task-gap", 3)) // 缺 2
	_ = resultLog.Sync(ctx)
	_ = resultLog.Close(ctx)

	// 测试步骤：Resume snapshot.CommitSeq=2，期望 ValidateTail 检测到缺口
	mgr := newV2IntegrationManager(t, tmpDir)
	config := traversal.Config{
		TaskID: "task-gap", Channels: []int{0, 1, 2, 3, 4},
		Path:          []traversal.Point{{X: 0}, {X: 1}, {X: 2}},
		SavePath:      csvPath,
		ChannelLabels: v2ChannelLabels,
	}
	snapshot := traversal.TraversalRunSnapshot{
		Config: config, TotalPoints: 3, CommittedPoints: 2, CommitSeq: 2,
		CSVPath: csvPath, ResultLogPath: logPath,
	}
	session, err := mgr.beginSession(context.Background(), config.TaskID, snapshot)
	if err != nil {
		t.Fatalf("beginSession: %v", err)
	}

	err = mgr.openReliabilityPorts(session, ports.TraversalOutputResume, snapshot, config)
	if err == nil {
		t.Fatal("expected ValidateTail to reject corrupted log, got nil error")
	}
	// ValidateTail 对缺口/不足均会报错。日志写 commitSeq=1,3（缺 2）但 snapshot.CommitSeq=2，
	// 实际触发"提交记录不足: got=1 want=2"（commitSeq=3 超出 want=2 范围被忽略，最终记录数不匹配）。
	// 接受"不足"或"缺口"任一关键字，都表明日志水位损坏。
	errMsg := err.Error()
	if !strings.Contains(errMsg, "不足") && !strings.Contains(errMsg, "缺口") {
		t.Errorf("expected insufficiency or gap error, got: %v", err)
	}
	// 关闭已打开的端口（csvPort.Open 先成功，ValidateTail 失败前已打开）
	_ = mgr.csvPort.Close(ctx)
	_ = mgr.resultLogPort.Close(ctx)
}

// TestResumeReliabilityPorts_AppliesColumnConfigInHeader 验证 v2 Open 路径
// 应用 SaveOptions/Channels/ChannelLabels 构建完整表头（Critical-3 回归网）。
//
// 测试前置：
//   - 用 Channels=[0..4] + ChannelLabels={0:P1..4:P5} 创建 CSV
//
// 测试步骤：
//   - 读回 CSV 文件，断言表头包含 P1..P5 列
//   - openReliabilityPorts(Resume) 不报错（openResumeLocked 表头匹配）
//
// 期待结果：
//   - CSV 表头包含 P1, P2, P3, P4, P5（Critical-3：列配置在 Open 路径已应用）
//   - Resume 不返回 error（openResumeLocked 的 records[0] vs w.header 校验通过）
func TestResumeReliabilityPorts_AppliesColumnConfigInHeader(t *testing.T) {
	tmpDir := t.TempDir()
	csvPath := filepath.Join(tmpDir, "result.csv")
	logPath := csvPath + ".results.jsonl"
	ctx := context.Background()

	// 前置：用完整列配置创建 CSV，并写 1 行 commitSeq=1
	csvWriter := storage.NewTraversalCsvWriter()
	if err := csvWriter.Open(ctx, ports.TraversalOutputSession{
		TaskID: "task-col", Mode: ports.TraversalOutputCreate, Path: csvPath,
		Channels: []int{0, 1, 2, 3, 4}, ChannelLabels: v2ChannelLabels,
	}); err != nil {
		t.Fatalf("csv open: %v", err)
	}
	if _, err := csvWriter.Append(ctx, makeV2Result("task-col", 1)); err != nil {
		t.Fatalf("csv append: %v", err)
	}
	_ = csvWriter.Sync(ctx)
	_ = csvWriter.Close(ctx)

	// 前置 2：result log 同步写 1 行 commitSeq=1（ValidateTail snapshot.CommitSeq=1 需要匹配）
	resultLog := storage.NewTraversalResultLog()
	if err := resultLog.Open(ctx, ports.TraversalOutputSession{
		TaskID: "task-col", Mode: ports.TraversalOutputCreate, Path: logPath,
	}); err != nil {
		t.Fatalf("log open: %v", err)
	}
	if err := resultLog.AppendPrepared(ctx, makeV2Result("task-col", 1)); err != nil {
		t.Fatalf("log append: %v", err)
	}
	_ = resultLog.Sync(ctx)
	_ = resultLog.Close(ctx)

	// 期待结果 1：CSV 表头包含 P1..P5 列
	csvData, err := os.ReadFile(csvPath)
	if err != nil {
		t.Fatalf("read csv: %v", err)
	}
	csvStr := string(csvData)
	for _, want := range []string{"P1", "P2", "P3", "P4", "P5"} {
		if !strings.Contains(csvStr, want) {
			t.Errorf("csv header missing column %q (Critical-3: column config not applied in Open path)", want)
		}
	}

	// 测试步骤：Resume 时配置与创建时一致，openResumeLocked 表头校验应通过
	mgr := newV2IntegrationManager(t, tmpDir)
	config := traversal.Config{
		TaskID: "task-col", Channels: []int{0, 1, 2, 3, 4},
		Path:          []traversal.Point{{X: 0}, {X: 1}},
		SavePath:      csvPath,
		ChannelLabels: v2ChannelLabels,
	}
	snapshot := traversal.TraversalRunSnapshot{
		Config: config, TotalPoints: 2, CommittedPoints: 1, CommitSeq: 1,
		CSVPath: csvPath, ResultLogPath: logPath,
	}
	session, err := mgr.beginSession(context.Background(), config.TaskID, snapshot)
	if err != nil {
		t.Fatalf("beginSession: %v", err)
	}
	if err := mgr.openReliabilityPorts(session, ports.TraversalOutputResume, snapshot, config); err != nil {
		t.Fatalf("openReliabilityPorts resume: %v", err)
	}
	_ = mgr.csvPort.Close(ctx)
	_ = mgr.resultLogPort.Close(ctx)
}

// TestResumeFromCheckpoint_LegacySavePathIsDirectory 验证 ResumeFromCheckpoint 对旧格式
// checkpoint（SavePath=目录、Snapshot.CSVPath=""）的回退路径已用 ResolveOutputPath 重算，
// 不再把目录当文件传给 csvPort.Open（残留 bug 回归网）。
//
// 测试前置：
//   - tmpDir 作为数据目录（旧格式 checkpoint 的 SavePath 存的是目录）
//   - 手动构造旧格式 traversal.Checkpoint：
//       SavePath = tmpDir（目录，模拟 v2 之前的格式）
//       Snapshot.CSVPath = ""（旧格式没有此字段）
//       Snapshot.Config.SavePath = tmpDir（目录）+ SaveFileName = "result"
//       CompletedPoints = 1, TotalPoints = 3, CommitSeq = 1
//   - 同步在 tmpDir 内预先创建 result.csv（含 BOM+表头+1 行数据）与 result.results.jsonl
//     （1 行 commitSeq=1），模拟旧版本写入的真实数据文件
//     路径派生须与 core/traversal.ResolveResultLogPath 保持一致：
//     csvPath=result.csv → 去 .csv 后缀 → 加 .results.jsonl → result.results.jsonl
//
// 测试步骤：
//   - 调用 mgr.ResumeFromCheckpoint(cp)
//
// 期待结果：
//   - 不返回 "is a directory" 错误
//   - 不返回其他 error
//   - snapshot.CSVPath 经 ResolveOutputPath 解析为 tmpDir/result.csv
//   - csvPort.Open(Resume) 成功打开并截断到 commitSeq=1
//   - checkpointPortFactory.Create 基于解析后的 CSVPath 派生 .checkpoint.json，
//     路径落在 tmpDir 内（而非父目录的 "tmpDir.checkpoint.json"）
func TestResumeFromCheckpoint_LegacySavePathIsDirectory(t *testing.T) {
	tmpDir := t.TempDir()
	csvPath := filepath.Join(tmpDir, "result.csv")
	// logPath 必须与 traversal.ResolveResultLogPath 派生规则一致：
	// csvPath 去 .csv 后缀加 .results.jsonl → result.results.jsonl
	// 不能写 csvPath + ".results.jsonl"（会变成 result.csv.results.jsonl，
	// 与 ResumeFromCheckpoint 内 ResolveResultLogPath 的产物不匹配，Open 时报"找不到文件"）
	logPath := filepath.Join(tmpDir, "result.results.jsonl")
	ctx := context.Background()

	// 前置 1：预创建 CSV（BOM+表头+1 行 commitSeq=1），模拟旧版本写入的数据文件
	csvWriter := storage.NewTraversalCsvWriter()
	if err := csvWriter.Open(ctx, ports.TraversalOutputSession{
		TaskID: "task-legacy", Mode: ports.TraversalOutputCreate, Path: csvPath,
		Channels: []int{0, 1, 2, 3, 4}, ChannelLabels: v2ChannelLabels,
	}); err != nil {
		t.Fatalf("csv open create: %v", err)
	}
	if _, err := csvWriter.Append(ctx, makeV2Result("task-legacy", 1)); err != nil {
		t.Fatalf("csv append: %v", err)
	}
	_ = csvWriter.Sync(ctx)
	_ = csvWriter.Close(ctx)

	// 前置 2：预创建 result log（1 行 commitSeq=1）
	resultLog := storage.NewTraversalResultLog()
	if err := resultLog.Open(ctx, ports.TraversalOutputSession{
		TaskID: "task-legacy", Mode: ports.TraversalOutputCreate, Path: logPath,
	}); err != nil {
		t.Fatalf("log open create: %v", err)
	}
	if err := resultLog.AppendPrepared(ctx, makeV2Result("task-legacy", 1)); err != nil {
		t.Fatalf("log append: %v", err)
	}
	_ = resultLog.Sync(ctx)
	_ = resultLog.Close(ctx)

	// 前置 3：构造旧格式 checkpoint——SavePath 是目录，Snapshot.CSVPath 为空
	config := traversal.Config{
		TaskID:          "task-legacy",
		DeviceID:        "sim-1",
		Channels:        []int{0, 1, 2, 3, 4},
		ChannelLabels:   v2ChannelLabels,
		Path:            []traversal.Point{{X: 0}, {X: 1}, {X: 2}},
		DwellTimeMs:     10,
		SamplesPerPoint: 1,
		SavePath:        tmpDir, // 旧格式：目录
		SaveFileName:    "result",
	}
	cp := traversal.Checkpoint{
		Version:         0, // 旧格式无版本号
		TaskID:          config.TaskID,
		State:           traversal.StateRunning,
		CompletedPoints: 1,
		TotalPoints:     3,
		SavePath:        tmpDir, // 旧格式：目录（不是文件路径）
		CreatedAt:       1,
		// Snapshot.CSVPath / ResultLogPath 留空——旧格式没有这两个字段
	}
	cp.Snapshot.Config = config
	cp.Snapshot.TotalPoints = 3
	cp.Snapshot.CommittedPoints = 1
	cp.Snapshot.CommitSeq = 1

	// 测试步骤：调 ResumeFromCheckpoint
	mgr := newV2IntegrationManager(t, tmpDir)
	if _, err := mgr.ResumeFromCheckpoint(cp); err != nil {
		// 关键断言：不能因目录被当文件而报 "is a directory"
		if strings.Contains(err.Error(), "is a directory") {
			t.Fatalf("ResumeFromCheckpoint regressed: directory treated as file: %v", err)
		}
		t.Fatalf("ResumeFromCheckpoint failed: %v", err)
	}

	// 期待结果 1：active.json 内 task-legacy 记录的 checkpointPath 落在 tmpDir 内。
	// 旧代码用 cp.SavePath（目录）派生会让 checkpointPath 落到父目录的 "tmpDir.checkpoint.json"，
	// activeIndex.Register 的 validatePath 会拒绝，Tasks 不会记录该 task。
	// 修复后用 snapshot.CSVPath（ResolveOutputPath 重算的文件路径）派生，
	// 落在 tmpDir/result.csv.checkpoint.json，validatePath 通过，Tasks 记录该 task。
	activeData, err := os.ReadFile(filepath.Join(tmpDir, "active.json"))
	if err != nil {
		t.Fatalf("read active index: %v", err)
	}
	var active struct {
		Version int               `json:"version"`
		Tasks   map[string]string `json:"tasks"`
	}
	if err := json.Unmarshal(activeData, &active); err != nil {
		t.Fatalf("parse active index: %v", err)
	}
	got, ok := active.Tasks["task-legacy"]
	if !ok {
		t.Fatalf("active index missing task-legacy entry (validatePath rejected checkpoint path — #3 fix regressed)")
	}
	expectedCheckpoint, err := filepath.Abs(csvPath + ".checkpoint.json")
	if err != nil {
		t.Fatalf("abs checkpoint path: %v", err)
	}
	if got != expectedCheckpoint {
		t.Errorf("active index checkpoint path: got=%s want=%s", got, expectedCheckpoint)
	}

	// 期待结果 2：父目录残留 checkpoint 不存在
	// 旧代码用 cp.SavePath 派生会落到父目录的 "tmpDir.checkpoint.json"，
	// 修复后路径严格落在 tmpDir 内。此处作为残留路径检查。
	parentCheckpoint := tmpDir + ".checkpoint.json"
	if _, err := os.Stat(parentCheckpoint); err == nil {
		t.Errorf("checkpoint incorrectly created at parent directory: %s (旧 savePath 语义残留)", parentCheckpoint)
	}

	// 清理：让 loop 退出，避免持续日志噪声与后台 goroutine 泄漏
	_ = mgr.Stop()
}

// TestCommitPointV2_SanitizesNaNInPointResult 验证 commitPointV2 三阶段提交在 PointResult
// 含 NaN 时的处理，覆盖 line/rectangle/sector 模式 markAxesNaN 场景。
//
// 测试前置：
//   - tmpDir 准备 csvPath / logPath
//   - newV2IntegrationManager 注入真实 v2 端口
//   - config.Path 用 line 模式 Point（Y/Z/U = NaN，模拟 markAxesNaN）
//   - beginSession + openReliabilityPorts(Create) 打开 CSV / result log
//
// 测试步骤：
//   - 构造 PointResult：Point.Y/Z/U = NaN（markAxesNaN 语义），Calculated.Alpha = NaN（设备异常）
//   - 调用 mgr.commitPointV2(taskID, &result)
//
// 期待结果：
//   - 不返回 error（三阶段提交全部通过，无 "unsupported value: NaN"）
//   - result.Point.Y/Z/U 保持 NaN（运动恢复语义依据，由 Point.MarshalJSON 序列化为 null）
//   - result.Calculated.Alpha 已被清洗为 0（设备层异常防御性清洗，无 null 契约）
//   - result log 文件能被 json.Unmarshal 解析，Point 字段为 null 而非 NaN
//   - 反序列化后 Point.Y/Z/U 还原为 NaN（null → NaN 往返一致性）
func TestCommitPointV2_SanitizesNaNInPointResult(t *testing.T) {
	tmpDir := t.TempDir()
	csvPath := filepath.Join(tmpDir, "result.csv")
	logPath := filepath.Join(tmpDir, "result.results.jsonl")
	ctx := context.Background()

	// 前置 1：构造 manager 注入真实 v2 端口
	mgr := newV2IntegrationManager(t, tmpDir)

	// 前置 2：line 模式 config（Y/Z/U = NaN，模拟 markAxesNaN）
	config := traversal.Config{
		TaskID:          "task-nan",
		DeviceID:        "sim-1",
		Channels:        []int{0, 1, 2, 3, 4},
		ChannelLabels:   v2ChannelLabels,
		Path:            []traversal.Point{{X: 0, Y: math.NaN(), Z: math.NaN(), U: math.NaN()}},
		DwellTimeMs:     10,
		SamplesPerPoint: 1,
		SavePath:        tmpDir,
		SaveFileName:    "result",
	}
	snapshot := traversal.TraversalRunSnapshot{
		Config:          config,
		TotalPoints:     1,
		CommittedPoints: 0,
		CommitSeq:       0,
		CSVPath:         csvPath,
		ResultLogPath:   logPath,
	}
	session, err := mgr.beginSession(ctx, config.TaskID, snapshot)
	if err != nil {
		t.Fatalf("beginSession: %v", err)
	}
	// Create 模式打开 CSV / result log
	if err := mgr.openReliabilityPorts(session, ports.TraversalOutputCreate, snapshot, config); err != nil {
		t.Fatalf("openReliabilityPorts create: %v", err)
	}

	// 前置 3：构造含 NaN 的 PointResult（模拟 RunCurrentPoint 阶段4 的 result）
	// Point.Y/Z/U = NaN（与 config.Path 一致，line 模式未配置轴，运动恢复语义依据）
	// Calculated.Alpha = NaN（模拟插值器极端输入，防御性清洗验证）
	result := traversal.PointResult{
		TaskID:      config.TaskID,
		CommitSeq:   1,
		PointStatus: traversal.PointStatusCompleted,
		PointIndex:  0,
		Point:       traversal.Point{X: 0, Y: math.NaN(), Z: math.NaN(), U: math.NaN()},
		Timestamp:   1000,
		Values:      map[int]float64{0: 1, 1: 2, 2: 3, 3: 4, 4: 5},
		SampleCount: 1,
		Calculated: &traversal.CalculatedResult{
			Valid: true,
			Alpha: math.NaN(),
			Beta:  0.2,
			Pt:    100,
			Ps:    90,
			Mach:  0.3,
		},
	}

	// 测试步骤：调 commitPointV2（三阶段提交）
	if err := mgr.commitPointV2(config.TaskID, &result); err != nil {
		if strings.Contains(err.Error(), "unsupported value: NaN") {
			t.Fatalf("commitPointV2 regressed: NaN leaked to json.Marshal: %v", err)
		}
		t.Fatalf("commitPointV2 failed: %v", err)
	}

	// 期待结果 1：result.Point.Y/Z/U 保持 NaN（Point.MarshalJSON 负责序列化为 null，
	// 不在入口清洗——运动恢复仍需 NaN 语义让 availableAxisTargets 跳过这些轴）
	if !math.IsNaN(result.Point.Y) || !math.IsNaN(result.Point.Z) || !math.IsNaN(result.Point.U) {
		t.Errorf("expected Point.Y/Z/U to remain NaN (motion-recovery semantic), got Y=%v Z=%v U=%v",
			result.Point.Y, result.Point.Z, result.Point.U)
	}
	// 期待结果 2：Calculated.Alpha 已被清洗为 0（设备层异常防御性清洗，无 null 契约）
	if result.Calculated.Alpha != 0 {
		t.Errorf("expected Calculated.Alpha = 0 after sanitize, got %v", result.Calculated.Alpha)
	}

	// 期待结果 3：result log 文件能被 json.Unmarshal 解析（Point NaN 序列化为 null）
	logData, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read result log: %v", err)
	}
	lines := splitNonEmptyLines(logData)
	if len(lines) != 1 {
		t.Fatalf("expected 1 line in result log, got %d", len(lines))
	}
	var record struct {
		Version int                   `json:"version"`
		Phase   string                `json:"phase"`
		Result  traversal.PointResult `json:"result"`
	}
	if err := json.Unmarshal(lines[0], &record); err != nil {
		t.Fatalf("result log line not parseable (NaN leaked?): %v", err)
	}

	// 期待结果 4：反序列化后 Point.Y/Z/U 还原为 NaN（null → NaN 往返一致）
	// 这是运动恢复的关键：checkpoint / result log 经序列化→反序列化后，
	// availableAxisTargets 仍能通过 math.IsNaN 跳过这些轴。
	if !math.IsNaN(record.Result.Point.Y) || !math.IsNaN(record.Result.Point.Z) || !math.IsNaN(record.Result.Point.U) {
		t.Errorf("expected Point.Y/Z/U to round-trip as NaN (null→NaN), got Y=%v Z=%v U=%v",
			record.Result.Point.Y, record.Result.Point.Z, record.Result.Point.U)
	}
	// 期待结果 5：Calculated.Alpha 反序列化后是 0（被入口清洗过，再序列化为 0）
	if record.Result.Calculated.Alpha != 0 {
		t.Errorf("expected Calculated.Alpha = 0 in log (sanitized before serialize), got %v",
			record.Result.Calculated.Alpha)
	}

	// 清理：关闭 v2 端口
	_ = mgr.csvPort.Close(ctx)
	_ = mgr.resultLogPort.Close(ctx)
}
