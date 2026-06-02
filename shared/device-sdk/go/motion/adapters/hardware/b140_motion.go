package hardware

import (
	"bufio"
	"context"
	"fmt"
	"math"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"shared.local/device-sdk/go/motion/core"
)

const (
	b140DefaultPort    = 5000
	b140CommandTimeout = 5 * time.Second
	b140DefaultStepDeg = 1.8
	b140DefaultScale   = 0.005
)

// B140MotionController implements Galil DMC-B140-M TCP ASCII motion control.
type B140MotionController struct {
	mu                 sync.Mutex
	profile            core.MotionControllerProfile
	status             core.ControllerStatus
	conn               net.Conn
	reader             *bufio.Reader
	directionSignature string
}

// NewB140MotionController creates a B140 motion controller adapter.
func NewB140MotionController(profile core.MotionControllerProfile) *B140MotionController {
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
			status.Axes = append(status.Axes, core.AxisStatus{Name: axisCfg.Name})
		}
	}

	return &B140MotionController{profile: profile, status: status}
}

// GetProfile returns the controller profile.
func (c *B140MotionController) GetProfile() core.MotionControllerProfile {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.profile
}

// Connect opens TCP connection, enables all axes, and configures direction.
func (c *B140MotionController) Connect(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.conn != nil && c.status.Connected {
		return nil
	}

	port := c.profile.Port
	if port == 0 {
		port = b140DefaultPort
	}
	address := fmt.Sprintf("%s:%d", c.profile.Address, port)
	conn, err := (&net.Dialer{}).DialContext(ctx, "tcp", address)
	if err != nil {
		c.status.LastError = err.Error()
		return err
	}
	c.conn = conn
	c.reader = bufio.NewReader(conn)
	c.directionSignature = ""

	if _, err := c.sendCommandLocked(ctx, "SH"); err != nil {
		_ = conn.Close()
		c.conn = nil
		c.reader = nil
		c.status.LastError = err.Error()
		return err
	}
	if err := c.ensureDirectionConfiguredLocked(ctx); err != nil {
		_ = conn.Close()
		c.conn = nil
		c.reader = nil
		c.status.LastError = err.Error()
		return err
	}

	c.status.Connected = true
	c.status.EmergencyStopped = false
	c.status.LastError = ""
	return nil
}

// Disconnect closes TCP connection and clears cached hardware state.
func (c *B140MotionController) Disconnect(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	var err error
	if c.conn != nil {
		err = c.conn.Close()
	}
	c.conn = nil
	c.reader = nil
	c.directionSignature = ""
	c.status.Connected = false
	for i := range c.status.Axes {
		c.status.Axes[i].Moving = false
		c.status.Axes[i].Velocity = 0
		c.status.Axes[i].Compensating = false
	}
	return err
}

// Status reads register position, motion state, and limit switches.
func (c *B140MotionController) Status(ctx context.Context) (core.ControllerStatus, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if err := c.checkConnectedLocked(); err != nil {
		return c.copyStatusLocked(), err
	}
	if err := c.ensureDirectionConfiguredLocked(ctx); err != nil {
		c.status.LastError = err.Error()
		return c.copyStatusLocked(), err
	}

	tdPayload, err := c.sendCommandLocked(ctx, "TD")
	if err != nil {
		c.status.LastError = err.Error()
		return c.copyStatusLocked(), err
	}
	registerPositions, err := parseB140Numbers(tdPayload)
	if err != nil {
		c.status.LastError = err.Error()
		return c.copyStatusLocked(), err
	}

	tsPayload, err := c.sendCommandLocked(ctx, "TS")
	if err != nil {
		c.status.LastError = err.Error()
		return c.copyStatusLocked(), err
	}
	statusBytes, err := parseB140Numbers(tsPayload)
	if err != nil {
		c.status.LastError = err.Error()
		return c.copyStatusLocked(), err
	}

	for i := range c.status.Axes {
		axisStatus := &c.status.Axes[i]
		axisCfg, ok := c.axisConfigLocked(axisStatus.Name)
		if !ok {
			continue
		}
		physical, axisIndex, err := b140PhysicalAxis(axisStatus.Name)
		if err != nil {
			c.status.LastError = err.Error()
			return c.copyStatusLocked(), err
		}

		registerPulse := numberAt(registerPositions, axisIndex)
		position := pulseToEngineering(axisCfg, registerPulse)
		if axisCfg.PositionSource == core.PositionSourceEncoder {
			payload, err := c.sendCommandLocked(ctx, "TP"+physical)
			if err == nil {
				encoderCount, parseErr := strconv.ParseFloat(strings.TrimSpace(payload), 64)
				if parseErr == nil {
					position = encoderCountToEngineering(axisCfg, encoderCount)
				}
			}
		}

		forwardPayload, err := c.sendCommandLocked(ctx, "MG _LF"+physical)
		if err != nil {
			c.status.LastError = err.Error()
			return c.copyStatusLocked(), err
		}
		reversePayload, err := c.sendCommandLocked(ctx, "MG _LR"+physical)
		if err != nil {
			c.status.LastError = err.Error()
			return c.copyStatusLocked(), err
		}

		axisStatus.Position = position
		axisStatus.Moving = int(numberAt(statusBytes, axisIndex))&0x80 != 0
		axisStatus.Homed = b140IsHomed(position, axisCfg)
		axisStatus.PosLimit = parseB140Limit(forwardPayload)
		axisStatus.NegLimit = parseB140Limit(reversePayload)
		axisStatus.Compensating = false
		axisStatus.CompensationError = ""
		axisStatus.PositionError = 0
	}
	c.status.LastError = ""
	return c.copyStatusLocked(), nil
}

