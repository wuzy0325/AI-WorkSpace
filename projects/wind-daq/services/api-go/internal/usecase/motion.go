package usecase

import (
	"context"
	"fmt"
	"sync"

	"shared/device-sdk/go/motion/core"
	"shared/device-sdk/go/motion/ports"
)

// MotionControllerFactory factory function for creating motion controller instances
type MotionControllerFactory func(profile core.MotionControllerProfile) (ports.MotionController, error)

// MotionManager motion controller manager
type MotionManager struct {
	mu                sync.RWMutex
	controllers       map[string]ports.MotionController
	profiles          []core.MotionControllerProfile
	profileStore      ports.MotionProfileStore
	controllerFactory MotionControllerFactory
}

// NewMotionManager create motion controller manager
func NewMotionManager(profileStore ports.MotionProfileStore, factory MotionControllerFactory) *MotionManager {
	return &MotionManager{
		controllers:       make(map[string]ports.MotionController),
		profileStore:      profileStore,
		controllerFactory: factory,
	}
}

// LoadProfiles load controller profiles
func (m *MotionManager) LoadProfiles() ([]core.MotionControllerProfile, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.profileStore != nil {
		profiles, err := m.profileStore.LoadProfiles()
		if err != nil {
			return nil, err
		}
		m.profiles = profiles
		return profiles, nil
	}

	if len(m.profiles) == 0 {
		m.profiles = []core.MotionControllerProfile{
			{
				ID:          "sim-mc-1",
				Name:        "Simulated Controller 1",
				Type:        core.ControllerTypeSimulated,
				Address:     "127.0.0.1",
				Port:        9000,
				AutoConnect: false,
				Axes: []core.AxisConfig{
					{Name: core.AxisX, Enabled: true, Kind: core.AxisKindLinear, StepsPerRev: core.PtrFloat64(1.8), MicroSteps: core.PtrInt(4), Lead: core.PtrFloat64(4), GearRatio: core.PtrFloat64(1), MaxSpeed: core.PtrFloat64(10), PositionSource: core.PositionSourceRegister, EncoderScale: core.PtrFloat64(0.005)},
					{Name: core.AxisY, Enabled: true, Kind: core.AxisKindLinear, StepsPerRev: core.PtrFloat64(1.8), MicroSteps: core.PtrInt(4), Lead: core.PtrFloat64(4), GearRatio: core.PtrFloat64(1), MaxSpeed: core.PtrFloat64(10), PositionSource: core.PositionSourceRegister, EncoderScale: core.PtrFloat64(0.005)},
					{Name: core.AxisZ, Enabled: true, Kind: core.AxisKindLinear, StepsPerRev: core.PtrFloat64(1.8), MicroSteps: core.PtrInt(4), Lead: core.PtrFloat64(4), GearRatio: core.PtrFloat64(1), MaxSpeed: core.PtrFloat64(10), PositionSource: core.PositionSourceRegister, EncoderScale: core.PtrFloat64(0.005)},
					{Name: core.AxisU, Enabled: false, Kind: core.AxisKindRotary, StepsPerRev: core.PtrFloat64(1.8), MicroSteps: core.PtrInt(4), Lead: core.PtrFloat64(4), GearRatio: core.PtrFloat64(1), MaxSpeed: core.PtrFloat64(10), PositionSource: core.PositionSourceRegister, EncoderScale: core.PtrFloat64(0.005)},
				},
			},
		}
	}

	return m.profiles, nil
}

// SaveProfiles save controller profiles
func (m *MotionManager) SaveProfiles(profiles []core.MotionControllerProfile) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.profiles = profiles

	if m.profileStore != nil {
		return m.profileStore.SaveProfiles(profiles)
	}

	return nil
}

// GetProfiles get all controller profiles
func (m *MotionManager) GetProfiles() []core.MotionControllerProfile {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.profiles
}

// UpsertProfile insert or update controller profile
func (m *MotionManager) UpsertProfile(profile core.MotionControllerProfile) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	found := false
	for i, p := range m.profiles {
		if p.ID == profile.ID {
			m.profiles[i] = profile
			found = true
			break
		}
	}

	if !found {
		m.profiles = append(m.profiles, profile)
	}

	if m.profileStore != nil {
		return m.profileStore.SaveProfiles(m.profiles)
	}

	return nil
}

// DeleteProfile delete controller profile
func (m *MotionManager) DeleteProfile(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	newProfiles := make([]core.MotionControllerProfile, 0)
	for _, p := range m.profiles {
		if p.ID != id {
			newProfiles = append(newProfiles, p)
		}
	}
	m.profiles = newProfiles

	if ctrl, ok := m.controllers[id]; ok {
		ctrl.Disconnect(context.Background())
		delete(m.controllers, id)
	}

	if m.profileStore != nil {
		return m.profileStore.SaveProfiles(m.profiles)
	}

	return nil
}

