package usecase

import (
	"fmt"
	"sync"
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

// hubSubscriberCount 返回 hub 当前持有 deviceID 的订阅者数量。
// 测试与 hub 同包，直接读 shard 状态做确定性断言，替代 sleep 猜 goroutine 完成。
func hubSubscriberCount(hub *AcquisitionHub, deviceID string) int {
	shard := &hub.shards[shardIndexForDevice(deviceID)]
	shard.mu.RLock()
	defer shard.mu.RUnlock()
	return len(shard.subscribers[deviceID])
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
	// Unsubscribe 返回时已同步等待 worker 完成 hub 侧 unsub()（O-3），
	// 直接断言 hub 订阅已注销，无需 sleep 猜 goroutine 完成。
	if n := hubSubscriberCount(hub, "dev-1"); n != 0 {
		t.Fatalf("Unsubscribe 返回后 hub 仍持有 dev-1 的 %d 个订阅", n)
	}
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
	// defer Stop 保证 t.Fatalf 提前退出时也能清理 goroutine，避免泄漏
	defer relay.Stop()

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
	// Stop 返回时已同步等待所有 worker 完成 hub 侧 unsub()（O-3），直接断言。
	for _, id := range []string{"dev-1", "dev-2"} {
		if n := hubSubscriberCount(hub, id); n != 0 {
			t.Fatalf("Stop 返回后 hub 仍持有 %s 的 %d 个订阅", id, n)
		}
	}
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

// TestDataStreamRelayUnsubscribeWaitsForHubUnsub 验证 Unsubscribe 是确定性的：
// 返回时 hub 侧订阅已完成注销（不允许 sleep 猜 goroutine 完成）。
// 覆盖 O-3：Unsubscribe 同步等待 worker 完成 unsub()。
func TestDataStreamRelayUnsubscribeWaitsForHubUnsub(t *testing.T) {
	hub := NewAcquisitionHub(&capturePublisher{}, 100)
	relay := NewDataStreamRelay(hub)
	defer relay.Stop()

	relay.Subscribe("dev-1")
	if n := hubSubscriberCount(hub, "dev-1"); n != 1 {
		t.Fatalf("Subscribe 后期望 hub 持有 1 个订阅，实际 %d", n)
	}

	relay.Unsubscribe("dev-1")
	// Unsubscribe 返回即应完成 hub 侧注销
	if n := hubSubscriberCount(hub, "dev-1"); n != 0 {
		t.Fatalf("Unsubscribe 返回后 hub 仍持有 dev-1 的 %d 个订阅", n)
	}
}

// TestDataStreamRelayStopWaitsForAllHubUnsub 验证 Stop 是确定性的：
// 返回时所有设备的 hub 侧订阅均已完成注销。
// 覆盖 O-3：Stop 同步等待所有 worker 完成 unsub()。
func TestDataStreamRelayStopWaitsForAllHubUnsub(t *testing.T) {
	hub := NewAcquisitionHub(&capturePublisher{}, 100)
	relay := NewDataStreamRelay(hub)
	defer relay.Stop()

	relay.Subscribe("dev-1")
	relay.Subscribe("dev-2")

	relay.Stop()
	for _, id := range []string{"dev-1", "dev-2"} {
		if n := hubSubscriberCount(hub, id); n != 0 {
			t.Fatalf("Stop 返回后 hub 仍持有 %s 的 %d 个订阅", id, n)
		}
	}
}

// TestDataStreamRelayStopIdempotent 验证重复 Stop（含空 relay 上 Stop）安全：
// 不 panic、不死锁，且首次 Stop 的注销结果保持有效。
func TestDataStreamRelayStopIdempotent(t *testing.T) {
	hub := NewAcquisitionHub(&capturePublisher{}, 100)

	// 空 relay 上连续 Stop
	idle := NewDataStreamRelay(hub)
	idle.Stop()
	idle.Stop()

	relay := NewDataStreamRelay(hub)
	relay.Subscribe("dev-1")

	relay.Stop()
	relay.Stop()
	relay.Stop()

	if n := hubSubscriberCount(hub, "dev-1"); n != 0 {
		t.Fatalf("重复 Stop 后 hub 仍持有 dev-1 的 %d 个订阅", n)
	}
}

// TestDataStreamRelaySubscribeAfterStopRejected 验证 Stop 是终止性的：
// Stop 后 Subscribe 被拒绝（现有签名无 error 返回值，实现记录 warn 日志并跳过），
// hub 不会建立新订阅，也不会有数据帧被转发。
func TestDataStreamRelaySubscribeAfterStopRejected(t *testing.T) {
	hub := NewAcquisitionHub(&capturePublisher{}, 100)
	relay := NewDataStreamRelay(hub)

	relay.Stop()
	relay.Subscribe("dev-1")

	if n := hubSubscriberCount(hub, "dev-1"); n != 0 {
		t.Fatalf("Stop 后 Subscribe 应被拒绝，hub 却持有 %d 个订阅", n)
	}

	hub.OnData(device.DataPayload{
		DeviceID: "dev-1",
		Channels: []float64{1},
	})
	select {
	case payload := <-relay.Payloads():
		t.Fatalf("Stop 后订阅应被拒绝，不应收到数据帧，实际收到 %+v", payload)
	case <-time.After(100 * time.Millisecond):
		// 期望超时：订阅未建立
	}
}

// TestDataStreamRelayConcurrentSubscribeUnsubscribeStop 验证并发
// Subscribe/Unsubscribe/Stop 在 -race 下安全：无数据竞争、不死锁，
// 且全部结束后 hub 不残留任何订阅（Subscribe 与 Stop 竞争时被拒绝属预期）。
func TestDataStreamRelayConcurrentSubscribeUnsubscribeStop(t *testing.T) {
	hub := NewAcquisitionHub(&capturePublisher{}, 100)
	relay := NewDataStreamRelay(hub)

	const devices = 8
	const iterations = 25

	var wg sync.WaitGroup
	for i := 0; i < devices; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			id := fmt.Sprintf("dev-%d", i)
			for j := 0; j < iterations; j++ {
				relay.Subscribe(id)
				relay.Unsubscribe(id)
			}
		}(i)
	}
	// 与订阅循环并发地调用 Stop（含重复调用，验证并发幂等）
	wg.Add(1)
	go func() {
		defer wg.Done()
		relay.Stop()
		relay.Stop()
	}()
	wg.Wait()

	// 收尾幂等 Stop：无论上面竞争结果如何，返回后 relay 必须完全终止
	relay.Stop()
	for i := 0; i < devices; i++ {
		id := fmt.Sprintf("dev-%d", i)
		if n := hubSubscriberCount(hub, id); n != 0 {
			t.Fatalf("并发结束后 hub 仍持有 %s 的 %d 个订阅", id, n)
		}
	}
}

