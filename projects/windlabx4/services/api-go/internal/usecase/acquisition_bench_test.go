// acquisition_bench_test.go 采集 hub 的基准测试。
//
// 目标场景：10 台设备 × 1kHz 采样率 × 60 秒 ≈ 60 万帧。
// 通过 b.RunParallel 模拟多设备并发写入，验证：
//   - 单帧 OnData 处理延迟（应 < 100μs，才能在 1 秒内消化 1 万帧）
//   - 10 设备并发下的分片锁扩展性（吞吐应近线性）
//   - 启用订阅者时的额外开销（订阅者 channel 投递）
//
// 运行：go test -bench=BenchmarkAcquisitionHub -benchmem -benchtime=2s ./internal/usecase/...
package usecase

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"

	"windlabx4/services/api-go/internal/core/device"
)

// benchPublisher 是 benchmark 专用的 noop 发布者，
// 不持有任何数据，避免在 b.N 较大时引入内存压力。
type benchPublisher struct{}

func (benchPublisher) Publish(string, any) {}

// makeBenchPayloads 构造 N 个设备的测试 payload。
// 每台设备 8 个通道（典型 DAQ-P-1604 配置），通道值覆盖正负数。
func makeBenchPayloads(n int) []device.DataPayload {
	payloads := make([]device.DataPayload, n)
	for i := range payloads {
		payloads[i] = device.DataPayload{
			DeviceID:       fmt.Sprintf("dev-%d", i),
			Timestamp:      int64(i),
			Channels:       []float64{1.0, -2.5, 3.14, 0.0, 5.5, -6.6, 7.7, 8.8},
			ChannelIndices: []int{0, 1, 2, 3, 4, 5, 6, 7},
		}
	}
	return payloads
}

// BenchmarkAcquisitionHub_OnData_SinglePayload 单设备单帧处理延迟。
// 通过 b.N 反复调用 OnData，测量分片锁 + history ring push 的开销。
// 基准：i7-12700K 上预期 < 1μs/op（含防御性拷贝）。
func BenchmarkAcquisitionHub_OnData_SinglePayload(b *testing.B) {
	hub := NewAcquisitionHub(benchPublisher{}, 20)
	payload := device.DataPayload{
		DeviceID:       "dev-1",
		Timestamp:      12345,
		Channels:       []float64{1.0, -2.5, 3.14, 0.0, 5.5, -6.6, 7.7, 8.8},
		ChannelIndices: []int{0, 1, 2, 3, 4, 5, 6, 7},
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		hub.OnData(payload)
	}
}

// BenchmarkAcquisitionHub_OnData_10DevicesParallel 模拟 10 台设备 1kHz 并发写入。
// 通过 b.RunParallel 让多个 goroutine 同时调用 OnData，
// 验证 16 分片锁在多核并发下的扩展性。
//
// 评估口径：b.N 次 OnData 总耗时 / b.N = 单帧延迟。
// 若 < 100μs，则 1 秒可处理 1 万帧（= 10 设备 × 1kHz）。
func BenchmarkAcquisitionHub_OnData_10DevicesParallel(b *testing.B) {
	hub := NewAcquisitionHub(benchPublisher{}, 20)
	payloads := makeBenchPayloads(10)

	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			hub.OnData(payloads[i%10])
			i++
		}
	})
}

// BenchmarkAcquisitionHub_OnData_WithSubscriber 启用订阅者时的处理延迟。
// 订阅者 channel 缓冲 1024，消费 goroutine 持续 drain，
// 模拟前端 500ms 轮询 + 实时订阅订阅者场景下的 hub 开销。
func BenchmarkAcquisitionHub_OnData_WithSubscriber(b *testing.B) {
	hub := NewAcquisitionHub(benchPublisher{}, 500)
	ch, unsubscribe := hub.Subscribe("dev-1", 1024)

	// 消费 goroutine：持续 drain 订阅 channel，避免缓冲满导致丢包
	var consumed atomic.Int64
	var consumeWg sync.WaitGroup
	consumeWg.Add(1)
	go func() {
		defer consumeWg.Done()
		for range ch {
			consumed.Add(1)
		}
	}()

	payload := device.DataPayload{
		DeviceID:       "dev-1",
		Timestamp:      12345,
		Channels:       []float64{1.0, -2.5, 3.14, 0.0, 5.5, -6.6, 7.7, 8.8},
		ChannelIndices: []int{0, 1, 2, 3, 4, 5, 6, 7},
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		hub.OnData(payload)
	}
	b.StopTimer()

	unsubscribe()
	consumeWg.Wait()
}

// BenchmarkAcquisitionHub_GetLatestData 并发读 latest 的开销。
// 模拟前端 500ms 轮询 + 多个 SSE 订阅者并发读取的场景。
func BenchmarkAcquisitionHub_GetLatestData(b *testing.B) {
	hub := NewAcquisitionHub(benchPublisher{}, 20)
	hub.OnData(device.DataPayload{
		DeviceID:       "dev-1",
		Timestamp:      12345,
		Channels:       []float64{1.0, 2.0, 3.0},
		ChannelIndices: []int{0, 1, 2},
	})

	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, _ = hub.GetLatestData("dev-1")
		}
	})
}

// BenchmarkAcquisitionHub_GetRecentData 并发读 history ring 的开销。
// 模拟前端拉取最近 N 帧数据的场景。
func BenchmarkAcquisitionHub_GetRecentData(b *testing.B) {
	hub := NewAcquisitionHubWithHistoryCapacity(benchPublisher{}, 20, 256)
	for i := 0; i < 256; i++ {
		hub.OnData(device.DataPayload{
			DeviceID:       "dev-1",
			Timestamp:      int64(i),
			Channels:       []float64{float64(i)},
			ChannelIndices: []int{0},
		})
	}

	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_ = hub.GetRecentData("dev-1", 32)
		}
	})
}
