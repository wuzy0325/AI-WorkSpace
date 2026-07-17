package protocol

import (
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestStopReasonTracker_SetGetClear(t *testing.T) {
	var tr StopReasonTracker
	if got := tr.GetStopReason(); got != "" {
		t.Fatalf("initial reason should be empty, got %q", got)
	}
	tr.SetStopReason("first")
	if got := tr.GetStopReason(); got != "first" {
		t.Fatalf("after set first, want %q, got %q", "first", got)
	}
	// 多次设置只保留首个
	tr.SetStopReason("second")
	if got := tr.GetStopReason(); got != "first" {
		t.Fatalf("second SetStopReason should not override, want %q, got %q", "first", got)
	}
	tr.ClearStopReason()
	if got := tr.GetStopReason(); got != "" {
		t.Fatalf("after clear, want empty, got %q", got)
	}
	// Clear 后可再次设置
	tr.SetStopReason("third")
	if got := tr.GetStopReason(); got != "third" {
		t.Fatalf("after clear+set, want %q, got %q", "third", got)
	}
}

// TestStopReasonTracker_Concurrent 验证并发 Set/Get/Clear 无数据竞争
func TestStopReasonTracker_Concurrent(t *testing.T) {
	var tr StopReasonTracker
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			tr.SetStopReason(StopReasonUserRequested)
			_ = tr.GetStopReason()
			if n%2 == 0 {
				tr.ClearStopReason()
			}
		}(i)
	}
	wg.Wait()
}

func TestIsConnectionFault(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"i/o timeout", errors.New("read tcp: i/o timeout"), true},
		{"broken pipe", errors.New("write tcp: broken pipe"), true},
		{"reset by peer", errors.New("read: connection reset by peer"), true},
		{"closed", errors.New("use of closed network connection"), true},
		{"refused", errors.New("dial: connection refused"), true},
		{"device disconnected", errors.New("device disconnected"), true},
		{"unrelated", errors.New("some other error"), false},
	}
	for _, c := range cases {
		if got := IsConnectionFault(c.err); got != c.want {
			t.Errorf("%s: want %v, got %v", c.name, c.want, got)
		}
	}
}

func TestIsClosedConnError(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"net.ErrClosed", net.ErrClosed, true},
		{"closed msg", errors.New("use of closed network connection"), true},
		{"timeout not closed", &timeoutErr{}, false},
		{"unrelated", errors.New("read: connection reset by peer"), false},
	}
	for _, c := range cases {
		if got := IsClosedConnError(c.err); got != c.want {
			t.Errorf("%s: want %v, got %v", c.name, c.want, got)
		}
	}
}

// TestIsConnResetByPeer 验证"对端已 FIN/RST"判定。
// 仅匹配硬证据（EOF / reset / broken pipe / WSAECONNABORTED），
// 不匹配 timeout（软错误，连接可能仍可用）。
func TestIsConnResetByPeer(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"io.EOF", io.EOF, true},
		{"wrapped EOF", fmt.Errorf("read frame: %w", io.EOF), true},
		{"connection reset by peer", errors.New("read tcp: connection reset by peer"), true},
		{"broken pipe", errors.New("write tcp: broken pipe"), true},
		{"wsasend aborted", errors.New("write tcp 192.168.1.11:64695->192.168.1.7:9000: wsasend: An established connection was aborted by the software in your host machine."), true},
		{"wsarecv aborted", errors.New("read tcp: wsarecv: An existing connection was forcibly closed"), true},
		{"connection abort", errors.New("connection abort"), true},
		// 软错误不匹配
		{"i/o timeout", errors.New("read tcp: i/o timeout"), false},
		{"timeout net error", &timeoutErr{}, false},
		{"device error N05", errors.New("device returned error: N05"), false},
		{"parse error", errors.New("parse coefficient \"abc\": strconv.ParseFloat: invalid syntax"), false},
	}
	for _, c := range cases {
		if got := IsConnResetByPeer(c.err); got != c.want {
			t.Errorf("%s: want %v, got %v", c.name, c.want, got)
		}
	}
}

