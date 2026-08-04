//go:build windows

package hardware

import (
	"context"
	"fmt"
	"math"
	"os"
	"strconv"
	"sync"
	"testing"
	"time"
	"unsafe"

	"shared.local/device-sdk/go/motion/core"
)

// TestWTNMC4AReadOnlyStability performs only position/status reads. It never
// sends movement, stop, reset, homing, or register-write commands.
func TestWTNMC4AReadOnlyStability(t *testing.T) {
	ip := os.Getenv("WTNMC4A_READONLY_IP")
	if ip == "" {
		t.Skip("set WTNMC4A_READONLY_IP to run this read-only hardware test")
	}
	iterations := 2500 // Four axes per Status call = 10,000 position reads.
	if raw := os.Getenv("WTNMC4A_READONLY_ITERATIONS"); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value <= 0 {
			t.Fatalf("invalid WTNMC4A_READONLY_ITERATIONS %q", raw)
		}
		iterations = value
	}

	profile := wtnmc4aTestProfile()
	profile.Address = ip
	profile.Axes = []core.AxisConfig{
		{Name: core.AxisX, Enabled: true, Kind: core.AxisKindLinear, StepsPerRev: core.PtrFloat64(1.8), MicroSteps: core.PtrInt(4), Lead: core.PtrFloat64(4), MaxSpeed: core.PtrFloat64(10)},
		{Name: core.AxisY, Enabled: true, Kind: core.AxisKindLinear, StepsPerRev: core.PtrFloat64(1.8), MicroSteps: core.PtrInt(4), Lead: core.PtrFloat64(4), MaxSpeed: core.PtrFloat64(10)},
		{Name: core.AxisZ, Enabled: true, Kind: core.AxisKindLinear, StepsPerRev: core.PtrFloat64(1.8), MicroSteps: core.PtrInt(4), Lead: core.PtrFloat64(4), MaxSpeed: core.PtrFloat64(10)},
		{Name: core.AxisU, Enabled: true, Kind: core.AxisKindRotary, StepsPerRev: core.PtrFloat64(1.8), MicroSteps: core.PtrInt(4), GearRatio: core.PtrFloat64(1), MaxSpeed: core.PtrFloat64(10)},
	}

	ctrl := NewWTNMC4AMotionController(profile)
	ctx := context.Background()
	if err := ctrl.Connect(ctx); err != nil {
		t.Fatalf("connect %s failed: %v", ip, err)
	}
	defer ctrl.Disconnect(ctx)

	minDuration := time.Duration(math.MaxInt64)
	var maxDuration, totalDuration time.Duration
	for i := 0; i < iterations; i++ {
		started := time.Now()
		status, err := ctrl.Status(ctx)
		duration := time.Since(started)
		if err != nil {
			t.Fatalf("status iteration %d failed after %s: %v", i, duration, err)
		}
		if duration < minDuration {
			minDuration = duration
		}
		if duration > maxDuration {
			maxDuration = duration
		}
		totalDuration += duration
		for _, axis := range status.Axes {
			if math.IsNaN(axis.Position) || math.IsInf(axis.Position, 0) {
				t.Fatalf("iteration %d axis %s returned non-finite position %v", i, axis.Name, axis.Position)
			}
		}
	}

	t.Logf("read-only stability: statuses=%d position_reads=%d avg=%s min=%s max=%s",
		iterations, iterations*len(profile.Axes), totalDuration/time.Duration(iterations), minDuration, maxDuration)
}

