package hardware

import (
	"fmt"
	"math"
	"math/rand"
	"sync"
	"time"

	"windlabx4/services/api-go/internal/core/device"
)

// simulatedChannelBufInitial 池中切片的初始容量。
// 取 32 是为了覆盖大多数 DAQ 设备的通道数（典型 8~16 通道），
// 同时避免占用过大内存；通道数超过此值的设备会在 emit 时按需扩容。
const simulatedChannelBufInitial = 32

// simulatedChannelBufMax pool 归还切片的容量上限。
// 超过此阈值的切片不归还（让 GC 回收），避免混用大/小通道设备时
// pool 积累过多大 cap 切片导致内存驻留膨胀。
// 取 128 = 4× 初始容量，覆盖偶发 100 通道设备仍保留复用收益。
const simulatedChannelBufMax = simulatedChannelBufInitial * 4

// putFloatSlice 归还 []float64 到 pool，超过容量阈值则丢弃。
// 阈值检查避免 pool 长期驻留过大切片（例如某次 100 通道设备触发的 100-cap 切片
// 被后续 8 通道设备 Get 后只用前 8 个元素，剩余 92 个 cap 永久驻留）。
func putFloatSlice(s []float64) {
	if cap(s) > simulatedChannelBufMax {
		return // 让 GC 回收，不归还
	}
	floatSlicePool.Put(s[:0])
}

// putIntSlice 归还 []int 到 pool，超过容量阈值则丢弃。语义同 putFloatSlice。
func putIntSlice(s []int) {
	if cap(s) > simulatedChannelBufMax {
		return
	}
	intSlicePool.Put(s[:0])
}

// floatSlicePool / intSlicePool 复用 emit() 中临时切片的底层容量，
// 减少 1kHz × 10 设备场景下每秒 2 万次小切片分配造成的 GC 压力。
// 安全前提：hub.OnData 在存储前已做防御性拷贝，sink 返回后即可归还。
var (
	floatSlicePool = sync.Pool{
		New: func() any { s := make([]float64, 0, simulatedChannelBufInitial); return s },
	}
	intSlicePool = sync.Pool{
		New: func() any { s := make([]int, 0, simulatedChannelBufInitial); return s },
	}
)

type SimulatedDevice struct {
	mu        sync.RWMutex
	profile   device.Profile
	status    device.Status
	sink      device.DataSink
	stop      chan struct{}
	acquiring bool
}

func NewSimulatedDevice(profile device.Profile) *SimulatedDevice {
	return &SimulatedDevice{
		profile: profile,
		status: device.Status{
			ID:         profile.ID,
			Name:       profile.Name,
			Type:       profile.Type,
			Connection: device.ConnectionDisconnected,
		},
	}
}

func (d *SimulatedDevice) ID() string { return d.profile.ID }

func (d *SimulatedDevice) Connect() error {
	d.mu.Lock()
	d.status.Connection = device.ConnectionConnected
	d.mu.Unlock()
	return nil
}

func (d *SimulatedDevice) Disconnect() error {
	_ = d.StopAcquisition()
	d.mu.Lock()
	d.status.Connection = device.ConnectionDisconnected
	d.mu.Unlock()
	return nil
}

func (d *SimulatedDevice) StartAcquisition() error {
	d.mu.Lock()
	if d.acquiring {
		d.mu.Unlock()
		return nil
	}
	d.acquiring = true
	d.status.Acquiring = true
	d.status.Connection = device.ConnectionAcquiring
	d.stop = make(chan struct{})
	stop := d.stop
	d.mu.Unlock()

	go d.loop(stop)
	return nil
}

func (d *SimulatedDevice) StopAcquisition() error {
	d.mu.Lock()
	if d.acquiring && d.stop != nil {
		close(d.stop)
	}
	d.acquiring = false
	d.stop = nil
	d.status.Acquiring = false
	if d.status.Connection == device.ConnectionAcquiring {
		d.status.Connection = device.ConnectionConnected
	}
	d.mu.Unlock()
	return nil
}

