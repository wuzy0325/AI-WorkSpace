package apiserver

import (
	"context"
	"testing"
	"time"
)

func TestStartReturnsServer(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	srv, err := Start(ctx, ":0")
	if err != nil {
		t.Fatalf("Start returned error: %v", err)
	}
	if srv == nil {
		t.Fatal("expected non-nil server")
	}
}

func TestStartShutdownOnCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	srv, err := Start(ctx, ":0")
	if err != nil {
		t.Fatalf("Start returned error: %v", err)
	}
	if srv == nil {
		t.Fatal("expected non-nil server")
	}

	cancel()
	time.Sleep(100 * time.Millisecond)
}
