package calibration

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"cal1604/internal/device"
	"cal1604/internal/domain"
	"cal1604/internal/events"
	"cal1604/internal/workflow"
)

type valveGateFakeMeasureDriver struct {
	valveStatus string
}

func (d *valveGateFakeMeasureDriver) Connect(_ context.Context) error { return nil }

func (d *valveGateFakeMeasureDriver) Disconnect(_ context.Context) error { return nil }

func (d *valveGateFakeMeasureDriver) ReadValveStatus(_ context.Context) (string, error) {
	if d.valveStatus == "" {
		return "measurement", nil
	}
	return d.valveStatus, nil
}

func (d *valveGateFakeMeasureDriver) SetValveStatus(_ context.Context, status string) error {
	d.valveStatus = status
	return nil
}

func (d *valveGateFakeMeasureDriver) ReadUnit(_ context.Context) (string, error) { return "kPa", nil }

func (d *valveGateFakeMeasureDriver) SetUnit(_ context.Context, _ string) error { return nil }

func (d *valveGateFakeMeasureDriver) CollectData(_ context.Context, channels []int) ([]float64, error) {
	result := make([]float64, len(channels))
	return result, nil
}

func (d *valveGateFakeMeasureDriver) ReadDeviceInfo(_ context.Context) (map[string]string, error) {
	return map[string]string{"model": "fake"}, nil
}

func (d *valveGateFakeMeasureDriver) Reset(_ context.Context) error { return nil }

func (d *valveGateFakeMeasureDriver) CalibrateZero(_ context.Context, _ []int) ([]float64, error) {
	return nil, nil
}

func (d *valveGateFakeMeasureDriver) CalibrateFullScale(_ context.Context, _ []int, _ float64) ([]float64, error) {
	return nil, nil
}

func newValveGateTestService() *Service {
	svc := NewService(workflow.NewWorkflowCoordinator(), nil, nil, nil, nil, nil)
	svc.setMeasureDriver("m1", &valveGateFakeMeasureDriver{valveStatus: "measurement"})
	svc.config = domain.WorkflowConfig{
		Channels:       []int{1},
		PointCount: 2,
		ControlMode:    domain.ControlModeManual,
	}
	return svc
}

// setMeasureDriver 测试辅助：把驱动注册进校准服务的多设备驱动表。
func (s *Service) setMeasureDriver(id string, drv device.MeasureDriver) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.measureDrivers == nil {
		s.measureDrivers = make(map[string]device.MeasureDriver)
	}
	s.measureDrivers[id] = drv
	s.measureDevID = id
	s.measureDevIDs = []string{id}
}

func TestValidateStartPrerequisitesValveGateSwitch(t *testing.T) {
	svc := newValveGateTestService()
	svc.SetStartPrerequisiteConfig(StartPrerequisiteConfig{EnforceValveCalibration: true})

	err := svc.ValidateStartPrerequisites(context.Background())
	if err == nil || !strings.Contains(err.Error(), "valve must be in calibration state") {
		t.Fatalf("expected valve gate error, got %v", err)
	}

	svc.SetStartPrerequisiteConfig(StartPrerequisiteConfig{EnforceValveCalibration: false})
	if err := svc.ValidateStartPrerequisites(context.Background()); err != nil {
		t.Fatalf("expected prerequisites pass when valve gate disabled, got %v", err)
	}
}

func TestStartCalibrationValveGateSwitch(t *testing.T) {
	svc := newValveGateTestService()
	svc.SetStartPrerequisiteConfig(StartPrerequisiteConfig{EnforceValveCalibration: true})

	err := svc.StartCalibration(context.Background())
	if err == nil || !strings.Contains(err.Error(), "valve must be in calibration state") {
		t.Fatalf("expected valve gate error, got %v", err)
	}

	svc.SetStartPrerequisiteConfig(StartPrerequisiteConfig{EnforceValveCalibration: false})
	if err := svc.StartCalibration(context.Background()); err != nil {
		t.Fatalf("expected start pass when valve gate disabled, got %v", err)
	}
}

type calibrationCollectFakeMeasureDriver struct {
	collectDataCalled             bool
	collectCalibrationPointCalled bool
	collectCalibrationPointCount  int
}

func (d *calibrationCollectFakeMeasureDriver) Connect(_ context.Context) error { return nil }

