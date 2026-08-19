package hardware

import (
	"fmt"
	"sync"
	"time"

	"shared.local/device-sdk/go/daq/core"
	"shared.local/device-sdk/go/daq/ports"
	"shared.local/device-sdk/go/protocol"
	"shared.local/device-sdk/go/serialport"
)

const (
	PACE1000DefaultBaudRate = 9600
	PACE1000QueryWait       = 500 * time.Millisecond
	pace1000FailureLimit    = 3
	pace1000ReadBufferSize  = 256
)

type pace1000Port interface {
	Write([]byte) (int, error)
	Read([]byte) (int, error)
	Close() error
}

type PACE1000 struct {
	mu           sync.RWMutex
	profile      core.Profile
	status       core.Status
	port         pace1000Port
	sink         core.DataSink
	onReadError  func(error)
	stop         chan struct{}
	readLoopDone chan struct{}
	open         func(serialport.Config) (pace1000Port, error)
	wait         func(time.Duration)
}

var _ ports.Device = (*PACE1000)(nil)

func NewPACE1000(profile core.Profile) *PACE1000 {
	return &PACE1000{
		profile: profile,
		status:  core.Status{ID: profile.ID, Name: profile.Name, Type: profile.Type, Connection: core.ConnectionDisconnected},
		open:    func(cfg serialport.Config) (pace1000Port, error) { return serialport.Open(cfg) },
		wait:    time.Sleep,
	}
}

func (d *PACE1000) ID() string { return d.profile.ID }

func (d *PACE1000) SetOnError(fn func(error)) {
	d.mu.Lock()
	d.onReadError = fn
	d.mu.Unlock()
}

func (d *PACE1000) Connect() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.port != nil {
		return nil
	}
	name := d.profile.SerialPort
	if name == "" {
		return fmt.Errorf("PACE1000 serial port is required")
	}
	cfg := serialport.DefaultConfig(name)
	cfg.BaudRate = PACE1000DefaultBaudRate
	port, err := d.open(cfg)
	if err != nil {
		d.status.Connection = core.ConnectionError
		d.status.LastError = err.Error()
		return err
	}
	d.port = port
	d.status.Connection = core.ConnectionConnected
	d.status.LastError = ""
	return nil
}

func (d *PACE1000) Disconnect() error {
	d.mu.Lock()
	port := d.port
	d.port = nil
	wasAcquiring := d.status.Acquiring
	stop := d.stop
	d.status.Acquiring = false
	d.status.Connection = core.ConnectionDisconnected
	d.mu.Unlock()
	if wasAcquiring && stop != nil {
		close(stop)
	}
	if port != nil {
		if err := port.Close(); err != nil {
			return err
		}
	}
	if wasAcquiring && stop != nil {
		d.waitForLoop()
	}
	return nil
}

func (d *PACE1000) StartAcquisition() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.port == nil {
		return fmt.Errorf("device not connected")
	}
	if d.status.Acquiring {
		return nil
	}
	d.stop = make(chan struct{})
	d.readLoopDone = make(chan struct{})
	d.status.Acquiring = true
	d.status.Connection = core.ConnectionAcquiring
	go d.readLoop(d.port, d.stop, d.readLoopDone)
	return nil
}

func (d *PACE1000) StopAcquisition() error {
	d.mu.Lock()
	if !d.status.Acquiring {
		d.mu.Unlock()
		return nil
	}
	stop := d.stop
	d.status.Acquiring = false
	d.status.Connection = core.ConnectionConnected
	d.mu.Unlock()
	close(stop)
	d.waitForLoop()
	return nil
}

func (d *PACE1000) SetDataSink(sink core.DataSink) {
	d.mu.Lock()
	d.sink = sink
	d.mu.Unlock()
}

func (d *PACE1000) Status() core.Status {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.status
}

func (d *PACE1000) readLoop(port pace1000Port, stop <-chan struct{}, done chan<- struct{}) {
	defer close(done)
	failures := 0
	for {
		select {
		case <-stop:
			return
		default:
		}
		pressure, err := d.readPressure(port)
		if err != nil {
			failures++
			if failures >= pace1000FailureLimit {
				d.handleReadError(err)
				return
			}
			continue
		}
		failures = 0
		d.emitPressure(pressure)
	}
}

func (d *PACE1000) readPressure(port pace1000Port) (float64, error) {
	if _, err := port.Write([]byte(protocol.PACE1000Query)); err != nil {
		return 0, err
	}
	d.wait(PACE1000QueryWait)
	buffer := make([]byte, pace1000ReadBufferSize)
	n, err := port.Read(buffer)
	if err != nil {
		return 0, err
	}
	return protocol.ParsePACE1000Pressure(buffer[:n])
}

func (d *PACE1000) emitPressure(pressure float64) {
	d.mu.RLock()
	sink := d.sink
	profile := d.profile
	d.mu.RUnlock()
	if sink != nil {
		sink(core.DataPayload{DeviceID: profile.ID, Timestamp: core.NowMs(), Channels: []float64{pressure}, ChannelIndices: []int{0}})
	}
}

func (d *PACE1000) handleReadError(err error) {
	d.mu.Lock()
	port := d.port
	d.port = nil
	d.status.Acquiring = false
	d.status.Connection = core.ConnectionError
	d.status.LastError = err.Error()
	fn := d.onReadError
	d.mu.Unlock()
	if port != nil {
		_ = port.Close()
	}
	if fn != nil {
		fn(err)
	}
}

func (d *PACE1000) waitForLoop() {
	d.mu.RLock()
	done := d.readLoopDone
	d.mu.RUnlock()
	if done == nil {
		return
	}
	select {
	case <-done:
	case <-time.After(2 * time.Second):
	}
}
