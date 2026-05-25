package scan

import (
	"net"
	"testing"
	"time"
)

func TestNetworkScannerReturnsNoErrorOnTimeout(t *testing.T) {
	scanner := NewNetworkScanner(WithTimeout(100 * time.Millisecond))
	results, err := scanner.Scan()
	if err != nil {
		t.Fatalf("Scan returned error: %v", err)
	}
	if results == nil {
		t.Fatal("expected non-nil results slice")
	}
}

func TestParseDaqP1604ResponseCSV(t *testing.T) {
	csv := "192.168.1.100, AA-BB-CC-DD-EE-FF, 0, SN001, v1.2.3, 1, 1, 9000, 255.255.255.0, 192.168.1.1"
	result := parseDaqP1604Response([]byte(csv), "192.168.1.100:7000")

	if result == nil {
		t.Fatal("expected parsed result")
	}
	if result.Type != "DAQ-P-1604" {
		t.Fatalf("expected DAQ-P-1604 type, got %s", result.Type)
	}
	if result.Address != "192.168.1.100" {
		t.Fatalf("expected address 192.168.1.100, got %s", result.Address)
	}
	if result.Port != 9000 {
		t.Fatalf("expected port 9000, got %d", result.Port)
	}
	if result.MacAddress != "AA-BB-CC-DD-EE-FF" {
		t.Fatalf("expected MAC AA-BB-CC-DD-EE-FF, got %s", result.MacAddress)
	}
	if result.SerialNumber != "SN001" {
		t.Fatalf("expected serial SN001, got %s", result.SerialNumber)
	}
	if result.FirmwareVersion != "v1.2.3" {
		t.Fatalf("expected firmware v1.2.3, got %s", result.FirmwareVersion)
	}
}

func TestParseDaqP1604ResponseShort(t *testing.T) {
	result := parseDaqP1604Response([]byte("DAQP1604"), "192.168.1.100:7000")

	if result == nil {
		t.Fatal("expected parsed result for short DAQP1604 response")
	}
	if result.Type != "DAQ-P-1604" {
		t.Fatalf("expected DAQ-P-1604 type, got %s", result.Type)
	}
	if !result.Available {
		t.Fatal("expected available")
	}
}

func TestParseDaqT1603ResponseCSV(t *testing.T) {
	csv := "192.168.1.101, AA-BB-CC-DD-EE-11, SN002, ModelT, v2.0, 1, 1, 9000"
	result := parseDaqT1603Response([]byte(csv), "192.168.1.101:7000")

	if result == nil {
		t.Fatal("expected parsed result")
	}
	if result.Type != "DAQ-T-1603" {
		t.Fatalf("expected DAQ-T-1603 type, got %s", result.Type)
	}
	if result.Address != "192.168.1.101" {
		t.Fatalf("expected address 192.168.1.101, got %s", result.Address)
	}
	if result.Port != 9000 {
		t.Fatalf("expected port 9000, got %d", result.Port)
	}
}

func TestParseDaqT1603ResponseJSON(t *testing.T) {
	json := `{"ip":"192.168.1.102","port":9000,"mac":"CC-DD-EE-FF-00-11","serialNumber":"SN003","firmwareVersion":"v3.0"}`
	result := parseDaqT1603Response([]byte(json), "192.168.1.102:7000")

	if result == nil {
		t.Fatal("expected parsed result")
	}
	if result.Type != "DAQ-T-1603" {
		t.Fatalf("expected DAQ-T-1603 type, got %s", result.Type)
	}
	if result.Address != "192.168.1.102" {
		t.Fatalf("expected address 192.168.1.102, got %s", result.Address)
	}
	if result.MacAddress != "CC-DD-EE-FF-00-11" {
		t.Fatalf("expected MAC CC-DD-EE-FF-00-11, got %s", result.MacAddress)
	}
}

func TestParseDaqT1603ResponseShort(t *testing.T) {
	result := parseDaqT1603Response([]byte("DAQT1603"), "192.168.1.103:7000")

	if result == nil {
		t.Fatal("expected parsed result for short DAQT1603 response")
	}
	if result.Type != "DAQ-T-1603" {
		t.Fatalf("expected DAQ-T-1603 type, got %s", result.Type)
	}
}

func TestParseDaqP1064PreResponse(t *testing.T) {
	data := make([]byte, 36)
	data[5] = 192
	data[6] = 168
	data[7] = 1
	data[8] = 50
	data[9] = 0xAA
	data[10] = 0xBB
	data[11] = 0xCC
	data[12] = 0xDD
	data[13] = 0xEE
	data[14] = 0xFF

	result := parseDaqP1064PreResponse(data, "192.168.1.50:1901")

	if result == nil {
		t.Fatal("expected parsed result")
	}
	if result.Type != "DAQ-P-1064Pre" {
		t.Fatalf("expected DAQ-P-1064Pre type, got %s", result.Type)
	}
	if result.Address != "192.168.1.50" {
		t.Fatalf("expected address 192.168.1.50, got %s", result.Address)
	}
	if result.Port != 23 {
		t.Fatalf("expected port 23, got %d", result.Port)
	}
	if result.MacAddress != "AA:BB:CC:DD:EE:FF" {
		t.Fatalf("expected MAC AA:BB:CC:DD:EE:FF, got %s", result.MacAddress)
	}
}

func TestParseDaqP1064PreResponseTooShort(t *testing.T) {
	data := make([]byte, 10)
	result := parseDaqP1064PreResponse(data, "192.168.1.50:1901")
	if result != nil {
		t.Fatal("expected nil for short response")
	}
}

func TestNetworkScannerIgnoresUnknownResponse(t *testing.T) {
	result := parseDaqP1604Response([]byte("UNKNOWN_DEVICE"), "192.168.1.200:7000")
	if result != nil {
		t.Fatal("expected nil for unknown device")
	}
}

func TestComputeBroadcastAddress(t *testing.T) {
	ip := net.ParseIP("192.168.1.100").To4()
	mask := net.IPv4Mask(255, 255, 255, 0)
	broadcast := computeBroadcastAddress(ip, mask)
	if broadcast != "192.168.1.255" {
		t.Fatalf("expected 192.168.1.255, got %s", broadcast)
	}

	ip2 := net.ParseIP("10.0.0.1").To4()
	mask2 := net.IPv4Mask(255, 0, 0, 0)
	broadcast2 := computeBroadcastAddress(ip2, mask2)
	if broadcast2 != "10.255.255.255" {
		t.Fatalf("expected 10.255.255.255, got %s", broadcast2)
	}
}

func TestGetAllBroadcastTargets(t *testing.T) {
	targets := getAllBroadcastTargets()
	if len(targets) == 0 {
		t.Fatal("expected at least one broadcast target")
	}

	found := false
	for _, t := range targets {
		if t == limitedBroadcast {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected limited broadcast address in targets")
	}
}
