package usecase

import (
	"errors"
	"testing"

	"windlabx4/services/api-go/internal/core/device"
	"windlabx4/services/api-go/internal/core/storage"
	"windlabx4/services/api-go/internal/ports"
)

type fakeRecordingSink struct {
	started bool
	stopped bool
	writes  []device.DataPayload
	err     error
	// doneCh 在 Start 时创建，模拟 sink.Done() 信号；
	// 测试可通过 close(fakeSink.doneCh) 触发自停止通知。
	doneCh chan struct{}
}

// 确保 fakeRecordingSink 实现 ports.RecordingSink 接口。
var _ ports.RecordingSink = (*fakeRecordingSink)(nil)

func (s *fakeRecordingSink) Start(config storage.RecordingConfig) error {
	if s.err != nil {
		return s.err
	}
	s.started = true
	if s.doneCh == nil {
		s.doneCh = make(chan struct{})
	}
	return nil
}

func (s *fakeRecordingSink) Write(payload device.DataPayload) error {
	if s.err != nil {
		return s.err
	}
	s.writes = append(s.writes, payload)
	return nil
}

func (s *fakeRecordingSink) Stop() error {
	s.stopped = true
	return nil
}

// Status 返回 fake sink 的简化状态，仅用于满足接口契约。
func (s *fakeRecordingSink) Status() storage.RecordingStatus {
	return storage.RecordingStatus{
		Recording: s.started && !s.stopped,
	}
}

// Done 返回自停止信号 channel；测试可关闭它以模拟 sink 自停止。
func (s *fakeRecordingSink) Done() <-chan struct{} {
	if s.doneCh == nil {
		s.doneCh = make(chan struct{})
	}
	return s.doneCh
}

func TestStorageRecorderWritesPayloadOnlyWhenRecording(t *testing.T) {
	sink := &fakeRecordingSink{}
	recorder := NewStorageRecorder(sink)
	payload := device.DataPayload{DeviceID: "sim-1", Timestamp: 123, Channels: []float64{1.2}, ChannelIndices: []int{0}}

	if err := recorder.HandlePayload(payload); err != nil {
		t.Fatalf("HandlePayload before recording returned error: %v", err)
	}
	if len(sink.writes) != 0 {
		t.Fatalf("expected no writes before recording, got %d", len(sink.writes))
	}

	if err := recorder.Start(storage.RecordingConfig{OutputDir: t.TempDir(), FilePrefix: "run"}); err != nil {
		t.Fatalf("Start returned error: %v", err)
	}
	if err := recorder.HandlePayload(payload); err != nil {
		t.Fatalf("HandlePayload while recording returned error: %v", err)
	}
	if len(sink.writes) != 1 {
		t.Fatalf("expected one write while recording, got %d", len(sink.writes))
	}

	if err := recorder.Stop(); err != nil {
		t.Fatalf("Stop returned error: %v", err)
	}
	if err := recorder.HandlePayload(payload); err != nil {
		t.Fatalf("HandlePayload after stop returned error: %v", err)
	}
	if len(sink.writes) != 1 {
		t.Fatalf("expected no writes after stop, got %d", len(sink.writes))
	}
}

func TestStorageRecorderReportsSinkErrors(t *testing.T) {
	wantErr := errors.New("disk full")
	sink := &fakeRecordingSink{err: wantErr}
	recorder := NewStorageRecorder(sink)

	if err := recorder.Start(storage.RecordingConfig{OutputDir: t.TempDir(), FilePrefix: "run"}); !errors.Is(err, wantErr) {
		t.Fatalf("expected start error %v, got %v", wantErr, err)
	}

	sink.err = nil
	if err := recorder.Start(storage.RecordingConfig{OutputDir: t.TempDir(), FilePrefix: "run"}); err != nil {
		t.Fatalf("Start returned error: %v", err)
	}
	sink.err = wantErr
	if err := recorder.HandlePayload(device.DataPayload{DeviceID: "sim-1"}); !errors.Is(err, wantErr) {
		t.Fatalf("expected write error %v, got %v", wantErr, err)
	}
}