// MoveTo moves an axis to an absolute engineering position.
func (c *B140MotionController) MoveTo(ctx context.Context, axis core.AxisName, position float64) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	axisCfg, physical, err := c.prepareAxisCommandLocked(ctx, axis)
	if err != nil {
		return err
	}
	pulse := engineeringToPulse(axisCfg, position)
	if _, err := c.sendCommandLocked(ctx, fmt.Sprintf("PA%s=%d", physical, pulse)); err != nil {
		c.status.LastError = err.Error()
		return err
	}
	if _, err := c.sendCommandLocked(ctx, "BG"+physical); err != nil {
		c.status.LastError = err.Error()
		return err
	}
	c.status.LastError = ""
	return nil
}

// MoveBy moves an axis by a relative engineering delta.
func (c *B140MotionController) MoveBy(ctx context.Context, axis core.AxisName, delta float64) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	axisCfg, physical, err := c.prepareAxisCommandLocked(ctx, axis)
	if err != nil {
		return err
	}
	deltaPulse := engineeringToPulse(axisCfg, delta)
	if deltaPulse == 0 {
		return nil
	}
	if _, err := c.sendCommandLocked(ctx, fmt.Sprintf("PR%s=%d", physical, deltaPulse)); err != nil {
		c.status.LastError = err.Error()
		return err
	}
	if _, err := c.sendCommandLocked(ctx, "BG"+physical); err != nil {
		c.status.LastError = err.Error()
		return err
	}
	c.status.LastError = ""
	return nil
}

// Jog moves one engineering unit in the velocity direction.
func (c *B140MotionController) Jog(ctx context.Context, axis core.AxisName, velocity float64) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	axisCfg, physical, err := c.prepareAxisCommandLocked(ctx, axis)
	if err != nil {
		return err
	}
	maxSpeed := valueOrFloat(axisCfg.MaxSpeed, 10)
	jogSpeed := math.Abs(velocity)
	if jogSpeed > maxSpeed {
		jogSpeed = maxSpeed
	}
	if jogSpeed == 0 {
		jogSpeed = maxSpeed
	}
	pulseSpeed := engineeringToPulse(axisCfg, jogSpeed)
	step := 1.0
	if velocity < 0 {
		step = -1
	}
	stepPulse := engineeringToPulse(axisCfg, step)

	if _, err := c.sendCommandLocked(ctx, fmt.Sprintf("SP%s=%d", physical, pulseSpeed)); err != nil {
		c.status.LastError = err.Error()
		return err
	}
	if _, err := c.sendCommandLocked(ctx, fmt.Sprintf("PR%s=%d", physical, stepPulse)); err != nil {
		c.status.LastError = err.Error()
		return err
	}
	if _, err := c.sendCommandLocked(ctx, "BG"+physical); err != nil {
		c.status.LastError = err.Error()
		return err
	}
	c.status.LastError = ""
	return nil
}

// Home starts the Galil home mode on one axis.
func (c *B140MotionController) Home(ctx context.Context, axis core.AxisName) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	_, physical, err := c.prepareAxisCommandLocked(ctx, axis)
	if err != nil {
		return err
	}
	if _, err := c.sendCommandLocked(ctx, "HM"+physical); err != nil {
		c.status.LastError = err.Error()
		return err
	}
	if _, err := c.sendCommandLocked(ctx, "BG"+physical); err != nil {
		c.status.LastError = err.Error()
		return err
	}
	c.status.LastError = ""
	return nil
}

// Stop decelerates either one axis or all axes when axis is empty.
func (c *B140MotionController) Stop(ctx context.Context, axis core.AxisName) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if err := c.checkConnectedLocked(); err != nil {
		return err
	}
	cmd := "ST"
	if axis != "" {
		physical, _, err := b140PhysicalAxis(axis)
		if err != nil {
			return err
		}
		if _, ok := c.axisConfigLocked(axis); !ok {
			return fmt.Errorf("unknown motion axis: %s", axis)
		}
		cmd += physical
	}
	if _, err := c.sendCommandLocked(ctx, cmd); err != nil {
		c.status.LastError = err.Error()
		return err
	}
	c.status.LastError = ""
	return nil
}

