package hardware

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"shared.local/device-sdk/go/motion/core"
)

// makeCompensationProfile 构造启用编码器补偿的轴配置。
// stepsPerRev=1.8°, microSteps=4, lead=4mm, encoderScale=0.01mm/count。
// 即 1mm 工程位置 = 1000 脉冲 = 100 编码器计数。
func makeCompensationProfile() core.MotionControllerProfile {
	enabled := true
	scale := 0.01
	return core.MotionControllerProfile{
		ID:      "b140-comp",
		Name:    "B140-Comp",
		Type:    core.ControllerTypeB140,
		Address: "127.0.0.1",
		Axes: []core.AxisConfig{
			{
				Name:           core.AxisX,
				Enabled:        true,
				Kind:           core.AxisKindLinear,
				StepsPerRev:    core.PtrFloat64(1.8),
				MicroSteps:     core.PtrInt(4),
				Lead:           core.PtrFloat64(4),
				MaxSpeed:       core.PtrFloat64(20),
				PositionSource: core.PositionSourceEncoder,
				EncoderScale:   &scale,
				EncoderCompensation: &core.AxisEncoderCompensationConfig{
					Enabled:   enabled,
					Tolerance: 0.01,
					MaxCycles: 3,
					SettleMs:  50, // 50ms 给 Status 轮询足够时间检测到 Compensating=true
					MinStep:   0.001,
					TimeoutMs: 500,
				},
			},
		},
	}
}

// newCompensationController 创建并连接一个补偿测试用 controller。
// 提供补偿测试默认需要的全部命令响应。
func newCompensationController(t *testing.T, server *b140FakeServer) *B140MotionController {
	t.Helper()
	profile := makeCompensationProfile()
	ctrl := NewB140MotionController(profile)
	ctrl.profile.Address = server.host
	ctrl.profile.Port = server.port
	if err := ctrl.Connect(context.Background()); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	return ctrl
}

// newCompensationControllerStuck 创建一个 SettleMs 很大的 controller。
// 用于取消类测试（场景 4/5/5b）：补偿激活后卡在 Settling 状态，
// 给测试足够时间发 Stop/EmergencyStop/新 MoveTo 验证取消行为。
func newCompensationControllerStuck(t *testing.T, server *b140FakeServer) *B140MotionController {
	t.Helper()
	profile := makeCompensationProfile()
	profile.Axes[0].EncoderCompensation.SettleMs = 10000 // 10s 卡住补偿循环
	ctrl := NewB140MotionController(profile)
	ctrl.profile.Address = server.host
	ctrl.profile.Port = server.port
	if err := ctrl.Connect(context.Background()); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	return ctrl
}

