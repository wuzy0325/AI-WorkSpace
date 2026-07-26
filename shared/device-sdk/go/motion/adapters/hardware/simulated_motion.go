package hardware

import (
	"context"
	"fmt"
	"math"
	"math/rand"
	"sync"
	"time"

	"shared.local/device-sdk/go/motion/core"
)

// SimulatedMotionController 模拟运动控制器
// 模拟真实位移机构的行为，包括连续位置更新、速度限制、限位保护等
type SimulatedMotionController struct {
	mu      sync.RWMutex
	profile core.MotionControllerProfile
	status  core.ControllerStatus
	stopCh  chan struct{}
	// cancelChs 每个轴独立的取消通道，用于精确停止单轴运动
	cancelChs map[core.AxisName]chan struct{}
}

// simSpeedFactor 模拟运动加速因子，使模拟运动以10倍速完成
const simSpeedFactor = 0.1

// NewSimulatedMotionController 创建模拟运动控制器
func NewSimulatedMotionController(profile core.MotionControllerProfile) *SimulatedMotionController {
	status := core.ControllerStatus{
		ID:               profile.ID,
		Name:             profile.Name,
		Type:             profile.Type,
		Connected:        false,
		EmergencyStopped: false,
		Axes:             make([]core.AxisStatus, 0),
	}

	cancelChs := make(map[core.AxisName]chan struct{})
	for _, axisCfg := range profile.Axes {
		if axisCfg.Enabled {
			status.Axes = append(status.Axes, core.AxisStatus{
				Name:          axisCfg.Name,
				Position:      0,
				Velocity:      0,
				Moving:        false,
				Homed:         false,
				PosLimit:      false,
				NegLimit:      false,
				Compensating:  false,
				PositionError: 0,
			})
			cancelChs[axisCfg.Name] = make(chan struct{})
		}
	}

	return &SimulatedMotionController{
		profile:   profile,
		status:    status,
		stopCh:    make(chan struct{}),
		cancelChs: cancelChs,
	}
}

// ApplyConfig 应用新的控制器配置（模拟控制器无需硬件操作，仅更新 profile）
func (c *SimulatedMotionController) ApplyConfig(ctx context.Context, profile core.MotionControllerProfile) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.profile = profile
	return nil
}

// GetProfile 获取控制器配置
func (c *SimulatedMotionController) GetProfile() core.MotionControllerProfile {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.profile
}

// Connect connect controller
func (c *SimulatedMotionController) Connect(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.status.Connected = true
	c.status.EmergencyStopped = false
	c.status.LastError = ""
	return nil
}

// Disconnect disconnect controller
func (c *SimulatedMotionController) Disconnect(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// 通知所有运动协程停止
	select {
	case <-c.stopCh:
	default:
		close(c.stopCh)
	}
	c.stopCh = make(chan struct{})

	// 重建每轴取消通道
	for name, ch := range c.cancelChs {
		select {
		case <-ch:
		default:
			close(ch)
		}
		c.cancelChs[name] = make(chan struct{})
	}

	c.status.Connected = false
	for i := range c.status.Axes {
		c.status.Axes[i].Moving = false
		c.status.Axes[i].Velocity = 0
	}
	return nil
}

// Status get status
func (c *SimulatedMotionController) Status(ctx context.Context) (core.ControllerStatus, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	status := c.status
	status.Axes = append([]core.AxisStatus(nil), c.status.Axes...)
	return status, nil
}

// MoveTo move to absolute position
func (c *SimulatedMotionController) MoveTo(ctx context.Context, axis core.AxisName, position float64) error {
	c.mu.Lock()

	if err := c.checkReadyLocked(); err != nil {
		c.mu.Unlock()
		return err
	}

	axisIndex, err := c.axisIndexLocked(axis)
	if err != nil {
		c.mu.Unlock()
		return err
	}

	// 检查限位
	if err := c.checkLimitsLocked(axisIndex, position); err != nil {
		c.status.LastError = err.Error()
		c.mu.Unlock()
		return err
	}

	// 取消该轴正在进行的运动
	c.cancelAxisLocked(axis)

	// 获取轴配置中的最大速度
	maxSpeed := c.getMaxSpeedLocked(axisIndex)
	currentPos := c.status.Axes[axisIndex].Position
	distance := math.Abs(position - currentPos)

	// 如果已经在目标位置，直接返回
	if distance < 0.001 {
		c.status.Axes[axisIndex].Moving = false
		c.status.Axes[axisIndex].Velocity = 0
		c.mu.Unlock()
		return nil
	}

	// 计算运动方向
	direction := 1.0
	if position < currentPos {
		direction = -1.0
	}
	c.status.Axes[axisIndex].Velocity = maxSpeed
	c.status.Axes[axisIndex].Moving = true

	// 计算运动持续时间（使用加速因子，使模拟运动更快完成）
	duration := time.Duration(float64(time.Second) * (distance / maxSpeed) * simSpeedFactor)
	if duration < 50*time.Millisecond {
		duration = 50 * time.Millisecond
	}

	// 获取当前stopCh和cancelCh的引用
	stopCh := c.stopCh
	cancelCh := c.cancelChs[axis]

	c.mu.Unlock()

	// 在后台模拟连续运动
	go c.simulateMovement(axisIndex, axis, position, direction, maxSpeed, duration, stopCh, cancelCh)

	return nil
}