// EmergencyStop aborts all motion immediately.
func (c *B140MotionController) EmergencyStop(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if err := c.checkConnectedLocked(); err != nil {
		return err
	}
	if _, err := c.sendCommandLocked(ctx, "AB"); err != nil {
		c.status.LastError = err.Error()
		return err
	}
	c.status.EmergencyStopped = true
	c.status.LastError = ""
	return nil
}

// ResetEmergencyStop re-enables servo output and refreshes direction config.
func (c *B140MotionController) ResetEmergencyStop(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if err := c.checkConnectedLocked(); err != nil {
		return err
	}
	if _, err := c.sendCommandLocked(ctx, "SH"); err != nil {
		c.status.LastError = err.Error()
		return err
	}
	c.directionSignature = ""
	if err := c.ensureDirectionConfiguredLocked(ctx); err != nil {
		c.status.LastError = err.Error()
		return err
	}
	c.status.EmergencyStopped = false
	c.status.LastError = ""
	return nil
}

// DefinePosition sets both register and encoder position counters.
func (c *B140MotionController) DefinePosition(ctx context.Context, axis core.AxisName, position float64) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	axisCfg, physical, err := c.prepareAxisCommandLocked(ctx, axis)
	if err != nil {
		return err
	}
	pulse := engineeringToPulse(axisCfg, position)
	encoderCount := engineeringToEncoderCount(axisCfg, position)
	if _, err := c.sendCommandLocked(ctx, fmt.Sprintf("DP%s=%d", physical, pulse)); err != nil {
		c.status.LastError = err.Error()
		return err
	}
	if _, err := c.sendCommandLocked(ctx, fmt.Sprintf("DE%s=%d", physical, encoderCount)); err != nil {
		c.status.LastError = err.Error()
		return err
	}
	c.status.LastError = ""
	return nil
}

func (c *B140MotionController) prepareAxisCommandLocked(ctx context.Context, axis core.AxisName) (core.AxisConfig, string, error) {
	if err := c.checkConnectedLocked(); err != nil {
		return core.AxisConfig{}, "", err
	}
	if err := c.ensureDirectionConfiguredLocked(ctx); err != nil {
		c.status.LastError = err.Error()
		return core.AxisConfig{}, "", err
	}
	axisCfg, ok := c.axisConfigLocked(axis)
	if !ok {
		return core.AxisConfig{}, "", fmt.Errorf("unknown motion axis: %s", axis)
	}
	if axisCfg.PositionSource == core.PositionSourceEncoder && axisCfg.EncoderCompensation != nil && axisCfg.EncoderCompensation.Enabled {
		return core.AxisConfig{}, "", fmt.Errorf("B140 encoder compensation is not implemented")
	}
	physical, _, err := b140PhysicalAxis(axis)
	if err != nil {
		return core.AxisConfig{}, "", err
	}
	return axisCfg, physical, nil
}

func (c *B140MotionController) checkConnectedLocked() error {
	if c.conn == nil || c.reader == nil || !c.status.Connected {
		return fmt.Errorf("controller not connected")
	}
	return nil
}

func (c *B140MotionController) ensureDirectionConfiguredLocked(ctx context.Context) error {
	signature := c.directionConfigSignatureLocked()
	if c.directionSignature == signature {
		return nil
	}
	for _, axisCfg := range c.profile.Axes {
		if !axisCfg.Enabled {
			continue
		}
		physical, _, err := b140PhysicalAxis(axisCfg.Name)
		if err != nil {
			return err
		}
		motorDirection := 2
		encoderDirection := 0
		if axisCfg.Inverted {
			motorDirection = -2
			encoderDirection = 2
		}
		if _, err := c.sendCommandLocked(ctx, fmt.Sprintf("MT%s=%d", physical, motorDirection)); err != nil {
			return err
		}
		if _, err := c.sendCommandLocked(ctx, fmt.Sprintf("CE%s=%d", physical, encoderDirection)); err != nil {
			return err
		}
	}
	c.directionSignature = signature
	return nil
}

func (c *B140MotionController) directionConfigSignatureLocked() string {
	parts := make([]string, 0, len(c.profile.Axes))
	for _, axisCfg := range c.profile.Axes {
		if axisCfg.Enabled {
			parts = append(parts, fmt.Sprintf("%s:%t", axisCfg.Name, axisCfg.Inverted))
		}
	}
	return strings.Join(parts, "|")
}