// runCompensationUntilState 调用 Status() 轮询直到断言函数返回 true 或超时。
// 补偿 goroutine 异步运行，需要给状态机时间走完循环。
func runCompensationUntilState(t *testing.T, ctrl *B140MotionController, assert func(core.ControllerStatus) bool, what string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		status, err := ctrl.Status(context.Background())
		if err != nil {
			t.Fatalf("Status error while waiting for %s: %v", what, err)
		}
		if assert(status) {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	status, _ := ctrl.Status(context.Background())
	t.Fatalf("timed out waiting for %s; final status: %+v", what, status)
}

// runCompensationToCompletion 两阶段等待：先等 pending 激活为 job，再等补偿结束（成功或失败）。
// 避免断言 `!Compensating` 在补偿尚未激活时误判为已结束。
func runCompensationToCompletion(t *testing.T, ctrl *B140MotionController, what string, timeout time.Duration) {
	t.Helper()
	// phase 1: 等激活（Compensating=true）
	runCompensationUntilState(t, ctrl, func(s core.ControllerStatus) bool {
		return axisBy(t, s, core.AxisX).Compensating
	}, "compensation activated for "+what, timeout)
	// phase 2: 等结束（Compensating=false）
	runCompensationUntilState(t, ctrl, func(s core.ControllerStatus) bool {
		return !axisBy(t, s, core.AxisX).Compensating
	}, "compensation done for "+what, timeout)
}

// axisBy 找 axis status，找不到 fatal。
func axisBy(t *testing.T, status core.ControllerStatus, name core.AxisName) core.AxisStatus {
	t.Helper()
	for _, a := range status.Axes {
		if a.Name == name {
			return a
		}
	}
	t.Fatalf("axis %s not found in status", name)
	return core.AxisStatus{}
}

// axisStatusOrDie 取一次 status，失败 fatal。
func axisStatusOrDie(t *testing.T, ctrl *B140MotionController) core.ControllerStatus {
	t.Helper()
	status, err := ctrl.Status(context.Background())
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	return status
}

// movingThenStoppedTS 返回一个动态 TS 响应函数：
// 第一次调用返回 moving=true（0x80），后续返回 moving=false。
// 用于让 pending 补偿正常激活：硬件必须先动起来，再停下来才能升级为 job。
func movingThenStoppedTS() func() string {
	var count int
	var mu sync.Mutex
	return func() string {
		mu.Lock()
		defer mu.Unlock()
		count++
		if count == 1 {
			return "128,0,0,0"
		}
		return "0,0,0,0"
	}
}

// 场景 1：误差在容差内不补偿，直接成功。
// TP 返回的目标编码器位置正好等于目标工程位置换算的编码器计数。
// profile: stepsPerRev=1.8, microSteps=4, lead=4, encoderScale=0.01
// → 1mm 工程位置 = 200 脉冲 = 100 编码器计数
func TestB140CompensationWithinToleranceSucceedsImmediately(t *testing.T) {
	server := newB140FakeServer(t, map[string]string{
		"SH":       "",
		"MTA=2":    "",
		"CEA=0":    "",
		"SPA=4000": "",
		"PAA=200":  "",
		"BGA":      "",
		// 1mm 目标 = 100 编码器计数
		"TPA":      "100",
		"TD":       "200,0,0,0",
		"MG _LFA":  "1.0000",
		"MG _LRA":  "1.0000",
		"DPA=200":  "",
	})
	// 让 hardware 经历 moving → stopped，使 pending 能正常激活
	server.setDynamic("TS", movingThenStoppedTS())
	defer server.close()

	ctrl := newCompensationController(t, server)
	if err := ctrl.MoveTo(context.Background(), core.AxisX, 1.0); err != nil {
		t.Fatalf("MoveTo: %v", err)
	}
	// 等补偿激活→结束。
	runCompensationToCompletion(t, ctrl, "within tolerance success", 2*time.Second)

	// 补偿成功后不应再下发 PR 命令
	commands := server.commands(0)
	for _, c := range commands {
		if strings.HasPrefix(c, "PRA=") {
			t.Fatalf("unexpected PR correction command: %s", c)
		}
	}
}

// 场景 2：单次补偿到位。
// 流程：
//   - checking 第一次 TP=0（误差 1mm）
//   - compensating 第二次 TP=0 → correctionPulse=200 → PR=200 → BG
//   - waitingStop → settling → checking 第三次 TP=100（到位）
//   - 期望共 1 个 PR 命令
func TestB140CompensationSingleCorrectionSucceeds(t *testing.T) {
	// TP 行为：补偿代码下发过 PR 后下一次 TP 返回 100（到位）。
	// 不能用固定次数切换，因为 Status 轮询也会读 TP，会快速消耗次数。
	var prCount int
	var prMu sync.Mutex
	server := newB140FakeServer(t, map[string]string{
		"SH":       "",
		"MTA=2":    "",
		"CEA=0":    "",
		"SPA=4000": "",
		"PAA=200":  "",
		"BGA":      "",
		"TD":       "200,0,0,0",
		"MG _LFA":  "1.0000",
		"MG _LRA":  "1.0000",
		"DPA=0":    "",
		"DPA=200":  "",
	})
	// 让 hardware 经历 moving → stopped
	server.setDynamic("TS", movingThenStoppedTS())
	// 接受任意 PRA 响应并计数：补偿下发 PR 后 TP 切换到 100
	server.setDynamic("PRA", func() string {
		prMu.Lock()
		defer prMu.Unlock()
		prCount++
		return ""
	})
	// 补偿前 TP=0（偏差 1mm），下发 PR 后 TP=100（到位）
	server.setDynamic("TPA", func() string {
		prMu.Lock()
		defer prMu.Unlock()
		if prCount > 0 {
			return "100"
		}
		return "0"
	})
	defer server.close()

	ctrl := newCompensationController(t, server)
	if err := ctrl.MoveTo(context.Background(), core.AxisX, 1.0); err != nil {
		t.Fatalf("MoveTo: %v", err)
	}
	runCompensationToCompletion(t, ctrl, "single correction success", 2*time.Second)

	commands := server.commands(0)
	prCountActual := 0
	for _, c := range commands {
		if strings.HasPrefix(c, "PRA=") {
			prCountActual++
		}
	}
	if prCountActual != 1 {
		t.Fatalf("expected 1 PR correction, got %d (commands: %v)", prCountActual, commands)
	}
}

// 场景 3：多次补偿失败，超过 maxCycles 报错。
// TP 始终返回 0（误差永远 1mm > tolerance），3 次补偿后应失败。
func TestB140CompensationExceedsMaxCyclesFails(t *testing.T) {
	server := newB140FakeServer(t, map[string]string{
		"SH":       "",
		"MTA=2":    "",
		"CEA=0":    "",
		"SPA=4000": "",
		"PAA=200":  "",
		"BGA":      "",
		"TD":       "200,0,0,0",
		"MG _LFA":  "1.0000",
		"MG _LRA":  "1.0000",
		"TPA":      "0", // 永远偏差 1mm
	})
	// 让 hardware 经历 moving → stopped
	server.setDynamic("TS", movingThenStoppedTS())
	// 接受任意 PR/DPA 命令
	server.setDynamic("PRA", func() string { return "" })
	defer server.close()

	ctrl := newCompensationController(t, server)
	if err := ctrl.MoveTo(context.Background(), core.AxisX, 1.0); err != nil {
		t.Fatalf("MoveTo: %v", err)
	}
	runCompensationToCompletion(t, ctrl, "max cycles failure", 2*time.Second)

	a := axisBy(t, axisStatusOrDie(t, ctrl), core.AxisX)
	if !strings.Contains(a.CompensationError, "max cycles") {
		t.Fatalf("expected max cycles error, got: %q", a.CompensationError)
	}
}

// 场景 4：新 moveTo 取消旧补偿任务。
// 流程：
//   - MoveTo #1 入队 pending1
//   - Status 第一次：TS moving=true → pending1.observedMotion=true
//   - Status 第二次：TS moving=false → 激活 job1（generation=1）
//   - 在 job1 跑 waitForAxisStop 时发 MoveTo #2 → cancelAxisCompensation：generation=2
func TestB140CompensationNewMoveToCancelsOld(t *testing.T) {
	server := newB140FakeServer(t, map[string]string{
		"SH":       "",
		"MTA=2":    "",
		"CEA=0":    "",
		"SPA=4000": "",
		"BGA":      "",
		"TD":       "0,0,0,0",
		"MG _LFA":  "1.0000",
		"MG _LRA":  "1.0000",
		"TPA":      "0",
	})
	// 让 hardware 经历 moving → stopped
	server.setDynamic("TS", movingThenStoppedTS())
	// 接受任意 PAA/PRA/DPA 命令
	server.setDynamic("PAA", func() string { return "" })
	server.setDynamic("PRA", func() string { return "" })
	server.setDynamic("DPA", func() string { return "" })
	defer server.close()

	ctrl := newCompensationControllerStuck(t, server)
	if err := ctrl.MoveTo(context.Background(), core.AxisX, 1.0); err != nil {
		t.Fatalf("MoveTo #1: %v", err)
	}
	// 激活第一次补偿（卡在 Settling 状态）
	runCompensationUntilState(t, ctrl, func(s core.ControllerStatus) bool {
		return axisBy(t, s, core.AxisX).Compensating
	}, "first compensation activated", 1*time.Second)

	// 记录第一个 generation
	ctrl.compMu.Lock()
	gen1 := ctrl.compensationGenerationCounter
	ctrl.compMu.Unlock()

	// 发新 MoveTo，应取消旧任务
	if err := ctrl.MoveTo(context.Background(), core.AxisX, 2.0); err != nil {
		t.Fatalf("MoveTo #2: %v", err)
	}

	// 第二次 MoveTo 应导致 generation 自增
	ctrl.compMu.Lock()
	gen2 := ctrl.compensationGenerationCounter
	ctrl.compMu.Unlock()
	if gen2 <= gen1 {
		t.Fatalf("generation should increase after new MoveTo: gen1=%d gen2=%d", gen1, gen2)
	}
}

// 场景 5：Stop 取消补偿任务。
func TestB140CompensationStopCancels(t *testing.T) {
	server := newB140FakeServer(t, map[string]string{
		"SH":       "",
		"MTA=2":    "",
		"CEA=0":    "",
		"SPA=4000": "",
		"PAA=200":  "",
		"BGA":      "",
		"TD":       "0,0,0,0",
		"MG _LFA":  "1.0000",
		"MG _LRA":  "1.0000",
		"TPA":      "0",
		"STA":      "",
	})
	// 让 hardware 经历 moving → stopped
	server.setDynamic("TS", movingThenStoppedTS())
	// 接受任意 PRA 命令
	server.setDynamic("PRA", func() string { return "" })
	defer server.close()

	ctrl := newCompensationControllerStuck(t, server)
	if err := ctrl.MoveTo(context.Background(), core.AxisX, 1.0); err != nil {
		t.Fatalf("MoveTo: %v", err)
	}
	runCompensationUntilState(t, ctrl, func(s core.ControllerStatus) bool {
		return axisBy(t, s, core.AxisX).Compensating
	}, "compensation activated", 1*time.Second)

	if err := ctrl.Stop(context.Background(), core.AxisX); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	time.Sleep(50 * time.Millisecond)
	ctrl.compMu.Lock()
	_, hasJob := ctrl.jobs[core.AxisX]
	ctrl.compMu.Unlock()
	if hasJob {
		t.Fatal("job should be cancelled after Stop")
	}
}

// 场景 5b：EmergencyStop 取消所有补偿任务。
func TestB140CompensationEmergencyStopCancelsAll(t *testing.T) {
	server := newB140FakeServer(t, map[string]string{
		"SH":       "",
		"MTA=2":    "",
		"CEA=0":    "",
		"SPA=4000": "",
		"PAA=200":  "",
		"BGA":      "",
		"TD":       "0,0,0,0",
		"MG _LFA":  "1.0000",
		"MG _LRA":  "1.0000",
		"TPA":      "0",
		"AB":       "",
	})
	// 让 hardware 经历 moving → stopped
	server.setDynamic("TS", movingThenStoppedTS())
	// 接受任意 PRA 命令
	server.setDynamic("PRA", func() string { return "" })
	defer server.close()

	ctrl := newCompensationControllerStuck(t, server)
	if err := ctrl.MoveTo(context.Background(), core.AxisX, 1.0); err != nil {
		t.Fatalf("MoveTo: %v", err)
	}
	runCompensationUntilState(t, ctrl, func(s core.ControllerStatus) bool {
		return axisBy(t, s, core.AxisX).Compensating
	}, "compensation activated", 1*time.Second)

	if err := ctrl.EmergencyStop(context.Background()); err != nil {
		t.Fatalf("EmergencyStop: %v", err)
	}
	time.Sleep(50 * time.Millisecond)
	ctrl.compMu.Lock()
	jobs := len(ctrl.jobs)
	ctrl.compMu.Unlock()
	if jobs != 0 {
		t.Fatalf("all jobs should be cancelled after EmergencyStop, got %d", jobs)
	}
}

// 场景 6：限位触发失败。
// MG _LFA 返回 0.0000（限位激活），补偿应失败。
func TestB140CompensationLimitTriggeredFails(t *testing.T) {
	server := newB140FakeServer(t, map[string]string{
		"SH":       "",
		"MTA=2":    "",
		"CEA=0":    "",
		"SPA=4000": "",
		"PAA=200":  "",
		"BGA":      "",
		"TD":       "0,0,0,0",
		"MG _LFA":  "0.0000", // 正向限位激活
		"MG _LRA":  "1.0000",
		"TPA":      "0",
	})
	// 让 hardware 经历 moving → stopped
	server.setDynamic("TS", movingThenStoppedTS())
	defer server.close()

	ctrl := newCompensationController(t, server)
	if err := ctrl.MoveTo(context.Background(), core.AxisX, 1.0); err != nil {
		t.Fatalf("MoveTo: %v", err)
	}
	runCompensationToCompletion(t, ctrl, "limit-triggered failure", 2*time.Second)

	a := axisBy(t, axisStatusOrDie(t, ctrl), core.AxisX)
	if !strings.Contains(a.CompensationError, "limit") {
		t.Fatalf("expected limit error, got: %q", a.CompensationError)
	}
}

// 场景 7：补偿中 moving=true，防止上层误判运动完成。
func TestB140CompensationKeepsAxisMovingWhileCompensating(t *testing.T) {
	// TP 返回使误差永远 > tolerance，让补偿一直在循环。
	server := newB140FakeServer(t, map[string]string{
		"SH":       "",
		"MTA=2":    "",
		"CEA=0":    "",
		"SPA=4000": "",
		"PAA=200":  "",
		"BGA":      "",
		"TD":       "0,0,0,0",
		"MG _LFA":  "1.0000",
		"MG _LRA":  "1.0000",
		"TPA":      "0", // 误差永远 1mm
	})
	// 让 hardware 经历 moving → stopped
	server.setDynamic("TS", movingThenStoppedTS())
	// 接受任意 PRA 命令
	server.setDynamic("PRA", func() string { return "" })
	defer server.close()

	ctrl := newCompensationControllerStuck(t, server)
	if err := ctrl.MoveTo(context.Background(), core.AxisX, 1.0); err != nil {
		t.Fatalf("MoveTo: %v", err)
	}
	runCompensationUntilState(t, ctrl, func(s core.ControllerStatus) bool {
		a := axisBy(t, s, core.AxisX)
		// 补偿中：Moving 应被强制为 true（硬件 TS=0 但补偿在跑）
		return a.Compensating && a.Moving
	}, "axis kept moving during compensation", 1*time.Second)
}

// 场景 8：编码器反馈无效报错。
// TP 返回非数字（模拟硬件故障），补偿应失败。
func TestB140CompensationInvalidEncoderFeedbackFails(t *testing.T) {
	server := newB140FakeServer(t, map[string]string{
		"SH":       "",
		"MTA=2":    "",
		"CEA=0":    "",
		"SPA=4000": "",
		"PAA=200":  "",
		"BGA":      "",
		"TD":       "0,0,0,0",
		"MG _LFA":  "1.0000",
		"MG _LRA":  "1.0000",
		"TPA":      "INVALID",
	})
	// 让 hardware 经历 moving → stopped
	server.setDynamic("TS", movingThenStoppedTS())
	defer server.close()

	ctrl := newCompensationController(t, server)
	if err := ctrl.MoveTo(context.Background(), core.AxisX, 1.0); err != nil {
		t.Fatalf("MoveTo: %v", err)
	}
	runCompensationToCompletion(t, ctrl, "invalid encoder feedback failure", 2*time.Second)

	a := axisBy(t, axisStatusOrDie(t, ctrl), core.AxisX)
	if !strings.Contains(a.CompensationError, "encoder") {
		t.Fatalf("expected encoder parse error, got: %q", a.CompensationError)
	}
}

// 场景 9：防静止轴误触发。
// MoveTo 后立即 Status 且硬件从未 moving，但仍在启动宽限期内 → 不应激活补偿。
// 超过宽限期后才丢弃 pending。
func TestB140CompensationDoesNotActivateForStaticAxisAfterGrace(t *testing.T) {
	server := newB140FakeServer(t, map[string]string{
		"SH":       "",
		"MTA=2":    "",
		"CEA=0":    "",
		"SPA=4000": "",
		"PAA=200":  "",
		"BGA":      "",
		"TS":       "0,0,0,0", // 从未 moving
		"TD":       "0,0,0,0",
		"MG _LFA":  "1.0000",
		"MG _LRA":  "1.0000",
		"TPA":      "0",
	})
	defer server.close()

	ctrl := newCompensationController(t, server)
	if err := ctrl.MoveTo(context.Background(), core.AxisX, 1.0); err != nil {
		t.Fatalf("MoveTo: %v", err)
	}
	// 立即 Status：仍在宽限期内，pending 应保留（不激活也不丢弃）
	status, _ := ctrl.Status(context.Background())
	a := axisBy(t, status, core.AxisX)
	if a.Compensating {
		t.Fatal("compensation should not be activated within startup grace for static axis")
	}
	ctrl.compMu.Lock()
	_, hasPending := ctrl.pendingRequests[core.AxisX]
	ctrl.compMu.Unlock()
	if !hasPending {
		t.Fatal("pending should be retained within startup grace")
	}

	// 等超过宽限期后再 Status，pending 应被丢弃
	time.Sleep(b140CompensationStartupGrace + 50*time.Millisecond)
	_, _ = ctrl.Status(context.Background())
	ctrl.compMu.Lock()
	_, hasPending = ctrl.pendingRequests[core.AxisX]
	_, hasJob := ctrl.jobs[core.AxisX]
	ctrl.compMu.Unlock()
	if hasPending || hasJob {
		t.Fatal("pending should be dropped after grace expires for static axis")
	}
}

// 场景 10：编码器源下 TP 读取失败时，Status 不得静默回退到寄存器位置。
// 寄存器与编码器在补偿启用时本就允许有偏差，静默替换会向操作员呈现失真位置。
// 期望：Status 整体不报错（TD/TS/限位仍可用），但 LastError 必须暴露编码器故障。
func TestB140StatusEncoderReadFailureSurfacesError(t *testing.T) {
	server := newB140FakeServer(t, map[string]string{
		"SH":      "",
		"MTA=2":   "",
		"CEA=0":   "",
		"TD":      "0,0,0,0",
		"TS":      "0,0,0,0",
		"MG _LFA": "1.0000",
		"MG _LRA": "1.0000",
		// TPA 故意不注册：fake server 对未知命令返回 "?"，触发 sendCommand 错误。
	})
	defer server.close()

	ctrl := newCompensationController(t, server)
	status, err := ctrl.Status(context.Background())
	if err != nil {
		t.Fatalf("Status should not fail overall when only TP fails: %v", err)
	}
	if !strings.Contains(status.LastError, "encoder read") {
		t.Fatalf("expected LastError to surface encoder read fault, got: %q", status.LastError)
	}
}
