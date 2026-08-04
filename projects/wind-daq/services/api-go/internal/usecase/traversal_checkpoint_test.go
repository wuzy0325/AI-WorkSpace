package usecase

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"wind-daq/services/api-go/internal/core/device"
	"wind-daq/services/api-go/internal/core/motion"
	"wind-daq/services/api-go/internal/core/traversal"
	"wind-daq/services/api-go/pkg/wiring"
)

// resolveTestCheckpointPath 返回测试用的 checkpoint 路径，与 FileCheckpointPort.path() 派生规则一致。
// 路径格式：${dir}/.traversal/${stem}.checkpoint.json
func resolveTestCheckpointPath(savePath string) string {
	ext := filepath.Ext(savePath)
	base := strings.TrimSuffix(savePath, ext)
	dir := filepath.Dir(base)
	stem := filepath.Base(base)
	return filepath.Join(dir, ".traversal", stem+".checkpoint.json")
}

// ===== 测试用 mock 实现 =====

// mockLatestDataReader 模拟最新数据读取器，返回固定的通道数据
type mockLatestDataReader struct {
	data device.DataPayload
}

func (r *mockLatestDataReader) GetLatestData(deviceID string) (device.DataPayload, bool) {
	r.data.DeviceID = deviceID
	return r.data, true
}

func (r *mockLatestDataReader) GetLatestTimestamp(_ string) (int64, bool) { return 0, false }

// mockMotionAccess 模拟运动控制访问，记录 MoveTo 调用并返回静止状态。
//
// statuses 字段用于覆写 StatusAll 返回值：未设置时回退默认（单个 Connected=true
// 控制器），设置后原样返回。新测试用例（如"运动控制器未连接"前置检查）通过
// 显式注入 statuses 模拟断开场景，原有测试不设置该字段即可保持旧行为。
//
// statusSequence 字段用于按顺序返回不同的状态快照——运动安全测试需要模拟
// "运动中→到位"、"运动中→越过目标"、"运动中→卡死"等多帧场景。设置后优先于 statuses。
//
// emergencyStopCalls / stopCalls 记录急停与普通停止调用，用于运动安全测试验证。
type mockMotionAccess struct {
	statuses []motion.ControllerStatus
	// statusSequence 按顺序返回的状态快照序列；每次 StatusAll 弹出队首。
	// 用于模拟跨样本的运动状态变化（如位置随时间推移而变化）。
	// 优先级高于 statuses；为空时回退到 statuses 或默认值。
	statusSequence [][]motion.ControllerStatus
	moveToCalls    []struct {
		id       string
		axis     motion.AxisName
		position float64
	}
	stopCalls []struct {
		id   string
		axis motion.AxisName
	}
	emergencyStopCalls []string
	moveToErr          error
	stopErr            error
	// emergencyStopErr 注入 EmergencyStop 调用错误（用于测试急停失败 fallback）
	emergencyStopErr error
}

func (m *mockMotionAccess) StatusAll(ctx context.Context) []motion.ControllerStatus {
	// 优先消费序列——运动安全测试需要按帧推进状态
	if len(m.statusSequence) > 0 {
		next := m.statusSequence[0]
		m.statusSequence = m.statusSequence[1:]
		return next
	}
	if m.statuses != nil {
		return m.statuses
	}
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
	return m.moveToErr
}

func (m *mockMotionAccess) Stop(ctx context.Context, id string, axis motion.AxisName) error {
	m.stopCalls = append(m.stopCalls, struct {
		id   string
		axis motion.AxisName
	}{id, axis})
	if m.stopErr == nil {
		for controllerIndex := range m.statuses {
			if m.statuses[controllerIndex].ID != id {
				continue
			}
			for axisIndex := range m.statuses[controllerIndex].Axes {
				if m.statuses[controllerIndex].Axes[axisIndex].Name == axis {
					m.statuses[controllerIndex].Axes[axisIndex].Moving = false
				}
			}
		}
	}
	return m.stopErr
}

// EmergencyStop 记录急停调用并返回注入错误（若有）
func (m *mockMotionAccess) EmergencyStop(ctx context.Context, id string) error {
	m.emergencyStopCalls = append(m.emergencyStopCalls, id)
	return m.emergencyStopErr
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

// TestSaveCheckpoint 验证 saveCheckpoint 写盘内容完整（任务标识/进度/快照配置）
func TestSaveCheckpoint(t *testing.T) {
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
	checkpointPath := resolveTestCheckpointPath(savePath)
	if _, err := os.Stat(checkpointPath); err != nil {
		t.Fatalf("checkpoint file not created: %v", err)
	}

	// 直接读取 checkpoint 文件校验写盘内容
	data, err := os.ReadFile(checkpointPath)
	if err != nil {
		t.Fatalf("read checkpoint file failed: %v", err)
	}
	var cp traversal.Checkpoint
	if err := json.Unmarshal(data, &cp); err != nil {
		t.Fatalf("unmarshal checkpoint failed: %v", err)
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

	// 验证 Snapshot.Config 可还原为原始配置（P0-C4：Config 单源真相）
	restoredConfig := cp.Snapshot.Config
	if restoredConfig.TaskID != config.TaskID {
		t.Errorf("restored config taskId mismatch: %q vs %q", restoredConfig.TaskID, config.TaskID)
	}
	if len(restoredConfig.Path) != 3 {
		t.Errorf("restored config path length mismatch: %d", len(restoredConfig.Path))
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
	checkpointPath := resolveTestCheckpointPath(savePath)

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
	checkpointPath := resolveTestCheckpointPath(savePath)
	data, err := os.ReadFile(checkpointPath)
	if err != nil {
		t.Fatalf("read checkpoint file failed: %v", err)
	}
	var cp traversal.Checkpoint
	if err := json.Unmarshal(data, &cp); err != nil {
		t.Fatalf("unmarshal checkpoint failed: %v", err)
	}
}
