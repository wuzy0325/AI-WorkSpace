package usecase

import (
	"context"
	"sync"
	"testing"

	sharedcore "shared.local/device-sdk/go/motion/core"
	sharedports "shared.local/device-sdk/go/motion/ports"
	motionmanager "shared.local/motion-control/go/manager"

	"wind-daq/services/api-go/internal/core/motion"
	"wind-daq/services/api-go/internal/ports"
	"wind-daq/services/api-go/pkg/wiring"
)

type mockMotionController struct {
	mu         sync.RWMutex
	profile    sharedcore.MotionControllerProfile
	status     sharedcore.ControllerStatus
	connected  bool
	estopped   bool
	lastMoveTo map[sharedcore.AxisName]float64
	lastMoveBy map[sharedcore.AxisName]float64
	lastJog    map[sharedcore.AxisName]float64
	homeCalled map[sharedcore.AxisName]bool
	stopCalled map[sharedcore.AxisName]bool
}

func newMockMotionController(profile sharedcore.MotionControllerProfile) *mockMotionController {
	return &mockMotionController{
		profile:    profile,
		connected:  false,
		estopped:   false,
		lastMoveTo: make(map[sharedcore.AxisName]float64),
		lastMoveBy: make(map[sharedcore.AxisName]float64),
		lastJog:    make(map[sharedcore.AxisName]float64),
		homeCalled: make(map[sharedcore.AxisName]bool),
		stopCalled: make(map[sharedcore.AxisName]bool),
		status: sharedcore.ControllerStatus{
			ID:   profile.ID,
			Name: profile.Name,
			Type: profile.Type,
			Axes: []sharedcore.AxisStatus{
				{Name: sharedcore.AxisX, Position: 0, Homed: false, Moving: false},
				{Name: sharedcore.AxisY, Position: 0, Homed: false, Moving: false},
				{Name: sharedcore.AxisZ, Position: 0, Homed: false, Moving: false},
			},
		},
	}
}

func (m *mockMotionController) GetProfile() sharedcore.MotionControllerProfile { return m.profile }
func (m *mockMotionController) Connect(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.connected = true
	m.status.Connected = true
	return nil
}
func (m *mockMotionController) Disconnect(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.connected = false
	m.status.Connected = false
	return nil
}
func (m *mockMotionController) Status(ctx context.Context) (sharedcore.ControllerStatus, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.status, nil
}
func (m *mockMotionController) MoveTo(ctx context.Context, axis sharedcore.AxisName, position float64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.lastMoveTo[axis] = position
	return nil
}
func (m *mockMotionController) MoveBy(ctx context.Context, axis sharedcore.AxisName, delta float64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.lastMoveBy[axis] = delta
	return nil
}
func (m *mockMotionController) Jog(ctx context.Context, axis sharedcore.AxisName, velocity float64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.lastJog[axis] = velocity
	return nil
}
func (m *mockMotionController) Home(ctx context.Context, axis sharedcore.AxisName) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.homeCalled[axis] = true
	return nil
}
func (m *mockMotionController) Stop(ctx context.Context, axis sharedcore.AxisName) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.stopCalled[axis] = true
	return nil
}
func (m *mockMotionController) EmergencyStop(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.estopped = true
	m.status.EmergencyStopped = true
	return nil
}
func (m *mockMotionController) ResetEmergencyStop(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.estopped = false
	m.status.EmergencyStopped = false
	return nil
}
func (m *mockMotionController) DefinePosition(ctx context.Context, axis sharedcore.AxisName, position float64) error {
	return nil
}

// ApplyConfig 应用控制器配置：mock 实现更新内部 profile，便于测试配置热更新场景
func (m *mockMotionController) ApplyConfig(ctx context.Context, profile sharedcore.MotionControllerProfile) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.profile = profile
	return nil
}

var _ sharedports.MotionController = (*mockMotionController)(nil)

func testProfile() motion.MotionControllerProfile {
	return motion.MotionControllerProfile{
		ID:      "test-mc-1",
		Name:    "Test Controller",
		Type:    motion.ControllerTypeSimulated,
		Address: "127.0.0.1",
		Port:    9000,
		Axes: []motion.AxisConfig{
			{Name: motion.AxisX, Enabled: true, Kind: motion.AxisKindLinear, MaxSpeed: f64ptr(10)},
			{Name: motion.AxisY, Enabled: true, Kind: motion.AxisKindLinear, MaxSpeed: f64ptr(10)},
			{Name: motion.AxisZ, Enabled: true, Kind: motion.AxisKindLinear, MaxSpeed: f64ptr(10)},
		},
	}
}

func f64ptr(v float64) *float64 { return &v }

