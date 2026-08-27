package measurement_test

import (
	"bytes"
	"context"
	"math"
	"strings"
	"testing"
	"time"

	"cal1604/internal/application/measurement"
	"cal1604/internal/application/session"
	"cal1604/internal/device"
	"cal1604/internal/domain"
	"cal1604/internal/infrastructure/driver"
	"cal1604/internal/workflow"
)

func float64Ptr(v float64) *float64 { return &v }

// fakeMeasureDriver 最小实现，仅 CollectData。
// 默认返回 calibration 阀位以满足启动门禁（阀门=校准模式是必要条件）。
// 单测如需验证门禁失败场景，可通过 valveStatus 字段覆盖。
type fakeMeasureDriver struct {
	data        []float64
	err         error
	valveStatus string
}

func (f *fakeMeasureDriver) Connect(_ context.Context) error    { return nil }
func (f *fakeMeasureDriver) Disconnect(_ context.Context) error { return nil }
func (f *fakeMeasureDriver) ReadValveStatus(_ context.Context) (string, error) {
	if f.valveStatus == "" {
		return "calibration", nil
	}
	return f.valveStatus, nil
}
func (f *fakeMeasureDriver) SetValveStatus(_ context.Context, _ string) error { return nil }
func (f *fakeMeasureDriver) ReadUnit(_ context.Context) (string, error)       { return "kPa", nil }
func (f *fakeMeasureDriver) SetUnit(_ context.Context, _ string) error        { return nil }
func (f *fakeMeasureDriver) CollectData(_ context.Context, _ []int) ([]float64, error) {
	return f.data, f.err
}
func (f *fakeMeasureDriver) ReadDeviceInfo(_ context.Context) (map[string]string, error) {
	return nil, nil
}
func (f *fakeMeasureDriver) Reset(_ context.Context) error { return nil }
func (f *fakeMeasureDriver) CalibrateZero(_ context.Context, _ []int) ([]float64, error) {
	return nil, nil
}
func (f *fakeMeasureDriver) CalibrateFullScale(_ context.Context, _ []int, _ float64) ([]float64, error) {
	return nil, nil
}

type fakePressureDriver struct {
	targets          []float64
	currentPressure  float64
	stable           bool
	startControlCall int
	stopCalled       bool
}

func (f *fakePressureDriver) Connect(_ context.Context) error    { return nil }
func (f *fakePressureDriver) Disconnect(_ context.Context) error { return nil }
func (f *fakePressureDriver) SetTargetPressure(_ context.Context, target float64) error {
	f.targets = append(f.targets, target)
	f.currentPressure = target
	return nil
}
func (f *fakePressureDriver) Stop(_ context.Context) error {
	f.stopCalled = true
	return nil
}
func (f *fakePressureDriver) Exhaust(_ context.Context) error { return nil }
func (f *fakePressureDriver) ReadCurrentPressure(_ context.Context) (float64, error) {
	return f.currentPressure, nil
}
func (f *fakePressureDriver) ReadUnit(_ context.Context) (string, error) { return "kPa", nil }
func (f *fakePressureDriver) SetUnit(_ context.Context, _ string) error  { return nil }
func (f *fakePressureDriver) ReadStability(_ context.Context) (bool, error) {
	return f.stable, nil
}
func (f *fakePressureDriver) StartControl(_ context.Context) error {
	f.startControlCall++
	return nil
}

type fakeStore struct {
	devices map[string]domain.Device
}

func (s *fakeStore) Upsert(dev domain.Device)                      { s.devices[dev.ID] = dev }
func (s *fakeStore) UpdateStatus(string, domain.DeviceStatus) bool { return true }
func (s *fakeStore) UpdateUnit(string, string) bool                { return true }
func (s *fakeStore) Delete(string)                                 {}
func (s *fakeStore) Get(id string) (domain.Device, bool)           { d, ok := s.devices[id]; return d, ok }
func (s *fakeStore) List() []domain.Device                         { return nil }
func (s *fakeStore) CheckUnitConsistency() (bool, []string)        { return true, nil }

type embedMD struct{ device.MeasureDriver }

type mapProvider struct {
	drivers map[string]device.ConnectionDriver
}

func (p *mapProvider) GetActiveDriver(id string) device.ConnectionDriver {
	return p.drivers[id]
}

func setupMeasurementService() (*measurement.Service, *fakeMeasureDriver) {
	mDrv := &fakeMeasureDriver{data: []float64{1.1, 2.2, 3.3}}
	store := &fakeStore{devices: map[string]domain.Device{
		"m1": {ID: "m1", Type: domain.DeviceTypeMeasure, Model: "WTN1604", Host: "127.0.0.1", Port: 9000},
	}}
	sessSvc := session.NewService(store, driver.NewFactory(), func(string, any) {}, &mapProvider{
		drivers: map[string]device.ConnectionDriver{"m1": embedMD{mDrv}},
	})
	_, _ = sessSvc.BindMeasureDevice("m1", "test")
	svc := measurement.NewService(sessSvc, func(string, any) {}, workflow.NewWorkflowCoordinator())
	return svc, mDrv
}

