// csv_sink_bench_test.go CSV sink 的基准测试。
//
// 目标场景：10 台设备 × 1kHz 采样率 × 60 秒 ≈ 60 万帧 ≈ 480 万通道值。
// 通过 b.RunParallel 模拟多设备并发写入，验证：
//   - Write 投递到队列的延迟（应 < 10μs，队列非阻塞投递）
//   - 10 设备并发下的队列争用情况（应近线性扩展）
//   - 端到端吞吐：从 Start 到 Stop 完成所有 payload 落盘的总耗时
//
// 运行：go test -bench=BenchmarkCSVRecordingSink -benchmem -benchtime=2s ./internal/adapters/storage/...
package storage

import (
	"bufio"
	"fmt"
	"io"
	"sync"
	"testing"

	"wind-daq/services/api-go/internal/core/device"
	corestorage "wind-daq/services/api-go/internal/core/storage"
)

// makeBenchPayloads 构造 N 个设备的测试 payload，每台设备 8 个通道。
func makeBenchPayloads(n int) []device.DataPayload {
	payloads := make([]device.DataPayload, n)
	for i := range payloads {
		payloads[i] = device.DataPayload{
			DeviceID:       fmt.Sprintf("dev-%d", i),
			DeviceName:     fmt.Sprintf("bench-dev-%d", i),
			Timestamp:      int64(i),
			Channels:       []float64{1.0, -2.5, 3.14, 0.0, 5.5, -6.6, 7.7, 8.8},
			ChannelIndices: []int{0, 1, 2, 3, 4, 5, 6, 7},
		}
	}
	return payloads
}

// BenchmarkCSVRecordingSink_Write_SinglePayload 单帧 Write 延迟。
// 测量 channel 投递的开销（队列未满时为 select case + chan send）。
// 基准：i7-12700K 上预期 < 100ns/op。
func BenchmarkCSVRecordingSink_Write_SinglePayload(b *testing.B) {
	dir := b.TempDir()
	sink := NewCSVRecordingSink()
	if err := sink.Start(corestorage.RecordingConfig{OutputDir: dir, FilePrefix: "bench"}); err != nil {
		b.Fatalf("Start: %v", err)
	}
	b.Cleanup(func() { _ = sink.Stop() })

	payload := device.DataPayload{
		DeviceID:       "dev-1",
		Timestamp:      12345,
		Channels:       []float64{1.0, -2.5, 3.14, 0.0, 5.5, -6.6, 7.7, 8.8},
		ChannelIndices: []int{0, 1, 2, 3, 4, 5, 6, 7},
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = sink.Write(payload)
	}
	b.StopTimer()
}

// BenchmarkCSVRecordingSink_Write_10DevicesParallel 10 设备并发写入。
// 模拟 10 台 1kHz 设备同时调用 Write 的场景。
//
// 评估口径：b.N 次 Write 总耗时 / b.N = 单次 Write 延迟。
// 10 设备 × 1kHz = 1 万次/秒，需 < 100μs/op 才能跟上。
// 队列容量 8192，writer goroutine 持续消费，正常情况下不会丢包。
func BenchmarkCSVRecordingSink_Write_10DevicesParallel(b *testing.B) {
	dir := b.TempDir()
	sink := NewCSVRecordingSink()
	if err := sink.Start(corestorage.RecordingConfig{OutputDir: dir, FilePrefix: "bench"}); err != nil {
		b.Fatalf("Start: %v", err)
	}
	b.Cleanup(func() { _ = sink.Stop() })

	payloads := makeBenchPayloads(10)

	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			_ = sink.Write(payloads[i%10])
			i++
		}
	})
	b.StopTimer()
}

// BenchmarkCSVRecordingSink_EndToEnd_10Devices_60kFrames 端到端吞吐：
// 10 设备并发写入 6 万帧（≈ 10 × 1kHz × 6 秒），
// 然后调用 Stop 触发 drain + flush + sync，测量总耗时。
//
// 评估口径：总耗时 / 60000 = 单帧端到端成本（含队列、bufio、fsync 均摊）。
// 若 < 100μs，则 60 万帧（60 秒场景）可在 60 秒内完成落盘。
func BenchmarkCSVRecordingSink_EndToEnd_10Devices_60kFrames(b *testing.B) {
	const deviceCount = 10
	const framesPerDevice = 6000 // 总计 6 万帧，控制单次 bench 时长

	dir := b.TempDir()
	payloads := makeBenchPayloads(deviceCount)

	b.ReportAllocs()
	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		sink := NewCSVRecordingSink()
		if err := sink.Start(corestorage.RecordingConfig{
			OutputDir:  dir,
			FilePrefix: fmt.Sprintf("bench-%d", n),
		}); err != nil {
			b.Fatalf("Start: %v", err)
		}

		// 10 个 goroutine 模拟 10 台设备并发写入
		var wg sync.WaitGroup
		for d := 0; d < deviceCount; d++ {
			wg.Add(1)
			go func(deviceIdx int) {
				defer wg.Done()
				p := payloads[deviceIdx]
				for f := 0; f < framesPerDevice; f++ {
					p.Timestamp = int64(f)
					_ = sink.Write(p)
				}
			}(d)
		}
		wg.Wait()

		// 输出累计丢弃数供排查（正常情况下应为 0）
		if dropped := sink.DroppedCount(); dropped > 0 {
			b.Logf("EndToEnd bench dropped %d payloads (queue capacity may be insufficient)", dropped)
		}

		if err := sink.Stop(); err != nil {
			b.Fatalf("Stop: %v", err)
		}
	}
}

// BenchmarkCSVRecordingSink_WritePayload_FormatOnly 仅测量 writePayloadDynamicWide 的格式化开销，
// 不含 channel 投递、fsync、文件 I/O 等。用于隔离 strconv.AppendXxx 相对 fmt.Fprintf 的性能。
//
// 使用 bufio.Writer + io.Discard 作为输出，避免文件 I/O 噪声。
// 直接构造 perDeviceWriter 注入 discard bw，绕过文件创建路径。
func BenchmarkCSVRecordingSink_WritePayload_FormatOnly(b *testing.B) {
	sink := &CSVRecordingSink{}
	payload := device.DataPayload{
		DeviceID:       "dev-1",
		Timestamp:      1234567890,
		Channels:       []float64{1.0, -2.5, 3.14, 0.0, 5.5, -6.6, 7.7, 8.8},
		ChannelIndices: []int{0, 1, 2, 3, 4, 5, 6, 7},
	}

	bw := bufio.NewWriter(io.Discard)
	// 预置列布局并标记 headerWritten，跳过首帧表头写入路径，聚焦格式化开销
	w := &perDeviceWriter{
		deviceID:      "dev-1",
		isWideFormat:  false, // 动态宽格式路径
		bw:            bw,
		columnIndices: []int{0, 1, 2, 3, 4, 5, 6, 7},
		headerWritten: true,
	}
	var buf []byte

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		buf = buf[:0]
		if err := sink.writePayloadDynamicWide(&buf, w, payload); err != nil {
			b.Fatalf("writePayloadDynamicWide: %v", err)
		}
		// 每 1024 次 flush 一次，避免 bufio 内部缓冲无限增长
		if i%1024 == 0 {
			_ = bw.Flush()
		}
	}
	_ = bw.Flush()
}
