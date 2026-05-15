package usecase

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"wind-daq/services/api-go/internal/core/motion"
	"wind-daq/services/api-go/internal/ports"
)

// ==================== 运动控制器管理器 ====================
// 负责多轴运动控制的 lifecycle 管理
// 支持: 配置管理、连接管理、运动指令、状态轮询

// 状态轮询间隔(10Hz = 100ms)
const motionPollInterval = 100 * time.Millisecond

// MotionInstance 运动控制器运行时实例
// 包含: 配置、运行时状态、硬件驱动
type MotionInstance struct {
	Profile motion.MotionControllerProfile // 配置模板
	Status  motion.MotionControllerStatus  // 运行时状态
	Driver  ports.MotionController         // 硬件驱动接口
}

// MotionManager 运动控制器管理器
// 管理多个运动控制器的配置、连接和运动指令
type MotionManager struct {
	mu         sync.RWMutex
	instances  map[string]*MotionInstance // id -> 实例映射
	publisher  ports.DataPublisher        // 数据广播接口
	factory    ports.MotionFactory        // 运动控制器工厂
	pollCancel context.CancelFunc         // 状态轮询取消函数
}

// NewMotionManager 构建运动控制器管理器
// 参数: publisher 数据广播接口, factory 运动控制器工厂
// 返回: *MotionManager 管理器实例
func NewMotionManager(publisher ports.DataPublisher, factory ports.MotionFactory) *MotionManager {
	return &MotionManager{
		instances: make(map[string]*MotionInstance),
		publisher: publisher,
		factory:   factory,
	}
}

// LoadProfiles 加载配置并创建实例
// 根据配置列表批量创建运动控制器实例(不自动连接)
// 参数: profiles 配置列表
func (m *MotionManager) LoadProfiles(profiles []motion.MotionControllerProfile) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, p := range profiles {
		m.instances[p.ID] = &MotionInstance{
			Profile: p,
			Status: motion.MotionControllerStatus{
				ID:   p.ID,
				Name: p.Name,
				Type: p.Type,
			},
			Driver: m.factory.Create(p),
		}
	}
}

// GetProfiles 获取所有配置
// 返回: []motion.MotionControllerProfile 配置列表
func (m *MotionManager) GetProfiles() []motion.MotionControllerProfile {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make([]motion.MotionControllerProfile, 0, len(m.instances))
	for _, inst := range m.instances {
		result = append(result, inst.Profile)
	}
	return result
}

// UpsertProfile 添加或更新配置
// 如果配置需要重连(如IP/端口/类型变化),会断开旧驱动并创建新驱动
// 参数: profile 运动控制器配置
func (m *MotionManager) UpsertProfile(profile motion.MotionControllerProfile) {
	m.mu.Lock()
	inst, ok := m.instances[profile.ID]
	// 不存在则创建新实例
	if !ok {
		m.instances[profile.ID] = &MotionInstance{
			Profile: profile,
			Status: motion.MotionControllerStatus{
				ID:   profile.ID,
				Name: profile.Name,
				Type: profile.Type,
			},
			Driver: m.factory.Create(profile),
		}
		m.mu.Unlock()
		return
	}

	// 已存在配置,检查是否需要重连
	wasConnected := inst.Driver.IsConnected()
	requireReconnect := profileRequiresReconnect(inst.Profile, profile)
	oldDriver := inst.Driver

	// 更新配置
	inst.Profile = profile
	inst.Status.ID = profile.ID
	inst.Status.Name = profile.Name
	inst.Status.Type = profile.Type

	// 不需要重连则只更新配置
	if !requireReconnect {
		inst.Driver.UpdateProfile(profile)
		m.mu.Unlock()
		return
	}

	// 需要重连:创建新驱动
	newDriver := m.factory.Create(profile)
	inst.Driver = newDriver
	inst.Status.Connected = false
	inst.Status.Axes = nil
	inst.Status.LastError = ""
	m.mu.Unlock()

	// 如果之前未连接,直接返回
	if !wasConnected {
		return
	}

	// 之前已连接:断开旧驱动,连接新驱动
	if err := oldDriver.Disconnect(); err != nil {
		slog.Warn("Disconnect old motion driver fail during profile reconnect", "id", profile.ID, "err", err)
	}

	if err := newDriver.Connect(); err != nil {
		m.mu.Lock()
		if latest, exists := m.instances[profile.ID]; exists && latest.Driver == newDriver {
			latest.Status.Connected = false
			latest.Status.LastError = err.Error()
		}
		m.mu.Unlock()
		return
	}

	// 获取新驱动状态
	axes, axesErr := newDriver.GetAllAxisStatus()

	m.mu.Lock()
	if latest, exists := m.instances[profile.ID]; !exists || latest.Driver != newDriver {
		m.mu.Unlock()
		_ = newDriver.Disconnect()
		return
	}
	inst = m.instances[profile.ID]
	inst.Status.Connected = true
	if axesErr != nil {
		inst.Status.LastError = axesErr.Error()
	} else {
		inst.Status.Axes = axes
		inst.Status.LastError = buildAxisStatusError(axes)
	}
	m.mu.Unlock()

	if axesErr != nil {
		slog.Debug("Motion status refresh after profile reconnect failed", "id", profile.ID, "err", axesErr)
	}
}

