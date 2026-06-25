package usecase

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"wind-daq/services/api-go/internal/core/device"
	"wind-daq/services/api-go/internal/core/motion"
	"wind-daq/services/api-go/internal/core/traversal"
	"wind-daq/services/api-go/pkg/wiring"
)

// ===== 测试用 mock 实现 =====

// mockLatestDataReader 模拟最新数据读取器，返回固定的通道数据
type mockLatestDataReader struct {
	data device.DataPayload
}

func (r *mockLatestDataReader) GetLatestData(deviceID string) (device.DataPayload, bool) {
	r.data.DeviceID = deviceID
	return r.data, true
}

// mockMotionAccess 模拟运动控制访问，记录 MoveTo 调用并返回静止状态
type mockMotionAccess struct {
	moveToCalls []struct {
		id       string
		axis     motion.AxisName
		position float64
	}
}

func (m *mockMotionAccess) StatusAll(ctx context.Context) []motion.ControllerStatus {
	return []motion.ControllerStatus{
		{ID: "mc-1", Connected: true, Axes: []motion.AxisStatus{
			{Name: motion.AxisX, Position: 0, Homed: true, Moving: false},
			{Name: motion.AxisY, Position: 0, Homed: true, Moving: false},
			{Name: motion.AxisZ, Position: 0, Homed: true, Moving: false},
		}},
	}
}

func (m *mockMotionAccess) MoveTo(ctx context.Context, id string, axis motion.AxisName, position float64) error {
	m.moveToCalls = append(m.moveToCalls, struct {
		id       string
		axis     motion.AxisName
		position float64
	}{id, axis, position})
	return nil
}

func (m *mockMotionAccess) Stop(ctx context.Context, id string, axis motion.AxisName) error {
	return nil
}

// mockTraversalPointSink 模拟数据点写入器，记录所有写入的点
type mockTraversalPointSink struct {
	mu sync.Mutex // protects all fields — TraversalManager calls Finalize / Write
	// from its worker goroutine while tests inspect from the test goroutine.
	points    []traversal.PointResult
	initCount int
	finalized bool
}

func (s *mockTraversalPointSink) InitializeTraversal(_ traversal.Config) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.initCount++
	return nil
}

func (s *mockTraversalPointSink) WriteTraversalPoint(point traversal.PointResult) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.points = append(s.points, point)
	return nil
}

func (s *mockTraversalPointSink) FinalizeTraversal() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.finalized = true
	return nil
}

// snapshot copies the recorded state under the lock for test assertions.
func (s *mockTraversalPointSink) snapshot() (points []traversal.PointResult, initCount int, finalized bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	points = append([]traversal.PointResult(nil), s.points...)
	return points, s.initCount, s.finalized
}

// mockTraversalResultStore 模拟结果存储，支持 Save/Get
type mockTraversalResultStore struct {
	data map[string]traversal.Status
}

func newMockTraversalResultStore() *mockTraversalResultStore {
	return &mockTraversalResultStore{data: make(map[string]traversal.Status)}
}

func (s *mockTraversalResultStore) Save(taskID string, status traversal.Status) error {
	s.data[taskID] = status
	return nil
}

func (s *mockTraversalResultStore) Get(taskID string) (traversal.Status, bool) {
	st, ok := s.data[taskID]
	return st, ok
}

// ===== 辅助函数 =====

// newCheckpointTestManager 构造一个用于断点测试的 TraversalManager
func newCheckpointTestManager() (*TraversalManager, *mockMotionAccess, *mockTraversalPointSink, *mockTraversalResultStore) {
	reader := &mockLatestDataReader{data: device.DataPayload{Channels: []float64{1, 2, 3, 4, 5}}}
	motionAccess := &mockMotionAccess{}
	sink := &mockTraversalPointSink{}
	store := newMockTraversalResultStore()
	mgr := NewTraversalManager(reader, motionAccess, sink, store, wiring.NewFileCheckpointStore())
	return mgr, motionAccess, sink, store
}

