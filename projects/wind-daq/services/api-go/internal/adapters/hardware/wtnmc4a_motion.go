package hardware

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"shared.local/device-sdk/go/ffi"
	"shared.local/device-sdk/go/motion/core"
	"shared.local/device-sdk/go/motion/ports"
)

// WTNMC4AMotionController 基于 WTNMC4A DLL FFI 的运动控制器适配器
// 通过独立 goroutine 持有网络连接 + FFI 调用，channel 序列化命令
// 每个控制器实例的 I/O 操作在独立 goroutine 中执行，避免阻塞其他设备
type WTNMC4AMotionController struct {
	mu        sync.RWMutex
	profile   core.MotionControllerProfile
	status    core.ControllerStatus
	handle    uintptr
	cmdCh     chan wtnmc4aCmd
	stopCh    chan struct{}
	connected bool
	dllLoaded bool
}

// wtnmc4aCmd FFI 命令封装，通过 channel 传递到独立 goroutine 执行
type wtnmc4aCmd struct {
	cmd      string
	axis     int
	value    int64
	floatVal float64
	respCh   chan wtnmc4aResp
}

// wtnmc4aResp FFI 命令响应
type wtnmc4aResp struct {
	value int64
	err   error
}

// axisNameToIndex 将轴名称映射为 WTNMC4A 的轴索引（0-based）
func axisNameToIndex(axis core.AxisName) (int, error) {
	switch axis {
	case core.AxisX:
		return 0, nil
	case core.AxisY:
		return 1, nil
	case core.AxisZ:
		return 2, nil
	case core.AxisU:
		return 3, nil
	default:
		return 0, fmt.Errorf("unknown axis: %s", axis)
	}
}

// NewWTNMC4AMotionController 创建 WTNMC4A 运动控制器适配器
func NewWTNMC4AMotionController(profile core.MotionControllerProfile) *WTNMC4AMotionController {
	status := core.ControllerStatus{
		ID:               profile.ID,
		Name:             profile.Name,
		Type:             profile.Type,
		Connected:        false,
		EmergencyStopped: false,
		Axes:             make([]core.AxisStatus, 0),
	}

	for _, axisCfg := range profile.Axes {
		if axisCfg.Enabled {
			status.Axes = append(status.Axes, core.AxisStatus{
				Name:     axisCfg.Name,
				Position: 0,
				Velocity: 0,
				Moving:   false,
				Homed:    false,
			})
		}
	}

	return &WTNMC4AMotionController{
		profile: profile,
		status:  status,
		cmdCh:   make(chan wtnmc4aCmd, 32),
		stopCh:  make(chan struct{}),
	}
}

// ApplyConfig 应用新的控制器配置（wind-daq 适配器无运行时配置下发，仅更新 profile）
func (c *WTNMC4AMotionController) ApplyConfig(ctx context.Context, profile core.MotionControllerProfile) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.profile = profile
	return nil
}

// GetProfile 获取控制器配置
func (c *WTNMC4AMotionController) GetProfile() core.MotionControllerProfile {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.profile
}

// Connect 连接控制器
// 初始化 DLL 并创建设备连接，启动命令处理 goroutine
func (c *WTNMC4AMotionController) Connect(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.connected {
		return nil
	}

	// 初始化 DLL（仅首次调用）
	if !c.dllLoaded {
		if err := ffi.Init("WTNMC4A.dll"); err != nil {
			return fmt.Errorf("WTNMC4A DLL 加载失败: %w", err)
		}
		c.dllLoaded = true
	}

	// 创建设备连接
	handle, err := ffi.WTNMC4ADEVCreate(c.profile.Address, 3000, 3000)
	if err != nil {
		return fmt.Errorf("WTNMC4A 设备连接失败 (%s): %w", c.profile.Address, err)
	}
	c.handle = handle

	// 启动命令处理 goroutine
	go c.commandLoop()

	c.connected = true
	c.status.Connected = true
	c.status.EmergencyStopped = false
	c.status.LastError = ""

	slog.Info("WTNMC4A 控制器已连接", "id", c.profile.ID, "address", c.profile.Address)
	return nil
}

// Disconnect 断开控制器连接
func (c *WTNMC4AMotionController) Disconnect(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.connected {
		return nil
	}

	// 通知命令处理 goroutine 停止
	select {
	case <-c.stopCh:
	default:
		close(c.stopCh)
	}

	// 排空 cmdCh 中残余命令，回应 disconnect 错误，避免 sendCommand 挂起
	for {
		select {
		case cmd := <-c.cmdCh:
			cmd.respCh <- wtnmc4aResp{err: fmt.Errorf("controller disconnected")}
		default:
			goto drained
		}
	}
drained:
	c.stopCh = make(chan struct{})

	// 释放设备连接
	if c.handle != 0 {
		ffi.WTNMC4ADEVRelease(c.handle)
		c.handle = 0
	}

	c.connected = false
	c.status.Connected = false
	for i := range c.status.Axes {
		c.status.Axes[i].Moving = false
		c.status.Axes[i].Velocity = 0
	}

	slog.Info("WTNMC4A 控制器已断开", "id", c.profile.ID)
	return nil
}

