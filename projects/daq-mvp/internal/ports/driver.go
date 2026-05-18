package ports

import (
	"context"

	"daq-mvp/internal/core"
)

// DeviceInfo describes a detected device.
type DeviceInfo struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Channels int    `json:"channels"`
}

// DeviceDriver is the abstract interface for any DAQ device.
type DeviceDriver interface {
	Info() DeviceInfo
	Connect(ctx context.Context) error
	ReadBatch(ctx context.Context) (core.SampleBatch, error)
	Disconnect() error
}