func (c *B140MotionController) sendCommandLocked(ctx context.Context, cmd string) (string, error) {
	if c.conn == nil || c.reader == nil {
		return "", fmt.Errorf("controller not connected")
	}
	deadline := time.Now().Add(b140CommandTimeout)
	if ctxDeadline, ok := ctx.Deadline(); ok && ctxDeadline.Before(deadline) {
		deadline = ctxDeadline
	}
	if err := c.conn.SetDeadline(deadline); err != nil {
		return "", err
	}
	if _, err := c.conn.Write([]byte(cmd + "\r")); err != nil {
		return "", err
	}

	var payload strings.Builder
	for {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		default:
		}
		b, err := c.reader.ReadByte()
		if err != nil {
			return "", err
		}
		switch b {
		case ':':
			return strings.TrimSpace(payload.String()), nil
		case '?':
			return "", fmt.Errorf("B140 command %q failed: %s", cmd, strings.TrimSpace(payload.String()))
		default:
			payload.WriteByte(b)
		}
	}
}

func (c *B140MotionController) axisConfigLocked(axis core.AxisName) (core.AxisConfig, bool) {
	for _, axisCfg := range c.profile.Axes {
		if axisCfg.Enabled && axisCfg.Name == axis {
			return axisCfg, true
		}
	}
	return core.AxisConfig{}, false
}

func (c *B140MotionController) copyStatusLocked() core.ControllerStatus {
	status := c.status
	status.Axes = append([]core.AxisStatus(nil), c.status.Axes...)
	return status
}

func b140PhysicalAxis(axis core.AxisName) (string, int, error) {
	switch axis {
	case core.AxisX:
		return "A", 0, nil
	case core.AxisY:
		return "B", 1, nil
	case core.AxisZ:
		return "C", 2, nil
	case core.AxisU:
		return "D", 3, nil
	default:
		return "", 0, fmt.Errorf("unknown motion axis: %s", axis)
	}
}

func parseB140Numbers(payload string) ([]float64, error) {
	if strings.TrimSpace(payload) == "" {
		return nil, nil
	}
	parts := strings.Split(payload, ",")
	values := make([]float64, 0, len(parts))
	for _, part := range parts {
		value, err := strconv.ParseFloat(strings.TrimSpace(part), 64)
		if err != nil {
			return nil, err
		}
		values = append(values, value)
	}
	return values, nil
}

func numberAt(values []float64, index int) float64 {
	if index < 0 || index >= len(values) {
		return 0
	}
	return values[index]
}

func parseB140Limit(payload string) bool {
	value, err := strconv.ParseFloat(strings.TrimSpace(payload), 64)
	if err != nil {
		return false
	}
	return value < 1
}

func engineeringToPulse(axisCfg core.AxisConfig, value float64) int64 {
	return int64(math.Round(value * pulsesPerUnit(axisCfg)))
}

func pulseToEngineering(axisCfg core.AxisConfig, pulse float64) float64 {
	pulses := pulsesPerUnit(axisCfg)
	if pulses == 0 {
		return 0
	}
	return pulse / pulses
}

func engineeringToEncoderCount(axisCfg core.AxisConfig, value float64) int64 {
	scale := valueOrFloat(axisCfg.EncoderScale, b140DefaultScale)
	if scale == 0 {
		return 0
	}
	return int64(math.Round(value / scale))
}

func encoderCountToEngineering(axisCfg core.AxisConfig, count float64) float64 {
	return count * valueOrFloat(axisCfg.EncoderScale, b140DefaultScale)
}

func pulsesPerUnit(axisCfg core.AxisConfig) float64 {
	stepAngleDeg := valueOrFloat(axisCfg.StepsPerRev, b140DefaultStepDeg)
	if stepAngleDeg == 0 {
		stepAngleDeg = b140DefaultStepDeg
	}
	microSteps := valueOrInt(axisCfg.MicroSteps, 1)
	stepsPerRev := 360 / stepAngleDeg
	if axisCfg.Kind == core.AxisKindRotary {
		gearRatio := valueOrFloat(axisCfg.GearRatio, 1)
		if gearRatio == 0 {
			gearRatio = 1
		}
		return (stepsPerRev * float64(microSteps) * gearRatio) / 360
	}
	lead := valueOrFloat(axisCfg.Lead, 1)
	if lead == 0 {
		lead = 1
	}
	return (stepsPerRev * float64(microSteps)) / lead
}

func valueOrFloat(value *float64, fallback float64) float64 {
	if value == nil {
		return fallback
	}
	return *value
}

func valueOrInt(value *int, fallback int) int {
	if value == nil {
		return fallback
	}
	return *value
}

func b140IsHomed(position float64, axisCfg core.AxisConfig) bool {
	if math.Abs(position) >= 0.001 {
		return false
	}
	if axisCfg.MinLimit != nil && position < *axisCfg.MinLimit {
		return false
	}
	if axisCfg.MaxLimit != nil && position > *axisCfg.MaxLimit {
		return false
	}
	return true
}