// MoveBy relative move
func (c *SimulatedMotionController) MoveBy(ctx context.Context, axis core.AxisName, delta float64) error {
	c.mu.Lock()

	if err := c.checkReadyLocked(); err != nil {
		c.mu.Unlock()
		return err
	}

	axisIndex, err := c.axisIndexLocked(axis)
	if err != nil {
		c.mu.Unlock()
		return err
	}

	newPos := c.status.Axes[axisIndex].Position + delta

	if err := c.checkLimitsLocked(axisIndex, newPos); err != nil {
		c.status.LastError = err.Error()
		c.mu.Unlock()
		return err
	}

	// 取消该轴正在进行的运动
	c.cancelAxisLocked(axis)

	maxSpeed := c.getMaxSpeedLocked(axisIndex)
	distance := math.Abs(delta)

	if distance < 0.001 {
		c.status.Axes[axisIndex].Moving = false
		c.status.Axes[axisIndex].Velocity = 0
		c.mu.Unlock()
		return nil
	}

	direction := 1.0
	if delta < 0 {
		direction = -1.0
	}
	c.status.Axes[axisIndex].Velocity = maxSpeed
	c.status.Axes[axisIndex].Moving = true

	duration := time.Duration(float64(time.Second) * (distance / maxSpeed) * simSpeedFactor)
	if duration < 50*time.Millisecond {
		duration = 50 * time.Millisecond
	}

	stopCh := c.stopCh
	cancelCh := c.cancelChs[axis]

	c.mu.Unlock()

	go c.simulateMovement(axisIndex, axis, newPos, direction, maxSpeed, duration, stopCh, cancelCh)

	return nil
}

// Jog jog movement
func (c *SimulatedMotionController) Jog(ctx context.Context, axis core.AxisName, velocity float64) error {
	c.mu.Lock()

	if err := c.checkReadyLocked(); err != nil {
		c.mu.Unlock()
		return err
	}

	axisIndex, err := c.axisIndexLocked(axis)
	if err != nil {
		c.mu.Unlock()
		return err
	}

	// 取消该轴正在进行的运动
	c.cancelAxisLocked(axis)

	// 限制Jog速度不超过最大速度
	maxSpeed := c.getMaxSpeedLocked(axisIndex)
	jogSpeed := math.Abs(velocity)
	if jogSpeed > maxSpeed {
		jogSpeed = maxSpeed
	}

	c.status.Axes[axisIndex].Moving = true
	c.status.Axes[axisIndex].Velocity = jogSpeed

	stopCh := c.stopCh
	cancelCh := c.cancelChs[axis]

	c.mu.Unlock()

	// 在后台持续更新位置
	go c.simulateJog(axisIndex, axis, velocity, maxSpeed, stopCh, cancelCh)

	return nil
}

// Home home axis
func (c *SimulatedMotionController) Home(ctx context.Context, axis core.AxisName) error {
	c.mu.Lock()

	if err := c.checkReadyLocked(); err != nil {
		c.mu.Unlock()
		return err
	}

	axisIndex, err := c.axisIndexLocked(axis)
	if err != nil {
		c.mu.Unlock()
		return err
	}

	// 取消该轴正在进行的运动
	c.cancelAxisLocked(axis)

	// 模拟归位过程
	c.status.Axes[axisIndex].Moving = true
	c.status.Axes[axisIndex].Velocity = 0.5
	c.status.Axes[axisIndex].Homed = false

	stopCh := c.stopCh
	cancelCh := c.cancelChs[axis]

	c.mu.Unlock()

	// 在后台模拟归位
	go c.simulateHome(axisIndex, axis, stopCh, cancelCh)

	return nil
}