// newTestMotionManager 构造一个包装了 shared MotionManager 的 ports.MotionManager，
// 经 pkg/wiring 装配以匹配生产装配根，避免测试直接依赖 adapters。
func newTestMotionManager(factory func(sharedcore.MotionControllerProfile) (sharedports.MotionController, error)) ports.MotionManager {
	return wiring.WrapMotionManager(motionmanager.NewMotionManager(nil, factory))
}

func TestMotionManager_Connect(t *testing.T) {
	profile := testProfile()
	var ctrl *mockMotionController
	mgr := newTestMotionManager(func(p sharedcore.MotionControllerProfile) (sharedports.MotionController, error) {
		ctrl = newMockMotionController(p)
		return ctrl, nil
	})

	if err := mgr.UpsertProfile(profile); err != nil {
		t.Fatalf("UpsertProfile failed: %v", err)
	}

	if err := mgr.Connect(context.Background(), profile.ID); err != nil {
		t.Fatalf("Connect failed: %v", err)
	}

	if !ctrl.connected {
		t.Error("expected controller to be connected")
	}
}

func TestMotionManager_MoveTo(t *testing.T) {
	profile := testProfile()
	var ctrl *mockMotionController
	mgr := newTestMotionManager(func(p sharedcore.MotionControllerProfile) (sharedports.MotionController, error) {
		ctrl = newMockMotionController(p)
		return ctrl, nil
	})

	if err := mgr.UpsertProfile(profile); err != nil {
		t.Fatalf("UpsertProfile failed: %v", err)
	}
	if err := mgr.Connect(context.Background(), profile.ID); err != nil {
		t.Fatalf("Connect failed: %v", err)
	}

	if err := mgr.MoveTo(context.Background(), profile.ID, motion.AxisX, 100.5); err != nil {
		t.Fatalf("MoveTo failed: %v", err)
	}

	if ctrl.lastMoveTo[sharedcore.AxisX] != 100.5 {
		t.Errorf("expected MoveTo X=100.5, got %v", ctrl.lastMoveTo[sharedcore.AxisX])
	}
}

func TestMotionManager_MoveBy(t *testing.T) {
	profile := testProfile()
	var ctrl *mockMotionController
	mgr := newTestMotionManager(func(p sharedcore.MotionControllerProfile) (sharedports.MotionController, error) {
		ctrl = newMockMotionController(p)
		return ctrl, nil
	})

	if err := mgr.UpsertProfile(profile); err != nil {
		t.Fatalf("UpsertProfile failed: %v", err)
	}
	if err := mgr.Connect(context.Background(), profile.ID); err != nil {
		t.Fatalf("Connect failed: %v", err)
	}

	if err := mgr.MoveBy(context.Background(), profile.ID, motion.AxisY, -5.0); err != nil {
		t.Fatalf("MoveBy failed: %v", err)
	}

	if ctrl.lastMoveBy[sharedcore.AxisY] != -5.0 {
		t.Errorf("expected MoveBy Y=-5.0, got %v", ctrl.lastMoveBy[sharedcore.AxisY])
	}
}

func TestMotionManager_Jog(t *testing.T) {
	profile := testProfile()
	var ctrl *mockMotionController
	mgr := newTestMotionManager(func(p sharedcore.MotionControllerProfile) (sharedports.MotionController, error) {
		ctrl = newMockMotionController(p)
		return ctrl, nil
	})

	if err := mgr.UpsertProfile(profile); err != nil {
		t.Fatalf("UpsertProfile failed: %v", err)
	}
	if err := mgr.Connect(context.Background(), profile.ID); err != nil {
		t.Fatalf("Connect failed: %v", err)
	}

	if err := mgr.Jog(context.Background(), profile.ID, motion.AxisZ, 3.0); err != nil {
		t.Fatalf("Jog failed: %v", err)
	}

	if ctrl.lastJog[sharedcore.AxisZ] != 3.0 {
		t.Errorf("expected Jog Z=3.0, got %v", ctrl.lastJog[sharedcore.AxisZ])
	}
}

func TestMotionManager_Home(t *testing.T) {
	profile := testProfile()
	var ctrl *mockMotionController
	mgr := newTestMotionManager(func(p sharedcore.MotionControllerProfile) (sharedports.MotionController, error) {
		ctrl = newMockMotionController(p)
		return ctrl, nil
	})

	if err := mgr.UpsertProfile(profile); err != nil {
		t.Fatalf("UpsertProfile failed: %v", err)
	}
	if err := mgr.Connect(context.Background(), profile.ID); err != nil {
		t.Fatalf("Connect failed: %v", err)
	}

	if err := mgr.Home(context.Background(), profile.ID, motion.AxisX); err != nil {
		t.Fatalf("Home failed: %v", err)
	}

	if !ctrl.homeCalled[sharedcore.AxisX] {
		t.Error("expected Home to be called for X axis")
	}
}

