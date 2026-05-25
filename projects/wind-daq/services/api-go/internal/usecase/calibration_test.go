package usecase

import (
	"testing"

	"wind-daq/services/api-go/internal/adapters/hardware"
	"wind-daq/services/api-go/internal/core/calibration"
	"wind-daq/services/api-go/internal/core/device"
	"wind-daq/services/api-go/internal/core/motion"
	"wind-daq/services/api-go/internal/ports"
)

type fakeLatestDataReader struct {
	payloads []device.DataPayload
	index    int
}

func newFakeReaderWithPayload(payload device.DataPayload) *fakeLatestDataReader {
	return &fakeLatestDataReader{payloads: []device.DataPayload{payload}}
}

func newFakeReaderWithPayloads(payloads []device.DataPayload) *fakeLatestDataReader {
	return &fakeLatestDataReader{payloads: payloads}
}

func (r *fakeLatestDataReader) GetLatestData(deviceID string) (device.DataPayload, bool) {
	if len(r.payloads) == 0 {
		return device.DataPayload{}, false
	}
	p := r.payloads[r.index%len(r.payloads)]
	r.index++
	return p, true
}

type fakeCalibrationPointSink struct {
	points []calibration.PointResult
}

func (s *fakeCalibrationPointSink) WriteCalibrationPoint(point calibration.PointResult) error {
	s.points = append(s.points, point)
	return nil
}

type fakeCalibrationResultStore struct {
	data map[string]calibration.Status
}

func newFakeCalibrationResultStore() *fakeCalibrationResultStore {
	return &fakeCalibrationResultStore{data: make(map[string]calibration.Status)}
}

func (s *fakeCalibrationResultStore) Save(taskID string, status calibration.Status) error {
	s.data[taskID] = status
	return nil
}

func (s *fakeCalibrationResultStore) Get(taskID string) (calibration.Status, bool) {
	status, ok := s.data[taskID]
	return status, ok
}

func TestCalibrationManagerStartAndCollectsPressurePoint(t *testing.T) {
	reader := newFakeReaderWithPayload(device.DataPayload{
		DeviceID:       "daq-1",
		Timestamp:      123,
		Channels:       []float64{10.5, 20.5},
		ChannelIndices: []int{0, 1},
	})
	store := newFakeCalibrationResultStore()
	manager := NewCalibrationManager(reader, nil, nil, store)

	if err := manager.Start(calibration.Config{
		TaskID:         "cal-1",
		DeviceID:       "daq-1",
		Channels:       []int{0, 1},
		PressurePoints: []float64{0, 50},
		AverageSamples: 1,
	}); err != nil {
		t.Fatalf("Start returned error: %v", err)
	}
	if status := manager.Status(); status.State != calibration.StateRunning || status.CurrentPoint != 0 {
		t.Fatalf("expected running at point 0, got %+v", status)
	}

	if err := manager.CollectCurrentPoint(); err != nil {
		t.Fatalf("CollectCurrentPoint returned error: %v", err)
	}
	status := manager.Status()
	if status.CurrentPoint != 1 || len(status.Results) != 1 {
		t.Fatalf("expected one collected point and next index 1, got %+v", status)
	}
	if got := status.Results[0].Values[1]; got != 20.5 {
		t.Fatalf("expected channel 1 value 20.5, got %.2f", got)
	}
}

func TestCalibrationManagerCollectsAllPointsAndCompletes(t *testing.T) {
	payload := device.DataPayload{
		DeviceID: "daq-1", Timestamp: 123,
		Channels: []float64{10}, ChannelIndices: []int{0},
	}
	reader := newFakeReaderWithPayload(payload)
	sink := &fakeCalibrationPointSink{}
	store := newFakeCalibrationResultStore()
	manager := NewCalibrationManager(reader, nil, sink, store)

	if err := manager.Start(calibration.Config{
		TaskID: "cal-2", DeviceID: "daq-1",
		Channels: []int{0}, PressurePoints: []float64{0, 50, 100},
		AverageSamples: 1,
	}); err != nil {
		t.Fatalf("Start: %v", err)
	}

	for i := 0; i < 3; i++ {
		if err := manager.CollectCurrentPoint(); err != nil {
			t.Fatalf("Collect point %d: %v", i, err)
		}
	}

	status := manager.Status()
	if status.State != calibration.StateIdle {
		t.Fatalf("expected idle after all points, got %s", status.State)
	}
	if len(sink.points) != 3 {
		t.Fatalf("expected 3 sink points, got %d", len(sink.points))
	}

	saved, ok := store.Get("cal-2")
	if !ok {
		t.Fatal("expected result to be saved in store")
	}
	if len(saved.Results) != 3 {
		t.Fatalf("expected 3 saved results, got %d", len(saved.Results))
	}
}

