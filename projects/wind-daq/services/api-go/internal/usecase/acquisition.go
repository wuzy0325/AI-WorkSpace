package usecase

import (
	"fmt"
	"sync"

	"wind-daq/services/api-go/internal/core/device"
	"wind-daq/services/api-go/internal/ports"
)

const (
	minPublishHz           = 1.0
	maxPublishHz           = 100.0
	defaultHistoryCapacity = 256
)

type AcquisitionHub struct {
	mu              sync.RWMutex
	publisher       ports.Publisher
	publishHz       float64
	latestByDevice  map[string]device.DataPayload
	historyByDevice map[string][]device.DataPayload
	historyCapacity int
	subscribers     map[string]map[chan device.DataPayload]struct{}
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
	}
}

func (h *AcquisitionHub) OnData(payload device.DataPayload) {
	h.mu.Lock()
	h.latestByDevice[payload.DeviceID] = payload
	history := append(h.historyByDevice[payload.DeviceID], payload)
	if len(history) > h.historyCapacity {
		history = append([]device.DataPayload(nil), history[len(history)-h.historyCapacity:]...)
	}
	h.historyByDevice[payload.DeviceID] = history
	for subscriber := range h.subscribers[payload.DeviceID] {
		select {
		case subscriber <- payload:
		default:
		}
	}
	h.mu.Unlock()
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
