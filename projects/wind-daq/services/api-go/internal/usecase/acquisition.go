package usecase

import (
	"fmt"
	"log/slog"
	"sync"
	"time"

	"wind-daq/services/api-go/internal/core/device"
	"wind-daq/services/api-go/internal/ports"
)

const (
	minPublishHz           = 1.0
	maxPublishHz           = 100.0
	defaultHistoryCapacity = 256
	// 订阅者缓冲区满导致丢包时，至多每 dropLogInterval 输出一条聚合日志，
	// 避免高采样率（如 1 kHz）设备遇到慢订阅者时按设备速率刷屏。
	dropLogInterval = 5 * time.Second
)

type AcquisitionHub struct {
	mu              sync.RWMutex
	publisher       ports.Publisher
	publishHz       float64
	latestByDevice  map[string]device.DataPayload
	historyByDevice map[string][]device.DataPayload
	historyCapacity int
	subscribers     map[string]map[chan device.DataPayload]struct{}
	lastPublishAt   map[string]time.Time
	// dropCount 累计每个 device 自上次告警以来被丢弃的样本数；
	// dropLastLogAt 记录上次告警时间。两者均在 h.mu 保护下访问。
	dropCount     map[string]int
	dropLastLogAt map[string]time.Time
}

func NewAcquisitionHub(publisher ports.Publisher, publishHz float64) *AcquisitionHub {
	return NewAcquisitionHubWithHistoryCapacity(publisher, publishHz, defaultHistoryCapacity)
}

func NewAcquisitionHubWithHistoryCapacity(publisher ports.Publisher, publishHz float64, historyCapacity int) *AcquisitionHub {
	if publishHz < minPublishHz || publishHz > maxPublishHz {
		publishHz = 20
	}
	if historyCapacity < 1 {
		historyCapacity = defaultHistoryCapacity
	}
	return &AcquisitionHub{
		publisher:       publisher,
		publishHz:       publishHz,
		latestByDevice:  make(map[string]device.DataPayload),
		historyByDevice: make(map[string][]device.DataPayload),
		historyCapacity: historyCapacity,
		subscribers:     make(map[string]map[chan device.DataPayload]struct{}),
		lastPublishAt:   make(map[string]time.Time),
		dropCount:       make(map[string]int),
		dropLastLogAt:   make(map[string]time.Time),
	}
}

func (h *AcquisitionHub) OnData(payload device.DataPayload) {
	payload.EnsureNonNilSlices()
	h.mu.Lock()

	// 始终更新最新数据和历史记录
	h.latestByDevice[payload.DeviceID] = payload
	history := append(h.historyByDevice[payload.DeviceID], payload)
	if len(history) > h.historyCapacity {
		history = append([]device.DataPayload(nil), history[len(history)-h.historyCapacity:]...)
	}
	h.historyByDevice[payload.DeviceID] = history

	// 按 publishHz 节流推送到订阅者
	now := time.Now()
	lastPublish := h.lastPublishAt[payload.DeviceID]
	interval := time.Duration(float64(time.Second) / h.publishHz)
	shouldPublish := now.Sub(lastPublish) >= interval
	var subscribers []chan device.DataPayload
	if shouldPublish {
		h.lastPublishAt[payload.DeviceID] = now
		// 复制订阅者列表，避免在锁内发送导致阻塞
		for ch := range h.subscribers[payload.DeviceID] {
			subscribers = append(subscribers, ch)
		}
	}

	h.mu.Unlock()

	// 在锁外发送数据，避免阻塞其他设备的数据处理；
	// 缓冲区满时仅累计丢弃计数，按 dropLogInterval 节流输出聚合告警。
	if shouldPublish {
		var dropped int
		for _, ch := range subscribers {
			select {
			case ch <- payload:
			default:
				dropped++
			}
		}
		if dropped > 0 {
			h.recordDrops(payload.DeviceID, dropped)
		}
	}
}

// recordDrops 累计丢弃计数；距离上次告警超过 dropLogInterval 时输出一条聚合日志。
func (h *AcquisitionHub) recordDrops(deviceID string, dropped int) {
	now := time.Now()

	h.mu.Lock()
	h.dropCount[deviceID] += dropped
	total := h.dropCount[deviceID]
	last := h.dropLastLogAt[deviceID]
	shouldLog := last.IsZero() || now.Sub(last) >= dropLogInterval
	if shouldLog {
		h.dropLastLogAt[deviceID] = now
		h.dropCount[deviceID] = 0
	}
	h.mu.Unlock()

	if shouldLog {
		slog.Warn("AcquisitionHub: 订阅者缓冲区已满，数据被丢弃",
			"device", deviceID, "dropped", total, "interval", dropLogInterval)
	}
}

func (h *AcquisitionHub) GetLatestData(deviceID string) (device.DataPayload, bool) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	payload, ok := h.latestByDevice[deviceID]
	return payload, ok
}

func (h *AcquisitionHub) GetRecentData(deviceID string, limit int) []device.DataPayload {
	h.mu.RLock()
	defer h.mu.RUnlock()
	history := h.historyByDevice[deviceID]
	if limit < 1 || limit > len(history) {
		limit = len(history)
	}
	start := len(history) - limit
	return append([]device.DataPayload(nil), history[start:]...)
}

func (h *AcquisitionHub) Subscribe(deviceID string, buffer int) (<-chan device.DataPayload, func()) {
	if buffer < 1 {
		buffer = 1
	}
	ch := make(chan device.DataPayload, buffer)
	h.mu.Lock()
	if h.subscribers[deviceID] == nil {
		h.subscribers[deviceID] = make(map[chan device.DataPayload]struct{})
	}
	h.subscribers[deviceID][ch] = struct{}{}
	h.mu.Unlock()

	unsubscribe := func() {
		h.mu.Lock()
		if subscribers := h.subscribers[deviceID]; subscribers != nil {
			delete(subscribers, ch)
			if len(subscribers) == 0 {
				delete(h.subscribers, deviceID)
			}
		}
		h.mu.Unlock()
	}
	return ch, unsubscribe
}

func (h *AcquisitionHub) SetPublishRate(hz float64) error {
	if hz < minPublishHz || hz > maxPublishHz {
		return fmt.Errorf("publish rate must be between %.0f and %.0f Hz", minPublishHz, maxPublishHz)
	}
	h.mu.Lock()
	h.publishHz = hz
	h.mu.Unlock()
	return nil
}

func (h *AcquisitionHub) PublishRate() float64 {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.publishHz
}
