package usecase

import (
	"testing"

	"windlabx4/services/api-go/internal/core/device"
	"windlabx4/services/api-go/internal/core/traversal"
	"windlabx4/services/api-go/pkg/wiring"
)

// === 遍历测试自定义布点 per-point 配置测试 ===
//
// 覆盖 spec §3 中的 4 个场景：
//  1. test=false 跳过分支：走到位置后不采集不保存，写入 PointStatusNotTested 行
//  2. per-point DwellMs 覆盖全局 DwellTimeMs（waitForStabilization 内部应用）
//  3. per-point Samples 覆盖全局 SamplesPerPoint（collectAveragedSamples 应用）
//  4. nil 字段回退全局默认值（既有行为不回归）
//
// 这些测试与 traversal_acquisition_test.go 中现有的
// TestRunCurrentPointSkipLastPersistsCompletedState 共用 mock 装配，
// 仅关注 per-point 字段的传递与应用是否正确。

// 测试前置：单点 custom 布局，point.Test=false；用 countingLatestDataReader 计数 reader 调用
// 测试步骤：Start + RunCurrentPoint，检查 sink 收到的结果 + reader 调用次数
// 期待结果：写入 PointStatusNotTested，Values=nil，SampleCount=0，CurrentPoint 推进到末点导致 Completed；
//          reader.GetLatestData 调用次数=0（test=false 完全不触发采集）
func TestRunCurrentPoint_PerPointTestFalse_SkipsAcquisition(t *testing.T) {
	// countingLatestDataReader：每次调用 Channels 递增，便于通过 calls==0 断言"未采集"
	reader := &countingLatestDataReader{}
	motionAccess := &mockMotionAccess{}
	sink := &mockTraversalPointSink{}
	store := newMockTraversalResultStore()
	manager := NewTraversalManager(reader, motionAccess, sink, store, wiring.NewFileCheckpointStore())

	testFalse := false
	config := traversal.Config{
		TaskID:          "trav-perpoint-test-false",
		DeviceID:        "sim-1",
		Channels:        []int{0, 1, 2, 3, 4},
		ChannelLabels:   map[int]string{0: "P1", 1: "P2", 2: "P3", 3: "P4", 4: "P5"},
		// X/Y/Z/U 全为 0：mockMotionAccess 默认返回 position=0，避免触发 motion safety deviation
		// 关注点是 per-point Test=false 跳过分支，与运动目标位置无关
		Path:            []traversal.Point{{X: 0, Y: 0, Z: 0, U: 0, Test: &testFalse}},
		DwellTimeMs:     1,
		SamplesPerPoint: 3,
		SavePath:        t.TempDir(),
		SaveFileName:    "perpoint-test-false",
	}
	if err := manager.Start(config); err != nil {
		t.Fatalf("Start returned error: %v", err)
	}
	t.Cleanup(func() { _ = manager.Stop() })

	if err := manager.RunCurrentPoint(); err != nil {
		t.Fatalf("RunCurrentPoint returned error: %v", err)
	}

	// 期待结果：sink 收到一条 PointStatusNotTested，Values=nil，SampleCount=0
	pts, _, _ := sink.snapshot()
	if len(pts) != 1 {
		t.Fatalf("sink points len=%d, want 1 (test=false 仍占一行)", len(pts))
	}
	got := pts[0]
	if got.PointStatus != traversal.PointStatusNotTested {
		t.Fatalf("PointStatus = %q, want %q", got.PointStatus, traversal.PointStatusNotTested)
	}
	if got.Values != nil {
		t.Fatalf("Values = %v, want nil (test=false 不采集)", got.Values)
	}
	if got.SampleCount != 0 {
		t.Fatalf("SampleCount = %d, want 0 (test=false 不采集)", got.SampleCount)
	}
	// 显式断言 reader 调用次数为 0：test=false 分支完全跳过 collectAveragedSamples
	// 防止未来在跳过分支误加 reader 调用（如预热缓存）而测试通过
	if reader.calls != 0 {
		t.Fatalf("GetLatestData calls = %d, want 0 (test=false 完全不触发采集)", reader.calls)
	}
	// 推进到末点 → 整体 Completed
	if manager.Status().State != traversal.StateCompleted {
		t.Fatalf("state = %q, want %q (test=false 末点也要进入完成态)",
			manager.Status().State, traversal.StateCompleted)
	}
	// PointStatusNotTested.IsCommitted()=true，崩溃恢复时不会重走该点
	if !got.PointStatus.IsCommitted() {
		t.Fatalf("PointStatusNotTested.IsCommitted() should be true to avoid re-walk on resume")
	}
}