// Stop stop axis movement
func (c *SimulatedMotionController) Stop(ctx context.Context, axis core.AxisName) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.status.Connected {
		return fmt.Errorf("controller not connected")
	}
	if axis == "" {
		for _, ch := range c.cancelChs {
			select {
			case <-ch:
			default:
				close(ch)
			}
		}
		for name := range c.cancelChs {
			c.cancelChs[name] = make(chan struct{})
		}
		for i := range c.status.Axes {
			c.status.Axes[i].Moving = false
			c.status.Axes[i].Velocity = 0
			c.status.Axes[i].Compensating = false
		}
		return nil
	}

	axisIndex, err := c.axisIndexLocked(axis)
	if err != nil {
		return err
	}

	// 取消该轴的运动协程
	c.cancelAxisLocked(axis)

	c.status.Axes[axisIndex].Moving = false
	c.status.Axes[axisIndex].Velocity = 0
	c.status.Axes[axisIndex].Compensating = false

	return nil
}

// EmergencyStop emergency stop
func (c *SimulatedMotionController) EmergencyStop(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// 通知所有运动协程停止
	select {
	case <-c.stopCh:
	default:
		close(c.stopCh)
	}
	c.stopCh = make(chan struct{})

	// 重建每轴取消通道
	for name, ch := range c.cancelChs {
		select {
		case <-ch:
		default:
			close(ch)
		}
		c.cancelChs[name] = make(chan struct{})
	}

	c.status.EmergencyStopped = true
	for i := range c.status.Axes {
		c.status.Axes[i].Moving = false
		c.status.Axes[i].Velocity = 0
		c.status.Axes[i].Compensating = false
	}

	return nil
}

// ResetEmergencyStop reset emergency stop state
func (c *SimulatedMotionController) ResetEmergencyStop(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.status.Connected {
		return fmt.Errorf("controller not connected")
	}

	c.status.EmergencyStopped = false
	return nil
}

// DefinePosition define current position as specified value
func (c *SimulatedMotionController) DefinePosition(ctx context.Context, axis core.AxisName, position float64) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.status.Connected {
		return fmt.Errorf("controller not connected")
	}

	axisIndex, err := c.axisIndexLocked(axis)
	if err != nil {
		return err
	}

	c.status.Axes[axisIndex].Position = position
	return nil
}

// cancelAxisLocked 取消指定轴的运动协程（需要持有锁）
func (c *SimulatedMotionController) cancelAxisLocked(axis core.AxisName) {
	if ch, ok := c.cancelChs[axis]; ok {
		select {
		case <-ch:
		default:
			close(ch)
		}
		c.cancelChs[axis] = make(chan struct{})
	}
}

// ownsAxisCommandLocked 判断异步运动协程是否仍拥有当前轴命令。
// 仅依赖关闭取消通道无法阻止已经进入 ticker 分支、正在等待互斥锁的旧协程；
// 通过比较通道身份，确保被新命令替换的旧协程不能再写位置或清除 Moving 状态。
func (c *SimulatedMotionController) ownsAxisCommandLocked(axis core.AxisName, cancelCh chan struct{}) bool {
	current, ok := c.cancelChs[axis]
	return ok && current == cancelCh
}

// checkReadyLocked 检查控制器是否就绪（需要持有锁）
func (c *SimulatedMotionController) checkReadyLocked() error {
	if !c.status.Connected {
		return fmt.Errorf("controller not connected")
	}
	if c.status.EmergencyStopped {
		return fmt.Errorf("controller is in emergency stop state")
	}
	return nil
}

// axisProfileLocked 根据轴名称查找 profile 中的轴配置（需要持有锁）
func (c *SimulatedMotionController) axisProfileLocked(axis core.AxisName) (core.AxisConfig, bool) {
	for _, a := range c.profile.Axes {
		if a.Name == axis {
			return a, true
		}
	}
	return core.AxisConfig{}, false
}

// getMaxSpeedLocked 获取轴的最大速度（需要持有锁）
func (c *SimulatedMotionController) getMaxSpeedLocked(axisIndex int) float64 {
	axisName := c.axisNameByIndexLocked(axisIndex)
	if axisCfg, ok := c.axisProfileLocked(axisName); ok {
		if axisCfg.MaxSpeed != nil {
			return *axisCfg.MaxSpeed
		}
	}
	return 10.0
}

