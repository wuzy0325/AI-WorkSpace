package usecase

import (
	"testing"
	"time"

	"wind-daq/services/api-go/internal/core/device"
)

// drainPayloads 非阻塞地排空 relay.Payloads() 缓冲区中残留的数据帧。
// 在 Unsubscribe/Stop 后调用，确保后续的 "不应再收到数据" 断言不被残留帧污染。
func drainPayloads(ch <-chan device.DataPayload) {
	for {
		select {
		case <-ch:
		default:
			return
		}
	}
}

// TestDataStreamRelayForwardsPayloadsToChannel 验证 Subscribe 后，
// hub.OnData 触发的数据帧能异步转发到 relay.Payloads() 通道。
// 覆盖 TC-APP-13：relayStream 基本转发链路。
func TestDataStreamRelayForwardsPayloadsToChannel(t *testing.T) {
	// 使用 100Hz 节流频率，确保 OnData 后立即推送给订阅者
	hub := NewAcquisitionHub(&capturePublisher{}, 100)
	relay := NewDataStreamRelay(hub)
	defer relay.Stop()

	relay.Subscribe("dev-1")

	hub.OnData(device.DataPayload{
		DeviceID: "dev-1",
		Channels: []float64{42},
	})

	select {
	case payload := <-relay.Payloads():
		if payload.DeviceID != "dev-1" {
			t.Fatalf("期望 deviceID=dev-1，实际 %q", payload.DeviceID)
		}
		if len(payload.Channels) != 1 || payload.Channels[0] != 42 {
			t.Fatalf("期望 Channels=[42]，实际 %v", payload.Channels)
		}
	case <-time.After(300 * time.Millisecond):
		t.Fatal("等待 relay 转发数据帧超时")
	}
}

// TestDataStreamRelayUnsubscribeStopsForwarding 验证 Unsubscribe 后，
// hub.OnData 不再将数据帧转发到 relay.Payloads()。
// 覆盖 TC-APP-14：relayStream 取消订阅后停止转发。
func TestDataStreamRelayUnsubscribeStopsForwarding(t *testing.T) {
	hub := NewAcquisitionHub(&capturePublisher{}, 100)
	relay := NewDataStreamRelay(hub)
	defer relay.Stop()

	relay.Subscribe("dev-1")

	// 先触发一次数据帧，确认转发链路已打通
	hub.OnData(device.DataPayload{
		DeviceID: "dev-1",
		Channels: []float64{1},
	})
	select {
	case <-relay.Payloads():
	case <-time.After(300 * time.Millisecond):
		t.Fatal("取消订阅前未收到初始数据帧")
	}

	relay.Unsubscribe("dev-1")
	// 等待 relay goroutine 退出并完成 hub 侧 unsub()，确保 hub 不再持有订阅
	time.Sleep(50 * time.Millisecond)
	// 排空可能在取消前一刻写入缓冲区的残留帧
	drainPayloads(relay.Payloads())

	// 再次触发数据帧，不应再被转发
	hub.OnData(device.DataPayload{
		DeviceID: "dev-1",
		Channels: []float64{2},
	})

	select {
	case payload := <-relay.Payloads():
		t.Fatalf("取消订阅后不应再收到数据帧，实际收到 %+v", payload)
	case <-time.After(150 * time.Millisecond):
		// 期望超时：转发已停止
	}
}

// TestDataStreamRelayStopClosesAllSubscriptions 验证 Stop 后所有订阅停止，
// 不再转发任何设备的数据帧。
// 覆盖 TC-APP-15：relayStream 全局停止。
func TestDataStreamRelayStopClosesAllSubscriptions(t *testing.T) {
	hub := NewAcquisitionHub(&capturePublisher{}, 100)
	relay := NewDataStreamRelay(hub)

	relay.Subscribe("dev-1")
	relay.Subscribe("dev-2")

	// 确认两个设备都能转发
	hub.OnData(device.DataPayload{
		DeviceID: "dev-1",
		Channels: []float64{1},
	})
	hub.OnData(device.DataPayload{
		DeviceID: "dev-2",
		Channels: []float64{2},
	})

	received := 0
	deadline := time.After(300 * time.Millisecond)
	for received < 2 {
		select {
		case <-relay.Payloads():
			received++
		case <-deadline:
			t.Fatalf("期望收到 2 个数据帧，实际 %d", received)
		}
	}

	relay.Stop()
	// 等待所有 relay goroutine 退出并完成 hub 侧 unsub()
	time.Sleep(50 * time.Millisecond)
	// 排空可能在停止前一刻写入缓冲区的残留帧
	drainPayloads(relay.Payloads())

	// 再次触发两个设备的数据帧，不应再被转发
	hub.OnData(device.DataPayload{
		DeviceID: "dev-1",
		Channels: []float64{3},
	})
	hub.OnData(device.DataPayload{
		DeviceID: "dev-2",
		Channels: []float64{4},
	})

	select {
	case payload := <-relay.Payloads():
		t.Fatalf("Stop 后不应再收到数据帧，实际收到 %+v", payload)
	case <-time.After(150 * time.Millisecond):
		// 期望超时：所有转发已停止
	}
}

// TestDataStreamRelaySubscribeIdempotent 验证重复 Subscribe 同一设备不报错（幂等）。
// 第二次 Subscribe 应跳过并输出 warn 日志，不影响已有订阅。
func TestDataStreamRelaySubscribeIdempotent(t *testing.T) {
	hub := NewAcquisitionHub(&capturePublisher{}, 100)
	relay := NewDataStreamRelay(hub)
	defer relay.Stop()

	relay.Subscribe("dev-1")
	// 重复订阅同一设备，应幂等返回（仅输出 warn 日志）
	relay.Subscribe("dev-1")

	// 仍然能收到一份数据帧（不应因重复订阅而产生两份）
	hub.OnData(device.DataPayload{
		DeviceID: "dev-1",
		Channels: []float64{42},
	})

	select {
	case payload := <-relay.Payloads():
		if payload.DeviceID != "dev-1" {
			t.Fatalf("期望 deviceID=dev-1，实际 %q", payload.DeviceID)
		}
	case <-time.After(300 * time.Millisecond):
		t.Fatal("幂等订阅后等待数据帧超时")
	}

	// 确保不会收到第二份重复数据帧
	select {
	case payload := <-relay.Payloads():
		t.Fatalf("幂等订阅不应产生重复数据帧，实际收到 %+v", payload)
	case <-time.After(100 * time.Millisecond):
		// 期望超时：仅有一份数据帧
	}
}

// TestDataStreamRelayUnsubscribeNotSubscribedIsNoop 验证 Unsubscribe
// 未订阅的设备时不报错（no-op，仅输出 warn 日志）。
func TestDataStreamRelayUnsubscribeNotSubscribedIsNoop(t *testing.T) {
	hub := NewAcquisitionHub(&capturePublisher{}, 100)
	relay := NewDataStreamRelay(hub)
	defer relay.Stop()

	// 未订阅就 Unsubscribe，不应 panic 也不应报错
	relay.Unsubscribe("never-subscribed")

	// 订阅另一个设备仍能正常工作，验证 relay 状态未被破坏
	relay.Subscribe("dev-1")
	hub.OnData(device.DataPayload{
		DeviceID: "dev-1",
		Channels: []float64{42},
	})

	select {
	case payload := <-relay.Payloads():
		if payload.DeviceID != "dev-1" {
			t.Fatalf("期望 deviceID=dev-1，实际 %q", payload.DeviceID)
		}
	case <-time.After(300 * time.Millisecond):
		t.Fatal("no-op Unsubscribe 后等待数据帧超时")
	}
}
