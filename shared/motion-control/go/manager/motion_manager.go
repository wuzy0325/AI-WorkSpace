package manager

import (
	"context"
	"fmt"
	"reflect"
	"sync"

	"shared.local/device-sdk/go/motion/core"
	"shared.local/device-sdk/go/motion/ports"
)

// MotionControllerFactory creates motion controller instances from profiles.
type MotionControllerFactory func(profile core.MotionControllerProfile) (ports.MotionController, error)

// MotionManager coordinates controller profiles and command dispatch.
type MotionManager struct {
	mu                sync.RWMutex
	controllers       map[string]ports.MotionController
	profiles          []core.MotionControllerProfile
	profileStore      ports.MotionProfileStore
	controllerFactory MotionControllerFactory
}

// NewMotionManager creates a motion controller manager.
func NewMotionManager(profileStore ports.MotionProfileStore, factory MotionControllerFactory) *MotionManager {
	return &MotionManager{
		controllers:       make(map[string]ports.MotionController),
		profileStore:      profileStore,
		controllerFactory: factory,
	}
}

// LoadProfiles loads controller profiles from the configured store.
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
		m.profiles = defaultProfiles()
	}

	return m.profiles, nil
}

// SaveProfiles replaces and persists all controller profiles.
func (m *MotionManager) SaveProfiles(profiles []core.MotionControllerProfile) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.profiles = profiles
	if m.profileStore != nil {
		return m.profileStore.SaveProfiles(profiles)
	}
	return nil
}

// GetProfiles returns all known controller profiles.
func (m *MotionManager) GetProfiles() []core.MotionControllerProfile {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return append([]core.MotionControllerProfile(nil), m.profiles...)
}