func (d *calibrationCollectFakeMeasureDriver) Disconnect(_ context.Context) error { return nil }

func (d *calibrationCollectFakeMeasureDriver) ReadValveStatus(_ context.Context) (string, error) {
	return "calibration", nil
}

func (d *calibrationCollectFakeMeasureDriver) SetValveStatus(_ context.Context, _ string) error {
	return nil
}

func (d *calibrationCollectFakeMeasureDriver) ReadUnit(_ context.Context) (string, error) {
	return "kPa", nil
}

func (d *calibrationCollectFakeMeasureDriver) SetUnit(_ context.Context, _ string) error { return nil }

func (d *calibrationCollectFakeMeasureDriver) CollectData(_ context.Context, _ []int) ([]float64, error) {
	d.collectDataCalled = true
	return nil, errors.New("collect data command not allowed in calibration mode")
}

func (d *calibrationCollectFakeMeasureDriver) CollectCalibrationPoint(_ context.Context, pointIndex int, targetPressure float64) ([]float64, error) {
	d.collectCalibrationPointCalled = true
	d.collectCalibrationPointCount++
	if pointIndex != 1 {
		return nil, errors.New("unexpected point index")
	}
	if targetPressure != 10 {
		return nil, errors.New("unexpected target pressure")
	}
	return []float64{10.11, 10.22}, nil
}

func (d *calibrationCollectFakeMeasureDriver) ReadDeviceInfo(_ context.Context) (map[string]string, error) {
	return map[string]string{"model": "fake"}, nil
}

func (d *calibrationCollectFakeMeasureDriver) Reset(_ context.Context) error { return nil }

func (d *calibrationCollectFakeMeasureDriver) CalibrateZero(_ context.Context, _ []int) ([]float64, error) {
	return nil, nil
}

func (d *calibrationCollectFakeMeasureDriver) CalibrateFullScale(_ context.Context, _ []int, _ float64) ([]float64, error) {
	return nil, nil
}

func (d *calibrationCollectFakeMeasureDriver) StartCalibration(_ context.Context, _ []int, _, _ int) error {
	return nil
}

func (d *calibrationCollectFakeMeasureDriver) PerformFitting(_ context.Context) error {
	return nil
}

func (d *calibrationCollectFakeMeasureDriver) SaveCoefficients(_ context.Context) error {
	return nil
}

func (d *calibrationCollectFakeMeasureDriver) EndCalibration(_ context.Context) error {
	return nil
}

type calibrationPressureDriverForStatusTest struct {
	target float64
}

func (d *calibrationPressureDriverForStatusTest) Connect(_ context.Context) error { return nil }

func (d *calibrationPressureDriverForStatusTest) Disconnect(_ context.Context) error { return nil }

func (d *calibrationPressureDriverForStatusTest) SetTargetPressure(_ context.Context, target float64) error {
	d.target = target
	return nil
}

func (d *calibrationPressureDriverForStatusTest) Stop(_ context.Context) error { return nil }

func (d *calibrationPressureDriverForStatusTest) Exhaust(_ context.Context) error { return nil }

func (d *calibrationPressureDriverForStatusTest) ReadCurrentPressure(_ context.Context) (float64, error) {
	return d.target, nil
}

func (d *calibrationPressureDriverForStatusTest) ReadUnit(_ context.Context) (string, error) {
	return "kPa", nil
}

func (d *calibrationPressureDriverForStatusTest) SetUnit(_ context.Context, _ string) error {
	return nil
}

func (d *calibrationPressureDriverForStatusTest) ReadStability(_ context.Context) (bool, error) {
	return true, nil
}

type calibrationEventRecorder struct {
	mu       sync.Mutex
	events   []string
	statuses []string
}

func (r *calibrationEventRecorder) Publish(eventType string, data any) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, eventType)
	if eventType != events.EventCalibrationPointStatus {
		return
	}
	point, ok := data.(domain.PressurePoint)
	if !ok {
		return
	}
	r.statuses = append(r.statuses, point.Status)
}

func containsStatus(statuses []string, target string) bool {
	for _, status := range statuses {
		if status == target {
			return true
		}
	}
	return false
}

type alarmDecisionMeasureDriver struct {
	samples [][]float64
	idx     int
}