// getController get or create controller instance
func (m *MotionManager) getController(id string) (ports.MotionController, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if ctrl, exists := m.controllers[id]; exists {
		return ctrl, nil
	}

	profile, profileExists := m.findProfileLocked(id)
	if !profileExists {
		return nil, fmt.Errorf("controller not found: %s", id)
	}

	if m.controllerFactory == nil {
		return nil, fmt.Errorf("no controller factory configured")
	}

	ctrl, err := m.controllerFactory(profile)
	if err != nil {
		return nil, err
	}

	m.controllers[id] = ctrl
	return ctrl, nil
}

// findProfileLocked find profile (requires lock)
func (m *MotionManager) findProfileLocked(id string) (core.MotionControllerProfile, bool) {
	for _, p := range m.profiles {
		if p.ID == id {
			return p, true
		}
	}
	return core.MotionControllerProfile{}, false
}

// Connect connect controller
func (m *MotionManager) Connect(ctx context.Context, id string) error {
	ctrl, err := m.getController(id)
	if err != nil {
		return err
	}
	return ctrl.Connect(ctx)
}

// Disconnect disconnect controller
func (m *MotionManager) Disconnect(ctx context.Context, id string) error {
	m.mu.RLock()
	ctrl, exists := m.controllers[id]
	m.mu.RUnlock()

	if !exists {
		return nil
	}
	return ctrl.Disconnect(ctx)
}

// Status get controller status
func (m *MotionManager) Status(ctx context.Context, id string) (core.ControllerStatus, error) {
	m.mu.RLock()
	ctrl, exists := m.controllers[id]
	m.mu.RUnlock()

	if !exists {
		return core.ControllerStatus{}, fmt.Errorf("controller not connected: %s", id)
	}
	return ctrl.Status(ctx)
}

// StatusAll get all controller statuses
// 先提取控制器引用和配置信息后释放锁，再逐个查询，避免 Status（含 FFI/网络 I/O）阻塞写操作
func (m *MotionManager) StatusAll(ctx context.Context) []core.ControllerStatus {
	m.mu.RLock()
	controllers := make(map[string]ports.MotionController, len(m.controllers))
	for id, ctrl := range m.controllers {
		controllers[id] = ctrl
	}
	profileMap := make(map[string]core.MotionControllerProfile, len(m.profiles))
	for _, p := range m.profiles {
		profileMap[p.ID] = p
	}
	m.mu.RUnlock()

	statuses := make([]core.ControllerStatus, 0, len(controllers))
	for id, ctrl := range controllers {
		status, err := ctrl.Status(ctx)
		if err != nil {
			continue
		}
		if p, ok := profileMap[id]; ok {
			status.Name = p.Name
			status.Type = p.Type
		}
		statuses = append(statuses, status)
	}

	return statuses
}

// MoveTo move to absolute position
func (m *MotionManager) MoveTo(ctx context.Context, id string, axis core.AxisName, position float64) error {
	ctrl, err := m.getController(id)
	if err != nil {
		return err
	}
	return ctrl.MoveTo(ctx, axis, position)
}

// MoveBy relative move
func (m *MotionManager) MoveBy(ctx context.Context, id string, axis core.AxisName, delta float64) error {
	ctrl, err := m.getController(id)
	if err != nil {
		return err
	}
	return ctrl.MoveBy(ctx, axis, delta)
}

// Jog jog movement
func (m *MotionManager) Jog(ctx context.Context, id string, axis core.AxisName, velocity float64) error {
	ctrl, err := m.getController(id)
	if err != nil {
		return err
	}
	return ctrl.Jog(ctx, axis, velocity)
}

// Home home axis
func (m *MotionManager) Home(ctx context.Context, id string, axis core.AxisName) error {
	ctrl, err := m.getController(id)
	if err != nil {
		return err
	}
	return ctrl.Home(ctx, axis)
}

// Stop stop axis movement
func (m *MotionManager) Stop(ctx context.Context, id string, axis core.AxisName) error {
	m.mu.RLock()
	ctrl, exists := m.controllers[id]
	m.mu.RUnlock()

	if !exists {
		return nil
	}
	return ctrl.Stop(ctx, axis)
}

// EmergencyStop emergency stop
func (m *MotionManager) EmergencyStop(ctx context.Context, id string) error {
	m.mu.RLock()
	ctrl, exists := m.controllers[id]
	m.mu.RUnlock()

	if !exists {
		return nil
	}
	return ctrl.EmergencyStop(ctx)
}

// DefinePosition define current position
func (m *MotionManager) DefinePosition(ctx context.Context, id string, axis core.AxisName, position float64) error {
	ctrl, err := m.getController(id)
	if err != nil {
		return err
	}
	return ctrl.DefinePosition(ctx, axis, position)
}

// ResetEmergencyStop 重置急停状态，恢复控制器正常操作
func (m *MotionManager) ResetEmergencyStop(ctx context.Context, id string) error {
	ctrl, err := m.getController(id)
	if err != nil {
		return err
	}
	return ctrl.ResetEmergencyStop(ctx)
}
