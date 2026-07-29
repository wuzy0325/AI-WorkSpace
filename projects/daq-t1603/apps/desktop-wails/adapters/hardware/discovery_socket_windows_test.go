//go:build windows

package hardware

import (
	"testing"
	"time"
)

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

// TestDiscoverySocketReceiveUnblocksOnClose 验证 watchdog 兜底的前提:
// 即使 SO_RCVTIMEO 设为很大的值(模拟 Windows IOCP deadline 失效场景,
// ADR-009 第 6 条要求的"忽略 deadline、只在 Close 后返回"连接 double),
// Closesocket 也能解除阻塞的 Recvfrom。
//
// Receive 内部 watchdog 使用相同的 timeout 启动 AfterFunc,本测试用大 timeout
// 保证 SO_RCVTIMEO 不在测试窗口内触发,然后由外部 Close 模拟 watchdog 触发路径。
// 若 Closesocket 不能解除阻塞的 Recvfrom,watchdog 路径同样无效,生产代码必卡。
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
