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
}

func TestSimulatedMotionControllerGetProfile(t *testing.T) {
	profile := createTestMotionProfile([]motion.AxisName{"X"})
	c := NewSimulatedMotionController(profile)
	p := c.GetProfile()
	if p.ID != "test-motion" {
		t.Fatalf("expected profile ID test-motion, got %s", p.ID)
	}
}
