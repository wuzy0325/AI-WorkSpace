package hardware

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"shared.local/device-sdk/go/motion/core"
)

func TestDefaultMotionControllerFactoryCreatesB140(t *testing.T) {
	ctrl, err := NewDefaultMotionControllerFactory().Create(core.MotionControllerProfile{
		ID:      "b140-1",
		Name:    "B140",
		Type:    core.ControllerTypeB140,
		Address: "127.0.0.1",
	})
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	if _, ok := ctrl.(*B140MotionController); !ok {
		t.Fatalf("Create returned %T, want *B140MotionController", ctrl)
	}
}

func newTestB140WithServer(t *testing.T, server *b140FakeServer, extra ...core.AxisConfig) *B140MotionController {
	profile := core.MotionControllerProfile{
		ID:      "b140-1",
		Name:    "B140",
		Type:    core.ControllerTypeB140,
		Address: server.host,
		Axes: []core.AxisConfig{
			{Name: core.AxisX, Enabled: true, Kind: core.AxisKindLinear, StepsPerRev: core.PtrFloat64(1.8), MicroSteps: core.PtrInt(4), Lead: core.PtrFloat64(4), MaxSpeed: core.PtrFloat64(20), PositionSource: core.PositionSourceRegister},
		},
	}
	profile.Axes = append(profile.Axes, extra...)
	ctrl := NewB140MotionController(profile)
	ctrl.profile.Port = server.port
	return ctrl
}

func TestB140ConnectSendsServoOnAndDirectionConfig(t *testing.T) {
	server := newB140FakeServer(t, map[string]string{
		"SH":     "",
		"MTA=2":  "",
		"CEA=0":  "",
		"MTB=-2": "",
		"CEB=2":  "",
	})
	defer server.close()

	ctrl := newTestB140WithServer(t, server, core.AxisConfig{Name: core.AxisY, Enabled: true, Kind: core.AxisKindLinear, StepsPerRev: core.PtrFloat64(1.8), MicroSteps: core.PtrInt(4), Lead: core.PtrFloat64(4), Inverted: true, MaxSpeed: core.PtrFloat64(20), PositionSource: core.PositionSourceRegister})
	if err := ctrl.Connect(context.Background()); err != nil {
		t.Fatalf("Connect returned error: %v", err)
	}

	want := []string{"SH", "MTA=2", "CEA=0", "MTB=-2", "CEB=2"}
	if got := server.commands(len(want)); !reflect.DeepEqual(got, want) {
		t.Fatalf("commands = %#v, want %#v", got, want)
	}
}

func TestB140MoveToSendsGalilCommands(t *testing.T) {
	server := newB140FakeServer(t, map[string]string{
		"SH":       "",
		"MTA=2":    "",
		"CEA=0":    "",
		"SPA=4000": "",
		"PAA=2000": "",
		"BGA":      "",
	})
	defer server.close()

	ctrl := newTestB140WithServer(t, server)
	if err := ctrl.Connect(context.Background()); err != nil {
		t.Fatalf("Connect returned error: %v", err)
	}
	if err := ctrl.MoveTo(context.Background(), core.AxisX, 10); err != nil {
		t.Fatalf("MoveTo returned error: %v", err)
	}

	want := []string{"SH", "MTA=2", "CEA=0", "SPA=4000", "PAA=2000", "BGA"}
	if got := server.commands(len(want)); !reflect.DeepEqual(got, want) {
		t.Fatalf("commands = %#v, want %#v", got, want)
	}
}

func TestB140MoveByAppliesConfiguredSpeed(t *testing.T) {
	server := newB140FakeServer(t, map[string]string{
		"SH":       "",
		"MTA=2":    "",
		"CEA=0":    "",
		"TD":       "0,0,0,0",
		"SPA=4000": "",
		"PRA=200":  "",
		"BGA":      "",
	})
	defer server.close()

	ctrl := newTestB140WithServer(t, server)
	if err := ctrl.Connect(context.Background()); err != nil {
		t.Fatalf("Connect returned error: %v", err)
	}
	if err := ctrl.MoveBy(context.Background(), core.AxisX, 1); err != nil {
		t.Fatalf("MoveBy returned error: %v", err)
	}

	want := []string{"SH", "MTA=2", "CEA=0", "TD", "SPA=4000", "PRA=200", "BGA"}
	if got := server.commands(len(want)); !reflect.DeepEqual(got, want) {
		t.Fatalf("commands = %#v, want %#v", got, want)
	}
}