// profileRequiresReconnect 检查配置变更是否需要重连
// 如果IP/端口/类型变化则需要重连
func profileRequiresReconnect(current motion.MotionControllerProfile, next motion.MotionControllerProfile) bool {
	return current.Address != next.Address || current.Port != next.Port || current.Type != next.Type
}

// DeleteProfile 删除配置
// 如果控制器已连接则先断开
// 参数: id 配置ID
// 返回: error 错误信息
func (m *MotionManager) DeleteProfile(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if inst, ok := m.instances[id]; ok {
		if inst.Driver.IsConnected() {
			inst.Driver.Disconnect()
		}
		delete(m.instances, id)
	}
	return nil
}

// Connect 连接运动控制器
// 参数: id 控制器ID
// 返回: error 错误信息
func (m *MotionManager) Connect(id string) error {
	m.mu.Lock()
	inst, ok := m.instances[id]
	if !ok {
		m.mu.Unlock()
		return ErrMotionNotFound
	}
	m.mu.Unlock()

	// 执行连接
	if err := inst.Driver.Connect(); err != nil {
		m.mu.Lock()
		inst.Status.Connected = false
		inst.Status.LastError = err.Error()
		m.mu.Unlock()
		return err
	}

	m.mu.Lock()
	inst.Status.Connected = true
	inst.Status.LastError = ""
	m.mu.Unlock()
	return nil
}

// Disconnect 断开运动控制器
// 参数: id 控制器ID
// 返回: error 错误信息
func (m *MotionManager) Disconnect(id string) error {
	m.mu.Lock()
	inst, ok := m.instances[id]
	if !ok {
		m.mu.Unlock()
		return ErrMotionNotFound
	}
	m.mu.Unlock()

	if err := inst.Driver.Disconnect(); err != nil {
		return err
	}
	m.mu.Lock()
	inst.Status.Connected = false
	m.mu.Unlock()
	return nil
}

// MoveTo 绝对位置运动
// 参数: id 控制器ID, axis 轴名称, position 目标位置
// 返回: error 错误信息
func (m *MotionManager) MoveTo(id string, axis motion.AxisName, position float64) error {
	m.mu.RLock()
	inst, ok := m.instances[id]
	m.mu.RUnlock()
	if !ok {
		return ErrMotionNotFound
	}
	return inst.Driver.MoveTo(axis, position)
}

// MoveBy 相对位置运动(增量)
// 参数: id 控制器ID, axis 轴名称, delta 增量距离
// 返回: error 错误信息
func (m *MotionManager) MoveBy(id string, axis motion.AxisName, delta float64) error {
	m.mu.RLock()
	inst, ok := m.instances[id]
	m.mu.RUnlock()
	if !ok {
		return ErrMotionNotFound
	}
	return inst.Driver.MoveBy(axis, delta)
}

// Jog 寸动(连续运动)
// 参数: id 控制器ID, axis 轴名称, direction 方向(+/-), speed 速度
// 返回: error 错误信息
func (m *MotionManager) Jog(id string, axis motion.AxisName, direction string, speed float64) error {
	m.mu.RLock()
	inst, ok := m.instances[id]
	m.mu.RUnlock()
	if !ok {
		return ErrMotionNotFound
	}
	return inst.Driver.Jog(axis, direction, speed)
}

// Home 回零(寻找原点)
// 参数: id 控制器ID, axis 轴名称
// 返回: error 错误信息
func (m *MotionManager) Home(id string, axis motion.AxisName) error {
	m.mu.RLock()
	inst, ok := m.instances[id]
	m.mu.RUnlock()
	if !ok {
		return ErrMotionNotFound
	}
	return inst.Driver.Home(axis)
}

// Stop 停止指定轴
// 参数: id 控制器ID, axis 轴名称
// 返回: error 错误信息
func (m *MotionManager) Stop(id string, axis motion.AxisName) error {
	m.mu.RLock()
	inst, ok := m.instances[id]
	m.mu.RUnlock()
	if !ok {
		return ErrMotionNotFound
	}
	return inst.Driver.Stop(axis)
}

// EmergencyStop 急停(所有轴)
// 参数: id 控制器ID
// 返回: error 错误信息
func (m *MotionManager) EmergencyStop(id string) error {
	m.mu.RLock()
	inst, ok := m.instances[id]
	m.mu.RUnlock()
	if !ok {
		return ErrMotionNotFound
	}
	return inst.Driver.EmergencyStop()
}

