package hardware

import (
	"fmt"
	"math"
	"sync"
	"time"

	"wind-daq/services/api-go/internal/core/motion"
)

// SimulatedMotionController 模拟运动控制器
type SimulatedMotionController struct {
	mu      sync.RWMutex
	profile motion.MotionControllerProfile
	status  motion.ControllerStatus
	stopCh  chan struct{}
}

// NewSimulatedMotionController 创建模拟运动控制器
func NewSimulatedMotionController(profile motion.MotionControllerProfile) *SimulatedMotionController {
	status := motion.ControllerStatus{
		ID:               profile.ID,
		Name:             profile.Name,
		Type:             profile.Type,
		Connected:        false,
		EmergencyStopped: false,
		Axes:             make([]motion.AxisStatus, 0),
	}

	for _, axisCfg := range profile.Axes {
		if axisCfg.Enabled {
			status.Axes = append(status.Axes, motion.AxisStatus{
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
		}
	}

	return &SimulatedMotionController{
		profile: profile,
		status:  status,
		stopCh:  make(chan struct{}),
	}
}

// GetProfile 获取控制器配置
func (c *SimulatedMotionController) GetProfile() motion.MotionControllerProfile {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.profile
}

// Connect 连接控制器
func (c *SimulatedMotionController) Connect() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.status.Connected = true
	c.status.EmergencyStopped = false
	c.status.LastError = ""
	return nil
}

// Disconnect 断开连接
func (c *SimulatedMotionController) Disconnect() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	close(c.stopCh)
	c.stopCh = make(chan struct{})
	c.status.Connected = false
	for i := range c.status.Axes {
		c.status.Axes[i].Moving = false
		c.status.Axes[i].Velocity = 0
	}
	return nil
}

// Status 获取状态
func (c *SimulatedMotionController) Status() motion.ControllerStatus {
	c.mu.RLock()
	defer c.mu.RUnlock()
	status := c.status
	status.Axes = append([]motion.AxisStatus(nil), c.status.Axes...)
	return status
}

// MoveTo 移动到绝对位置
func (c *SimulatedMotionController) MoveTo(axis motion.AxisName, position float64) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	axisIndex, err := c.axisIndexLocked(axis)
	if err != nil {
		return err
	}

	// 检查限位
	if err := c.checkLimitsLocked(axisIndex, position); err != nil {
		c.status.LastError = err.Error()
		return err
	}

	// 模拟运动过程
	c.status.Axes[axisIndex].Moving = true
	c.status.Axes[axisIndex].Velocity = 1.0

	// 在后台模拟运动
	go c.simulateMovement(axis, position, 500*time.Millisecond)

	return nil
}

// MoveBy 相对移动
func (c *SimulatedMotionController) MoveBy(axis motion.AxisName, delta float64) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	axisIndex, err := c.axisIndexLocked(axis)
	if err != nil {
		return err
	}

	newPos := c.status.Axes[axisIndex].Position + delta

	if err := c.checkLimitsLocked(axisIndex, newPos); err != nil {
		c.status.LastError = err.Error()
		return err
	}

	c.status.Axes[axisIndex].Moving = true
	c.status.Axes[axisIndex].Velocity = 1.0
	targetPos := c.status.Axes[axisIndex].Position + delta

	go c.simulateMovement(axis, targetPos, 300*time.Millisecond)

	return nil
}

// Jog 点动运动
func (c *SimulatedMotionController) Jog(axis motion.AxisName, velocity float64) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	axisIndex, err := c.axisIndexLocked(axis)
	if err != nil {
		return err
	}

	c.status.Axes[axisIndex].Moving = true
	c.status.Axes[axisIndex].Velocity = math.Abs(velocity)

	// 在后台持续更新位置
	go c.simulateJog(axis, velocity)

	return nil
}

// Home 归原点
func (c *SimulatedMotionController) Home(axis motion.AxisName) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	axisIndex, err := c.axisIndexLocked(axis)
	if err != nil {
		return err
	}

	// 模拟归位过程
	c.status.Axes[axisIndex].Moving = true
	c.status.Axes[axisIndex].Velocity = 0.5
	axisIndexCopy := axisIndex

	// 在后台模拟归位
	go func() {
		select {
		case <-c.stopCh:
			return
		case <-time.After(800 * time.Millisecond):
		}
		c.mu.Lock()
		if axisIndexCopy < len(c.status.Axes) {
			c.status.Axes[axisIndexCopy].Position = 0
			c.status.Axes[axisIndexCopy].Velocity = 0
			c.status.Axes[axisIndexCopy].Moving = false
			c.status.Axes[axisIndexCopy].Homed = true
		}
		c.mu.Unlock()
	}()

	return nil
}

