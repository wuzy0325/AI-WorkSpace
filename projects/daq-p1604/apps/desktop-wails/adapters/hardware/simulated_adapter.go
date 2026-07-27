package hardware

import (
	"fmt"
	"math"
	"sync"
	"time"

	"daq-p1604/core"
	"daq-p1604/ports"
)

const (
	// simulatedDefaultPeriodMs 默认采样周期（毫秒）。1kHz = 1ms，100Hz = 10ms。
	simulatedDefaultPeriodMs = 100
	// simulatedMinPeriodMs 最小采样周期（1ms = 1kHz），用于性能验证
	simulatedMinPeriodMs = 1
	// simulatedChannelCount 模拟通道数（与 P1604 一致：16 压力 + 大气压 + 大温）
	simulatedChannelCount = 18
)

// SimulatedAdapter 模拟设备适配器（支持 1kHz 采样率）
type SimulatedAdapter struct {
	mu       sync.RWMutex
	status   map[string]*core.DeviceState
	sinks    map[string]func(core.PressureSnapshot)
	channels map[string]chan core.PressureSnapshot
	stopChs  map[string]chan struct{}
	logSink  func(DeviceLogEntry)
}

// NewSimulatedAdapter 创建模拟设备适配器
func NewSimulatedAdapter() *SimulatedAdapter {
	return &SimulatedAdapter{
		status:   make(map[string]*core.DeviceState),
		sinks:    make(map[string]func(core.PressureSnapshot)),
		channels: make(map[string]chan core.PressureSnapshot),
		stopChs:  make(map[string]chan struct{}),
	}
}

var _ ports.DevicePort = (*SimulatedAdapter)(nil)

// Connect 模拟连接
func (a *SimulatedAdapter) Connect(profile core.PressureProfile) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if _, exists := a.status[profile.ID]; exists {
		return fmt.Errorf("device %s already connected", profile.ID)
	}
	a.status[profile.ID] = &core.DeviceState{
		Profile:     profile,
		Status:      core.StatusConnected,
		StatusText:  core.StatusConnected.String(),
		ConnectedAt: core.TimestampMs(),
	}
	return nil
}

// Disconnect 模拟断开
func (a *SimulatedAdapter) Disconnect(id string) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.stopAcquisitionLocked(id)
	if st, exists := a.status[id]; exists {
		st.SetStatus(core.StatusDisconnected)
	}
	return nil
}

// StartAcquisition 模拟启动采集
// 采样周期由 profile.P1604Cfg.SamplingRate 决定（毫秒），最小 1ms（1kHz）。
func (a *SimulatedAdapter) StartAcquisition(id string) (<-chan core.PressureSnapshot, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if _, exists := a.status[id]; !exists {
		return nil, fmt.Errorf("device %s not connected", id)
	}
	if _, exists := a.channels[id]; exists {
		return nil, fmt.Errorf("device %s already acquiring", id)
	}

	// 解析采样周期
	periodMs := simulatedDefaultPeriodMs
	if st, exists := a.status[id]; exists {
		if st.Profile.P1604Cfg.SamplingRate > 0 {
			periodMs = st.Profile.P1604Cfg.SamplingRate
		}
	}
	if periodMs < simulatedMinPeriodMs {
		periodMs = simulatedMinPeriodMs
	}

	// 队列容量按采样率调整：1kHz 需要更大缓冲避免 readLoop 阻塞
	// 容量 = max(64, periodMs 对应的 8 秒积压)
	queueCap := 64
	if periodMs > 0 {
		needed := 8000 / periodMs // 8 秒积压
		if needed > queueCap {
			queueCap = needed
		}
	}

	ch := make(chan core.PressureSnapshot, queueCap)
	done := make(chan struct{})
	a.channels[id] = ch
	a.stopChs[id] = done
	if st, exists := a.status[id]; exists {
		st.SetStatus(core.StatusAcquiring)
		st.AcquiringAt = core.TimestampMs()
	}
	t0 := time.Now()
	go a.simulateLoop(id, ch, done, t0, time.Duration(periodMs)*time.Millisecond)
	return ch, nil
}

