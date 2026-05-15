package usecase

import (
	"strings"
	"testing"

	"wind-daq/services/api-go/internal/core/calibration"
	"wind-daq/services/api-go/internal/ports"
)

func TestStartCalibrationWhenRunning(t *testing.T) {
	svc := NewCalibrationService(nil, nil, nil)
	svc.status = calibration.CalRunning
	svc.taskID = "cal-test"

	_, err := svc.Start(calibration.CalibrationConfig{})
	if err == nil {
		t.Fatal("expected error when starting already running calibration")
	}
	if !strings.Contains(err.Error(), "calibration already running") {
		t.Fatalf("unexpected error message: %v", err)
	}
}

func TestPauseWhenNotRunning(t *testing.T) {
	svc := NewCalibrationService(nil, nil, nil)

	err := svc.Pause()
	if err == nil {
		t.Fatal("expected error when pausing idle calibration")
	}
	if !strings.Contains(err.Error(), "calibration not running") {
		t.Fatalf("unexpected error message: %v", err)
	}
}

func TestResumeWhenNotPaused(t *testing.T) {
	svc := NewCalibrationService(nil, nil, nil)

	err := svc.Resume()
	if err == nil {
		t.Fatal("expected error when resuming non-paused calibration")
	}
	if !strings.Contains(err.Error(), "calibration not paused") {
		t.Fatalf("unexpected error message: %v", err)
	}

	svc.status = calibration.CalRunning
	err = svc.Resume()
	if err == nil {
		t.Fatal("expected error when resuming running calibration")
	}
}

func TestAcquireFiveHoleDataMissingP1(t *testing.T) {
	svc := NewCalibrationService(nil, nil, nil)

	data := map[string]calibration.ChannelData{
		"dev1": {DeviceID: "dev1", Channels: map[int]float64{0: 1.0}},
	}

	_, err := svc.acquireFiveHoleData(
		calibration.CalibrationPoint{},
		data,
		[]calibration.ProbeChannelConfig{},
		1,
	)
	if err == nil {
		t.Fatal("expected error for missing p1 channel role")
	}
	if !strings.Contains(err.Error(), "get P1") {
		t.Fatalf("unexpected error message: %v", err)
	}
}

func TestAcquireDataWithNilDeviceManager(t *testing.T) {
	svc := NewCalibrationService(nil, nil, nil)
	svc.config = &calibration.CalibrationConfig{
		Type: calibration.CalFiveHole,
	}

	_, err := svc.acquireData(calibration.CalibrationPoint{})
	if err == nil {
		t.Fatal("expected error when deviceManager is nil")
	}
	if !strings.Contains(err.Error(), "device manager not initialized") {
		t.Fatalf("unexpected error message: %v", err)
	}
}

func TestReadChannelDataWithNoConnectedDevices(t *testing.T) {
	dm := &DeviceManager{devices: make(map[string]ports.Device)}
	svc := NewCalibrationService(dm, nil, nil)
	svc.config = &calibration.CalibrationConfig{
		ProbeChannels: []calibration.ProbeChannelConfig{
			{DeviceID: "dev1", Channel: 0, Role: "p1"},
		},
	}

	_, err := svc.readChannelData()
	if err == nil {
		t.Fatal("expected error when no devices are connected")
	}
	if !strings.Contains(err.Error(), "no connected devices available") {
		t.Fatalf("unexpected error message: %v", err)
	}
}
