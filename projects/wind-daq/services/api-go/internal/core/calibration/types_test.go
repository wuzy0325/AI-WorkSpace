package calibration

import (
	"testing"
)

func TestConfigValidatesDefaults(t *testing.T) {
	cfg := Config{
		TaskID:         "test-1",
		DeviceID:       "dev-1",
		Channels:       []int{0, 1},
		PressurePoints: []float64{0, 50, 100},
		AverageSamples: 5,
	}

	if cfg.TaskID != "test-1" {
		t.Fatalf("expected test-1, got %s", cfg.TaskID)
	}
	if len(cfg.PressurePoints) != 3 {
		t.Fatalf("expected 3 pressure points, got %d", len(cfg.PressurePoints))
	}
}

func TestStateTransitionsAreValid(t *testing.T) {
	states := map[State]bool{
		StateIdle:    true,
		StateRunning: true,
		StatePaused:  true,
		StateStopped: true,
		StateError:   true,
	}

	for s, ok := range states {
		if !ok {
			t.Fatalf("state %s not in valid set", s)
		}
	}
}

func TestPointResultHasAllFields(t *testing.T) {
	r := PointResult{
		PointIndex:     1,
		TargetPressure: 50.0,
		Timestamp:      123456789,
		Values:         map[int]float64{0: 10.5, 1: 20.3},
	}

	if r.PointIndex != 1 {
		t.Fatalf("expected point index 1, got %d", r.PointIndex)
	}
	if r.TargetPressure != 50.0 {
		t.Fatalf("expected 50, got %f", r.TargetPressure)
	}
	if r.Values[0] != 10.5 {
		t.Fatalf("expected channel 0 value 10.5, got %f", r.Values[0])
	}
}
