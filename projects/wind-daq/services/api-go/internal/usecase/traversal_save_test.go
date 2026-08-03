package usecase

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"wind-daq/services/api-go/internal/core/device"
	"wind-daq/services/api-go/internal/core/traversal"
	"wind-daq/services/api-go/pkg/wiring"
)

type pathReportingTraversalSink struct {
	outputPath  string
	finalizeErr error
}

func (s *pathReportingTraversalSink) InitializeTraversal(traversal.Config) error { return nil }
func (s *pathReportingTraversalSink) WriteTraversalPoint(traversal.PointResult) error {
	return nil
}
func (s *pathReportingTraversalSink) FinalizeTraversal() error { return s.finalizeErr }
func (s *pathReportingTraversalSink) OutputPath() string       { return s.outputPath }

func TestFinalizeSinkRecordsSaveError(t *testing.T) {
	sink := &pathReportingTraversalSink{finalizeErr: errors.New("disk flush failed")}
	mgr := NewTraversalManager(nil, nil, sink, nil, nil)
	mgr.mu.Lock()
	mgr.config.TaskID = "trav-finalize-error"
	mgr.status = traversal.Status{TaskID: "trav-finalize-error", State: traversal.StateCompleted}
	mgr.mu.Unlock()

	mgr.finalizeSink()

	status := mgr.Status()
	if status.State != traversal.StateError {
		t.Fatalf("state = %q, want error", status.State)
	}
	if status.LastErrorCode != traversal.ErrSaveFailed {
		t.Fatalf("lastErrorCode = %q, want %q", status.LastErrorCode, traversal.ErrSaveFailed)
	}
	if status.LastError == "" {
		t.Fatal("expected lastError to be set")
	}
}

func TestRunCurrentPointCheckpointUsesSinkOutputPath(t *testing.T) {
	tmpDir := t.TempDir()
	outputPath := filepath.Join(tmpDir, "actual.csv")
	reader := &mockLatestDataReader{data: device.DataPayload{Channels: []float64{1, 2, 3, 4, 5}}}
	motionAccess := &mockMotionAccess{}
	sink := &pathReportingTraversalSink{outputPath: outputPath}
	mgr := NewTraversalManager(reader, motionAccess, sink, newMockTraversalResultStore(), wiring.NewFileCheckpointStore())
	config := traversal.Config{
		TaskID:          "trav-checkpoint-output-path",
		DeviceID:        "sim-1",
		Channels:        []int{0, 1, 2, 3, 4},
		Path:            make([]traversal.Point, 11),
		DwellTimeMs:     1,
		SamplesPerPoint: 1,
		SavePath:        tmpDir,
		SaveFileName:    "configured-name",
	}
	for i := range config.Path {
		config.Path[i] = traversal.Point{X: 0, Y: 0, Z: 0}
	}
	if err := mgr.Start(config); err != nil {
		t.Fatalf("Start returned error: %v", err)
	}
	mgr.mu.Lock()
	mgr.status.CurrentPoint = 9
	mgr.mu.Unlock()

	if err := mgr.RunCurrentPoint(); err != nil {
		t.Fatalf("RunCurrentPoint returned error: %v", err)
	}
	t.Cleanup(func() { _ = mgr.Stop() })

	cpPath := traversal.ResolveCheckpointPathFromCSV(outputPath)
	data, err := os.ReadFile(cpPath)
	if err != nil {
		t.Fatalf("read checkpoint file failed: %v", err)
	}
	var cp traversal.Checkpoint
	if err := json.Unmarshal(data, &cp); err != nil {
		t.Fatalf("unmarshal checkpoint failed: %v", err)
	}
	if cp.SavePath != outputPath {
		t.Fatalf("checkpoint SavePath = %q, want %q", cp.SavePath, outputPath)
	}
}
