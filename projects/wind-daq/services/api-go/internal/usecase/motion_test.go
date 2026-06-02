package usecase

import (
	"context"
	"sync"
	"testing"

	"shared.local/device-sdk/go/motion/core"
	"shared.local/device-sdk/go/motion/ports"
)

// mockMotionController 模拟运动控制器，用于测试
type mockMotionController struct {
	mu         sync.RWMutex
	profile    core.MotionControllerProfile
	status     core.ControllerStatus
	connected  bool
	estopped   bool
	lastMoveTo map[core.AxisName]float64
	lastMoveBy map[core.AxisName]float64
	lastJog    map[core.AxisName]float64
	homeCalled map[core.AxisName]bool
	stopCalled map[core.AxisName]bool
}

func newMockMotionController(profile core.MotionControllerProfile) *mockMotionController {
	return &mockMotionController{
		profile:    profile,
		connected:  false,
		estopped:   false,
		lastMoveTo: make(map[core.AxisName]float64),
		lastMoveBy: make(map[core.AxisName]float64),
		lastJog:    make(map[core.AxisName]float64),
		homeCalled: make(map[core.AxisName]bool),
		stopCalled: make(map[core.AxisName]bool),
		status: core.ControllerStatus{
			ID:   profile.ID,
			Name: profile.Name,
			Type: profile.Type,
			Axes: []core.AxisStatus{
				{Name: core.AxisX, Position: 0, Homed: false, Moving: false},
				{Name: core.AxisY, Position: 0, Homed: false, Moving: false},
				{Name: core.AxisZ, Position: 0, Homed: false, Moving: false},
			},
		},
	}
}

