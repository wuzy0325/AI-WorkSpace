package hardware

import (
	"bufio"
	"net"
	"testing"
	"time"

	"wind-daq/services/api-go/internal/core/device"
)

func TestDSA3217StartStopDoesNotDeadlock(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	d := NewDSA3217(device.Profile{ID: "dsa-1", Name: "DSA", Type: device.DeviceDSA3217})
	d.conn = client
	d.reader = bufio.NewReader(client)
	d.status.Connection = device.ConnectionConnected

	commands := make(chan string, 2)
	go func() {
		reader := bufio.NewReader(server)
		for i := 0; i < 2; i++ {
			line, err := reader.ReadString('\n')
			if err != nil {
				return
			}
			commands <- line
			_, _ = server.Write([]byte("OK\n"))
		}
	}()

	mustFinish(t, "StartAcquisition", func() error { return d.StartAcquisition() })
	mustReceiveCommand(t, commands, "SCAN\r\n")
	mustFinish(t, "StopAcquisition", func() error { return d.StopAcquisition() })
	mustReceiveCommand(t, commands, "STOP\r\n")
}

func mustFinish(t *testing.T, name string, fn func() error) {
	t.Helper()
	done := make(chan error, 1)
	go func() { done <- fn() }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("%s returned error: %v", name, err)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatalf("%s deadlocked", name)
	}
}

func mustReceiveCommand(t *testing.T, commands <-chan string, want string) {
	t.Helper()
	select {
	case got := <-commands:
		if got != want {
			t.Fatalf("expected command %q, got %q", want, got)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatalf("timed out waiting for command %q", want)
	}
}