func setupMeasurementServiceWithPressure() (*measurement.Service, *fakeMeasureDriver, *fakePressureDriver) {
	mDrv := &fakeMeasureDriver{data: []float64{1.1, 2.2, 3.3}}
	pDrv := &fakePressureDriver{stable: true}
	store := &fakeStore{devices: map[string]domain.Device{
		"m1": {ID: "m1", Type: domain.DeviceTypeMeasure, Model: "WTN1604", Host: "127.0.0.1", Port: 9000},
		"p1": {ID: "p1", Type: domain.DeviceTypePressure, Model: "ConST811A", Host: "127.0.0.1", Port: 9001},
	}}
	sessSvc := session.NewService(store, driver.NewFactory(), func(string, any) {}, &mapProvider{
		drivers: map[string]device.ConnectionDriver{
			"m1": embedMD{mDrv},
			"p1": embedPD{pDrv},
		},
	})
	_, _ = sessSvc.BindDevices("m1", "p1", "test")
	svc := measurement.NewService(sessSvc, func(string, any) {}, workflow.NewWorkflowCoordinator())
	return svc, mDrv, pDrv
}

type embedPD struct{ device.PressureDriver }

func TestInitialState(t *testing.T) {
	svc, _ := setupMeasurementService()
	if svc.State() != domain.SessionStateIdle {
		t.Fatalf("expected idle, got %s", svc.State())
	}
}

func TestGeneratePressurePointsUsesMeasurementConfig(t *testing.T) {
	svc, _ := setupMeasurementService()
	svc.SetConfig(domain.WorkflowConfig{
		MinPressure:  0,
		MaxPressure:  100,
		PointCount:   5,
		Precision:    2,
		PressureMode: domain.PressureModeRoundTrip,
	})

	points, err := svc.GeneratePressurePoints()
	if err != nil {
		t.Fatalf("GeneratePressurePoints: %v", err)
	}

	if len(points) != 10 {
		t.Fatalf("expected 10 points (5 forward + 5 backward), got %d", len(points))
	}

	if points[0].Direction != "forward" {
		t.Fatalf("expected first point direction forward, got %s", points[0].Direction)
	}

	if points[len(points)-1].Direction != "backward" {
		t.Fatalf("expected last point direction backward, got %s", points[len(points)-1].Direction)
	}

	// forward: 0, 25, 50, 75, 100; backward: 100, 75, 50, 25, 0
	expected := []float64{0, 25, 50, 75, 100, 100, 75, 50, 25, 0}
	for i, exp := range expected {
		if points[i].TargetPressure != exp {
			t.Fatalf("points[%d].TargetPressure = %v, want %v", i, points[i].TargetPressure, exp)
		}
	}
}

func TestStartWorkflowUsesGeneratedPointsAndTransitionsToReady(t *testing.T) {
	svc, _ := setupMeasurementService()
	svc.SetConfig(domain.WorkflowConfig{
		MinPressure: 0,
		MaxPressure: 20,
		PointCount:  3,
		Precision:   2,
	})
	if _, err := svc.GeneratePressurePoints(); err != nil {
		t.Fatalf("GeneratePressurePoints: %v", err)
	}

	if err := svc.StartWorkflow(context.Background(), []int{1, 2}); err != nil {
		t.Fatalf("StartWorkflow: %v", err)
	}

	if svc.State() != domain.SessionStateReady {
		t.Fatalf("expected ready, got %s", svc.State())
	}

	session := svc.GetSession()
	if session == nil {
		t.Fatal("expected measurement session to be created")
	}
	if len(session.Points) != 3 {
		t.Fatalf("expected 3 session points, got %d", len(session.Points))
	}
	if len(session.Config.Channels) != 2 {
		t.Fatalf("expected workflow channels to be stored, got %+v", session.Config.Channels)
	}
}

func TestStartWorkflowRejectedWhenValveNotCalibration(t *testing.T) {
	// 阀门=校准模式是计量启动的必要条件，valve != calibration 时应直接拒绝。
	svc, mDrv := setupMeasurementService()
	mDrv.valveStatus = "measurement"

	svc.SetConfig(domain.WorkflowConfig{
		MinPressure: 0,
		MaxPressure: 20,
		PointCount:  3,
		Precision:   2,
	})
	if _, err := svc.GeneratePressurePoints(); err != nil {
		t.Fatalf("GeneratePressurePoints: %v", err)
	}

	err := svc.StartWorkflow(context.Background(), []int{1})
	if err == nil {
		t.Fatal("expected StartWorkflow to be rejected when valve is not calibration")
	}
	if !strings.Contains(err.Error(), "valve must be in calibration state") {
		t.Fatalf("expected valve gate error, got: %v", err)
	}
}

func TestStartWorkflowBypassesValveGateWhenDisabled(t *testing.T) {
	// 显式关闭门禁后，测量阀位也允许启动（用于联调/特殊运维场景）。
	svc, mDrv := setupMeasurementService()
	mDrv.valveStatus = "measurement"
	svc.SetStartPrerequisiteConfig(measurement.StartPrerequisiteConfig{EnforceValveCalibration: false})

	svc.SetConfig(domain.WorkflowConfig{
		MinPressure: 0,
		MaxPressure: 20,
		PointCount:  3,
		Precision:   2,
	})
	if _, err := svc.GeneratePressurePoints(); err != nil {
		t.Fatalf("GeneratePressurePoints: %v", err)
	}

	if err := svc.StartWorkflow(context.Background(), []int{1}); err != nil {
		t.Fatalf("expected StartWorkflow to succeed with gate disabled: %v", err)
	}
}

