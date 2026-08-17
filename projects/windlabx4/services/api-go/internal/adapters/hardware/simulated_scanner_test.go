package hardware

import (
	"testing"

	"windlabx4/services/api-go/internal/core/device"
)

func TestSimulatedScannerReturnsMultipleDeviceTypes(t *testing.T) {
	results, err := NewSimulatedScanner().Scan()
	if err != nil {
		t.Fatalf("Scan returned error: %v", err)
	}
	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}

	expected := []struct {
		id   string
		typ  device.Type
		addr string
		port int
	}{
		{"scan-daq-p-1604-sim-192.168.1.100-9000", device.DeviceDAQP1604, "192.168.1.100", 9000},
		{"scan-daq-t-1603-sim-192.168.1.101-9000", device.DeviceDaqT1603, "192.168.1.101", 9000},
		{"scan-daq-p-1604pre-sim-192.168.1.102-23", device.DeviceDAQP1604Pre, "192.168.1.102", 23},
	}

	for i, exp := range expected {
		if results[i].ID != exp.id {
			t.Errorf("result[%d].ID = %q, want %q", i, results[i].ID, exp.id)
		}
		if results[i].Type != exp.typ {
			t.Errorf("result[%d].Type = %q, want %q", i, results[i].Type, exp.typ)
		}
		if results[i].Address != exp.addr {
			t.Errorf("result[%d].Address = %q, want %q", i, results[i].Address, exp.addr)
		}
		if results[i].Port != exp.port {
			t.Errorf("result[%d].Port = %d, want %d", i, results[i].Port, exp.port)
		}
		if !results[i].Available {
			t.Errorf("result[%d].Available = false, want true", i)
		}
	}
}
