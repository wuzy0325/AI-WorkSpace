package core

// 前端事件名常量。所有 Service 推送事件时统一引用，避免字符串散落各处。
//
// 与 wista 的差异：
//   - 没有 EventPayload（wispa v0.3.0 已移除 payload 事件，改为前端 500ms 轮询
//     GetLatestSnapshot(s)，避免 Wails v3 Event.Emit 触发 WebView2 同步阻塞）
//   - 新增 EventRecordingWarning（多设备录制时单台断连的警告事件）
//   - 新增 EventDeviceState（adapter 状态变更回调，双参数事件 id + state）
//   - 没有 EventRecordingBackpressure / EventRecordingFatal（wispa 的 CSVRecorder
//     未实现这两个回调，背压通过 droppedCount 在 status 中体现）
const (
	EventLog               = "daq:log"                // 日志事件（App.EmitLog 推送）
	EventRecordingStatus   = "daq:recording-status"   // 录制状态变化（App.emitRecordingStatus 推送）
	EventRecordingWarning  = "daq:recording-warning"  // 录制期间设备断连警告（多设备场景，单台断连不停止录制）
	EventDeviceState       = "daq:device-state"       // 设备状态变更（App.EmitDeviceState 推送，双参数 id + state）
)

// EventBus 抽象"向前端推送事件"的能力。
//
// 设计动机：原 Wails v3 实现通过 application.App.Event.Emit 推送，
// Win7 分支改用 HTTP + WebSocket 后由 httpserver.WSHub 实现。
// backend 层依赖此抽象而非具体传输，便于在不同壳（Wails / Electron+WS）间复用。
type EventBus interface {
	// Emit 推送一个事件到所有订阅的前端连接。
	//   - name：事件名（见上方常量）
	//   - data：事件 payload，会被 JSON 序列化；多参数事件（如 daq:device-state）
	//     传两个元素，前端 onmessage 解构数组
	Emit(name string, data ...any)
}

// noopEventBus 在 EventBus 未注入时使用，丢弃所有事件，保证启动期不 panic。
type noopEventBus struct{}

func (noopEventBus) Emit(string, ...any) {}
