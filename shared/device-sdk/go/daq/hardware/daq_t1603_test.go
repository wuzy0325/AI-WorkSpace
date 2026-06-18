package hardware

import (
	"fmt"
	"net"
	"reflect"
	"testing"
	"time"

	"shared.local/device-sdk/go/daq/core"
	"shared.local/device-sdk/go/protocol"
)

func readWithTimeout(conn net.Conn, timeout time.Duration) (string, error) {
	type result struct {
		data string
		err  error
	}
	ch := make(chan result, 1)
	go func() {
		buf := make([]byte, 4096)
		n, err := conn.Read(buf)
		ch <- result{string(buf[:n]), err}
	}()
	select {
	case r := <-ch:
		return r.data, r.err
	case <-time.After(timeout):
		return "", fmt.Errorf("timeout")
	}
}

func TestDAQT1603ApplyConfigSendsHardwareCommands(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	commandsCh := make(chan []string, 1)
	go func() {
		commands := make([]string, 0)
		for {
			cmd, err := readWithTimeout(server, 200*time.Millisecond)
			if err != nil {
				commandsCh <- commands
				return
			}
			commands = append(commands, cmd)
			_, _ = server.Write([]byte("A\n"))
		}
	}()

	device := NewDAQT1603(core.Profile{ID: "t1603-1", Type: core.DeviceDaqT1603})
	device.conn = client
	cfg := core.DaqT1603HardwareConfig{
		ThermocoupleTypes: "KKKKKKKKKKKKKKKK",
		SamplingRate:      20,
		BinaryFormat:      true,
		AverageCount:      4,
		TriggerMode:       2,
		TriggerEdge:       1,
		TriggerCount:      3,
		ShowTimestamp:     true,
		ShowSequence:      true,
	}

	if err := device.ApplyDaqT1603Config(cfg); err != nil {
		t.Fatalf("ApplyDaqT1603Config returned error: %v", err)
	}
	client.Close()

	commands := <-commandsCh
	want := []string{
		"@f3 0KKKKKKKKKKKKKKKK0",
		"@fe BIN 1",
		"@fe TIME 1",
		"@fe HEAD 1",
		"@fe SPS 20",
		"@fe AVG 4",
		"@fe TYPE 2",
		"@fe TRIG 1",
		"@fe TNUM 3",
	}
	if !reflect.DeepEqual(commands, want) {
		t.Fatalf("commands = %#v, want %#v", commands, want)
	}

	got, err := device.GetDaqT1603Config()
	if err != nil {
		t.Fatalf("GetDaqT1603Config returned error: %v", err)
	}
	if !reflect.DeepEqual(got, cfg) {
		t.Fatalf("config = %#v, want %#v", got, cfg)
	}
}

func TestDAQT1603StartAcquisitionNormalizesHardwareTrigger(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	commandsCh := make(chan []string, 1)
	go func() {
		commands := make([]string, 0)
		for {
			cmd, err := readWithTimeout(server, 200*time.Millisecond)
			if err != nil {
				commandsCh <- commands
				return
			}
			commands = append(commands, cmd)
			_, _ = server.Write([]byte("A\n"))
		}
	}()

	device := NewDAQT1603(core.Profile{ID: "t1603-1", Type: core.DeviceDaqT1603})
	device.conn = client
	device.frameReader = protocol.NewT1603FrameReader(client)
	device.status.Connection = core.ConnectionConnected
	device.configSyncDone = make(chan struct{})
	close(device.configSyncDone)
	device.config = core.DaqT1603HardwareConfig{
		ChannelMask:   "FFFF",
		TriggerMode:   2,
		TriggerEdge:   1,
		TriggerCount:  3,
		BinaryFormat:  true,
		ShowTimestamp: true,
		ShowSequence:  true,
		SamplingRate:  10,
		AverageCount:  1,
	}

	if err := device.StartAcquisition(); err != nil {
		t.Fatalf("StartAcquisition returned error: %v", err)
	}

	if err := device.StopAcquisition(); err != nil {
		t.Fatalf("StopAcquisition returned error: %v", err)
	}
	time.Sleep(20 * time.Millisecond)
	client.Close()

	commands := <-commandsCh
	wantPrefix := []string{
		"@f1",
		"@f0 FFFF 2",
	}
	if len(commands) < len(wantPrefix) {
		t.Fatalf("commands = %#v, want prefix %#v", commands, wantPrefix)
	}
	for i, want := range wantPrefix {
		if commands[i] != want {
			t.Fatalf("commands[%d] = %q, want %q (all=%#v)", i, commands[i], want, commands)
		}
	}
}