func TestSetConfigInvalidatesExistingPoints(t *testing.T) {
	svc, _ := setupMeasurementService()
	svc.SetConfig(domain.WorkflowConfig{
		MinPressure: 0,
		MaxPressure: 20,
		PointCount:  3,
		Precision:   2,
	})
	if _, err := svc.GeneratePressurePoints(); err != nil {
		t.Fatalf("GeneratePressurePoints: %v", err)
	}

	svc.SetConfig(domain.WorkflowConfig{
		MinPressure: 0,
		MaxPressure: 50,
		PointCount:  5,
		Precision:   2,
	})

	if err := svc.StartWorkflow(context.Background(), []int{1}); err == nil {
		t.Fatal("expected workflow start to require regenerated points after config change")
	}
}

func TestStopWorkflowUpdatesSessionStatusToStopped(t *testing.T) {
	svc, _ := setupMeasurementService()
	svc.SetConfig(domain.WorkflowConfig{
		MinPressure: 0,
		MaxPressure: 10,
		PointCount:  2,
		Precision:   2,
	})
	if _, err := svc.GeneratePressurePoints(); err != nil {
		t.Fatalf("GeneratePressurePoints: %v", err)
	}
	if err := svc.StartWorkflow(context.Background(), []int{1}); err != nil {
		t.Fatalf("StartWorkflow: %v", err)
	}

	if err := svc.Stop(); err != nil {
		t.Fatalf("Stop: %v", err)
	}

	session := svc.GetSession()
	if session == nil {
		t.Fatal("expected measurement session after stop")
	}
	if session.Status != domain.SessionStateStopped {
		t.Fatalf("expected session status stopped, got %s", session.Status)
	}
	if session.EndTime == nil {
		t.Fatal("expected session end time to be recorded")
	}
}

func TestResumeRealtimeSamplingSyncsWorkflowSessionStatus(t *testing.T) {
	svc, _ := setupMeasurementService()
	svc.SetConfig(domain.WorkflowConfig{
		MinPressure: 0,
		MaxPressure: 10,
		PointCount:  2,
		Precision:   2,
	})
	if _, err := svc.GeneratePressurePoints(); err != nil {
		t.Fatalf("GeneratePressurePoints: %v", err)
	}
	if err := svc.StartWorkflow(context.Background(), []int{1}); err != nil {
		t.Fatalf("StartWorkflow: %v", err)
	}
	if err := svc.Pause(); err != nil {
		t.Fatalf("Pause: %v", err)
	}
	if err := svc.Start(context.Background(), []int{1}); err != nil {
		t.Fatalf("Start realtime resume: %v", err)
	}

	session := svc.GetSession()
	if session == nil {
		t.Fatal("expected measurement session after resume")
	}
	if session.Status != domain.SessionStateCollecting {
		t.Fatalf("expected session status collecting after resume, got %s", session.Status)
	}
	_ = svc.Stop()
}

func TestRunAutoCollectionAdvancesMeasurementPoints(t *testing.T) {
	svc, _, pressureDrv := setupMeasurementServiceWithPressure()
	svc.SetConfig(domain.WorkflowConfig{
		MinPressure:  0,
		MaxPressure:  10,
		PointCount:   2,
		Precision:    2,
		AverageCount: 1,
		StableWaitMs: 10,
		ControlMode:  domain.ControlModeAuto,
	})
	if _, err := svc.GeneratePressurePoints(); err != nil {
		t.Fatalf("GeneratePressurePoints: %v", err)
	}
	if err := svc.StartWorkflow(context.Background(), []int{1}); err != nil {
		t.Fatalf("StartWorkflow: %v", err)
	}

	if err := svc.RunAutoCollection(context.Background()); err != nil {
		t.Fatalf("RunAutoCollection: %v", err)
	}

	points := svc.GetPoints()
	if len(points) != 2 {
		t.Fatalf("expected 2 points, got %d", len(points))
	}
	if points[0].Status != domain.PointStatusCompleted || points[1].Status != domain.PointStatusCompleted {
		t.Fatalf("expected all points completed, got %+v", points)
	}
	if len(pressureDrv.targets) != 2 {
		t.Fatalf("expected 2 pressure targets, got %+v", pressureDrv.targets)
	}
}

func TestManualCollectCapturesPointData(t *testing.T) {
	svc, _, pressureDrv := setupMeasurementServiceWithPressure()
	svc.SetConfig(domain.WorkflowConfig{
		MinPressure:  0,
		MaxPressure:  20,
		PointCount:   3,
		Precision:    2,
		AverageCount: 1,
		StableWaitMs: 10,
		ControlMode:  domain.ControlModeManual,
	})
	if _, err := svc.GeneratePressurePoints(); err != nil {
		t.Fatalf("GeneratePressurePoints: %v", err)
	}
	if err := svc.StartWorkflow(context.Background(), []int{1, 2}); err != nil {
		t.Fatalf("StartWorkflow: %v", err)
	}

	if err := svc.ManualPressurize(context.Background(), 1); err != nil {
		t.Fatalf("ManualPressurize: %v", err)
	}
	if err := svc.ManualCollect(context.Background(), 1); err != nil {
		t.Fatalf("ManualCollect: %v", err)
	}

	points := svc.GetPoints()
	if points[0].Status != domain.PointStatusCompleted {
		t.Fatalf("expected first point completed, got %+v", points[0])
	}
	if len(points[0].CollectedData) == 0 {
		t.Fatalf("expected collected data on first point, got %+v", points[0])
	}
	if len(pressureDrv.targets) != 1 {
		t.Fatalf("expected 1 pressure target, got %+v", pressureDrv.targets)
	}
}