// Stop 停止轴运动
func (c *SimulatedMotionController) Stop(axis motion.AxisName) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	axisIndex, err := c.axisIndexLocked(axis)
	if err != nil {
		return err
	}

	c.status.Axes[axisIndex].Moving = false
	c.status.Axes[axisIndex].Velocity = 0

	return nil
}

// EmergencyStop 紧急停止
func (c *SimulatedMotionController) EmergencyStop() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	close(c.stopCh)
	c.stopCh = make(chan struct{})
	c.status.EmergencyStopped = true
	for i := range c.status.Axes {
		c.status.Axes[i].Moving = false
		c.status.Axes[i].Velocity = 0
	}

	return nil
}

// DefinePosition 定义当前位置为指定值
func (c *SimulatedMotionController) DefinePosition(axis motion.AxisName, position float64) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	axisIndex, err := c.axisIndexLocked(axis)
	if err != nil {
		return err
	}

	c.status.Axes[axisIndex].Position = position
	return nil
}

// simulateMovement 模拟运动到目标位置
func (c *SimulatedMotionController) simulateMovement(axis motion.AxisName, targetPosition float64, duration time.Duration) {
	select {
	case <-c.stopCh:
		return
	case <-time.After(duration):
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	axisIndex, err := c.axisIndexLocked(axis)
	if err != nil {
		return
	}

	c.status.Axes[axisIndex].Position = targetPosition
	c.status.Axes[axisIndex].Moving = false
	c.status.Axes[axisIndex].Velocity = 0
}

// simulateJog 模拟持续点动
func (c *SimulatedMotionController) simulateJog(axis motion.AxisName, velocity float64) {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	jogTimer := time.NewTimer(2 * time.Second)
	defer jogTimer.Stop()

	for {
		select {
		case <-c.stopCh:
			return
		case <-jogTimer.C:
			c.mu.Lock()
			axisIndex, err := c.axisIndexLocked(axis)
			if err == nil && axisIndex < len(c.status.Axes) {
				c.status.Axes[axisIndex].Moving = false
				c.status.Axes[axisIndex].Velocity = 0
			}
			c.mu.Unlock()
			return
		case <-ticker.C:
			c.mu.Lock()
			axisIndex, err := c.axisIndexLocked(axis)
			if err != nil || axisIndex >= len(c.status.Axes) || !c.status.Axes[axisIndex].Moving {
				c.mu.Unlock()
				return
			}

			step := velocity * 0.1
			newPos := c.status.Axes[axisIndex].Position + step

			if c.status.Axes[axisIndex].PosLimit || c.status.Axes[axisIndex].NegLimit {
				c.status.Axes[axisIndex].Moving = false
				c.status.Axes[axisIndex].Velocity = 0
				c.mu.Unlock()
				return
			}

			c.status.Axes[axisIndex].Position = newPos
			c.mu.Unlock()
		}
	}
}

// checkLimitsLocked 检查限位（需要持有锁）
func (c *SimulatedMotionController) checkLimitsLocked(axisIndex int, position float64) error {
	if axisIndex < 0 || axisIndex >= len(c.profile.Axes) {
		return fmt.Errorf("invalid axis index: %d", axisIndex)
	}

	axisCfg := c.profile.Axes[axisIndex]

	if axisCfg.MinLimit != nil && position < *axisCfg.MinLimit {
		c.status.Axes[axisIndex].NegLimit = true
		return fmt.Errorf("position %.2f below negative limit %.2f", position, *axisCfg.MinLimit)
	}

	if axisCfg.MaxLimit != nil && position > *axisCfg.MaxLimit {
		c.status.Axes[axisIndex].PosLimit = true
		return fmt.Errorf("position %.2f above positive limit %.2f", position, *axisCfg.MaxLimit)
	}

	// 清除限位状态
	c.status.Axes[axisIndex].NegLimit = false
	c.status.Axes[axisIndex].PosLimit = false

	return nil
}

// axisIndexLocked 根据轴名称获取索引（需要持有锁）
func (c *SimulatedMotionController) axisIndexLocked(axis motion.AxisName) (int, error) {
	for i := range c.status.Axes {
		if c.status.Axes[i].Name == axis {
			return i, nil
		}
	}
	return 0, fmt.Errorf("unknown motion axis: %s", axis)
}
