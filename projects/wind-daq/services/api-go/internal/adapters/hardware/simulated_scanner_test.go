package hardware

import (
	"testing"

	"wind-daq/services/api-go/internal/core/device"
)

func TestSimulatedScannerFindsDefaultSimulator(t *testing.T) {
	results, err := NewSimulatedScanner().Scan()
	if err != nil {
		t.Fatalf("Scan returned error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected one result, got %d", len(results))
	}
	if results[0].ID != "sim-1" || results[0].Type != device.DeviceSimulated || !results[0].Available {
		t.Fatalf("unexpected simulated scan result: %+v", results[0])
	}
}