// fakeSequentialMeasureDriver 每次 CollectData 返回下一组数据，用于验证平铺存储。
type fakeSequentialMeasureDriver struct {
	fakeMeasureDriver
	callCount int
	allData   [][]float64
}

func (d *fakeSequentialMeasureDriver) CollectData(_ context.Context, _ []int) ([]float64, error) {
	if d.callCount >= len(d.allData) {
		return nil, nil
	}
	data := d.allData[d.callCount]
	d.callCount++
	return data, nil
}

func TestManualCollectFlatStorage(t *testing.T) {
	seqDrv := &fakeSequentialMeasureDriver{
		allData: [][]float64{
			{1.0, 2.0},
			{1.1, 2.1},
			{1.2, 2.2},
		},
	}
	store := &fakeStore{devices: map[string]domain.Device{
		"m1": {ID: "m1", Type: domain.DeviceTypeMeasure, Model: "WTN1604", Host: "127.0.0.1", Port: 9000},
	}}
	sessSvc := session.NewService(store, driver.NewFactory(), func(string, any) {}, &mapProvider{
		drivers: map[string]device.ConnectionDriver{"m1": embedMD{seqDrv}},
	})
	_, _ = sessSvc.BindMeasureDevice("m1", "test")
	svc := measurement.NewService(sessSvc, func(string, any) {}, workflow.NewWorkflowCoordinator())

	svc.SetConfig(domain.WorkflowConfig{
		MinPressure:  0,
		MaxPressure:  20,
		PointCount:   2,
		Precision:    2,
		AverageCount: 3,
		StableWaitMs: 10,
	})
	if _, err := svc.GeneratePressurePoints(); err != nil {
		t.Fatalf("GeneratePressurePoints: %v", err)
	}
	if err := svc.StartWorkflow(context.Background(), []int{1, 2}); err != nil {
		t.Fatalf("StartWorkflow: %v", err)
	}

	// 只执行采集（不执行打压，直接注入状态）
	if err := svc.ManualCollect(context.Background(), 1); err != nil {
		t.Fatalf("ManualCollect: %v", err)
	}

	points := svc.GetPoints()
	expected := []float64{1.0, 2.0, 1.1, 2.1, 1.2, 2.2} // 平铺：3次 × 2通道
	if len(points[0].CollectedData) != len(expected) {
		t.Fatalf("expected flat data len %d, got %d", len(expected), len(points[0].CollectedData))
	}
	for i, v := range expected {
		if points[0].CollectedData[i] != v {
			t.Fatalf("flat data[%d] = %v, want %v", i, points[0].CollectedData[i], v)
		}
	}
}

func TestSetStateTransitions(t *testing.T) {
	tests := []struct {
		name      string
		from      domain.SessionState
		to        domain.SessionState
		wantError bool
	}{
		{name: "idle_to_pressurizing", from: domain.SessionStateIdle, to: domain.SessionStatePressurizing},
		{name: "idle_to_collecting", from: domain.SessionStateIdle, to: domain.SessionStateCollecting},
		{name: "pressurizing_to_stabilizing", from: domain.SessionStatePressurizing, to: domain.SessionStateStabilizing},
		{name: "pressurizing_to_paused", from: domain.SessionStatePressurizing, to: domain.SessionStatePaused},
		{name: "stabilizing_to_collecting", from: domain.SessionStateStabilizing, to: domain.SessionStateCollecting},
		{name: "collecting_to_completed", from: domain.SessionStateCollecting, to: domain.SessionStateCompleted},
		{name: "completed_to_idle", from: domain.SessionStateCompleted, to: domain.SessionStateIdle},
		{name: "error_to_idle", from: domain.SessionStateError, to: domain.SessionStateIdle},
		{name: "paused_to_collecting", from: domain.SessionStatePaused, to: domain.SessionStateCollecting},
		{name: "paused_to_pressurizing", from: domain.SessionStatePaused, to: domain.SessionStatePressurizing},
		{name: "paused_to_idle", from: domain.SessionStatePaused, to: domain.SessionStateIdle},
		{name: "collecting_to_idle_invalid", from: domain.SessionStateCollecting, to: domain.SessionStateIdle, wantError: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, _ := setupMeasurementService()
			reachState(t, svc, tt.from)

			err := svc.SetState(tt.to)
			if tt.wantError {
				if err == nil {
					t.Fatalf("expected transition %s -> %s to fail", tt.from, tt.to)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected transition error %s -> %s: %v", tt.from, tt.to, err)
			}
			if got := svc.State(); got != tt.to {
				t.Fatalf("expected state %s, got %s", tt.to, got)
			}
		})
	}
}

func reachState(t *testing.T, svc *measurement.Service, target domain.SessionState) {
	t.Helper()

	if target == domain.SessionStateIdle {
		return
	}

	steps := map[domain.SessionState][]domain.SessionState{
		domain.SessionStatePressurizing: {domain.SessionStatePressurizing},
		domain.SessionStateStabilizing: {domain.SessionStatePressurizing, domain.SessionStateStabilizing},
		domain.SessionStateCollecting:  {domain.SessionStatePressurizing, domain.SessionStateStabilizing, domain.SessionStateCollecting},
		domain.SessionStateCompleted:   {domain.SessionStatePressurizing, domain.SessionStateStabilizing, domain.SessionStateCollecting, domain.SessionStateCompleted},
		domain.SessionStateError:       {domain.SessionStatePressurizing, domain.SessionStateError},
		domain.SessionStatePaused:      {domain.SessionStatePressurizing, domain.SessionStatePaused},
	}

	path, ok := steps[target]
	if !ok {
		t.Fatalf("unsupported target state in test helper: %s", target)
	}

	for _, state := range path {
		if err := svc.SetState(state); err != nil {
			t.Fatalf("reach state %s failed at %s: %v", target, state, err)
		}
	}
}

