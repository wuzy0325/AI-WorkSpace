package driver_test

import (
	"testing"

	"cal1604/internal/domain"
	"cal1604/internal/infrastructure/driver"
)

func TestFactorySupportsMVPModels(t *testing.T) {
	factory := driver.NewFactory()

	testCases := []domain.Device{
		{
			ID:    "m1",
			Type:  domain.DeviceTypeMeasure,
			Model: "WTN1604",
			Host:  "127.0.0.1",
			Port:  9000,
		},
		{
			ID:    "p1",
			Type:  domain.DeviceTypePressure,
			Model: "ConST 811A",
			Host:  "127.0.0.1",
			Port:  7000,
		},
		{
			ID:    "p2",
			Type:  domain.DeviceTypePressure,
			Model: "ConST 820",
			Host:  "127.0.0.1",
			Port:  7001,
		},
	}

	for _, dev := range testCases {
		dev := dev
		t.Run(dev.Model, func(t *testing.T) {
			drv, err := factory.Create(dev)
			if err != nil {
				t.Fatalf("expected model %s to be supported, got error: %v", dev.Model, err)
			}

			if drv == nil {
				t.Fatalf("expected non-nil driver for model %s", dev.Model)
			}
		})
	}
}

func TestFactoryRejectsUnsupportedModel(t *testing.T) {
	factory := driver.NewFactory()

	_, err := factory.Create(domain.Device{
		ID:    "x1",
		Type:  domain.DeviceTypePressure,
		Model: "Unknown Model",
		Host:  "127.0.0.1",
		Port:  7002,
	})
	if err == nil {
		t.Fatal("expected unsupported model error, got nil")
	}
}

// TestFactoryCreatesP1603 验证 DAQ-P-1603 型号注册（含大小写归一化）。
func TestFactoryCreatesP1603(t *testing.T) {
	factory := driver.NewFactory()

	models := []string{"DAQ-P-1603", "daq-p-1603", "P1603"}
	for _, model := range models {
		dev := domain.Device{
			ID:       "m-p1603",
			Type:     domain.DeviceTypeMeasure,
			Model:    model,
			Host:     "192.168.1.50",
			Port:     0, // P1603 端口无意义（DLL 自管）
			Channels: domain.DefaultP1603Channels(),
		}
		t.Run(model, func(t *testing.T) {
			drv, err := factory.CreateMeasureDriver(dev)
			if err != nil {
				t.Fatalf("expected model %q supported, got error: %v", model, err)
			}
			if drv == nil {
				t.Fatal("expected non-nil measure driver")
			}
		})
	}
}
