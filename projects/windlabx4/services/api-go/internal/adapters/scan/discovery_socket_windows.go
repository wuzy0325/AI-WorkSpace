//go:build windows

package scan

import (
	"fmt"
	"net"
	"sync"
	"time"

	"golang.org/x/sys/windows"
)

// soSNDTIMEO 是 Winsock2 SO_SNDTIMEO 的原始常量值。
//
// golang.org/x/sys/windows 在 v0.43.0 / v0.47.0 均未导出 SO_SNDTIMEO
// （只导出 SO_RCVTIMEO=0x1006）。直接使用 Winsock2.h 定义的 0x1005 即可，
// 该值在所有 Windows 版本上稳定不变。
//
// 仅在本文件使用，不导出，避免污染包级命名空间。
const soSNDTIMEO = 0x1005

// winsockDiscoverySocket 封装 Windows raw winsock UDP socket，提供带 watchdog
// 兜底的 Send/Receive 操作。
//
// ADR-009 finding 5 修复：handle 由 handleMu 保护，watchdog callback 通过
// closeHandleLocked 原子取走 handle 并置 0，确保 Closesocket 只执行一次；
// startWatchdog 返回 stop-and-join 函数，确保 Send/Receive 返回前 callback
// 已完全退出或被成功取消，避免 callback 在操作返回后执行 Closesocket 误关
// 已复用的 socket 数值。
type winsockDiscoverySocket struct {
	handleMu sync.Mutex
	handle   windows.Handle
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

// closeHandleLocked 在 handleMu 保护下取走 handle 并 Closesocket。
// 调用方必须持有 handleMu。多次调用安全：handle 已被取走（=0）时直接返回。
//
// LSP 环境加固：Closesocket 在存在挂起 Sendto/Recvfrom 时可能被安全软件
// 拦截而永久阻塞。若在 handleMu 内同步执行，watchdog callback 会永久持有
// handleMu，导致 wg.Wait()（startWatchdog 的 stop-and-join）永久阻塞，
// Send/Receive 永不返回。因此本函数只负责取走 handle 置 0（原子性保证
// Closesocket 只执行一次），Closesocket 本身由调用方在锁外执行。
func (s *winsockDiscoverySocket) closeHandleLocked() {
	h := s.handle
	s.handle = 0
	if h != 0 {
		// 锁外 Closesocket：即使被 LSP 卡死，锁已释放，其他路径不受影响。
		go windows.Closesocket(h)
	}
}

// startWatchdog 启动独立 watchdog timer，到期 Closesocket 解除阻塞的 Sendto/Recvfrom。
// 返回 stop 函数：停止 timer 并等待 callback 完全退出（stop-and-join 语义）。
//
// stop-and-join 必要性（ADR-009 finding 5）：
//   - time.AfterFunc.Stop 返回 false 仅表示 timer 已 fire，不保证 callback 已完成；
//   - 若 callback 在 Send/Receive 返回后才执行 Closesocket，handle 数值可能已被
//     Windows 复用，导致误关其他 socket；
//   - stop 函数通过 sync.WaitGroup 确保 callback 完全退出后才返回，callback 未
//     启动时（Stop 返回 true）手动 Done 避免 Wait 永久阻塞。
//
// callback 内通过 closeHandleLocked 原子取走 handle，确保 Closesocket 只执行一次，
// 即使 callback 与 Stop 竞态也不会重复 Closesocket 同一数值。
func (s *winsockDiscoverySocket) startWatchdog(timeout time.Duration) func() {
	var wg sync.WaitGroup
	wg.Add(1)
	wd := time.AfterFunc(timeout, func() {
		defer wg.Done()
		s.handleMu.Lock()
		s.closeHandleLocked()
		s.handleMu.Unlock()
	})
	return func() {
		if wd.Stop() {
			// timer 已停止，callback 不会执行，手动 Done 避免 Wait 永久阻塞
			wg.Done()
		}
		wg.Wait()
	}
}

// Send 发送 UDP 发现包，覆盖 SO_SNDTIMEO + watchdog 双重兜底（ADR-009 R0-9）。
//
// 历史背景：原实现仅调用 windows.Sendto 无任何 deadline 或 watchdog，
// 在故障 Windows 环境下 Sendto 永久阻塞时 scanWithSocket 的 defer Close 永不执行。
//
// 整改后：先设 SO_SNDTIMEO 作为软超时（kernel 可能不兑现），再启动独立 watchdog
// timer 在 discoverySendTimeout 后强制 Closesocket 解除阻塞的 Sendto。
// 两者任一触发都能让 Sendto 返回。触发后 socket handle 失效，调用方不得复用。
//
// Closesocket 是 Windows raw winsock 的唯一可靠解阻塞手段：它直接关闭内核 socket
// 句柄，让阻塞在 Sendto 上的 goroutine 立即收到 WSAENOTSOCK/WSAEINTR 错误返回。
func (s *winsockDiscoverySocket) Send(data []byte, target string, port int) error {
	ip := net.ParseIP(target).To4()
	if ip == nil {
		return fmt.Errorf("invalid IPv4 target %q", target)
	}
	addr := &windows.SockaddrInet4{Port: port}
	copy(addr.Addr[:], ip)

	s.handleMu.Lock()
	handle := s.handle
	s.handleMu.Unlock()
	if handle == 0 {
		return fmt.Errorf("socket already closed by watchdog")
	}

	// SO_SNDTIMEO 作为软超时（Windows kernel 在某些环境下可能不兑现，ADR-009）。
	if err := windows.SetsockoptInt(handle, windows.SOL_SOCKET, soSNDTIMEO, max(1, int(discoverySendTimeout.Milliseconds()))); err != nil {
		return err
	}
	// 独立 watchdog：SO_SNDTIMEO 失效时强制 Closesocket 解除阻塞的 Sendto。
	// startWatchdog 返回 stop-and-join 函数，确保 callback 不会在 Send 返回后
	// 误关已复用的 handle 数值（ADR-009 finding 5）。
	stopWd := s.startWatchdog(discoverySendTimeout)
	defer stopWd()

	return windows.Sendto(handle, data, 0, addr)
}

// Receive 接收 UDP 响应，覆盖 SO_RCVTIMEO + watchdog 双重兜底（ADR-009 R0-8）。
//
// 历史背景：原实现仅设 SO_RCVTIMEO 后调用同步 Recvfrom，调用方 defer Close 与
// 阻塞调用在同一 goroutine，无法兜底。SO_RCVTIMEO 在故障 Windows 环境下不可靠。
//
// 整改后：先设 SO_RCVTIMEO 作为软超时，再启动独立 watchdog timer 在 timeout 后
// 强制 Closesocket 解除阻塞的 Recvfrom。两者任一触发都能让 Recvfrom 返回错误。
// 触发后 socket handle 失效，调用方不得复用。
func (s *winsockDiscoverySocket) Receive(buf []byte, timeout time.Duration) (int, string, error) {
	s.handleMu.Lock()
	handle := s.handle
	s.handleMu.Unlock()
	if handle == 0 {
		return 0, "", fmt.Errorf("socket already closed by watchdog")
	}

	if err := windows.SetsockoptInt(handle, windows.SOL_SOCKET, windows.SO_RCVTIMEO, max(1, int(timeout.Milliseconds()))); err != nil {
		return 0, "", err
	}
	stopWd := s.startWatchdog(timeout)
	defer stopWd()

	n, remote, err := windows.Recvfrom(handle, buf, 0)
	if err != nil {
		return 0, "", err
	}
	host, err := sockaddrString(remote)
	if err != nil {
		return 0, "", err
	}
	return n, host, nil
}

func (s *winsockDiscoverySocket) Close() error {
	s.handleMu.Lock()
	s.closeHandleLocked()
	s.handleMu.Unlock()
	return nil
}

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
