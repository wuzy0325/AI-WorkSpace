package hardware

import (
	"strings"
	"testing"

	sharedcore "shared.local/device-sdk/go/daq/core"
	sharedhw "shared.local/device-sdk/go/daq/hardware"

	"daq-t1603/core"
)

func TestT1603AdapterStartRejectsStopInProgress(t *testing.T) {
	adapter := NewT1603Adapter()
	adapter.operations["device-1"] = acquisitionOperationStopping

	_, err := adapter.StartAcquisition("device-1")
	if err == nil || !strings.Contains(err.Error(), "stop in progress") {
		t.Fatalf("StartAcquisition error = %v, want stop in progress", err)
	}
}

// TestT1603AdapterStopRemovesChannelsAtomically 验证 StopAcquisition 在持锁期间
// 就把 stopChs/channels 从 map 移除（随后才在锁外关闭），使 OnReadLoopExit 回调
// 永远无法对同一 channel 二次 close（panic: close of closed channel）。
// 快速点击 Start/Stop 实机复现：StopAcquisition 关闭 done 时仍留在 map，
// dev.StopAcquisition() 期间 readLoop 异常退出触发回调再次 close(done)。
func TestT1603AdapterStopRemovesChannelsAtomically(t *testing.T) {
	adapter := NewT1603Adapter()
	id := "device-1"

	done := make(chan struct{})
	ch := make(chan core.TemperatureSnapshot, 1)
	dev := sharedhw.NewDAQT1603(sharedcore.Profile{ID: id, Type: sharedcore.DeviceDaqT1603})

	adapter.mu.Lock()
	adapter.drivers[id] = dev
	adapter.stopChs[id] = done
	adapter.channels[id] = ch
	adapter.status[id] = &core.DeviceState{Profile: core.TemperatureProfile{ID: id}}
	adapter.mu.Unlock()

	if err := adapter.StopAcquisition(id); err != nil {
		t.Fatalf("StopAcquisition error: %v", err)
	}

	// Stop 返回后 map 必须已移除两个通道，回调路径再查 map 不会触发二次 close。
	adapter.mu.RLock()
	_, stopExists := adapter.stopChs[id]
	_, chExists := adapter.channels[id]
	adapter.mu.RUnlock()
	if stopExists || chExists {
		t.Fatalf("stopChs/channels still present after StopAcquisition: stop=%t ch=%t", stopExists, chExists)
	}

	// 模拟 OnReadLoopExit 回调路径：若 map 已无该通道，则不会 close → 不会 panic。
	adapter.mu.RLock()
	done2, ok := adapter.stopChs[id]
	adapter.mu.RUnlock()
	if ok {
		close(done2)
		t.Fatal("callback found a stale stopChs entry")
	}
}