func TestB140MoveByValidatesAgainstFreshHardwarePosition(t *testing.T) {
	server := newB140FakeServer(t, map[string]string{
		"SH":       "",
		"MTA=2":    "",
		"CEA=0":    "",
		"TD":       "1800,0,0,0",
		"SPA=4000": "",
		"PRA=200":  "",
		"BGA":      "",
	})
	defer server.close()

	minLimit := 0.0
	maxLimit := 10.0
	ctrl := newTestB140WithServer(t, server)
	ctrl.profile.Axes[0].MinLimit = &minLimit
	ctrl.profile.Axes[0].MaxLimit = &maxLimit
	ctrl.status.Axes[0].Position = 10

	if err := ctrl.Connect(context.Background()); err != nil {
		t.Fatalf("Connect returned error: %v", err)
	}
	if err := ctrl.MoveBy(context.Background(), core.AxisX, 1); err != nil {
		t.Fatalf("MoveBy returned error: %v", err)
	}

	want := []string{"SH", "MTA=2", "CEA=0", "TD", "SPA=4000", "PRA=200", "BGA"}
	if got := server.commands(len(want)); !reflect.DeepEqual(got, want) {
		t.Fatalf("commands = %#v, want %#v", got, want)
	}
}

func TestB140JogValidatesAgainstFreshHardwarePosition(t *testing.T) {
	server := newB140FakeServer(t, map[string]string{
		"SH":       "",
		"MTA=2":    "",
		"CEA=0":    "",
		"TD":       "1800,0,0,0",
		"SPA=1000": "",
		"PRA=200":  "",
		"BGA":      "",
	})
	defer server.close()

	minLimit := 0.0
	maxLimit := 10.0
	ctrl := newTestB140WithServer(t, server)
	ctrl.profile.Axes[0].MinLimit = &minLimit
	ctrl.profile.Axes[0].MaxLimit = &maxLimit
	ctrl.status.Axes[0].Position = 10

	if err := ctrl.Connect(context.Background()); err != nil {
		t.Fatalf("Connect returned error: %v", err)
	}
	if err := ctrl.Jog(context.Background(), core.AxisX, 5); err != nil {
		t.Fatalf("Jog returned error: %v", err)
	}

	want := []string{"SH", "MTA=2", "CEA=0", "TD", "SPA=1000", "PRA=200", "BGA"}
	if got := server.commands(len(want)); !reflect.DeepEqual(got, want) {
		t.Fatalf("commands = %#v, want %#v", got, want)
	}
}

func TestB140StatusParsesPositionAndSwitches(t *testing.T) {
	server := newB140FakeServer(t, map[string]string{
		"SH":      "",
		"MTA=2":   "",
		"CEA=0":   "",
		"TD":      "2000,0,0,0",
		"TS":      "128,0,0,0",
		"MG _LFA": "0.0000",
		"MG _LRA": "1.0000",
	})
	defer server.close()

	ctrl := newTestB140WithServer(t, server)
	if err := ctrl.Connect(context.Background()); err != nil {
		t.Fatalf("Connect returned error: %v", err)
	}
	status, err := ctrl.Status(context.Background())
	if err != nil {
		t.Fatalf("Status returned error: %v", err)
	}
	if !status.Connected {
		t.Fatal("status.Connected = false, want true")
	}
	if len(status.Axes) != 1 {
		t.Fatalf("len(status.Axes) = %d, want 1", len(status.Axes))
	}
	axis := status.Axes[0]
	if axis.Position != 10 {
		t.Fatalf("axis.Position = %v, want 10", axis.Position)
	}
	if !axis.Moving {
		t.Fatal("axis.Moving = false, want true")
	}
	if !axis.PosLimit {
		t.Fatal("axis.PosLimit = false, want true")
	}
	if axis.NegLimit {
		t.Fatal("axis.NegLimit = true, want false")
	}

	want := []string{"SH", "MTA=2", "CEA=0", "TD", "TS", "MG _LFA", "MG _LRA"}
	if got := server.commands(len(want)); !reflect.DeepEqual(got, want) {
		t.Fatalf("commands = %#v, want %#v", got, want)
	}
}

