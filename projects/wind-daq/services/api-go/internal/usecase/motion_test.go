package usecase

import (
	"testing"

	"wind-daq/services/api-go/internal/core/motion"
)

type fakeMotionController struct {
	status motion.ControllerStatus
}

func newFakeMotionController() *fakeMotionController {
	return &fakeMotionController{status: motion.ControllerStatus{
		ID:        "motion-1",
		Connected: false,
		Axes: []motion.AxisStatus{
			{Name: "X"},
		},
	}}
}

func (c *fakeMotionController) Connect() error {
	c.status.Connected = true
	return nil
}

func (c *fakeMotionController) Disconnect() error {
	c.status.Connected = false
	return nil
}

func (c *fakeMotionController) Status() motion.ControllerStatus { return c.status }

func (c *fakeMotionController) MoveTo(axis motion.AxisName, position float64) error {
	c.status.Axes[0].Position = position
	return nil
}

func (c *fakeMotionController) MoveBy(axis motion.AxisName, delta float64) error {
	c.status.Axes[0].Position += delta
	return nil
}

func (c *fakeMotionController) Jog(axis motion.AxisName, velocity float64) error {
	c.status.Axes[0].Moving = true
	c.status.Axes[0].Velocity = velocity
	return nil
}

func (c *fakeMotionController) Home(axis motion.AxisName) error {
	c.status.Axes[0].Position = 0
	c.status.Axes[0].Homed = true
	return nil
}

func (c *fakeMotionController) Stop(axis motion.AxisName) error {
	c.status.Axes[0].Moving = false
	c.status.Axes[0].Velocity = 0
	return nil
}

func (c *fakeMotionController) EmergencyStop() error {
	c.status.EmergencyStopped = true
	c.status.Axes[0].Moving = false
	c.status.Axes[0].Velocity = 0
	return nil
}

func TestMotionManagerCoordinatesControllerCommands(t *testing.T) {
	controller := newFakeMotionController()
	manager := NewMotionManager(controller)

	if err := manager.Connect(); err != nil {
		t.Fatalf("Connect returned error: %v", err)
	}
	if !manager.Status().Connected {
		t.Fatal("expected connected controller")
	}
	if err := manager.MoveTo("X", 10); err != nil {
		t.Fatalf("MoveTo returned error: %v", err)
	}
	if err := manager.MoveBy("X", -2.5); err != nil {
		t.Fatalf("MoveBy returned error: %v", err)
	}
	if got := manager.Status().Axes[0].Position; got != 7.5 {
		t.Fatalf("expected position 7.5, got %.2f", got)
	}
	if err := manager.Jog("X", 1.25); err != nil {
		t.Fatalf("Jog returned error: %v", err)
	}
	if !manager.Status().Axes[0].Moving {
		t.Fatal("expected jogging axis to be moving")
	}
	if err := manager.Stop("X"); err != nil {
		t.Fatalf("Stop returned error: %v", err)
	}
	if manager.Status().Axes[0].Moving {
		t.Fatal("expected stopped axis")
	}
	if err := manager.Home("X"); err != nil {
		t.Fatalf("Home returned error: %v", err)
	}
	if !manager.Status().Axes[0].Homed || manager.Status().Axes[0].Position != 0 {
		t.Fatalf("expected homed axis at zero, got %+v", manager.Status().Axes[0])
	}
}

func TestMotionManagerEmergencyStop(t *testing.T) {
	controller := newFakeMotionController()
	manager := NewMotionManager(controller)

	if err := manager.Connect(); err != nil {
		t.Fatalf("Connect returned error: %v", err)
	}
	if err := manager.Jog("X", 5); err != nil {
		t.Fatalf("Jog returned error: %v", err)
	}
	if err := manager.EmergencyStop(); err != nil {
		t.Fatalf("EmergencyStop returned error: %v", err)
	}
	status := manager.Status()
	if !status.EmergencyStopped {
		t.Fatal("expected emergency stopped status")
	}
	if status.Axes[0].Moving || status.Axes[0].Velocity != 0 {
		t.Fatalf("expected all motion stopped, got %+v", status.Axes[0])
	}
}
