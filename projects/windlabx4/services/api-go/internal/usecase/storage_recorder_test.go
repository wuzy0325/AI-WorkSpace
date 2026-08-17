package usecase

import (
	"strings"
	"testing"

	"windlabx4/services/api-go/internal/core/device"
	"windlabx4/services/api-go/internal/core/storage"
)

// TestStorageRecorderWritesMultiplePayloadsInOrder 验证连续写入多个 payload 时，
// sink.writes 按调用顺序依次记录，不丢帧、不乱序。
// 覆盖 TC-UC-13：StorageRecorder 多负载顺序写入。
func TestStorageRecorderWritesMultiplePayloadsInOrder(t *testing.T) {
	sink := &fakeRecordingSink{}
	recorder := NewStorageRecorder(sink)

	if err := recorder.Start(storage.RecordingConfig{
		OutputDir:  t.TempDir(),
		FilePrefix: "run",
	}); err != nil {
		t.Fatalf("Start 返回错误: %v", err)
	}

	// 构造 5 个具有不同时间戳和通道值的 payload，便于顺序校验
	payloads := []device.DataPayload{
		{DeviceID: "sim-1", Timestamp: 1, Channels: []float64{1.0}, ChannelIndices: []int{0}},
		{DeviceID: "sim-1", Timestamp: 2, Channels: []float64{2.0}, ChannelIndices: []int{0}},
		{DeviceID: "sim-1", Timestamp: 3, Channels: []float64{3.0}, ChannelIndices: []int{0}},
		{DeviceID: "sim-1", Timestamp: 4, Channels: []float64{4.0}, ChannelIndices: []int{0}},
		{DeviceID: "sim-1", Timestamp: 5, Channels: []float64{5.0}, ChannelIndices: []int{0}},
	}

	for i, p := range payloads {
		if err := recorder.HandlePayload(p); err != nil {
			t.Fatalf("第 %d 个 payload HandlePayload 返回错误: %v", i+1, err)
		}
	}

	if len(sink.writes) != len(payloads) {
		t.Fatalf("期望 %d 次写入，实际 %d", len(payloads), len(sink.writes))
	}

	// 逐个校验写入顺序和内容
	for i, want := range payloads {
		got := sink.writes[i]
		if got.Timestamp != want.Timestamp {
			t.Fatalf("writes[%d]: 期望 Timestamp=%d，实际 %d", i, want.Timestamp, got.Timestamp)
		}
		if len(got.Channels) != 1 || got.Channels[0] != want.Channels[0] {
			t.Fatalf("writes[%d]: 期望 Channels=%v，实际 %v", i, want.Channels, got.Channels)
		}
	}
}

// TestStorageRecorderStartValidatesConfig 验证 Start 对配置参数的校验：
// 空 OutputDir、空 FilePrefix、nil sink 分别返回对应错误，且不修改 recorder 状态。
func TestStorageRecorderStartValidatesConfig(t *testing.T) {
	// 场景 1：空 OutputDir
	sink := &fakeRecordingSink{}
	recorder := NewStorageRecorder(sink)
	err := recorder.Start(storage.RecordingConfig{OutputDir: "", FilePrefix: "run"})
	if err == nil {
		t.Fatal("期望空 OutputDir 返回错误，实际返回 nil")
	}
	if !strings.Contains(err.Error(), "outputDir is required") {
		t.Fatalf("期望错误包含 'outputDir is required'，实际 %q", err.Error())
	}
	if recorder.Status().Recording {
		t.Fatal("校验失败后不应进入录制状态")
	}

	// 场景 2：空 FilePrefix
	err = recorder.Start(storage.RecordingConfig{OutputDir: t.TempDir(), FilePrefix: ""})
	if err == nil {
		t.Fatal("期望空 FilePrefix 返回错误，实际返回 nil")
	}
	if !strings.Contains(err.Error(), "filePrefix is required") {
		t.Fatalf("期望错误包含 'filePrefix is required'，实际 %q", err.Error())
	}
	if recorder.Status().Recording {
		t.Fatal("校验失败后不应进入录制状态")
	}

	// 场景 3：nil sink（NewStorageRecorder(nil) 构造的 recorder）
	nilRecorder := NewStorageRecorder(nil)
	err = nilRecorder.Start(storage.RecordingConfig{OutputDir: t.TempDir(), FilePrefix: "run"})
	if err == nil {
		t.Fatal("期望 nil sink 返回错误，实际返回 nil")
	}
	if !strings.Contains(err.Error(), "recording sink is required") {
		t.Fatalf("期望错误包含 'recording sink is required'，实际 %q", err.Error())
	}
	if nilRecorder.Status().Recording {
		t.Fatal("校验失败后不应进入录制状态")
	}
}

// TestStorageRecorderStopWhenNotRecordingIsNoop 验证未启动录制就调用 Stop
// 时不报错，且不调用 sink.Stop()。
func TestStorageRecorderStopWhenNotRecordingIsNoop(t *testing.T) {
	sink := &fakeRecordingSink{}
	recorder := NewStorageRecorder(sink)

	// 未启动录制就 Stop，应无错误返回
	if err := recorder.Stop(); err != nil {
		t.Fatalf("未录制时 Stop 返回错误: %v", err)
	}
	if sink.stopped {
		t.Fatal("未录制时不应调用 sink.Stop()")
	}

	// Status 应反映未录制状态
	if status := recorder.Status(); status.Recording {
		t.Fatal("期望 Recording=false，实际 true")
	}
}

// TestStorageRecorderStatusReflectsState 验证 Status() 能正确反映
// 录制状态变化：初始未录制 -> Start 后录制中 -> Stop 后未录制。
func TestStorageRecorderStatusReflectsState(t *testing.T) {
	sink := &fakeRecordingSink{}
	recorder := NewStorageRecorder(sink)

	// 初始状态：未录制
	if status := recorder.Status(); status.Recording {
		t.Fatal("初始状态期望 Recording=false，实际 true")
	}

	// Start 后：录制中
	if err := recorder.Start(storage.RecordingConfig{
		OutputDir:  t.TempDir(),
		FilePrefix: "run",
	}); err != nil {
		t.Fatalf("Start 返回错误: %v", err)
	}
	if status := recorder.Status(); !status.Recording {
		t.Fatal("Start 后期望 Recording=true，实际 false")
	}

	// Stop 后：未录制
	if err := recorder.Stop(); err != nil {
		t.Fatalf("Stop 返回错误: %v", err)
	}
	if status := recorder.Status(); status.Recording {
		t.Fatal("Stop 后期望 Recording=false，实际 true")
	}
}
