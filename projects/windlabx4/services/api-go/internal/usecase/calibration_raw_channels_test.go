package usecase

import (
	"reflect"
	"testing"

	"windlabx4/services/api-go/internal/core/calibration"
	"windlabx4/services/api-go/internal/core/device"
)

func TestBuildCalibrationRawDeviceLayoutsUsesReferencedDeviceOrderAndAllProfileChannels(t *testing.T) {
	probeChannels := []calibration.ProbeChannel{
		{DeviceID: "temp", ChannelIndex: 5, Enabled: true},
		{DeviceID: "pressure", ChannelIndex: 1, Enabled: true},
		{DeviceID: "temp", ChannelIndex: 2, Enabled: true},
		{DeviceID: "ignored-disabled", ChannelIndex: 0, Enabled: false},
	}
	profiles := []device.Profile{
		{ID: "pressure", Channels: []device.ChannelConfig{{Index: 0, Unit: "Pa"}, {Index: 4, Unit: "Pa"}}},
		{ID: "unreferenced", Channels: []device.ChannelConfig{{Index: 0, Unit: "Pa"}}},
		{ID: "temp", Channels: []device.ChannelConfig{{Index: 2, Unit: "degC"}, {Index: 5, Unit: "degC"}, {Index: 9, Unit: "degC"}}},
	}

	got, err := buildCalibrationRawDeviceLayouts(probeChannels, profiles)
	if err != nil {
		t.Fatalf("build layouts: %v", err)
	}
	want := []calibration.RawDeviceLayout{
		{
			DeviceID: "temp",
			Channels: []calibration.RawDeviceChannel{
				{Index: 2, Unit: "degC"},
				{Index: 5, Unit: "degC"},
				{Index: 9, Unit: "degC"},
			},
		},
		{
			DeviceID: "pressure",
			Channels: []calibration.RawDeviceChannel{
				{Index: 0, Unit: "Pa"},
				{Index: 4, Unit: "Pa"},
			},
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected layouts:\n got: %#v\nwant: %#v", got, want)
	}
}

func TestBuildCalibrationRawDeviceLayoutsRejectsReferencedDeviceWithoutProfile(t *testing.T) {
	_, err := buildCalibrationRawDeviceLayouts(
		[]calibration.ProbeChannel{{DeviceID: "missing", Enabled: true}},
		nil,
	)
	if err == nil {
		t.Fatal("expected missing profile error")
	}
}