// Status 获取控制器状态
// FFI 调用（网络 I/O）在锁外执行，避免阻塞急停等写操作
func (c *WTNMC4AMotionController) Status(ctx context.Context) (core.ControllerStatus, error) {
	c.mu.RLock()
	status := c.status
	status.Axes = append([]core.AxisStatus(nil), c.status.Axes...)
	connected := c.connected
	handle := c.handle
	c.mu.RUnlock()

	if connected && handle != 0 {
		for i := range status.Axes {
			axisIdx, err := axisNameToIndex(status.Axes[i].Name)
			if err != nil {
				continue
			}
			lp, err := ffi.WTNMC4AReadLP(handle, axisIdx)
			if err == nil {
				status.Axes[i].Position = float64(lp)
			}
			rr1 := ffi.WTNMC4AGetRR1Status(handle, axisIdx)
			status.Axes[i].Moving = rr1.ASND || rr1.DSND || rr1.CNST
			status.Axes[i].PosLimit = rr1.LMTP
			status.Axes[i].NegLimit = rr1.LMTM
			if rr1.EMG {
				status.EmergencyStopped = true
			}
		}
	}

	c.mu.Lock()
	if c.connected && c.handle == handle {
		c.status = status
	}
	c.mu.Unlock()

	return status, nil
}

// MoveTo 移动到绝对位置
func (c *WTNMC4AMotionController) MoveTo(ctx context.Context, axis core.AxisName, position float64) error {
	axisIdx, err := axisNameToIndex(axis)
	if err != nil {
		return err
	}
	return c.sendCommand(ctx, "moveTo", axisIdx, int64(position), 0)
}

// MoveBy 相对移动
func (c *WTNMC4AMotionController) MoveBy(ctx context.Context, axis core.AxisName, delta float64) error {
	axisIdx, err := axisNameToIndex(axis)
	if err != nil {
		return err
	}

	c.mu.RLock()
	connected := c.connected
	handle := c.handle
	c.mu.RUnlock()
	if !connected || handle == 0 {
		return fmt.Errorf("controller not connected")
	}
	currentPos, err := ffi.WTNMC4AReadLP(handle, axisIdx)
	if err != nil {
		return fmt.Errorf("ReadLP failed: %w", err)
	}

	targetPos := float64(currentPos) + delta
	return c.sendCommand(ctx, "moveTo", axisIdx, int64(targetPos), 0)
}

// Jog 点动运动
func (c *WTNMC4AMotionController) Jog(ctx context.Context, axis core.AxisName, velocity float64) error {
	axisIdx, err := axisNameToIndex(axis)
	if err != nil {
		return err
	}
	return c.sendCommand(ctx, "jog", axisIdx, 0, velocity)
}

// Home 归零操作
func (c *WTNMC4AMotionController) Home(ctx context.Context, axis core.AxisName) error {
	axisIdx, err := axisNameToIndex(axis)
	if err != nil {
		return err
	}
	return c.sendCommand(ctx, "home", axisIdx, 0, 0)
}

// Stop 停止轴运动（减速停止）
func (c *WTNMC4AMotionController) Stop(ctx context.Context, axis core.AxisName) error {
	if axis == "" {
		var firstErr error
		c.mu.RLock()
		axes := append([]core.AxisStatus(nil), c.status.Axes...)
		c.mu.RUnlock()
		for _, axisStatus := range axes {
			axisIdx, err := axisNameToIndex(axisStatus.Name)
			if err != nil {
				if firstErr == nil {
					firstErr = err
				}
				continue
			}
			if err := c.sendCommand(ctx, "decStop", axisIdx, 0, 0); err != nil && firstErr == nil {
				firstErr = err
			}
		}
		return firstErr
	}
	axisIdx, err := axisNameToIndex(axis)
	if err != nil {
		return err
	}
	return c.sendCommand(ctx, "decStop", axisIdx, 0, 0)
}

// EmergencyStop 紧急停止（瞬时停止所有轴）
// 聚合所有轴的停止错误，确保部分失败时调用方能感知
func (c *WTNMC4AMotionController) EmergencyStop(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.connected || c.handle == 0 {
		return fmt.Errorf("controller not connected")
	}

	var errs []error
	for i := range c.status.Axes {
		axisIdx, _ := axisNameToIndex(c.status.Axes[i].Name)
		if err := ffi.WTNMC4AInstStop(c.handle, axisIdx); err != nil {
			slog.Warn("WTNMC4A 瞬停轴失败", "axis", c.status.Axes[i].Name, "error", err)
			errs = append(errs, fmt.Errorf("axis %s: %w", c.status.Axes[i].Name, err))
		}
		c.status.Axes[i].Moving = false
		c.status.Axes[i].Velocity = 0
	}

	c.status.EmergencyStopped = true

	if len(errs) > 0 {
		return fmt.Errorf("emergency stop partially failed: %v", errs)
	}
	return nil
}