// axisNameByIndexLocked 根据 status.Axes 索引获取轴名称（需要持有锁）
func (c *SimulatedMotionController) axisNameByIndexLocked(axisIndex int) core.AxisName {
	if axisIndex >= 0 && axisIndex < len(c.status.Axes) {
		return c.status.Axes[axisIndex].Name
	}
	return ""
}

// simulateMovement 模拟运动到目标位置
// 使用ticker定期更新位置，模拟真实的连续运动过程
func (c *SimulatedMotionController) simulateMovement(axisIndex int, axis core.AxisName, targetPosition, direction, speed float64, duration time.Duration, stopCh, cancelCh chan struct{}) {
	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()

	startTime := time.Now()
	startPos := func() float64 {
		c.mu.RLock()
		defer c.mu.RUnlock()
		return c.status.Axes[axisIndex].Position
	}()
	totalDistance := math.Abs(targetPosition - startPos)

	for {
		select {
		case <-stopCh:
			return
		case <-cancelCh:
			return
		case <-ticker.C:
			elapsed := time.Since(startTime)
			if elapsed >= duration {
				// 运动完成
				c.mu.Lock()
				if !c.ownsAxisCommandLocked(axis, cancelCh) {
					c.mu.Unlock()
					return
				}
				if axisIndex < len(c.status.Axes) {
					c.status.Axes[axisIndex].Position = targetPosition
					c.status.Axes[axisIndex].Moving = false
					c.status.Axes[axisIndex].Velocity = 0
					c.status.Axes[axisIndex].PositionError = 0
				}
				c.mu.Unlock()
				return
			}

			c.mu.Lock()
			if !c.ownsAxisCommandLocked(axis, cancelCh) || axisIndex >= len(c.status.Axes) || !c.status.Axes[axisIndex].Moving {
				c.mu.Unlock()
				return
			}

			// 基于时间进度计算位置（线性插值），确保精确到达目标
			progress := float64(elapsed) / float64(duration)
			newPos := startPos + direction*totalDistance*progress

			// 检查是否到达或超过目标位置
			if (direction > 0 && newPos >= targetPosition) || (direction < 0 && newPos <= targetPosition) {
				newPos = targetPosition
			}

			// 检查限位
			if err := c.checkLimitsLocked(axisIndex, newPos); err != nil {
				c.status.Axes[axisIndex].Moving = false
				c.status.Axes[axisIndex].Velocity = 0
				c.status.LastError = err.Error()
				c.mu.Unlock()
				return
			}

			// 模拟跟随误差（真实电机存在滞后）
			followError := (rand.Float64()-0.5)*0.02*speed + (rand.Float64()-0.5)*0.005

			c.status.Axes[axisIndex].Position = newPos
			c.status.Axes[axisIndex].PositionError = math.Abs(followError)
			c.mu.Unlock()
		}
	}
}

// simulateJog 模拟持续点动
func (c *SimulatedMotionController) simulateJog(axisIndex int, axis core.AxisName, velocity float64, maxSpeed float64, stopCh, cancelCh chan struct{}) {
	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()

	jogTimer := time.NewTimer(2 * time.Second)
	defer jogTimer.Stop()

	for {
		select {
		case <-stopCh:
			return
		case <-cancelCh:
			return
		case <-jogTimer.C:
			c.mu.Lock()
			if !c.ownsAxisCommandLocked(axis, cancelCh) {
				c.mu.Unlock()
				return
			}
			if axisIndex < len(c.status.Axes) {
				c.status.Axes[axisIndex].Moving = false
				c.status.Axes[axisIndex].Velocity = 0
			}
			c.mu.Unlock()
			return
		case <-ticker.C:
			c.mu.Lock()
			if !c.ownsAxisCommandLocked(axis, cancelCh) || axisIndex >= len(c.status.Axes) || !c.status.Axes[axisIndex].Moving {
				c.mu.Unlock()
				return
			}

			step := velocity * 0.02
			newPos := c.status.Axes[axisIndex].Position + step

			// 检查限位
			if err := c.checkLimitsLocked(axisIndex, newPos); err != nil {
				c.status.Axes[axisIndex].Moving = false
				c.status.Axes[axisIndex].Velocity = 0
				c.status.LastError = err.Error()
				c.mu.Unlock()
				return
			}

			// 模拟跟随误差
			followError := (rand.Float64()-0.5)*0.02*maxSpeed + (rand.Float64()-0.5)*0.005

			c.status.Axes[axisIndex].Position = newPos
			c.status.Axes[axisIndex].PositionError = math.Abs(followError)
			c.mu.Unlock()
		}
	}
}

