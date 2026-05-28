package hardware

import (
	"fmt"
	"testing"
	"time"

	"wind-daq/services/api-go/internal/core/motion"
)

func TestDebugMoveTo(t *testing.T) {
	profile := createTestMotionProfile([]motion.AxisName{"X"})
	c := NewSimulatedMotionController(profile)
	c.Connect()

	fmt.Printf("Before MoveTo: position=%.2f, moving=%v\n",
		c.Status().Axes[0].Position, c.Status().Axes[0].Moving)

	if err := c.MoveTo("X", 100); err != nil {
		t.Fatalf("MoveTo returned error: %v", err)
	}

	fmt.Printf("After MoveTo call: position=%.2f, moving=%v, velocity=%.2f\n",
		c.Status().Axes[0].Position, c.Status().Axes[0].Moving, c.Status().Axes[0].Velocity)

	// 等待并定期检查位置
	for i := 0; i < 20; i++ {
		time.Sleep(100 * time.Millisecond)
		s := c.Status()
		fmt.Printf("Tick %d: position=%.2f, moving=%v\n", i, s.Axes[0].Position, s.Axes[0].Moving)
		if s.Axes[0].Position == 100 && !s.Axes[0].Moving {
			fmt.Println("Reached target!")
			return
		}
	}

	t.Fatal("timed out waiting for MoveTo to reach position 100")
}
