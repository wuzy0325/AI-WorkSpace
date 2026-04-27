package usecase

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"wind-daq/services/api-go/internal/core/device"
	"wind-daq/services/api-go/internal/ports"
)

// ==================== 采集数据聚合器 ====================
// 负责从多个设备聚合采集数据,并以固定频率推送到前端
// 采用"最新值覆盖"策略:同一设备的多次采集只保留最新一次

// 采样率配置常量
const (
	defaultPublishHz = 20.0  // 默认推送频率20Hz
	minPublishHz     = 1.0   // 最小推送频率1Hz
	maxPublishHz     = 100.0 // 最大推送频率100Hz
)

// AcquisitionHub 采集数据聚合器
// 聚合多设备数据,定时推送到前端
type AcquisitionHub struct {
	mu             sync.RWMutex
	latestByDevice map[string]device.DataPayload // deviceID -> 最新数据
	publishHz      float64                       // 推送频率(Hz)
	publisher      ports.DataPublisher           // 数据广播
	store          ports.PublishRateStore        // 配置存储
	acquiring      bool                          // 是否正在推送
}

// NewAcquisitionHub 构建采集Hub
// 参数: wsHub WebSocket广播Hub, store 配置存储
// 返回: *AcquisitionHub 采集Hub实例
func NewAcquisitionHub(publisher ports.DataPublisher, store ports.PublishRateStore) *AcquisitionHub {
	// 从存储加载推送频率配置
	hz, err := store.LoadPublishRate()
	if err != nil || hz <= 0 {
		hz = defaultPublishHz
	}
	return &AcquisitionHub{
		latestByDevice: make(map[string]device.DataPayload),
		publishHz:      hz,
		publisher:      publisher,
		store:          store,
	}
}

// Start 启动周期推送
// 启动后台goroutine,按publishHz频率推送数据到前端
// 参数: ctx 上下文(用于取消)
func (h *AcquisitionHub) Start(ctx context.Context) {
	h.mu.Lock()
	h.acquiring = true
	h.mu.Unlock()

	// 计算推送间隔
	interval := time.Duration(float64(time.Second) / h.publishHz)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	slog.Info("AcquisitionHub started", "publishHz", h.publishHz, "interval", interval)

	for {
		select {
		case <-ctx.Done():
			h.mu.Lock()
			h.acquiring = false
			h.mu.Unlock()
			slog.Info("AcquisitionHub stopped")
			return
		case <-ticker.C:
			snapshot := h.takeSnapshot()
			if len(snapshot) > 0 {
				h.publisher.Broadcast(ports.ChannelDAQSnapshot, snapshot)
			}
		}
	}
}

// OnData 接收设备数据回调
// 设备采集数据通过此方法推送进来,采用最新值覆盖策略
// 参数: payload 采集数据包
func (h *AcquisitionHub) OnData(payload device.DataPayload) {
	h.mu.Lock()
	h.latestByDevice[payload.DeviceID] = payload
	h.mu.Unlock()
}

// UpdatePublishRate 更新推送频率
// 参数: hz 目标推送频率
// 返回: error 错误信息(超出范围时返回错误)
func (h *AcquisitionHub) UpdatePublishRate(hz float64) error {
	// 校验频率范围
	if hz < minPublishHz || hz > maxPublishHz {
		return ErrInvalidPublishRate
	}
	h.mu.Lock()
	h.publishHz = hz
	h.mu.Unlock()

	// 持久化保存
	if err := h.store.SavePublishRate(hz); err != nil {
		slog.Warn("Failed to save publish rate", "err", err)
	}
	slog.Info("Publish rate updated", "hz", hz)
	return nil
}

// GetPublishRate 获取当前推送频率
// 返回: float64 当前推送频率
func (h *AcquisitionHub) GetPublishRate() float64 {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.publishHz
}

// IsAcquiring 获取当前推送状态
// 返回: bool 是否正在推送
func (h *AcquisitionHub) IsAcquiring() bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.acquiring
}

// DataSink 获取数据接收回调函数
// 用于注册到DeviceManager
// 返回: device.DataSink 数据回调函数
func (h *AcquisitionHub) DataSink() device.DataSink {
	return h.OnData
}

// takeSnapshot 获取当前数据快照
// 内部方法,获取所有设备的最新数据
// 返回: []device.DataPayload 数据快照
func (h *AcquisitionHub) takeSnapshot() []device.DataPayload {
	h.mu.RLock()
	defer h.mu.RUnlock()
	snapshot := make([]device.DataPayload, 0, len(h.latestByDevice))
	for _, p := range h.latestByDevice {
		snapshot = append(snapshot, p)
	}
	return snapshot
}

// ErrInvalidPublishRate 推送频率无效错误
var ErrInvalidPublishRate = fmt.Errorf("publish rate must be between %.0f and %.0f Hz", minPublishHz, maxPublishHz)