func (d *alarmDecisionMeasureDriver) Connect(_ context.Context) error { return nil }

func (d *alarmDecisionMeasureDriver) Disconnect(_ context.Context) error { return nil }

func (d *alarmDecisionMeasureDriver) ReadValveStatus(_ context.Context) (string, error) {
	return "calibration", nil
}

func (d *alarmDecisionMeasureDriver) SetValveStatus(_ context.Context, _ string) error { return nil }

func (d *alarmDecisionMeasureDriver) ReadUnit(_ context.Context) (string, error) { return "kPa", nil }

func (d *alarmDecisionMeasureDriver) SetUnit(_ context.Context, _ string) error { return nil }

func (d *alarmDecisionMeasureDriver) CollectData(_ context.Context, _ []int) ([]float64, error) {
	if len(d.samples) == 0 {
		return []float64{0}, nil
	}
	if d.idx >= len(d.samples) {
		return append([]float64(nil), d.samples[len(d.samples)-1]...), nil
	}
	result := append([]float64(nil), d.samples[d.idx]...)
	d.idx++
	return result, nil
}

func (d *alarmDecisionMeasureDriver) ReadDeviceInfo(_ context.Context) (map[string]string, error) {
	return map[string]string{"model": "alarm-fake"}, nil
}

func (d *alarmDecisionMeasureDriver) Reset(_ context.Context) error { return nil }

func (d *alarmDecisionMeasureDriver) CalibrateZero(_ context.Context, _ []int) ([]float64, error) {
	return nil, nil
}

func (d *alarmDecisionMeasureDriver) CalibrateFullScale(_ context.Context, _ []int, _ float64) ([]float64, error) {
	return nil, nil
}

func waitAlarmPending(t *testing.T, svc *Service) {
	t.Helper()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		svc.alarmMu.Lock()
		pending := svc.alarmPending
		svc.alarmMu.Unlock()
		if pending {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}

	t.Fatal("timeout waiting alarm pending state")
}

func TestCollectUsesCalibrationPointCommandWhenSupported(t *testing.T) {
	drv := &calibrationCollectFakeMeasureDriver{}
	svc := NewService(workflow.NewWorkflowCoordinator(), nil, nil, nil, nil, nil)
	svc.setMeasureDriver("m1", drv)
	svc.config = domain.WorkflowConfig{
		Channels:       []int{1, 2},
		AverageCount:   5,
		PointCount: 2,
	}
	svc.pressurePoints = []domain.PressurePoint{
		{
			Index:          1,
			TargetPressure: 10,
			Status:         domain.PointStatusStabilizing,
		},
	}

	data, err := svc.Collect(context.Background(), 1)
	if err != nil {
		t.Fatalf("Collect should succeed with calibration point command, got %v", err)
	}

	if !drv.collectCalibrationPointCalled {
		t.Fatalf("expected CollectCalibrationPoint to be called")
	}
	if drv.collectDataCalled {
		t.Fatalf("expected CollectData not to be called when calibration command is supported")
	}
	if drv.collectCalibrationPointCount != 5 {
		t.Fatalf("expected CollectCalibrationPoint to be called 5 times, got %d", drv.collectCalibrationPointCount)
	}

	if len(data) != 2 || data[0] != 10.11 || data[1] != 10.22 {
		t.Fatalf("unexpected collect result: %#v", data)
	}

	if svc.pressurePoints[0].Status != domain.PointStatusCompleted {
		t.Fatalf("expected point status completed, got %s", svc.pressurePoints[0].Status)
	}
}