func TestMotionManager_EmergencyStop(t *testing.T) {
	profile := testProfile()
	var ctrl *mockMotionController
	mgr := newTestMotionManager(func(p sharedcore.MotionControllerProfile) (sharedports.MotionController, error) {
		ctrl = newMockMotionController(p)
		return ctrl, nil
	})

	if err := mgr.UpsertProfile(profile); err != nil {
		t.Fatalf("UpsertProfile failed: %v", err)
	}
	if err := mgr.Connect(context.Background(), profile.ID); err != nil {
		t.Fatalf("Connect failed: %v", err)
	}

	if err := mgr.EmergencyStop(context.Background(), profile.ID); err != nil {
		t.Fatalf("EmergencyStop failed: %v", err)
	}

	if !ctrl.estopped {
		t.Error("expected controller to be emergency stopped")
	}
}

func TestMotionManager_ResetEmergencyStop(t *testing.T) {
	profile := testProfile()
	var ctrl *mockMotionController
	mgr := newTestMotionManager(func(p sharedcore.MotionControllerProfile) (sharedports.MotionController, error) {
		ctrl = newMockMotionController(p)
		return ctrl, nil
	})

	if err := mgr.UpsertProfile(profile); err != nil {
		t.Fatalf("UpsertProfile failed: %v", err)
	}
	if err := mgr.Connect(context.Background(), profile.ID); err != nil {
		t.Fatalf("Connect failed: %v", err)
	}

	if err := mgr.EmergencyStop(context.Background(), profile.ID); err != nil {
		t.Fatalf("EmergencyStop failed: %v", err)
	}
	if !ctrl.estopped {
		t.Fatal("expected controller to be emergency stopped after EStop")
	}

	if err := mgr.ResetEmergencyStop(context.Background(), profile.ID); err != nil {
		t.Fatalf("ResetEmergencyStop failed: %v", err)
	}

	if ctrl.estopped {
		t.Error("expected controller to not be emergency stopped after reset")
	}
}

func TestMotionManager_Stop(t *testing.T) {
	profile := testProfile()
	var ctrl *mockMotionController
	mgr := newTestMotionManager(func(p sharedcore.MotionControllerProfile) (sharedports.MotionController, error) {
		ctrl = newMockMotionController(p)
		return ctrl, nil
	})

	if err := mgr.UpsertProfile(profile); err != nil {
		t.Fatalf("UpsertProfile failed: %v", err)
	}
	if err := mgr.Connect(context.Background(), profile.ID); err != nil {
		t.Fatalf("Connect failed: %v", err)
	}

	if err := mgr.Stop(context.Background(), profile.ID, motion.AxisX); err != nil {
		t.Fatalf("Stop failed: %v", err)
	}

	if !ctrl.stopCalled[sharedcore.AxisX] {
		t.Error("expected Stop to be called for X axis")
	}
}

func TestMotionManager_DeleteProfile(t *testing.T) {
	profile := testProfile()
	var ctrl *mockMotionController
	mgr := newTestMotionManager(func(p sharedcore.MotionControllerProfile) (sharedports.MotionController, error) {
		ctrl = newMockMotionController(p)
		return ctrl, nil
	})

	if err := mgr.UpsertProfile(profile); err != nil {
		t.Fatalf("UpsertProfile failed: %v", err)
	}
	if err := mgr.Connect(context.Background(), profile.ID); err != nil {
		t.Fatalf("Connect failed: %v", err)
	}

	if err := mgr.DeleteProfile(profile.ID); err != nil {
		t.Fatalf("DeleteProfile failed: %v", err)
	}

	profiles := mgr.GetProfiles()
	for _, p := range profiles {
		if p.ID == profile.ID {
			t.Error("expected profile to be deleted")
		}
	}
}

func TestMotionManager_StatusAll(t *testing.T) {
	profile := testProfile()
	mgr := newTestMotionManager(func(p sharedcore.MotionControllerProfile) (sharedports.MotionController, error) {
		return newMockMotionController(p), nil
	})

	if err := mgr.UpsertProfile(profile); err != nil {
		t.Fatalf("UpsertProfile failed: %v", err)
	}
	if err := mgr.Connect(context.Background(), profile.ID); err != nil {
		t.Fatalf("Connect failed: %v", err)
	}

	statuses := mgr.StatusAll(context.Background())
	found := false
	for _, s := range statuses {
		if s.ID == profile.ID {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected to find status for test profile")
	}
}

func TestMotionManager_ConnectNonExistent(t *testing.T) {
	mgr := newTestMotionManager(func(p sharedcore.MotionControllerProfile) (sharedports.MotionController, error) {
		return newMockMotionController(p), nil
	})

	err := mgr.Connect(context.Background(), "nonexistent")
	if err == nil {
		t.Error("expected error when connecting to non-existent controller")
	}
}