// inconsistentStore 模拟设备单位不一致的存储，用于验证开始计量的单位一致性门禁。
type inconsistentStore struct {
	*fakeStore
	conflicts []string
}

func (s *inconsistentStore) CheckUnitConsistency() (bool, []string) {
	return len(s.conflicts) == 0, s.conflicts
}

func TestStartRejectedWhenUnitsInconsistent(t *testing.T) {
	// 计量设备与打压设备单位不一致时，即使阀门满足校准状态，也必须拒绝开始计量。
	mDrv := &fakeMeasureDriver{data: []float64{1.1, 2.2, 3.3}, valveStatus: "calibration"}
	pDrv := &fakePressureDriver{stable: true}
	store := &inconsistentStore{
		fakeStore: &fakeStore{devices: map[string]domain.Device{
			"m1": {ID: "m1", Type: domain.DeviceTypeMeasure, Model: "WTN1604", Host: "127.0.0.1", Port: 9000, Unit: "kPa"},
			"p1": {ID: "p1", Type: domain.DeviceTypePressure, Model: "ConST811A", Host: "127.0.0.1", Port: 9001, Unit: "MPa"},
		}},
		conflicts: []string{"p1"},
	}
	sessSvc := session.NewService(store, driver.NewFactory(), func(string, any) {}, &mapProvider{
		drivers: map[string]device.ConnectionDriver{
			"m1": embedMD{mDrv},
			"p1": embedPD{pDrv},
		},
	})
	_, _ = sessSvc.BindDevices("m1", "p1", "test")
	svc := measurement.NewService(sessSvc, func(string, any) {}, workflow.NewWorkflowCoordinator())
	svc.SetStartPrerequisiteConfig(measurement.StartPrerequisiteConfig{EnforceValveCalibration: false})

	if err := svc.Start(context.Background(), []int{1, 2, 3}); err == nil {
		t.Fatal("expected error when device pressure units inconsistent")
	}
	if svc.State() != domain.SessionStateIdle {
		t.Fatalf("expected state idle when start rejected, got %s", svc.State())
	}
}

func TestStartTransition(t *testing.T) {
	svc, _ := setupMeasurementService()
	if err := svc.Start(context.Background(), []int{1, 2, 3}); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if svc.State() != domain.SessionStateCollecting {
		t.Fatalf("expected collecting, got %s", svc.State())
	}
	_ = svc.Stop()
}

func TestStartWithoutDevice(t *testing.T) {
	sessSvc := session.NewService(&fakeStore{}, driver.NewFactory(), func(string, any) {}, nil)
	svc := measurement.NewService(sessSvc, func(string, any) {}, workflow.NewWorkflowCoordinator())
	if err := svc.Start(context.Background(), []int{1}); err == nil {
		t.Fatal("expected error when no device bound")
	}
}

func TestPauseFromCollecting(t *testing.T) {
	svc, _ := setupMeasurementService()
	_ = svc.Start(context.Background(), []int{1, 2})
	defer svc.Stop()
	if err := svc.Pause(); err != nil {
		t.Fatalf("Pause: %v", err)
	}
	if svc.State() != domain.SessionStatePaused {
		t.Fatalf("expected paused, got %s", svc.State())
	}
}

func TestPauseFromIdleFails(t *testing.T) {
	svc, _ := setupMeasurementService()
	if err := svc.Pause(); err == nil {
		t.Fatal("expected error pausing from idle")
	}
}

func TestStopFromCollecting(t *testing.T) {
	svc, _ := setupMeasurementService()
	_ = svc.Start(context.Background(), []int{1})
	if err := svc.Stop(); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if svc.State() != domain.SessionStateIdle {
		t.Fatalf("expected idle, got %s", svc.State())
	}
}

func TestStopFromPaused(t *testing.T) {
	svc, _ := setupMeasurementService()
	_ = svc.Start(context.Background(), []int{1})
	_ = svc.Pause()
	if err := svc.Stop(); err != nil {
		t.Fatalf("Stop from paused: %v", err)
	}
	if svc.State() != domain.SessionStateIdle {
		t.Fatalf("expected idle, got %s", svc.State())
	}
}

func TestStopFromIdleFails(t *testing.T) {
	svc, _ := setupMeasurementService()
	if err := svc.Stop(); err == nil {
		t.Fatal("expected error stopping from idle")
	}
}

func TestResumeFromPaused(t *testing.T) {
	svc, _ := setupMeasurementService()
	_ = svc.Start(context.Background(), []int{1, 2})
	_ = svc.Pause()
	if err := svc.Start(context.Background(), []int{1, 2}); err != nil {
		t.Fatalf("Resume: %v", err)
	}
	if svc.State() != domain.SessionStateCollecting {
		t.Fatalf("expected collecting, got %s", svc.State())
	}
	_ = svc.Stop()
}

func TestWriteCSVEmpty(t *testing.T) {
	svc, _ := setupMeasurementService()
	var buf bytes.Buffer
	if err := svc.WriteCSV(&buf); err == nil {
		t.Fatal("expected error for empty data")
	}
}

