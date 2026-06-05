package hardware

import (
	"testing"

	"wind-daq/services/api-go/internal/core/device"
	"wind-daq/services/api-go/internal/ports"
)

func TestT1603AdapterImplementsInterfaces(t *testing.T) {
	p := device.Profile{ID: "t1603-test", Type: device.DeviceDaqT1603}
	a := NewT1603Adapter(p)

	var _ ports.Device = a
	var _ ports.UnitConfigurable = a
	var _ ports.TareConfigurable = a
	var _ ports.DaqT1603Configurable = a
	_ = a
}

func TestT1603Adapter_ID(t *testing.T) {
	a := NewT1603Adapter(device.Profile{ID: "my-t1603", Name: "Test"})
	if a.ID() != "my-t1603" {
		t.Fatalf("expected id my-t1603, got %q", a.ID())
	}
}

func TestT1603Adapter_StatusNotConnected(t *testing.T) {
	a := NewT1603Adapter(device.Profile{ID: "t1", Name: "Test", Type: device.DeviceDaqT1603})
	st := a.Status()
	if st.Connection != device.ConnectionDisconnected {
		t.Fatalf("expected Disconnected, got %v", st.Connection)
	}
	if st.ID != "t1" {
		t.Fatalf("expected id t1, got %q", st.ID)
	}
}

func TestT1603Adapter_GetConfigDefault(t *testing.T) {
	p := device.Profile{ID: "t1"}
	p.DaqT1603Config = device.DaqT1603HardwareConfig{
		ThermocoupleTypes: "KKKKKKKKKKKKKKKK",
		ChannelMask:       "FFFF",
		SamplingRate:      10,
	}
	a := NewT1603Adapter(p)

	cfg, err := a.GetDaqT1603Config()
	if err != nil {
		t.Fatalf("GetDaqT1603Config: %v", err)
	}
	if cfg.ThermocoupleTypes != "KKKKKKKKKKKKKKKK" {
		t.Fatalf("expected thermocouple types, got %q", cfg.ThermocoupleTypes)
	}
	if cfg.SamplingRate != 10 {
		t.Fatalf("expected samplingRate 10, got %d", cfg.SamplingRate)
	}
}

func TestT1603Adapter_ApplyConfig(t *testing.T) {
	a := NewT1603Adapter(device.Profile{ID: "t1"})

	cfg := device.DaqT1603HardwareConfig{
		ThermocoupleTypes: "SSSSSSSSSSSSSSSS",
		ChannelMask:       "FF00",
		SamplingRate:      20,
		AverageCount:      8,
	}
	if err := a.ApplyDaqT1603Config(cfg); err != nil {
		t.Fatalf("ApplyDaqT1603Config: %v", err)
	}

	got, _ := a.GetDaqT1603Config()
	if got.SamplingRate != 20 {
		t.Fatalf("expected 20, got %d", got.SamplingRate)
	}
	if got.ChannelMask != "FF00" {
		t.Fatalf("expected FF00, got %q", got.ChannelMask)
	}
}

func TestT1603Adapter_DisconnectNotConnected(t *testing.T) {
	a := NewT1603Adapter(device.Profile{ID: "t1"})
	if err := a.Disconnect(); err != nil {
		t.Fatalf("Disconnect on idle adapter should be safe: %v", err)
	}
}

func TestT1603Adapter_SetDataSinkNotConnected(t *testing.T) {
	a := NewT1603Adapter(device.Profile{ID: "t1"})
	var called bool
	a.SetDataSink(func(payload device.DataPayload) {
		called = true
	})
	_ = called
}

func TestT1603Adapter_SetUnitNotConnected(t *testing.T) {
	a := NewT1603Adapter(device.Profile{ID: "t1"})
	if err := a.SetUnit("degF"); err == nil {
		t.Fatal("expected error setting unit on not connected device")
	}
}

func TestT1603Adapter_TareNotConnected(t *testing.T) {
	a := NewT1603Adapter(device.Profile{ID: "t1"})

	if err := a.SetTare(0, 1.5); err == nil {
		t.Fatal("expected error setting tare on not connected device")
	}
	if _, err := a.GetTare(0); err == nil {
		t.Fatal("expected error getting tare on not connected device")
	}
	if err := a.ClearTare(0); err == nil {
		t.Fatal("expected error clearing tare on not connected device")
	}
}

func TestT1603Adapter_MapToSharedProfile(t *testing.T) {
	p := device.Profile{
		ID:           "t1",
		Name:         "Test T1603",
		Address:      "192.168.1.7",
		Port:         9000,
		SamplingRate: 10,
		Channels: []device.ChannelConfig{
			{Index: 0, Name: "TC1", Enabled: true, Unit: "degC", Precision: 2},
		},
		DaqT1603Config: device.DaqT1603HardwareConfig{
			ThermocoupleTypes: "KKKKKKKKKKKKKKKK",
			SamplingRate:      10,
		},
	}

	sp := mapToSharedProfile(p)
	if sp.ID != "t1" {
		t.Fatalf("expected id t1, got %q", sp.ID)
	}
	if sp.Address != "192.168.1.7" {
		t.Fatalf("expected 192.168.1.7, got %q", sp.Address)
	}
	if len(sp.Channels) != 16 {
		t.Fatalf("expected 16 channels, got %d", len(sp.Channels))
	}
	if sp.Channels[0].Name != "TC1" {
		t.Fatalf("expected ch0 name TC1, got %q", sp.Channels[0].Name)
	}
}

func TestT1603Adapter_StatusMapping(t *testing.T) {
	shared := struct {
		ID, Name string
		Type     string
		Conn     string
		Acq      bool
	}{
		ID: "t1", Name: "Test", Type: "DAQ-T-1603", Conn: "Connected", Acq: false,
	}

	st := device.Status{
		ID: shared.ID, Name: shared.Name,
		Type: device.Type(shared.Type),
		Connection: device.Connection(shared.Conn),
		Acquiring:  shared.Acq,
	}

	if st.ID != "t1" || st.Type != device.DeviceDaqT1603 || st.Connection != device.ConnectionConnected {
		t.Fatalf("status mapping mismatch: %+v", st)
	}
}