// makeTestConfig 构造一个最小可用的遍历测试配置
func makeTestConfig(savePath string) traversal.Config {
	return traversal.Config{
		TaskID:          "trav-checkpoint-test",
		DeviceID:        "sim-1",
		Channels:        []int{0, 1, 2, 3, 4},
		Path:            []traversal.Point{{X: 0, Y: 0, Z: 0}, {X: 1, Y: 0, Z: 0}, {X: 2, Y: 0, Z: 0}},
		DwellTimeMs:     10,
		SamplesPerPoint: 1,
		SavePath:        savePath,
		SaveFileName:    "test-traversal",
	}
}

// ===== 测试用例 =====

// TestSaveAndLoadCheckpoint 验证 saveCheckpoint 写入文件后 LoadCheckpoint 能正确读回
func TestSaveAndLoadCheckpoint(t *testing.T) {
	mgr, _, _, _ := newCheckpointTestManager()
	tmpDir := t.TempDir()
	savePath := filepath.Join(tmpDir, "result.csv")

	// 初始化 manager 状态：设置 taskID 和 config
	config := makeTestConfig(savePath)
	mgr.mu.Lock()
	mgr.config = config
	mgr.status.TaskID = config.TaskID
	mgr.configRaw, _ = json.Marshal(config)
	mgr.mu.Unlock()

	points := config.Path
	// 模拟完成 2 个点后保存断点
	mgr.saveCheckpoint(points, 2, savePath)

	// 验证文件存在
	checkpointPath := savePath + ".checkpoint.json"
	if _, err := os.Stat(checkpointPath); err != nil {
		t.Fatalf("checkpoint file not created: %v", err)
	}

	// LoadCheckpoint 应返回正确的内容
	cp, err := mgr.LoadCheckpoint()
	if err != nil {
		t.Fatalf("LoadCheckpoint failed: %v", err)
	}
	if cp == nil {
		t.Fatal("LoadCheckpoint returned nil checkpoint")
	}
	if cp.TaskID != config.TaskID {
		t.Errorf("expected taskId %q, got %q", config.TaskID, cp.TaskID)
	}
	if cp.CompletedPoints != 2 {
		t.Errorf("expected completedPoints 2, got %d", cp.CompletedPoints)
	}
	if cp.TotalPoints != 3 {
		t.Errorf("expected totalPoints 3, got %d", cp.TotalPoints)
	}
	if cp.LastPoint == nil {
		t.Fatal("expected lastPoint non-nil")
	}
	if cp.LastPoint.X != 1 {
		t.Errorf("expected lastPoint.X 1, got %f", cp.LastPoint.X)
	}
	if cp.SavePath != savePath {
		t.Errorf("expected savePath %q, got %q", savePath, cp.SavePath)
	}

	// 验证 Config 字段可反序列化为原始配置
	var restoredConfig traversal.Config
	if err := json.Unmarshal(cp.Config, &restoredConfig); err != nil {
		t.Fatalf("unmarshal checkpoint config failed: %v", err)
	}
	if restoredConfig.TaskID != config.TaskID {
		t.Errorf("restored config taskId mismatch: %q vs %q", restoredConfig.TaskID, config.TaskID)
	}
	if len(restoredConfig.Path) != 3 {
		t.Errorf("restored config path length mismatch: %d", len(restoredConfig.Path))
	}
}

// TestLoadCheckpointEmptyPath 验证 lastCheckpointPath 为空时返回 nil 且无错误
func TestLoadCheckpointEmptyPath(t *testing.T) {
	mgr, _, _, _ := newCheckpointTestManager()
	cp, err := mgr.LoadCheckpoint()
	if err != nil {
		t.Fatalf("LoadCheckpoint should not return error: %v", err)
	}
	if cp != nil {
		t.Errorf("expected nil checkpoint, got %+v", cp)
	}
}