// DefinePosition 定义当前位置
// 用于重新初始化或校准
// 参数: id 控制器ID, axis 轴名称, position 位置值
// 返回: error 错误信息
func (m *MotionManager) DefinePosition(id string, axis motion.AxisName, position float64) error {
	m.mu.RLock()
	inst, ok := m.instances[id]
	m.mu.RUnlock()
	if !ok {
		return ErrMotionNotFound
	}
	return inst.Driver.DefinePosition(axis, position)
}

// GetStatusAll 获取所有控制器状态
// 返回: []motion.MotionControllerStatus 状态列表
func (m *MotionManager) GetStatusAll() []motion.MotionControllerStatus {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make([]motion.MotionControllerStatus, 0, len(m.instances))
	for _, inst := range m.instances {
		result = append(result, inst.Status)
	}
	return result
}

// GetStatus 获取单个控制器状态
// 参数: id 控制器ID
// 返回: (状态, 错误信息)
func (m *MotionManager) GetStatus(id string) (motion.MotionControllerStatus, error) {
	m.mu.RLock()
	inst, ok := m.instances[id]
	m.mu.RUnlock()
	if !ok {
		return motion.MotionControllerStatus{}, ErrMotionNotFound
	}
	return inst.Status, nil
}

// StartStatusPolling 启动状态轮询(10Hz)
// 参数: ctx 上下文(用于取消)
func (m *MotionManager) StartStatusPolling(ctx context.Context) {
	pollCtx, cancel := context.WithCancel(ctx)
	m.pollCancel = cancel

	ticker := time.NewTicker(motionPollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-pollCtx.Done():
			return
		case <-ticker.C:
			m.refreshAllStatus()
		}
	}
}

// StopStatusPolling 停止状态轮询
func (m *MotionManager) StopStatusPolling() {
	if m.pollCancel != nil {
		m.pollCancel()
	}
}

// Shutdown 关闭所有控制器
// 参数: ctx 上下文(用于超时控制)
func (m *MotionManager) Shutdown(ctx context.Context) {
	m.StopStatusPolling()
	m.mu.Lock()
	defer m.mu.Unlock()
	for id, inst := range m.instances {
		if inst.Driver.IsConnected() {
			if err := inst.Driver.Disconnect(); err != nil {
				slog.Warn("Failed to disconnect motion controller", "id", id, "err", err)
			}
		}
	}
}

// refreshAllStatus 刷新所有已连接控制器的状态
func (m *MotionManager) refreshAllStatus() {
	m.mu.RLock()
	snapshot := make(map[string]*MotionInstance, len(m.instances))
	for id, inst := range m.instances {
		if inst.Driver.IsConnected() {
			snapshot[id] = inst
		}
	}
	m.mu.RUnlock()

	// 逐个刷新状态
	for id, inst := range snapshot {
		axes, err := inst.Driver.GetAllAxisStatus()
		if err != nil {
			slog.Debug("Motion status poll error", "id", id, "err", err)
			m.mu.Lock()
			inst.Status.LastError = err.Error()
			m.mu.Unlock()
			continue
		}

		lastError := buildAxisStatusError(axes)

		m.mu.Lock()
		inst.Status.Axes = axes
		inst.Status.LastError = lastError
		m.mu.Unlock()
	}

	// 推送到前端
	m.mu.RLock()
	allStatus := make([]motion.MotionControllerStatus, 0, len(m.instances))
	for _, inst := range m.instances {
		allStatus = append(allStatus, inst.Status)
	}
	m.mu.RUnlock()

	if m.publisher != nil && len(allStatus) > 0 {
		m.publisher.Broadcast(ports.ChannelMotionStatus, allStatus)
	}
}

// buildAxisStatusError 构建轴状态错误信息
// 从多个轴的状态中汇总错误信息
func buildAxisStatusError(axes []motion.AxisStatus) string {
	messages := make([]string, 0)
	for _, axis := range axes {
		// 检查限位开关状态
		if axis.PosLimit && axis.NegLimit {
			messages = append(messages, fmt.Sprintf("%s 正负限位触发", axis.Name))
		} else if axis.PosLimit {
			messages = append(messages, fmt.Sprintf("%s 正限位触发", axis.Name))
		} else if axis.NegLimit {
			messages = append(messages, fmt.Sprintf("%s 负限位触发", axis.Name))
		}

		// 检查补偿错误
		if axis.CompensationError != "" {
			messages = append(messages, fmt.Sprintf("%s 补偿失败: %s", axis.Name, axis.CompensationError))
		}
	}

	if len(messages) == 0 {
		return ""
	}

	return strings.Join(messages, "; ")
}

// ErrMotionNotFound 运动控制器未找到错误
var ErrMotionNotFound = fmt.Errorf("motion controller not found")