// ResetEmergencyStop 重置急停状态
func (c *WTNMC4AMotionController) ResetEmergencyStop(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.connected {
		return fmt.Errorf("controller not connected")
	}

	c.status.EmergencyStopped = false
	return nil
}

// DefinePosition 定义当前位置
func (c *WTNMC4AMotionController) DefinePosition(ctx context.Context, axis core.AxisName, position float64) error {
	axisIdx, err := axisNameToIndex(axis)
	if err != nil {
		return err
	}
	return c.sendCommand(ctx, "setLP", axisIdx, int64(position), 0)
}

// sendCommand 通过 channel 发送命令到命令处理 goroutine
// 确保所有 FFI 调用在同一个 goroutine 中序列化执行
func (c *WTNMC4AMotionController) sendCommand(ctx context.Context, cmd string, axis int, value int64, floatVal float64) error {
	c.mu.RLock()
	connected := c.connected
	c.mu.RUnlock()

	if !connected {
		return fmt.Errorf("controller not connected")
	}

	cmdMsg := wtnmc4aCmd{
		cmd:      cmd,
		axis:     axis,
		value:    value,
		floatVal: floatVal,
		respCh:   make(chan wtnmc4aResp, 1),
	}

	select {
	case c.cmdCh <- cmdMsg:
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(5 * time.Second):
		return fmt.Errorf("command timeout: %s", cmd)
	}

	select {
	case resp := <-cmdMsg.respCh:
		return resp.err
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(10 * time.Second):
		return fmt.Errorf("response timeout: %s", cmd)
	}
}

// commandLoop 命令处理 goroutine
// 所有 FFI 调用在此 goroutine 中序列化执行，避免并发 FFI 调用冲突
func (c *WTNMC4AMotionController) commandLoop() {
	for {
		select {
		case <-c.stopCh:
			return
		case cmd := <-c.cmdCh:
			resp := c.executeCommand(cmd)
			select {
			case cmd.respCh <- resp:
			default:
			}
		}
	}
}

// executeCommand 在命令处理 goroutine 中执行 FFI 命令
func (c *WTNMC4AMotionController) executeCommand(cmd wtnmc4aCmd) wtnmc4aResp {
	c.mu.RLock()
	handle := c.handle
	c.mu.RUnlock()

	if handle == 0 {
		return wtnmc4aResp{err: fmt.Errorf("device handle is nil")}
	}

	switch cmd.cmd {
	case "moveTo":
		if err := ffi.WTNMC4ASetP(handle, cmd.axis, cmd.value); err != nil {
			return wtnmc4aResp{err: fmt.Errorf("SetP failed: %w", err)}
		}
		if err := ffi.WTNMC4AStartLVDV(handle, cmd.axis); err != nil {
			return wtnmc4aResp{err: fmt.Errorf("StartLVDV failed: %w", err)}
		}
		c.updateAxisMoving(cmd.axis, true)

	case "jog":
		velocity := int64(cmd.floatVal)
		if err := ffi.WTNMC4ASetV(handle, cmd.axis, velocity); err != nil {
			return wtnmc4aResp{err: fmt.Errorf("SetV failed: %w", err)}
		}
		if err := ffi.WTNMC4AStartLVDV(handle, cmd.axis); err != nil {
			return wtnmc4aResp{err: fmt.Errorf("StartLVDV failed: %w", err)}
		}
		c.updateAxisMoving(cmd.axis, true)

	case "home":
		if err := ffi.WTNMC4AStartAutoHomeSearch(handle, cmd.axis); err != nil {
			return wtnmc4aResp{err: fmt.Errorf("StartAutoHomeSearch failed: %w", err)}
		}
		c.updateAxisMoving(cmd.axis, true)

	case "decStop":
		if err := ffi.WTNMC4ADecStop(handle, cmd.axis); err != nil {
			return wtnmc4aResp{err: fmt.Errorf("DecStop failed: %w", err)}
		}
		c.updateAxisMoving(cmd.axis, false)

	case "setLP":
		if err := ffi.WTNMC4ASetLP(handle, cmd.axis, cmd.value); err != nil {
			return wtnmc4aResp{err: fmt.Errorf("SetLP failed: %w", err)}
		}

	default:
		return wtnmc4aResp{err: fmt.Errorf("unknown command: %s", cmd.cmd)}
	}

	return wtnmc4aResp{}
}

// updateAxisMoving 更新轴运动状态
func (c *WTNMC4AMotionController) updateAxisMoving(axisIdx int, moving bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if axisIdx >= 0 && axisIdx < len(c.status.Axes) {
		c.status.Axes[axisIdx].Moving = moving
		if !moving {
			c.status.Axes[axisIdx].Velocity = 0
		}
	}
}

// 确保 WTNMC4AMotionController 实现了 MotionController 接口
var _ ports.MotionController = (*WTNMC4AMotionController)(nil)
