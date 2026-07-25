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

// testReadTimeout 是测试侧读取超时上限，与 drainConnection 的总耗时上限对齐
// （maxIters=10 × timeout=100ms = 1s）。低于该值会在 drainConnection 尚未
// 结束前误判超时；高于该值则拖长测试。三处 readWithTimeout 调用复用此常量。
const testReadTimeout = time.Second

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
			cmd, err := readWithTimeout(server, testReadTimeout)
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
			cmd, err := readWithTimeout(server, testReadTimeout)
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

func TestDAQT1603StopCommandCompletesBeforeReturn(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	commandsCh := make(chan string, 4)
	go func() {
		for {
			cmd, err := readWithTimeout(server, testReadTimeout)
			if err != nil {
				return
			}
			commandsCh <- cmd
			_, _ = server.Write([]byte("A\n"))
		}
	}()

	device := NewDAQT1603(core.Profile{ID: "t1603-1", Type: core.DeviceDaqT1603})
	device.conn = client
	device.frameReader = protocol.NewT1603FrameReader(client)
	device.status.Connection = core.ConnectionConnected
	device.config = core.DaqT1603HardwareConfig{
		ChannelMask:  "FFFF",
		BinaryFormat: true,
	}

	if err := device.StartAcquisition(); err != nil {
		t.Fatalf("StartAcquisition returned error: %v", err)
	}
	for _, want := range []string{"@f1", "@f0 FFFF 2"} {
		if got := <-commandsCh; got != want {
			t.Fatalf("command = %q, want %q", got, want)
		}
	}

	if err := device.StopAcquisition(); err != nil {
		t.Fatalf("StopAcquisition returned error: %v", err)
	}
	select {
	case got := <-commandsCh:
		if got != "@f1" {
			t.Fatalf("command = %q, want %q", got, "@f1")
		}
	default:
		t.Fatal("StopAcquisition returned before the stop command reached the connection")
	}
}

func TestDAQT1603StopAcquisitionWaitsForReadLoopExit(t *testing.T) {
	device := NewDAQT1603(core.Profile{ID: "t1603-1", Type: core.DeviceDaqT1603})
	device.acquiring = true
	device.stop = make(chan struct{})
	device.readLoopDone = make(chan struct{})
	done := device.readLoopDone
	device.status.Connection = core.ConnectionAcquiring
	device.status.Acquiring = true

	returned := make(chan error, 1)
	go func() {
		returned <- device.StopAcquisition()
	}()

	select {
	case err := <-returned:
		t.Fatalf("StopAcquisition returned before readLoop exited: %v", err)
	case <-time.After(50 * time.Millisecond):
	}

	close(done)

	select {
	case err := <-returned:
		if err != nil {
			t.Fatalf("StopAcquisition returned error: %v", err)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("StopAcquisition did not return after readLoop exited")
	}

	if device.status.Acquiring {
		t.Fatal("device still marked acquiring after stop")
	}
	if device.status.Connection != core.ConnectionConnected {
		t.Fatalf("connection status = %v, want %v", device.status.Connection, core.ConnectionConnected)
	}
	if device.readLoopDone != nil {
		t.Fatal("readLoopDone was not cleared after stop")
	}
}

func TestDAQT1603DrainConnectionWaitsForDelayedFrameTail(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	device := NewDAQT1603(core.Profile{ID: "t1603-1", Type: core.DeviceDaqT1603})
	written := make(chan struct{})
	go func() {
		time.Sleep(150 * time.Millisecond)
		_, _ = server.Write([]byte{1, 2, 3, 4})
		close(written)
	}()

	device.drainConnection(client, 100*time.Millisecond)

	select {
	case <-written:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("drainConnection returned before the delayed frame tail arrived")
	}
}