// TestLoadCheckpointFileDeleted 验证断点文件被外部删除后 LoadCheckpoint 返回 nil 且重置路径
func TestLoadCheckpointFileDeleted(t *testing.T) {
	mgr, _, _, _ := newCheckpointTestManager()
	tmpDir := t.TempDir()
	savePath := filepath.Join(tmpDir, "result.csv")

	config := makeTestConfig(savePath)
	mgr.mu.Lock()
	mgr.config = config
	mgr.status.TaskID = config.TaskID
	mgr.configRaw, _ = json.Marshal(config)
	mgr.mu.Unlock()

	mgr.saveCheckpoint(config.Path, 1, savePath)
	checkpointPath := savePath + ".checkpoint.json"

	// 外部删除文件
	if err := os.Remove(checkpointPath); err != nil {
		t.Fatalf("remove checkpoint file failed: %v", err)
	}

	// LoadCheckpoint 应返回 nil 且无错误，并重置 lastCheckpointPath
	cp, err := mgr.LoadCheckpoint()
	if err != nil {
		t.Fatalf("LoadCheckpoint should not return error: %v", err)
	}
	if cp != nil {
		t.Errorf("expected nil checkpoint after file deleted, got %+v", cp)
	}

	mgr.mu.RLock()
	path := mgr.lastCheckpointPath
	mgr.mu.RUnlock()
	if path != "" {
		t.Errorf("expected lastCheckpointPath reset to empty, got %q", path)
	}
}

// TestClearCheckpoint 验证 ClearCheckpoint 删除文件并清空路径
func TestClearCheckpoint(t *testing.T) {
	mgr, _, _, _ := newCheckpointTestManager()
	tmpDir := t.TempDir()
	savePath := filepath.Join(tmpDir, "result.csv")

	config := makeTestConfig(savePath)
	mgr.mu.Lock()
	mgr.config = config
	mgr.status.TaskID = config.TaskID
	mgr.configRaw, _ = json.Marshal(config)
	mgr.mu.Unlock()

	mgr.saveCheckpoint(config.Path, 1, savePath)
	checkpointPath := savePath + ".checkpoint.json"

	// 文件应存在
	if _, err := os.Stat(checkpointPath); err != nil {
		t.Fatalf("checkpoint file not created: %v", err)
	}

	mgr.ClearCheckpoint()

	// 文件应被删除
	if _, err := os.Stat(checkpointPath); !os.IsNotExist(err) {
		t.Errorf("expected checkpoint file deleted, got err=%v", err)
	}

	// 路径应被清空
	mgr.mu.RLock()
	path := mgr.lastCheckpointPath
	mgr.mu.RUnlock()
	if path != "" {
		t.Errorf("expected lastCheckpointPath empty, got %q", path)
	}
}

// TestClearCheckpointNoOp 验证 lastCheckpointPath 为空时 ClearCheckpoint 不报错
func TestClearCheckpointNoOp(t *testing.T) {
	mgr, _, _, _ := newCheckpointTestManager()
	// 不应 panic 或报错
	mgr.ClearCheckpoint()
}

// TestResumeFromCheckpoint 验证从断点恢复后状态正确
func TestResumeFromCheckpoint(t *testing.T) {
	mgr, _, _, store := newCheckpointTestManager()
	tmpDir := t.TempDir()
	savePath := filepath.Join(tmpDir, "result.csv")

	config := makeTestConfig(savePath)
	configRaw, _ := json.Marshal(config)

	// 预置 store 中已有 1 个已完成点的结果（模拟之前中断时的状态）
	prevStatus := traversal.Status{
		TaskID:       config.TaskID,
		CurrentPoint: 1,
		Results: []traversal.PointResult{
			{PointIndex: 0, Point: config.Path[0], Values: map[int]float64{0: 1.0}},
		},
	}
	store.Save(config.TaskID, prevStatus)

	cp := traversal.Checkpoint{
		TaskID:          config.TaskID,
		Config:          configRaw,
		CompletedPoints: 1,
		TotalPoints:     3,
		LastPoint:       &config.Path[0],
		SavePath:        savePath,
		CreatedAt:       1700000000000,
	}

	taskID, err := mgr.ResumeFromCheckpoint(cp)
	if err != nil {
		t.Fatalf("ResumeFromCheckpoint failed: %v", err)
	}
	if taskID != config.TaskID {
		t.Errorf("expected taskID %q, got %q", config.TaskID, taskID)
	}

	// 验证状态
	status := mgr.Status()
	if status.State != traversal.StateRunning {
		t.Errorf("expected state running, got %q", status.State)
	}
	if status.CurrentPoint != 1 {
		t.Errorf("expected currentPoint 1, got %d", status.CurrentPoint)
	}
	if status.TotalPoints != 3 {
		t.Errorf("expected totalPoints 3, got %d", status.TotalPoints)
	}
	// 应恢复已完成的点结果
	if len(status.Results) != 1 {
		t.Errorf("expected 1 restored result, got %d", len(status.Results))
	}

	// 停止后台循环，避免泄漏 goroutine
	_ = mgr.Stop()
}