func TestStateTransitions(t *testing.T) {
	tests := []struct {
		name    string
		from    domain.SessionState
		to      domain.SessionState
		wantErr bool
	}{
		{"idle_to_collecting", domain.SessionStateIdle, domain.SessionStateCollecting, false},
		{"idle_to_paused", domain.SessionStateIdle, domain.SessionStatePaused, true},
		{"collecting_to_paused", domain.SessionStateCollecting, domain.SessionStatePaused, false},
		{"collecting_to_idle", domain.SessionStateCollecting, domain.SessionStateIdle, false},
		{"collecting_to_collecting", domain.SessionStateCollecting, domain.SessionStateCollecting, true},
		{"paused_to_collecting", domain.SessionStatePaused, domain.SessionStateCollecting, false},
		{"paused_to_idle", domain.SessionStatePaused, domain.SessionStateIdle, false},
		{"paused_to_paused", domain.SessionStatePaused, domain.SessionStatePaused, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, _ := setupMeasurementService()
			switch tt.from {
			case domain.SessionStateCollecting:
				_ = svc.Start(context.Background(), []int{1})
			case domain.SessionStatePaused:
				_ = svc.Start(context.Background(), []int{1})
				_ = svc.Pause()
			}

			var err error
			switch tt.to {
			case domain.SessionStateCollecting:
				err = svc.Start(context.Background(), []int{1})
			case domain.SessionStatePaused:
				err = svc.Pause()
			case domain.SessionStateIdle:
				err = svc.Stop()
			}

			if tt.wantErr && err == nil {
				t.Fatal("expected error")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestMeasurementAlarmConfig(t *testing.T) {
	svc, _ := setupMeasurementService()
	svc.SetAlarmConfig(domain.AlarmConfig{
		Enabled:         true,
		EnabledChannels: []int{1, 2},
		ConfirmOnAlarm:  true,
		SoundEnabled:    false,
	})

	cfg := svc.GetAlarmConfig()
	if !cfg.Enabled {
		t.Fatal("expected alarm enabled")
	}
	if len(cfg.EnabledChannels) != 2 {
		t.Fatalf("expected 2 enabled channels, got %d", len(cfg.EnabledChannels))
	}
	if !cfg.ConfirmOnAlarm {
		t.Fatal("expected confirm on alarm")
	}
}

func TestMeasurementAlarmCheckNoAlarm(t *testing.T) {
	svc, _ := setupMeasurementService()
	svc.SetConfig(domain.WorkflowConfig{
		MinPressure:    0,
		MaxPressure:    1000,
		PrecisionLevel: 0.0002, // 0.02% FS
	})
	svc.SetAlarmConfig(domain.AlarmConfig{
		Enabled:         true,
		EnabledChannels: []int{1},
		ConfirmOnAlarm:  true,
	})

	point := domain.PressurePoint{
		Index:          1,
		TargetPressure: 100,
		ActualPressure: float64Ptr(100.05),
	}

	alarm, err := svc.CheckAlarm(point)
	if err != nil {
		t.Fatalf("CheckAlarm: %v", err)
	}
	if alarm != nil {
		t.Fatalf("expected no alarm, got %+v", alarm)
	}
}

func TestMeasurementAlarmCheckTriggersAlarm(t *testing.T) {
	svc, _ := setupMeasurementService()
	svc.SetConfig(domain.WorkflowConfig{
		MinPressure:    0,
		MaxPressure:    100,
		PrecisionLevel: 0.0004, // 0.04% FS, allowance=0.04, dev=0.05 > 0.04
	})
	svc.SetAlarmConfig(domain.AlarmConfig{
		Enabled:         true,
		EnabledChannels: []int{1},
		ConfirmOnAlarm:  true,
	})

	point := domain.PressurePoint{
		Index:          1,
		TargetPressure: 100,
		ActualPressure: float64Ptr(100.05),
		CollectedData:  []float64{100.05},
	}

	alarm, err := svc.CheckAlarm(point)
	if err != nil {
		t.Fatalf("CheckAlarm: %v", err)
	}
	if alarm == nil {
		t.Fatal("expected alarm to be triggered")
	}
	if len(alarm.OverLimitChannels) == 0 {
		t.Fatal("expected over limit channels")
	}
	if alarm.Threshold != 0.0004 {
		t.Fatalf("expected threshold 0.0004, got %v", alarm.Threshold)
	}
}

// TestMeasurementAlarmDeviceErrorTriggers 验证多设备场景下某台设备采集失败会触发报警并携带 deviceId。
func TestMeasurementAlarmDeviceErrorTriggers(t *testing.T) {
	svc, _ := setupMeasurementService()
	svc.SetConfig(domain.WorkflowConfig{
		MinPressure:    0,
		MaxPressure:    100,
		PrecisionLevel: 0.0004,
	})
	svc.SetAlarmConfig(domain.AlarmConfig{
		Enabled:         true,
		EnabledChannels: []int{1},
		ConfirmOnAlarm:  true,
	})

	point := domain.PressurePoint{
		Index:          1,
		TargetPressure: 100,
		ActualPressure: float64Ptr(100.05),
		CollectedByDevice: map[string]domain.DevicePointData{
			"dev-a": {DeviceID: "dev-a", Collected: []float64{100.05}, Status: domain.PointStatusCompleted},
			"dev-b": {DeviceID: "dev-b", Status: domain.PointStatusError, Error: "collect sample 1: timeout"},
		},
	}

	alarm, err := svc.CheckAlarm(point)
	if err != nil {
		t.Fatalf("CheckAlarm: %v", err)
	}
	if alarm == nil {
		t.Fatal("expected alarm for device error")
	}
	if alarm.DeviceID != "dev-b" {
		t.Fatalf("expected deviceId dev-b, got %q", alarm.DeviceID)
	}
	if alarm.ErrorMessage == "" {
		t.Fatal("expected error message on alarm")
	}
}

func TestMeasurementAlarmBlocksWhenConfirmRequired(t *testing.T) {
	svc, _ := setupMeasurementService()
	svc.SetConfig(domain.WorkflowConfig{
		MinPressure:    0,
		MaxPressure:    100,
		PrecisionLevel: 0.0004, // 0.04% FS, allowance=0.04, dev=0.05 > 0.04
	})
	svc.SetAlarmConfig(domain.AlarmConfig{
		Enabled:         true,
		EnabledChannels: []int{1},
		ConfirmOnAlarm:  true,
	})

	point := domain.PressurePoint{
		Index:          1,
		TargetPressure: 100,
		ActualPressure: float64Ptr(100.05),
		CollectedData:  []float64{100.05},
	}

	alarm, err := svc.CheckAlarm(point)
	if err != nil {
		t.Fatalf("CheckAlarm: %v", err)
	}
	if alarm == nil {
		t.Fatal("expected alarm")
	}

	if !svc.IsAlarmPending() {
		t.Fatal("expected alarm to be pending")
	}

	err = svc.ResolveAlarm("continue")
	if err != nil {
		t.Fatalf("ResolveAlarm: %v", err)
	}

	if svc.IsAlarmPending() {
		t.Fatal("expected alarm to be resolved")
	}
}

// TestMeasurementAlarmZeroSpanFallback 验证量程为 0 时降级为按目标值比例计算，
// 且 MaxDeviation 不出现 NaN/Inf。
func TestMeasurementAlarmZeroSpanFallback(t *testing.T) {
	svc, _ := setupMeasurementService()
	svc.SetConfig(domain.WorkflowConfig{
		MinPressure:    100,
		MaxPressure:    100, // span = 0
		PrecisionLevel: 0.01,
	})
	svc.SetAlarmConfig(domain.AlarmConfig{
		Enabled:         true,
		EnabledChannels: []int{1},
	})

	point := domain.PressurePoint{
		Index:          1,
		TargetPressure: 100,
		ActualPressure: float64Ptr(100),
		CollectedData:  []float64{102}, // 偏差 2，超过 100*0.01=1
	}

	alarm, err := svc.CheckAlarm(point)
	if err != nil {
		t.Fatalf("CheckAlarm: %v", err)
	}
	if alarm == nil {
		t.Fatal("expected alarm to be triggered")
	}
	// 验证 maxDeviation 是有限值，没有 NaN/Inf
	if math.IsNaN(alarm.MaxDeviation) || math.IsInf(alarm.MaxDeviation, 0) {
		t.Fatalf("MaxDeviation must be finite, got %v", alarm.MaxDeviation)
	}
	// span=0 退化到按目标值的比例：2/100 = 0.02
	if alarm.MaxDeviation < 0.0199 || alarm.MaxDeviation > 0.0201 {
		t.Fatalf("expected MaxDeviation ~0.02, got %v", alarm.MaxDeviation)
	}
}

// TestRunAutoCollectionRecollect 验证用户选 recollect 后重新仅采集，不重新打压。
func TestRunAutoCollectionRecollect(t *testing.T) {
	svc, mDrv, pDrv := setupMeasurementServiceWithPressure()
	pDrv.stable = true
	svc.SetConfig(domain.WorkflowConfig{
		MinPressure:    0,
		MaxPressure:    100,
		PointCount:     1,
		CustomPoints:   []float64{50},
		PrecisionLevel: 0.0004,
		StableWaitMs:   1,
		ControlMode:    domain.ControlModeAuto,
		PressureMode:   domain.PressureModeSingle,
	})
	svc.SetAlarmConfig(domain.AlarmConfig{
		Enabled:         true,
		EnabledChannels: []int{1},
		ConfirmOnAlarm:  true,
	})

	if _, err := svc.GeneratePressurePoints(); err != nil {
		t.Fatalf("GeneratePressurePoints: %v", err)
	}
	if err := svc.StartWorkflow(context.Background(), []int{1}); err != nil {
		t.Fatalf("StartWorkflow: %v", err)
	}

	// 第一次采集触发报警，等待用户决策时返回 recollect；第二次采集合格。
	mDrv.data = []float64{60} // 偏差 10，远超允许偏差 0.04

	done := make(chan error, 1)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	go func() {
		done <- svc.RunAutoCollection(ctx)
	}()

	// 等报警触发后选 recollect，并把采集数据调整为合格值。
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if svc.IsAlarmPending() {
			break
		}
		select {
		case err := <-done:
			t.Fatalf("RunAutoCollection exited early: %v", err)
		default:
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !svc.IsAlarmPending() {
		t.Fatal("expected alarm to be pending")
	}

	mDrv.data = []float64{50} // 重采集时返回合格数据
	pressurizeCountBefore := len(pDrv.targets)
	if err := svc.ResolveAlarm("recollect"); err != nil {
		t.Fatalf("ResolveAlarm: %v", err)
	}

	if err := <-done; err != nil {
		t.Fatalf("RunAutoCollection: %v", err)
	}

	// recollect 后不能再次打压（targets 数量保持）。
	if got := len(pDrv.targets); got != pressurizeCountBefore {
		t.Fatalf("expected no additional pressurize after recollect, got %d -> %d", pressurizeCountBefore, got)
	}
}

// TestRunAutoCollectionContinueAfterAlarm 回归测试：用户在报警对话框选 continue 后，
// 状态机必须能从 await_alarm_resolution 合法迁回 collecting，并继续完成后续测点。
// 此前的实现在第二个点触发报警时 continue 后会因状态机非法迁移而中断自动采集。
func TestRunAutoCollectionContinueAfterAlarm(t *testing.T) {
	svc, mDrv, pDrv := setupMeasurementServiceWithPressure()
	pDrv.stable = true
	svc.SetConfig(domain.WorkflowConfig{
		MinPressure:    0,
		MaxPressure:    100,
		PointCount:     3,
		CustomPoints:   []float64{25, 50, 75},
		PrecisionLevel: 0.0004,
		StableWaitMs:   1,
		ControlMode:    domain.ControlModeAuto,
		PressureMode:   domain.PressureModeSingle,
	})
	svc.SetAlarmConfig(domain.AlarmConfig{
		Enabled:         true,
		EnabledChannels: []int{1},
		ConfirmOnAlarm:  true,
	})

	if _, err := svc.GeneratePressurePoints(); err != nil {
		t.Fatalf("GeneratePressurePoints: %v", err)
	}
	if err := svc.StartWorkflow(context.Background(), []int{1}); err != nil {
		t.Fatalf("StartWorkflow: %v", err)
	}

	// 让每个点都触发报警（偏差远超允许偏差 0.04），以验证 continue 后能继续到第3点。
	mDrv.data = []float64{60}

	done := make(chan error, 1)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	go func() {
		done <- svc.RunAutoCollection(ctx)
	}()

	// 期待 3 次报警，每次都选 continue。
	for i := 1; i <= 3; i++ {
		deadline := time.Now().Add(2 * time.Second)
		for time.Now().Before(deadline) {
			if svc.IsAlarmPending() {
				break
			}
			select {
			case err := <-done:
				t.Fatalf("RunAutoCollection exited early at point %d: %v", i, err)
			default:
			}
			time.Sleep(5 * time.Millisecond)
		}
		if !svc.IsAlarmPending() {
			t.Fatalf("expected alarm pending at point %d", i)
		}
		if err := svc.ResolveAlarm("continue"); err != nil {
			t.Fatalf("ResolveAlarm continue at point %d: %v", i, err)
		}
	}

	if err := <-done; err != nil {
		t.Fatalf("RunAutoCollection: %v", err)
	}

	// 3 个点应全部完成打压。
	if got := len(pDrv.targets); got < 3 {
		t.Fatalf("expected 3 pressurize targets, got %d", got)
	}
	if st := svc.State(); st != domain.SessionStateCompleted {
		t.Fatalf("expected completed state, got %s", st)
	}
}

// fakeStabilityPressureDriver 模拟 SCPI 打压设备，实现 StabilityStatusProvider。
type fakeStabilityPressureDriver struct {
	fakePressureDriver
	deviceStable bool
}

func (d *fakeStabilityPressureDriver) IsStable(_ context.Context) (bool, error) {
	return d.deviceStable, nil
}

func TestManualPressurizeSCPIStability(t *testing.T) {
	_, mDrv, pDrv := setupMeasurementServiceWithPressure()
	stableDrv := &fakeStabilityPressureDriver{
		fakePressureDriver: *pDrv,
		deviceStable:       true,
	}
	store := &fakeStore{devices: map[string]domain.Device{
		"m1": {ID: "m1", Type: domain.DeviceTypeMeasure, Model: "WTN1604", Host: "127.0.0.1", Port: 9000},
		"p1": {ID: "p1", Type: domain.DeviceTypePressure, Model: "ConST811A", Host: "127.0.0.1", Port: 9001},
	}}
	sessSvc := session.NewService(store, driver.NewFactory(), func(string, any) {}, &mapProvider{
		drivers: map[string]device.ConnectionDriver{
			"m1": embedMD{mDrv},
			"p1": embedPD{stableDrv},
		},
	})
	_, _ = sessSvc.BindDevices("m1", "p1", "test")
	svc := measurement.NewService(sessSvc, func(string, any) {}, workflow.NewWorkflowCoordinator())

	svc.SetConfig(domain.WorkflowConfig{
		MinPressure:        0,
		MaxPressure:        20,
		PointCount:         2,
		Precision:          2,
		AverageCount:       1,
		StableWaitMs:       10,
		StabilityTimeoutMs: 5000,
	})
	if _, err := svc.GeneratePressurePoints(); err != nil {
		t.Fatalf("GeneratePressurePoints: %v", err)
	}
	if err := svc.StartWorkflow(context.Background(), []int{1, 2}); err != nil {
		t.Fatalf("StartWorkflow: %v", err)
	}

	// 使用设备判稳路径执行打压，加压后状态应为 stabilizing（采集成功后变为 completed）
	if err := svc.ManualPressurize(context.Background(), 1); err != nil {
		t.Fatalf("ManualPressurize with SCPI stability: %v", err)
	}

	points := svc.GetPoints()
	if points[0].Status != domain.PointStatusStabilizing {
		t.Fatalf("expected first point stabilizing after pressurize, got %+v", points[0])
	}
}