type timeoutErr struct{}

func (e *timeoutErr) Error() string   { return "i/o timeout" }
func (e *timeoutErr) Timeout() bool   { return true }
func (e *timeoutErr) Temporary() bool { return false }

func TestSendCommandNoNewline_NilConn(t *testing.T) {
	if err := SendCommandNoNewline(nil, "u01101", time.Second); err == nil {
		t.Fatal("expected error for nil conn")
	}
}

func TestSendCommandNoNewline_NoNewline(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	received := make(chan string, 1)
	go func() {
		buf := make([]byte, 64)
		n, _ := server.Read(buf)
		received <- string(buf[:n])
	}()

	if err := SendCommandNoNewline(client, "u01101", time.Second); err != nil {
		t.Fatalf("send: %v", err)
	}

	select {
	case got := <-received:
		// 关键断言：发送内容必须是 "u01101"，不带任何 \r 或 \n
		if got != "u01101" {
			t.Fatalf("sent payload must be exactly %q, got %q (bytes=%v)",
				"u01101", got, []byte(got))
		}
		if strings.ContainsAny(got, "\r\n") {
			t.Fatalf("sent payload must NOT contain \\r or \\n, got %q", got)
		}
	case <-time.After(time.Second):
		t.Fatal("server did not receive command within 1s")
	}
}

// TestSendCommandNoNewline_ZeroTimeout 验证 timeout<=0 时不设置 deadline，
// 命令仍能正常发送（适合调用方已自行管理 deadline 的场景）。
func TestSendCommandNoNewline_ZeroTimeout(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	received := make(chan string, 1)
	go func() {
		buf := make([]byte, 64)
		n, _ := server.Read(buf)
		received <- string(buf[:n])
	}()

	// timeout=0：函数内部不应设置/清除 deadline
	if err := SendCommandNoNewline(client, "q00", 0); err != nil {
		t.Fatalf("send with zero timeout: %v", err)
	}

	select {
	case got := <-received:
		if got != "q00" {
			t.Fatalf("sent payload must be %q, got %q", "q00", got)
		}
	case <-time.After(time.Second):
		t.Fatal("server did not receive command within 1s")
	}
}

func TestDrainConnection_Nil(t *testing.T) {
	// 不应 panic，返回 0
	if got := DrainConnection(nil, 100*time.Millisecond); got != 0 {
		t.Errorf("DrainConnection(nil) should return 0, got %d", got)
	}
}

func TestDrainConnection_DrainsAndStops(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	// 服务端先写一些残留数据
	go func() {
		_, _ = server.Write([]byte("residual-junk"))
		time.Sleep(20 * time.Millisecond)
		// 模拟后续无数据：让客户端的 Read 在 deadline 后超时
	}()

	start := time.Now()
	drained := DrainConnection(client, 50*time.Millisecond)
	elapsed := time.Since(start)

	// 至少应等满 1 次超时（50ms），最多 3 次（150ms）
	if elapsed < 40*time.Millisecond {
		t.Errorf("DrainConnection exited too quickly: %v", elapsed)
	}
	if elapsed > 300*time.Millisecond {
		t.Errorf("DrainConnection took too long: %v", elapsed)
	}
	// 应该读到了残留数据
	if drained <= 0 {
		t.Errorf("expected drained > 0, got %d", drained)
	}
}

func TestDrainW1601Response_NilArgs(t *testing.T) {
	// 不应 panic
	DrainW1601Response(nil, nil, 100*time.Millisecond)
}

func TestDrainW1601Response_ReadsFrame(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	// 服务端发一帧 A 应答
	go func() {
		writeFrame(t, server, "A")
	}()

	fr := NewFrameReader(client)
	// 应能读出这一帧且不阻塞
	DrainW1601Response(fr, client, 500*time.Millisecond)
}
