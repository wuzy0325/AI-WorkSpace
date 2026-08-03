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
		// Go 1.20 不支持 `for range N` 整数迭代（Go 1.22+ 语法），改写为经典三段式
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

// TestWTNMC4ADLLLatency 杩炴帴鐪熷疄鎺у埗鍣ㄥ苟娴嬮噺鍚?DLL 璋冪敤鑰楁椂銆?// 鐢ㄦ硶锛?//
//	set WTNMC4A_BENCH_IP=192.168.3.141
//	go test ./adapters/hardware/ -run TestWTNMC4ADLLLatency -v -timeout 120s
//
// 璇ユ祴璇曞彧鍦?WTNMC4A_BENCH_IP 鐜鍙橀噺璁剧疆鏃惰繍琛岋紝鍚﹀垯 skip銆?func TestWTNMC4ADLLLatency(t *testing.T) {
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

	// 鍗曟 DLL 璋冪敤鑰楁椂缁熻
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

	// 1. 鍗曡酱 readLP
	measure("readLP(axis0)", N, func() {
		procs.readLP.Call(handle, 0)
	})

	// 2. 鍗曡酱 getRR1Status
	// SDK 鍐欏叆 64 瀛楄妭 WTNMC4A_PARA_RR1 缁撴瀯浣擄紝蹇呴』鐢ㄥ尮閰嶇殑缂撳啿鍖?	var rr1Buf wtnmc4aRR1Struct
	measure("getRR1(axis0)", N, func() {
		procs.getRR1.Call(handle, 0, uintptr(unsafe.Pointer(&rr1Buf)))
	})

	// 3. 4 杞?readLP 涓茶
	measure("readLP x4 serial", N, func() {
		for ax := 0; ax < 4; ax++ {
			procs.readLP.Call(handle, uintptr(ax))
		}
	})

	// 4. 4 杞?readLP + getRR1 涓茶锛堝綋鍓?Status 瀹屾暣璺緞锛?	measure("readLP+getRR1 x4 serial (full Status)", N, func() {
		for ax := 0; ax < 4; ax++ {
			procs.readLP.Call(handle, uintptr(ax))
			procs.getRR1.Call(handle, uintptr(ax), uintptr(unsafe.Pointer(&rr1Buf)))
		}
	})

	// 5. 4 杞?readLP 骞跺彂锛坓oroutine锛?	measure("readLP x4 concurrent", N, func() {
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

	// 6. 瀹屾暣 Status锛堥€氳繃 Status 鏂规硶锛?	measure("Status() method", N, func() {
		_, _ = ctrl.Status(ctx)
	})

	// 7. 瀹屾暣 Status 骞跺彂锛? 涓苟鍙戣皟鐢級
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

	// 8. MoveTo 1mm 姝ラ暱鑰楁椂
	measure("MoveTo 1mm", 10, func() {
		_ = ctrl.MoveTo(ctx, core.AxisX, 1)
		// 绛夊緟杩愬姩瀹屾垚
		time.Sleep(200 * time.Millisecond)
		for {
			s, _ := ctrl.Status(ctx)
			if !s.Axes[0].Moving {
				break
			}
			time.Sleep(50 * time.Millisecond)
		}
	})

	// 9. 妯℃嫙鐐瑰姩 + 100ms 杞鐨勫畬鏁存椂搴忥紝璁板綍姣忔 Status 鐪嬪埌鐨勪綅缃?	fmt.Println("\n=== 鐐瑰姩 1mm + 100ms 杞鏃跺簭 ===")
	for trial := 0; trial < 3; trial++ {
		fmt.Printf("--- trial %d ---\n", trial)
		_ = ctrl.MoveTo(ctx, core.AxisX, 0) // 澶嶄綅
		time.Sleep(300 * time.Millisecond)

		startTime := time.Now()
		_ = ctrl.MoveTo(ctx, core.AxisX, 1) // 鐐瑰姩 1mm

		// 妯℃嫙鍓嶇 100ms 杞
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
