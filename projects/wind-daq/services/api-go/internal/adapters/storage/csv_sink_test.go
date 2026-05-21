package storage

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"wind-daq/services/api-go/internal/core/device"
	corestorage "wind-daq/services/api-go/internal/core/storage"
)

func TestCSVRecordingSinkWritesPayloadsToFile(t *testing.T) {
	dir := t.TempDir()
	sink := NewCSVRecordingSink()

	if err := sink.Start(corestorage.RecordingConfig{OutputDir: dir, FilePrefix: "run"}); err != nil {
		t.Fatalf("Start returned error: %v", err)
	}
	if err := sink.Write(device.DataPayload{DeviceID: "sim-1", Timestamp: 123, ChannelIndices: []int{0, 1}, Channels: []float64{1.2, 3.4}}); err != nil {
		t.Fatalf("Write returned error: %v", err)
	}
	if err := sink.Stop(); err != nil {
		t.Fatalf("Stop returned error: %v", err)
	}

	files, err := filepath.Glob(filepath.Join(dir, "run-*.csv"))
	if err != nil {
		t.Fatalf("glob recording files: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("expected one recording file, got %d", len(files))
	}
	content, err := os.ReadFile(files[0])
	if err != nil {
		t.Fatalf("read recording file: %v", err)
	}
	text := string(content)
	if !strings.Contains(text, "timestamp,deviceId,channelIndex,value") {
		t.Fatalf("expected CSV header, got %q", text)
	}
	if !strings.Contains(text, "123,sim-1,0,1.200000") || !strings.Contains(text, "123,sim-1,1,3.400000") {
		t.Fatalf("expected payload rows, got %q", text)
	}
}

func TestCSVRecordingSinkReturnsErrorWhenNotStarted(t *testing.T) {
	sink := NewCSVRecordingSink()

	if err := sink.Write(device.DataPayload{DeviceID: "sim-1"}); err == nil {
		t.Fatal("expected write before start to fail")
	}
}
