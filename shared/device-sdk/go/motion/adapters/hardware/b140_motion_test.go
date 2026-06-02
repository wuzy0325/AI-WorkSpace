package hardware

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"shared.local/device-sdk/go/motion/core"
)

func TestDefaultMotionControllerFactoryCreatesB140(t *testing.T) {
	profile := testB140Profile("127.0.0.1", 23)
	ctrl, err := NewDefaultMotionControllerFactory().Create(profile)
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	if _, ok := ctrl.(*B140MotionController); !ok {
		t.Fatalf("Create returned %T, want *B140MotionController", ctrl)
	}
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

	profile := testB140Profile(server.host, server.port)
	profile.Axes = append(profile.Axes, core.AxisConfig{Name: core.AxisY, Enabled: true, Kind: core.AxisKindLinear, StepsPerRev: core.PtrFloat64(1.8), MicroSteps: core.PtrInt(4), Lead: core.PtrFloat64(4), Inverted: true, MaxSpeed: core.PtrFloat64(20), PositionSource: core.PositionSourceRegister})
	ctrl := NewB140MotionController(profile)
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
		"PAA=2000": "",
		"BGA":      "",
	})
	defer server.close()

	ctrl := NewB140MotionController(testB140Profile(server.host, server.port))
	if err := ctrl.Connect(context.Background()); err != nil {
		t.Fatalf("Connect returned error: %v", err)
	}
	if err := ctrl.MoveTo(context.Background(), core.AxisX, 10); err != nil {
		t.Fatalf("MoveTo returned error: %v", err)
	}

	want := []string{"SH", "MTA=2", "CEA=0", "PAA=2000", "BGA"}
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

	ctrl := NewB140MotionController(testB140Profile(server.host, server.port))
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

func testB140Profile(host string, port int) core.MotionControllerProfile {
	return core.MotionControllerProfile{
		ID:      "b140-1",
		Name:    "B140",
		Type:    core.ControllerTypeB140,
		Address: host,
		Port:    port,
		Axes: []core.AxisConfig{
			{Name: core.AxisX, Enabled: true, Kind: core.AxisKindLinear, StepsPerRev: core.PtrFloat64(1.8), MicroSteps: core.PtrInt(4), Lead: core.PtrFloat64(4), MaxSpeed: core.PtrFloat64(20), PositionSource: core.PositionSourceRegister},
		},
	}
}

type b140FakeServer struct {
	host      string
	port      int
	listener  net.Listener
	responses map[string]string
	mu        sync.Mutex
	received  []string
}

func newB140FakeServer(t *testing.T, responses map[string]string) *b140FakeServer {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	server := &b140FakeServer{listener: listener, responses: responses}
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
		response, ok := s.responses[cmd]
		s.mu.Unlock()
		if !ok {
			_, _ = conn.Write([]byte("?"))
			continue
		}
		_, _ = conn.Write([]byte(response + ":"))
	}
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