func (m *mockMotionController) GetProfile() core.MotionControllerProfile { return m.profile }
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
func (m *mockMotionController) Status(ctx context.Context) (core.ControllerStatus, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.status, nil
}
func (m *mockMotionController) MoveTo(ctx context.Context, axis core.AxisName, position float64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.lastMoveTo[axis] = position
	return nil
}
func (m *mockMotionController) MoveBy(ctx context.Context, axis core.AxisName, delta float64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.lastMoveBy[axis] = delta
	return nil
}
func (m *mockMotionController) Jog(ctx context.Context, axis core.AxisName, velocity float64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.lastJog[axis] = velocity
	return nil
}
func (m *mockMotionController) Home(ctx context.Context, axis core.AxisName) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.homeCalled[axis] = true
	return nil
}
func (m *mockMotionController) Stop(ctx context.Context, axis core.AxisName) error {
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
func (m *mockMotionController) DefinePosition(ctx context.Context, axis core.AxisName, position float64) error {
	return nil
}

var _ ports.MotionController = (*mockMotionController)(nil)

func testProfile() core.MotionControllerProfile {
	return core.MotionControllerProfile{
		ID:      "test-mc-1",
		Name:    "Test Controller",
		Type:    core.ControllerTypeSimulated,
		Address: "127.0.0.1",
		Port:    9000,
		Axes: []core.AxisConfig{
			{Name: core.AxisX, Enabled: true, Kind: core.AxisKindLinear, MaxSpeed: core.PtrFloat64(10)},
			{Name: core.AxisY, Enabled: true, Kind: core.AxisKindLinear, MaxSpeed: core.PtrFloat64(10)},
			{Name: core.AxisZ, Enabled: true, Kind: core.AxisKindLinear, MaxSpeed: core.PtrFloat64(10)},
		},
	}
}

func TestMotionManager_Connect(t *testing.T) {
	profile := testProfile()
	var ctrl *mockMotionController
	mgr := NewMotionManager(nil, func(p core.MotionControllerProfile) (ports.MotionController, error) {
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
	mgr := NewMotionManager(nil, func(p core.MotionControllerProfile) (ports.MotionController, error) {
		ctrl = newMockMotionController(p)
		return ctrl, nil
	})

	if err := mgr.UpsertProfile(profile); err != nil {
		t.Fatalf("UpsertProfile failed: %v", err)
	}
	if err := mgr.Connect(context.Background(), profile.ID); err != nil {
		t.Fatalf("Connect failed: %v", err)
	}

	if err := mgr.MoveTo(context.Background(), profile.ID, core.AxisX, 100.5); err != nil {
		t.Fatalf("MoveTo failed: %v", err)
	}

	if ctrl.lastMoveTo[core.AxisX] != 100.5 {
		t.Errorf("expected MoveTo X=100.5, got %v", ctrl.lastMoveTo[core.AxisX])
	}
}

func TestMotionManager_MoveBy(t *testing.T) {
	profile := testProfile()
	var ctrl *mockMotionController
	mgr := NewMotionManager(nil, func(p core.MotionControllerProfile) (ports.MotionController, error) {
		ctrl = newMockMotionController(p)
		return ctrl, nil
	})

	if err := mgr.UpsertProfile(profile); err != nil {
		t.Fatalf("UpsertProfile failed: %v", err)
	}
	if err := mgr.Connect(context.Background(), profile.ID); err != nil {
		t.Fatalf("Connect failed: %v", err)
	}

	if err := mgr.MoveBy(context.Background(), profile.ID, core.AxisY, -5.0); err != nil {
		t.Fatalf("MoveBy failed: %v", err)
	}

	if ctrl.lastMoveBy[core.AxisY] != -5.0 {
		t.Errorf("expected MoveBy Y=-5.0, got %v", ctrl.lastMoveBy[core.AxisY])
	}
}

func TestMotionManager_Jog(t *testing.T) {
	profile := testProfile()
	var ctrl *mockMotionController
	mgr := NewMotionManager(nil, func(p core.MotionControllerProfile) (ports.MotionController, error) {
		ctrl = newMockMotionController(p)
		return ctrl, nil
	})

	if err := mgr.UpsertProfile(profile); err != nil {
		t.Fatalf("UpsertProfile failed: %v", err)
	}
	if err := mgr.Connect(context.Background(), profile.ID); err != nil {
		t.Fatalf("Connect failed: %v", err)
	}

	if err := mgr.Jog(context.Background(), profile.ID, core.AxisZ, 3.0); err != nil {
		t.Fatalf("Jog failed: %v", err)
	}

	if ctrl.lastJog[core.AxisZ] != 3.0 {
		t.Errorf("expected Jog Z=3.0, got %v", ctrl.lastJog[core.AxisZ])
	}
}

func TestMotionManager_Home(t *testing.T) {
	profile := testProfile()
	var ctrl *mockMotionController
	mgr := NewMotionManager(nil, func(p core.MotionControllerProfile) (ports.MotionController, error) {
		ctrl = newMockMotionController(p)
		return ctrl, nil
	})

	if err := mgr.UpsertProfile(profile); err != nil {
		t.Fatalf("UpsertProfile failed: %v", err)
	}
	if err := mgr.Connect(context.Background(), profile.ID); err != nil {
		t.Fatalf("Connect failed: %v", err)
	}

	if err := mgr.Home(context.Background(), profile.ID, core.AxisX); err != nil {
		t.Fatalf("Home failed: %v", err)
	}

	if !ctrl.homeCalled[core.AxisX] {
		t.Error("expected Home to be called for X axis")
	}
}

func TestMotionManager_EmergencyStop(t *testing.T) {
	profile := testProfile()
	var ctrl *mockMotionController
	mgr := NewMotionManager(nil, func(p core.MotionControllerProfile) (ports.MotionController, error) {
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
	mgr := NewMotionManager(nil, func(p core.MotionControllerProfile) (ports.MotionController, error) {
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
	mgr := NewMotionManager(nil, func(p core.MotionControllerProfile) (ports.MotionController, error) {
		ctrl = newMockMotionController(p)
		return ctrl, nil
	})

	if err := mgr.UpsertProfile(profile); err != nil {
		t.Fatalf("UpsertProfile failed: %v", err)
	}
	if err := mgr.Connect(context.Background(), profile.ID); err != nil {
		t.Fatalf("Connect failed: %v", err)
	}

	if err := mgr.Stop(context.Background(), profile.ID, core.AxisX); err != nil {
		t.Fatalf("Stop failed: %v", err)
	}

	if !ctrl.stopCalled[core.AxisX] {
		t.Error("expected Stop to be called for X axis")
	}
}

func TestMotionManager_DeleteProfile(t *testing.T) {
	profile := testProfile()
	var ctrl *mockMotionController
	mgr := NewMotionManager(nil, func(p core.MotionControllerProfile) (ports.MotionController, error) {
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
	mgr := NewMotionManager(nil, func(p core.MotionControllerProfile) (ports.MotionController, error) {
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
	mgr := NewMotionManager(nil, func(p core.MotionControllerProfile) (ports.MotionController, error) {
		return newMockMotionController(p), nil
	})

	err := mgr.Connect(context.Background(), "nonexistent")
	if err == nil {
		t.Error("expected error when connecting to non-existent controller")
	}
}
