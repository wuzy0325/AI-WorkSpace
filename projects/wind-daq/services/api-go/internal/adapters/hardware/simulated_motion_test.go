package hardware

import (
	"testing"
	"wind-daq/services/api-go/internal/core/motion"
)

func TestSimulatedMotionControllerConnectAndStatus(t *testing.T) {
	c := NewSimulatedMotionController("test-motion", []motion.AxisName{"X", "Y", "Z"})
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
	c := NewSimulatedMotionController("test-motion", []motion.AxisName{"X"})
	c.Connect()
	if err := c.MoveTo("X", 100); err != nil {
		t.Fatalf("MoveTo returned error: %v", err)
	}
	s := c.Status()
	if s.Axes[0].Position != 100 {
		t.Fatalf("expected position 100, got %f", s.Axes[0].Position)
	}
}

func TestSimulatedMotionControllerMoveBy(t *testing.T) {
	c := NewSimulatedMotionController("test-motion", []motion.AxisName{"X"})
	c.Connect()
	c.MoveTo("X", 50)
	c.MoveBy("X", 25)
	s := c.Status()
	if s.Axes[0].Position != 75 {
		t.Fatalf("expected position 75 after moveBy, got %f", s.Axes[0].Position)
	}
}

func TestSimulatedMotionControllerJogAndStop(t *testing.T) {
	c := NewSimulatedMotionController("test-motion", []motion.AxisName{"Y"})
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
	c := NewSimulatedMotionController("test-motion", []motion.AxisName{"X"})
	c.Connect()
	c.MoveTo("X", 100)
	if err := c.Home("X"); err != nil {
		t.Fatalf("Home returned error: %v", err)
	}
	s := c.Status()
	if !s.Axes[0].Homed {
		t.Fatal("expected homed after home")
	}
	if s.Axes[0].Position != 0 {
		t.Fatalf("expected position 0 after home, got %f", s.Axes[0].Position)
	}
}

func TestSimulatedMotionControllerEmergencyStop(t *testing.T) {
	c := NewSimulatedMotionController("test-motion", []motion.AxisName{"X", "Y"})
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
	c := NewSimulatedMotionController("test-motion", []motion.AxisName{"X"})
	c.Connect()
	c.Disconnect()
	s := c.Status()
	if s.Connected {
		t.Fatal("expected disconnected")
	}
}