func (d *SimulatedDevice) SetDataSink(sink device.DataSink) {
	d.mu.Lock()
	d.sink = sink
	d.mu.Unlock()
}

func (d *SimulatedDevice) SetUnit(unit string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	for i := range d.profile.Channels {
		d.profile.Channels[i].Unit = unit
	}
	return nil
}

func (d *SimulatedDevice) SetTare(channelIndex int, offset float64) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if channelIndex < 0 || channelIndex >= len(d.profile.Channels) {
		return fmt.Errorf("invalid channel index: %d", channelIndex)
	}
	d.profile.Channels[channelIndex].TareOffset = offset
	return nil
}

func (d *SimulatedDevice) GetTare(channelIndex int) (float64, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	if channelIndex < 0 || channelIndex >= len(d.profile.Channels) {
		return 0, fmt.Errorf("invalid channel index: %d", channelIndex)
	}
	return d.profile.Channels[channelIndex].TareOffset, nil
}

func (d *SimulatedDevice) ClearTare(channelIndex int) error {
	return d.SetTare(channelIndex, 0)
}

func (d *SimulatedDevice) Status() device.Status {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.status
}

func (d *SimulatedDevice) loop(stop <-chan struct{}) {
	rate := d.profile.SamplingRate
	if rate <= 0 {
		rate = 20
	}
	interval := time.Second / time.Duration(rate)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	start := time.Now()
	for {
		select {
		case <-stop:
			return
		case now := <-ticker.C:
			d.emit(now.Sub(start).Seconds())
		}
	}
}

func pressureSim(t float64) float64 {
	// 返回大气压力值，单位 Pa（约 101325 Pa 标准大气压）
	return 101325 + 200*math.Sin(t*0.1) + (rand.Float64()-0.5)*20
}

func tempSim(t float64) float64 {
	return 22.5 + 0.3*math.Sin(t*0.05) + (rand.Float64()-0.5)*0.1
}

func (d *SimulatedDevice) emit(seconds float64) {
	d.mu.RLock()
	sink := d.sink
	channels := d.profile.Channels
	d.mu.RUnlock()
	if sink == nil {
		return
	}

	// 从 sync.Pool 获取切片，避免每帧 make([]float64, 0, N) 触发的小对象分配。
	// 若池中切片容量不足以容纳全部通道，则按需重新分配（旧切片由 GC 回收）。
	channelCount := len(channels)

	valuesBuf := floatSlicePool.Get().([]float64)
	if cap(valuesBuf) < channelCount {
		valuesBuf = make([]float64, 0, channelCount)
	}
	values := valuesBuf[:0]

	indicesBuf := intSlicePool.Get().([]int)
	if cap(indicesBuf) < channelCount {
		indicesBuf = make([]int, 0, channelCount)
	}
	indices := indicesBuf[:0]

	for _, channel := range channels {
		if !channel.Enabled {
			continue
		}
		indices = append(indices, channel.Index)
		var v float64
		switch channel.Index {
		case 16:
			v = pressureSim(seconds)
		case 17:
			v = tempSim(seconds)
		default:
			v = (rand.Float64()*2 - 1) * 10
		}
		values = append(values, v)
	}

	sink(device.DataPayload{
		DeviceID:       d.profile.ID,
		DeviceType:     d.profile.Type,
		DeviceName:     d.profile.Name,
		Timestamp:      device.NowMs(),
		Channels:       values,
		ChannelIndices: indices,
	})

	// sink 已返回（dataSink 闭包内已做防御性拷贝，hub 和 recorder 持有独立副本），
	// 可安全归还切片到 pool。
	// 清空 len 范围内的元素引用，避免敏感数据驻留（养成数据卫生习惯）。
	for i := range values {
		values[i] = 0
	}
	for i := range indices {
		indices[i] = 0
	}
	// 归还时通过 putXxxSlice 检查 cap 阈值，避免积累过大切片
	putFloatSlice(values)
	putIntSlice(indices)
}