// TestDataStreamRelayStopWithFullPayloadChannel 验证 payloads 通道已满
// （下游 drain 停滞）时 Stop 不会永久阻塞：cancel 会解除 worker 的发送阻塞，
// Stop 在有界时间内返回且 hub 订阅完成注销。
func TestDataStreamRelayStopWithFullPayloadChannel(t *testing.T) {
	hub := NewAcquisitionHub(&capturePublisher{}, 100)
	relay := NewDataStreamRelay(hub)
	defer relay.Stop()

	relay.Subscribe("dev-1")

	// 直接填满 payloads 缓冲，模拟下游停滞、worker 阻塞在发送上
	for i := 0; i < cap(relay.payloads); i++ {
		relay.payloads <- device.DataPayload{DeviceID: "dev-1"}
	}
	// 再推一帧进 hub，让 worker 取到后阻塞在 full payloads 的发送 select 上
	hub.OnData(device.DataPayload{
		DeviceID: "dev-1",
		Channels: []float64{1},
	})

	stopped := make(chan struct{})
	go func() {
		relay.Stop()
		close(stopped)
	}()
	select {
	case <-stopped:
	case <-time.After(2 * time.Second):
		t.Fatal("payload 通道已满时 Stop 未在有界时间内完成（疑似永久阻塞）")
	}

	if n := hubSubscriberCount(hub, "dev-1"); n != 0 {
		t.Fatalf("Stop 返回后 hub 仍持有 dev-1 的 %d 个订阅", n)
	}
}

