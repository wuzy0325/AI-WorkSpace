//go:build windows

package hardware

import (
	"fmt"
	"net"
	"time"

	"golang.org/x/sys/windows"
)

type winsockDiscoverySocket struct {
	handle windows.Handle
}

func openDiscoverySocket(localPort int) (discoverySocket, error) {
	handle, err := windows.Socket(windows.AF_INET, windows.SOCK_DGRAM, windows.IPPROTO_UDP)
	if err != nil {
		return nil, err
	}
	socket := &winsockDiscoverySocket{handle: handle}
	if err := windows.SetsockoptInt(handle, windows.SOL_SOCKET, windows.SO_BROADCAST, 1); err != nil {
		_ = socket.Close()
		return nil, err
	}
	if err := windows.Bind(handle, &windows.SockaddrInet4{Port: localPort}); err != nil {
		_ = socket.Close()
		return nil, err
	}
	return socket, nil
}

func (s *winsockDiscoverySocket) Send(data []byte, target string, port int) error {
	ip := net.ParseIP(target).To4()
	if ip == nil {
		return fmt.Errorf("invalid IPv4 target %q", target)
	}
	addr := &windows.SockaddrInet4{Port: port}
	copy(addr.Addr[:], ip)
	return windows.Sendto(s.handle, data, 0, addr)
}

func (s *winsockDiscoverySocket) Receive(buf []byte, timeout time.Duration) (int, string, error) {
	if err := windows.SetsockoptInt(s.handle, windows.SOL_SOCKET, windows.SO_RCVTIMEO, max(1, int(timeout.Milliseconds()))); err != nil {
		return 0, "", err
	}
	n, remote, err := windows.Recvfrom(s.handle, buf, 0)
	if err != nil {
		return 0, "", err
	}
	return n, sockaddrString(remote), nil
}

func (s *winsockDiscoverySocket) Close() error { return windows.Closesocket(s.handle) }

func sockaddrString(addr windows.Sockaddr) string {
	inet, ok := addr.(*windows.SockaddrInet4)
	if !ok {
		return ""
	}
	return net.JoinHostPort(net.IP(inet.Addr[:]).String(), fmt.Sprint(inet.Port))
}