// TestWTNMC4AReadOnlyConcurrentStatus verifies that overlapping status callers
// share one hardware query. It performs no movement or register writes.
func TestWTNMC4AReadOnlyConcurrentStatus(t *testing.T) {
	ip := os.Getenv("WTNMC4A_READONLY_IP")
	if ip == "" {
		t.Skip("set WTNMC4A_READONLY_IP to run this read-only hardware test")
	}

	profile := wtnmc4aTestProfile()
	profile.Address = ip
	ctrl := NewWTNMC4AMotionController(profile)
	ctx := context.Background()
	if err := ctrl.Connect(ctx); err != nil {
		t.Fatalf("connect %s failed: %v", ip, err)
	}
	defer ctrl.Disconnect(ctx)

	const (
		batches = 30
		callers = 3
	)
	var total, maximum time.Duration
	for batch := 0; batch < batches; batch++ {
		ready := make(chan struct{})
		errs := make(chan error, callers)
		var wg sync.WaitGroup
		started := time.Now()
		// lts/win7 go 1.20 不支持 Go 1.22 的 range over int 语法。
		for c := 0; c < callers; c++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-ready
				_, err := ctrl.Status(ctx)
				errs <- err
			}()
		}
		close(ready)
		wg.Wait()
		duration := time.Since(started)
		total += duration
		if duration > maximum {
			maximum = duration
		}
		close(errs)
		for err := range errs {
			if err != nil {
				t.Fatalf("batch %d status failed: %v", batch, err)
			}
		}
	}

	t.Logf("read-only concurrent status: batches=%d callers=%d avg_batch=%s max_batch=%s",
		batches, callers, total/batches, maximum)
}

