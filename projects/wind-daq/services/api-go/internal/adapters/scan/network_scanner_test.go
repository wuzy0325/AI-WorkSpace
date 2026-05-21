package scan

import (
	"net"
	"testing"
	"time"
)

func TestNetworkScannerReturnsEmptyOnTimeout(t *testing.T) {
	scanner := NewNetworkScanner(WithTimeout(100 * time.Millisecond))
	results, err := scanner.Scan()
	if err != nil {
		t.Fatalf("Scan returned error: %v", err)
	}
	if results == nil || len(results) != 0 {
		t.Fatalf("expected empty results on timeout, got %d items", len(results))
	}
}

func TestNetworkScannerParsesDAQP1604Response(t *testing.T) {
	scanner := NewNetworkScanner()

	addr := &net.UDPAddr{IP: net.ParseIP("192.168.1.100"), Port: 30303}
	result := scanner.parseResponse([]byte("DAQP1604"), addr)

	if result == nil {
		t.Fatal("expected parsed result")
	}
	if result.Type != "DAQ_P_1604" {
		t.Fatalf("expected DAQ_P_1604 type, got %s", result.Type)
	}
	if !result.Available {
		t.Fatal("expected available")
	}
}

func TestNetworkScannerParsesDAQT1603Response(t *testing.T) {
	scanner := NewNetworkScanner()

	addr := &net.UDPAddr{IP: net.ParseIP("192.168.1.101"), Port: 30303}
	result := scanner.parseResponse([]byte("DAQT1603"), addr)

	if result == nil {
		t.Fatal("expected parsed result")
	}
	if result.Type != "DAQ_T_1603" {
		t.Fatalf("expected DAQ_T_1603 type, got %s", result.Type)
	}
}

func TestNetworkScannerIgnoresUnknownResponse(t *testing.T) {
	scanner := NewNetworkScanner()
	addr := &net.UDPAddr{IP: net.ParseIP("192.168.1.200"), Port: 30303}
	result := scanner.parseResponse([]byte("UNKNOWN_DEVICE"), addr)
	if result != nil {
		t.Fatal("expected nil for unknown device")
	}
}
