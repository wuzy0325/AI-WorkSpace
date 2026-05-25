package usecase

import (
	"fmt"
	"sync"

	"wind-daq/services/api-go/internal/core/motion"
	"wind-daq/services/api-go/internal/ports"
)

// MotionControllerFactory 创建运动控制器实例的工厂函数
type MotionControllerFactory func(profile motion.MotionControllerProfile) ports.MotionController

// MotionManager 运动控制器管理器
type MotionManager struct {
	mu                sync.RWMutex
	controllers       map[string]ports.MotionController
	profiles          []motion.MotionControllerProfile
	profileStore      ports.MotionProfileStore
	controllerFactory MotionControllerFactory
}

// NewMotionManager 创建运动控制器管理器
func NewMotionManager(profileStore ports.MotionProfileStore, factory MotionControllerFactory) *MotionManager {
	return &MotionManager{
		controllers:       make(map[string]ports.MotionController),
		profileStore:      profileStore,
		controllerFactory: factory,
	}
}

// LoadProfiles 加载控制器配置
func (m *MotionManager) LoadProfiles() ([]motion.MotionControllerProfile, error) {
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

	// 如果没有配置存储，返回默认配置
	if len(m.profiles) == 0 {
		m.profiles = []motion.MotionControllerProfile{
			{
				ID:          "sim-mc-1",
				Name:        "模拟控制器 1",
				Type:        motion.ControllerTypeSimulated,
				Address:     "127.0.0.1",
				Port:        9000,
				AutoConnect: false,
				Axes: []motion.AxisConfig{
					{Name: motion.AxisX, Enabled: true, Kind: motion.AxisKindLinear, MaxSpeed: ptrFloat64(10)},
					{Name: motion.AxisY, Enabled: true, Kind: motion.AxisKindLinear, MaxSpeed: ptrFloat64(10)},
					{Name: motion.AxisZ, Enabled: true, Kind: motion.AxisKindLinear, MaxSpeed: ptrFloat64(10)},
					{Name: motion.AxisU, Enabled: false, Kind: motion.AxisKindRotary, MaxSpeed: ptrFloat64(10)},
				},
			},
		}
	}

	return m.profiles, nil
}

// SaveProfiles 保存控制器配置
func (m *MotionManager) SaveProfiles(profiles []motion.MotionControllerProfile) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.profiles = profiles

	if m.profileStore != nil {
		return m.profileStore.SaveProfiles(profiles)
	}

	return nil
}

// GetProfiles 获取所有控制器配置
func (m *MotionManager) GetProfiles() []motion.MotionControllerProfile {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.profiles
}

// UpsertProfile 插入或更新控制器配置
func (m *MotionManager) UpsertProfile(profile motion.MotionControllerProfile) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// 查找是否已存在
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

	// 保存到存储
	if m.profileStore != nil {
		return m.profileStore.SaveProfiles(m.profiles)
	}

	return nil
}

// DeleteProfile 删除控制器配置
func (m *MotionManager) DeleteProfile(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// 从列表中删除
	newProfiles := make([]motion.MotionControllerProfile, 0)
	for _, p := range m.profiles {
		if p.ID != id {
			newProfiles = append(newProfiles, p)
		}
	}
	m.profiles = newProfiles

	// 断开并删除控制器实例
	if ctrl, ok := m.controllers[id]; ok {
		ctrl.Disconnect()
		delete(m.controllers, id)
	}

	// 保存到存储
	if m.profileStore != nil {
		return m.profileStore.SaveProfiles(m.profiles)
	}

	return nil
}

// getController 获取或创建控制器实例
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

	// 创建新的控制器实例
	m.mu.Lock()
	defer m.mu.Unlock()

	// 再次检查（可能在其他协程中已创建）
	if ctrl, exists = m.controllers[id]; exists {
		return ctrl, nil
	}

	if m.controllerFactory != nil {
		ctrl = m.controllerFactory(profile)
	} else {
		return nil, fmt.Errorf("no controller factory configured")
	}

	m.controllers[id] = ctrl
	return ctrl, nil
}

// findProfileLocked 查找配置（需要持有锁）
func (m *MotionManager) findProfileLocked(id string) (motion.MotionControllerProfile, bool) {
	for _, p := range m.profiles {
		if p.ID == id {
			return p, true
		}
	}
	return motion.MotionControllerProfile{}, false
}

// Connect 连接控制器
func (m *MotionManager) Connect(id string) error {
	ctrl, err := m.getController(id)
	if err != nil {
		return err
	}
	return ctrl.Connect()
}

// Disconnect 断开控制器
func (m *MotionManager) Disconnect(id string) error {
	m.mu.RLock()
	ctrl, exists := m.controllers[id]
	m.mu.RUnlock()

	if !exists {
		return nil
	}
	return ctrl.Disconnect()
}

// Status 获取控制器状态
func (m *MotionManager) Status(id string) (motion.ControllerStatus, error) {
	m.mu.RLock()
	ctrl, exists := m.controllers[id]
	m.mu.RUnlock()

	if !exists {
		return motion.ControllerStatus{}, fmt.Errorf("controller not connected: %s", id)
	}
	return ctrl.Status(), nil
}

// StatusAll 获取所有控制器状态
func (m *MotionManager) StatusAll() []motion.ControllerStatus {
	m.mu.RLock()
	defer m.mu.RUnlock()

	statuses := make([]motion.ControllerStatus, 0, len(m.controllers))
	for id, ctrl := range m.controllers {
		status := ctrl.Status()
		// 补充名称和类型信息
		for _, p := range m.profiles {
			if p.ID == id {
				status.Name = p.Name
				status.Type = p.Type
				break
			}
		}
		statuses = append(statuses, status)
	}

	return statuses
}

// MoveTo 移动到绝对位置
func (m *MotionManager) MoveTo(id string, axis motion.AxisName, position float64) error {
	ctrl, err := m.getController(id)
	if err != nil {
		return err
	}
	return ctrl.MoveTo(axis, position)
}

// MoveBy 相对移动
func (m *MotionManager) MoveBy(id string, axis motion.AxisName, delta float64) error {
	ctrl, err := m.getController(id)
	if err != nil {
		return err
	}
	return ctrl.MoveBy(axis, delta)
}

// Jog 点动运动
func (m *MotionManager) Jog(id string, axis motion.AxisName, velocity float64) error {
	ctrl, err := m.getController(id)
	if err != nil {
		return err
	}
	return ctrl.Jog(axis, velocity)
}

// Home 归原点
func (m *MotionManager) Home(id string, axis motion.AxisName) error {
	ctrl, err := m.getController(id)
	if err != nil {
		return err
	}
	return ctrl.Home(axis)
}

// Stop 停止轴运动
func (m *MotionManager) Stop(id string, axis motion.AxisName) error {
	m.mu.RLock()
	ctrl, exists := m.controllers[id]
	m.mu.RUnlock()

	if !exists {
		return nil
	}
	return ctrl.Stop(axis)
}

// EmergencyStop 紧急停止
func (m *MotionManager) EmergencyStop(id string) error {
	m.mu.RLock()
	ctrl, exists := m.controllers[id]
	m.mu.RUnlock()

	if !exists {
		return nil
	}
	return ctrl.EmergencyStop()
}

// DefinePosition 定义当前位置
func (m *MotionManager) DefinePosition(id string, axis motion.AxisName, position float64) error {
	ctrl, err := m.getController(id)
	if err != nil {
		return err
	}
	return ctrl.DefinePosition(axis, position)
}

// ptrFloat64 创建float64指针
func ptrFloat64(v float64) *float64 {
	return &v
}
