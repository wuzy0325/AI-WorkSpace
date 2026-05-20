package device

import "testing"

func TestDefaultSimulatedProfileHasEnabledChannels(t *testing.T) {
	profile := NewDefaultProfile("sim-1", DeviceSimulated)

	if profile.ID != "sim-1" {
		t.Fatalf("expected profile id sim-1, got %q", profile.ID)
	}
	if profile.Type != DeviceSimulated {
		t.Fatalf("expected simulated type, got %q", profile.Type)
	}
	if len(profile.Channels) != 4 {
		t.Fatalf("expected 4 default channels, got %d", len(profile.Channels))
	}
	for _, channel := range profile.Channels {
		if !channel.Enabled {
			t.Fatalf("expected channel %d to be enabled", channel.Index)
		}
		if channel.Unit == "" {
			t.Fatalf("expected channel %d to have a unit", channel.Index)
		}
	}
}
