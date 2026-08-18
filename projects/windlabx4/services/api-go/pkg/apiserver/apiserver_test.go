package apiserver

import (
	"context"
	"errors"
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
	select {
	case <-srv.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("server shutdown did not finish")
	}
	if err := srv.ShutdownErr(); err != nil {
		t.Fatalf("ShutdownErr: %v", err)
	}
}

type fakeRegistryShutter struct{ err error }

func (f fakeRegistryShutter) Shutdown(context.Context) error { return f.err }

type fakeCalibrationShutter struct{ called bool }

func (f *fakeCalibrationShutter) Shutdown() error { f.called = true; return nil }

type fakeHTTPCloser struct{ called bool }

func (f *fakeHTTPCloser) Close() error { f.called = true; return nil }

func TestShutdownOwnedServer_RegistryFailureIsReturnedAndHTTPRemainsOpen(t *testing.T) {
	shutdownErr := errors.New("registry stuck")
	calibration := &fakeCalibrationShutter{}
	httpServer := &fakeHTTPCloser{}
	err := shutdownOwnedServer(fakeRegistryShutter{err: shutdownErr}, calibration, httpServer)
	if !errors.Is(err, shutdownErr) {
		t.Fatalf("registry shutdown error must be observable, got %v", err)
	}
	if calibration.called || httpServer.called {
		t.Fatal("registry failure must not close shared services or HTTP")
	}
}
