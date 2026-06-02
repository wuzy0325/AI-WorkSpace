package hardware

import (
	"bufio"
	"net"
	"reflect"
	"strings"
	"testing"

	"shared.local/device-sdk/go/daq/core"
)

func TestDAQT1603ApplyConfigSendsHardwareCommands(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	commandsCh := make(chan []string, 1)
	go func() {
		reader := bufio.NewReader(server)
		commands := make([]string, 0)
		for {
			cmd, err := reader.ReadString('\n')
			if err != nil {
				commandsCh <- commands
				return
			}
			commands = append(commands, strings.TrimSpace(cmd))
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