func TestB140StatusRefreshesRegisterPositionWhenAxisStops(t *testing.T) {
	server := newB140FakeServer(t, map[string]string{
		"SH":      "",
		"MTA=2":   "",
		"CEA=0":   "",
		"TS":      "0,0,0,0",
		"MG _LFA": "1.0000",
		"MG _LRA": "1.0000",
	})
	defer server.close()

	positions := []string{"-5876,0,0,0", "-6000,0,0,0"}
	var reads atomic.Int32
	server.setDynamic("TD", func() string {
		return positions[reads.Add(1)-1]
	})

	ctrl := newTestB140WithServer(t, server)
	if err := ctrl.Connect(context.Background()); err != nil {
		t.Fatalf("Connect returned error: %v", err)
	}
	ctrl.status.Axes[0].Moving = true

	status, err := ctrl.Status(context.Background())
	if err != nil {
		t.Fatalf("Status returned error: %v", err)
	}
	axis := status.Axes[0]
	if axis.Moving {
		t.Fatal("stopped axis remained marked moving")
	}
	if axis.Position != -30 {
		t.Fatalf("axis.Position = %v, want refreshed position -30", axis.Position)
	}
	if reads.Load() != 2 {
		t.Fatalf("TD calls = %d, want 2", reads.Load())
	}
}

func TestB140StatusRefreshesEncoderPositionWhenAxisStops(t *testing.T) {
	server := newB140FakeServer(t, map[string]string{
		"SH":      "",
		"MTA=2":   "",
		"CEA=0":   "",
		"TD":      "-6000,0,0,0",
		"TS":      "0,0,0,0",
		"MG _LFA": "1.0000",
		"MG _LRA": "1.0000",
	})
	defer server.close()

	positions := []string{"-5876", "-6000"}
	var reads atomic.Int32
	server.setDynamic("TPA", func() string {
		return positions[reads.Add(1)-1]
	})

	ctrl := newTestB140WithServer(t, server)
	ctrl.profile.Axes[0].PositionSource = core.PositionSourceEncoder
	ctrl.profile.Axes[0].EncoderScale = core.PtrFloat64(0.005)
	if err := ctrl.Connect(context.Background()); err != nil {
		t.Fatalf("Connect returned error: %v", err)
	}
	ctrl.status.Axes[0].Moving = true

	status, err := ctrl.Status(context.Background())
	if err != nil {
		t.Fatalf("Status returned error: %v", err)
	}
	axis := status.Axes[0]
	if axis.Moving {
		t.Fatal("stopped axis remained marked moving")
	}
	if axis.Position != -30 {
		t.Fatalf("axis.Position = %v, want refreshed position -30", axis.Position)
	}
	if reads.Load() != 2 {
		t.Fatalf("TPA calls = %d, want 2", reads.Load())
	}
}

type b140FakeServer struct {
	host      string
	port      int
	listener  net.Listener
	responses map[string]string
	// dynamic 命令处理函数：命中时优先于 responses。
	// 用于补偿测试中"每次 TP 返回不同值"的场景。
	dynamic  map[string]func() string
	mu       sync.Mutex
	received []string
}

func newB140FakeServer(t *testing.T, responses map[string]string) *b140FakeServer {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	server := &b140FakeServer{listener: listener, responses: responses, dynamic: make(map[string]func() string)}
	server.host, server.port = splitTCPAddr(t, listener.Addr().String())
	go server.serve()
	return server
}

func (s *b140FakeServer) serve() {
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			return
		}
		go s.handle(conn)
	}
}

func (s *b140FakeServer) handle(conn net.Conn) {
	defer conn.Close()
	reader := bufio.NewReader(conn)
	for {
		cmd, err := reader.ReadString('\r')
		if err != nil {
			return
		}
		cmd = strings.TrimSpace(cmd)
		s.mu.Lock()
		s.received = append(s.received, cmd)
		// 匹配顺序：
		//   1. 精确匹配 dynamic（如 "TD"）
		//   2. 命令前缀匹配 dynamic（如 prefix="PRA" 匹配 "PRA=200"，但不会匹配 "PRB=200"）
		//   3. 精确匹配 responses（如 "PAA=200"）
		var response string
		var ok bool
		if fn, hasDynamic := s.dynamic[cmd]; hasDynamic {
			response = fn()
			ok = true
		} else {
			for prefix, fn := range s.dynamic {
				if isB140CmdPrefix(cmd, prefix) {
					response = fn()
					ok = true
					break
				}
			}
			if !ok {
				response, ok = s.responses[cmd]
			}
		}
		s.mu.Unlock()
		if !ok {
			_, _ = conn.Write([]byte("?"))
			continue
		}
		_, _ = conn.Write([]byte(response + ":"))
	}
}

// setDynamic 注册动态响应函数。命中时优先于 responses 表。
// prefix 既可精确匹配（如 "TD"），也可命令前缀匹配（如 "PRA" 匹配 "PRA=200"）。
// 不会误匹配兄弟命令（"PRA" 不会匹配 "PRB=200"）。
func (s *b140FakeServer) setDynamic(cmd string, fn func() string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.dynamic[cmd] = fn
}

