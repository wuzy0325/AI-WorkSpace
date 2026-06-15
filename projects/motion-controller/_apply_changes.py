#!/usr/bin/env python3
"""Apply priority channel changes to wtnmc4a_motion.go"""

import os

path = r"c:\Users\wuzhy\Documents\D\SVN\SoftWare\trunk\AI-Workspace\shared\device-sdk\go\motion\adapters\hardware\wtnmc4a_motion.go"

content = r"""//go:build windows

package hardware

import (
	"context"
	"fmt"
	"math"
	"sync"
	"time"

	"shared.local/device-sdk/go/ffi"
	"shared.local/device-sdk/go/motion/core"
	"shared.local/device-sdk/go/motion/ports"
)

const (
	speedMin = 1
	speedMax = 8000
)

// WTNMC4AMotionController 基于 WTNMC4A DLL FFI 的运动控制器适配器。
// FFI 调用通过 channel 序列化在独立 goroutine 中执行，避免跨线程 DLL 调用冲突。
// 每个控制器实例持有独立设备句柄，并发安全。
// 使用优先级通道（priorityCmdCh）处理 Stop 等关键命令，
// 确保动作命令不会被 Status 查询阻塞。
type WTNMC4AMotionController struct {
	mu            sync.RWMutex
	profile       core.MotionControllerProfile
	status        core.ControllerStatus
	handle        uintptr
	cmdCh         chan wtnmc4aCmd // 普通优先级命令通道（Status 查询等）
	priorityCmdCh chan wtnmc4aCmd // 高优先级命令通道（Stop/Jog/MoveTo 等动作命令）
	stopCh        chan struct{}
	connected     bool
}

// wtnmc4aCmd FFI 命令封装
type wtnmc4aCmd struct {
	cmd    string
	axis   int
	value  int64
	respCh chan wtnmc4aResp
}

type wtnmc4aResp struct {
	value int64
	err   error
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
				Name: axisCfg.Name,
			})
		}
	}
	return &WTNMC4AMotionController{
		profile:       profile,
		status:        status,
		cmdCh:         make(chan wtnmc4aCmd, 32),
		priorityCmdCh: make(chan wtnmc4aCmd, 16),
		stopCh:        make(chan struct{}),
	}
}

func (c *WTNMC4AMotionController) ApplyConfig(ctx context.Context, profile core.MotionControllerProfile) error {
	c.mu.Lock()
	c.profile = profile
	needReconfig := c.connected && c.handle != 0
	c.mu.Unlock()

	if needReconfig {
		c.configureAxisSpeeds()
	}
	return nil
}

func (c *WTNMC4AMotionController) GetProfile() core.MotionControllerProfile {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.profile
}

func (c *WTNMC4AMotionController) Connect(ctx context.Context) error {
	c.mu.Lock()

	if c.connected {
		c.mu.Unlock()
		return nil
	}

	if err := ffi.Init("WTNMC4A_64.dll"); err != nil {
		c.status.LastError = err.Error()
		c.mu.Unlock()
		return fmt.Errorf("WTNMC4A DLL 加载失败: %w", err)
	}

	handle, err := ffi.WTNMC4ADEVCreate(c.profile.Address, 200, 200)
	if err != nil {
		c.status.LastError = err.Error()
		c.mu.Unlock()
		return fmt.Errorf("WTNMC4A 连接 %s 失败: %w", c.profile.Address, err)
	}
	c.handle = handle

	c.connected = true
	c.status.Connected = true
	c.status.EmergencyStopped = false
	c.status.LastError = ""

	go c.commandLoop()
	c.mu.Unlock()

	c.configureAxisSpeeds()

	return nil
}

func (c *WTNMC4AMotionController) configureAxisSpeeds() {
	for _, axisCfg := range c.profile.Axes {
		if !axisCfg.Enabled {
			continue
		}
		if axisCfg.MaxSpeed == nil || *axisCfg.MaxSpeed <= 0 {
			continue
		}
		axisIdx := wtnmc4aAxisNum(axisCfg.Name)
		ppu := core.PulsesPerUnit(axisCfg)
		pulseSpeed := int64(math.Round(*axisCfg.MaxSpeed * math.Abs(ppu)))
		pulseSpeed = core.ClampInt64(pulseSpeed, speedMin, speedMax)
		_ = c.sendPriorityCommand(context.Background(), "setSpeed", axisIdx, pulseSpeed)
	}
}

func (c *WTNMC4AMotionController) Disconnect(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.connected {
		return nil
	}

	select {
	case <-c.stopCh:
	default:
		close(c.stopCh)
	}

	// 排空两个通道中的待处理命令
	drainCh := func(ch chan wtnmc4aCmd) {
		for {
			select {
			case cmd := <-ch:
				cmd.respCh <- wtnmc4aResp{err: fmt.Errorf("controller disconnected")}
			default:
				return
			}
		}
	}
	drainCh(c.priorityCmdCh)
	drainCh(c.cmdCh)

	c.stopCh = make(chan struct{})

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
	return nil
}

func (c *WTNMC4AMotionController) Status(ctx context.Context) (core.ControllerStatus, error) {
	c.mu.RLock()
	connected := c.connected
	axisConfigs := make([]core.AxisConfig, len(c.profile.Axes))
	copy(axisConfigs, c.profile.Axes)
	axesSnapshot := make([]core.AxisStatus, len(c.status.Axes))
	copy(axesSnapshot, c.status.Axes)
	c.mu.RUnlock()

	if !connected {
		return core.ControllerStatus{
			ID:        c.profile.ID,
			Name:      c.profile.Name,
			Type:      c.profile.Type,
			Connected: false,
			Axes:      axesSnapshot,
		}, fmt.Errorf("controller not connected")
	}

	for i := range axesSnapshot {
		axisIdx := wtnmc4aAxisNum(axesSnapshot[i].Name)
		axisCfg, ok := findAxisConfig(axisConfigs, axesSnapshot[i].Name)
		if !ok {
			continue
		}
		lp, err := c.sendCommandValue(ctx, "readLP", axisIdx, 0)
		if err == nil {
			axesSnapshot[i].Position = wtnmc4aPulseToEngineering(axisCfg, float64(lp))
		}
		rr1Bits, err := c.sendCommandValue(ctx, "getRR1", axisIdx, 0)
		if err == nil {
			rr1 := rr1FromBits(uint16(rr1Bits))
			axesSnapshot[i].Moving = rr1.ASND || rr1.DSND || rr1.CNST
			axesSnapshot[i].PosLimit = rr1.LMTP
			axesSnapshot[i].NegLimit = rr1.LMTM
			axesSnapshot[i].Homed = core.IsHomed(axesSnapshot[i].Position, axisCfg)
			if rr1.EMG {
				axesSnapshot[i].PositionError = 0
			}
		}
	}

	c.mu.Lock()
	if c.connected {
		c.status.Axes = axesSnapshot
		c.status.LastError = ""
	}
	status := c.status
	status.Axes = append([]core.AxisStatus(nil), c.status.Axes...)
	c.mu.Unlock()

	return status, nil
}

func (c *WTNMC4AMotionController) MoveTo(ctx context.Context, axis core.AxisName, position float64) error {
	axisIdx := wtnmc4aAxisNum(axis)
	c.mu.RLock()
	axisCfg, ok := c.axisConfigByName(axis)
	c.mu.RUnlock()
	if !ok {
		return fmt.Errorf("未知运动轴: %s", axis)
	}
	// 动作命令使用优先级通道，避免被 Status 查询阻塞
	return c.sendPriorityCommand(ctx, "moveTo", axisIdx, wtnmc4aEngineeringToPulse(axisCfg, position))
}

func (c *WTNMC4AMotionController) MoveBy(ctx context.Context, axis core.AxisName, delta float64) error {
	axisIdx := wtnmc4aAxisNum(axis)
	c.mu.RLock()
	axisCfg, ok := c.axisConfigByName(axis)
	c.mu.RUnlock()
	if !ok {
		return fmt.Errorf("未知运动轴: %s", axis)
	}

	// readLP 使用普通通道（属于 Status 查询类），moveTo 使用优先级通道
	currentPulse, err := c.sendCommandValue(ctx, "readLP", axisIdx, 0)
	if err != nil {
		return fmt.Errorf("ReadLP failed: %w", err)
	}
	deltaPulse := wtnmc4aEngineeringToPulse(axisCfg, delta)
	targetPulse := currentPulse + deltaPulse
	if targetPulse < 0 {
		targetPulse = 0
	}
	return c.sendPriorityCommand(ctx, "moveTo", axisIdx, targetPulse)
}

func (c *WTNMC4AMotionController) Jog(ctx context.Context, axis core.AxisName, velocity float64) error {
	axisIdx := wtnmc4aAxisNum(axis)
	c.mu.RLock()
	axisCfg, ok := c.axisConfigByName(axis)
	c.mu.RUnlock()
	if !ok {
		return fmt.Errorf("未知运动轴: %s", axis)
	}
	maxSpeed := core.ValueOrFloat(axisCfg.MaxSpeed, 100)
	jogSpeed := math.Abs(velocity)
	if jogSpeed > maxSpeed {
		jogSpeed = maxSpeed
	}
	if jogSpeed == 0 {
		jogSpeed = maxSpeed
	}
	ppu := core.PulsesPerUnit(axisCfg)
	pulseSpeed := int64(math.Round(jogSpeed * math.Abs(ppu)))
	pulseSpeed = core.ClampInt64(pulseSpeed, speedMin, speedMax)
	if velocity < 0 {
		pulseSpeed = -pulseSpeed
	}
	if axisCfg.Inverted {
		pulseSpeed = -pulseSpeed
	}
	// 动作命令使用优先级通道，避免被 Status 查询阻塞
	return c.sendPriorityCommand(ctx, "jog", axisIdx, pulseSpeed)
}

func (c *WTNMC4AMotionController) Home(ctx context.Context, axis core.AxisName) error {
	axisIdx := wtnmc4aAxisNum(axis)
	// 动作命令使用优先级通道
	return c.sendPriorityCommand(ctx, "home", axisIdx, 0)
}

func (c *WTNMC4AMotionController) Stop(ctx context.Context, axis core.AxisName) error {
	if axis == "" {
		var firstErr error
		c.mu.RLock()
		axes := append([]core.AxisStatus(nil), c.status.Axes...)
		c.mu.RUnlock()
		for _, axisStatus := range axes {
			axisIdx := wtnmc4aAxisNum(axisStatus.Name)
			// 使用优先级通道，确保 Stop 命令不被 Status 查询阻塞
			if err := c.sendPriorityCommand(ctx, "decStop", axisIdx, 0); err != nil && firstErr == nil {
				firstErr = err
			}
		}
		return firstErr
	}
	axisIdx := wtnmc4aAxisNum(axis)
	// 使用优先级通道，确保 Stop 命令不被 Status 查询阻塞
	return c.sendPriorityCommand(ctx, "decStop", axisIdx, 0)
}

func (c *WTNMC4AMotionController) EmergencyStop(ctx context.Context) error {
	c.mu.RLock()
	connected := c.connected
	c.mu.RUnlock()

	if !connected {
		return fmt.Errorf("controller not connected")
	}

	// 通过优先级通道发送急停命令，避免与 commandLoop 中的 FFI 调用产生竞态
	var firstErr error
	c.mu.RLock()
	axes := append([]core.AxisStatus(nil), c.status.Axes...)
	c.mu.RUnlock()
	for _, axisStatus := range axes {
		axisIdx := wtnmc4aAxisNum(axisStatus.Name)
		if err := c.sendPriorityCommand(ctx, "instStop", axisIdx, 0); err != nil && firstErr == nil {
			firstErr = err
		}
	}

	c.mu.Lock()
	c.status.EmergencyStopped = true
	for i := range c.status.Axes {
		c.status.Axes[i].Moving = false
		c.status.Axes[i].Velocity = 0
	}
	c.mu.Unlock()

	return firstErr
}

func (c *WTNMC4AMotionController) ResetEmergencyStop(ctx context.Context) error {
	c.mu.RLock()
	connected := c.connected
	c.mu.RUnlock()

	if !connected {
		return fmt.Errorf("controller not connected")
	}
	// 通过优先级通道发送复位命令，避免与 commandLoop 中的 FFI 调用产生竞态
	if err := c.sendPriorityCommand(ctx, "reset", 0, 0); err != nil {
		return err
	}
	c.mu.Lock()
	c.status.EmergencyStopped = false
	c.mu.Unlock()
	return nil
}

func (c *WTNMC4AMotionController) DefinePosition(ctx context.Context, axis core.AxisName, position float64) error {
	axisIdx := wtnmc4aAxisNum(axis)
	c.mu.RLock()
	axisCfg, ok := c.axisConfigByName(axis)
	c.mu.RUnlock()
	if !ok {
		return fmt.Errorf("未知运动轴: %s", axis)
	}
	pulse := wtnmc4aEngineeringToPulse(axisCfg, position)
	// 动作命令使用优先级通道
	if err := c.sendPriorityCommand(ctx, "setLP", axisIdx, pulse); err != nil {
		return err
	}
	encoderCount := core.EngineeringToEncoderCount(axisCfg, position)
	return c.sendPriorityCommand(ctx, "setEP", axisIdx, encoderCount)
}

// commandLoop 所有 FFI 调用在此 goroutine 中序列化执行
// 优先处理高优先级命令（Stop/Jog/MoveTo），确保动作命令不被 Status 查询阻塞
func (c *WTNMC4AMotionController) commandLoop() {
	for {
		// 优先检查高优先级通道，确保 Stop 等关键命令立即执行
		select {
		case <-c.stopCh:
			return
		case cmd := <-c.priorityCmdCh:
			resp := c.executeCommand(cmd)
			select {
			case cmd.respCh <- resp:
			default:
			}
		default:
		}

		// 同时检查两个通道，高优先级通道优先
		select {
		case <-c.stopCh:
			return
		case cmd := <-c.priorityCmdCh:
			resp := c.executeCommand(cmd)
			select {
			case cmd.respCh <- resp:
			default:
			}
		case cmd := <-c.cmdCh:
			resp := c.executeCommand(cmd)
			select {
			case cmd.respCh <- resp:
			default:
			}
		}
	}
}

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
		if err := ffi.WTNMC4ASetV(handle, cmd.axis, cmd.value); err != nil {
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

	case "instStop":
		if err := ffi.WTNMC4AInstStop(handle, cmd.axis); err != nil {
			return wtnmc4aResp{err: fmt.Errorf("InstStop failed: %w", err)}
		}
		c.updateAxisMoving(cmd.axis, false)

	case "reset":
		if err := ffi.WTNMC4AReset(handle); err != nil {
			return wtnmc4aResp{err: fmt.Errorf("Reset failed: %w", err)}
		}

	case "setLP":
		if err := ffi.WTNMC4ASetLP(handle, cmd.axis, cmd.value); err != nil {
			return wtnmc4aResp{err: fmt.Errorf("SetLP failed: %w", err)}
		}

	case "setEP":
		if err := ffi.WTNMC4ASetEP(handle, cmd.axis, cmd.value); err != nil {
			return wtnmc4aResp{err: fmt.Errorf("SetEP failed: %w", err)}
		}

	case "readLP":
		v, err := ffi.WTNMC4AReadLP(handle, cmd.axis)
		return wtnmc4aResp{value: v, err: err}

	case "getRR1":
		rr1 := ffi.WTNMC4AGetRR1Status(handle, cmd.axis)
		var bits uint16
		if rr1.CMPP {
			bits |= 1 << 0
		}
		if rr1.CMPM {
			bits |= 1 << 1
		}
		if rr1.ASND {
			bits |= 1 << 2
		}
		if rr1.CNST {
			bits |= 1 << 3
		}
		if rr1.DSND {
			bits |= 1 << 4
		}
		if rr1.IN0 {
			bits |= 1 << 5
		}
		if rr1.IN1 {
			bits |= 1 << 6
		}
		if rr1.IN2 {
			bits |= 1 << 7
		}
		if rr1.IN3 {
			bits |= 1 << 8
		}
		if rr1.LMTP {
			bits |= 1 << 9
		}
		if rr1.LMTM {
			bits |= 1 << 10
		}
		if rr1.ALARM {
			bits |= 1 << 11
		}
		if rr1.EMG {
			bits |= 1 << 12
		}
		return wtnmc4aResp{value: int64(bits)}

	case "setSpeed":
		if err := ffi.WTNMC4ASetV(handle, cmd.axis, cmd.value); err != nil {
			return wtnmc4aResp{err: fmt.Errorf("SetV(speed) failed: %w", err)}
		}

	default:
		return wtnmc4aResp{err: fmt.Errorf("unknown command: %s", cmd.cmd)}
	}

	return wtnmc4aResp{}
}

func (c *WTNMC4AMotionController) updateAxisMoving(hwAxis int, moving bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	name := wtnmc4aHwToName(hwAxis)
	for i := range c.status.Axes {
		if c.status.Axes[i].Name == name {
			c.status.Axes[i].Moving = moving
			if !moving {
				c.status.Axes[i].Velocity = 0
			}
			return
		}
	}
}

func (c *WTNMC4AMotionController) axisConfigByName(axis core.AxisName) (core.AxisConfig, bool) {
	for _, ac := range c.profile.Axes {
		if ac.Enabled && ac.Name == axis {
			return ac, true
		}
	}
	return core.AxisConfig{}, false
}

func (c *WTNMC4AMotionController) sendCommand(ctx context.Context, cmd string, axis int, value int64) error {
	_, err := c.sendCommandValue(ctx, cmd, axis, value)
	return err
}

func (c *WTNMC4AMotionController) sendCommandValue(ctx context.Context, cmd string, axis int, value int64) (int64, error) {
	c.mu.RLock()
	connected := c.connected
	c.mu.RUnlock()

	if !connected {
		return 0, fmt.Errorf("controller not connected")
	}

	cmdMsg := wtnmc4aCmd{
		cmd:    cmd,
		axis:   axis,
		value:  value,
		respCh: make(chan wtnmc4aResp, 1),
	}

	select {
	case c.cmdCh <- cmdMsg:
	case <-ctx.Done():
		return 0, ctx.Err()
	case <-time.After(5 * time.Second):
		return 0, fmt.Errorf("command timeout: %s", cmd)
	}

	select {
	case resp := <-cmdMsg.respCh:
		return resp.value, resp.err
	case <-ctx.Done():
		return 0, ctx.Err()
	case <-time.After(10 * time.Second):
		return 0, fmt.Errorf("response timeout: %s", cmd)
	}
}

// sendPriorityCommand 发送高优先级命令（Stop/Jog/MoveTo 等），优先于 Status 查询处理
func (c *WTNMC4AMotionController) sendPriorityCommand(ctx context.Context, cmd string, axis int, value int64) error {
	_, err := c.sendPriorityCommandValue(ctx, cmd, axis, value)
	return err
}

// sendPriorityCommandValue 发送高优先级命令并等待响应值
func (c *WTNMC4AMotionController) sendPriorityCommandValue(ctx context.Context, cmd string, axis int, value int64) (int64, error) {
	c.mu.RLock()
	connected := c.connected
	c.mu.RUnlock()

	if !connected {
		return 0, fmt.Errorf("controller not connected")
	}

	cmdMsg := wtnmc4aCmd{
		cmd:    cmd,
		axis:   axis,
		value:  value,
		respCh: make(chan wtnmc4aResp, 1),
	}

	select {
	case c.priorityCmdCh <- cmdMsg:
	case <-ctx.Done():
		return 0, ctx.Err()
	case <-time.After(5 * time.Second):
		return 0, fmt.Errorf("priority command timeout: %s", cmd)
	}

	select {
	case resp := <-cmdMsg.respCh:
		return resp.value, resp.err
	case <-ctx.Done():
		return 0, ctx.Err()
	case <-time.After(10 * time.Second):
		return 0, fmt.Errorf("priority response timeout: %s", cmd)
	}
}

func findAxisConfig(configs []core.AxisConfig, axis core.AxisName) (core.AxisConfig, bool) {
	for _, ac := range configs {
		if ac.Name == axis {
			return ac, true
		}
	}
	return core.AxisConfig{}, false
}

func wtnmc4aAxisNum(axis core.AxisName) int {
	switch axis {
	case core.AxisX:
		return 0
	case core.AxisY:
		return 1
	case core.AxisZ:
		return 2
	case core.AxisU:
		return 3
	default:
		return 0
	}
}

func wtnmc4aHwToName(hw int) core.AxisName {
	switch hw {
	case 0:
		return core.AxisX
	case 1:
		return core.AxisY
	case 2:
		return core.AxisZ
	case 3:
		return core.AxisU
	default:
		return ""
	}
}

func wtnmc4aEngineeringToPulse(axisCfg core.AxisConfig, value float64) int64 {
	pulse := core.EngineeringToPulse(axisCfg, value)
	if axisCfg.Inverted {
		pulse = -pulse
	}
	return pulse
}

func wtnmc4aPulseToEngineering(axisCfg core.AxisConfig, pulse float64) float64 {
	signedPulse := pulse
	if axisCfg.Inverted {
		signedPulse = -signedPulse
	}
	return core.PulseToEngineering(axisCfg, signedPulse)
}

func rr1FromBits(status uint16) ffi.RR1Status {
	return ffi.RR1Status{
		CMPP: (status&(1<<0)) != 0, CMPM: (status&(1<<1)) != 0,
		ASND: (status&(1<<2)) != 0, CNST: (status&(1<<3)) != 0,
		DSND: (status&(1<<4)) != 0, IN0: (status&(1<<5)) != 0,
		IN1: (status&(1<<6)) != 0, IN2: (status&(1<<7)) != 0,
		IN3: (status&(1<<8)) != 0, LMTP: (status&(1<<9)) != 0,
		LMTM: (status&(1<<10)) != 0, ALARM: (status&(1<<11)) != 0,
		EMG: (status&(1<<12)) != 0,
	}
}

var _ ports.MotionController = (*WTNMC4AMotionController)(nil)
"""

with open(path, "w", encoding="utf-8") as f:
    f.write(content)

print(f"Successfully wrote {len(content)} chars to {path}")
