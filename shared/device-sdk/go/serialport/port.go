package serialport

import (
	"fmt"
	"io"
	"sync"
	"time"

	"go.bug.st/serial"
	"shared.local/device-sdk/go/pkg/slog"
)

// Port serial port wrapper
type Port struct {
	mu   sync.Mutex
	port serial.Port
	name string
}

// Config serial port configuration
type Config struct {
	Name        string
	BaudRate    int
	DataBits    int
	Parity      serial.Parity
	StopBits    serial.StopBits
	ReadTimeout time.Duration
}

// DefaultConfig default config (9600-8-N-1)
func DefaultConfig(name string) Config {
	return Config{
		Name:        name,
		BaudRate:    9600,
		DataBits:    8,
		Parity:      serial.NoParity,
		StopBits:    serial.OneStopBit,
		ReadTimeout: 1 * time.Second,
	}
}

// Open opens serial port
func Open(cfg Config) (*Port, error) {
	mode := &serial.Mode{
		BaudRate: cfg.BaudRate,
		DataBits: cfg.DataBits,
		Parity:   cfg.Parity,
		StopBits: cfg.StopBits,
	}
	p, err := serial.Open(cfg.Name, mode)
	if err != nil {
		return nil, fmt.Errorf("open serial %s: %w", cfg.Name, err)
	}
	slog.Info("Serial port opened", "port", cfg.Name, "baud", cfg.BaudRate)
	return &Port{port: p, name: cfg.Name}, nil
}

// Close closes serial port
func (p *Port) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.port == nil {
		return nil
	}
	err := p.port.Close()
	p.port = nil
	slog.Info("Serial port closed", "port", p.name)
	return err
}

// Write writes data
func (p *Port) Write(data []byte) (int, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.port == nil {
		return 0, fmt.Errorf("serial port %s not open", p.name)
	}
	return p.port.Write(data)
}

// Read reads data
func (p *Port) Read(buf []byte) (int, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.port == nil {
		return 0, fmt.Errorf("serial port %s not open", p.name)
	}
	return p.port.Read(buf)
}

// SetReadTimeout sets read timeout
func (p *Port) SetReadTimeout(timeout time.Duration) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.port == nil {
		return fmt.Errorf("serial port %s not open", p.name)
	}
	return p.port.SetReadTimeout(timeout)
}

// ReadFull reads exact length of data
func (p *Port) ReadFull(buf []byte) error {
	_, err := io.ReadFull(p, buf)
	return err
}

// Name returns port name
func (p *Port) Name() string {
	return p.name
}

// ListPorts lists available serial ports
func ListPorts() ([]string, error) {
	ports, err := serial.GetPortsList()
	if err != nil {
		return nil, fmt.Errorf("list serial ports: %w", err)
	}
	return ports, nil
}
