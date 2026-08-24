package calibration

import "testing"

func TestProbeCsvSchemasAppendConfiguredRawDeviceChannels(t *testing.T) {
	layouts := []RawDeviceLayout{
		{DeviceID: "dev-b", DeviceName: "压力采集B", Channels: []RawDeviceChannel{{Index: 1, Unit: "Pa"}, {Index: 3, Unit: "Pa"}}},
		{DeviceID: "dev-a", Channels: []RawDeviceChannel{{Index: 2, Unit: "degC"}}},
	}
	// 有 DeviceName 用设备名；未填时回退设备 ID
	wantSuffix := []string{"压力采集B_CH2(Pa)", "压力采集B_CH4(Pa)", "dev-a_CH3(degC)"}

	tests := []struct {
		name   string
		schema CsvSchema
	}{
		{name: "three-hole", schema: NewCsvSchema(Config{Type: string(TypeThreeHole), RawDeviceLayouts: layouts})},
		{name: "total-pressure", schema: NewCsvSchema(Config{Type: string(TypeTotalPressure), RawDeviceLayouts: layouts})},
		{name: "seven-hole certificate", schema: NewSevenHoleCsvSchema(Config{Type: string(TypeSevenHole), RawDeviceLayouts: layouts}, "inner", 0)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			header := tt.schema.BuildHeader()
			for i, want := range wantSuffix {
				if got := header[len(header)-len(wantSuffix)+i]; got != want {
					t.Fatalf("suffix column %d: got %q want %q", i, got, want)
				}
			}
		})
	}
}

func TestSevenHoleReferenceExportKeepsFixedColumnContract(t *testing.T) {
	layouts := []RawDeviceLayout{{DeviceID: "dev-1", Channels: []RawDeviceChannel{{Index: 0, Unit: "Pa"}}}}
	schema := NewSevenHoleExportCsvSchema(Config{Type: string(TypeSevenHole), RawDeviceLayouts: layouts}, "inner", 0)
	if got := len(schema.BuildHeader()); got != 18 {
		t.Fatalf("seven-hole reference export header columns: got %d want 18", got)
	}
}

func TestRawDeviceChannelValuesLeaveUnavailableReadsBlank(t *testing.T) {
	layouts := []RawDeviceLayout{{DeviceID: "dev-1", Channels: []RawDeviceChannel{{Index: 0}, {Index: 1}}}}
	values := buildRawDeviceChannelValues(
		layouts,
		map[string][]float64{"dev-1": {0, 12.5}},
		map[string][]bool{"dev-1": {true, false}},
	)
	if values[0] != "0.000" {
		t.Fatalf("valid zero should be retained, got %q", values[0])
	}
	if values[1] != "" {
		t.Fatalf("unavailable channel should be blank, got %q", values[1])
	}
}

func TestTotalTemperatureCsvSchemaIncludesConfiguredRawDeviceChannels(t *testing.T) {
	schema := NewCsvSchema(Config{
		Type: string(TypeTotalTemperature),
		RawDeviceLayouts: []RawDeviceLayout{
			{DeviceID: "dev-1", Channels: []RawDeviceChannel{{Index: 2, Unit: "degC"}}},
		},
	})
	record := schema.BuildRecord(&TotalTemperatureDataPoint{
		RawDeviceChannels: map[string][]float64{"dev-1": {21.5}},
		RawDeviceValid:    map[string][]bool{"dev-1": {true}},
	})
	header := schema.BuildHeader()
	if header[len(header)-1] != "dev-1_CH3(degC)" {
		t.Fatalf("unexpected raw-channel header: %v", header)
	}
	if record[len(record)-1] != "21.500" {
		t.Fatalf("unexpected raw-channel value: %v", record)
	}
	if len(header) != len(record) {
		t.Fatalf("header/record mismatch: %d != %d", len(header), len(record))
	}
}
