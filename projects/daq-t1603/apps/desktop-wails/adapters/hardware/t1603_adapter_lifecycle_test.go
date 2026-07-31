package hardware

import (
	"strings"
	"testing"
)

func TestT1603AdapterStartRejectsStopInProgress(t *testing.T) {
	adapter := NewT1603Adapter()
	adapter.operations["device-1"] = acquisitionOperationStopping

	_, err := adapter.StartAcquisition("device-1")
	if err == nil || !strings.Contains(err.Error(), "stop in progress") {
		t.Fatalf("StartAcquisition error = %v, want stop in progress", err)
	}
}
