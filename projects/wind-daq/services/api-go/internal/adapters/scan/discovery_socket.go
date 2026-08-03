package scan

import (
	"net"
	"time"
)

// discoverySendTimeout 是 UDP 发现包 Send 阶段的独立 watchdog 预算（ADR-009 R0-9）。
//
// 设计动机：UDP WriteTo 在 Windows 故障机器上可能因 kernel IOCP deadline 失效而
// 永久阻塞（与 Recvfrom 同根问题）。SO_SNDTIMEO 同样不可靠，必须由独立 owner
// 在超时后强制 Close 解除阻塞。
//
// 取值 2s：覆盖典型 LAN UDP 发送（<10ms）+ 一定余量；触发后 socket 废弃，
// 调用方（scanWithSocket）仅记录日志继续下一目标，不影响整体扫描流程。
// 与 daq-p1604 discovery_socket.go 保持同款常量，便于跨项目 grep 复用。
const discoverySendTimeout = 2 * time.Second

type discoverySocket interface {
	Send([]byte, string, int) error
	Receive([]byte, time.Duration) (int, string, error)
	Close() error
}

type packetDiscoverySocket struct {
	conn net.PacketConn
}

// Send 发送 UDP 发现包，覆盖 SetWriteDeadline + watchdog 双重兜底（ADR-009 R0-9）。
//
// 历史背景：原实现仅 `_, err := s.conn.WriteTo(...)` 无任何 deadline 或 watchdog，
// 在故障 Windows 环境下 WriteTo 永久阻塞时 scanWithSocket 的 defer Close 永不执行。
//
// 整改后：先设 SetWriteDeadline 作为软超时，再启动独立 watchdog timer 在
// discoverySendTimeout 后强制 conn.Close 解除阻塞。两者任一触发都能让 WriteTo 返回。
// 触发后 socket 废弃，调用方不得复用。
func (s *packetDiscoverySocket) Send(data []byte, target string, port int) error {
	addr := &net.UDPAddr{IP: net.ParseIP(target), Port: port}
	// SetWriteDeadline 作为软超时（部分场景下 OS 能正常兑现）。
	_ = s.conn.SetWriteDeadline(time.Now().Add(discoverySendTimeout))
	// 独立 watchdog：deadline 失效时强制 Close conn 解除阻塞的 WriteTo。
	// 与 Receive 的 watchdog 模式一致，跨平台行为统一。
	// LSP 环境加固：Close 在挂起 I/O 时可能永久阻塞，放后台执行不卡 timer goroutine。
	watchdog := time.AfterFunc(discoverySendTimeout, func() { go s.conn.Close() })
	defer watchdog.Stop()
	_, err := s.conn.WriteTo(data, addr)
	return err
}

// Receive 接收 UDP 响应，覆盖 SetReadDeadline + watchdog 双重兜底（ADR-009 R0-8）。
//
// 历史背景：原实现仅设 SetReadDeadline 后调用同步 ReadFrom，调用方 defer Close 与
// 阻塞调用在同一 goroutine，无法兜底。SetReadDeadline 在故障 Windows 环境下不可靠。
//
// 整改后：先设 SetReadDeadline 作为软超时，再启动独立 watchdog timer 在 timeout 后
// 强制 conn.Close 解除阻塞的 ReadFrom。两者任一触发都能让 ReadFrom 返回错误。
// 触发后 socket 废弃，调用方不得复用。
func (s *packetDiscoverySocket) Receive(buf []byte, timeout time.Duration) (int, string, error) {
	if err := s.conn.SetReadDeadline(time.Now().Add(timeout)); err != nil {
		return 0, "", err
	}
	// LSP 环境加固：Close 在挂起 I/O 时可能永久阻塞，放后台执行不卡 timer goroutine。
	watchdog := time.AfterFunc(timeout, func() { go s.conn.Close() })
	defer watchdog.Stop()
	n, remote, err := s.conn.ReadFrom(buf)
	if err != nil {
		return 0, "", err
	}
	return n, remote.String(), nil
}

func (s *packetDiscoverySocket) Close() error { return s.conn.Close() }