// TestWTNMC4ADLLLatency 连接真实控制器并测量各 DLL 调用耗时。
// 用法：
//
//	set WTNMC4A_BENCH_IP=192.168.3.141
//	go test ./adapters/hardware/ -run TestWTNMC4ADLLLatency -v -timeout 120s
//
// 该测试只在 WTNMC4A_BENCH_IP 环境变量设置时运行，否则 skip。
func TestWTNMC4ADLLLatency(t *testing.T) {
	ip := os.Getenv("WTNMC4A_BENCH_IP")
	if ip == "" {
		t.Skip("set WTNMC4A_BENCH_IP to run this test")
	}

	profile := core.MotionControllerProfile{
		ID:      "bench",
		Name:    "bench",
		Type:    core.ControllerTypeWTNMC4A,
		Address: ip,
		Port:    5000,
		Axes: []core.AxisConfig{
			{Name: core.AxisX, Enabled: true, Kind: core.AxisKindLinear, StepsPerRev: core.PtrFloat64(1.8), MicroSteps: core.PtrInt(4), Lead: core.PtrFloat64(4), GearRatio: core.PtrFloat64(1), MaxSpeed: core.PtrFloat64(10), PositionSource: core.PositionSourceRegister, EncoderScale: core.PtrFloat64(0.005)},
			{Name: core.AxisY, Enabled: true, Kind: core.AxisKindLinear, StepsPerRev: core.PtrFloat64(1.8), MicroSteps: core.PtrInt(4), Lead: core.PtrFloat64(4), GearRatio: core.PtrFloat64(1), MaxSpeed: core.PtrFloat64(10), PositionSource: core.PositionSourceRegister, EncoderScale: core.PtrFloat64(0.005)},
			{Name: core.AxisZ, Enabled: true, Kind: core.AxisKindLinear, StepsPerRev: core.PtrFloat64(1.8), MicroSteps: core.PtrInt(4), Lead: core.PtrFloat64(4), GearRatio: core.PtrFloat64(1), MaxSpeed: core.PtrFloat64(10), PositionSource: core.PositionSourceRegister, EncoderScale: core.PtrFloat64(0.005)},
			{Name: core.AxisU, Enabled: true, Kind: core.AxisKindRotary, StepsPerRev: core.PtrFloat64(1.8), MicroSteps: core.PtrInt(4), Lead: core.PtrFloat64(4), GearRatio: core.PtrFloat64(1), MaxSpeed: core.PtrFloat64(10), PositionSource: core.PositionSourceRegister, EncoderScale: core.PtrFloat64(0.005)},
		},
	}

	ctrl := NewWTNMC4AMotionController(profile)
	ctx := context.Background()
	if err := ctrl.Connect(ctx); err != nil {
		t.Fatalf("connect %s failed: %v", ip, err)
	}
	defer ctrl.Disconnect(ctx)

	handle := ctrl.handle
	procs := ctrl.procs

	// 单次 DLL 调用耗时统计
	measure := func(name string, n int, fn func()) {
		// warmup
		fn()

		durations := make([]time.Duration, 0, n)
		total := time.Duration(0)
		for i := 0; i < n; i++ {
			start := time.Now()
			fn()
			d := time.Since(start)
			durations = append(durations, d)
			total += d
		}
		avg := total / time.Duration(n)
		minD, maxD := durations[0], durations[0]
		for _, d := range durations[1:] {
			if d < minD {
				minD = d
			}
			if d > maxD {
				maxD = d
			}
		}
		t.Logf("[%s] n=%d avg=%s min=%s max=%s total=%s",
			name, n, avg, minD, maxD, total)
	}

	const N = 30

	// 1. 单轴 readLP
	measure("readLP(axis0)", N, func() {
		procs.readLP.Call(handle, 0)
	})

	// 2. 单轴 getRR1Status
	// SDK 写入 64 字节 WTNMC4A_PARA_RR1 结构体，必须用匹配的缓冲区
	var rr1Buf wtnmc4aRR1Struct
	measure("getRR1(axis0)", N, func() {
		procs.getRR1.Call(handle, 0, uintptr(unsafe.Pointer(&rr1Buf)))
	})

	// 3. 4 轴 readLP 串行
	measure("readLP x4 serial", N, func() {
		for ax := 0; ax < 4; ax++ {
			procs.readLP.Call(handle, uintptr(ax))
		}
	})

	// 4. 4 轴 readLP + getRR1 串行（当前 Status 完整路径）
	measure("readLP+getRR1 x4 serial (full Status)", N, func() {
		for ax := 0; ax < 4; ax++ {
			procs.readLP.Call(handle, uintptr(ax))
			procs.getRR1.Call(handle, uintptr(ax), uintptr(unsafe.Pointer(&rr1Buf)))
		}
	})

	// 5. 4 轴 readLP 并发（goroutine）
	measure("readLP x4 concurrent", N, func() {
		var wg sync.WaitGroup
		for ax := 0; ax < 4; ax++ {
			wg.Add(1)
			go func(a int) {
				defer wg.Done()
				procs.readLP.Call(handle, uintptr(a))
			}(ax)
		}
		wg.Wait()
	})

	// 6. 完整 Status（通过 Status 方法）
	measure("Status() method", N, func() {
		_, _ = ctrl.Status(ctx)
	})

	// 7. 完整 Status 并发（2 个并发调用）
	measure("Status() x2 concurrent", N, func() {
		var wg sync.WaitGroup
		for i := 0; i < 2; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				_, _ = ctrl.Status(ctx)
			}()
		}
		wg.Wait()
	})

	// 8. MoveTo 1mm 步长耗时
	measure("MoveTo 1mm", 10, func() {
		_ = ctrl.MoveTo(ctx, core.AxisX, 1)
		// 等待运动完成
		time.Sleep(200 * time.Millisecond)
		for {
			s, _ := ctrl.Status(ctx)
			if !s.Axes[0].Moving {
				break
			}
			time.Sleep(50 * time.Millisecond)
		}
	})

	// 9. 模拟点动 + 100ms 轮询的完整时序，记录每次 Status 看到的位置
	fmt.Println("\n=== 点动 1mm + 100ms 轮询时序 ===")
	for trial := 0; trial < 3; trial++ {
		fmt.Printf("--- trial %d ---\n", trial)
		_ = ctrl.MoveTo(ctx, core.AxisX, 0) // 复位
		time.Sleep(300 * time.Millisecond)

		startTime := time.Now()
		_ = ctrl.MoveTo(ctx, core.AxisX, 1) // 点动 1mm

		// 模拟前端 100ms 轮询
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()
		positions := []struct {
			t  time.Duration
			p  float64
			ms int64
		}{}
		for {
			s, _ := ctrl.Status(ctx)
			elapsed := time.Since(startTime)
			pos := s.Axes[0].Position
			positions = append(positions, struct {
				t  time.Duration
				p  float64
				ms int64
			}{elapsed, pos, elapsed.Milliseconds()})
			fmt.Printf("  t=%4dms position=%.4f\n", elapsed.Milliseconds(), pos)
			if !s.Axes[0].Moving && elapsed > 200*time.Millisecond {
				break
			}
			<-ticker.C
		}
	}

	fmt.Println("=== benchmark done ===")
}
