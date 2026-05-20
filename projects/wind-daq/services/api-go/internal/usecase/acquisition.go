package usecase

import (
	"fmt"
	"sync"

	"wind-daq/services/api-go/internal/core/device"
	"wind-daq/services/api-go/internal/ports"
)

const (
	minPublishHz = 1.0
	maxPublishHz = 100.0
)

type AcquisitionHub struct {
	mu             sync.RWMutex
	publisher      ports.Publisher
	publishHz      float64
	latestByDevice map[string]device.DataPayload
}

func NewAcquisitionHub(publisher ports.Publisher, publishHz float64) *AcquisitionHub {
	if publishHz < minPublishHz || publishHz > maxPublishHz {
		publishHz = 20
	}
	return &AcquisitionHub{
		publisher:      publisher,
		publishHz:      publishHz,
		latestByDevice: make(map[string]device.DataPayload),
	}
}

func (h *AcquisitionHub) OnData(payload device.DataPayload) {
	h.mu.Lock()
	h.latestByDevice[payload.DeviceID] = payload
	h.mu.Unlock()
}

func (h *AcquisitionHub) GetLatestData(deviceID string) (device.DataPayload, bool) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	payload, ok := h.latestByDevice[deviceID]
	return payload, ok
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
