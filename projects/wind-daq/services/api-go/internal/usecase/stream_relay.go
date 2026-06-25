package usecase

import (
	"context"
	"sync"

	"wind-daq/services/api-go/internal/core/device"
)

type DataStreamRelay struct {
	mu       sync.Mutex
	hub      *AcquisitionHub
	subs     map[string]context.CancelFunc
	payloads chan device.DataPayload
}

func NewDataStreamRelay(hub *AcquisitionHub) *DataStreamRelay {
	return &DataStreamRelay{
		hub:      hub,
		subs:     make(map[string]context.CancelFunc),
		payloads: make(chan device.DataPayload, 64),
	}
}

func (r *DataStreamRelay) Subscribe(deviceID string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.subs[deviceID]; exists {
		return
	}

	ch, unsub := r.hub.Subscribe(deviceID, 16)
	ctx, cancel := context.WithCancel(context.Background())
	r.subs[deviceID] = cancel

	go func() {
		defer cancel()
		for {
			select {
			case <-ctx.Done():
				unsub()
				return
			case payload, ok := <-ch:
				if !ok {
					unsub()
					return
				}
				select {
				case <-ctx.Done():
					unsub()
					return
				case r.payloads <- payload:
				}
			}
		}
	}()
}

func (r *DataStreamRelay) Unsubscribe(deviceID string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if cancel, exists := r.subs[deviceID]; exists {
		cancel()
		delete(r.subs, deviceID)
	}
}

func (r *DataStreamRelay) Payloads() <-chan device.DataPayload {
	return r.payloads
}

func (r *DataStreamRelay) Stop() {
	r.mu.Lock()
	defer r.mu.Unlock()

	for id, cancel := range r.subs {
		cancel()
		delete(r.subs, id)
	}
}
