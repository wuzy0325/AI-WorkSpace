package hardware

import (
	"testing"

	sharedcore "shared.local/device-sdk/go/daq/core"

	"windlabx4/services/api-go/internal/core/device"
	"windlabx4/services/api-go/internal/ports"
)

func TestPACE1000AdapterImplementsInterfaces(t *testing.T) {
	var adapter any = NewPACE1000Adapter(device.Profile{})
	if _, ok := adapter.(ports.Device); !ok {
		t.Fatal("PACE1000Adapter must implement ports.Device")
	}
	if _, ok := adapter.(ports.ErrorNotifiable); !ok {
		t.Fatal("PACE1000Adapter must implement ports.ErrorNotifiable")
	}
}

func TestMapToSharedPACE1000ProfileIncludesSerialAndChannel(t *testing.T) {
	profile := device.Profile{
		ID: "pace-1", Name: "Atmosphere", Type: device.DevicePACE1000,
		Transport: "serial", SerialPort: "COM7", BaudRate: 9600, SamplingRate: 2,
		Channels: []device.ChannelConfig{{Index: 0, Name: "大气压力", Enabled: true, Unit: "Pa", Precision: 1}},
	}

	got := mapToSharedPACE1000Profile(profile)
	if got.Type != sharedcore.DevicePACE1000 || got.SerialPort != "COM7" || got.BaudRate != 9600 {
		t.Fatalf("unexpected shared profile: %+v", got)
	}
	if len(got.Channels) != 1 || got.Channels[0].Name != "大气压力" || got.Channels[0].Unit != "Pa" {
		t.Fatalf("unexpected shared channels: %+v", got.Channels)
	}
}

func TestPACE1000AdapterStartRequiresConnect(t *testing.T) {
	adapter := NewPACE1000Adapter(device.Profile{ID: "pace-1", Type: device.DevicePACE1000})
	if err := adapter.StartAcquisition(); err == nil {
		t.Fatal("StartAcquisition should reject a disconnected adapter")
	}
}