// isB140CmdPrefix 判断 prefix 是否为 cmd 的命令名前缀。
// 等价或 prefix+"=" 前缀，避免 "PRA" 误匹配 "PRB=200"。
func isB140CmdPrefix(cmd, prefix string) bool {
	if cmd == prefix {
		return true
	}
	return strings.HasPrefix(cmd, prefix+"=")
}

func (s *b140FakeServer) commands(minCount int) []string {
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		s.mu.Lock()
		commands := append([]string(nil), s.received...)
		s.mu.Unlock()
		if len(commands) >= minCount {
			return commands
		}
		time.Sleep(10 * time.Millisecond)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.received...)
}

func (s *b140FakeServer) close() {
	_ = s.listener.Close()
}

func splitTCPAddr(t *testing.T, addr string) (string, int) {
	t.Helper()
	host, portString, err := net.SplitHostPort(addr)
	if err != nil {
		t.Fatalf("SplitHostPort: %v", err)
	}
	var port int
	if _, err := fmt.Sscanf(portString, "%d", &port); err != nil {
		t.Fatalf("parse port: %v", err)
	}
	return host, port
}

// ---------- ADR-009 watchdog 兜底测试 ----------

// deadlineIgnoringConn 是忽略 SetDeadline 的连接替身，仅在 Close 后返回。
//
// 设计依据 ADR-009：必须包含忽略 deadline、仅在 Close 后才返回的连接替身，
// 用于模拟 SetDeadline 在故障 Windows 电脑上失效的场景。
// 此时 Read 不会在 deadline 到期后返回，必须依赖 watchdog Close 解除阻塞。
//
// 实现参考 shared/device-sdk/go/protocol/conn_helpers_test.go:481-508。
type deadlineIgnoringConn struct {
	net.Conn
	closed chan struct{}
	once   sync.Once
}

func newDeadlineIgnoringConn(inner net.Conn) *deadlineIgnoringConn {
	return &deadlineIgnoringConn{
		Conn:   inner,
		closed: make(chan struct{}),
	}
}

// SetDeadline 覆盖为 no-op，模拟 Windows 故障环境下 deadline 失效。
// b140 sendCommand 调用 SetDeadline（同时设置读写 deadline），此处全部忽略。
func (c *deadlineIgnoringConn) SetDeadline(t time.Time) error {
	return nil
}

// SetReadDeadline 同样覆盖为 no-op。
func (c *deadlineIgnoringConn) SetReadDeadline(t time.Time) error {
	return nil
}

// SetWriteDeadline 同样覆盖为 no-op，避免 Write 因 deadline 提前返回。
func (c *deadlineIgnoringConn) SetWriteDeadline(t time.Time) error {
	return nil
}

// Close 关闭连接并通知 closed channel，让测试可验证 Close 被调用。
func (c *deadlineIgnoringConn) Close() error {
	c.once.Do(func() { close(c.closed) })
	return c.Conn.Close()
}

// newB140WithDeadlineIgnoringConn 构造一个 c.conn 为 deadlineIgnoringConn 的 controller，
// 用于测试 watchdog 在 SetDeadline 失效场景下的兜底行为。
//
// 返回 controller 和 server 端 conn。server 端在后台 goroutine 中读取并丢弃
// client 发来的命令（让 client 的 Write 完成），但不写任何响应（让 client 的 Read 阻塞）。
// 测试结束时调用方应 defer server.Close()。
func newB140WithDeadlineIgnoringConn(t *testing.T) (*B140MotionController, net.Conn) {
	t.Helper()
	server, client := net.Pipe()
	ignored := newDeadlineIgnoringConn(client)

	ctrl := NewB140MotionController(core.MotionControllerProfile{
		ID:      "b140-watchdog-test",
		Name:    "B140-WatchDog-Test",
		Type:    core.ControllerTypeB140,
		Address: "127.0.0.1",
		Axes: []core.AxisConfig{
			{Name: core.AxisX, Enabled: true, Kind: core.AxisKindLinear, MaxSpeed: core.PtrFloat64(20)},
		},
	})
	ctrl.conn = ignored
	ctrl.reader = bufio.NewReader(ignored)
	ctrl.status.Connected = true

	// 服务端读取并丢弃命令，让 client 的 Write 完成；不写任何响应让 client 的 Read 阻塞。
	go func() {
		buf := make([]byte, 256)
		for {
			if _, err := server.Read(buf); err != nil {
				return
			}
		}
	}()

	return ctrl, server
}