func TestCollectMultiDeviceStoresPerDeviceData(t *testing.T) {
	recorder := &calibrationEventRecorder{}
	drvA := &calibrationCollectFakeMeasureDriver{}
	drvB := &calibrationCollectFakeMeasureDriver{}
	svc := NewService(workflow.NewWorkflowCoordinator(), nil, nil, recorder.Publish, nil, nil)
	svc.setMeasureDriver("dev-a", drvA)
	svc.setMeasureDriver("dev-b", drvB)
	svc.measureDevIDs = []string{"dev-a", "dev-b"}
	svc.config = domain.WorkflowConfig{
		Channels:       []int{1, 2},
		AverageCount:   3,
		PointCount: 2,
	}
	svc.pressurePoints = []domain.PressurePoint{
		{
			Index:          1,
			TargetPressure: 10,
			Status:         domain.PointStatusStabilizing,
		},
	}

	data, err := svc.Collect(context.Background(), 1)
	if err != nil {
		t.Fatalf("Collect multi-device failed: %v", err)
	}

	// 首个设备数据兼容返回
	if len(data) == 0 {
		t.Fatal("expected first device data")
	}

	// 设备维度数据必须包含两台设备
	point := svc.pressurePoints[0]
	if len(point.CollectedByDevice) != 2 {
		t.Fatalf("expected 2 devices in CollectedByDevice, got %d", len(point.CollectedByDevice))
	}
	for _, devID := range []string{"dev-a", "dev-b"} {
		d, ok := point.CollectedByDevice[devID]
		if !ok {
			t.Fatalf("missing device %s in CollectedByDevice", devID)
		}
		if d.Status != domain.PointStatusCompleted {
			t.Fatalf("device %s status = %s, want completed", devID, d.Status)
		}
		if len(d.Collected) == 0 {
			t.Fatalf("device %s has no collected data", devID)
		}
	}
}

func TestResolveSkipDeviceMarksRemainingPoints(t *testing.T) {
	recorder := &calibrationEventRecorder{}
	svc := NewService(workflow.NewWorkflowCoordinator(), nil, nil, recorder.Publish, nil, nil)
	svc.setMeasureDriver("dev-a", &calibrationCollectFakeMeasureDriver{})
	svc.setMeasureDriver("dev-b", &calibrationCollectFakeMeasureDriver{})
	svc.measureDevIDs = []string{"dev-a", "dev-b"}
	svc.pressurePoints = []domain.PressurePoint{
		{
			Index:   1,
			Status:  domain.PointStatusCompleted,
			CollectedByDevice: map[string]domain.DevicePointData{
				"dev-a": {DeviceID: "dev-a", Collected: []float64{1.0}, Status: domain.PointStatusCompleted},
				"dev-b": {DeviceID: "dev-b", Collected: []float64{2.0}, Status: domain.PointStatusCompleted},
			},
		},
		{
			Index:  2,
			Status: domain.PointStatusPending,
		},
	}

	if err := svc.ResolveSkipDevice("dev-b", "采集超时"); err != nil {
		t.Fatalf("ResolveSkipDevice: %v", err)
	}

	// 已完成点数据保留
	completed := svc.pressurePoints[0]
	if d := completed.CollectedByDevice["dev-b"]; d.Status != domain.PointStatusCompleted {
		t.Fatalf("completed point dev-b should stay completed, got %s", d.Status)
	}

	// 未完成点标记 skipped + 原因
	next := svc.pressurePoints[1]
	d, ok := next.CollectedByDevice["dev-b"]
	if !ok {
		t.Fatal("missing dev-b in next point CollectedByDevice")
	}
	if d.Status != domain.PointStatusSkipped || d.SkipReason != "采集超时" {
		t.Fatalf("expected skipped with reason, got %+v", d)
	}
}

