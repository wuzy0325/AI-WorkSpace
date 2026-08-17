package hardware

import (
	"testing"

	"windlabx4/services/api-go/internal/core/device"
	"windlabx4/services/api-go/internal/ports"
)

func TestT1602AdapterImplementsInterfaces(t *testing.T) {
	p := device.Profile{ID: "t1602-test", Type: device.DeviceDaqT1602}
	a := NewT1602Adapter(p)

	var _ ports.Device = a
	var _ ports.DaqT1602Configurable = a
	var _ ports.ErrorNotifiable = a
	_ = a
}

func TestT1602Adapter_SampleRateHzRoundTrip(t *testing.T) {
	in := device.DaqT1602HardwareConfig{SampleRateHz: 3}
	shared := mapToSharedT1602Config(in)
	if shared.SampleRateHz != 3 {
		t.Fatalf("mapToShared SampleRateHz = %v, want 3", shared.SampleRateHz)
	}
	out := mapFromSharedT1602Config(shared)
	if out.SampleRateHz != 3 {
		t.Fatalf("mapFromShared SampleRateHz = %v, want 3", out.SampleRateHz)
	}
}

func TestT1602Adapter_ID(t *testing.T) {
	a := NewT1602Adapter(device.Profile{ID: "my-t1602", Name: "Test"})
	if a.ID() != "my-t1602" {
		t.Fatalf("expected id my-t1602, got %q", a.ID())
	}
}

func TestT1602Adapter_StatusNotConnected(t *testing.T) {
	a := NewT1602Adapter(device.Profile{ID: "t1", Name: "Test", Type: device.DeviceDaqT1602})
	st := a.Status()
	if st.Connection != device.ConnectionDisconnected {
		t.Fatalf("expected Disconnected, got %v", st.Connection)
	}
	if st.ID != "t1" {
		t.Fatalf("expected id t1, got %q", st.ID)
	}
	if st.Type != device.DeviceDaqT1602 {
		t.Fatalf("expected type DAQ-T-1602, got %q", st.Type)
	}
}

func TestT1602Adapter_GetConfigDefault(t *testing.T) {
	p := device.Profile{ID: "t1"}
	var cfg device.DaqT1602HardwareConfig
	for i := range cfg.TypeCodes {
		cfg.TypeCodes[i] = 2 // T 型
	}
	p.DaqT1602Config = cfg
	a := NewT1602Adapter(p)

	got, err := a.GetDaqT1602Config()
	if err != nil {
		t.Fatalf("GetDaqT1602Config: %v", err)
	}
	if got.TypeCodes != cfg.TypeCodes {
		t.Fatalf("expected typeCodes %v, got %v", cfg.TypeCodes, got.TypeCodes)
	}
}

func TestT1602Adapter_ApplyConfig(t *testing.T) {
	a := NewT1602Adapter(device.Profile{ID: "t1"})

	var cfg device.DaqT1602HardwareConfig
	for i := range cfg.TypeCodes {
		cfg.TypeCodes[i] = 1 // K 型
	}
	cfg.TypeCodes[15] = 3 // E 型
	if err := a.ApplyDaqT1602Config(cfg); err != nil {
		t.Fatalf("ApplyDaqT1602Config: %v", err)
	}

	got, _ := a.GetDaqT1602Config()
	if got.TypeCodes != cfg.TypeCodes {
		t.Fatalf("expected typeCodes %v, got %v", cfg.TypeCodes, got.TypeCodes)
	}
}

func TestT1602Adapter_DisconnectNotConnected(t *testing.T) {
	a := NewT1602Adapter(device.Profile{ID: "t1"})
	if err := a.Disconnect(); err != nil {
		t.Fatalf("Disconnect on idle adapter should be safe: %v", err)
	}
}

func TestT1602Adapter_StartAcquisitionNotConnected(t *testing.T) {
	a := NewT1602Adapter(device.Profile{ID: "t1"})
	if err := a.StartAcquisition(); err == nil {
		t.Fatal("expected error starting acquisition on not connected device")
	}
}

func TestT1602Adapter_StopAcquisitionNotConnected(t *testing.T) {
	a := NewT1602Adapter(device.Profile{ID: "t1"})
	if err := a.StopAcquisition(); err != nil {
		t.Fatalf("StopAcquisition on idle adapter should be safe: %v", err)
	}
}

func TestT1602Adapter_SetDataSinkNotConnected(t *testing.T) {
	a := NewT1602Adapter(device.Profile{ID: "t1"})
	var called bool
	a.SetDataSink(func(payload device.DataPayload) {
		called = true
	})
	_ = called
}

func TestT1602Adapter_MapToSharedProfile(t *testing.T) {
	var tcConfig device.DaqT1602HardwareConfig
	for i := range tcConfig.TypeCodes {
		tcConfig.TypeCodes[i] = 2
	}
	p := device.Profile{
		ID:           "t1",
		Name:         "Test T1602",
		Address:      "192.168.3.201",
		Port:         502,
		SamplingRate: 5,
		Channels: []device.ChannelConfig{
			{Index: 0, Name: "TC1", Enabled: true, Unit: "degC", Precision: 2},
		},
		DaqT1602Config: tcConfig,
	}

	sp := mapToSharedT1602Profile(p)
	if sp.ID != "t1" {
		t.Fatalf("expected id t1, got %q", sp.ID)
	}
	if sp.Address != "192.168.3.201" {
		t.Fatalf("expected 192.168.3.201, got %q", sp.Address)
	}
	if sp.Port != 502 {
		t.Fatalf("expected port 502, got %d", sp.Port)
	}
	if string(sp.Type) != "DAQ-T-1602" {
		t.Fatalf("expected shared type DAQ-T-1602, got %q", sp.Type)
	}
	if len(sp.Channels) != 16 {
		t.Fatalf("expected 16 channels, got %d", len(sp.Channels))
	}
	if sp.Channels[0].Name != "TC1" {
		t.Fatalf("expected ch0 name TC1, got %q", sp.Channels[0].Name)
	}
	if sp.DaqT1602Config.TypeCodes != tcConfig.TypeCodes {
		t.Fatalf("expected shared typeCodes %v, got %v", tcConfig.TypeCodes, sp.DaqT1602Config.TypeCodes)
	}
}
