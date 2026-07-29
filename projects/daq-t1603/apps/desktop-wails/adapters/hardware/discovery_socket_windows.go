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
	// SO_RCVTIMEO 作为软超时:Windows IOCP/kernel 在某些环境下可能不兑现(ADR-009),
	// 因此同时启动 watchdog,到期后强制 Closesocket 解除阻塞的 Recvfrom。
	// watchdog 路径与 SO_RCVTIMEO 路径任一触发都能让 Recvfrom 返回错误。
	if err := windows.SetsockoptInt(s.handle, windows.SOL_SOCKET, windows.SO_RCVTIMEO, max(1, int(timeout.Milliseconds()))); err != nil {
		return 0, "", err
	}
	watchdog := time.AfterFunc(timeout, func() { _ = windows.Closesocket(s.handle) })
	defer watchdog.Stop()

	n, remote, err := windows.Recvfrom(s.handle, buf, 0)
	if err != nil {
		return 0, "", err
	}
	host, err := sockaddrString(remote)
	if err != nil {
		return 0, "", err
	}
	return n, host, nil
}

func (s *winsockDiscoverySocket) Close() error { return windows.Closesocket(s.handle) }

// sockaddrString 把 windows.Sockaddr 转为 "host:port" 字符串。
// socket 绑定的是 AF_INET4,理论上 remote 必为 *SockaddrInet4;
// 若出现其他类型,说明发生了不可能的状态,显式返回错误而非静默丢弃设备。
func sockaddrString(addr windows.Sockaddr) (string, error) {
	inet, ok := addr.(*windows.SockaddrInet4)
	if !ok {
		return "", fmt.Errorf("unexpected sockaddr type %T", addr)
	}
	return net.JoinHostPort(net.IP(inet.Addr[:]).String(), fmt.Sprint(inet.Port)), nil
}