// 测试前置：mock reader 记录 GetLatestData 调用次数；per-point Samples=2 覆盖全局 SamplesPerPoint=5
// 测试步骤：Start + RunCurrentPoint，检查 reader 调用次数
// 期待结果：collectAveragedSamples 调用次数 = per-point Samples，不是全局 SamplesPerPoint
func TestRunCurrentPoint_PerPointSamples_OverridesGlobal(t *testing.T) {
	// 计数 reader：每次调用 Channels 不同（递增），便于检查调用次数
	counter := &countingLatestDataReader{}
	motionAccess := &mockMotionAccess{}
	sink := &mockTraversalPointSink{}
	store := newMockTraversalResultStore()
	manager := NewTraversalManager(counter, motionAccess, sink, store, wiring.NewFileCheckpointStore())

	perPointSamples := 2
	config := traversal.Config{
		TaskID:          "trav-perpoint-samples",
		DeviceID:        "sim-1",
		Channels:        []int{0},
		ChannelLabels:   map[int]string{0: "P1"},
		// X/Y/Z/U 全为 0：避免触发 motion safety deviation
		Path:            []traversal.Point{{X: 0, Y: 0, Z: 0, U: 0, Samples: &perPointSamples}},
		DwellTimeMs:     1,
		SamplesPerPoint: 5, // 全局 5；若 per-point 未生效会调用 5 次
		SavePath:        t.TempDir(),
		SaveFileName:    "perpoint-samples",
	}
	if err := manager.Start(config); err != nil {
		t.Fatalf("Start returned error: %v", err)
	}
	t.Cleanup(func() { _ = manager.Stop() })

	if err := manager.RunCurrentPoint(); err != nil {
		t.Fatalf("RunCurrentPoint returned error: %v", err)
	}

	// 期待结果：reader 调用次数应等于 per-point Samples=2（而非全局 5）
	// collectAveragedSamples 每次循环对每个设备调用一次 GetLatestData
	if counter.calls != perPointSamples {
		t.Fatalf("GetLatestData calls = %d, want %d (per-point Samples=%d, global=%d)",
			counter.calls, perPointSamples, perPointSamples, config.SamplesPerPoint)
	}
	// 末点完成 → Completed
	if manager.Status().State != traversal.StateCompleted {
		t.Fatalf("state = %q, want %q", manager.Status().State, traversal.StateCompleted)
	}
	// SampleCount 反映 per-point Samples
	pts, _, _ := sink.snapshot()
	if len(pts) != 1 {
		t.Fatalf("sink points len=%d, want 1", len(pts))
	}
	if pts[0].SampleCount != perPointSamples {
		t.Fatalf("SampleCount = %d, want %d", pts[0].SampleCount, perPointSamples)
	}
}

// 测试前置：per-point DwellMs=10 覆盖全局 DwellTimeMs=1000
// 测试步骤：测量 waitForStabilization 内部等待时长
// 期待结果：实际等待 ≈ 10ms（远小于 1000ms），证明 per-point 生效
func TestRunCurrentPoint_PerPointDwellMs_OverridesGlobal(t *testing.T) {
	reader := &mockLatestDataReader{data: device.DataPayload{Channels: []float64{1.0}}}
	motionAccess := &mockMotionAccess{}
	sink := &mockTraversalPointSink{}
	store := newMockTraversalResultStore()
	manager := NewTraversalManager(reader, motionAccess, sink, store, wiring.NewFileCheckpointStore())

	perPointDwellMs := 10 // 远小于全局 1000ms，便于用时长区分
	config := traversal.Config{
		TaskID:          "trav-perpoint-dwell",
		DeviceID:        "sim-1",
		Channels:        []int{0},
		ChannelLabels:   map[int]string{0: "P1"},
		// X/Y/Z/U 全为 0：避免触发 motion safety deviation
		Path:            []traversal.Point{{X: 0, Y: 0, Z: 0, U: 0, DwellMs: &perPointDwellMs}},
		DwellTimeMs:     1000, // 全局 1000ms；若 per-point 未生效会卡 1s
		SamplesPerPoint: 1,
		SavePath:        t.TempDir(),
		SaveFileName:    "perpoint-dwell",
	}
	if err := manager.Start(config); err != nil {
		t.Fatalf("Start returned error: %v", err)
	}
	t.Cleanup(func() { _ = manager.Stop() })

	if err := manager.RunCurrentPoint(); err != nil {
		t.Fatalf("RunCurrentPoint returned error: %v", err)
	}

	// 期待结果：sink PointResult.DwellTimeElapsed == 10（per-point 优先于全局）
	pts, _, _ := sink.snapshot()
	if len(pts) != 1 {
		t.Fatalf("sink points len=%d, want 1", len(pts))
	}
	if pts[0].DwellTimeElapsed != perPointDwellMs {
		t.Fatalf("DwellTimeElapsed = %d, want %d (per-point DwellMs 应覆盖全局 DwellTimeMs=%d)",
			pts[0].DwellTimeElapsed, perPointDwellMs, config.DwellTimeMs)
	}
}

