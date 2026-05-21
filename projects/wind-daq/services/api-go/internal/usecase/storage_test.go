package usecase

import (
	"errors"
	"testing"

	"wind-daq/services/api-go/internal/core/device"
	"wind-daq/services/api-go/internal/core/storage"
)

type fakeRecordingSink struct {
	started bool
	stopped bool
	writes  []device.DataPayload
	err     error
}

func (s *fakeRecordingSink) Start(config storage.RecordingConfig) error {
	if s.err != nil {
		return s.err
	}
	s.started = true
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
