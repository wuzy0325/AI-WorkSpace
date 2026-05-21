package scan

import (
	"fmt"
	"net"
	"time"

	"wind-daq/services/api-go/internal/core/device"
)

const (
	defaultDiscoveryPort = 30303
	defaultTimeout       = 3 * time.Second
	broadcastAddr        = "255.255.255.255"
)

type NetworkScanner struct {
	discoveryPort int
	timeout       time.Duration
}

type NetworkScannerOption func(*NetworkScanner)

func WithDiscoveryPort(port int) NetworkScannerOption {
	return func(s *NetworkScanner) { s.discoveryPort = port }
}

func WithTimeout(timeout time.Duration) NetworkScannerOption {
	return func(s *NetworkScanner) { s.timeout = timeout }
}

func NewNetworkScanner(opts ...NetworkScannerOption) *NetworkScanner {
	s := &NetworkScanner{
		discoveryPort: defaultDiscoveryPort,
		timeout:       defaultTimeout,
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

func (s *NetworkScanner) Scan() ([]device.ScanResult, error) {
	conn, err := net.ListenPacket("udp4", ":0")
	if err != nil {
		return nil, fmt.Errorf("udp listen: %w", err)
	}
	defer conn.Close()

	if err := conn.SetDeadline(time.Now().Add(s.timeout)); err != nil {
		return nil, fmt.Errorf("set deadline: %w", err)
	}

	discoveryMsg := []byte("WIND_DAQ_DISCOVER")
	addr := &net.UDPAddr{IP: net.ParseIP(broadcastAddr), Port: s.discoveryPort}
	if _, err := conn.WriteTo(discoveryMsg, addr); err != nil {
		return nil, fmt.Errorf("broadcast: %w", err)
	}

	var results []device.ScanResult
	buf := make([]byte, 1024)

	for {
		n, remote, err := conn.ReadFrom(buf)
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				break
			}
			return results, nil
		}

		result := s.parseResponse(buf[:n], remote)
		if result != nil {
			results = append(results, *result)
		}
	}

	if results == nil {
		results = []device.ScanResult{}
	}
	return results, nil
}

func (s *NetworkScanner) parseResponse(data []byte, addr net.Addr) *device.ScanResult {
	msg := string(data)

	switch {
	case msg == "DAQP1604" || len(msg) >= 8 && msg[:8] == "DAQP1604":
		id := fmt.Sprintf("daq-p1604-%s", addr.String())
		return &device.ScanResult{
			ID:        id,
			Name:      fmt.Sprintf("DAQ-P-1604 (%s)", addr.String()),
			Type:      device.DeviceDAQP1604,
			Available: true,
			Address:   addr.String(),
		}
	case msg == "DAQT1603" || len(msg) >= 8 && msg[:8] == "DAQT1603":
		id := fmt.Sprintf("daq-t1603-%s", addr.String())
		return &device.ScanResult{
			ID:        id,
			Name:      fmt.Sprintf("DAQ-T-1603 (%s)", addr.String()),
			Type:      device.DeviceDaqT1603,
			Available: true,
			Address:   addr.String(),
		}
	}

	return nil
}