// simulateHome 模拟归位过程
func (c *SimulatedMotionController) simulateHome(axisIndex int, axis core.AxisName, stopCh, cancelCh chan struct{}) {
	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()

	homeDuration := 800 * time.Millisecond
	startTime := time.Now()

	// 获取当前位置，计算向零点移动的方向和速度
	c.mu.RLock()
	startPos := c.status.Axes[axisIndex].Position
	c.mu.RUnlock()

	if math.Abs(startPos) < 0.001 {
		// 已经在原点附近
		c.mu.Lock()
		if axisIndex < len(c.status.Axes) {
			c.status.Axes[axisIndex].Position = 0
			c.status.Axes[axisIndex].Velocity = 0
			c.status.Axes[axisIndex].Moving = false
			c.status.Axes[axisIndex].Homed = true
			c.status.Axes[axisIndex].PositionError = 0
		}
		c.mu.Unlock()
		return
	}

	totalDistance := math.Abs(startPos)
	direction := -1.0
	if startPos < 0 {
		direction = 1.0
	}

	for {
		select {
		case <-stopCh:
			return
		case <-cancelCh:
			return
		case <-ticker.C:
			elapsed := time.Since(startTime)
			if elapsed >= homeDuration {
				// 归位完成
				c.mu.Lock()
				if !c.ownsAxisCommandLocked(axis, cancelCh) {
					c.mu.Unlock()
					return
				}
				if axisIndex < len(c.status.Axes) {
					c.status.Axes[axisIndex].Position = 0
					c.status.Axes[axisIndex].Velocity = 0
					c.status.Axes[axisIndex].Moving = false
					c.status.Axes[axisIndex].Homed = true
					c.status.Axes[axisIndex].PositionError = 0
				}
				c.mu.Unlock()
				return
			}

			c.mu.Lock()
			if !c.ownsAxisCommandLocked(axis, cancelCh) || axisIndex >= len(c.status.Axes) || !c.status.Axes[axisIndex].Moving {
				c.mu.Unlock()
				return
			}

			// 基于时间进度计算位置（线性插值）
			progress := float64(elapsed) / float64(homeDuration)
			newPos := startPos + direction*totalDistance*progress

			// 检查是否到达原点
			if (direction > 0 && newPos >= 0) || (direction < 0 && newPos <= 0) {
				newPos = 0
			}

			c.status.Axes[axisIndex].Position = newPos
			c.mu.Unlock()
		}
	}
}

// checkLimitsLocked 检查限位（需要持有锁）
func (c *SimulatedMotionController) checkLimitsLocked(axisIndex int, position float64) error {
	if axisIndex < 0 || axisIndex >= len(c.status.Axes) {
		return fmt.Errorf("invalid axis index: %d", axisIndex)
	}

	axisName := c.status.Axes[axisIndex].Name
	axisCfg, ok := c.axisProfileLocked(axisName)
	if !ok {
		return fmt.Errorf("axis config not found: %s", axisName)
	}

	if axisCfg.MinLimit != nil && position < *axisCfg.MinLimit {
		c.status.Axes[axisIndex].NegLimit = true
		return fmt.Errorf("position %.2f below negative limit %.2f", position, *axisCfg.MinLimit)
	}

	if axisCfg.MaxLimit != nil && position > *axisCfg.MaxLimit {
		c.status.Axes[axisIndex].PosLimit = true
		return fmt.Errorf("position %.2f above positive limit %.2f", position, *axisCfg.MaxLimit)
	}

	// 清除限位状态（只有在未触发限位时才清除）
	c.status.Axes[axisIndex].NegLimit = false
	c.status.Axes[axisIndex].PosLimit = false

	return nil
}

// axisIndexLocked 根据轴名称获取索引（需要持有锁）
func (c *SimulatedMotionController) axisIndexLocked(axis core.AxisName) (int, error) {
	for i := range c.status.Axes {
		if c.status.Axes[i].Name == axis {
			return i, nil
		}
	}
	return 0, fmt.Errorf("unknown motion axis: %s", axis)
}
