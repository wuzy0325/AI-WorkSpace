// repro_panic_test.go 用于复现 /api/daq/latest/ handler 中
// "reflect: call of reflect.Value.Int on string Value" panic。
//
// 复现思路：
//   - 启动 SimulatedDevice（使用 sync.Pool 复用底层切片）
//   - 通过 NewDataSink 接入 AcquisitionHub（dataSink 内部做防御性拷贝）
//   - 并发执行 JSON 编码（模拟 HTTP handler writeJSON）
//   - 使用 -race 检测数据竞争
package usecase

import (
	"encoding/json"
	"runtime"
	"sync"
	"testing"
	"time"

	"wind-daq/services/api-go/internal/core/device"
)

func TestReproReflectIntOnStringPanic(t *testing.T) {
	// 该测试需要在 adapters/hardware 包之外复现 JSON 编码 panic。
	// 由于 dataSink 已做防御性拷贝，理论上 hub 中的 payload 应独立于生产者切片。
	// 这里通过反复写入 + 并发 JSON 编码验证是否存在数据竞争导致字段错位。
	hub := NewAcquisitionHub(nil, 200)
	uuid := "de9200b5-330d-4151-8bc9-0095e4c62ef6"

	stop := make(chan struct{})
	var wg sync.WaitGroup

	// 生产者：模拟设备高频写入（直接调 hub.OnData，跳过 dataSink 防御性拷贝，
	// 看是否能复现 panic；若不能，则问题在 dataSink 之后）
	wg.Add(1)
	go func() {
		defer wg.Done()
		i := int64(0)
		for {
			select {
			case <-stop:
				return
			default:
				hub.OnData(device.DataPayload{
					DeviceID:        uuid,
					Timestamp:       i,
					DeviceTimestamp: i,
					Channels:        []float64{float64(i), float64(i + 1)},
					ChannelIndices:  []int{0, 1},
				})
				i++
			}
		}
	}()

	// 消费者：并发读取并 JSON 编码
	for w := 0; w < 4; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
					payload, ok := hub.GetLatestData(uuid)
					if !ok {
						// 暂无数据：让出 CPU 避免与生产者争用，减少 4 个消费者自旋导致的 GOMAXPROCS 抢占。
						runtime.Gosched()
						continue
					}
					if _, err := json.Marshal(payload); err != nil {
						t.Errorf("json.Marshal failed: %v", err)
						return
					}
				}
			}
		}()
	}

	time.Sleep(200 * time.Millisecond)
	close(stop)
	wg.Wait()
}
