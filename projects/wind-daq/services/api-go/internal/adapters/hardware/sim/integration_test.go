package sim

import (
	"fmt"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"wind-daq/services/api-go/internal/adapters/hardware"
	"wind-daq/services/api-go/internal/core/device"
)

// integration_test.go 端到端集成测试：真实 adapter 连接模拟器，验证 SCPI 命令
// 解析、数据帧接收、断连重连、错误回调等全链路行为。
//
// 这里 import 父包 hardware（adapters/hardware）以使用真实 DAQP1604 adapter。
// sim 是 hardware 的子包，import hardware 不构成循环（hardware 不 import sim）。
// 这是测试代码（_test.go），不影响生产装配（bootstrap.go 仍走生产 deviceFactory）。

// newQuietLogger 返回丢弃输出的 logger，避免测试日志噪声。
func newQuietLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// waitForFrames 轮询 frames 切片直到长度 >= min 或超时，返回是否满足。
func waitForFrames(mu *sync.Mutex, frames *[]device.DataPayload, min int, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		mu.Lock()
		n := len(*frames)
		mu.Unlock()
		if n >= min {
			return true
		}
		time.Sleep(10 * time.Millisecond)
	}
	return false
}

// TestIntegration_P1604AdapterVsSimulator 端到端主验证：
// 启动 P1604 模拟器，用真实 DAQP1604 adapter 连接，StartAcquisition，
// 断言收到有效数据帧（18 通道、值非零）。
//
// 全链路：adapter.sendCommand("c 01 1\r\n") → 模拟器 P1604Responder 识别
// StartStream → sendLoop 用 P1604BinaryFrameProducer 生成 77 字节二进制帧
// （2 字节长度前缀 + 5 头 + 18×float32 BE）→ adapter FrameReader.ReadFrame
// 读取 → ParseStreamFrame 解析 → sink 收到 DataPayload。
func TestIntegration_P1604AdapterVsSimulator(t *testing.T) {
	WithSimulatedDevice(t, device.DeviceDAQP1604, func(sim Simulator, profile device.Profile) {
		adapter := hardware.NewDAQP1604(profile)

		var mu sync.Mutex
		var frames []device.DataPayload
		adapter.SetDataSink(func(p device.DataPayload) {
			mu.Lock()
			frames = append(frames, p)
			mu.Unlock()
		})
		adapter.SetOnError(func(err error) {}) // 防御性：不期望错误

		if err := adapter.Connect(); err != nil {
			t.Fatalf("connect: %v", err)
		}
		defer adapter.Disconnect()

		if err := adapter.StartAcquisition(); err != nil {
			t.Fatalf("start acquisition: %v", err)
		}
		defer adapter.StopAcquisition()

		// 等待收到 3 帧（initStream 约 150ms + 首帧，3 秒足够）
		if !waitForFrames(&mu, &frames, 3, 3*time.Second) {
			t.Fatalf("no frames received within timeout (got %d)", len(frames))
		}

		mu.Lock()
		first := frames[0]
		mu.Unlock()

		if len(first.Channels) != 18 {
			t.Fatalf("channels=%d want 18", len(first.Channels))
		}
		for i, v := range first.Channels {
			if v == 0 {
				t.Fatalf("channel %d is zero", i)
			}
		}
		if first.DeviceID != profile.ID {
			t.Fatalf("deviceID=%q want %q", first.DeviceID, profile.ID)
		}
	})
}

