package hardware

import (
	"net"
	"time"
)

type discoverySocket interface {
	Send([]byte, string, int) error
	Receive([]byte, time.Duration) (int, string, error)
	Close() error
}

type packetDiscoverySocket struct {
	conn net.PacketConn
}

func (s *packetDiscoverySocket) Send(data []byte, target string, port int) error {
	_, err := s.conn.WriteTo(data, &net.UDPAddr{IP: net.ParseIP(target), Port: port})
	return err
}

func (s *packetDiscoverySocket) Receive(buf []byte, timeout time.Duration) (int, string, error) {
	if err := s.conn.SetReadDeadline(time.Now().Add(timeout)); err != nil {
		return 0, "", err
	}
	watchdog := time.AfterFunc(timeout, func() { _ = s.conn.Close() })
	defer watchdog.Stop()
	n, remote, err := s.conn.ReadFrom(buf)
	if err != nil {
		return 0, "", err
	}
	return n, remote.String(), nil
}

func (s *packetDiscoverySocket) Close() error { return s.conn.Close() }
