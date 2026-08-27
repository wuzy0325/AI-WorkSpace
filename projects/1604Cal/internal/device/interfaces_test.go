package device_test

import (
	"testing"

	"cal1604/internal/domain"
)

func TestDeviceTypeValues(t *testing.T) {
	if domain.DeviceTypeMeasure == "" {
		t.Fatal("expected DeviceTypeMeasure to be defined")
	}

	if domain.DeviceTypePressure == "" {
		t.Fatal("expected DeviceTypePressure to be defined")
	}
}
