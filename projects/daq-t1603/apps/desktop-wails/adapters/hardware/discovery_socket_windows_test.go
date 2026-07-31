//go:build windows

package hardware

import (
	"testing"
	"time"
)

// TestDiscoverySocketReceiveReturnsAtTimeout 验证 SO_RCVTIMEO 软超时路径:
// 没有数据时 Recvfrom 必须在 timeout 内返回错误,不得永久阻塞。
// 该路径只覆盖 SO_RCVTIMEO 兑现的场景,不证明 watchdog 兜底(下个测试覆盖)。
func TestDiscoverySocketReceiveReturnsAtTimeout(t *testing.T) {
	socket, err := openDiscoverySocket(0)
	if err != nil {
		t.Fatalf("open discovery socket: %v", err)
	}
	defer socket.Close()

	started := time.Now()
	_, _, err = socket.Receive(make([]byte, 1024), 20*time.Millisecond)
	if err == nil {
		t.Fatal("expected receive timeout")
	}
	if elapsed := time.Since(started); elapsed > 200*time.Millisecond {
		t.Fatalf("receive timeout took too long: %v", elapsed)
	}
}

// TestDiscoverySocketReceiveUnblocksOnClose 验证 watchdog 兜底的前提(ADR-009 R0-8):
// 即使 SO_RCVTIMEO 设为很大的值(模拟 Windows IOCP deadline 失效场景,
// ADR-009 第 6 条要求的"忽略 deadline、只在 Close 后返回"连接 double),
// Closesocket 也能解除阻塞的 Recvfrom。
//
// Receive 内部 watchdog 使用相同的 timeout 启动 AfterFunc,本测试用大 timeout
// 保证 SO_RCVTIMEO 不在测试窗口内触发,然后由外部 Close 模拟 watchdog 触发路径。
// 若 Closesocket 不能解除阻塞的 Recvfrom,watchdog 路径同样无效,生产代码必卡。
//
// 对齐 daq-p1604 / wind-daq 的同名测试,确保三套 Windows scanner 一致验收。
func TestDiscoverySocketReceiveUnblocksOnClose(t *testing.T) {
	socket, err := openDiscoverySocket(0)
	if err != nil {
		t.Fatalf("open discovery socket: %v", err)
	}

	done := make(chan error, 1)
	go func() {
		// 30s SO_RCVTIMEO:确保测试期间 SO_RCVTIMEO 不会先返回,
		// 唯一可能的退出路径是外部 Close(watchdog 路径的等价物)。
		_, _, err := socket.Receive(make([]byte, 1024), 30*time.Second)
		done <- err
	}()

	// 让 Receive 进入 Recvfrom 阻塞后再 Close。
	time.Sleep(50 * time.Millisecond)
	_ = socket.Close()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("expected error after Close, got nil")
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("Receive did not unblock within 500ms after Close — Closesocket cannot release blocked Recvfrom, watchdog path is broken")
	}
}

// TestDiscoverySocketSendReturnsAtTimeout 验证 SO_SNDTIMEO 软超时路径(ADR-009 R0-9):
// Send 到不可达地址(无 ARP/路由)时必须在合理预算内返回错误,不得永久阻塞。
// 注意:UDP Sendto 通常立即返回(内核缓冲吸收),本测试仅覆盖触发 SO_SNDTIMEO 的场景,
// 真正的 watchdog 兜底由 PacketConn 测试覆盖(见 discovery_socket_test.go)。
func TestDiscoverySocketSendReturnsAtTimeout(t *testing.T) {
	socket, err := openDiscoverySocket(0)
	if err != nil {
		t.Fatalf("open discovery socket: %v", err)
	}
	defer socket.Close()

	// 向 192.0.2.1 (TEST-NET-1,无路由) 发送:大多数 Windows 实现会立即返回,
	// 极少触发 SO_SNDTIMEO。但 Send 本身必须能正常返回,不得永久阻塞。
	started := time.Now()
	err = socket.Send([]byte("probe"), "192.0.2.1", 7000)
	elapsed := time.Since(started)

	if elapsed > 3*time.Second {
		t.Fatalf("Send 阻塞过久: %v (SO_SNDTIMEO + watchdog 均未生效)", elapsed)
	}
	// err 可能为 nil(UDP Send 不保证对端接收)或非 nil,关键是不能阻塞。
	_ = err
}