// TestIntegration_P1604Adapter_InjectFrame 验证 InjectFrame 注入的帧能被真实 adapter 解析。
// 设大 latency 使 sendLoop 首帧后几乎停发，InjectFrame 注入一帧后 sink 帧数应增加。
func TestIntegration_P1604Adapter_InjectFrame(t *testing.T) {
	WithSimulatedDevice(t, device.DeviceDAQP1604, func(sim Simulator, profile device.Profile) {
		// 大延迟：sendLoop 发完首帧后长时间不再发，避免干扰 InjectFrame 断言
		sim.SetLatency(30 * time.Second)

		adapter := hardware.NewDAQP1604(profile)
		var mu sync.Mutex
		var frames []device.DataPayload
		adapter.SetDataSink(func(p device.DataPayload) {
			mu.Lock()
			frames = append(frames, p)
			mu.Unlock()
		})
		adapter.SetOnError(func(err error) {})

		if err := adapter.Connect(); err != nil {
			t.Fatalf("connect: %v", err)
		}
		defer adapter.Disconnect()

		if err := adapter.StartAcquisition(); err != nil {
			t.Fatalf("start: %v", err)
		}
		defer adapter.StopAcquisition()

		// 等 sendLoop 首帧（c 01 1 触发 StartStream）
		if !waitForFrames(&mu, &frames, 1, 3*time.Second) {
			t.Fatalf("no initial frame from sendLoop")
		}
		mu.Lock()
		n0 := len(frames)
		mu.Unlock()

		// 注入一帧（producer 生成完整线上字节）。
	// 用带时间戳的 producer 与 adapter 默认行为（useDeviceTs=true）对齐，
	// 否则 ParseStreamFrameEx 期望时间戳字段但帧内无该字段会解析失败。
	frame, err := P1604BinaryFrameProducerWithDeviceTimestamp(999, 18)
		if err != nil {
			t.Fatalf("producer: %v", err)
		}
		if err := sim.InjectFrame(frame); err != nil {
			t.Fatalf("inject: %v", err)
		}

		// 等待 adapter 收到注入帧
		if !waitForFrames(&mu, &frames, n0+1, 2*time.Second) {
			t.Fatalf("injected frame not received: %d -> %d", n0, len(frames))
		}
	})
}

// TestIntegration_P1604Adapter_DisconnectTriggersOnError 验证模拟器主动断开
// 客户端连接后，adapter readLoop 读取失败并触发 ErrorNotifiable 回调。
func TestIntegration_P1604Adapter_DisconnectTriggersOnError(t *testing.T) {
	WithSimulatedDevice(t, device.DeviceDAQP1604, func(sim Simulator, profile device.Profile) {
		adapter := hardware.NewDAQP1604(profile)
		adapter.SetDataSink(func(p device.DataPayload) {}) // 不关心数据

		var errMu sync.Mutex
		var gotErr error
		errCh := make(chan error, 1)
		adapter.SetOnError(func(err error) {
			errMu.Lock()
			gotErr = err
			errMu.Unlock()
			select {
			case errCh <- err:
			default:
			}
		})

		if err := adapter.Connect(); err != nil {
			t.Fatalf("connect: %v", err)
		}
		defer adapter.Disconnect()

		if err := adapter.StartAcquisition(); err != nil {
			t.Fatalf("start: %v", err)
		}

		// 确认连接已进入采集状态
		time.Sleep(300 * time.Millisecond)

		// 模拟设备掉线
		sim.DisconnectClient()

		select {
		case <-errCh:
			// onError 已触发
		case <-time.After(3 * time.Second):
			t.Fatal("onError not triggered after disconnect")
		}
		errMu.Lock()
		if gotErr == nil {
			t.Fatal("onError triggered with nil error")
		}
		errMu.Unlock()
	})
}

// TestIntegration_P1604Adapter_SetFailOnConnect 验证 SetFailOnConnect 端到端：
// 模拟器拒绝连接后，adapter 的 StartAcquisition 失败或 readLoop 触发 onError。
func TestIntegration_P1604Adapter_SetFailOnConnect(t *testing.T) {
	WithSimulatedDevice(t, device.DeviceDAQP1604, func(sim Simulator, profile device.Profile) {
		sim.SetFailOnConnect(true)

		adapter := hardware.NewDAQP1604(profile)
		adapter.SetDataSink(func(p device.DataPayload) {})
		errCh := make(chan error, 1)
		adapter.SetOnError(func(err error) {
			select {
			case errCh <- err:
			default:
			}
		})

		// Connect 可能成功（TCP 握手在 Accept 前完成），但连接会被模拟器立即关闭
		_ = adapter.Connect()
		defer adapter.Disconnect()

		// 等待模拟器 Accept 并关闭连接
		time.Sleep(150 * time.Millisecond)

		startErr := adapter.StartAcquisition()
		if startErr == nil {
			// StartAcquisition 成功（命令写入 buffer）时，readLoop 会因连接被关闭触发 onError
			select {
			case <-errCh:
				// OK：连接被拒，readLoop 退出并通知
			case <-time.After(3 * time.Second):
				t.Fatal("expected StartAcquisition failure or onError after SetFailOnConnect")
			}
		}
		// startErr != nil 也视为通过（initStream 因连接关闭而失败）
	})
}