func TestCalibrationManagerAverageMultipleSamples(t *testing.T) {
	payloads := make([]device.DataPayload, 5)
	for i := 0; i < 5; i++ {
		payloads[i] = device.DataPayload{
			DeviceID: "daq-1", Timestamp: int64(100 + i),
			Channels: []float64{float64(10 + i)}, ChannelIndices: []int{0},
		}
	}
	reader := newFakeReaderWithPayloads(payloads)
	manager := NewCalibrationManager(reader, nil, nil, nil)

	manager.Start(calibration.Config{
		TaskID: "cal-3", DeviceID: "daq-1",
		Channels: []int{0}, PressurePoints: []float64{0, 50},
		AverageSamples: 3,
	})

	manager.CollectCurrentPoint()
	result := manager.Status().Results[0]
	// samples: 10, 11, 12 → average 11
	if got := result.Values[0]; got < 10.5 || got > 11.5 {
		t.Fatalf("expected average near 11, got %.2f", got)
	}
}

func TestCalibrationManagerReturnsErrorWhenDeviceHasNoData(t *testing.T) {
	reader := &fakeLatestDataReader{}
	manager := NewCalibrationManager(reader, nil, nil, nil)

	if err := manager.Start(calibration.Config{
		TaskID: "cal-err", DeviceID: "daq-1",
		Channels: []int{0}, PressurePoints: []float64{0, 50},
		AverageSamples: 1,
	}); err != nil {
		t.Fatalf("Start should succeed: %v", err)
	}
	if err := manager.CollectCurrentPoint(); err == nil {
		t.Fatal("expected collect error for device with no data")
	}
	if status := manager.Status(); status.State != calibration.StateError || status.LastError == "" {
		t.Fatalf("expected error state with message, got %+v", status)
	}
}

func TestCalibrationManagerPauseResumeAndStop(t *testing.T) {
	profile := motion.MotionControllerProfile{
		ID:      "motion-1",
		Name:    "Test Controller",
		Type:    motion.ControllerTypeSimulated,
		Address: "127.0.0.1",
		Axes: []motion.AxisConfig{
			{Name: motion.AxisX, Enabled: true},
		},
	}
	profileStore := &fakeProfileStore{profiles: []motion.MotionControllerProfile{profile}}
	motionManager := NewMotionManager(profileStore, func(profile motion.MotionControllerProfile) ports.MotionController {
		return hardware.NewSimulatedMotionController(profile)
	})

	// 先加载配置，这样管理器才知道有哪些控制器
	if _, err := motionManager.LoadProfiles(); err != nil {
		t.Fatalf("LoadProfiles returned error: %v", err)
	}

	// 连接控制器
	if err := motionManager.Connect("motion-1"); err != nil {
		t.Fatalf("Connect returned error: %v", err)
	}

	manager := NewCalibrationManager(newFakeReaderWithPayload(device.DataPayload{
		DeviceID: "daq-1", Timestamp: 1,
		Channels: []float64{1}, ChannelIndices: []int{0},
	}), motionManager, nil, nil)

	if err := manager.Start(calibration.Config{
		TaskID: "cal-4", DeviceID: "daq-1",
		Channels: []int{0}, PressurePoints: []float64{0, 50},
		AverageSamples: 1,
	}); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := manager.Pause(); err != nil {
		t.Fatalf("Pause: %v", err)
	}
	if status := manager.Status(); status.State != calibration.StatePaused || status.CurrentPoint != 0 {
		t.Fatalf("expected paused at point 0, got %+v", status)
	}
	if err := manager.Resume(); err != nil {
		t.Fatalf("Resume: %v", err)
	}

	if err := motionManager.Jog("motion-1", motion.AxisX, 1); err != nil {
		t.Fatalf("Jog: %v", err)
	}
	if err := manager.Stop(); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if status := manager.Status(); status.State != calibration.StateStopped {
		t.Fatalf("expected stopped, got %+v", status)
	}
	if status, _ := motionManager.Status("motion-1"); status.Axes[0].Moving {
		t.Fatal("expected stop to release motion")
	}
}

func TestCalibrationManagerGetResult(t *testing.T) {
	reader := newFakeReaderWithPayload(device.DataPayload{
		DeviceID: "daq-1", Timestamp: 123,
		Channels: []float64{5}, ChannelIndices: []int{0},
	})
	store := newFakeCalibrationResultStore()
	manager := NewCalibrationManager(reader, nil, nil, store)

	manager.Start(calibration.Config{
		TaskID: "cal-get", DeviceID: "daq-1",
		Channels: []int{0}, PressurePoints: []float64{0, 50},
		AverageSamples: 1,
	})
	manager.CollectCurrentPoint()
	manager.CollectCurrentPoint()

	result, ok := manager.GetResult("cal-get")
	if !ok {
		t.Fatal("expected result for cal-get")
	}
	if result.State != calibration.StateIdle {
		t.Fatalf("expected idle, got %s", result.State)
	}
	if len(result.Results) != 2 {
		t.Fatalf("expected 2 points, got %d", len(result.Results))
	}
}