// UpsertProfile inserts or updates a controller profile.
func (m *MotionManager) UpsertProfile(profile core.MotionControllerProfile) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	found := false
	for i, existing := range m.profiles {
		if existing.ID == profile.ID {
			if motionControllerConfigChanged(existing, profile) {
				if ctrl, ok := m.controllers[profile.ID]; ok {
					_ = ctrl.Disconnect(context.Background())
					delete(m.controllers, profile.ID)
				}
			}
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

func motionControllerConfigChanged(oldProfile, newProfile core.MotionControllerProfile) bool {
	return oldProfile.Type != newProfile.Type ||
		oldProfile.Address != newProfile.Address ||
		oldProfile.Port != newProfile.Port ||
		!reflect.DeepEqual(oldProfile.Axes, newProfile.Axes)
}

// DeleteProfile deletes a controller profile and disconnects its controller.
func (m *MotionManager) DeleteProfile(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	newProfiles := make([]core.MotionControllerProfile, 0, len(m.profiles))
	for _, profile := range m.profiles {
		if profile.ID != id {
			newProfiles = append(newProfiles, profile)
		}
	}
	m.profiles = newProfiles

	if ctrl, ok := m.controllers[id]; ok {
		_ = ctrl.Disconnect(context.Background())
		delete(m.controllers, id)
	}

	if m.profileStore != nil {
		return m.profileStore.SaveProfiles(m.profiles)
	}
	return nil
}

func (m *MotionManager) getController(id string) (ports.MotionController, error) {
	m.mu.RLock()
	ctrl, exists := m.controllers[id]
	profile, profileExists := m.findProfileLocked(id)
	m.mu.RUnlock()

	if exists {
		return ctrl, nil
	}
	if !profileExists {
		return nil, fmt.Errorf("controller not found: %s", id)
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if ctrl, exists = m.controllers[id]; exists {
		return ctrl, nil
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

func (m *MotionManager) findProfileLocked(id string) (core.MotionControllerProfile, bool) {
	for _, profile := range m.profiles {
		if profile.ID == id {
			return profile, true
		}
	}
	return core.MotionControllerProfile{}, false
}

// Connect connects a controller.
func (m *MotionManager) Connect(ctx context.Context, id string) error {
	ctrl, err := m.getController(id)
	if err != nil {
		return err
	}
	return ctrl.Connect(ctx)
}

// Disconnect disconnects a controller.
func (m *MotionManager) Disconnect(ctx context.Context, id string) error {
	m.mu.RLock()
	ctrl, exists := m.controllers[id]
	m.mu.RUnlock()

	if !exists {
		return nil
	}
	return ctrl.Disconnect(ctx)
}

// Status returns a connected controller status.
func (m *MotionManager) Status(ctx context.Context, id string) (core.ControllerStatus, error) {
	m.mu.RLock()
	ctrl, exists := m.controllers[id]
	m.mu.RUnlock()

	if !exists {
		return core.ControllerStatus{}, fmt.Errorf("controller not connected: %s", id)
	}
	return ctrl.Status(ctx)
}

// StatusAll returns all connected controller statuses.
func (m *MotionManager) StatusAll(ctx context.Context) []core.ControllerStatus {
	m.mu.RLock()
	controllers := make(map[string]ports.MotionController, len(m.controllers))
	for id, ctrl := range m.controllers {
		controllers[id] = ctrl
	}
	profiles := make(map[string]core.MotionControllerProfile, len(m.profiles))
	for _, profile := range m.profiles {
		profiles[profile.ID] = profile
	}
	m.mu.RUnlock()

	statuses := make([]core.ControllerStatus, 0, len(controllers))
	for id, ctrl := range controllers {
		status, err := ctrl.Status(ctx)
		if err != nil {
			continue
		}
		if profile, ok := profiles[id]; ok {
			status.Name = profile.Name
			status.Type = profile.Type
		}
		statuses = append(statuses, status)
	}
	return statuses
}

// MoveTo moves an axis to an absolute position.
func (m *MotionManager) MoveTo(ctx context.Context, id string, axis core.AxisName, position float64) error {
	ctrl, err := m.getController(id)
	if err != nil {
		return err
	}
	return ctrl.MoveTo(ctx, axis, position)
}

// MoveBy moves an axis by a relative delta.
func (m *MotionManager) MoveBy(ctx context.Context, id string, axis core.AxisName, delta float64) error {
	ctrl, err := m.getController(id)
	if err != nil {
		return err
	}
	return ctrl.MoveBy(ctx, axis, delta)
}

// Jog starts jogging an axis.
func (m *MotionManager) Jog(ctx context.Context, id string, axis core.AxisName, velocity float64) error {
	ctrl, err := m.getController(id)
	if err != nil {
		return err
	}
	return ctrl.Jog(ctx, axis, velocity)
}

// Home homes an axis.
func (m *MotionManager) Home(ctx context.Context, id string, axis core.AxisName) error {
	ctrl, err := m.getController(id)
	if err != nil {
		return err
	}
	return ctrl.Home(ctx, axis)
}

// Stop stops an axis. Empty axis stops all moving axes individually.
func (m *MotionManager) Stop(ctx context.Context, id string, axis core.AxisName) error {
	m.mu.RLock()
	ctrl, exists := m.controllers[id]
	m.mu.RUnlock()

	if !exists {
		return nil
	}
	return ctrl.Stop(ctx, axis)
}

// EmergencyStop issues controller emergency stop.
func (m *MotionManager) EmergencyStop(ctx context.Context, id string) error {
	m.mu.RLock()
	ctrl, exists := m.controllers[id]
	m.mu.RUnlock()

	if !exists {
		return nil
	}
	return ctrl.EmergencyStop(ctx)
}

// ResetEmergencyStop resets emergency stop state for a connected controller.
func (m *MotionManager) ResetEmergencyStop(ctx context.Context, id string) error {
	m.mu.RLock()
	ctrl, exists := m.controllers[id]
	m.mu.RUnlock()

	if !exists {
		return nil
	}
	return ctrl.ResetEmergencyStop(ctx)
}

// DefinePosition defines the current axis position.
func (m *MotionManager) DefinePosition(ctx context.Context, id string, axis core.AxisName, position float64) error {
	ctrl, err := m.getController(id)
	if err != nil {
		return err
	}
	return ctrl.DefinePosition(ctx, axis, position)
}

func defaultProfiles() []core.MotionControllerProfile {
	return []core.MotionControllerProfile{
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
