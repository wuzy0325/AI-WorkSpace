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
