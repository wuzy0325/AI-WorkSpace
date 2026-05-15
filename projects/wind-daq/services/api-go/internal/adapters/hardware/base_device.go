package hardware

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"sync"
	"time"

	"wind-daq/services/api-go/internal/core/device"
)

// ==================== 设备驱动基类 ====================
// 提供所有设备驱动的公共功能:
// - 状态管理(连接/采集/错误)
// - 数据回调(Sink)
// - 自动重连(指数退避策略)

// BaseDevice 设备驱动基类
// 封装设备公共逻辑,各具体驱动嵌入此基类
type BaseDevice struct {
	mu     sync.RWMutex
	config device.DeviceConfig // 设备配置
	status device.DeviceStatus // 设备状态
	sink   device.DataSink     // 数据回调
	cancel context.CancelFunc  // 采集取消函数
}

// 重连配置常量
const (
	maxReconnectAttempts = 5     // 最大重连次数
	reconnectBaseDelayMs = 1000  // 基础重连延迟(毫秒)
	reconnectMaxDelayMs  = 30000 // 最大重连延迟(毫秒)
)

// NewBaseDevice 构建设备基类
// 参数: config 设备配置
// 返回: *BaseDevice 设备基类实例
func NewBaseDevice(config device.DeviceConfig) *BaseDevice {
	return &BaseDevice{
		config: config,
		status: device.DeviceStatus{
			ID:           config.ID,
			Name:         config.Name,
			Type:         config.Type,
			Connection:   device.ConnectionDisconnected,
			Acquiring:    false,
			SamplingRate: config.SamplingRate,
		},
	}
}

// ID 获取设备ID
func (b *BaseDevice) ID() string { return b.config.ID }

// Config 获取设备配置
func (b *BaseDevice) Config() device.DeviceConfig { return b.config }

// Status 获取设备状态
func (b *BaseDevice) Status() device.DeviceStatus {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.status
}

// SetDataSink 设置数据回调
func (b *BaseDevice) SetDataSink(sink device.DataSink) {
	b.mu.Lock()
	b.sink = sink
	b.mu.Unlock()
}

// GetDataSink 获取数据回调
func (b *BaseDevice) GetDataSink() device.DataSink {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.sink
}

// setState 设置连接状态
// 参数: state 新的设备状态
func (b *BaseDevice) setState(state device.DeviceState) {
	b.mu.Lock()
	b.status.Connection = device.StateToConnection(state)
	// 如果不是采集中状态,则清除采集中标志
	if state != device.StateAcquiring {
		b.status.Acquiring = false
	}
	b.mu.Unlock()
}

// setAcquiring 设置采集中状态
func (b *BaseDevice) setAcquiring(acquiring bool) {
	b.mu.Lock()
	b.status.Acquiring = acquiring
	b.mu.Unlock()
}

// setError 设置错误状态
// 参数: err 错误信息字符串
func (b *BaseDevice) setError(err string) {
	b.mu.Lock()
	b.status.Connection = device.ConnectionError
	b.status.LastError = err
	b.mu.Unlock()
}

// EmitData 发送采集数据
// 调用注册的回调函数推送数据
// 参数: payload 采集数据包
func (b *BaseDevice) EmitData(payload device.DataPayload) {
	if sink := b.GetDataSink(); sink != nil {
		sink(payload)
	}
}

// ReconnectWithBackoff 带指数退避的重连
// 每次重连失败后延迟时间翻倍,最大30秒
// 参数: ctx 上下文(用于超时取消)
// 参数: connectFn 连接函数
// 返回: error 错误信息(达到最大重试次数返回错误)
func (b *BaseDevice) ReconnectWithBackoff(ctx context.Context, connectFn func() error) error {
	for attempt := 0; attempt < maxReconnectAttempts; attempt++ {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		// 计算延迟时间(指数退避)
		delay := reconnectBaseDelayMs * int(math.Pow(2, float64(attempt)))
		if delay > reconnectMaxDelayMs {
			delay = reconnectMaxDelayMs
		}

		slog.Info("Reconnect attempt", "device", b.config.ID, "attempt", attempt+1, "delay_ms", delay)
		time.Sleep(time.Duration(delay) * time.Millisecond)

		// 执行连接
		if err := connectFn(); err != nil {
			slog.Warn("Reconnect failed", "device", b.config.ID, "attempt", attempt+1, "err", err)
			continue
		}

		slog.Info("Reconnect succeeded", "device", b.config.ID, "attempt", attempt+1)
		return nil
	}

	return ErrMaxReconnectAttempts
}

// ErrMaxReconnectAttempts 最大重连次数达到错误
var ErrMaxReconnectAttempts = fmt.Errorf("max reconnect attempts reached")
