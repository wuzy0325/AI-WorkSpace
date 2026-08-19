package serialport

import (
	"errors"
	"testing"
	"time"

	"go.bug.st/serial"
)

type blockingSerialPort struct {
	readStarted chan struct{}
	closed      chan struct{}
}

func newBlockingSerialPort() *blockingSerialPort {
	return &blockingSerialPort{
		readStarted: make(chan struct{}),
		closed:      make(chan struct{}),
	}
}

func (p *blockingSerialPort) SetMode(*serial.Mode) error { return nil }

func (p *blockingSerialPort) Read([]byte) (int, error) {
	close(p.readStarted)
	<-p.closed
	return 0, errors.New("port closed")
}

func (p *blockingSerialPort) Write(data []byte) (int, error) { return len(data), nil }
func (p *blockingSerialPort) Drain() error                   { return nil }
func (p *blockingSerialPort) ResetInputBuffer() error        { return nil }
func (p *blockingSerialPort) ResetOutputBuffer() error       { return nil }
func (p *blockingSerialPort) SetDTR(bool) error              { return nil }
func (p *blockingSerialPort) SetRTS(bool) error              { return nil }
func (p *blockingSerialPort) GetModemStatusBits() (*serial.ModemStatusBits, error) {
	return &serial.ModemStatusBits{}, nil
}
func (p *blockingSerialPort) SetReadTimeout(time.Duration) error { return nil }
func (p *blockingSerialPort) Break(time.Duration) error          { return nil }

func (p *blockingSerialPort) Close() error {
	select {
	case <-p.closed:
	default:
		close(p.closed)
	}
	return nil
}

func TestPortCloseUnblocksRead(t *testing.T) {
	underlying := newBlockingSerialPort()
	port := &Port{port: underlying, name: "COM1"}
	readDone := make(chan error, 1)
	go func() {
		_, err := port.Read(make([]byte, 1))
		readDone <- err
	}()

	select {
	case <-underlying.readStarted:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("read did not start")
	}

	closeDone := make(chan error, 1)
	go func() { closeDone <- port.Close() }()

	select {
	case err := <-closeDone:
		if err != nil {
			t.Fatalf("close failed: %v", err)
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("Close did not unblock the active Read")
	}

	select {
	case <-readDone:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("Read remained blocked after Close")
	}

	if err := port.Close(); err != nil {
		t.Fatalf("second close failed: %v", err)
	}
	if _, err := port.Read(make([]byte, 1)); err == nil {
		t.Fatal("read after close should fail")
	}
}
