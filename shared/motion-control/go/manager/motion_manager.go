package manager

import (
	"context"
	"fmt"
	"reflect"
	"sync"

	"shared.local/device-sdk/go/motion/core"
	"shared.local/device-sdk/go/motion/ports"
)

type MotionControllerFactory func(profile core.MotionControllerProfile) (ports.MotionController, error)

type MotionManager struct {
	mu                sync.RWMutex
	controllers       map[string]ports.MotionController
	profiles          []core.MotionControllerProfile
	profileStore      ports.MotionProfileStore
	controllerFactory MotionControllerFactory
}

func NewMotionManager(profileStore ports.MotionProfileStore, factory MotionControllerFactory) *MotionManager {
	return &MotionManager{
		controllers:       make(map[string]ports.MotionController),
		profileStore:      profileStore,
		controllerFactory: factory,
	}
}

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

func (m *MotionManager) SaveProfiles(profiles []core.MotionControllerProfile) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.profiles = profiles
	if m.profileStore != nil {
		return m.profileStore.SaveProfiles(profiles)
	}
	return nil
}

func (m *MotionManager) GetProfiles() []core.MotionControllerProfile {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return append([]core.MotionControllerProfile(nil), m.profiles...)
}

func (m *MotionManager) UpsertProfile(profile core.MotionControllerProfile) error {
	// 边界兜底：阻断物理不可能的编码器补偿配置（如容差小于编码器分辨率，永不收敛）。
	// 前端校验可被绕过（直接改 profile 文件等），此处作为最后防线。仅阻断 error 级，
	// warning 级（可忽略的建议）由前端展示，不阻断保存。
	if err := validateProfileCompensation(profile); err != nil {
		return err
	}

	var toDisconnect ports.MotionController
	var toApplyConfig ports.MotionController

	m.mu.Lock()
	found := false
	for i, existing := range m.profiles {
		if existing.ID == profile.ID {
			connectionChanged := existing.Type != profile.Type ||
				existing.Address != profile.Address ||
				existing.Port != profile.Port
			axesChanged := !reflect.DeepEqual(existing.Axes, profile.Axes)

			if connectionChanged {
				if ctrl, ok := m.controllers[profile.ID]; ok {
					toDisconnect = ctrl
					delete(m.controllers, profile.ID)
				}
			} else if axesChanged {
				if ctrl, ok := m.controllers[profile.ID]; ok {
					toApplyConfig = ctrl
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
		err := m.profileStore.SaveProfiles(m.profiles)
		m.mu.Unlock()
		if toDisconnect != nil {
			_ = toDisconnect.Disconnect(context.Background())
		}
		if toApplyConfig != nil {
			_ = toApplyConfig.ApplyConfig(context.Background(), profile)
		}
		return err
	}
	m.mu.Unlock()

	if toDisconnect != nil {
		_ = toDisconnect.Disconnect(context.Background())
	}
	if toApplyConfig != nil {
		_ = toApplyConfig.ApplyConfig(context.Background(), profile)
	}
	return nil
}

func (m *MotionManager) DeleteProfile(id string) error {
	m.mu.Lock()
	newProfiles := make([]core.MotionControllerProfile, 0, len(m.profiles))
	for _, profile := range m.profiles {
		if profile.ID != id {
			newProfiles = append(newProfiles, profile)
		}
	}
	m.profiles = newProfiles

	var toDisconnect ports.MotionController
	if ctrl, ok := m.controllers[id]; ok {
		toDisconnect = ctrl
		delete(m.controllers, id)
	}

	if m.profileStore != nil {
		err := m.profileStore.SaveProfiles(m.profiles)
		m.mu.Unlock()
		if toDisconnect != nil {
			_ = toDisconnect.Disconnect(context.Background())
		}
		return err
	}
	m.mu.Unlock()
	if toDisconnect != nil {
		_ = toDisconnect.Disconnect(context.Background())
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

func (m *MotionManager) Connect(ctx context.Context, id string) error {
	ctrl, err := m.getController(id)
	if err != nil {
		return err
	}
	return ctrl.Connect(ctx)
}

func (m *MotionManager) Disconnect(ctx context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if ctrl, exists := m.controllers[id]; exists {
		_ = ctrl.Disconnect(ctx)
		delete(m.controllers, id)
	}
	return nil
}

func (m *MotionManager) Status(ctx context.Context, id string) (core.ControllerStatus, error) {
	m.mu.RLock()
	ctrl, exists := m.controllers[id]
	m.mu.RUnlock()

	if !exists {
		return core.ControllerStatus{}, fmt.Errorf("controller not connected: %s", id)
	}
	return ctrl.Status(ctx)
}

func (m *MotionManager) StatusAll(ctx context.Context) []core.ControllerStatus {
	m.mu.RLock()
	controllers := make([]ports.MotionController, 0, len(m.controllers))
	controllerIDs := make([]string, 0, len(m.controllers))
	for id, ctrl := range m.controllers {
		controllerIDs = append(controllerIDs, id)
		controllers = append(controllers, ctrl)
	}
	profiles := make(map[string]core.MotionControllerProfile, len(m.profiles))
	for _, profile := range m.profiles {
		profiles[profile.ID] = profile
	}
	m.mu.RUnlock()

	if len(controllers) == 0 {
		return nil
	}

	type statusResult struct {
		id     string
		status core.ControllerStatus
		err    error
	}
	ch := make(chan statusResult, len(controllers))
	for i, ctrl := range controllers {
		go func(id string, c ports.MotionController) {
			s, err := c.Status(ctx)
			ch <- statusResult{id: id, status: s, err: err}
		}(controllerIDs[i], ctrl)
	}

	statuses := make([]core.ControllerStatus, 0, len(controllers))
	for range controllers {
		r := <-ch
		if profile, ok := profiles[r.id]; ok {
			r.status.Name = profile.Name
			r.status.Type = profile.Type
		}
		if r.err != nil {
			r.status.ID = r.id
			r.status.Connected = false
			r.status.LastError = r.err.Error()
		}
		statuses = append(statuses, r.status)
	}
	return statuses
}

func (m *MotionManager) MoveTo(ctx context.Context, id string, axis core.AxisName, position float64) error {
	ctrl, err := m.getController(id)
	if err != nil {
		return err
	}
	return ctrl.MoveTo(ctx, axis, position)
}

func (m *MotionManager) MoveBy(ctx context.Context, id string, axis core.AxisName, delta float64) error {
	ctrl, err := m.getController(id)
	if err != nil {
		return err
	}
	return ctrl.MoveBy(ctx, axis, delta)
}

func (m *MotionManager) Jog(ctx context.Context, id string, axis core.AxisName, velocity float64) error {
	ctrl, err := m.getController(id)
	if err != nil {
		return err
	}
	return ctrl.Jog(ctx, axis, velocity)
}

func (m *MotionManager) Home(ctx context.Context, id string, axis core.AxisName) error {
	ctrl, err := m.getController(id)
	if err != nil {
		return err
	}
	return ctrl.Home(ctx, axis)
}

func (m *MotionManager) Stop(ctx context.Context, id string, axis core.AxisName) error {
	m.mu.RLock()
	ctrl, exists := m.controllers[id]
	m.mu.RUnlock()

	if !exists {
		return fmt.Errorf("controller not connected: %s", id)
	}
	return ctrl.Stop(ctx, axis)
}

func (m *MotionManager) EmergencyStop(ctx context.Context, id string) error {
	m.mu.RLock()
	ctrl, exists := m.controllers[id]
	m.mu.RUnlock()

	if !exists {
		return fmt.Errorf("controller not connected: %s", id)
	}
	return ctrl.EmergencyStop(ctx)
}

func (m *MotionManager) ResetEmergencyStop(ctx context.Context, id string) error {
	m.mu.RLock()
	ctrl, exists := m.controllers[id]
	m.mu.RUnlock()

	if !exists {
		return fmt.Errorf("controller not connected: %s", id)
	}
	return ctrl.ResetEmergencyStop(ctx)
}

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
				{Name: core.AxisU, Enabled: true, Kind: core.AxisKindRotary, StepsPerRev: core.PtrFloat64(1.8), MicroSteps: core.PtrInt(4), Lead: core.PtrFloat64(4), GearRatio: core.PtrFloat64(1), MaxSpeed: core.PtrFloat64(10), PositionSource: core.PositionSourceRegister, EncoderScale: core.PtrFloat64(0.005)},
			},
		},
	}
}

// validateProfileCompensation 校验编码器补偿配置的物理合理性，仅阻断 error 级告警。
// 只对启用编码器位置源且启用补偿的轴校验，与运行时补偿生效条件一致。
func validateProfileCompensation(profile core.MotionControllerProfile) error {
	for _, axis := range profile.Axes {
		if !axis.Enabled || axis.PositionSource != core.PositionSourceEncoder {
			continue
		}
		if axis.EncoderCompensation == nil || !axis.EncoderCompensation.Enabled {
			continue
		}
		for _, w := range core.ValidateCompensationConfig(*axis.EncoderCompensation, axis) {
			if w.Severity == "error" {
				return fmt.Errorf("axis %s: %s", axis.Name, w.Message)
			}
		}
	}
	return nil
}
