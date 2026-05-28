package hardware

import (
	"testing"
	"time"

	"wind-daq/services/api-go/internal/core/motion"
)

func createTestMotionProfile(axisNames []motion.AxisName) motion.MotionControllerProfile {
	axes := make([]motion.AxisConfig, len(axisNames))
	for i, name := range axisNames {
		axes[i] = motion.AxisConfig{
			Name:     name,
			Enabled:  true,
			Kind:     motion.AxisKindLinear,
			MaxSpeed: func() *float64 { v := 10.0; return &v }(),
		}
	}
	return motion.MotionControllerProfile{
		ID:      "test-motion",
		Name:    "Test Controller",
		Type:    motion.ControllerTypeSimulated,
		Address: "127.0.0.1",
		Port:    9000,
		Axes:    axes,
	}
}

func waitFor(t *testing.T, timeout time.Duration, label string, fn func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if fn() {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", label)
}

func TestSimulatedMotionControllerConnectAndStatus(t *testing.T) {
	profile := createTestMotionProfile([]motion.AxisName{"X", "Y", "Z"})
	c := NewSimulatedMotionController(profile)
	if err := c.Connect(); err != nil {
		t.Fatalf("Connect returned error: %v", err)
	}
	s := c.Status()
	if !s.Connected {
		t.Fatal("expected connected")
	}
	if len(s.Axes) != 3 {
		t.Fatalf("expected 3 axes, got %d", len(s.Axes))
	}
}

func TestSimulatedMotionControllerMoveTo(t *testing.T) {
	profile := createTestMotionProfile([]motion.AxisName{"X"})
	c := NewSimulatedMotionController(profile)
	c.Connect()
	if err := c.MoveTo("X", 100); err != nil {
		t.Fatalf("MoveTo returned error: %v", err)
	}
	waitFor(t, 2*time.Second, "MoveTo to reach position 100", func() bool {
		return c.Status().Axes[0].Position == 100
	})
}

func TestSimulatedMotionControllerMoveToContinuous(t *testing.T) {
	profile := createTestMotionProfile([]motion.AxisName{"X"})
	c := NewSimulatedMotionController(profile)
	c.Connect()

	if err := c.MoveTo("X", 50); err != nil {
		t.Fatalf("MoveTo returned error: %v", err)
	}

	// 等待一小段时间，检查位置是否在中间值
	time.Sleep(50 * time.Millisecond)
	midStatus := c.Status()
	if midStatus.Axes[0].Position == 0 || midStatus.Axes[0].Position == 50 {
		t.Fatalf("expected intermediate position, got %f", midStatus.Axes[0].Position)
	}
	if !midStatus.Axes[0].Moving {
		t.Fatal("expected axis to be moving during motion")
	}

	// 等待运动完成
	waitFor(t, 2*time.Second, "MoveTo to reach position 50", func() bool {
		return c.Status().Axes[0].Position == 50
	})

	finalStatus := c.Status()
	if finalStatus.Axes[0].Moving {
		t.Fatal("expected axis to stop moving after reaching target")
	}
	if finalStatus.Axes[0].Velocity != 0 {
		t.Fatalf("expected velocity 0, got %f", finalStatus.Axes[0].Velocity)
	}
}

func TestSimulatedMotionControllerMoveBy(t *testing.T) {
	profile := createTestMotionProfile([]motion.AxisName{"X"})
	c := NewSimulatedMotionController(profile)
	c.Connect()
	c.MoveTo("X", 50)
	waitFor(t, 2*time.Second, "MoveTo to reach position 50", func() bool {
		return c.Status().Axes[0].Position == 50
	})
	c.MoveBy("X", 25)
	waitFor(t, 2*time.Second, "MoveBy to reach position 75", func() bool {
		return c.Status().Axes[0].Position == 75
	})
}

func TestSimulatedMotionControllerJogAndStop(t *testing.T) {
	profile := createTestMotionProfile([]motion.AxisName{"Y"})
	c := NewSimulatedMotionController(profile)
	c.Connect()
	if err := c.Jog("Y", 5); err != nil {
		t.Fatalf("Jog returned error: %v", err)
	}
	s := c.Status()
	if !s.Axes[0].Moving {
		t.Fatal("expected moving after jog")
	}
	if err := c.Stop("Y"); err != nil {
		t.Fatalf("Stop returned error: %v", err)
	}
	s2 := c.Status()
	if s2.Axes[0].Moving {
		t.Fatal("expected not moving after stop")
	}
}

func TestSimulatedMotionControllerJogLimit(t *testing.T) {
	maxSpeed := 10.0
	profile := motion.MotionControllerProfile{
		ID:   "test-jog-limit",
		Name: "Test Jog Limit",
		Type: motion.ControllerTypeSimulated,
		Axes: []motion.AxisConfig{
			{Name: "X", Enabled: true, Kind: motion.AxisKindLinear, MaxSpeed: &maxSpeed, MaxLimit: func() *float64 { v := 20.0; return &v }()},
		},
	}
	c := NewSimulatedMotionController(profile)
	c.Connect()

	if err := c.Jog("X", 15); err != nil {
		t.Fatalf("Jog returned error: %v", err)
	}

	// 等待Jog超时或限位触发
	waitFor(t, 3*time.Second, "Jog to stop at limit", func() bool {
		s := c.Status()
		return !s.Axes[0].Moving || s.Axes[0].PosLimit
	})

	s := c.Status()
	if s.Axes[0].Position > 20.1 {
		t.Fatalf("position %f exceeded limit 20", s.Axes[0].Position)
	}
}

func TestSimulatedMotionControllerHome(t *testing.T) {
	profile := createTestMotionProfile([]motion.AxisName{"X"})
	c := NewSimulatedMotionController(profile)
	c.Connect()
	c.MoveTo("X", 100)
	waitFor(t, 2*time.Second, "MoveTo to reach position 100", func() bool {
		return c.Status().Axes[0].Position == 100
	})
	if err := c.Home("X"); err != nil {
		t.Fatalf("Home returned error: %v", err)
	}
	waitFor(t, 2*time.Second, "Home to complete", func() bool {
		s := c.Status()
		return s.Axes[0].Homed && s.Axes[0].Position == 0
	})
}

func TestSimulatedMotionControllerDefinePosition(t *testing.T) {
	profile := createTestMotionProfile([]motion.AxisName{"X"})
	c := NewSimulatedMotionController(profile)
	c.Connect()
	if err := c.DefinePosition("X", 42); err != nil {
		t.Fatalf("DefinePosition returned error: %v", err)
	}
	s := c.Status()
	if s.Axes[0].Position != 42 {
		t.Fatalf("expected position 42, got %f", s.Axes[0].Position)
	}
}

func TestSimulatedMotionControllerMoveToLimitViolation(t *testing.T) {
	maxSpeed := 10.0
	profile := motion.MotionControllerProfile{
		ID:   "test-motion-limit",
		Name: "Test Limit",
		Type: motion.ControllerTypeSimulated,
		Axes: []motion.AxisConfig{
			{Name: "X", Enabled: true, Kind: motion.AxisKindLinear, MaxSpeed: &maxSpeed, MaxLimit: func() *float64 { v := 100.0; return &v }()},
		},
	}
	c := NewSimulatedMotionController(profile)
	c.Connect()
	err := c.MoveTo("X", 200)
	if err == nil {
		t.Fatal("expected error for position above MaxLimit")
	}
}

func TestSimulatedMotionControllerEmergencyStop(t *testing.T) {
	profile := createTestMotionProfile([]motion.AxisName{"X", "Y"})
	c := NewSimulatedMotionController(profile)
	c.Connect()
	c.Jog("X", 10)
	c.Jog("Y", 10)
	if err := c.EmergencyStop(); err != nil {
		t.Fatalf("EmergencyStop returned error: %v", err)
	}
	s := c.Status()
	for _, axis := range s.Axes {
		if axis.Moving {
			t.Fatalf("axis %s still moving after emergency stop", axis.Name)
		}
	}
	if !s.EmergencyStopped {
		t.Fatal("expected emergency stopped state")
	}
}

func TestSimulatedMotionControllerResetEmergencyStop(t *testing.T) {
	profile := createTestMotionProfile([]motion.AxisName{"X"})
	c := NewSimulatedMotionController(profile)
	c.Connect()
	c.EmergencyStop()

	if err := c.ResetEmergencyStop(); err != nil {
		t.Fatalf("ResetEmergencyStop returned error: %v", err)
	}

	s := c.Status()
	if s.EmergencyStopped {
		t.Fatal("expected emergency stop to be reset")
	}

	// 复位后应该可以正常运动
	if err := c.MoveTo("X", 10); err != nil {
		t.Fatalf("MoveTo after reset returned error: %v", err)
	}
}

func TestSimulatedMotionControllerNotConnected(t *testing.T) {
	profile := createTestMotionProfile([]motion.AxisName{"X"})
	c := NewSimulatedMotionController(profile)

	if err := c.MoveTo("X", 10); err == nil {
		t.Fatal("expected error when not connected")
	}
	if err := c.MoveBy("X", 10); err == nil {
		t.Fatal("expected error when not connected")
	}
	if err := c.Jog("X", 10); err == nil {
		t.Fatal("expected error when not connected")
	}
	if err := c.Home("X"); err == nil {
		t.Fatal("expected error when not connected")
	}
}

func TestSimulatedMotionControllerEmergencyStopBlocksMotion(t *testing.T) {
	profile := createTestMotionProfile([]motion.AxisName{"X"})
	c := NewSimulatedMotionController(profile)
	c.Connect()
	c.EmergencyStop()

	if err := c.MoveTo("X", 10); err == nil {
		t.Fatal("expected error when emergency stopped")
	}
	if err := c.MoveBy("X", 10); err == nil {
		t.Fatal("expected error when emergency stopped")
	}
	if err := c.Jog("X", 10); err == nil {
		t.Fatal("expected error when emergency stopped")
	}
}

func TestSimulatedMotionControllerDisconnect(t *testing.T) {
	profile := createTestMotionProfile([]motion.AxisName{"X"})
	c := NewSimulatedMotionController(profile)
	c.Connect()
	c.Disconnect()
	s := c.Status()
	if s.Connected {
		t.Fatal("expected disconnected")
	}
	if s.Axes[0].Moving {
		t.Fatal("expected axis to stop after disconnect")
	}
}

func TestSimulatedMotionControllerGetProfile(t *testing.T) {
	profile := createTestMotionProfile([]motion.AxisName{"X"})
	c := NewSimulatedMotionController(profile)
	p := c.GetProfile()
	if p.ID != "test-motion" {
		t.Fatalf("expected profile ID test-motion, got %s", p.ID)
	}
}

func TestSimulatedMotionControllerFollowError(t *testing.T) {
	profile := createTestMotionProfile([]motion.AxisName{"X"})
	c := NewSimulatedMotionController(profile)
	c.Connect()

	if err := c.MoveTo("X", 50); err != nil {
		t.Fatalf("MoveTo returned error: %v", err)
	}

	// 等待一小段时间，检查是否有跟随误差
	time.Sleep(100 * time.Millisecond)
	s := c.Status()
	if s.Axes[0].PositionError == 0 {
		t.Fatal("expected non-zero position error during motion")
	}

	// 等待运动完成，跟随误差应该清零
	waitFor(t, 2*time.Second, "MoveTo to complete", func() bool {
		return !c.Status().Axes[0].Moving
	})

	finalStatus := c.Status()
	if finalStatus.Axes[0].PositionError != 0 {
		t.Fatalf("expected position error to be 0 after motion, got %f", finalStatus.Axes[0].PositionError)
	}
}

func TestSimulatedMotionControllerMaxSpeed(t *testing.T) {
	maxSpeed := 5.0
	profile := motion.MotionControllerProfile{
		ID:   "test-max-speed",
		Name: "Test Max Speed",
		Type: motion.ControllerTypeSimulated,
		Axes: []motion.AxisConfig{
			{Name: "X", Enabled: true, Kind: motion.AxisKindLinear, MaxSpeed: &maxSpeed},
		},
	}
	c := NewSimulatedMotionController(profile)
	c.Connect()

	if err := c.MoveTo("X", 50); err != nil {
		t.Fatalf("MoveTo returned error: %v", err)
	}

	s := c.Status()
	if s.Axes[0].Velocity != maxSpeed {
		t.Fatalf("expected velocity %f, got %f", maxSpeed, s.Axes[0].Velocity)
	}
}

func TestSimulatedMotionControllerStopCancelsMovement(t *testing.T) {
	profile := createTestMotionProfile([]motion.AxisName{"X"})
	c := NewSimulatedMotionController(profile)
	c.Connect()

	if err := c.MoveTo("X", 100); err != nil {
		t.Fatalf("MoveTo returned error: %v", err)
	}

	// 等待运动开始
	time.Sleep(50 * time.Millisecond)
	s := c.Status()
	if !s.Axes[0].Moving {
		t.Fatal("expected axis to be moving")
	}
	currentPos := s.Axes[0].Position

	// 停止运动
	if err := c.Stop("X"); err != nil {
		t.Fatalf("Stop returned error: %v", err)
	}

	// 等待一下确保协程已退出
	time.Sleep(100 * time.Millisecond)

	// 位置应该保持在停止时的位置，不再变化
	s2 := c.Status()
	if s2.Axes[0].Moving {
		t.Fatal("expected axis to stop")
	}
	if s2.Axes[0].Position != currentPos {
		t.Fatalf("position changed after stop: was %f, now %f", currentPos, s2.Axes[0].Position)
	}
}

func TestSimulatedMotionControllerMoveToCancelsPrevious(t *testing.T) {
	profile := createTestMotionProfile([]motion.AxisName{"X"})
	c := NewSimulatedMotionController(profile)
	c.Connect()

	// 先移动到100
	c.MoveTo("X", 100)
	time.Sleep(50 * time.Millisecond)

	// 在运动过程中重新移动到50
	if err := c.MoveTo("X", 50); err != nil {
		t.Fatalf("MoveTo returned error: %v", err)
	}

	// 应该最终到达50
	waitFor(t, 2*time.Second, "MoveTo to reach position 50", func() bool {
		s := c.Status()
		return !s.Axes[0].Moving && s.Axes[0].Position == 50
	})
}

func TestSimulatedMotionControllerMoveToExactTarget(t *testing.T) {
	profile := createTestMotionProfile([]motion.AxisName{"X"})
	c := NewSimulatedMotionController(profile)
	c.Connect()

	// 测试运动完成后位置精确等于目标位置
	c.MoveTo("X", 37.5)
	waitFor(t, 2*time.Second, "MoveTo to reach position 37.5", func() bool {
		s := c.Status()
		return !s.Axes[0].Moving && s.Axes[0].Position == 37.5
	})

	c.MoveTo("X", -15.3)
	waitFor(t, 2*time.Second, "MoveTo to reach position -15.3", func() bool {
		s := c.Status()
		return !s.Axes[0].Moving && s.Axes[0].Position == -15.3
	})
}
