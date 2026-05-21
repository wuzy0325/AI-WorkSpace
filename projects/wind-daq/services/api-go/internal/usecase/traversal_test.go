package usecase

import (
	"testing"

	"wind-daq/services/api-go/internal/core/device"
	"wind-daq/services/api-go/internal/core/traversal"
)

type fakeTraversalResultStore struct {
	data map[string]traversal.Status
}

func newFakeTraversalResultStore() *fakeTraversalResultStore {
	return &fakeTraversalResultStore{data: make(map[string]traversal.Status)}
}

func (s *fakeTraversalResultStore) Save(taskID string, status traversal.Status) error {
	s.data[taskID] = status
	return nil
}

func (s *fakeTraversalResultStore) Get(taskID string) (traversal.Status, bool) {
	status, ok := s.data[taskID]
	return status, ok
}

type fakeTraversalPointSink struct {
	points []traversal.PointResult
}

func (s *fakeTraversalPointSink) WriteTraversalPoint(point traversal.PointResult) error {
	s.points = append(s.points, point)
	return nil
}

func TestTraversalManagerRunsPointAndRecordsData(t *testing.T) {
	reader := newFakeReaderWithPayload(device.DataPayload{
		DeviceID:       "daq-1",
		Timestamp:      456,
		Channels:       []float64{1.2, 3.4},
		ChannelIndices: []int{0, 1},
	})
	motionManager := NewMotionManager(newFakeMotionController())
	sink := &fakeTraversalPointSink{}
	manager := NewTraversalManager(reader, motionManager, sink, newFakeTraversalResultStore())

	if err := manager.Start(traversal.Config{
		TaskID:   "trav-1",
		DeviceID: "daq-1",
		Channels: []int{0, 1},
		Path:     []traversal.Point{{X: 2.5, Y: 0, Z: 0}},
	}); err != nil {
		t.Fatalf("Start returned error: %v", err)
	}
	if status := manager.Status(); status.State != traversal.StateRunning || status.CurrentPoint != 0 {
		t.Fatalf("expected running at point 0, got %+v", status)
	}

	if err := manager.RunCurrentPoint(); err != nil {
		t.Fatalf("RunCurrentPoint returned error: %v", err)
	}
	status := manager.Status()
	if status.CurrentPoint != 1 || len(status.Results) != 1 {
		t.Fatalf("expected one result and next point index, got %+v", status)
	}
	if got := motionManager.Status().Axes[0].Position; got != 2.5 {
		t.Fatalf("expected X axis moved to 2.5, got %.2f", got)
	}
	if got := status.Results[0].Values[1]; got != 3.4 {
		t.Fatalf("expected recorded channel value 3.4, got %.2f", got)
	}
	if len(sink.points) != 1 {
		t.Fatalf("expected storage sink to receive one traversal point, got %d", len(sink.points))
	}
}

func TestTraversalManagerPauseResumeAndStop(t *testing.T) {
	motionManager := NewMotionManager(newFakeMotionController())
	manager := NewTraversalManager(newFakeReaderWithPayload(device.DataPayload{DeviceID: "daq-1", Timestamp: 1, Channels: []float64{1}, ChannelIndices: []int{0}}), motionManager, nil, newFakeTraversalResultStore())

	if err := manager.Start(traversal.Config{
		TaskID:   "trav-1",
		DeviceID: "daq-1",
		Channels: []int{0},
		Path:     []traversal.Point{{X: 1}},
	}); err != nil {
		t.Fatalf("Start returned error: %v", err)
	}
	if err := manager.Pause(); err != nil {
		t.Fatalf("Pause returned error: %v", err)
	}
	if status := manager.Status(); status.State != traversal.StatePaused || status.CurrentPoint != 0 {
		t.Fatalf("expected paused without losing current point, got %+v", status)
	}
	if err := manager.Resume(); err != nil {
		t.Fatalf("Resume returned error: %v", err)
	}
	if status := manager.Status(); status.State != traversal.StateRunning || status.CurrentPoint != 0 {
		t.Fatalf("expected resumed at point 0, got %+v", status)
	}

	if err := motionManager.Jog("X", 1); err != nil {
		t.Fatalf("Jog returned error: %v", err)
	}
	if err := manager.Stop(); err != nil {
		t.Fatalf("Stop returned error: %v", err)
	}
	if status := manager.Status(); status.State != traversal.StateStopped {
		t.Fatalf("expected stopped state, got %+v", status)
	}
	if motionManager.Status().Axes[0].Moving {
		t.Fatal("expected stop to stop moving axis")
	}
}

func TestTraversalManagerReturnsErrorWhenDeviceHasNoData(t *testing.T) {
	manager := NewTraversalManager(&fakeLatestDataReader{}, NewMotionManager(newFakeMotionController()), nil, newFakeTraversalResultStore())

	if err := manager.Start(traversal.Config{
		TaskID:   "trav-1",
		DeviceID: "daq-1",
		Channels: []int{0},
		Path:     []traversal.Point{{X: 1}},
	}); err != nil {
		t.Fatalf("Start returned error: %v", err)
	}
	if err := manager.RunCurrentPoint(); err == nil {
		t.Fatal("expected no data error")
	}
	if status := manager.Status(); status.State != traversal.StateError || status.LastError == "" {
		t.Fatalf("expected error state with message, got %+v", status)
	}
}