// TestB140SendCommand_WatchdogTriggersOnDeadlineIgnoringConn 验证 ADR-009 watchdog 兜底：
// 当 SetDeadline 失效（Read 无限阻塞）时，watchdog 在 timeout 后强制 Close conn，
// 解除 sendCommand 阻塞并返回包含 "watchdog triggered" 或 "conn closed" 的错误。
//
// 修复前：sendCommand 的 <-done 无界等待，deadlineIgnoringConn 场景下永久阻塞。
// 修复后：watchdog 在 watchdogTimeout 后 Close conn，sendCommand 在有界时间内返回。
func TestB140SendCommand_WatchdogTriggersOnDeadlineIgnoringConn(t *testing.T) {
	ctrl, server := newB140WithDeadlineIgnoringConn(t)
	defer server.Close()
	defer ctrl.Disconnect(context.Background())

	// 用 100ms ctx 让 watchdog timeout = 100ms（远小于 b140CommandTimeout=5s）。
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	done := make(chan error, 1)
	go func() {
		_, err := ctrl.sendCommand(ctx, "TD")
		done <- err
	}()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !strings.Contains(err.Error(), "watchdog triggered") && !strings.Contains(err.Error(), "conn closed") {
			t.Errorf("error should mention 'watchdog triggered' or 'conn closed', got: %v", err)
		}
	case <-time.After(b140CommandTimeout + time.Second):
		t.Fatal("sendCommand did not return in time (watchdog did not trigger)")
	}
}

// TestB140Disconnect_DoesNotDeadlockWhenSendCommandBlocked 验证 ADR-009 死锁修复：
// sendCommand 阻塞时（deadlineIgnoringConn + Read 永久阻塞），Disconnect 能在 1s 内返回。
//
// 修复前的死锁链：
//  1. sendCommand 持 connMu，卡在 <-done（I/O goroutine 的 Read 永久阻塞）
//  2. Disconnect 调用 sendCommandLocked("ST") 或直接抢 connMu，永久等待
//
// 修复后：watchdog 在 100ms 后 Close conn → sendCommand 释放 connMu → Disconnect 继续。
func TestB140Disconnect_DoesNotDeadlockWhenSendCommandBlocked(t *testing.T) {
	ctrl, server := newB140WithDeadlineIgnoringConn(t)
	defer server.Close()

	sendDone := make(chan error, 1)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()
		_, err := ctrl.sendCommand(ctx, "TD")
		sendDone <- err
	}()

	// 等 sendCommand 进入阻塞（acquire connMu + Read 阻塞）。
	// 50ms 足够 goroutine 调度并完成 Write（server 端立即读取）。
	time.Sleep(50 * time.Millisecond)

	disconnectDone := make(chan struct{})
	go func() {
		_ = ctrl.Disconnect(context.Background())
		close(disconnectDone)
	}()

	select {
	case <-disconnectDone:
		// Disconnect 在 1s 内返回，继续验证 sendCommand
	case <-time.After(time.Second):
		t.Fatal("Disconnect deadlocked (did not return in 1s)")
	}

	select {
	case <-sendDone:
		// sendCommand 也已返回，无 goroutine 泄漏
	case <-time.After(time.Second):
		t.Fatal("sendCommand did not return after Disconnect")
	}
}

// TestB140SendCommand_InvalidatesConnectionOnWatchdogTrigger 验证 watchdog 触发后
// 连接被失效：c.conn==nil 且 c.status.Connected==false。
//
// 这避免后续命令复用已关闭的 conn，让调用方快速收到 "not connected" 错误，
// 而不是尝试向已关闭的 conn 写入导致难以诊断的 I/O 错误。
func TestB140SendCommand_InvalidatesConnectionOnWatchdogTrigger(t *testing.T) {
	ctrl, server := newB140WithDeadlineIgnoringConn(t)
	defer server.Close()
	defer ctrl.Disconnect(context.Background())

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	done := make(chan error, 1)
	go func() {
		_, err := ctrl.sendCommand(ctx, "TD")
		done <- err
	}()

	select {
	case <-done:
		// sendCommand 已返回
	case <-time.After(b140CommandTimeout + time.Second):
		t.Fatal("sendCommand did not return in time")
	}

	ctrl.connMu.Lock()
	connNil := ctrl.conn == nil
	ctrl.connMu.Unlock()
	if !connNil {
		t.Fatal("c.conn should be nil after watchdog trigger")
	}

	ctrl.mu.Lock()
	connected := ctrl.status.Connected
	ctrl.mu.Unlock()
	if connected {
		t.Fatal("c.status.Connected should be false after watchdog trigger")
	}
}