// simulateLoop 模拟数据生成循环（18 通道）
// 支持可配置采样周期（最小 1ms = 1kHz），用于性能验证。
// 注：Values 切片投递后所有权转移到 channel 接收方（recorder 异步消费），
// 因此不复用切片，由 GC 回收。真正的零分配优化在 recorder 内部（buf []byte 复用）。
//
// 单位同步：模拟模式下，模拟器即"硬件"，Unit 直接来自 profile.P1604Cfg.Unit，
// 模拟硬件 EU 系数已转换的语义——压力数值按用户选择的单位呈现，
// CH17 大气压力固定 Pa、CH18 大气温度固定 °C（这两个物理量不归压力 EU 系数管理）。
func (a *SimulatedAdapter) simulateLoop(id string, ch chan<- core.PressureSnapshot, done <-chan struct{}, t0 time.Time, period time.Duration) {
	ticker := time.NewTicker(period)
	defer ticker.Stop()
	for {
		select {
		case <-done:
			return
		case t := <-ticker.C:
			elapsed := t.Sub(t0).Seconds()
			// 每帧分配 18 元素切片（1kHz × 10 设备 = 1 万次/sec，约 1.4MB/sec，GC 可接受）
			values := make([]float64, simulatedChannelCount)
			// CH1-CH16: 压力值（模拟 0-100 psi 范围波动）
			for i := 0; i < 16; i++ {
				base := 50.0 + float64(i)*2.5
				values[i] = base + 10*math.Sin(elapsed*0.5+float64(i)*0.3) + 2*math.Sin(elapsed*3+float64(i)*0.7)
			}
			// CH17: 大气压力（约 101325 Pa）
			values[16] = 101325.0 + 50*math.Sin(elapsed*0.2)
			// CH18: 大气温度（约 25°C）
			values[17] = 25.0 + 2*math.Sin(elapsed*0.3)

			// 读取当前 profile 单位（模拟硬件 EU 系数已应用的语义）
			a.mu.RLock()
			unit := "psi"
			if st, ok := a.status[id]; ok && st.Profile.P1604Cfg.Unit != "" {
				unit = st.Profile.P1604Cfg.Unit
			}
			a.mu.RUnlock()

			snapshot := core.PressureSnapshot{
				DeviceID:  id,
				Timestamp: t.UnixMilli(),
				Values:    values,
				Unit:      unit,
			}

			// 非阻塞投递：先检查 done（退出优先），再尝试投递，最后队列满时丢弃。
			// 改为嵌套 select：避免 done 与 ch 同时就绪时 Go 随机选择导致退出前多写一条快照。
			select {
			case <-done:
				return
			default:
			}
			select {
			case ch <- snapshot:
			case <-done:
				return
			default:
				// 队列满：丢弃（采集 readLoop 不应阻塞 recorder）
				// 注：recorder 异步投递 + 32k 队列容量，正常场景下极少满
				continue
			}
		}
	}
}

func (a *SimulatedAdapter) stopAcquisitionLocked(id string) {
	if done, ok := a.stopChs[id]; ok {
		close(done)
		delete(a.stopChs, id)
	}
	delete(a.sinks, id)
	if ch, ok := a.channels[id]; ok {
		close(ch)
		delete(a.channels, id)
	}
	if st, exists := a.status[id]; exists {
		st.SetStatus(core.StatusConnected)
	}
}

// StopAcquisition 模拟停止采集
func (a *SimulatedAdapter) StopAcquisition(id string) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.stopAcquisitionLocked(id)
	return nil
}

// ZeroCalibration 模拟零点校准，采集状态保持不变
func (a *SimulatedAdapter) ZeroCalibration(id string) error {
	a.mu.RLock()
	defer a.mu.RUnlock()
	if _, exists := a.status[id]; !exists {
		return fmt.Errorf("device %s not connected", id)
	}
	return nil
}

// Status 获取模拟设备状态
func (a *SimulatedAdapter) Status(id string) (core.DeviceState, bool) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	st, ok := a.status[id]
	if !ok {
		return core.DeviceState{}, false
	}
	st.StatusText = st.Status.String()
	return *st, true
}

// ApplyConfig 应用模拟配置
func (a *SimulatedAdapter) ApplyConfig(id string, cfg core.P1604Config) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if st, exists := a.status[id]; exists {
		if cfg.SamplingRate < simulatedMinPeriodMs {
			cfg.SamplingRate = simulatedDefaultPeriodMs
		}
		if cfg.Unit == "" {
			cfg.Unit = "psi"
		}
		st.Profile.P1604Cfg = cfg
		return nil
	}
	return fmt.Errorf("device %s not connected", id)
}

// SetDataSink 设置数据回调
func (a *SimulatedAdapter) SetDataSink(id string, sink func(core.PressureSnapshot)) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.sinks[id] = sink
}

// SetLogSink 设置日志回调（模拟模式下也支持日志）
func (a *SimulatedAdapter) SetLogSink(sink func(DeviceLogEntry)) {
	a.mu.Lock()
	a.logSink = sink
	a.mu.Unlock()
}
