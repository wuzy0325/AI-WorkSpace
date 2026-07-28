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