// TestResumeFromCheckpointInvalid 验证无效断点返回错误
func TestResumeFromCheckpointInvalid(t *testing.T) {
	mgr, _, _, _ := newCheckpointTestManager()

	tests := []struct {
		name string
		cp   traversal.Checkpoint
		err  string
	}{
		{
			name: "empty taskId",
			cp:   traversal.Checkpoint{CompletedPoints: 0, TotalPoints: 3, Config: json.RawMessage(`{}`)},
			err:  "checkpoint taskId is required",
		},
		{
			name: "empty config",
			cp:   traversal.Checkpoint{TaskID: "t1", CompletedPoints: 0, TotalPoints: 3},
			err:  "checkpoint config is empty",
		},
		{
			name: "completedPoints out of range (negative)",
			cp:   traversal.Checkpoint{TaskID: "t1", CompletedPoints: -1, TotalPoints: 3, Config: json.RawMessage(`{}`)},
			err:  "checkpoint completedPoints out of range",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := mgr.ResumeFromCheckpoint(tt.cp)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if err.Error() != tt.err {
				t.Errorf("expected error %q, got %q", tt.err, err.Error())
			}
		})
	}
}

// TestResumeFromCheckpointAlreadyCompleted 验证已完成点数等于总点数时返回错误
func TestResumeFromCheckpointAlreadyCompleted(t *testing.T) {
	mgr, _, _, _ := newCheckpointTestManager()
	tmpDir := t.TempDir()
	savePath := filepath.Join(tmpDir, "result.csv")

	config := makeTestConfig(savePath)
	configRaw, _ := json.Marshal(config)

	cp := traversal.Checkpoint{
		TaskID:          config.TaskID,
		Config:          configRaw,
		CompletedPoints: 3, // 等于总点数
		TotalPoints:     3,
		SavePath:        savePath,
	}

	_, err := mgr.ResumeFromCheckpoint(cp)
	if err == nil {
		t.Fatal("expected error for already completed checkpoint")
	}
	if err.Error() != "checkpoint already completed" {
		t.Errorf("expected 'checkpoint already completed', got %q", err.Error())
	}
}

// TestSaveCheckpointAtomicWrite 验证断点文件使用原子写入（无残留 .tmp 文件）
func TestSaveCheckpointAtomicWrite(t *testing.T) {
	mgr, _, _, _ := newCheckpointTestManager()
	tmpDir := t.TempDir()
	savePath := filepath.Join(tmpDir, "result.csv")

	config := makeTestConfig(savePath)
	mgr.mu.Lock()
	mgr.config = config
	mgr.status.TaskID = config.TaskID
	mgr.configRaw, _ = json.Marshal(config)
	mgr.mu.Unlock()

	mgr.saveCheckpoint(config.Path, 1, savePath)

	// 验证 .tmp 文件不存在（原子写入完成后应被 rename 掉）
	tmpPath := savePath + ".checkpoint.json.tmp"
	if _, err := os.Stat(tmpPath); !os.IsNotExist(err) {
		t.Errorf("expected .tmp file to not exist, got err=%v", err)
	}

	// 验证最终文件存在且内容可解析
	checkpointPath := savePath + ".checkpoint.json"
	data, err := os.ReadFile(checkpointPath)
	if err != nil {
		t.Fatalf("read checkpoint file failed: %v", err)
	}
	var cp traversal.Checkpoint
	if err := json.Unmarshal(data, &cp); err != nil {
		t.Fatalf("unmarshal checkpoint failed: %v", err)
	}
}