// TestIntegration_P1604Adapter_MultiDeviceConcurrency 验证多设备并发：
// 多个 P1604 adapter 连接各自独立模拟器（不同端口），并行收帧互不干扰。
// 对应 DeviceManager per-id 串行、跨设备并发的模型。
func TestIntegration_P1604Adapter_MultiDeviceConcurrency(t *testing.T) {
	t.Parallel()
	const n = 2
	for i := 0; i < n; i++ {
		i := i
		t.Run(fmt.Sprintf("device-%d", i), func(t *testing.T) {
			t.Parallel()
			WithSimulatedDevice(t, device.DeviceDAQP1604, func(sim Simulator, profile device.Profile) {
				adapter := hardware.NewDAQP1604(profile)
				var mu sync.Mutex
				var frames []device.DataPayload
				adapter.SetDataSink(func(p device.DataPayload) {
					mu.Lock()
					frames = append(frames, p)
					mu.Unlock()
				})
				adapter.SetOnError(func(err error) {})
				if err := adapter.Connect(); err != nil {
					t.Fatalf("connect: %v", err)
				}
				defer adapter.Disconnect()
				if err := adapter.StartAcquisition(); err != nil {
					t.Fatalf("start: %v", err)
				}
				defer adapter.StopAcquisition()
				if !waitForFrames(&mu, &frames, 2, 3*time.Second) {
					t.Fatalf("no frames for device %d", i)
				}
			})
		})
	}
}

// 确保 slog 包被引用（newQuietLogger 用于需要日志的扩展场景）
var _ = newQuietLogger

// TestIntegration_P1604Adapter_SetUnitRoundTrip 验证单位切换端到端：
// 已连接时 SetUnit 通过 v01101 写入模拟器 EU 系数，模拟器保存后，
// 断连重连时 syncUnitFromHardware 通过 u01101 读回同一系数并反查单位。
// 这覆盖“硬件是单位转换唯一执行者”链路：SetUnit 写硬件 → 硬件持久 → 重连以硬件为准。
func TestIntegration_P1604Adapter_SetUnitRoundTrip(t *testing.T) {
	WithSimulatedDevice(t, device.DeviceDAQP1604, func(sim Simulator, profile device.Profile) {
		adapter := hardware.NewDAQP1604(profile)
		adapter.SetDataSink(func(p device.DataPayload) {})
		adapter.SetOnError(func(err error) {})

		if err := adapter.Connect(); err != nil {
			t.Fatalf("connect: %v", err)
		}

		// 已连接：SetUnit 应写硬件成功（v01101 -> 模拟器保存系数）
		if err := adapter.SetUnit("kPa"); err != nil {
			t.Fatalf("set unit kPa: %v", err)
		}

		// 断连（保留模拟器状态：模拟器进程未重启，coeff 仍为 kPa 系数）
		if err := adapter.Disconnect(); err != nil {
			t.Fatalf("disconnect: %v", err)
		}

		// 重连：Connect 内部 syncUnitFromHardware 应读回 kPa 系数且不报错
		if err := adapter.Connect(); err != nil {
			t.Fatalf("reconnect: %v", err)
		}
		defer adapter.Disconnect()

		// 未连接场景：先断开，再 SetUnit 应只更新 profile 不报错
		if err := adapter.Disconnect(); err != nil {
			t.Fatalf("disconnect before offline set: %v", err)
		}
		if err := adapter.SetUnit("MPa"); err != nil {
			t.Fatalf("offline set unit MPa: %v", err)
		}

		// 不支持的单位应报错
		if err := adapter.SetUnit("bogus"); err == nil {
			t.Fatal("expected error for unsupported unit, got nil")
		}
	})
}
