package hardware

import (
	"fmt"
	"math"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"shared.local/device-sdk/go/daq/core"
)

func TestDAQT1603RealDeviceCaptureSpikes(t *testing.T) {
	if os.Getenv("DAQ_T1603_REAL") != "1" {
		t.Skip("set DAQ_T1603_REAL=1 to run against 192.168.1.10:9000")
	}

	device := NewDAQT1603(core.Profile{
		ID: "t1603-real-spike-capture", Type: core.DeviceDaqT1603,
		Address: "192.168.1.10", Port: 9000,
		DaqT1603Config: core.DaqT1603HardwareConfig{
			ChannelMask: "FFFF", BinaryFormat: true, TriggerMode: 2,
		},
	})
	device.OnLog(func(entry LogEntry) {
		if entry.Level == "warn" || entry.Level == "error" {
			t.Logf("driver %s: %s (%s)", entry.Level, entry.Message, entry.Detail)
		}
	})
	if err := device.Connect(); err != nil {
		t.Fatalf("Connect returned error: %v", err)
	}
	defer device.Disconnect()

	var frames atomic.Int64
	minValues := make([]float64, 16)
	maxValues := make([]float64, 16)
	previous := make([]float64, 16)
	for i := range minValues {
		minValues[i] = math.Inf(1)
		maxValues[i] = math.Inf(-1)
		previous[i] = math.NaN()
	}
	spikes := make(chan string, 64)
	device.SetDataSink(func(payload core.DataPayload) {
		frame := frames.Add(1)
		for i, value := range payload.Channels {
			if value < minValues[i] {
				minValues[i] = value
			}
			if value > maxValues[i] {
				maxValues[i] = value
			}
			if !math.IsNaN(previous[i]) && math.Abs(value-previous[i]) > 10 {
				select {
				case spikes <- fmt.Sprintf("frame=%d CH%02d %.6f -> %.6f all=%v", frame, i+1, previous[i], value, payload.Channels):
				default:
				}
			}
			previous[i] = value
		}
	})
	if err := device.StartAcquisition(); err != nil {
		t.Fatalf("StartAcquisition returned error: %v", err)
	}
	deadline := time.After(60 * time.Second)
	for {
		select {
		case spike := <-spikes:
			t.Logf("SPIKE %s", spike)
		case <-deadline:
			if err := device.StopAcquisition(); err != nil {
				t.Logf("StopAcquisition: %v", err)
			}
			t.Logf("captured=%d min=%v max=%v", frames.Load(), minValues, maxValues)
			return
		}
	}
}

func TestDAQT1603RealDeviceRapidRestart(t *testing.T) {
	if os.Getenv("DAQ_T1603_REAL") != "1" {
		t.Skip("set DAQ_T1603_REAL=1 to run against 192.168.1.10:9000")
	}

	device := NewDAQT1603(core.Profile{
		ID:      "t1603-real-rapid-restart",
		Name:    "T1603 real device",
		Type:    core.DeviceDaqT1603,
		Address: "192.168.1.10",
		Port:    9000,
		DaqT1603Config: core.DaqT1603HardwareConfig{
			ChannelMask:  "FFFF",
			BinaryFormat: true,
			TriggerMode:  2,
		},
	})
	if err := device.Connect(); err != nil {
		t.Fatalf("Connect returned error: %v", err)
	}
	defer device.Disconnect()

	payloads := make(chan core.DataPayload, 32)
	device.SetDataSink(func(payload core.DataPayload) {
		select {
		case payloads <- payload:
		default:
		}
	})

	for cycle := 1; cycle <= 20; cycle++ {
		for len(payloads) > 0 {
			<-payloads
		}
		if err := device.StartAcquisition(); err != nil {
			t.Fatalf("cycle %d StartAcquisition returned error: %v", cycle, err)
		}

		select {
		case payload := <-payloads:
			if len(payload.Channels) != 16 {
				t.Fatalf("cycle %d channel count = %d, want 16", cycle, len(payload.Channels))
			}
			if payload.Channels[1] < 10 || payload.Channels[1] > 100 {
				t.Fatalf("cycle %d CH02 = %.6f, want a plausible live temperature", cycle, payload.Channels[1])
			}
			t.Logf("cycle %02d CH02=%.3f", cycle, payload.Channels[1])
		case <-time.After(3 * time.Second):
			t.Fatalf("cycle %d timed out waiting for first frame", cycle)
		}

		if err := device.StopAcquisition(); err != nil {
			t.Fatalf("cycle %d StopAcquisition returned error: %v", cycle, err)
		}
	}
}

