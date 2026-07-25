package hardware

import (
	"context"
	"math"
	"testing"
	"time"

	"shared.local/device-sdk/go/motion/core"
)

func TestSimulatedMotionController_LatestMoveOwnsAxisState(t *testing.T) {
	maxSpeed := 20.0
	controller := NewSimulatedMotionController(core.MotionControllerProfile{
		ID:   "sim-1",
		Name: "模拟控制器",
		Type: core.ControllerTypeSimulated,
		Axes: []core.AxisConfig{{
			Name:     core.AxisX,
			Enabled:  true,
			MaxSpeed: &maxSpeed,
		}},
	})
	ctx := context.Background()
	if err := controller.Connect(ctx); err != nil {
		t.Fatalf("连接模拟控制器失败: %v", err)
	}
	if err := controller.MoveTo(ctx, core.AxisX, 100); err != nil {
		t.Fatalf("发送第一次运动失败: %v", err)
	}
	time.Sleep(25 * time.Millisecond)
	if err := controller.MoveTo(ctx, core.AxisX, -20); err != nil {
		t.Fatalf("发送覆盖运动失败: %v", err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		status, err := controller.Status(ctx)
		if err != nil {
			t.Fatalf("读取模拟控制器状态失败: %v", err)
		}
		axis := status.Axes[0]
		if !axis.Moving {
			if math.Abs(axis.Position-(-20)) > 0.001 {
				t.Fatalf("旧运动协程提前清除了新命令状态: position=%v", axis.Position)
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("覆盖运动未在超时内完成")
}