// TestCheckAlarmDeviceErrorTriggersPause 验证设备级采集失败（CollectedByDevice 中 status=error）会触发报警并暂停，
// 而不是静默返回 continue。这是多设备失败处理的核心回归点。
func TestCheckAlarmDeviceErrorTriggersPause(t *testing.T) {
	recorder := &calibrationEventRecorder{}
	svc := NewService(workflow.NewWorkflowCoordinator(), nil, nil, recorder.Publish, nil, nil)
	svc.setMeasureDriver("dev-a", &calibrationCollectFakeMeasureDriver{})
	svc.setMeasureDriver("dev-b", &calibrationCollectFakeMeasureDriver{})
	svc.measureDevIDs = []string{"dev-a", "dev-b"}
	svc.config = domain.WorkflowConfig{
		Channels:       []int{1, 2},
		AverageCount:   1,
		PointCount: 2,
		MaxPressure:    10,
		MinPressure:    0,
	}
	svc.alarmConfig = domain.AlarmConfig{
		Enabled:            true,
		PrecisionThreshold: 0.5,
		EnabledChannels:    []int{1},
	}
	svc.pressurePoints = []domain.PressurePoint{
		{
			Index:          1,
			TargetPressure: 10,
			Status:         domain.PointStatusPending,
			CollectedByDevice: map[string]domain.DevicePointData{
				// dev-a 成功，dev-b 采集失败（status=error）
				"dev-a": {DeviceID: "dev-a", Collected: []float64{10.1, 10.2}, Status: domain.PointStatusCompleted},
				"dev-b": {DeviceID: "dev-b", Status: domain.PointStatusError, Error: "collect sample 1: timeout"},
			},
		},
	}
	if err := svc.coordinator.Machine().Transition(domain.SessionStateReady); err != nil {
		t.Fatalf("transition to ready: %v", err)
	}
	if err := svc.coordinator.Machine().Transition(domain.SessionStateCollecting); err != nil {
		t.Fatalf("transition to collecting: %v", err)
	}
	if err := svc.coordinator.Machine().Transition(domain.SessionStatePointDone); err != nil {
		t.Fatalf("transition to point_done: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	done := make(chan struct{})
	go func() {
		defer close(done)
		// checkAlarm 遇到设备错误必须阻塞在 await_alarm_resolution，等待用户决策
		_, _ = svc.checkAlarm(ctx, 1, nil)
	}()

	// 设备错误必须触发报警等待，而不是直接返回 continue
	waitAlarmPending(t, svc)

	// 用户选择跳过该设备
	if err := svc.ResolveSkipDevice("dev-b", "采集超时"); err != nil {
		t.Fatalf("ResolveSkipDevice: %v", err)
	}

	<-done

	// dev-b 被跳过，dev-a 数据保留
	point := svc.pressurePoints[0]
	if d, ok := point.CollectedByDevice["dev-b"]; !ok || d.Status != domain.PointStatusSkipped {
		t.Fatalf("expected dev-b skipped, got %+v", d)
	}
	if d := point.CollectedByDevice["dev-a"]; d.Status != domain.PointStatusCompleted {
		t.Fatalf("expected dev-a completed, got %s", d.Status)
	}
}

func TestRetryPointManualModeWithoutPressureDeviceResetsOnly(t *testing.T) {
	svc := NewService(workflow.NewWorkflowCoordinator(), nil, nil, nil, nil, nil)
	svc.config = domain.WorkflowConfig{ControlMode: domain.ControlModeManual}
	ap := 10.15
	svc.pressurePoints = []domain.PressurePoint{
		{
			Index:          1,
			TargetPressure: 10,
			Status:         domain.PointStatusCompleted,
			CollectedData:  []float64{10.12, 10.23},
			ActualPressure: &ap,
		},
	}

	if err := svc.RetryPoint(context.Background(), 1); err != nil {
		t.Fatalf("RetryPoint should not fail in manual mode without pressure device, got %v", err)
	}

	point := svc.pressurePoints[0]
	if point.Status != domain.PointStatusPending {
		t.Fatalf("expected point status pending after retry reset, got %s", point.Status)
	}
	if point.CollectedData != nil {
		t.Fatalf("expected collected data to be cleared, got %#v", point.CollectedData)
	}
	if point.ActualPressure != nil {
		t.Fatalf("expected actual pressure reset to nil, got %v", point.ActualPressure)
	}
}

func TestPressurizePublishesPointStatusEvents(t *testing.T) {
	recorder := &calibrationEventRecorder{}
	svc := NewService(workflow.NewWorkflowCoordinator(), nil, nil, recorder.Publish, nil, nil)
	svc.pressureDriver = &calibrationPressureDriverForStatusTest{}
	if err := svc.coordinator.Machine().Transition(domain.SessionStateReady); err != nil {
		t.Fatalf("transition to ready: %v", err)
	}
	svc.pressurePoints = []domain.PressurePoint{{
		Index:          1,
		TargetPressure: 10,
		Status:         domain.PointStatusPending,
	}}
	svc.config = domain.WorkflowConfig{StableWaitMs: 1}

	if err := svc.Pressurize(context.Background(), 1); err != nil {
		t.Fatalf("Pressurize should succeed, got %v", err)
	}

	point := svc.pressurePoints[0]
	if point.Status != domain.PointStatusStabilizing {
		t.Fatalf("expected point status stabilizing after pressurize, got %s", point.Status)
	}

	if !containsStatus(recorder.statuses, domain.PointStatusPressurizing) {
		t.Fatalf("expected pressurizing status event, got %v", recorder.statuses)
	}
	if !containsStatus(recorder.statuses, domain.PointStatusStabilizing) {
		t.Fatalf("expected stabilizing status event, got %v", recorder.statuses)
	}
}

func TestCollectPublishesPointStatusEvents(t *testing.T) {
	recorder := &calibrationEventRecorder{}
	drv := &calibrationCollectFakeMeasureDriver{}
	svc := NewService(workflow.NewWorkflowCoordinator(), nil, nil, recorder.Publish, nil, nil)
	svc.setMeasureDriver("m1", drv)
	svc.config = domain.WorkflowConfig{
		Channels:       []int{1, 2},
		AverageCount:   1,
		PointCount: 2,
	}
	svc.pressurePoints = []domain.PressurePoint{{
		Index:          1,
		TargetPressure: 10,
		Status:         domain.PointStatusStabilizing,
	}}

	if _, err := svc.Collect(context.Background(), 1); err != nil {
		t.Fatalf("Collect should succeed, got %v", err)
	}

	if !containsStatus(recorder.statuses, domain.PointStatusCollecting) {
		t.Fatalf("expected collecting status event, got %v", recorder.statuses)
	}
	if !containsStatus(recorder.statuses, domain.PointStatusCompleted) {
		t.Fatalf("expected completed status event, got %v", recorder.statuses)
	}
}

func TestPauseAutoCollectionStopsRunningLoop(t *testing.T) {
	svc := NewService(workflow.NewWorkflowCoordinator(), nil, nil, nil, nil, nil)

	if err := svc.coordinator.Machine().Transition(domain.SessionStateReady); err != nil {
		t.Fatalf("transition to ready: %v", err)
	}
	if err := svc.coordinator.Machine().Transition(domain.SessionStatePressurizing); err != nil {
		t.Fatalf("transition to pressurizing: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	svc.autoCollectionCtx = ctx
	svc.autoCollectionCancel = cancel

	if err := svc.PauseAutoCollection(); err != nil {
		t.Fatalf("PauseAutoCollection should succeed, got %v", err)
	}

	if svc.IsAutoCollectionRunning() {
		t.Fatal("expected auto collection to be stopped after pause")
	}
}

func TestResumePointIndexLocked(t *testing.T) {
	svc := NewService(workflow.NewWorkflowCoordinator(), nil, nil, nil, nil, nil)
	svc.pressurePoints = []domain.PressurePoint{
		{Index: 1, Status: domain.PointStatusCompleted,},
		{Index: 2, Status: domain.PointStatusCompleted,},
		{Index: 3, Status: domain.PointStatusPending,},
		{Index: 4, Status: domain.PointStatusPending,},
	}
	svc.currentPoint = 0

	if got := svc.resumePointIndexLocked(); got != 2 {
		t.Fatalf("expected resume point index 2, got %d", got)
	}

	svc.currentPoint = 3
	if got := svc.resumePointIndexLocked(); got != 3 {
		t.Fatalf("expected resume point index 3, got %d", got)
	}
}

func TestResolveAlarmSupportsNewDecisions(t *testing.T) {
	recorder := &calibrationEventRecorder{}
	svc := NewService(workflow.NewWorkflowCoordinator(), nil, nil, recorder.Publish, nil, nil)
	svc.currentPoint = 1
	svc.alarmCh = make(chan string, 1)
	svc.alarmPending = true

	if err := svc.ResolveAlarm("skip"); err != nil {
		t.Fatalf("ResolveAlarm skip should succeed, got %v", err)
	}

	select {
	case decision := <-svc.alarmCh:
		if decision != "skip" {
			t.Fatalf("expected decision skip, got %s", decision)
		}
	default:
		t.Fatal("expected alarm decision to be sent to channel")
	}
}

func TestCollectPointAlarmDecisionSkip(t *testing.T) {
	svc := NewService(workflow.NewWorkflowCoordinator(), nil, nil, nil, nil, nil)
	svc.setMeasureDriver("m1", &alarmDecisionMeasureDriver{samples: [][]float64{{20}}})
	svc.pressureDriver = &calibrationPressureDriverForStatusTest{}
	svc.config = domain.WorkflowConfig{
		Channels:       []int{1},
		PointCount: 2,
		AverageCount:   1,
		StableWaitMs:   1,
		MaxPressure:    10,
	}
	svc.alarmConfig = domain.AlarmConfig{
		Enabled:            true,
		PrecisionThreshold: 0.5,
		EnabledChannels:    []int{1},
	}
	svc.pressurePoints = []domain.PressurePoint{{Index: 1, TargetPressure: 10, Status: domain.PointStatusPending,}}

	if err := svc.coordinator.Machine().Transition(domain.SessionStateReady); err != nil {
		t.Fatalf("transition to ready: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- svc.collectPoint(ctx, 1)
	}()

	waitAlarmPending(t, svc)
	if err := svc.ResolveAlarm("skip"); err != nil {
		t.Fatalf("ResolveAlarm skip: %v", err)
	}

	if err := <-done; err != nil {
		t.Fatalf("collectPoint with skip should succeed, got %v", err)
	}

	if got := svc.pressurePoints[0].Status; got != domain.PointStatusSkipped {
		t.Fatalf("expected skipped status, got %s", got)
	}
}

func TestCollectPointAlarmDecisionStop(t *testing.T) {
	svc := NewService(workflow.NewWorkflowCoordinator(), nil, nil, nil, nil, nil)
	svc.setMeasureDriver("m1", &alarmDecisionMeasureDriver{samples: [][]float64{{20}}})
	svc.pressureDriver = &calibrationPressureDriverForStatusTest{}
	svc.config = domain.WorkflowConfig{
		Channels:       []int{1},
		PointCount: 2,
		AverageCount:   1,
		StableWaitMs:   1,
		MaxPressure:    10,
	}
	svc.alarmConfig = domain.AlarmConfig{
		Enabled:            true,
		PrecisionThreshold: 0.5,
		EnabledChannels:    []int{1},
	}
	svc.pressurePoints = []domain.PressurePoint{{Index: 1, TargetPressure: 10, Status: domain.PointStatusPending,}}

	if err := svc.coordinator.Machine().Transition(domain.SessionStateReady); err != nil {
		t.Fatalf("transition to ready: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- svc.collectPoint(ctx, 1)
	}()

	waitAlarmPending(t, svc)
	if err := svc.ResolveAlarm("stop"); err != nil {
		t.Fatalf("ResolveAlarm stop: %v", err)
	}

	err := <-done
	if !errors.Is(err, errAutoCollectionStopped) {
		t.Fatalf("expected stop error, got %v", err)
	}
}

func TestCollectPointAlarmDecisionRecollect(t *testing.T) {
	measureDriver := &alarmDecisionMeasureDriver{samples: [][]float64{{20}, {10}}}
	svc := NewService(workflow.NewWorkflowCoordinator(), nil, nil, nil, nil, nil)
	svc.setMeasureDriver("m1", measureDriver)
	svc.pressureDriver = &calibrationPressureDriverForStatusTest{}
	svc.config = domain.WorkflowConfig{
		Channels:       []int{1},
		PointCount: 2,
		AverageCount:   1,
		StableWaitMs:   1,
		MaxPressure:    10,
	}
	svc.alarmConfig = domain.AlarmConfig{
		Enabled:            true,
		PrecisionThreshold: 0.5,
		EnabledChannels:    []int{1},
	}
	svc.pressurePoints = []domain.PressurePoint{{Index: 1, TargetPressure: 10, Status: domain.PointStatusPending,}}

	if err := svc.coordinator.Machine().Transition(domain.SessionStateReady); err != nil {
		t.Fatalf("transition to ready: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- svc.collectPoint(ctx, 1)
	}()

	waitAlarmPending(t, svc)
	if err := svc.ResolveAlarm("recollect"); err != nil {
		t.Fatalf("ResolveAlarm recollect: %v", err)
	}

	if err := <-done; err != nil {
		t.Fatalf("collectPoint with recollect should succeed, got %v", err)
	}

	if measureDriver.idx < 2 {
		t.Fatalf("expected recollect to perform at least 2 collections, got %d", measureDriver.idx)
	}
	if got := svc.pressurePoints[0].Status; got != domain.PointStatusCompleted {
		t.Fatalf("expected completed status after recollect flow, got %s", got)
	}
}