// TestDataStreamRelayStopWaitsForRetiringSubscriptions 验证 Stop 等待 retiring 集合
// 中的订阅完成 hub 侧 unsub()。回归保护：若 Stop 只看 r.subs 而忽略 retiring，
// Unsubscribe 锁外等 done 期间并发的 Stop 会在 worker 完成 unsub 之前返回，
// 违反"Stop 返回前所有 subscription 已注销"契约（review R-2 P1 缺陷）。
//
// 测试前置：Subscribe dev-1，手动构造 retiring 中间态（cancel + 移出 subs + 移入 retiring），
// 模拟 Unsubscribe 已释放锁、正在锁外等 sub.done 的瞬间。
// 测试步骤：调用 Stop()。
// 期待结果：Stop 返回后 hub 无 dev-1 订阅，retiring 集合为空（Stop 已消费并清空）。
func TestDataStreamRelayStopWaitsForRetiringSubscriptions(t *testing.T) {
	hub := NewAcquisitionHub(&capturePublisher{}, 100)
	relay := NewDataStreamRelay(hub)

	relay.Subscribe("dev-1")

	// 手动构造 Unsubscribe 的中间态：cancel + 移出 subs + 移入 retiring，但不等 done。
	// 这等价于 Unsubscribe 已执行完锁内部分、正要进入锁外 <-sub.done 时被调度走。
	relay.mu.Lock()
	sub := relay.subs["dev-1"]
	if sub == nil {
		relay.mu.Unlock()
		t.Fatal("Subscribe 后期望 r.subs[dev-1] 非空")
	}
	sub.cancel()
	delete(relay.subs, "dev-1")
	relay.retiring[sub] = struct{}{}
	relay.mu.Unlock()

	// 此时 worker 已被 cancel，正在执行 unsub() 路径。Stop 必须等其完成。
	relay.Stop()

	if n := hubSubscriberCount(hub, "dev-1"); n != 0 {
		t.Fatalf("Stop 返回后 hub 仍持有 dev-1 的 %d 个订阅（未等 retiring 完成）", n)
	}
	relay.mu.Lock()
	retiringCount := len(relay.retiring)
	relay.mu.Unlock()
	if retiringCount != 0 {
		t.Fatalf("Stop 返回后 retiring 仍持有 %d 个订阅（未清空）", retiringCount)
	}
}

// TestDataStreamRelayStopWithConcurrentUnsubscribe 验证 Unsubscribe 与 Stop 并发时
// Stop 返回前所有 subscription 已注销。回归保护：review R-2 P1 缺陷。
//
// 测试前置：Subscribe dev-1。
// 测试步骤：goroutine A 调用 Unsubscribe，主 goroutine 同时调用 Stop；用 race detector
// 重复运行让各种时序自然发生。
// 期待结果：Stop 返回后 hub 无 dev-1 订阅，Unsubscribe 也已返回（join A）。
func TestDataStreamRelayStopWithConcurrentUnsubscribe(t *testing.T) {
	hub := NewAcquisitionHub(&capturePublisher{}, 100)
	relay := NewDataStreamRelay(hub)

	relay.Subscribe("dev-1")

	unsubDone := make(chan struct{})
	go func() {
		defer close(unsubDone)
		relay.Unsubscribe("dev-1")
	}()

	relay.Stop()

	// Stop 返回后 hub 应无任何订阅。若 Stop 漏等 retiring 中的 sub，
	// worker 可能仍在执行 unsub，此处会捕获到残留订阅。
	if n := hubSubscriberCount(hub, "dev-1"); n != 0 {
		t.Fatalf("Stop 返回后 hub 仍持有 dev-1 的 %d 个订阅", n)
	}

	// Unsubscribe 应在 Stop 之后很快返回（Stop 等 sub.done 后 worker 完成 unsub，
	// done 关闭解除 Unsubscribe 的 <-sub.done 阻塞）。设 2s 超时兜底防死锁。
	select {
	case <-unsubDone:
	case <-time.After(2 * time.Second):
		t.Fatal("Stop 返回后 Unsubscribe 未在 2s 内返回（疑似死锁或漏等 retiring）")
	}
}
