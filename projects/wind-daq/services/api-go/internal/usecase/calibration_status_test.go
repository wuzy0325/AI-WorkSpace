package usecase

import (
	"testing"
	"time"

	"wind-daq/services/api-go/internal/core/calibration"
	"wind-daq/services/api-go/internal/core/device"
)

type calibrationStatusLatestReader struct{}

func (r calibrationStatusLatestReader) GetLatestData(_ string) (device.DataPayload, bool) {
	return device.DataPayload{
		Channels:       []float64{1, 2, 3, 4, 5, 101325, 25, 80, 15},
		ChannelIndices: []int{0, 1, 2, 3, 4, 5, 6, 7, 8},
	}, true
}

func TestCalibrationManagerStatusUpdatesDuringRunningAutomaticCalibration(t *testing.T) {
	manager := NewCalibrationManager(calibrationStatusLatestReader{}, nil, nil, nil)
	config := fiveHoleStatusTestConfig("cal-live-status")
	config.DwellTimeMs = 200

	if err := manager.Start(config); err != nil {
		t.Fatalf("start calibration: %v", err)
	}

	waitForStatus(t, manager, func(status calibration.Status) bool {
		return status.State == calibration.StateRunning &&
			status.CompletedPoints == 1 &&
			status.Progress > 0 &&
			len(status.DataPoints) == 1
	}, "running calibration to expose the first collected point")

	waitForStatus(t, manager, func(status calibration.Status) bool {
		return status.State == calibration.StateCompleted
	}, "calibration to complete after live status assertion")
}

func TestCalibrationManagerStopStateIsNotOverwrittenByAutomaticCompletion(t *testing.T) {
	manager := NewCalibrationManager(calibrationStatusLatestReader{}, nil, nil, nil)
	config := fiveHoleStatusTestConfig("cal-stop-status")
	config.DwellTimeMs = 200

	if err := manager.Start(config); err != nil {
		t.Fatalf("start calibration: %v", err)
	}

	waitForStatus(t, manager, func(status calibration.Status) bool {
		return status.State == calibration.StateRunning && status.CompletedPoints == 1
	}, "running calibration to collect the first point before stop")

	if err := manager.Stop(); err != nil {
		t.Fatalf("stop calibration: %v", err)
	}

	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		status := manager.Status()
		if status.State == calibration.StateCompleted {
			t.Fatalf("expected stopped state to be preserved, got completed with %d/%d points", status.CompletedPoints, status.TotalPoints)
		}
		time.Sleep(10 * time.Millisecond)
	}

	status := manager.Status()
	if status.State != calibration.StateStopped {
		t.Fatalf("expected stopped state, got %s", status.State)
	}
}

func fiveHoleStatusTestConfig(taskID string) calibration.Config {
	return calibration.Config{
		TaskID:          taskID,
		Type:            string(calibration.TypeFiveHole),
		SamplesPerPoint: 1,
		Points: []calibration.CalPoint{
			{ID: 1, Coordinates: map[string]float64{"α": 0, "β": 0}},
			{ID: 2, Coordinates: map[string]float64{"α": 1, "β": 0}},
		},
		ProbeChannels: []calibration.ProbeChannel{
			{Role: "fiveHole.p1", DeviceID: "dev-1", ChannelIndex: 0, Enabled: true},
			{Role: "fiveHole.p2", DeviceID: "dev-1", ChannelIndex: 1, Enabled: true},
			{Role: "fiveHole.p3", DeviceID: "dev-1", ChannelIndex: 2, Enabled: true},
			{Role: "fiveHole.p4", DeviceID: "dev-1", ChannelIndex: 3, Enabled: true},
			{Role: "fiveHole.p5", DeviceID: "dev-1", ChannelIndex: 4, Enabled: true},
			{Role: "fiveHole.pAtm", DeviceID: "dev-1", ChannelIndex: 5, Enabled: true},
			{Role: "fiveHole.tAtm", DeviceID: "dev-1", ChannelIndex: 6, Enabled: true},
			{Role: "fiveHole.pTotal", DeviceID: "dev-1", ChannelIndex: 7, Enabled: true},
			{Role: "fiveHole.pTunnelStatic", DeviceID: "dev-1", ChannelIndex: 8, Enabled: true},
		},
	}
}

func waitForStatus(t *testing.T, manager *CalibrationManager, predicate func(calibration.Status) bool, description string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		status := manager.Status()
		if predicate(status) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}

	status := manager.Status()
	t.Fatalf("timed out waiting for %s; state=%s completed=%d total=%d progress=%v dataPoints=%d error=%q",
		description,
		status.State,
		status.CompletedPoints,
		status.TotalPoints,
		status.Progress,
		len(status.DataPoints),
		status.LastError,
	)
}
