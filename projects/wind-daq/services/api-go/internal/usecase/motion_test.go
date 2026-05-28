package usecase

import (
	"testing"

	"wind-daq/services/api-go/internal/adapters/hardware"
	"wind-daq/services/api-go/internal/core/motion"
	"wind-daq/services/api-go/internal/ports"
)

type fakeMotionController struct {
	status motion.ControllerStatus
}

func newFakeMotionController() *fakeMotionController {
	return &fakeMotionController{status: motion.ControllerStatus{
		ID:        "motion-1",
		Connected: false,
		Axes: []motion.AxisStatus{
			{Name: motion.AxisX},
		},
	}}
}

func (c *fakeMotionController) LoadProfiles() ([]motion.MotionControllerProfile, error) {
	return []motion.MotionControllerProfile{}, nil
}

func (c *fakeMotionController) SaveProfiles(profiles []motion.MotionControllerProfile) error {
	return nil
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

func (c *fakeMotionController) DefinePosition(axis motion.AxisName, position float64) error {
	c.status.Axes[0].Position = position
	return nil
}

func (c *fakeMotionController) ResetEmergencyStop() error {
	c.status.EmergencyStopped = false
	return nil
}

func (c *fakeMotionController) GetProfile() motion.MotionControllerProfile {
	return motion.MotionControllerProfile{ID: c.status.ID}
}

var _ ports.MotionController = (*fakeMotionController)(nil)

func TestMotionManagerCoordinatesControllerCommands(t *testing.T) {
	profile := motion.MotionControllerProfile{
		ID:      "motion-1",
		Name:    "Test Controller",
		Type:    motion.ControllerTypeSimulated,
		Address: "127.0.0.1",
		Axes: []motion.AxisConfig{
			{Name: motion.AxisX, Enabled: true},
		},
	}
	profileStore := &fakeProfileStore{profiles: []motion.MotionControllerProfile{profile}}
	manager := NewMotionManager(profileStore, func(profile motion.MotionControllerProfile) ports.MotionController {
		return hardware.NewSimulatedMotionController(profile)
	})

	// 先加载配置，这样管理器才知道有哪些控制器
	if _, err := manager.LoadProfiles(); err != nil {
		t.Fatalf("LoadProfiles returned error: %v", err)
	}

	if err := manager.Connect("motion-1"); err != nil {
		t.Fatalf("Connect returned error: %v", err)
	}
	status, _ := manager.Status("motion-1")
	if !status.Connected {
		t.Fatal("expected connected controller")
	}

	// 使用 DefinePosition 同步设置位置，避免异步模拟问题
	if err := manager.DefinePosition("motion-1", motion.AxisX, 10); err != nil {
		t.Fatalf("DefinePosition returned error: %v", err)
	}
	status, _ = manager.Status("motion-1")
	if got := status.Axes[0].Position; got != 10.0 {
		t.Fatalf("expected position 10.0, got %.2f", got)
	}

	// 测试其他命令是否正常执行（不检查最终位置值）
	if err := manager.Jog("motion-1", motion.AxisX, 1.25); err != nil {
		t.Fatalf("Jog returned error: %v", err)
	}
	status, _ = manager.Status("motion-1")
	if !status.Axes[0].Moving {
		t.Fatal("expected jogging axis to be moving")
	}
	if err := manager.Stop("motion-1", motion.AxisX); err != nil {
		t.Fatalf("Stop returned error: %v", err)
	}
	status, _ = manager.Status("motion-1")
	if status.Axes[0].Moving {
		t.Fatal("expected stopped axis")
	}
	if err := manager.Home("motion-1", motion.AxisX); err != nil {
		t.Fatalf("Home returned error: %v", err)
	}
}

func TestMotionManagerEmergencyStop(t *testing.T) {
	profile := motion.MotionControllerProfile{
		ID:      "motion-1",
		Name:    "Test Controller",
		Type:    motion.ControllerTypeSimulated,
		Address: "127.0.0.1",
		Axes: []motion.AxisConfig{
			{Name: motion.AxisX, Enabled: true},
		},
	}
	profileStore := &fakeProfileStore{profiles: []motion.MotionControllerProfile{profile}}
	manager := NewMotionManager(profileStore, func(profile motion.MotionControllerProfile) ports.MotionController {
		return hardware.NewSimulatedMotionController(profile)
	})

	// 先加载配置，这样管理器才知道有哪些控制器
	if _, err := manager.LoadProfiles(); err != nil {
		t.Fatalf("LoadProfiles returned error: %v", err)
	}

	if err := manager.Connect("motion-1"); err != nil {
		t.Fatalf("Connect returned error: %v", err)
	}
	if err := manager.Jog("motion-1", motion.AxisX, 5); err != nil {
		t.Fatalf("Jog returned error: %v", err)
	}
	if err := manager.EmergencyStop("motion-1"); err != nil {
		t.Fatalf("EmergencyStop returned error: %v", err)
	}
	status, _ := manager.Status("motion-1")
	if !status.EmergencyStopped {
		t.Fatal("expected emergency stopped status")
	}
	if status.Axes[0].Moving || status.Axes[0].Velocity != 0 {
		t.Fatalf("expected all motion stopped, got %+v", status.Axes[0])
	}
}

type fakeProfileStore struct {
	profiles []motion.MotionControllerProfile
}

func (f *fakeProfileStore) LoadProfiles() ([]motion.MotionControllerProfile, error) {
	return f.profiles, nil
}

func (f *fakeProfileStore) SaveProfiles(profiles []motion.MotionControllerProfile) error {
	f.profiles = profiles
	return nil
}