func TestDAQT1603RealDeviceStopAfterSustainedAcquisition(t *testing.T) {
	if os.Getenv("DAQ_T1603_REAL") != "1" {
		t.Skip("set DAQ_T1603_REAL=1 to run against 192.168.1.10:9000")
	}

	device := NewDAQT1603(core.Profile{
		ID:      "t1603-real-sustained-stop",
		Type:    core.DeviceDaqT1603,
		Address: "192.168.1.10",
		Port:    9000,
		DaqT1603Config: core.DaqT1603HardwareConfig{
			ChannelMask:  "FFFF",
			BinaryFormat: true,
			TriggerMode:  2,
		},
	})
	if err := device.Connect(); err != nil {
		t.Fatalf("Connect returned error: %v", err)
	}
	defer device.Disconnect()

	var frames atomic.Int64
	device.SetDataSink(func(core.DataPayload) { frames.Add(1) })
	if err := device.StartAcquisition(); err != nil {
		t.Fatalf("StartAcquisition returned error: %v", err)
	}
	time.Sleep(10 * time.Second)

	started := time.Now()
	if err := device.StopAcquisition(); err != nil {
		t.Fatalf("StopAcquisition after %d frames returned error in %v: %v", frames.Load(), time.Since(started), err)
	}
	t.Logf("stopped after %d frames in %v", frames.Load(), time.Since(started))
}

func TestDAQT1603RealDeviceStopAllowsConfig(t *testing.T) {
	if os.Getenv("DAQ_T1603_REAL") != "1" {
		t.Skip("set DAQ_T1603_REAL=1 to run against 192.168.1.10:9000")
	}

	device := NewDAQT1603(core.Profile{
		ID:      "t1603-real-stop-config",
		Type:    core.DeviceDaqT1603,
		Address: "192.168.1.10",
		Port:    9000,
		DaqT1603Config: core.DaqT1603HardwareConfig{
			ChannelMask:  "FFFF",
			BinaryFormat: true,
			SamplingRate: 2,
			TriggerMode:  2,
		},
	})
	if err := device.Connect(); err != nil {
		t.Fatalf("Connect returned error: %v", err)
	}
	defer device.Disconnect()

	if err := device.StartAcquisition(); err != nil {
		t.Fatalf("StartAcquisition returned error: %v", err)
	}
	time.Sleep(50 * time.Millisecond)
	if err := device.StopAcquisition(); err != nil {
		t.Fatalf("StopAcquisition returned error: %v", err)
	}

	cfg, _ := device.GetDaqT1603Config()
	cfg.SamplingRate = 5
	if err := device.ApplyDaqT1603Config(cfg); err != nil {
		t.Fatalf("ApplyDaqT1603Config after stop returned error: %v", err)
	}
	resp, err := device.sendCommandExact(device.conn, "@fd SPS", 1)
	if err != nil || resp != "5" {
		t.Fatalf("SPS readback = %q, err = %v, want 5", resp, err)
	}

	cfg.SamplingRate = 2
	if err := device.ApplyDaqT1603Config(cfg); err != nil {
		t.Fatalf("restore SPS=2 returned error: %v", err)
	}
}

func TestDAQT1603RealDeviceRejectsStartDuringStop(t *testing.T) {
	if os.Getenv("DAQ_T1603_REAL") != "1" {
		t.Skip("set DAQ_T1603_REAL=1 to run against 192.168.1.10:9000")
	}

	device := NewDAQT1603(core.Profile{
		ID:      "t1603-real-stop-start-race",
		Type:    core.DeviceDaqT1603,
		Address: "192.168.1.10",
		Port:    9000,
		DaqT1603Config: core.DaqT1603HardwareConfig{
			ChannelMask:  "FFFF",
			BinaryFormat: true,
			SamplingRate: 2,
			TriggerMode:  2,
		},
	})
	if err := device.Connect(); err != nil {
		t.Fatalf("Connect returned error: %v", err)
	}
	defer device.Disconnect()

	for cycle := 1; cycle <= 20; cycle++ {
		if err := device.StartAcquisition(); err != nil {
			t.Fatalf("cycle %d StartAcquisition returned error: %v", cycle, err)
		}
		time.Sleep(5 * time.Millisecond)

		stopResult := make(chan error, 1)
		go func() { stopResult <- device.StopAcquisition() }()
		deadline := time.Now().Add(time.Second)
		for {
			device.mu.RLock()
			stopping := device.stopping
			device.mu.RUnlock()
			if stopping {
				break
			}
			if time.Now().After(deadline) {
				t.Fatalf("cycle %d did not enter stopping state", cycle)
			}
		}
		for {
			select {
			case err := <-stopResult:
				if err != nil {
					t.Fatalf("cycle %d StopAcquisition returned error: %v", cycle, err)
				}
				goto stopped
			default:
				err := device.StartAcquisition()
				if err == nil || !strings.Contains(err.Error(), "stop in progress") {
					t.Fatalf("cycle %d concurrent StartAcquisition error = %v, want stop in progress", cycle, err)
				}
			}
		}
	stopped:
	}
}