// 测试前置：point 字段全部 nil（line/rectangle/sector 模式生成路径默认行为）
// 测试步骤：Start + RunCurrentPoint，检查 sink 结果
// 期待结果：使用全局 DwellTimeMs 和 SamplesPerPoint，行为不回归
func TestRunCurrentPoint_NilPerPointFields_FallbackToGlobal(t *testing.T) {
	// countingLatestDataReader 每次返回递增 Timestamp，满足 collectAveragedSamples 对 fresh sample 的要求
	reader := &countingLatestDataReader{}
	motionAccess := &mockMotionAccess{}
	sink := &mockTraversalPointSink{}
	store := newMockTraversalResultStore()
	manager := NewTraversalManager(reader, motionAccess, sink, store, wiring.NewFileCheckpointStore())

	config := traversal.Config{
		TaskID:          "trav-perpoint-nil",
		DeviceID:        "sim-1",
		Channels:        []int{0},
		ChannelLabels:   map[int]string{0: "P1"},
		// Path 中 Point 的 DwellMs/Samples/Test 全部 nil（line/rectangle/sector 生成的默认行为）
		// X/Y/Z/U 全为 0：避免触发 motion safety deviation
		Path:            []traversal.Point{{X: 0, Y: 0, Z: 0, U: 0}},
		DwellTimeMs:     5,
		SamplesPerPoint: 2,
		SavePath:        t.TempDir(),
		SaveFileName:    "perpoint-nil",
	}
	if err := manager.Start(config); err != nil {
		t.Fatalf("Start returned error: %v", err)
	}
	t.Cleanup(func() { _ = manager.Stop() })

	if err := manager.RunCurrentPoint(); err != nil {
		t.Fatalf("RunCurrentPoint returned error: %v", err)
	}

	// 期待结果：使用全局 DwellTimeMs=5 和 SamplesPerPoint=2，状态为 Completed
	pts, _, _ := sink.snapshot()
	if len(pts) != 1 {
		t.Fatalf("sink points len=%d, want 1", len(pts))
	}
	if pts[0].PointStatus != traversal.PointStatusCompleted {
		t.Fatalf("PointStatus = %q, want %q (nil Test 字段应正常采集)",
			pts[0].PointStatus, traversal.PointStatusCompleted)
	}
	if pts[0].DwellTimeElapsed != config.DwellTimeMs {
		t.Fatalf("DwellTimeElapsed = %d, want %d (nil DwellMs 应回退全局)",
			pts[0].DwellTimeElapsed, config.DwellTimeMs)
	}
	if pts[0].SampleCount != config.SamplesPerPoint {
		t.Fatalf("SampleCount = %d, want %d (nil Samples 应回退全局)",
			pts[0].SampleCount, config.SamplesPerPoint)
	}
}

// countingLatestDataReader 计数 GetLatestData 调用次数，并返回单调递增的通道值
//
// P3 修复（code-review Important-5）：GetLatestTimestamp 与 GetLatestData 返回一致的 Timestamp，
// 防止未来重构采集层新鲜度检测（改用 GetLatestTimestamp）时 mock 静默破坏生产逻辑。
type countingLatestDataReader struct {
	calls int
}

func (r *countingLatestDataReader) GetLatestData(deviceID string) (device.DataPayload, bool) {
	r.calls++
	return device.DataPayload{
		DeviceID:       deviceID,
		Timestamp:      int64(r.calls),
		Channels:       []float64{float64(r.calls)},
		ChannelIndices: []int{0},
	}, true
}

// GetLatestTimestamp 返回与 GetLatestData 一致的递增 Timestamp，true 表示有新数据
func (r *countingLatestDataReader) GetLatestTimestamp(_ string) (int64, bool) {
	return int64(r.calls), true
}
