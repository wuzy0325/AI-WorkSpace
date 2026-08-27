package workflow_test

import (
	"testing"
	"time"

	eventtypes "cal1604/internal/events"
	"cal1604/internal/workflow"
)

type stabilityEvent struct {
	eventType string
	status    workflow.StabilityStatus
}

func TestStabilityAccumulatorResetsOnDrift(t *testing.T) {
	acc := workflow.NewStabilityAccumulator(0.2, 2*time.Second)

	stable, elapsed := acc.AddSample(10, 10.1, time.Second)
	if stable {
		t.Fatal("expected not stable after first sample")
	}
	if elapsed != time.Second {
		t.Fatalf("expected 1s elapsed, got %s", elapsed)
	}

	stable, elapsed = acc.AddSample(10, 10.6, time.Second)
	if stable {
		t.Fatal("expected drift sample to be unstable")
	}
	if elapsed != 0 {
		t.Fatalf("expected elapsed reset to 0, got %s", elapsed)
	}
}

func TestStabilityMonitorPublishesProgressAndAchieved(t *testing.T) {
	events := make([]stabilityEvent, 0)
	publisher := func(eventType string, data any) {
		status, ok := data.(workflow.StabilityStatus)
		if !ok {
			t.Fatalf("unexpected status payload type: %T", data)
		}
		events = append(events, stabilityEvent{eventType: eventType, status: status})
	}

	monitor := workflow.NewStabilityMonitor(0.5, time.Second, publisher)
	for i := 0; i < 6; i++ {
		monitor.FeedSample(10, 10.2)
	}

	progressEvents := 0
	achievedEvents := 0
	for _, event := range events {
		if event.eventType == eventtypes.EventCalibrationStabilityProgress {
			progressEvents++
			if event.status.Progress < 0 || event.status.Progress > 100 {
				t.Fatalf("progress out of range: %d", event.status.Progress)
			}
		}
		if event.eventType == eventtypes.EventCalibrationStabilityAchieved {
			achievedEvents++
		}
	}

	if progressEvents == 0 {
		t.Fatalf("expected progress events, got %v", events)
	}
	if achievedEvents != 1 {
		t.Fatalf("expected one achieved event, got %d", achievedEvents)
	}
}

func TestStabilityMonitorPublishesLostEvent(t *testing.T) {
	events := make([]string, 0)
	monitor := workflow.NewStabilityMonitor(0.2, time.Second, func(eventType string, _ any) {
		events = append(events, eventType)
	})

	monitor.FeedSample(10, 10.1)
	monitor.FeedSample(10, 10.8)

	hasLost := false
	for _, eventType := range events {
		if eventType == eventtypes.EventCalibrationStabilityLost {
			hasLost = true
			break
		}
	}

	if !hasLost {
		t.Fatalf("expected lost event, got %v", events)
	}
}
