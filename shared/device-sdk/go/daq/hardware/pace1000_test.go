package hardware

import (
	"errors"
	"sync"
	"testing"
	"time"

	"shared.local/device-sdk/go/daq/core"
	"shared.local/device-sdk/go/serialport"
)

type fakePACE1000Port struct {
	mu        sync.Mutex
	writes    [][]byte
	responses [][]byte
	closed    bool
}

func (p *fakePACE1000Port) Write(data []byte) (int, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.writes = append(p.writes, append([]byte(nil), data...))
	return len(data), nil
}

func (p *fakePACE1000Port) Read(data []byte) (int, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.responses) == 0 {
		return 0, errors.New("no response")
	}
	response := p.responses[0]
	p.responses = p.responses[1:]
	return copy(data, response), nil
}

func (p *fakePACE1000Port) Close() error {
	p.mu.Lock()
	p.closed = true
	p.mu.Unlock()
	return nil
}

func TestPACE1000ConnectUsesSerialDefaults(t *testing.T) {
	profile := core.Profile{ID: "pace-1", Type: core.DevicePACE1000, SerialPort: "COM7"}
	port := &fakePACE1000Port{}
	var got serialport.Config
	device := NewPACE1000(profile)
	device.open = func(cfg serialport.Config) (pace1000Port, error) {
		got = cfg
		return port, nil
	}

	if err := device.Connect(); err != nil {
		t.Fatalf("Connect returned error: %v", err)
	}
	if got.Name != "COM7" || got.BaudRate != 9600 || got.DataBits != 8 || got.ReadTimeout != time.Second {
		t.Fatalf("unexpected serial config: %+v", got)
	}
}

func TestPACE1000AcquisitionEmitsConvertedPressureAndExactQuery(t *testing.T) {
	port := &fakePACE1000Port{responses: [][]byte{[]byte("PACE 101.325\r\n")}}
	device := NewPACE1000(core.Profile{ID: "pace-1", Name: "PACE", Type: core.DevicePACE1000, SerialPort: "COM7"})
	device.open = func(serialport.Config) (pace1000Port, error) { return port, nil }
	device.wait = func(time.Duration) {}
	if err := device.Connect(); err != nil {
		t.Fatal(err)
	}
	payloads := make(chan core.DataPayload, 1)
	device.SetDataSink(func(payload core.DataPayload) { payloads <- payload })
	if err := device.StartAcquisition(); err != nil {
		t.Fatal(err)
	}
	select {
	case payload := <-payloads:
		if len(payload.Channels) != 1 || payload.Channels[0] != 101325 || payload.ChannelIndices[0] != 0 {
			t.Fatalf("unexpected payload: %+v", payload)
		}
	case <-time.After(time.Second):
		t.Fatal("did not receive pressure payload")
	}
	if err := device.StopAcquisition(); err != nil {
		t.Fatal(err)
	}
	port.mu.Lock()
	defer port.mu.Unlock()
	if len(port.writes) == 0 || string(port.writes[0]) != ":sens?\r" {
		t.Fatalf("unexpected query writes: %q", port.writes)
	}
}

func TestPACE1000DisconnectUnblocksBlockedRead(t *testing.T) {
	port := &blockingPACE1000Port{readStarted: make(chan struct{}), closed: make(chan struct{})}
	device := NewPACE1000(core.Profile{ID: "pace-1", Type: core.DevicePACE1000, SerialPort: "COM7"})
	device.open = func(serialport.Config) (pace1000Port, error) { return port, nil }
	device.wait = func(time.Duration) {}
	if err := device.Connect(); err != nil {
		t.Fatal(err)
	}
	if err := device.StartAcquisition(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-port.readStarted:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("read did not start")
	}
	done := make(chan error, 1)
	go func() { done <- device.Disconnect() }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Disconnect returned error: %v", err)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("Disconnect did not unblock the read")
	}
}

type blockingPACE1000Port struct {
	readStarted chan struct{}
	closed      chan struct{}
}

func (p *blockingPACE1000Port) Write([]byte) (int, error) { return 0, nil }

func (p *blockingPACE1000Port) Read([]byte) (int, error) {
	close(p.readStarted)
	<-p.closed
	return 0, errors.New("port closed")
}

func (p *blockingPACE1000Port) Close() error {
	select {
	case <-p.closed:
	default:
		close(p.closed)
	}
	return nil
}

func TestPACE1000ThreeReadFailuresNotifyAndEnterError(t *testing.T) {
	port := &fakePACE1000Port{}
	device := NewPACE1000(core.Profile{ID: "pace-1", Type: core.DevicePACE1000, SerialPort: "COM7"})
	device.open = func(serialport.Config) (pace1000Port, error) { return port, nil }
	device.wait = func(time.Duration) {}
	if err := device.Connect(); err != nil {
		t.Fatal(err)
	}
	errorsSeen := make(chan error, 1)
	device.SetOnError(func(err error) { errorsSeen <- err })
	if err := device.StartAcquisition(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-errorsSeen:
	case <-time.After(time.Second):
		t.Fatal("did not receive read error callback")
	}
	if status := device.Status(); status.Connection != core.ConnectionError || status.Acquiring {
		t.Fatalf("unexpected error status: %+v", status)
	}
}
