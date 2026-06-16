package hardware

import (
	"fmt"
	"math"
	"sync"
	"time"

	"daq-p1604/core"
	"daq-p1604/ports"
)

// SimulatedAdapter 模拟设备适配器
type SimulatedAdapter struct {
	mu       sync.RWMutex
	status   map[string]*core.DeviceState
	sinks    map[string]func(core.PressureSnapshot)
	channels map[string]chan core.PressureSnapshot
	stopChs  map[string]chan struct{}
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
func (a *SimulatedAdapter) StartAcquisition(id string) (<-chan core.PressureSnapshot, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if _, exists := a.status[id]; !exists {
		return nil, fmt.Errorf("device %s not connected", id)
	}
	if _, exists := a.channels[id]; exists {
		return nil, fmt.Errorf("device %s already acquiring", id)
	}
	ch := make(chan core.PressureSnapshot, 64)
	done := make(chan struct{})
	a.channels[id] = ch
	a.stopChs[id] = done
	if st, exists := a.status[id]; exists {
		st.SetStatus(core.StatusAcquiring)
		st.AcquiringAt = core.TimestampMs()
	}
	t0 := time.Now()
	go a.simulateLoop(id, ch, done, t0)
	return ch, nil
}

// simulateLoop 模拟数据生成循环（18 通道）
func (a *SimulatedAdapter) simulateLoop(id string, ch chan<- core.PressureSnapshot, done <-chan struct{}, t0 time.Time) {
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-done:
			return
		case t := <-ticker.C:
			elapsed := t.Sub(t0).Seconds()
			values := make([]float64, 18)
			// CH1-CH16: 压力值（模拟 0-100 psi 范围波动）
			for i := 0; i < 16; i++ {
				base := 50.0 + float64(i)*2.5
				values[i] = base + 10*math.Sin(elapsed*0.5+float64(i)*0.3) + 2*math.Sin(elapsed*3+float64(i)*0.7)
			}
			// CH17: 大气压力（约 101325 Pa）
			values[16] = 101325.0 + 50*math.Sin(elapsed*0.2)
			// CH18: 大气温度（约 25°C）
			values[17] = 25.0 + 2*math.Sin(elapsed*0.3)
			ch <- core.PressureSnapshot{
				DeviceID:  id,
				Timestamp: t.UnixMilli(),
				Values:    values,
				Unit:      "psi",
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
		if cfg.SamplingRate < 10 {
			cfg.SamplingRate = 100
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
