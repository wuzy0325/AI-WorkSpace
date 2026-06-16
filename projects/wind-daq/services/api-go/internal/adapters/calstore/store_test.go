package calstore

import (
	"testing"

	"wind-daq/services/api-go/internal/core/calibration"
	"wind-daq/services/api-go/internal/core/traversal"
)

func TestMemoryResultStoreSaveAndGetCalibration(t *testing.T) {
	store := NewMemoryResultStore()

	status := calibration.Status{
		TaskID: "cal-1", State: calibration.StateIdle,
		CurrentPoint: 3, TotalPoints: 3,
		DataPoints: []calibration.DataPoint{
			&calibration.PointResult{PointIndex: 0, TargetPressure: 0, Values: map[int]float64{0: 10.5}},
			&calibration.PointResult{PointIndex: 1, TargetPressure: 50, Values: map[int]float64{0: 11.2}},
		},
	}

	if err := store.Save("cal-1", status); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, ok := store.Get("cal-1")
	if !ok {
		t.Fatal("expected to find cal-1")
	}
	if got.State != calibration.StateIdle {
		t.Fatalf("expected idle, got %s", got.State)
	}
	if len(got.DataPoints) != 2 {
		t.Fatalf("expected 2 dataPoints, got %d", len(got.DataPoints))
	}
}

func TestMemoryResultStoreReturnsFalseForMissingKey(t *testing.T) {
	store := NewMemoryResultStore()
	_, ok := store.Get("nonexistent")
	if ok {
		t.Fatal("expected false for missing key")
	}
}

func TestMemoryResultStoreOverwritesExistingKey(t *testing.T) {
	store := NewMemoryResultStore()

	store.Save("cal-1", calibration.Status{TaskID: "cal-1", State: calibration.StateRunning})
	store.Save("cal-1", calibration.Status{TaskID: "cal-1", State: calibration.StateIdle})

	got, ok := store.Get("cal-1")
	if !ok {
		t.Fatal("expected to find cal-1")
	}
	if got.State != calibration.StateIdle {
		t.Fatalf("expected idle after overwrite, got %s", got.State)
	}
}

func TestTraversalResultStoreSaveAndGet(t *testing.T) {
	store := NewTraversalResultStore()

	status := traversal.Status{
		TaskID: "trav-1", State: traversal.StateIdle,
		TotalPoints: 2,
		Results: []traversal.PointResult{
			{PointIndex: 0, Point: traversal.Point{X: 10, Y: 20, Z: 0}},
		},
	}

	if err := store.Save("trav-1", status); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, ok := store.Get("trav-1")
	if !ok {
		t.Fatal("expected to find trav-1")
	}
	if len(got.Results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(got.Results))
	}
}

func TestTraversalResultStoreConcurrentAccess(t *testing.T) {
	store := NewTraversalResultStore()

	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func(n int) {
			taskID := string(rune('A' + n))
			store.Save(taskID, traversal.Status{TaskID: taskID})
			store.Get(taskID)
			done <- true
		}(i)
	}

	for i := 0; i < 10; i++ {
		<-done
	}
}
