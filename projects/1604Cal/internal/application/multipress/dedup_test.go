package multipress

import (
	"testing"
	"time"
)

// TestProcessPollResultsDedupesUnchangedPressure 验证轮询值无变化时
// 不重复发布 multipress.pressure.update，避免空闲时 SSE 空转事件
// 持续制造前端渲染与分配压力。
func TestProcessPollResultsDedupesUnchangedPressure(t *testing.T) {
	var published []map[string]any
	svc := NewService(nil, nil, func(eventType string, data any) {
		if eventType != "multipress.pressure.update" {
			return
		}
		published = append(published, data.(map[string]any))
	})

	svc.entries["p1"] = &deviceEntry{
		state: DevicePressureState{DeviceID: "p1", Status: "idle"},
	}

	unchanged := pollResult{deviceID: "p1", pressure: 100.5, stable: true}

	// 第一次：首帧必须发布，保证新注册设备能立即收到初始值。
	svc.processPollResults([]pollResult{unchanged})
	if len(published) != 1 {
		t.Fatalf("expected first snapshot to publish, got %d events", len(published))
	}

	// 值无变化：不得发布。
	svc.processPollResults([]pollResult{unchanged})
	svc.processPollResults([]pollResult{unchanged})
	if len(published) != 1 {
		t.Fatalf("expected unchanged value to be deduped, got %d events", len(published))
	}

	// 压力变化：恢复发布。
	changed := pollResult{deviceID: "p1", pressure: 101.0, stable: true}
	svc.processPollResults([]pollResult{changed})
	if len(published) != 2 {
		t.Fatalf("expected changed pressure to publish, got %d events", len(published))
	}
	if published[1]["currentPressure"] != 101.0 {
		t.Fatalf("unexpected published pressure: %v", published[1]["currentPressure"])
	}

	// stable 标志变化：即使压力不变也要发布。
	stableChanged := pollResult{deviceID: "p1", pressure: 101.0, stable: false}
	svc.processPollResults([]pollResult{stableChanged})
	if len(published) != 3 {
		t.Fatalf("expected stability change to publish, got %d events", len(published))
	}
}

// TestProcessPollResultsSkipsPublishOnTransientError 验证偶发读数失败不发布
// pressure.update（与去重前的行为一致），连续失败达阈值才标记断连。
func TestProcessPollResultsSkipsPublishOnTransientError(t *testing.T) {
	publishCh := make(chan string, 8)
	svc := NewService(nil, nil, func(eventType string, data any) {
		if eventType == "multipress.pressure.update" {
			publishCh <- eventType
		}
	})

	entry := &deviceEntry{
		state: DevicePressureState{DeviceID: "p1", Status: "pressurizing"},
		lastPub: lastPublished{
			sent:     true,
			pressure: 100.5,
			stable:   true,
			status:   "pressurizing",
		},
	}
	svc.entries["p1"] = entry

	svc.processPollResults([]pollResult{{deviceID: "p1", err: errFakeRead}})

	select {
	case evt := <-publishCh:
		t.Fatalf("expected no pressure update on transient error, got %q", evt)
	case <-time.After(50 * time.Millisecond):
	}
}

type fakeReadError struct{}

func (fakeReadError) Error() string { return "io timeout" }

var errFakeRead = fakeReadError{}
